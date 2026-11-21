package main

import (
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/charmbracelet/log"
	"github.com/metruzanca/caffeinator/internal/config"
	"github.com/metruzanca/caffeinator/internal/server"
	"github.com/spf13/viper"
)

func main() {
	config.LoadConfig()
	config.ConfigureLogging()

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
}
