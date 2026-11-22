package server

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
	"github.com/metruzanca/caffeinator/agent"
	"github.com/metruzanca/caffeinator/internal/ratelimit"
	"github.com/metruzanca/caffeinator/internal/server/commands"
)

const (
	discordMessageLimit = 2000
	typingInterval      = 8 * time.Second
)

var (
	userMentionPattern       = regexp.MustCompile(`<@!?(\d+)>`)
	memoryInstructionPattern = regexp.MustCompile(`(?i)\b(remember|memorize|memorise)\b`)
)

type chatSession interface {
	ChannelMessage(channelID, messageID string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessages(channelID string, limit int, beforeID, afterID, aroundID string, options ...discordgo.RequestOption) ([]*discordgo.Message, error)
	ChannelTyping(channelID string, options ...discordgo.RequestOption) error
	ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

type chatAgent interface {
	Ask(ctx context.Context, req agent.Request) (agent.Response, error)
	Learn(ctx context.Context, turns []agent.Turn, subject agent.User, includeServer bool) ([]agent.Fact, error)
}

// ChatConfig controls how mention and ask chat behaves in Discord.
type ChatConfig struct {
	ContextMessages      int
	MaxChunks            int
	RateLimitCapacity    int
	RateLimitWindow      time.Duration
	Memory               *agent.Store
	Memes                *agent.MemeStore
	MemeDir              string
	MemoryExtractTimeout time.Duration
	PassiveMemory        bool
	VisionEnabled        bool
	VisionMaxImages      int
	VisionMaxImageMB     int
	VisionDetail         string
}

// Chat handles messages that mention the bot or reply to it.
type Chat struct {
	session   chatSession
	agent     chatAgent
	cfg       ChatConfig
	memory    *agent.Store
	reminders *ReminderService
	limiter   *ratelimit.Limiter
}

func newChat(session chatSession, ai chatAgent, cfg ChatConfig) *Chat {
	if cfg.ContextMessages < 1 {
		cfg.ContextMessages = 10
	}
	if cfg.ContextMessages > 100 {
		cfg.ContextMessages = 100
	}
	if cfg.MaxChunks <= 0 {
		cfg.MaxChunks = 3
	}
	if cfg.RateLimitCapacity < 1 {
		cfg.RateLimitCapacity = 2
	}
	if cfg.RateLimitWindow <= 0 {
		cfg.RateLimitWindow = 10 * time.Second
	}
	if cfg.MemoryExtractTimeout <= 0 {
		cfg.MemoryExtractTimeout = 30 * time.Second
	}
	if cfg.VisionMaxImages <= 0 {
		cfg.VisionMaxImages = 4
	}
	if cfg.VisionMaxImageMB <= 0 {
		cfg.VisionMaxImageMB = 8
	}
	switch strings.ToLower(strings.TrimSpace(cfg.VisionDetail)) {
	case "auto", "low", "high", "original":
		cfg.VisionDetail = strings.ToLower(strings.TrimSpace(cfg.VisionDetail))
	default:
		cfg.VisionDetail = "auto"
	}

	return &Chat{
		session: session,
		agent:   ai,
		cfg:     cfg,
		memory:  cfg.Memory,
		limiter: ratelimit.New(cfg.RateLimitCapacity, cfg.RateLimitWindow),
	}
}

// Handle implements discordgo's MessageCreate handler signature.
func (c *Chat) Handle(s *discordgo.Session, m *discordgo.MessageCreate) {
	if s == nil || s.State == nil || s.State.User == nil {
		return
	}
	serverAdmin := false
	if m != nil {
		serverAdmin = isGuildAdmin(s, m.Message)
	}
	c.handle(m, s.State.User.ID, serverAdmin)
}

func (c *Chat) handle(m *discordgo.MessageCreate, botID string, serverAdmin bool) {
	if m == nil || m.Message == nil || m.Author == nil || m.Author.Bot {
		return
	}
	if m.GuildID == "" || botID == "" {
		return
	}

	if c.reminders != nil && c.reminders.HandleMessage(context.Background(), m) {
		return
	}

	mentioned := mentionsUser(m, botID)
	hasReply := m.ReferencedMessage != nil || m.MessageReference != nil
	if !mentioned && !hasReply {
		return
	}

	referenced := c.referencedMessage(m)
	replyToBot := referenced != nil && referenced.Author != nil && referenced.Author.ID == botID
	if !mentioned && !replyToBot {
		return
	}

	if !c.limiter.Allow(m.Author.ID) {
		log.Debug("chat rate limit exceeded", "user", m.Author.ID, "channel", m.ChannelID)
		return
	}

	go c.respond(m, referenced, mentioned, botID, serverAdmin)
}

func (c *Chat) respond(m *discordgo.MessageCreate, referenced *discordgo.Message, mentioned bool, botID string, serverAdmin bool) {
	prompt := cleanContent(m, botID, mentioned)
	if prompt == "" && len(m.Attachments) == 0 {
		c.send(m, "What's up?", nil)
		return
	}

	stopTyping := c.startTyping(m.ChannelID)
	defer stopTyping()

	extractMemory := !c.cfg.PassiveMemory || (serverAdmin && wantsMemoryUpdate(prompt))
	history := c.gatherContext(m, referenced, c.cfg.ContextMessages)
	turns := toAgentTurns(history)
	req := agent.Request{
		ID:          m.ID,
		Self:        agent.User{ID: botID},
		User:        toAgentUser(m.Author),
		Prompt:      prompt,
		Timestamp:   m.Timestamp,
		History:     turns,
		Attachments: toAgentAttachments(m.Attachments),
		Mentions:    requestMentions(m, referenced, prompt, botID),
		Images:      c.visionImages(m, referenced),
		UseMemory:   true,
	}

	log.Info("chat request",
		"guild", m.GuildID,
		"channel", m.ChannelID,
		"user", m.Author.ID,
		"history", len(turns),
		"images", len(req.Images),
	)

	started := time.Now()
	response, err := c.agent.Ask(context.Background(), req)
	if err != nil {
		log.Error("chat request failed",
			"err", err,
			"guild", m.GuildID,
			"channel", m.ChannelID,
			"user", m.Author.ID,
		)
		c.send(m, "My brain is offline right now. Try again in a bit.", nil)
		return
	}

	logResponse(m, started, response)

	if extractMemory && c.memory != nil {
		learningTurns := append(append([]agent.Turn{}, turns...), agent.Turn{
			ID:          m.ID,
			Author:      req.User,
			Content:     prompt,
			Timestamp:   m.Timestamp,
			Attachments: req.Attachments,
		})
		go c.learn(learningTurns, req.User, serverAdmin)
	}

	c.send(m, response.Text, response.Memes)
}

// Ask runs the chat pipeline for a slash command interaction.
func (c *Chat) Ask(ctx context.Context, req commands.ChatRequest) (commands.ChatReply, error) {
	if c == nil {
		return commands.ChatReply{}, errors.New("chat is disabled")
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		return commands.ChatReply{}, errors.New("text is empty")
	}

	historyLength := req.HistoryLength
	if historyLength < 1 {
		historyLength = c.cfg.ContextMessages
	}
	if historyLength > 100 {
		historyLength = 100
	}

	author := agent.User{
		ID:   req.UserID,
		Name: req.DisplayName,
	}
	now := time.Now()
	message := &discordgo.MessageCreate{Message: &discordgo.Message{
		ChannelID: req.ChannelID,
		GuildID:   req.GuildID,
		Author:    &discordgo.User{ID: req.UserID, Username: req.DisplayName, GlobalName: req.DisplayName},
		Content:   text,
		Timestamp: now,
	}}

	history := c.gatherContext(message, nil, historyLength)
	turns := toAgentTurns(history)
	areq := agent.Request{
		Self:      agent.User{ID: req.BotID},
		User:      author,
		Prompt:    text,
		Timestamp: now,
		History:   turns,
		Mentions:  requestMentions(nil, nil, text, req.BotID),
		UseMemory: !req.DisableMemory,
	}

	started := time.Now()
	response, err := c.agent.Ask(ctx, areq)
	if err != nil {
		return commands.ChatReply{}, err
	}

	log.Info("ask response",
		"guild", req.GuildID,
		"channel", req.ChannelID,
		"user", req.UserID,
		"duration", time.Since(started),
		"prompt_tokens", response.Usage.PromptTokens,
		"completion_tokens", response.Usage.CompletionTokens,
	)

	if !req.DisableMemory && c.memory != nil {
		learningTurns := append(append([]agent.Turn{}, turns...), agent.Turn{
			Author:    author,
			Content:   text,
			Timestamp: now,
		})
		go c.learn(learningTurns, author, false)
	}

	return commands.ChatReply{Text: response.Text, Memes: response.Memes}, nil
}

func (c *Chat) learn(turns []agent.Turn, user agent.User, includeServer bool) {
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.MemoryExtractTimeout)
	defer cancel()

	facts, err := c.agent.Learn(ctx, turns, user, includeServer)
	if err != nil {
		log.Error("memory extraction failed", "err", err)
		return
	}
	if len(facts) == 0 {
		log.Debug("memory extraction found nothing")
		return
	}
	log.Info("memory updated", "facts", len(facts), "server", includeServer)
}

func logResponse(m *discordgo.MessageCreate, started time.Time, response agent.Response) {
	log.Info("chat response",
		"guild", m.GuildID,
		"channel", m.ChannelID,
		"user", m.Author.ID,
		"duration", time.Since(started),
		"messages", response.Stats.Messages,
		"user_messages", response.Stats.UserMessages,
		"assistant_messages", response.Stats.AssistantMessages,
		"context_chars", response.Stats.ContextChars,
		"memory_subjects", response.Stats.MemorySubjects,
		"meme_candidates", response.Stats.MemeCandidates,
		"memes_used", len(response.Memes),
		"prompt_tokens", response.Usage.PromptTokens,
		"completion_tokens", response.Usage.CompletionTokens,
		"web_search_requests", response.Usage.ServerToolUseDetails.WebSearchRequests,
	)
}

func (c *Chat) referencedMessage(m *discordgo.MessageCreate) *discordgo.Message {
	if m.ReferencedMessage != nil {
		return m.ReferencedMessage
	}
	if m.MessageReference == nil || m.MessageReference.MessageID == "" {
		return nil
	}

	channelID := m.MessageReference.ChannelID
	if channelID == "" {
		channelID = m.ChannelID
	}

	referenced, err := c.session.ChannelMessage(channelID, m.MessageReference.MessageID)
	if err != nil {
		log.Debug("cannot fetch referenced message",
			"err", err,
			"channel", channelID,
			"message", m.MessageReference.MessageID,
		)
		return nil
	}
	return referenced
}

func (c *Chat) gatherContext(m *discordgo.MessageCreate, referenced *discordgo.Message, limit int) []*discordgo.Message {
	if limit < 1 {
		limit = c.cfg.ContextMessages
	}
	if limit > 100 {
		limit = 100
	}

	if referenced != nil {
		channelID := referenced.ChannelID
		if channelID == "" {
			channelID = m.ChannelID
		}

		priorLimit := limit - 1
		if priorLimit < 1 {
			return []*discordgo.Message{referenced}
		}

		prior, err := c.session.ChannelMessages(channelID, priorLimit, referenced.ID, "", "")
		if err == nil {
			history := reverseMessages(prior)
			return append(history, referenced)
		}

		log.Debug("cannot fetch reply context, falling back to current channel",
			"err", err,
			"channel", channelID,
			"message", referenced.ID,
		)
	}

	prior, err := c.session.ChannelMessages(m.ChannelID, limit, m.ID, "", "")
	if err != nil {
		log.Debug("cannot fetch channel context", "err", err, "channel", m.ChannelID)
		return nil
	}
	return reverseMessages(prior)
}

func reverseMessages(messages []*discordgo.Message) []*discordgo.Message {
	reversed := make([]*discordgo.Message, len(messages))
	for i, message := range messages {
		reversed[len(messages)-1-i] = message
	}
	return reversed
}

func toAgentUser(user *discordgo.User) agent.User {
	if user == nil {
		return agent.User{Name: "unknown"}
	}
	return agent.User{
		ID:   user.ID,
		Name: displayName(user),
		Bot:  user.Bot,
	}
}

func toAgentTurn(message *discordgo.Message) agent.Turn {
	if message == nil {
		return agent.Turn{}
	}
	return agent.Turn{
		ID:          message.ID,
		Author:      toAgentUser(message.Author),
		Content:     message.Content,
		Timestamp:   message.Timestamp,
		Attachments: toAgentAttachments(message.Attachments),
	}
}

func toAgentTurns(messages []*discordgo.Message) []agent.Turn {
	turns := make([]agent.Turn, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		turns = append(turns, toAgentTurn(message))
	}
	return turns
}

func toAgentAttachments(attachments []*discordgo.MessageAttachment) []agent.Attachment {
	result := make([]agent.Attachment, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		result = append(result, agent.Attachment{
			Filename:  attachment.Filename,
			MediaType: attachment.ContentType,
			URL:       attachment.URL,
			Size:      int64(attachment.Size),
		})
	}
	return result
}

func requestMentions(m *discordgo.MessageCreate, referenced *discordgo.Message, prompt, botID string) []agent.User {
	seen := make(map[string]bool)
	users := make([]agent.User, 0)

	add := func(user *discordgo.User) {
		if user == nil || user.ID == "" || user.ID == botID || seen[user.ID] {
			return
		}
		seen[user.ID] = true
		users = append(users, toAgentUser(user))
	}

	if m != nil {
		for _, user := range m.Mentions {
			add(user)
		}
	}
	if referenced != nil {
		add(referenced.Author)
	}
	for _, match := range userMentionPattern.FindAllStringSubmatch(prompt, -1) {
		id := match[1]
		if id == botID || seen[id] {
			continue
		}
		seen[id] = true
		users = append(users, agent.User{ID: id, Name: id})
	}

	return users
}

func displayName(user *discordgo.User) string {
	if user == nil {
		return "unknown"
	}
	if name := strings.TrimSpace(user.DisplayName()); name != "" {
		return name
	}
	return user.ID
}

func mentionsUser(m *discordgo.MessageCreate, userID string) bool {
	for _, user := range m.Mentions {
		if user != nil && user.ID == userID {
			return true
		}
	}
	return strings.Contains(m.Content, "<@"+userID+">") || strings.Contains(m.Content, "<@!"+userID+">")
}

func cleanContent(m *discordgo.MessageCreate, botID string, mentioned bool) string {
	content := m.Content
	if mentioned {
		content = strings.ReplaceAll(content, "<@"+botID+">", "")
		content = strings.ReplaceAll(content, "<@!"+botID+">", "")
	}
	return strings.TrimSpace(content)
}

// wantsMemoryUpdate reports whether a message explicitly asks the bot to
// remember something. Server memory writes require an explicit request so
// ordinary admin chatter does not become server facts.
func wantsMemoryUpdate(text string) bool {
	return memoryInstructionPattern.MatchString(text)
}

// isGuildAdmin reports whether the message author is an administrator or the
// guild owner. It fails closed so a cold cache never grants server memory
// writes.
func isGuildAdmin(s *discordgo.Session, m *discordgo.Message) bool {
	if s == nil || m == nil || m.Author == nil || m.GuildID == "" {
		return false
	}
	if s.State != nil {
		if perms, err := s.State.MessagePermissions(m); err == nil {
			return perms&discordgo.PermissionAdministrator != 0
		}
	}

	member, err := s.GuildMember(m.GuildID, m.Author.ID)
	if err != nil || member == nil {
		return false
	}
	guild, err := s.Guild(m.GuildID)
	if err != nil || guild == nil {
		return false
	}
	if guild.OwnerID == m.Author.ID {
		return true
	}
	for _, roleID := range member.Roles {
		for _, role := range guild.Roles {
			if role.ID == roleID && role.Permissions&discordgo.PermissionAdministrator != 0 {
				return true
			}
		}
	}
	return false
}

func (c *Chat) startTyping(channelID string) func() {
	stop := make(chan struct{})

	go func() {
		if err := c.session.ChannelTyping(channelID); err != nil {
			log.Debug("cannot send typing indicator", "err", err, "channel", channelID)
		}

		ticker := time.NewTicker(typingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if err := c.session.ChannelTyping(channelID); err != nil {
					log.Debug("cannot refresh typing indicator", "err", err, "channel", channelID)
				}
			}
		}
	}()

	return func() {
		close(stop)
	}
}

