package commands

import (
	"context"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
)

// MemesCmd lets admins inspect and refresh meme memory.
var MemesCmd = &Command{
	ApplicationCommand: discordgo.ApplicationCommand{
		Name:        "memes",
		Description: "View or refresh {bot} meme memory",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "status",
				Description: "Show the meme schedule, storage, and last result (admin only)",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "refresh",
				Description: "Scrape the configured subreddits now (admin only)",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list",
				Description: "List stored memes (admin only)",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionInteger,
						Name:        "count",
						Description: "How many memes to show (1-25, default 10)",
						Required:    false,
						MinValue:    float64Ptr(1),
						MaxValue:    25,
					},
				},
			},
		},
	},
	Handler: func(h *CommandHandler, s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h.Memes == nil {
			respondEphemeral(s, i, "Meme memory is not enabled.")
			return
		}
		if !isAdmin(i) {
			respondEphemeral(s, i, "Only admins can manage meme memory.")
			return
		}

		data := i.ApplicationCommandData()
		if len(data.Options) == 0 {
			respondEphemeral(s, i, "Choose a meme action.")
			return
		}

		action := data.Options[0]
		switch action.Name {
		case "status":
			respondEphemeral(s, i, h.Memes.StatusText())
		case "list":
			count := 10
			for _, option := range action.Options {
				if option.Name == "count" {
					count = int(option.IntValue())
				}
			}
			if count < 1 {
				count = 10
			}
			if count > 25 {
				count = 25
			}
			respondEphemeral(s, i, truncateText(h.Memes.ListText(count), 1800))
		case "refresh":
			refreshMemes(h, s, i)
		default:
			respondEphemeral(s, i, "Unknown meme action.")
		}
	},
}

func refreshMemes(h *CommandHandler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	}); err != nil {
		log.Error("cannot defer meme refresh", "err", err, "channel", i.ChannelID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	text, err := h.Memes.Refresh(ctx)
	if err != nil {
		log.Error("meme refresh failed", "err", err, "channel", i.ChannelID)
		editInteractionError(s, i, "Meme refresh failed: "+err.Error())
		return
	}
	if strings.TrimSpace(text) == "" {
		text = "Meme refresh finished."
	}
	if err := sendInteractionContent(h, s, i, ChatReply{Text: text}); err != nil {
		log.Error("cannot send meme refresh result", "err", err, "channel", i.ChannelID)
	}
}

func init() {
	registerCommand(MemesCmd)
}
