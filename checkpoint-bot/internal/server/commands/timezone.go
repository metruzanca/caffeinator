package commands

import (
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
)

// TimezoneCmd sets the user's IANA timezone.
var TimezoneCmd = &Command{
	ApplicationCommand: discordgo.ApplicationCommand{
		Name:        "timezone",
		Description: "Set your timezone for reminders",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "zone",
				Description: "IANA timezone, for example America/New_York",
				Required:    true,
			},
		},
	},
	Handler: func(h *CommandHandler, s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h.Reminders == nil {
			respondEphemeral(s, i, "Reminders are not enabled.")
			return
		}

		var zone string
		for _, opt := range i.ApplicationCommandData().Options {
			if opt.Name == "zone" {
				zone = strings.TrimSpace(opt.StringValue())
			}
		}
		if zone == "" {
			respondEphemeral(s, i, "Timezone cannot be empty.")
			return
		}

		userID := interactionUserID(i)
		if err := h.Reminders.SetTimezone(userID, zone); err != nil {
			log.Warn("cannot set timezone", "err", err, "user", userID, "zone", zone)
			respondEphemeral(s, i, "Invalid timezone. Use an IANA name like America/New_York or Europe/London.")
			return
		}

		respondEphemeral(s, i, "Timezone set to "+zone+".")
	},
}

func init() {
	registerCommand(TimezoneCmd)
}
