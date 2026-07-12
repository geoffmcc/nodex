package logging

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/geoffmcc/nodex/internal/redact"
)

// Level represents the logging level.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelSilent
)

// ParseLevel converts a string to a Level.
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	case "silent":
		return LevelSilent, nil
	default:
		return LevelInfo, fmt.Errorf("unknown log level: %s", s)
	}
}

// Logger provides level-based logging with optional redaction.
type Logger struct {
	level  Level
	w      io.Writer
	redact bool
}

// New creates a Logger that writes to w at the given level.
func New(w io.Writer, level Level, redactDebug bool) *Logger {
	return &Logger{level: level, w: w, redact: redactDebug}
}

// NewStderr creates a Logger writing to stderr.
func NewStderr(level Level, redactDebug bool) *Logger {
	return New(os.Stderr, level, redactDebug)
}

// Debug logs a debug message (only when level is Debug).
func (l *Logger) Debug(msg string, args ...any) {
	if l.level > LevelDebug {
		return
	}
	l.write("DEBUG", msg, args...)
}

// Info logs an info message.
func (l *Logger) Info(msg string, args ...any) {
	if l.level > LevelInfo {
		return
	}
	l.write("INFO", msg, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, args ...any) {
	if l.level > LevelWarn {
		return
	}
	l.write("WARN", msg, args...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, args ...any) {
	if l.level > LevelError {
		return
	}
	l.write("ERROR", msg, args...)
}

func (l *Logger) write(level, msg string, args ...any) {
	formatted := fmt.Sprintf(msg, args...)
	if l.redact {
		formatted = redact.String(formatted)
	}
	fmt.Fprintf(l.w, "[%s] %s\n", level, formatted)
}

// Level returns the current log level.
func (l *Logger) Level() Level {
	return l.level
}

// SetLevel changes the log level.
func (l *Logger) SetLevel(level Level) {
	l.level = level
}
