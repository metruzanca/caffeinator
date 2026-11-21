// Package config loads configuration from .env and environment variables.
package config

import (
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// LoadConfig registers defaults, reads .env, and enables environment lookups.
func LoadConfig() {
	SetDefaults()

	godotenv.Load() // Load .env file

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv() // Read environment variables
}
