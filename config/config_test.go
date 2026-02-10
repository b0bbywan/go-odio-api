package config

import (
	"testing"

	"github.com/spf13/viper"

	"github.com/b0bbywan/go-odio-api/logger"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected logger.Level
	}{
		{"debug", logger.DEBUG},
		{"DEBUG", logger.DEBUG},
		{"Debug", logger.DEBUG},
		{"info", logger.INFO},
		{"INFO", logger.INFO},
		{"warn", logger.WARN},
		{"WARN", logger.WARN},
		{"error", logger.ERROR},
		{"ERROR", logger.ERROR},
		{"fatal", logger.FATAL},
		{"FATAL", logger.FATAL},
		{"unknown", logger.WARN}, // default
		{"", logger.WARN},        // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseLogLevel(tt.input)
			if result != tt.expected {
				t.Errorf("parseLogLevel(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConfigStructFields(t *testing.T) {
	// Just verify the Config struct has the expected fields
	cfg := &Config{
		Systemd: &SystemdConfig{
			SupportsUTMP: false,
		},
		Api: &ApiConfig{
			Port: 8080,
		},
		LogLevel: logger.INFO,
	}

	if cfg.Api.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Api.Port)
	}
	if cfg.LogLevel != logger.INFO {
		t.Errorf("LogLevel = %d, want %d", cfg.LogLevel, logger.INFO)
	}
	if cfg.Systemd.SupportsUTMP != false {
		t.Errorf("Headless = %v, want false", cfg.Systemd.SupportsUTMP)
	}
	if cfg.Systemd == nil {
		t.Error("Services should not be nil")
	}
}

func TestSystemdConfigStructFields(t *testing.T) {
	sysCfg := &SystemdConfig{
		SystemServices: []string{"service1", "service2"},
		UserServices:   []string{"user-service1"},
	}

	if len(sysCfg.SystemServices) != 2 {
		t.Errorf("SystemServices length = %d, want 2", len(sysCfg.SystemServices))
	}
	if len(sysCfg.UserServices) != 1 {
		t.Errorf("UserServices length = %d, want 1", len(sysCfg.UserServices))
	}
}

func BenchmarkParseLogLevel(b *testing.B) {
	for i := 0; i < b.N; i++ {
		parseLogLevel("DEBUG")
	}
}

func TestNew_Defaults(t *testing.T) {
	// Reset viper to ensure clean state
	viper.Reset()

	// Isolate from user's config files by using a temp directory
	t.Setenv("HOME", t.TempDir())

	// Set XDG_SESSION_DESKTOP to avoid headless mode detection
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	// Test default port
	if cfg.Api.Port != 8018 {
		t.Errorf("Api.Port = %d, want 8018", cfg.Api.Port)
	}

	// Test default enabled flags
	if !cfg.Api.Enabled {
		t.Error("Api.Enabled should be true by default")
	}
	if cfg.Systemd.Enabled {
		t.Error("Systemd.Enabled should be false by default")
	}
	if !cfg.Pulseaudio.Enabled {
		t.Error("Pulseaudio.Enabled should be true by default")
	}
	if !cfg.MPRIS.Enabled {
		t.Error("MPRIS.Enabled should be true by default")
	}

	// Test default log level
	if cfg.LogLevel != logger.WARN {
		t.Errorf("LogLevel = %d, want %d (WARN)", cfg.LogLevel, logger.WARN)
	}
}

func TestNew_CustomPort(t *testing.T) {
	// Reset viper to ensure clean state
	viper.Reset()

	// Set custom port
	viper.Set("api.port", 9090)

	// Isolate from user's config files by using a temp directory
	t.Setenv("HOME", t.TempDir())

	// Set XDG_SESSION_DESKTOP to avoid headless mode detection
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	if cfg.Api.Port != 9090 {
		t.Errorf("Api.Port = %d, want 9090", cfg.Api.Port)
	}
}

func TestNew_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"zero port", 0},
		{"negative port", -1},
		{"port too high", 65536},
		{"port way too high", 100000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper to ensure clean state
			viper.Reset()

			// Set invalid port
			viper.Set("api.port", tt.port)

			// Isolate from user's config files by using a temp directory
			t.Setenv("HOME", t.TempDir())

			// Set XDG_SESSION_DESKTOP to avoid headless mode detection
			t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

			cfg, err := New(nil)
			if err == nil {
				t.Errorf("New(nil) with port %d should return error, got config: %+v", tt.port, cfg)
			}
			if cfg != nil {
				t.Errorf("New(nil) with invalid port should return nil config, got: %+v", cfg)
			}
		})
	}
}

func TestNew_CustomLogLevel(t *testing.T) {
	tests := []struct {
		level    string
		expected logger.Level
	}{
		{"DEBUG", logger.DEBUG},
		{"INFO", logger.INFO},
		{"WARN", logger.WARN},
		{"ERROR", logger.ERROR},
		{"FATAL", logger.FATAL},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			// Reset viper to ensure clean state
			viper.Reset()

			viper.Set("LogLevel", tt.level)

			// Isolate from user's config files by using a temp directory
			t.Setenv("HOME", t.TempDir())

			// Set XDG_SESSION_DESKTOP to avoid headless mode detection
			t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

			cfg, err := New(nil)
			if err != nil {
				t.Fatalf("New(nil) returned error: %v", err)
			}

			if cfg.LogLevel != tt.expected {
				t.Errorf("LogLevel = %d, want %d (%s)", cfg.LogLevel, tt.expected, tt.level)
			}
		})
	}
}
