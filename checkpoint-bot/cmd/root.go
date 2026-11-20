package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/log"
	"github.com/metruzanca/checkpoint-bot/internal/config"
	"github.com/metruzanca/checkpoint-bot/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:               "checkpoint",
	Short:             "Caffeinator, a Discord bot for The Coffee Break",
	PersistentPreRunE: config.PersistentPreRunE,
	Run: func(cmd *cobra.Command, args []string) {
		token := viper.GetString("TOKEN")
		if token == "" {
			log.Fatal("TOKEN is required")
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
	viper.SetDefault("STARTUP_MESSAGE", true)
	viper.SetDefault("OPENROUTER_MODEL", "deepseek/deepseek-v4.1-flash-1017")
	viper.SetDefault("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1")
	viper.SetDefault("CHAT_ENABLED", true)
	viper.SetDefault("CHAT_CONTEXT_MESSAGES", 10)
	viper.SetDefault("CHAT_MAX_TOKENS", 1024)
	viper.SetDefault("CHAT_MAX_CONTEXT_CHARS", 20000)
	viper.SetDefault("CHAT_MAX_CHARS_PER_MESSAGE", 4000)
	viper.SetDefault("CHAT_MAX_CHUNKS", 3)
	viper.SetDefault("CHAT_TIMEOUT_SECONDS", 60)
	viper.SetDefault("CHAT_MAX_CONCURRENT", 4)
	viper.SetDefault("CHAT_RATE_LIMIT_CAPACITY", 1)
	viper.SetDefault("CHAT_RATE_LIMIT_SECONDS", 5)

	rootCmd.PersistentFlags().String("TOKEN", "", "Discord bot token (required)")
	rootCmd.PersistentFlags().String("CLIENT_ID", "", "Discord application client ID")
	rootCmd.PersistentFlags().String("CHANNEL_ID", "", "Discord channel ID for startup notifications")
	rootCmd.PersistentFlags().Bool("STARTUP_MESSAGE", true, "Send startup notifications")
	rootCmd.PersistentFlags().String("OPENROUTER_MODEL", "deepseek/deepseek-v4.1-flash-1017", "OpenRouter model slug")
	rootCmd.PersistentFlags().Bool("CHAT_ENABLED", true, "Enable mention chat")
	rootCmd.PersistentFlags().Int("CHAT_CONTEXT_MESSAGES", 10, "Number of channel messages to include as context")
	rootCmd.PersistentFlags().Int("CHAT_MAX_TOKENS", 1024, "Maximum tokens for a model response")
	rootCmd.PersistentFlags().Int("CHAT_MAX_CONTEXT_CHARS", 20000, "Maximum characters of context to send")
	rootCmd.PersistentFlags().Int("CHAT_MAX_CHARS_PER_MESSAGE", 4000, "Maximum characters per context message")
	rootCmd.PersistentFlags().Int("CHAT_MAX_CHUNKS", 3, "Maximum Discord messages per response")
	rootCmd.PersistentFlags().Int("CHAT_TIMEOUT_SECONDS", 60, "OpenRouter request timeout in seconds")
	rootCmd.PersistentFlags().Int("CHAT_MAX_CONCURRENT", 4, "Maximum concurrent chat requests")
	rootCmd.PersistentFlags().Int("CHAT_RATE_LIMIT_CAPACITY", 1, "Chat requests allowed per user per rate limit window")
	rootCmd.PersistentFlags().Int("CHAT_RATE_LIMIT_SECONDS", 5, "Chat rate limit window in seconds")
}
