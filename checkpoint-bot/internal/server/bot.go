package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
	"github.com/metruzanca/checkpoint-bot/internal/imagegen"
	"github.com/metruzanca/checkpoint-bot/internal/llm"
	"github.com/metruzanca/checkpoint-bot/internal/memory"
	"github.com/metruzanca/checkpoint-bot/internal/ratelimit"
	"github.com/metruzanca/checkpoint-bot/internal/server/commands"
	"github.com/metruzanca/checkpoint-bot/internal/util"
	"github.com/metruzanca/checkpoint-bot/internal/version"
	"github.com/spf13/viper"
)

const defaultSystemPrompt = "You are Caffeinator, a Discord bot for The Coffee Break. You are built with Go and discordgo, and deployed on Railway. Chat runs through OpenRouter with DeepSeek V4.1 Flash 1017 by default, with optional web search. You can generate images with the /imagine command. If someone asks for an image in chat, tell them to use /imagine. They can attach a reference image to edit or restyle it. When you use web search, cite your sources inline with Discord markdown links: [short relevant phrase](url). Link a few relevant words, not a raw URL. Use the source domain as the link text when no better phrase fits. Use at most two citations unless the user asks for more. Never invent a URL. If you did not search, do not add citations. Citations count toward the tweet limit, so keep the prose short. Discord formatting: refer to users with mentions like <@1234567890>. Never write a raw user ID. Refer to channels with channel mentions like <#1234567890>. Do not use #[1234567890]. Refer to times with Discord timestamps: <t:1795222140:R> for relative, <t:1795222140:f> for full date and time. Use Unix seconds, not milliseconds. Never use @everyone, @here, or role mentions. Do not wrap mentions or timestamps in backticks. When images are attached, use them to answer. Do not invent details that are not visible. If an image is unclear or unavailable, say so. You have persistent markdown memory. It updates automatically from normal messages in this server and from conversations where you are invoked. You do not need to be mentioned for memory to update. You do not need a memory tool. People teach you facts about themselves. Admins can also teach you facts about the server. If someone who is not an admin asks you to remember something about the server, say that only admins can change server memory. If someone asks you to remember something about another person, say you only remember what people share about themselves. Never claim to have remembered something you cannot store. Relevant memory is loaded into your context. Do not say you cannot remember between sessions. If a fact is missing, say you do not have it yet. Reply like a peer, not a support agent or instructor. Be casual and direct, and assume competence. Never use em dashes. Keep sentences short and simple. If a thought would need an em dash, split it into separate sentences. Keep replies to a single tweet at most: 280 characters or fewer, unless the user explicitly asks for more detail, a longer response, or a list. Do not add unsolicited advice, action items, checklists, or step-by-step remediation. Do not explain basics or routine steps unless asked. If the user only asks whether something is true, answer that and stop. If the user asks what to do, answer concisely without lecturing."

// Bot owns the Discord session, slash commands, and mention chat.
type Bot struct {
	DiscordClient   *discordgo.Session
	CommandHandler  *commands.CommandHandler
	chat            *Chat
	memoryCollector *passiveCollector
}

// NewBot creates a Discord session with the intents required for slash
// commands and reading message context.
func NewBot(token string) *Bot {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatal("cannot create Discord session", "err", err)
	}

	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentMessageContent

	return &Bot{
		DiscordClient: session,
	}
}

var startTime time.Time

func init() {
	startTime = time.Now()
}

