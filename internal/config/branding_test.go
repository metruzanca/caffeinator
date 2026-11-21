package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestBrandingDefaults(t *testing.T) {
	viper.Reset()

	assert.Equal(t, DefaultBotName, BotName())
	assert.Equal(t, DefaultCommunityName, CommunityName())
	assert.Equal(t, DefaultAppURL, AppURL())
}

func TestBrandingOverrides(t *testing.T) {
	viper.Reset()

	viper.Set("BOT_NAME", "  Helper  ")
	viper.Set("COMMUNITY_NAME", "Test Server")
	viper.Set("OPENROUTER_APP_URL", "https://example.com/fork")

	assert.Equal(t, "Helper", BotName())
	assert.Equal(t, "Test Server", CommunityName())
	assert.Equal(t, "https://example.com/fork", AppURL())
}
