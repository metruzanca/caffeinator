package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
	"github.com/metruzanca/checkpoint-bot/internal/llm"
	"github.com/metruzanca/checkpoint-bot/internal/ratelimit"
)

const (
	discordMessageLimit = 2000
	typingInterval      = 8 * time.Second
)

type chatSession interface {
	ChannelMessage(channelID, messageID string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessages(channelID string, limit int, beforeID, afterID, aroundID string, options ...discordgo.RequestOption) ([]*discordgo.Message, error)
	ChannelTyping(channelID string, options ...discordgo.RequestOption) error
	ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

type chatClient interface {
	Chat(ctx context.Context, messages []llm.Message) (string, llm.Usage, error)
}

// ChatConfig controls how mention chat behaves.
type ChatConfig struct {
	ContextMessages    int
	MaxContextChars    int
	MaxCharsPerMessage int
	MaxChunks          int
	Timeout            time.Duration
	RateLimitCapacity  int
	RateLimitWindow    time.Duration
	MaxConcurrent      int
}

// Chat handles messages that mention the bot or reply to it.
type Chat struct {
	session chatSession
	client  chatClient
	cfg     ChatConfig
	limiter *ratelimit.Limiter
	sem     chan struct{}
}

func newChat(session chatSession, client chatClient, cfg ChatConfig) *Chat {
	if cfg.ContextMessages < 1 {
		cfg.ContextMessages = 10
	}
	if cfg.ContextMessages > 100 {
		cfg.ContextMessages = 100
	}
	if cfg.MaxContextChars <= 0 {
		cfg.MaxContextChars = 20000
	}
	if cfg.MaxCharsPerMessage <= 0 {
		cfg.MaxCharsPerMessage = 4000
	}
	if cfg.MaxChunks <= 0 {
		cfg.MaxChunks = 3
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.RateLimitCapacity < 1 {
		cfg.RateLimitCapacity = 2
	}
	if cfg.RateLimitWindow <= 0 {
		cfg.RateLimitWindow = 10 * time.Second
	}
	if cfg.MaxConcurrent < 1 {
		cfg.MaxConcurrent = 4
	}

	return &Chat{
		session: session,
		client:  client,
		cfg:     cfg,
		limiter: ratelimit.New(cfg.RateLimitCapacity, cfg.RateLimitWindow),
		sem:     make(chan struct{}, cfg.MaxConcurrent),
	}
}

// Handle implements discordgo's MessageCreate handler signature.
func (c *Chat) Handle(s *discordgo.Session, m *discordgo.MessageCreate) {
	if s == nil || s.State == nil || s.State.User == nil {
		return
	}
	c.handle(m, s.State.User.ID)
}

func (c *Chat) handle(m *discordgo.MessageCreate, botID string) {
	if m == nil || m.Message == nil || m.Author == nil || m.Author.Bot {
		return
	}
	if m.GuildID == "" || botID == "" {
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

	go c.respond(m, referenced, mentioned, botID)
}

func (c *Chat) respond(m *discordgo.MessageCreate, referenced *discordgo.Message, mentioned bool, botID string) {
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-time.After(5 * time.Second):
		c.send(m, "I'm a bit busy right now. Try again in a moment.")
		return
	}

	prompt := cleanContent(m, botID, mentioned)
	if prompt == "" && len(m.Attachments) == 0 {
		c.send(m, "What's up?")
		return
	}

	stopTyping := c.startTyping(m.ChannelID)
	defer stopTyping()

	history := c.gatherContext(m, referenced)
	messages := buildMessages(history, m, prompt, botID, c.cfg)
	if len(messages) == 0 {
		c.send(m, "I couldn't read the channel history.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.Timeout)
	defer cancel()

	log.Info("chat request",
		"guild", m.GuildID,
		"channel", m.ChannelID,
		"user", m.Author.ID,
		"messages", len(messages),
	)

	started := time.Now()
	content, usage, err := c.client.Chat(ctx, messages)
	if err != nil {
		log.Error("chat request failed",
			"err", err,
			"guild", m.GuildID,
			"channel", m.ChannelID,
			"user", m.Author.ID,
			"duration", time.Since(started),
		)
		c.send(m, "My brain is offline right now. Try again in a bit.")
		return
	}

	log.Info("chat response",
		"guild", m.GuildID,
		"channel", m.ChannelID,
		"user", m.Author.ID,
		"duration", time.Since(started),
		"prompt_tokens", usage.PromptTokens,
		"completion_tokens", usage.CompletionTokens,
	)

	c.send(m, content)
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

func (c *Chat) gatherContext(m *discordgo.MessageCreate, referenced *discordgo.Message) []*discordgo.Message {
	limit := c.cfg.ContextMessages
	if limit < 1 {
		limit = 1
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

func buildMessages(history []*discordgo.Message, current *discordgo.MessageCreate, prompt, botID string, cfg ChatConfig) []llm.Message {
	if cfg.MaxCharsPerMessage <= 0 {
		cfg.MaxCharsPerMessage = 4000
	}
	if cfg.MaxContextChars <= 0 {
		cfg.MaxContextChars = 20000
	}

	var messages []llm.Message

	for _, message := range history {
		if message == nil || message.Author == nil || message.ID == current.ID {
			continue
		}

		text := messageText(message)
		if text == "" {
			continue
		}

		role := "user"
		if message.Author.ID == botID {
			role = "assistant"
		} else {
			text = displayName(message.Author) + ": " + text
		}

		messages = append(messages, llm.Message{
			Role:    role,
			Content: truncateRunes(text, cfg.MaxCharsPerMessage),
		})
	}

	currentText := prompt
	if attachmentText := attachmentSummary(current.Attachments); attachmentText != "" {
		if currentText != "" {
			currentText += "\n"
		}
		currentText += attachmentText
	}
	if currentText == "" {
		return messages
	}

	messages = append(messages, llm.Message{
		Role:    "user",
		Content: truncateRunes(displayName(current.Author)+": "+currentText, cfg.MaxCharsPerMessage),
	})

	total := 0
	for _, message := range messages {
		total += len([]rune(message.Content))
	}
	for total > cfg.MaxContextChars && len(messages) > 1 {
		total -= len([]rune(messages[0].Content))
		messages = messages[1:]
	}
	if total > cfg.MaxContextChars && len(messages) == 1 {
		messages[0].Content = truncateRunes(messages[0].Content, cfg.MaxContextChars)
	}

	return messages
}

func messageText(message *discordgo.Message) string {
	text := strings.TrimSpace(message.Content)
	if attachmentText := attachmentSummary(message.Attachments); attachmentText != "" {
		if text != "" {
			text += "\n"
		}
		text += attachmentText
	}
	return strings.TrimSpace(text)
}

func attachmentSummary(attachments []*discordgo.MessageAttachment) string {
	if len(attachments) == 0 {
		return ""
	}

	names := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		name := attachment.Filename
		if name == "" {
			name = "attachment"
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
	}
	return "[attachments: " + strings.Join(names, ", ") + "]"
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

func (c *Chat) send(m *discordgo.MessageCreate, content string) {
	chunks := splitMessage(content, discordMessageLimit, c.cfg.MaxChunks)
	if len(chunks) == 0 {
		log.Warn("empty chat response", "channel", m.ChannelID, "user", m.Author.ID)
		return
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

		_, err := c.session.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Content:   chunk,
			Reference: reference,
			AllowedMentions: &discordgo.MessageAllowedMentions{
				Parse:       []discordgo.AllowedMentionType{},
				RepliedUser: false,
			},
		})
		if err != nil {
			log.Error("cannot send chat response", "err", err, "channel", m.ChannelID)
			return
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

// String is useful in logs and tests without exposing message contents.
func (c *Chat) String() string {
	return fmt.Sprintf("chat(context=%d, timeout=%s)", c.cfg.ContextMessages, c.cfg.Timeout)
}