// Start connects to Discord and registers handlers.
func (b *Bot) Start() error {
	b.DiscordClient.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Info("logged in", "username", s.State.User.Username, "bot_id", s.State.User.ID, "build", version.Describe())

		if !viper.GetBool("STARTUP_MESSAGE") {
			return
		}

		fmt.Println("Invite me to a server: ", util.GetInviteLink(s.State.User.ID))

		for _, guild := range b.DiscordClient.State.Guilds {
			if guild.Name == "" {
				// The guild cache is sometimes incomplete right after login.
				if _, err := b.DiscordClient.Guild(guild.ID); err != nil {
					log.Warn("cannot fetch guild details", "guild", guild.ID, "err", err)
				}
			}

			log.Info("guild", "id", guild.ID, "name", guild.Name, "member_count", guild.MemberCount)
		}

		for _, cmd := range commands.GetAvailableCommands() {
			log.Info("command", "name", cmd.Name, "description", cmd.Description)
		}

		channelID := viper.GetString("CHANNEL_ID")
		if channelID != "" {
			b.SendTextMessage(channelID, restartNotice())
		}
	})

	if chat := newChatFromConfig(b.DiscordClient); chat != nil {
		b.chat = chat
		b.DiscordClient.AddHandler(chat.Handle)
	}

	if b.chat != nil && b.chat.memory != nil && b.chat.extractor != nil && viper.GetBool("MEMORY_PASSIVE_ENABLED") {
		cfg := passiveCollectorConfig{
			Interval:         time.Duration(viper.GetInt("MEMORY_BATCH_SECONDS")) * time.Second,
			MaxBatchMessages: viper.GetInt("MEMORY_MAX_BATCH_MESSAGES"),
			ExtractTimeout:   time.Duration(viper.GetInt("MEMORY_EXTRACT_TIMEOUT_SECONDS")) * time.Second,
			IgnoreChannels:   parseChannelSet(viper.GetString("MEMORY_IGNORE_CHANNELS")),
		}
		b.memoryCollector = newPassiveCollector(b.chat.memory, b.chat.extractor, cfg)
		b.memoryCollector.Start()
		b.DiscordClient.AddHandler(b.memoryCollector.Handle)
		log.Info("passive memory enabled",
			"interval", cfg.Interval,
			"ignore_channels", len(cfg.IgnoreChannels),
		)
	}

	if err := b.DiscordClient.Open(); err != nil {
		return fmt.Errorf("cannot open Discord session: %w", err)
	}

	b.CommandHandler = commands.NewCommandHandler(b.DiscordClient)
	if b.chat != nil {
		b.CommandHandler.Chat = b.chat
		if b.chat.memory != nil {
			b.CommandHandler.Memory = b.chat.memory
		}
	}
	if imageClient := newImageClientFromConfig(); imageClient != nil {
		b.CommandHandler.Images = imageClient
		b.CommandHandler.ImageModel = imageClient.Model
		b.CommandHandler.ImageLimiter = ratelimit.New(viper.GetInt("IMAGE_RATE_LIMIT_PER_HOUR"), time.Hour)
		b.CommandHandler.ImageResolution = viper.GetString("IMAGE_DEFAULT_RESOLUTION")
		b.CommandHandler.ImageAspectRatio = viper.GetString("IMAGE_DEFAULT_ASPECT_RATIO")
		b.CommandHandler.ImageMaxReferenceBytes = int64(viper.GetInt("IMAGE_MAX_REFERENCE_MB")) << 20
		b.CommandHandler.ImageTimeout = time.Duration(viper.GetInt("IMAGE_TIMEOUT_SECONDS")) * time.Second
	}
	b.CommandHandler.RegisterCommands()

	b.DiscordClient.AddHandler(b.onGuildJoined)

	return nil
}

