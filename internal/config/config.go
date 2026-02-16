package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// DefaultScopes contains the Google Chat API OAuth2 scopes used for standard
// user authentication.
var DefaultScopes = []string{
	"https://www.googleapis.com/auth/chat.spaces",
	"https://www.googleapis.com/auth/chat.spaces.readonly",
	"https://www.googleapis.com/auth/chat.spaces.create",
	"https://www.googleapis.com/auth/chat.messages",
	"https://www.googleapis.com/auth/chat.messages.readonly",
	"https://www.googleapis.com/auth/chat.messages.create",
	"https://www.googleapis.com/auth/chat.messages.reactions",
	"https://www.googleapis.com/auth/chat.messages.reactions.readonly",
	"https://www.googleapis.com/auth/chat.messages.reactions.create",
	"https://www.googleapis.com/auth/chat.memberships",
	"https://www.googleapis.com/auth/chat.memberships.readonly",
	"https://www.googleapis.com/auth/chat.customemojis",
	"https://www.googleapis.com/auth/chat.customemojis.readonly",
	"https://www.googleapis.com/auth/chat.users.readstate",
	"https://www.googleapis.com/auth/chat.users.readstate.readonly",
	"https://www.googleapis.com/auth/chat.users.spacesettings",
}

// Config holds the application configuration.
type Config struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	TokenFile    string `mapstructure:"token_file"`
}

// ConfigDir returns the path to the gogchat configuration directory
// (~/.config/gogchat/) and creates it if it does not exist.
func ConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	dir := filepath.Join(home, ".config", "gogchat")
	_ = os.MkdirAll(dir, 0o700)
	return dir
}

// Load reads the configuration from the config file, environment variables,
// and returns a populated Config struct.
func Load() (*Config, error) {
	dir := ConfigDir()
	defaultTokenFile := filepath.Join(dir, "token.json")

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(dir)

	viper.SetEnvPrefix("GOGCHAT")
	viper.AutomaticEnv()

	viper.SetDefault("client_id", "")
	viper.SetDefault("client_secret", "")
	viper.SetDefault("token_file", defaultTokenFile)

	// Read the config file; ignore "not found" errors since env vars or
	// defaults may be sufficient.
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	return &cfg, nil
}
