package commands

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
)

// MemoryCmd lets users inspect and manage memory.
var MemoryCmd = &Command{
	ApplicationCommand: discordgo.ApplicationCommand{
		Name:        "memory",
		Description: "View or forget bot memory",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "view",
				Description: "View memory for yourself, another user, or the server",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "User to view. Omit to view yourself. Admin only for other users",
						Required:    false,
					},
					{
						Type:        discordgo.ApplicationCommandOptionBoolean,
						Name:        "server",
						Description: "View server memory. Admin only",
						Required:    false,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "forget",
				Description: "Remove memory entries or clear memory",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "fact",
						Description: "Text to match in a memory entry",
						Required:    false,
					},
					{
						Type:        discordgo.ApplicationCommandOptionBoolean,
						Name:        "all",
						Description: "Delete all memory for the target",
						Required:    false,
					},
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "User to forget. Omit to target yourself. Admin only for other users",
						Required:    false,
					},
					{
						Type:        discordgo.ApplicationCommandOptionBoolean,
						Name:        "server",
						Description: "Target server memory. Admin only",
						Required:    false,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list",
				Description: "List users with stored memory (admin only)",
			},
		},
	},
	Handler: func(h *CommandHandler, s *discordgo.Session, i *discordgo.InteractionCreate) {
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
			if !isAdmin(i) {
				respondEphemeral(s, i, "Only admins can list all memory.")
				return
			}
			listMemory(h, s, i)
		default:
			respondEphemeral(s, i, "Unknown memory action.")
		}
	},
}

func viewMemory(h *CommandHandler, s *discordgo.Session, i *discordgo.InteractionCreate, action *discordgo.ApplicationCommandInteractionDataOption) {
	subjectID, label, ok := memoryTarget(s, i, action)
	if !ok {
		return
	}

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
	subjectID, label, ok := memoryTarget(s, i, action)
	if !ok {
		return
	}

	fact := strings.TrimSpace(optionString(action, "fact"))
	all := optionBool(action, "all")

	if fact != "" && all {
		respondEphemeral(s, i, "Choose either fact or all, not both.")
		return
	}
	if fact == "" && !all {
		respondEphemeral(s, i, "Provide fact:<text> to remove matching entries, or all:true to clear all memory.")
		return
	}

	if fact != "" {
		removed, err := h.Memory.RemoveFacts(subjectID, fact)
		if err != nil {
			log.Error("cannot remove memory facts", "err", err, "subject", subjectID)
			respondEphemeral(s, i, "Error removing memory.")
			return
		}
		if removed == 0 {
			respondEphemeral(s, i, fmt.Sprintf("No matching memory found for %s.", label))
			return
		}
		log.Info("memory facts removed", "subject", subjectID, "count", removed, "user", interactionUserID(i))
		respondEphemeral(s, i, fmt.Sprintf("Removed %d memory entries for %s.", removed, label))
		return
	}

	if err := h.Memory.Forget(subjectID); err != nil {
		log.Error("cannot delete memory", "err", err, "subject", subjectID)
		respondEphemeral(s, i, "Error deleting memory.")
		return
	}
	log.Info("memory forgotten", "subject", subjectID, "user", interactionUserID(i))
	respondEphemeral(s, i, fmt.Sprintf("All memory forgotten for %s.", label))
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

func memoryTarget(s *discordgo.Session, i *discordgo.InteractionCreate, action *discordgo.ApplicationCommandInteractionDataOption) (string, string, bool) {
	user := optionUser(action, "user")
	server := optionBool(action, "server")
	invokerID := interactionUserID(i)

	if user != nil && server {
		respondEphemeral(s, i, "Choose either a user or server memory, not both.")
		return "", "", false
	}
	if server {
		if !isAdmin(i) {
			respondEphemeral(s, i, "Only admins can manage server memory.")
			return "", "", false
		}
		return "server", "the server", true
	}
	if user != nil {
		if user.ID != invokerID && !isAdmin(i) {
			respondEphemeral(s, i, "Only admins can manage another user's memory.")
			return "", "", false
		}
		return user.ID, fmt.Sprintf("<@%s>", user.ID), true
	}
	if invokerID == "" {
		respondEphemeral(s, i, "Cannot determine your user ID.")
		return "", "", false
	}
	return invokerID, "you", true
}

func optionString(action *discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, opt := range action.Options {
		if opt.Name == name {
			return opt.StringValue()
		}
	}
	return ""
}

func optionBool(action *discordgo.ApplicationCommandInteractionDataOption, name string) bool {
	for _, opt := range action.Options {
		if opt.Name == name {
			return opt.BoolValue()
		}
	}
	return false
}

func optionUser(action *discordgo.ApplicationCommandInteractionDataOption, name string) *discordgo.User {
	for _, opt := range action.Options {
		if opt.Name == name {
			return opt.UserValue(nil)
		}
	}
	return nil
}

func isAdmin(i *discordgo.InteractionCreate) bool {
	return i.Member != nil && i.Member.Permissions&discordgo.PermissionAdministrator != 0
}

func init() {
	registerCommand(MemoryCmd)
}
