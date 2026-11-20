package commands

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
)

var memoryAdminPermissions = int64(discordgo.PermissionAdministrator)

// MemoryCmd lets admins inspect and clear bot memory.
var MemoryCmd = &Command{
	ApplicationCommand: discordgo.ApplicationCommand{
		Name:                     "memory",
		Description:              "Manage bot memory (admin only)",
		DefaultMemberPermissions: &memoryAdminPermissions,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "view",
				Description: "View memory for a user or the server",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "User to view. Omit to view server memory",
						Required:    false,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "forget",
				Description: "Delete memory for a user or the server",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "User to forget. Omit to clear server memory",
						Required:    false,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list",
				Description: "List users with stored memory",
			},
		},
	},
	Handler: func(h *CommandHandler, s *discordgo.Session, i *discordgo.InteractionCreate) {
		if !isAdmin(i) {
			respondEphemeral(s, i, "Only admins can use this command.")
			return
		}
		if h.Memory == nil {
			respondEphemeral(s, i, "Memory is not enabled.")
			return
		}

		data := i.ApplicationCommandData()
		if len(data.Options) == 0 {
			respondEphemeral(s, i, "Choose a memory action.")
			return
		}

		action := data.Options[0]
		switch action.Name {
		case "view":
			viewMemory(h, s, i, action)
		case "forget":
			forgetMemory(h, s, i, action)
		case "list":
			listMemory(h, s, i)
		default:
			respondEphemeral(s, i, "Unknown memory action.")
		}
	},
}

func viewMemory(h *CommandHandler, s *discordgo.Session, i *discordgo.InteractionCreate, action *discordgo.ApplicationCommandInteractionDataOption) {
	subjectID, label := memorySubject(action)
	text, err := h.Memory.Get(subjectID)
	if err != nil {
		log.Error("cannot read memory", "err", err, "subject", subjectID)
		respondEphemeral(s, i, "Error reading memory.")
		return
	}
	if strings.TrimSpace(text) == "" {
		respondEphemeral(s, i, fmt.Sprintf("No memory found for %s.", label))
		return
	}

	text = truncateText(text, 1800)
	respondEphemeral(s, i, "```markdown\n"+text+"\n```")
}

func forgetMemory(h *CommandHandler, s *discordgo.Session, i *discordgo.InteractionCreate, action *discordgo.ApplicationCommandInteractionDataOption) {
	subjectID, label := memorySubject(action)
	if err := h.Memory.Forget(subjectID); err != nil {
		log.Error("cannot delete memory", "err", err, "subject", subjectID)
		respondEphemeral(s, i, "Error deleting memory.")
		return
	}
	log.Info("memory forgotten", "subject", subjectID, "admin", interactionUserID(i))
	respondEphemeral(s, i, fmt.Sprintf("Memory forgotten for %s.", label))
}

func listMemory(h *CommandHandler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	users, err := h.Memory.ListUsers()
	if err != nil {
		log.Error("cannot list memory", "err", err)
		respondEphemeral(s, i, "Error listing memory.")
		return
	}
	if len(users) == 0 {
		respondEphemeral(s, i, "No user memory found.")
		return
	}

	const maxUsers = 50
	lines := users
	if len(lines) > maxUsers {
		lines = lines[:maxUsers]
	}

	content := strings.Join(lines, "\n")
	if len(users) > maxUsers {
		content += fmt.Sprintf("\n...and %d more", len(users)-maxUsers)
	}
	respondEphemeral(s, i, "Users with memory:\n```\n"+content+"\n```")
}

func memorySubject(action *discordgo.ApplicationCommandInteractionDataOption) (string, string) {
	for _, opt := range action.Options {
		if opt.Name != "user" {
			continue
		}
		user := opt.UserValue(nil)
		if user != nil {
			return user.ID, fmt.Sprintf("<@%s>", user.ID)
		}
	}
	return "server", "the server"
}

func isAdmin(i *discordgo.InteractionCreate) bool {
	return i.Member != nil && i.Member.Permissions&discordgo.PermissionAdministrator != 0
}

func init() {
	registerCommand(MemoryCmd)
}
