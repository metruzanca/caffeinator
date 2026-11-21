package config

import (
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
)

// ConfigureLogging sends logs to stdout, which Railway treats as informational
// rather than error output. On Railway we also switch to JSON so Railway can
// read the level field and color errors, warnings, and info correctly.
func ConfigureLogging() {
	log.SetOutput(os.Stdout)

	format := strings.ToLower(strings.TrimSpace(viper.GetString("LOG_FORMAT")))
	if format == "" && onRailway() {
		format = "json"
	}

	switch format {
	case "json":
		log.SetFormatter(log.JSONFormatter)
	default:
		log.SetFormatter(log.TextFormatter)
	}
}

func onRailway() bool {
	return os.Getenv("RAILWAY_ENVIRONMENT") != "" ||
		os.Getenv("RAILWAY_ENVIRONMENT_ID") != "" ||
		os.Getenv("RAILWAY_PROJECT_ID") != ""
}
