package logger

import (
	"strings"
	"testing"
)

func TestLoggerLevelFiltering(t *testing.T) {
	tests := []struct {
		name         string
		level        Level
		messageLevel Level
		shouldLog    bool
	}{
		{"DEBUG logs at DEBUG level", DEBUG, DEBUG, true},
		{"INFO logs at DEBUG level", DEBUG, INFO, true},
		{"DEBUG doesn't log at INFO level", INFO, DEBUG, false},
		{"ERROR logs at INFO level", INFO, ERROR, true},
		{"WARN logs at ERROR level", ERROR, WARN, false},
		{"ERROR logs at ERROR level", ERROR, ERROR, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := New(tt.level)
			result := logger.shouldLog(tt.messageLevel, "")
			if result != tt.shouldLog {
				t.Errorf("shouldLog(%v) = %v, want %v", tt.messageLevel, result, tt.shouldLog)
			}
		})
	}
}

func TestLoggerFormat(t *testing.T) {
	logger := New(INFO)
	formatted := logger.format(INFO, "test message")

	if !strings.Contains(formatted, "[INFO ]") {
		t.Errorf("formatted message should contain '[INFO ]', got: %s", formatted)
	}
	if !strings.Contains(formatted, "test message") {
		t.Errorf("formatted message should contain 'test message', got: %s", formatted)
	}
}

func TestLevelNames(t *testing.T) {
	tests := map[Level]string{
		DEBUG: "DEBUG",
		INFO:  "INFO ",
		WARN:  "WARN ",
		ERROR: "ERROR",
		FATAL: "FATAL",
	}

	for level, expected := range tests {
		if levelNames[level] != expected {
			t.Errorf("levelNames[%d] = %s, want %s", level, levelNames[level], expected)
		}
	}
}

func TestSetLevel(t *testing.T) {
	// Save original level
	originalLevel := defaultLogger.level

	defer func() {
		defaultLogger.level = originalLevel
	}()

	SetLevel(DEBUG)
	if defaultLogger.level != DEBUG {
		t.Errorf("SetLevel(DEBUG) failed, level = %d, want %d", defaultLogger.level, DEBUG)
	}

	SetLevel(ERROR)
	if defaultLogger.level != ERROR {
		t.Errorf("SetLevel(ERROR) failed, level = %d, want %d", defaultLogger.level, ERROR)
	}
}

func TestGlobalLoggerInstance(t *testing.T) {
	// The global logger should be initialized
	if defaultLogger == nil {
		t.Fatal("defaultLogger should be initialized")
	}

	// Should have INFO level by default
	if defaultLogger.level != INFO {
		t.Errorf("defaultLogger.level = %d, want %d (INFO)", defaultLogger.level, INFO)
	}
}

func TestDebugFunction(t *testing.T) {
	originalLevel := defaultLogger.level
	defer func() { defaultLogger.level = originalLevel }()

	SetLevel(DEBUG)

	// This should NOT panic
	Debug("test %s", "message")
}

func TestInfoFunction(t *testing.T) {
	originalLevel := defaultLogger.level
	defer func() { defaultLogger.level = originalLevel }()

	SetLevel(INFO)

	// This should NOT panic
	Info("info %s", "message")
}

func TestWarnFunction(t *testing.T) {
	originalLevel := defaultLogger.level
	defer func() { defaultLogger.level = originalLevel }()

	SetLevel(WARN)

	// This should NOT panic
	Warn("warn %s", "message")
}

func TestErrorFunction(t *testing.T) {
	originalLevel := defaultLogger.level
	defer func() { defaultLogger.level = originalLevel }()

	SetLevel(ERROR)

	// This should NOT panic
	Error("error %s", "occurred")
}

func TestExtractComponent(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"[mpris] listener started", "mpris"},
		{"[pulseaudio] sink changed", "pulseaudio"},
		{"[api] server running", "api"},
		{"[ui] client error", "ui"},
		{"no prefix message", ""},
		{"", ""},
		{"[unclosed", ""},
	}
	for _, tt := range tests {
		got := extractComponent(tt.msg)
		if got != tt.want {
			t.Errorf("extractComponent(%q) = %q, want %q", tt.msg, got, tt.want)
		}
	}
}

func TestSetPackageLevels(t *testing.T) {
	original := defaultLogger.packageLevels
	defer func() { defaultLogger.packageLevels = original }()

	levels := map[string]Level{"mpris": DEBUG, "api": ERROR}
	SetPackageLevels(levels)

	if defaultLogger.packageLevels["mpris"] != DEBUG {
		t.Errorf("expected mpris=DEBUG, got %v", defaultLogger.packageLevels["mpris"])
	}
	if defaultLogger.packageLevels["api"] != ERROR {
		t.Errorf("expected api=ERROR, got %v", defaultLogger.packageLevels["api"])
	}
}

