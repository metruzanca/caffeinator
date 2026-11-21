package commands

import (
	"context"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
)

// AskCmd runs the chat pipeline with configurable history and memory.
var AskCmd = &Command{
	ApplicationCommand: discordgo.ApplicationCommand{
		Name:        "ask",
		Description: "Ask Caffeinator with a custom amount of channel context",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "text",
				Description: "What to ask",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "history_length",
				Description: "How many channel messages to include (1-100, default 10)",
				Required:    false,
				MinValue:    float64Ptr(1),
				MaxValue:    100,
			},
			{
				Type:        discordgo.ApplicationCommandOptionBoolean,
				Name:        "disable_memory",
				Description: "Do not read or write memory for this ask",
				Required:    false,
			},
		},
	},
	Handler: func(h *CommandHandler, s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h.Chat == nil {
			respondEphemeral(s, i, "Chat is not configured.")
			return
		}

		var text string
		historyLength := 10
		disableMemory := false

		for _, opt := range i.ApplicationCommandData().Options {
			switch opt.Name {
			case "text":
				text = strings.TrimSpace(opt.StringValue())
			case "history_length":
				historyLength = int(opt.IntValue())
			case "disable_memory":
				disableMemory = opt.BoolValue()
			}
		}

		if text == "" {
			respondEphemeral(s, i, "Ask text cannot be empty.")
			return
		}
		if historyLength < 1 {
			historyLength = 10
		}
		if historyLength > 100 {
			historyLength = 100
		}

		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		}); err != nil {
			log.Error("cannot defer ask interaction", "err", err, "channel", i.ChannelID)
			return
		}

		user := interactionUser(i)
		displayName := "unknown"
		userID := ""
		if user != nil {
			userID = user.ID
			displayName = user.DisplayName()
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		log.Info("ask started",
			"guild", i.GuildID,
			"channel", i.ChannelID,
			"user", userID,
			"history_length", historyLength,
			"memory_disabled", disableMemory,
		)

		content, err := h.Chat.Ask(ctx, ChatRequest{
			GuildID:       i.GuildID,
			ChannelID:     i.ChannelID,
			UserID:        userID,
			DisplayName:   displayName,
			BotID:         botID(s),
			Text:          text,
			HistoryLength: historyLength,
			DisableMemory: disableMemory,
		})
		if err != nil {
			log.Error("ask failed", "err", err, "guild", i.GuildID, "channel", i.ChannelID, "user", userID)
			editInteractionError(s, i, "My brain is offline right now. Try again in a bit.")
			return
		}

		if err := sendInteractionContent(s, i, content); err != nil {
			log.Error("cannot send ask response", "err", err, "channel", i.ChannelID)
		}
	},
}

func interactionUser(i *discordgo.InteractionCreate) *discordgo.User {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User
	}
	return i.User
}

func botID(s *discordgo.Session) string {
	if s == nil || s.State == nil || s.State.User == nil {
		return ""
	}
	return s.State.User.ID
}

func sendInteractionContent(s *discordgo.Session, i *discordgo.InteractionCreate, content string) error {
	chunks := splitResponse(content, 2000, 3)
	if len(chunks) == 0 {
		chunks = []string{"..."}
	}

	first := chunks[0]
	allowedMentions := &discordgo.MessageAllowedMentions{
		Parse:       []discordgo.AllowedMentionType{discordgo.AllowedMentionTypeUsers},
		RepliedUser: false,
	}
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:         &first,
		AllowedMentions: allowedMentions,
	}); err != nil {
		return err
	}

	for _, chunk := range chunks[1:] {
		if _, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content:         chunk,
			AllowedMentions: allowedMentions,
		}); err != nil {
			return err
		}
	}
	return nil
}

func editInteractionError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:         &message,
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
	}); err != nil {
		log.Error("cannot edit interaction response", "err", err, "channel", i.ChannelID)
	}
}

func splitResponse(content string, limit, maxChunks int) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if limit <= 0 {
		limit = 2000
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
		remaining = []rune(truncateText(string(remaining), limit))
	}
	if chunk := strings.TrimSpace(string(remaining)); chunk != "" {
		chunks = append(chunks, chunk)
	}

	return chunks
}

func truncateText(s string, max int) string {
	runes := []rune(s)
	if max <= 0 || len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func float64Ptr(value float64) *float64 {
	return &value
}

func init() {
	registerCommand(AskCmd)
}
