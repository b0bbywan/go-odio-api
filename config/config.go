package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"github.com/b0bbywan/go-odio-api/logger"
)

const (
	AppName = "odio-api"
)

type Config struct {
	Services *SystemdConfig
	Port     int
	LogLevel logger.Level
}

type SystemdConfig struct {
	SystemServices []string
	UserServices   []string
	Headless       bool
	XDGRuntimeDir  string
}

// parseLogLevel converts a string to a logger.Level
func parseLogLevel(levelStr string) logger.Level {
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		return logger.DEBUG
	case "INFO":
		return logger.INFO
	case "WARN":
		return logger.WARN
	case "ERROR":
		return logger.ERROR
	case "FATAL":
		return logger.FATAL
	default:
		return logger.WARN // default
	}
}

func New() (*Config, error) {
	viper.SetDefault("services.system", []string{})
	viper.SetDefault("services.user", []string{})
	viper.SetDefault("Port", 8080)
	viper.SetDefault("LogLevel", "WARN")

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
			logger.Warn("failed to read config: %v", err)
		}
	}

	var headless bool
	if desktop := os.Getenv("XDG_SESSION_DESKTOP"); desktop == "" {
		logger.Info("running in headless mode")
		headless = true
	}

	port := viper.GetInt("Port")
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port: %d", port)
	}

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}

	syscfg := SystemdConfig{
		SystemServices: viper.GetStringSlice("services.system"),
		UserServices:   viper.GetStringSlice("services.user"),
		Headless:       headless,
		XDGRuntimeDir:  xdgRuntimeDir,
	}

	cfg := Config{
		Services: &syscfg,
		Port:     port,
		LogLevel: parseLogLevel(viper.GetString("LogLevel")),
	}

	return &cfg, nil
}
