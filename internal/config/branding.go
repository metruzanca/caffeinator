package config

import (
	"strings"

	"github.com/spf13/viper"
)

// Defaults for the branding settings. Each one can be overridden with an
// environment variable, which lets people host their own copy of the bot
// without editing code.
const (
	DefaultBotName       = "Caffeinator"
	DefaultCommunityName = "The Coffee Break"
	DefaultAppURL        = "https://github.com/metruzanca/caffeinator"
)

// BotName is the name the bot uses when it refers to itself. The Discord
// username and avatar still live in the Discord Developer Portal.
func BotName() string {
	return valueOrDefault("BOT_NAME", DefaultBotName)
}

// CommunityName is the server or community the bot belongs to.
func CommunityName() string {
	return valueOrDefault("COMMUNITY_NAME", DefaultCommunityName)
}

// AppURL is the referer sent to OpenRouter for app attribution.
func AppURL() string {
	return valueOrDefault("OPENROUTER_APP_URL", DefaultAppURL)
}

func valueOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(viper.GetString(key)); value != "" {
		return value
	}
	return fallback
}
