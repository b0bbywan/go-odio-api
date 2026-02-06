package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/b0bbywan/go-odio-api/logger"
)

const (
	AppName    = "odio-api"
	AppVersion = "0.3.2"
)

type Config struct {
	Api        *ApiConfig
	Systemd    *SystemdConfig
	Pulseaudio *PulseAudioConfig
	MPRIS      *MPRISConfig
	LogLevel   logger.Level
}

type ApiConfig struct {
	Enabled bool
	Port    int
}

type SystemdConfig struct {
	Enabled        bool
	SystemServices []string
	UserServices   []string
	SupportsUTMP   bool
	XDGRuntimeDir  string
}

type PulseAudioConfig struct {
	Enabled       bool
	XDGRuntimeDir string
}

type MPRISConfig struct {
	Enabled bool
	Timeout time.Duration
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

func systemdHasUTMP() bool {
	_, err := os.Stat("/run/utmp")
	return err == nil
}

func New() (*Config, error) {
	viper.SetDefault("api.enabled", true)
	viper.SetDefault("systemd.enabled", true)
	viper.SetDefault("services.system", []string{})
	viper.SetDefault("services.user", []string{})

	viper.SetDefault("pulseaudio.enabled", true)

	viper.SetDefault("mpris.enabled", true)
	viper.SetDefault("mpris.timeout", "5s")

	viper.SetDefault("api.port", 8080)
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

	port := viper.GetInt("api.port")
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port: %d", port)
	}

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}

	apiCfg := ApiConfig{
		Enabled: viper.GetBool("api.enabled"),
		Port:    port,
	}

	syscfg := SystemdConfig{
		Enabled:        viper.GetBool("systemd.enabled"),
		SystemServices: viper.GetStringSlice("services.system"),
		UserServices:   viper.GetStringSlice("services.user"),
		SupportsUTMP:   systemdHasUTMP(),
		XDGRuntimeDir:  xdgRuntimeDir,
	}

	pulsecfg := PulseAudioConfig{
		Enabled:       viper.GetBool("pulseaudio.enabled"),
		XDGRuntimeDir: xdgRuntimeDir,
	}

	mprisTimeout := viper.GetDuration("mpris.timeout")
	if mprisTimeout <= 0 {
		mprisTimeout = 5 * time.Second
	}

	mpriscfg := MPRISConfig{
		Enabled: viper.GetBool("mpris.enabled"),
		Timeout: mprisTimeout,
	}

	cfg := Config{
		Api:        &apiCfg,
		Systemd:    &syscfg,
		Pulseaudio: &pulsecfg,
		MPRIS:      &mpriscfg,
		LogLevel:   parseLogLevel(viper.GetString("LogLevel")),
	}

	return &cfg, nil
}
