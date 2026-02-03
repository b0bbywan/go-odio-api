package config

import (
	"testing"

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
		Services: &SystemdConfig{},
		Port:     8080,
		Headless: false,
		LogLevel: logger.INFO,
	}

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.LogLevel != logger.INFO {
		t.Errorf("LogLevel = %d, want %d", cfg.LogLevel, logger.INFO)
	}
	if cfg.Headless != false {
		t.Errorf("Headless = %v, want false", cfg.Headless)
	}
	if cfg.Services == nil {
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
