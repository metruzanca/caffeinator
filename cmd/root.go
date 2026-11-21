package cmd

import (
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/charmbracelet/log"
	"github.com/metruzanca/caffeinator/internal/config"
	"github.com/metruzanca/caffeinator/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:               "caffeinator",
	Short:             "Caffeinator, a self-hostable Discord bot",
	PersistentPreRunE: config.PersistentPreRunE,
	Run: func(cmd *cobra.Command, args []string) {
		token := viper.GetString("TOKEN")
		if token == "" {
			log.Fatal("TOKEN is required")
		}

		if strings.TrimSpace(viper.GetString("OPENROUTER_API_KEY")) == "" {
			log.Fatal("OPENROUTER_API_KEY is required")
		}

		log.Info("starting bot")

		bot := server.NewBot(token)

		if err := bot.Start(); err != nil {
			log.Fatal("cannot start bot", "err", err)
		}

		// Docker sends SIGTERM when stopping a container.
		sc := make(chan os.Signal, 1)
		signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
		<-sc
		log.Info("shutting down")
		bot.Stop()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	viper.SetDefault("BOT_NAME", config.DefaultBotName)
	viper.SetDefault("COMMUNITY_NAME", config.DefaultCommunityName)
	viper.SetDefault("LOG_FORMAT", "")
	viper.SetDefault("STARTUP_MESSAGE", true)
	viper.SetDefault("OPENROUTER_MODEL", "deepseek/deepseek-v4.1-flash-1017")
	viper.SetDefault("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1")
	viper.SetDefault("OPENROUTER_APP_URL", config.DefaultAppURL)
	viper.SetDefault("OPENROUTER_WEB_SEARCH", true)
	viper.SetDefault("OPENROUTER_WEB_SEARCH_MAX_RESULTS", 3)
	viper.SetDefault("OPENROUTER_WEB_SEARCH_MAX_USES", 2)
	viper.SetDefault("CHAT_ENABLED", true)
	viper.SetDefault("CHAT_SYSTEM_PROMPT", "")
	viper.SetDefault("CHAT_CONTEXT_MESSAGES", 10)
	viper.SetDefault("CHAT_MAX_TOKENS", 2048)
	viper.SetDefault("CHAT_MAX_CONTEXT_CHARS", 20000)
	viper.SetDefault("CHAT_MAX_CHARS_PER_MESSAGE", 4000)
	viper.SetDefault("CHAT_MAX_CHUNKS", 3)
	viper.SetDefault("CHAT_TIMEOUT_SECONDS", 60)
	viper.SetDefault("CHAT_MAX_CONCURRENT", 4)
	viper.SetDefault("CHAT_RATE_LIMIT_CAPACITY", 1)
	viper.SetDefault("CHAT_RATE_LIMIT_SECONDS", 5)
	viper.SetDefault("MEMORY_ENABLED", true)
	viper.SetDefault("MEMORY_DIR", "./memory")
	viper.SetDefault("MEMORY_MAX_CONTEXT_CHARS", 3000)
	viper.SetDefault("MEMORY_MIN_CONFIDENCE", 0.7)
	viper.SetDefault("MEMORY_EXTRACT_TIMEOUT_SECONDS", 30)
	viper.SetDefault("MEMORY_PASSIVE_ENABLED", true)
	viper.SetDefault("MEMORY_BATCH_SECONDS", 60)
	viper.SetDefault("MEMORY_MAX_BATCH_MESSAGES", 20)
	viper.SetDefault("MEMORY_IGNORE_CHANNELS", "")
	viper.SetDefault("IMAGE_ENABLED", true)
	viper.SetDefault("IMAGE_MODEL", "black-forest-labs/flux-3-image-klein")
	viper.SetDefault("IMAGE_DEFAULT_RESOLUTION", "1K")
	viper.SetDefault("IMAGE_DEFAULT_ASPECT_RATIO", "1:1")
	viper.SetDefault("IMAGE_SAFETY_TOLERANCE", 2)
	viper.SetDefault("IMAGE_MAX_REFERENCE_MB", 8)
	viper.SetDefault("IMAGE_TIMEOUT_SECONDS", 120)
	viper.SetDefault("IMAGE_RATE_LIMIT_PER_HOUR", 20)
	viper.SetDefault("REMINDERS_ENABLED", true)
	viper.SetDefault("REMINDERS_PATH", "./reminders.json")
	viper.SetDefault("REMINDER_TICK_SECONDS", 15)
	viper.SetDefault("REMINDER_GRACE_MINUTES", 60)
	viper.SetDefault("REMINDER_MAX_PER_USER", 50)
	viper.SetDefault("REMINDER_RETRY_MINUTES", 2)
	viper.SetDefault("REMINDER_MAX_ATTEMPTS", 5)
	viper.SetDefault("VISION_ENABLED", true)
	viper.SetDefault("VISION_MAX_IMAGES", 4)
	viper.SetDefault("VISION_MAX_IMAGE_MB", 8)
	viper.SetDefault("VISION_DETAIL", "auto")

	rootCmd.PersistentFlags().String("BOT_NAME", config.DefaultBotName, "Name the bot uses when it refers to itself")
	rootCmd.PersistentFlags().String("COMMUNITY_NAME", config.DefaultCommunityName, "Server or community the bot belongs to")
	rootCmd.PersistentFlags().String("TOKEN", "", "Discord bot token (required)")
	rootCmd.PersistentFlags().String("LOG_FORMAT", "", "Log format: text or json (auto json on Railway)")
	rootCmd.PersistentFlags().String("CLIENT_ID", "", "Discord application client ID")
	rootCmd.PersistentFlags().String("CHANNEL_ID", "", "Discord channel ID for startup notifications")
	rootCmd.PersistentFlags().Bool("STARTUP_MESSAGE", true, "Send startup notifications")
	rootCmd.PersistentFlags().String("OPENROUTER_MODEL", "deepseek/deepseek-v4.1-flash-1017", "OpenRouter model slug")
	rootCmd.PersistentFlags().String("OPENROUTER_APP_URL", config.DefaultAppURL, "OpenRouter app URL used for attribution")
	rootCmd.PersistentFlags().Bool("OPENROUTER_WEB_SEARCH", true, "Enable OpenRouter web search for mention chat")
	rootCmd.PersistentFlags().Int("OPENROUTER_WEB_SEARCH_MAX_RESULTS", 3, "Maximum results per web search")
	rootCmd.PersistentFlags().Int("OPENROUTER_WEB_SEARCH_MAX_USES", 2, "Maximum web searches per chat request")
	rootCmd.PersistentFlags().Bool("CHAT_ENABLED", true, "Enable mention chat")
	rootCmd.PersistentFlags().String("CHAT_SYSTEM_PROMPT", "", "Override the default system prompt")
	rootCmd.PersistentFlags().Int("CHAT_CONTEXT_MESSAGES", 10, "Number of channel messages to include as context")
	rootCmd.PersistentFlags().Int("CHAT_MAX_TOKENS", 2048, "Maximum tokens for a model response")
	rootCmd.PersistentFlags().Int("CHAT_MAX_CONTEXT_CHARS", 20000, "Maximum characters of context to send")
	rootCmd.PersistentFlags().Int("CHAT_MAX_CHARS_PER_MESSAGE", 4000, "Maximum characters per context message")
	rootCmd.PersistentFlags().Int("CHAT_MAX_CHUNKS", 3, "Maximum Discord messages per response")
	rootCmd.PersistentFlags().Int("CHAT_TIMEOUT_SECONDS", 60, "OpenRouter request timeout in seconds")
	rootCmd.PersistentFlags().Int("CHAT_MAX_CONCURRENT", 4, "Maximum concurrent chat requests")
	rootCmd.PersistentFlags().Int("CHAT_RATE_LIMIT_CAPACITY", 1, "Chat requests allowed per user per rate limit window")
	rootCmd.PersistentFlags().Int("CHAT_RATE_LIMIT_SECONDS", 5, "Chat rate limit window in seconds")
	rootCmd.PersistentFlags().Bool("MEMORY_ENABLED", true, "Enable markdown memory")
	rootCmd.PersistentFlags().String("MEMORY_DIR", "./memory", "Directory for markdown memory files")
	rootCmd.PersistentFlags().Int("MEMORY_MAX_CONTEXT_CHARS", 3000, "Maximum memory characters injected into a chat request")
	rootCmd.PersistentFlags().Float64("MEMORY_MIN_CONFIDENCE", 0.7, "Minimum confidence for a stored memory fact")
	rootCmd.PersistentFlags().Int("MEMORY_EXTRACT_TIMEOUT_SECONDS", 30, "Memory extraction timeout in seconds")
	rootCmd.PersistentFlags().Bool("MEMORY_PASSIVE_ENABLED", true, "Learn from normal messages automatically")
	rootCmd.PersistentFlags().Int("MEMORY_BATCH_SECONDS", 60, "Batch window for passive memory")
	rootCmd.PersistentFlags().Int("MEMORY_MAX_BATCH_MESSAGES", 20, "Maximum messages per user batch")
	rootCmd.PersistentFlags().String("MEMORY_IGNORE_CHANNELS", "", "Comma-separated channel IDs to ignore for passive memory")
	rootCmd.PersistentFlags().Bool("IMAGE_ENABLED", true, "Enable image generation")
	rootCmd.PersistentFlags().String("IMAGE_MODEL", "black-forest-labs/flux-3-image-klein", "OpenRouter image model slug")
	rootCmd.PersistentFlags().String("IMAGE_DEFAULT_RESOLUTION", "1K", "Default image resolution")
	rootCmd.PersistentFlags().String("IMAGE_DEFAULT_ASPECT_RATIO", "1:1", "Default image aspect ratio")
	rootCmd.PersistentFlags().Int("IMAGE_SAFETY_TOLERANCE", 2, "Default moderation tolerance")
	rootCmd.PersistentFlags().Int("IMAGE_MAX_REFERENCE_MB", 8, "Maximum reference image size in MB")
	rootCmd.PersistentFlags().Int("IMAGE_TIMEOUT_SECONDS", 120, "Image generation timeout in seconds")
	rootCmd.PersistentFlags().Int("IMAGE_RATE_LIMIT_PER_HOUR", 20, "Image generations per user per hour")
	rootCmd.PersistentFlags().Bool("REMINDERS_ENABLED", true, "Enable reminders and events")
	rootCmd.PersistentFlags().String("REMINDERS_PATH", "./reminders.json", "Path to the reminders JSON file")
	rootCmd.PersistentFlags().Int("REMINDER_TICK_SECONDS", 15, "Reminder scheduler tick interval in seconds")
	rootCmd.PersistentFlags().Int("REMINDER_GRACE_MINUTES", 60, "How late a missed reminder may still fire")
	rootCmd.PersistentFlags().Int("REMINDER_MAX_PER_USER", 50, "Maximum active reminders per user")
	rootCmd.PersistentFlags().Int("REMINDER_RETRY_MINUTES", 2, "Delay before retrying a failed reminder")
	rootCmd.PersistentFlags().Int("REMINDER_MAX_ATTEMPTS", 5, "Maximum send attempts before giving up")
	rootCmd.PersistentFlags().Bool("VISION_ENABLED", true, "Enable image understanding for attachments")
	rootCmd.PersistentFlags().Int("VISION_MAX_IMAGES", 4, "Maximum images per chat request")
	rootCmd.PersistentFlags().Int("VISION_MAX_IMAGE_MB", 8, "Maximum image size in MB")
	rootCmd.PersistentFlags().String("VISION_DETAIL", "auto", "Image detail: auto, low, high, or original")
}