func (c *Chat) send(m *discordgo.MessageCreate, content string, selected []agent.Meme) {
	var file *discordgo.File
	memeID := ""
	if len(selected) > 0 {
		file, memeID, _ = loadMemeImage(c.cfg.MemeDir, selected)
	}

	chunks := splitMessage(content, discordMessageLimit, c.cfg.MaxChunks)
	if len(chunks) == 0 {
		if file == nil {
			log.Warn("empty chat response", "channel", m.ChannelID, "user", m.Author.ID)
			return
		}
		chunks = []string{""}
	}

	failIfNotExists := false
	for index, chunk := range chunks {
		var reference *discordgo.MessageReference
		if index == 0 {
			reference = &discordgo.MessageReference{
				MessageID:       m.ID,
				ChannelID:       m.ChannelID,
				GuildID:         m.GuildID,
				FailIfNotExists: &failIfNotExists,
			}
		}

		message := &discordgo.MessageSend{
			Content:   chunk,
			Reference: reference,
			AllowedMentions: &discordgo.MessageAllowedMentions{
				Parse:       []discordgo.AllowedMentionType{discordgo.AllowedMentionTypeUsers},
				RepliedUser: false,
			},
		}
		if index == 0 && file != nil {
			message.Files = []*discordgo.File{file}
		}

		sent, err := c.session.ChannelMessageSendComplex(m.ChannelID, message)
		if err != nil {
			log.Error("cannot send chat response", "err", err, "channel", m.ChannelID)
			return
		}
		if index == 0 && file != nil && memeID != "" && c.cfg.Memes != nil && sent != nil {
			if err := c.cfg.Memes.MarkShared(memeID, m.ChannelID, sent.ID); err != nil {
				log.Warn("cannot record shared meme", "meme", memeID, "err", err)
			}
		}
	}
}

func splitMessage(content string, limit, maxChunks int) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if limit <= 0 {
		limit = discordMessageLimit
	}
	if maxChunks < 1 {
		maxChunks = 1
	}

	remaining := []rune(content)
	var chunks []string

	for len(remaining) > limit && len(chunks) < maxChunks-1 {
		window := remaining[:limit]
		cut := -1
		for i := len(window) - 1; i >= 0; i-- {
			if window[i] == '\n' {
				cut = i
				break
			}
		}
		if cut <= 0 {
			cut = limit
		}

		chunk := strings.TrimSpace(string(remaining[:cut]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		remaining = []rune(strings.TrimSpace(string(remaining[cut:])))
	}

	if len(remaining) > limit {
		remaining = []rune(truncateRunes(string(remaining), limit))
	}
	if chunk := strings.TrimSpace(string(remaining)); chunk != "" {
		chunks = append(chunks, chunk)
	}

	return chunks
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if max <= 0 || len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func isDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// String is useful in logs and tests without exposing message contents.
func (c *Chat) String() string {
	return fmt.Sprintf("chat(context=%d)", c.cfg.ContextMessages)
}
