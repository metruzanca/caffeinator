package config

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestLoadConfig tests that defaults are registered and readable.
func TestLoadConfig(t *testing.T) {
	viper.Reset()

	viper.SetDefault("testkey", "default_value")
	viper.SetDefault("anotherkey", 10)

	LoadConfig()

	assert.Equal(t, "default_value", viper.GetString("testkey"), "testkey should be 'default_value'")
	assert.Equal(t, 10, viper.GetInt("anotherkey"), "anotherkey should be 10")
}

// TestLoadConfig_EnvironmentVariables tests that environment variables
// correctly override defaults.
func TestLoadConfig_EnvironmentVariables(t *testing.T) {
	viper.Reset()

	viper.SetDefault("envtestkey", "default_env")

	os.Setenv("ENVTESTKEY", "env_value")
	defer os.Unsetenv("ENVTESTKEY")

	LoadConfig()

	assert.Equal(t, "env_value", viper.GetString("envtestkey"), "envtestkey should be 'env_value' from environment")
}

// TestLoadConfig_Defaults tests that the packaged defaults are applied.
func TestLoadConfig_Defaults(t *testing.T) {
	viper.Reset()
	os.Unsetenv("TOKEN")

	LoadConfig()

	assert.Equal(t, DefaultBotName, viper.GetString("BOT_NAME"))
	assert.Equal(t, true, viper.GetBool("CHAT_ENABLED"))
	assert.Equal(t, "auto", viper.GetString("VISION_DETAIL"))
	assert.Equal(t, true, viper.GetBool("MEMES_ENABLED"))
	assert.Equal(t, "week", viper.GetString("MEMES_TOP_PERIOD"))
	assert.Equal(t, "0 15 * * 1,3,5", viper.GetString("MEMES_CRON"))
}
