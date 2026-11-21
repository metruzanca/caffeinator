package commands

import (
	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
)

// RemindersCmd lists and manages reminders.
var RemindersCmd = &Command{
	ApplicationCommand: discordgo.ApplicationCommand{
		Name:        "reminders",
		Description: "List or manage your reminders and events",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list",
				Description: "List your reminders and public events",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "cancel",
				Description: "Cancel a reminder you created",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "id",
						Description: "Reminder ID from /reminders list",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "leave",
				Description: "Leave a reminder or event",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "id",
						Description: "Reminder ID from /reminders list",
						Required:    true,
					},
				},
			},
		},
	},
	Handler: func(h *CommandHandler, s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h.Reminders == nil {
			respondEphemeral(s, i, "Reminders are not enabled.")
			return
		}

		data := i.ApplicationCommandData()
		if len(data.Options) == 0 {
			respondEphemeral(s, i, "Choose a reminders action.")
			return
		}

		action := data.Options[0]
		userID := interactionUserID(i)

		switch action.Name {
		case "list":
			respondEphemeral(s, i, h.Reminders.ListForUser(userID))
		case "cancel":
			id := optionString(action, "id")
			ok, err := h.Reminders.Cancel(userID, id)
			if err != nil {
				log.Error("cannot cancel reminder", "err", err, "user", userID, "id", id)
				respondEphemeral(s, i, "Error canceling reminder.")
				return
			}
			if !ok {
				respondEphemeral(s, i, "No reminder found with that ID, or you are not the creator.")
				return
			}
			respondEphemeral(s, i, "Reminder canceled.")
		case "leave":
			id := optionString(action, "id")
			ok, err := h.Reminders.Leave(userID, id)
			if err != nil {
				log.Warn("cannot leave reminder", "err", err, "user", userID, "id", id)
				respondEphemeral(s, i, "You cannot leave your own reminder. Cancel it instead.")
				return
			}
			if !ok {
				respondEphemeral(s, i, "You are not in that reminder.")
				return
			}
			respondEphemeral(s, i, "You left the reminder.")
		default:
			respondEphemeral(s, i, "Unknown reminders action.")
		}
	},
}

func init() {
	registerCommand(RemindersCmd)
}
