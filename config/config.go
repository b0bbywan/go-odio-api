package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	AppName = "odio-api"
)

type Config struct {
	Services []string
}

func New() (*Config, error) {
	viper.SetDefault("Services", []string{})
	// Load from configuration file, environment variables, and CLI flags
	viper.SetConfigName("config")                       // name of config file (without extension)
	viper.SetConfigType("yaml")                         // config file format
	viper.AddConfigPath(filepath.Join("/etc", AppName)) // Global configuration path
	if home, err := os.UserHomeDir(); err == nil {
		viper.AddConfigPath(filepath.Join(home, ".config", AppName)) // User config path
	}

	if err := viper.ReadInConfig(); err != nil {
		// Config file is optional, continue with defaults if not found
		if _, isNotFound := err.(viper.ConfigFileNotFoundError); !isNotFound {
			log.Printf("warning: failed to read config: %v", err)
		}
	}

	cfg := Config{
		Services: viper.GetStringSlice("services"),
	}

	return &cfg, nil
}