func TestPackageLevelFiltering(t *testing.T) {
	tests := []struct {
		name        string
		globalLevel Level
		pkgLevels   map[string]Level
		msgLevel    Level
		msg         string
		want        bool
	}{
		// No package override — global level rules
		{"no override: global INFO, INFO msg passes", INFO, nil, INFO, "[mpris] msg", true},
		{"no override: global INFO, DEBUG msg suppressed", INFO, nil, DEBUG, "[mpris] msg", false},
		{"no override: global DEBUG, DEBUG msg passes", DEBUG, nil, DEBUG, "[mpris] msg", true},

		// No [component] prefix — always falls back to global
		{"no prefix: global INFO, DEBUG suppressed", INFO, map[string]Level{"mpris": DEBUG}, DEBUG, "bare message", false},
		{"no prefix: global INFO, INFO passes", INFO, map[string]Level{"mpris": DEBUG}, INFO, "bare message", true},

		// Package level more verbose than global (DEBUG override, INFO global)
		{"pkg verbose: DEBUG override, DEBUG msg passes", INFO, map[string]Level{"mpris": DEBUG}, DEBUG, "[mpris] msg", true},
		{"pkg verbose: DEBUG override, INFO msg passes", INFO, map[string]Level{"mpris": DEBUG}, INFO, "[mpris] msg", true},

		// Package level more restrictive than global (ERROR override, INFO global)
		{"pkg restrictive: ERROR override, INFO msg suppressed", INFO, map[string]Level{"api": ERROR}, INFO, "[api] msg", false},
		{"pkg restrictive: ERROR override, WARN msg suppressed", INFO, map[string]Level{"api": ERROR}, WARN, "[api] msg", false},
		{"pkg restrictive: ERROR override, ERROR msg passes", INFO, map[string]Level{"api": ERROR}, ERROR, "[api] msg", true},

		// Unregistered package falls back to global
		{"unregistered pkg: global INFO, DEBUG suppressed", INFO, map[string]Level{"mpris": DEBUG}, DEBUG, "[pulseaudio] msg", false},
		{"unregistered pkg: global INFO, INFO passes", INFO, map[string]Level{"mpris": DEBUG}, INFO, "[pulseaudio] msg", true},

		// Multiple packages coexist independently
		{"multi-pkg: mpris DEBUG msg passes", INFO, map[string]Level{"mpris": DEBUG, "api": ERROR}, DEBUG, "[mpris] msg", true},
		{"multi-pkg: api WARN suppressed", INFO, map[string]Level{"mpris": DEBUG, "api": ERROR}, WARN, "[api] msg", false},
		{"multi-pkg: api ERROR passes", INFO, map[string]Level{"mpris": DEBUG, "api": ERROR}, ERROR, "[api] msg", true},
		{"multi-pkg: unregistered falls back to global INFO", INFO, map[string]Level{"mpris": DEBUG, "api": ERROR}, DEBUG, "[systemd] msg", false},

		// Exact boundary: level == threshold
		{"boundary: pkg WARN, WARN msg passes", INFO, map[string]Level{"mpris": WARN}, WARN, "[mpris] msg", true},
		{"boundary: pkg WARN, INFO msg suppressed", INFO, map[string]Level{"mpris": WARN}, INFO, "[mpris] msg", false},
		{"boundary: global WARN, WARN msg passes (no override)", WARN, nil, WARN, "[mpris] msg", true},
		{"boundary: global WARN, INFO msg suppressed (no override)", WARN, nil, INFO, "[mpris] msg", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.globalLevel)
			if tt.pkgLevels != nil {
				l.packageLevels = tt.pkgLevels
			}
			got := l.shouldLog(tt.msgLevel, tt.msg)
			if got != tt.want {
				t.Errorf("shouldLog(%v, %q) with global=%v pkgLevels=%v = %v, want %v",
					tt.msgLevel, tt.msg, tt.globalLevel, tt.pkgLevels, got, tt.want)
			}
		})
	}
}

func BenchmarkLoggerShouldLog(b *testing.B) {
	logger := New(INFO)
	for i := 0; i < b.N; i++ {
		logger.shouldLog(INFO, "")
	}
}

func BenchmarkLoggerFormat(b *testing.B) {
	logger := New(INFO)
	for i := 0; i < b.N; i++ {
		logger.format(INFO, "test message")
	}
}
