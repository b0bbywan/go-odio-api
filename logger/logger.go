package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
)

type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
	FATAL
)

var levelNames = map[Level]string{
	DEBUG: "DEBUG",
	INFO:  "INFO ",
	WARN:  "WARN ",
	ERROR: "ERROR",
	FATAL: "FATAL",
}

type Logger struct {
	level         Level
	packageLevels map[string]Level
	logger        *log.Logger
}

// Global logger instance
var defaultLogger *Logger

func init() {
	defaultLogger = New(INFO)
}

// New creates a new logger with the specified level
func New(level Level) *Logger {
	return &Logger{
		level:         level,
		packageLevels: map[string]Level{},
		logger:        log.New(os.Stderr, "", log.LstdFlags),
	}
}

// SetLevel sets the global logger level
func SetLevel(level Level) {
	defaultLogger.level = level
}

// SetPackageLevels sets per-package level overrides.
// Keys match the [component] prefix used in log messages (e.g. "mpris", "api", "ui").
func SetPackageLevels(levels map[string]Level) {
	defaultLogger.packageLevels = levels
}

// extractComponent returns the component name from a "[component] ..." message, or "".
func extractComponent(msg string) string {
	if len(msg) < 3 || msg[0] != '[' {
		return ""
	}
	end := strings.IndexByte(msg[1:], ']')
	if end < 0 {
		return ""
	}
	return msg[1 : end+1]
}

// shouldLog checks if a message at this level should be logged,
// applying a package-specific override when the message carries a [component] prefix.
func (l *Logger) shouldLog(level Level, msg string) bool {
	if pkg := extractComponent(msg); pkg != "" {
		if pkgLevel, ok := l.packageLevels[pkg]; ok {
			return level >= pkgLevel
		}
	}
	return level >= l.level
}

// format creates a formatted message with level prefix
func (l *Logger) format(level Level, msg string) string {
	return fmt.Sprintf("[%s] %s", levelNames[level], msg)
}

// Debug logs a debug message
func Debug(msg string, args ...interface{}) {
	if defaultLogger.shouldLog(DEBUG, msg) {
		formatted := fmt.Sprintf(msg, args...)
		defaultLogger.logger.Println(defaultLogger.format(DEBUG, formatted))
	}
}

// Info logs an info message
func Info(msg string, args ...interface{}) {
	if defaultLogger.shouldLog(INFO, msg) {
		formatted := fmt.Sprintf(msg, args...)
		defaultLogger.logger.Println(defaultLogger.format(INFO, formatted))
	}
}

// Warn logs a warning message
func Warn(msg string, args ...interface{}) {
	if defaultLogger.shouldLog(WARN, msg) {
		formatted := fmt.Sprintf(msg, args...)
		defaultLogger.logger.Println(defaultLogger.format(WARN, formatted))
	}
}

// Error logs an error message
func Error(msg string, args ...interface{}) {
	if defaultLogger.shouldLog(ERROR, msg) {
		formatted := fmt.Sprintf(msg, args...)
		defaultLogger.logger.Println(defaultLogger.format(ERROR, formatted))
	}
}

// Fatal logs a fatal message and exits
func Fatal(msg string, args ...interface{}) {
	formatted := fmt.Sprintf(msg, args...)
	defaultLogger.logger.Fatalln(defaultLogger.format(FATAL, formatted))
}
