package config

import (
	"os"
	"log"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	AppName = "odio-api"
)

type Config struct {
	Services *SystemdConfig
	Port     int
	Headless bool
}

type SystemdConfig struct {
	SystemServices []string
	UserServices   []string
}

func New() (*Config, error) {
	viper.SetDefault("services.system", []string{})
	viper.SetDefault("services.user", []string{})
	viper.SetDefault("Port", 8080)

	// Load from configuration file, environment variables, and CLI flags
	viper.SetConfigName("config")                       // name of config file (without extension)
	viper.SetConfigType("yaml")                         // config file format
	viper.AddConfigPath(filepath.Join("/etc", AppName)) // Global configuration path
	if home, err := os.UserHomeDir(); err == nil {
		viper.AddConfigPath(filepath.Join(home, ".config", AppName)) // User config path
	}

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var headless bool
	if desktop := os.Getenv("XDG_SESSION_DESKTOP"); desktop == "" {
		log.Println("running in headless mode")
		headless = true
	}

	syscfg := SystemdConfig{
		SystemServices: viper.GetStringSlice("services.system"),
		UserServices:   viper.GetStringSlice("services.user"),
	}

	cfg := Config{
		Services: &syscfg,
		Port:     viper.GetInt("Port"),
		Headless: headless,
	}

	return &cfg, nil
}