func newChatFromConfig(session *discordgo.Session) *Chat {
	if !viper.GetBool("CHAT_ENABLED") {
		log.Info("chat disabled")
		return nil
	}

	apiKey := strings.TrimSpace(viper.GetString("OPENROUTER_API_KEY"))
	if apiKey == "" {
		log.Warn("chat disabled: OPENROUTER_API_KEY is not set")
		return nil
	}

	model := strings.TrimSpace(viper.GetString("OPENROUTER_MODEL"))
	if model == "" {
		model = "deepseek/deepseek-v4.1-flash-1017"
	}

	baseURL := strings.TrimSpace(viper.GetString("OPENROUTER_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	systemPrompt := strings.TrimSpace(viper.GetString("CHAT_SYSTEM_PROMPT"))
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}

	var memoryStore *memory.Store
	var memoryExtractor *memory.Extractor
	if viper.GetBool("MEMORY_ENABLED") {
		var err error
		memoryStore, err = memory.NewStore(
			viper.GetString("MEMORY_DIR"),
			viper.GetInt("MEMORY_MAX_CONTEXT_CHARS"),
		)
		if err != nil {
			log.Error("memory disabled", "err", err)
			memoryStore = nil
		}
	}
	if memoryStore != nil {
		extractorClient := &llm.Client{
			APIKey:         apiKey,
			Model:          model,
			BaseURL:        baseURL,
			MaxTokens:      4096,
			SystemPrompt:   memory.ExtractionSystemPrompt,
			ResponseFormat: &llm.ResponseFormat{Type: "json_object"},
		}
		memoryExtractor = memory.NewExtractor(extractorClient, viper.GetFloat64("MEMORY_MIN_CONFIDENCE"))
	}

	webSearchMaxResults := viper.GetInt("OPENROUTER_WEB_SEARCH_MAX_RESULTS")
	if webSearchMaxResults < 1 {
		webSearchMaxResults = 3
	}
	webSearchMaxUses := viper.GetInt("OPENROUTER_WEB_SEARCH_MAX_USES")
	if webSearchMaxUses < 1 {
		webSearchMaxUses = 2
	}

	client := &llm.Client{
		APIKey:              apiKey,
		Model:               model,
		BaseURL:             baseURL,
		MaxTokens:           viper.GetInt("CHAT_MAX_TOKENS"),
		SystemPrompt:        systemPrompt,
		WebSearch:           viper.GetBool("OPENROUTER_WEB_SEARCH"),
		WebSearchMaxResults: webSearchMaxResults,
		WebSearchMaxUses:    webSearchMaxUses,
	}

	cfg := ChatConfig{
		ContextMessages:       viper.GetInt("CHAT_CONTEXT_MESSAGES"),
		MaxContextChars:       viper.GetInt("CHAT_MAX_CONTEXT_CHARS"),
		MaxCharsPerMessage:    viper.GetInt("CHAT_MAX_CHARS_PER_MESSAGE"),
		MaxChunks:             viper.GetInt("CHAT_MAX_CHUNKS"),
		Timeout:               time.Duration(viper.GetInt("CHAT_TIMEOUT_SECONDS")) * time.Second,
		RateLimitCapacity:     viper.GetInt("CHAT_RATE_LIMIT_CAPACITY"),
		RateLimitWindow:       time.Duration(viper.GetInt("CHAT_RATE_LIMIT_SECONDS")) * time.Second,
		MaxConcurrent:         viper.GetInt("CHAT_MAX_CONCURRENT"),
		SystemPrompt:          systemPrompt,
		Memory:                memoryStore,
		Extractor:             memoryExtractor,
		MemoryMaxContextChars: viper.GetInt("MEMORY_MAX_CONTEXT_CHARS"),
		MemoryExtractTimeout:  time.Duration(viper.GetInt("MEMORY_EXTRACT_TIMEOUT_SECONDS")) * time.Second,
		PassiveMemory:         viper.GetBool("MEMORY_PASSIVE_ENABLED"),
		VisionEnabled:         viper.GetBool("VISION_ENABLED"),
		VisionMaxImages:       viper.GetInt("VISION_MAX_IMAGES"),
		VisionMaxImageMB:      viper.GetInt("VISION_MAX_IMAGE_MB"),
		VisionDetail:          viper.GetString("VISION_DETAIL"),
	}

	memoryEnabled := memoryStore != nil
	memoryDir := ""
	if memoryEnabled {
		memoryDir = memoryStore.Dir()
	}

	log.Info("chat enabled",
		"model", model,
		"system_prompt_chars", len([]rune(systemPrompt)),
		"web_search", client.WebSearch,
		"web_search_max_results", client.WebSearchMaxResults,
		"web_search_max_uses", client.WebSearchMaxUses,
		"memory_enabled", memoryEnabled,
		"memory_dir", memoryDir,
	)
	return newChat(session, client, cfg)
}

func newImageClientFromConfig() *imagegen.Client {
	if !viper.GetBool("IMAGE_ENABLED") {
		log.Info("image generation disabled")
		return nil
	}

	apiKey := strings.TrimSpace(viper.GetString("OPENROUTER_API_KEY"))
	if apiKey == "" {
		log.Warn("image generation disabled: OPENROUTER_API_KEY is not set")
		return nil
	}

	model := strings.TrimSpace(viper.GetString("IMAGE_MODEL"))
	if model == "" {
		model = "bytedance-seed/seedream-5-0-flash"
	}

	baseURL := strings.TrimSpace(viper.GetString("OPENROUTER_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	log.Info("image generation enabled", "model", model)
	return &imagegen.Client{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: baseURL,
	}
}

func (b *Bot) onGuildJoined(s *discordgo.Session, g *discordgo.GuildCreate) {
	log.Info("bot added to guild", "guild_id", g.ID, "guild_name", g.Name, "member_count", g.MemberCount)
	b.CommandHandler.RegisterCommandsForGuild(g.ID)
}

// UnregisterCommands removes every slash command from every guild.
func (b *Bot) UnregisterCommands() {
	for _, guild := range b.DiscordClient.State.Guilds {
		registeredCommands, err := b.DiscordClient.ApplicationCommands(b.DiscordClient.State.User.ID, guild.ID)
		if err != nil {
			log.Error("failed to get registered commands", "guild", guild.ID, "err", err)
			continue
		}
		for _, cmd := range registeredCommands {
			if err := b.DiscordClient.ApplicationCommandDelete(b.DiscordClient.State.User.ID, guild.ID, cmd.ID); err != nil {
				log.Error("failed to delete command", "command", cmd.Name, "guild", guild.ID, "err", err)
			}
		}
	}

	log.Info("commands unregistered")
}

// Stop closes the Discord session.
func (b *Bot) Stop() {
	if b.memoryCollector != nil {
		b.memoryCollector.Stop()
	}
	if err := b.DiscordClient.Close(); err != nil {
		log.Error("cannot close Discord session", "err", err)
	}
	log.Info("bot stopped gracefully", "runtime", time.Since(startTime))
}

// restartNotice describes the build that just came up.
func restartNotice() string {
	return formatNotice(version.SHA(), version.Subject())
}

func formatNotice(sha, subject string) string {
	switch {
	case sha != "" && subject != "":
		return "Bot has restarted. Built from `" + sha + "` " + subject
	case sha != "":
		return "Bot has restarted. Built from `" + sha + "`"
	case subject != "":
		return "Bot has restarted. Built from " + subject
	default:
		return "Bot has restarted."
	}
}

// SendTextMessage sends a plain text message to a channel. Mentions are
// suppressed because startup notices can include commit messages.
func (b *Bot) SendTextMessage(channelID, message string) {
	data := &discordgo.MessageSend{
		Content:         message,
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
	}
	if _, err := b.DiscordClient.ChannelMessageSendComplex(channelID, data); err != nil {
		log.Error("cannot send message", "err", err, "channel", channelID)
		return
	}
	log.Info("message sent", "channel", channelID)
}

func parseChannelSet(raw string) map[string]bool {
	channels := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			channels[part] = true
		}
	}
	return channels
}
