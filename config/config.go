package config

import (
	"fmt"
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
	Port     int
	Headless bool
}

func New() (*Config, error) {
	viper.SetDefault("Services", []string{})
	viper.SetDefault("Port", 8080)

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

	var headless bool
	if desktop := os.Getenv("XDG_SESSION_DESKTOP"); desktop == "" {
		log.Println("running in headless mode")
		headless = true
	}

	port := viper.GetInt("Port")
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port: %d", port)
	}

	cfg := Config{
		Services: viper.GetStringSlice("services"),
		Port:     port,
		Headless: headless,
	}

	return &cfg, nil
}
