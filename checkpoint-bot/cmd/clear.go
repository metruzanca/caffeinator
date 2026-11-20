package cmd

import (
	"github.com/charmbracelet/log"
	"github.com/metruzanca/checkpoint-bot/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// clearCmd removes every registered slash command from every guild.
var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all slash commands",
	Run: func(cmd *cobra.Command, args []string) {
		bot := server.NewBot(viper.GetString("TOKEN"))
		if err := bot.Start(); err != nil {
			log.Fatal("cannot start bot", "err", err)
		}
		defer bot.Stop()

		bot.UnregisterCommands()
	},
}

func init() {
	commandCmd.AddCommand(clearCmd)
}
