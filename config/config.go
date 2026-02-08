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
	AppVersion = "0.3.3"
)

type Config struct {
	Api        *ApiConfig
	Bluetooth  *BluetoothConfig
	MPRIS      *MPRISConfig
	Pulseaudio *PulseAudioConfig
	Systemd    *SystemdConfig
	LogLevel   logger.Level
}

type UIConfig struct {
	Enabled bool
}

type ApiConfig struct {
	Enabled bool
	Port    int

	UI *UIConfig
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

type BluetoothConfig struct {
	Enabled        bool
	PairingTimeout time.Duration
	Timeout        time.Duration
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
	viper.SetDefault("api.port", 8080)

	viper.SetDefault("api.ui.enabled", true)

	viper.SetDefault("systemd.enabled", true)
	viper.SetDefault("services.system", []string{})
	viper.SetDefault("services.user", []string{})

	viper.SetDefault("pulseaudio.enabled", true)

	viper.SetDefault("mpris.enabled", true)
	viper.SetDefault("mpris.timeout", "5s")

	viper.SetDefault("bluetooth.enabled", true)
	viper.SetDefault("bluetooth.timeout", "5s")
	viper.SetDefault("bluetooth.pairingtimeout", "60s")

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

	uiCfg := UIConfig{
		Enabled: viper.GetBool("ui.enabled"),
	}

	apiCfg := ApiConfig{
		Enabled: viper.GetBool("api.enabled"),
		Port:    port,
		UI:      &uiCfg,
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

	bluetoothTimeout := viper.GetDuration("bluetooth.timeout")
	if bluetoothTimeout <= 0 {
		bluetoothTimeout = 5 * time.Second
	}

	bluetoothPairingTimeout := viper.GetDuration("bluetooth.pairingtimeout")
	if bluetoothPairingTimeout <= 0 {
		bluetoothPairingTimeout = 60 * time.Second
	}

	bluetoothcfg := BluetoothConfig{
		Enabled:        viper.GetBool("bluetooth.enabled"),
		Timeout:        bluetoothTimeout,
		PairingTimeout: bluetoothPairingTimeout,
	}

	cfg := Config{
		Api:        &apiCfg,
		Bluetooth:  &bluetoothcfg,
		MPRIS:      &mpriscfg,
		Pulseaudio: &pulsecfg,
		Systemd:    &syscfg,
		LogLevel:   parseLogLevel(viper.GetString("LogLevel")),
	}

	return &cfg, nil
}
