package commands

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
)

// ErrUnsupportedLink is returned when the URL host is not one we rewrite.
var ErrUnsupportedLink = errors.New("unsupported link host")

// LinkCmd rewrites social media links so Discord can embed them.
var LinkCmd = &Command{
	ApplicationCommand: discordgo.ApplicationCommand{
		Name:        "link",
		Description: "Rewrite an X/Twitter or Instagram link for better embeds",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "url",
				Description: "The link to rewrite",
				Required:    true,
			},
		},
	},
	Handler: func(_ *CommandHandler, s *discordgo.Session, i *discordgo.InteractionCreate) {
		var rawURL string
		for _, opt := range i.ApplicationCommandData().Options {
			if opt.Name == "url" {
				rawURL = opt.StringValue()
			}
		}

		fixedURL, err := RewriteLink(rawURL)
		if err != nil {
			message := "That doesn't look like a valid link."
			if errors.Is(err, ErrUnsupportedLink) {
				message = "I can only rewrite X/Twitter and Instagram links."
			}

			log.Warn("cannot rewrite link", "err", err, "user", interactionUserID(i), "channel", i.ChannelID, "guild", i.GuildID)
			respondEphemeral(s, i, message)
			return
		}

		log.Info("link rewritten", "user", interactionUserID(i), "channel", i.ChannelID, "guild", i.GuildID)
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fixedURL,
			},
		}); err != nil {
			log.Error("cannot respond with rewritten link", "err", err, "channel", i.ChannelID)
		}
	},
}

// RewriteLink converts known social media hosts to their embed-friendly
// equivalents and removes all query parameters and fragments.
func RewriteLink(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("link is empty")
	}

	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid link: %w", err)
	}
	if parsed.Host == "" {
		return "", errors.New("invalid link")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("invalid link scheme")
	}

	host := strings.ToLower(parsed.Hostname())
	host = strings.TrimPrefix(host, "www.")

	switch host {
	case "x.com", "twitter.com", "mobile.x.com", "mobile.twitter.com":
		parsed.Host = "fxtwitter.com"
	case "instagram.com", "m.instagram.com":
		parsed.Host = "kkinstagram.com"
	case "fxtwitter.com", "kkinstagram.com":
		parsed.Host = host
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedLink, host)
	}

	parsed.Scheme = "https"
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.RawFragment = ""

	if parsed.Path != "/" {
		parsed.Path = strings.TrimRight(parsed.Path, "/")
		parsed.RawPath = ""
	}

	return parsed.String(), nil
}

func interactionUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
			AllowedMentions: &discordgo.MessageAllowedMentions{
				Parse:       []discordgo.AllowedMentionType{},
				RepliedUser: false,
			},
		},
	}); err != nil {
		log.Error("cannot respond to interaction", "err", err, "channel", i.ChannelID)
	}
}

func init() {
	registerCommand(LinkCmd)
}
