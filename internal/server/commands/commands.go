package commands

import (
	"context"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
	"github.com/metruzanca/caffeinator/agent"
	"github.com/metruzanca/caffeinator/internal/config"
	"github.com/metruzanca/caffeinator/internal/ratelimit"
)

// Global mutable map, only modified in init() functions.
var commands = make(map[string]*Command)

// ChatRequest describes one chat request from a slash command.
type ChatRequest struct {
	GuildID       string
	ChannelID     string
	UserID        string
	DisplayName   string
	BotID         string
	Text          string
	HistoryLength int
	DisableMemory bool
}

// ChatService is the chat pipeline used by slash commands.
type ChatService interface {
	Ask(ctx context.Context, req ChatRequest) (string, error)
}

// MemoryService is the memory store used by the memory command.
type MemoryService interface {
	Get(subjectID string) (string, error)
	RemoveFacts(subjectID, match string) (int, error)
	Forget(subjectID string) error
	ListUsers() ([]string, error)
}

// ImageService generates images through OpenRouter.
type ImageService interface {
	Generate(ctx context.Context, req agent.ImageRequest) (agent.ImageResult, error)
}

// ReminderService manages user timezones and reminder records.
type ReminderService interface {
	SetTimezone(userID, zone string) error
	Timezone(userID string) string
	ListForUser(userID string) string
	Cancel(userID, idPrefix string) (bool, error)
	Leave(userID, idPrefix string) (bool, error)
}

// CommandHandler manages Discord slash command registration and execution.
type CommandHandler struct {
	DiscordClient          *discordgo.Session
	Chat                   ChatService
	Memory                 MemoryService
	Images                 ImageService
	Reminders              ReminderService
	ImageLimiter           *ratelimit.Limiter
	ImageModel             string
	ImageResolution        string
	ImageAspectRatio       string
	ImageSafetyTolerance   int
	ImageMaxReferenceBytes int64
	ImageTimeout           time.Duration
}

// Command represents a Discord slash command with its handler function.
type Command struct {
	discordgo.ApplicationCommand
	Handler func(h *CommandHandler, s *discordgo.Session, i *discordgo.InteractionCreate)
}

// NewCommandHandler creates a command handler and removes commands that no
// longer exist in this build.
func NewCommandHandler(discordClient *discordgo.Session) *CommandHandler {
	clearUnregisteredCommands(discordClient)
	return &CommandHandler{
		DiscordClient: discordClient,
	}
}

// RegisterCommands registers all commands for every guild the bot is in and
// installs the interaction handler.
func (h *CommandHandler) RegisterCommands() {
	for _, guild := range h.DiscordClient.State.Guilds {
		h.RegisterCommandsForGuild(guild.ID)
	}

	h.DiscordClient.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}

		commandName := i.ApplicationCommandData().Name
		userID := ""
		if i.Member != nil {
			userID = i.Member.User.ID
		} else if i.User != nil {
			userID = i.User.ID
		}

		if !commandRateLimiter.Allow(userID) {
			log.Warn("rate limit exceeded", "command", commandName, "user", userID, "channel", i.ChannelID, "guild", i.GuildID)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "You're using commands too quickly. Please wait a moment and try again.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		cmd, ok := commands[commandName]
		if !ok {
			log.Warn("unknown command received", "command", commandName, "channel", i.ChannelID, "guild", i.GuildID)
			return
		}

		log.Info("command executed", "command", commandName, "channel", i.ChannelID, "guild", i.GuildID, "user", userID)
		cmd.Handler(h, s, i)
	})
}

// RegisterCommandsForGuild registers every command in a single guild.
func (h *CommandHandler) RegisterCommandsForGuild(guildID string) {
	for _, cmd := range commands {
		appCommand := cmd.ApplicationCommand
		appCommand.Description = resolveDescription(appCommand.Description)
		registeredCmd, err := h.DiscordClient.ApplicationCommandCreate(h.DiscordClient.State.User.ID, guildID, &appCommand)
		if err != nil {
			log.Error("cannot register new command", "command", cmd.Name, "guild", guildID, "err", err)
			continue
		}
		log.Debug("registered command", "command", registeredCmd.Name, "guild", guildID)
	}
}

// registerCommand adds a command to the global map. It is called from init()
// functions in command files.
func registerCommand(cmd *Command) {
	commands[cmd.ApplicationCommand.Name] = cmd
}

// resolveDescription fills the bot name into command descriptions at
// registration time. init() runs before configuration is loaded, so the
// placeholder cannot be resolved earlier.
func resolveDescription(description string) string {
	return strings.ReplaceAll(description, "{bot}", config.BotName())
}

// GetAvailableCommands returns all registered command names and descriptions.
func GetAvailableCommands() []struct {
	Name        string
	Description string
} {
	result := make([]struct {
		Name        string
		Description string
	}, 0, len(commands))
	for _, cmd := range commands {
		result = append(result, struct {
			Name        string
			Description string
		}{
			Name:        cmd.Name,
			Description: resolveDescription(cmd.Description),
		})
	}
	return result
}

// clearUnregisteredCommands removes commands from Discord that are no longer in
// the commands map.
func clearUnregisteredCommands(discordClient *discordgo.Session) {
	for _, guild := range discordClient.State.Guilds {
		registeredCommands, err := discordClient.ApplicationCommands(discordClient.State.User.ID, guild.ID)
		if err != nil {
			log.Error("failed to get registered commands", "guild", guild.ID, "err", err)
			continue
		}
		for _, cmd := range registeredCommands {
			if _, exists := commands[cmd.Name]; exists {
				continue
			}
			if err := discordClient.ApplicationCommandDelete(discordClient.State.User.ID, guild.ID, cmd.ID); err != nil {
				log.Error("failed to delete command", "command", cmd.Name, "guild", guild.ID, "err", err)
				continue
			}
			log.Debug("deleted unregistered command", "command", cmd.Name, "guild", guild.ID)
		}
	}
}

// Global rate limiter: 10 commands per minute per user.
var commandRateLimiter = ratelimit.New(10, time.Minute)
