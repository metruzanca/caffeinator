package commands

import (
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
)

// TimezoneCmd sets or shows the user's IANA timezone.
var TimezoneCmd = &Command{
	ApplicationCommand: discordgo.ApplicationCommand{
		Name:        "timezone",
		Description: "Set or show your timezone for reminders",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "zone",
				Description: "IANA timezone, for example America/New_York. Omit to show the current value",
				Required:    false,
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

		userID := interactionUserID(i)
		if zone == "" {
			current := h.Reminders.Timezone(userID)
			if current == "" {
				respondEphemeral(s, i, "No timezone set. Use /timezone zone:America/New_York.")
				return
			}
			respondEphemeral(s, i, "Your timezone is "+current+".")
			return
		}

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
