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
			result := logger.shouldLog(tt.messageLevel)
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

func BenchmarkLoggerShouldLog(b *testing.B) {
	logger := New(INFO)
	for i := 0; i < b.N; i++ {
		logger.shouldLog(INFO)
	}
}

func BenchmarkLoggerFormat(b *testing.B) {
	logger := New(INFO)
	for i := 0; i < b.N; i++ {
		logger.format(INFO, "test message")
	}
}
