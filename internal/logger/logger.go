// Package logger provides application logging setup and helpers.
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"charm.land/log/v2"
)

var (
	logger  *log.Logger = log.New(io.Discard)
	logFile *os.File
)

// Init opens the log file in the system temp dir and configures the logger.
func Init() error {
	tmpDir := os.TempDir()
	logPath := filepath.Join(tmpDir, "neoviolet.log")
	// #nosec G304 -- logPath is a fixed file in the system temp dir.
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", logPath, err)
	}

	logger = log.New(f)
	logger.SetReportTimestamp(true)
	logger.SetTimeFormat(time.TimeOnly)
	logger.SetLevel(log.InfoLevel)
	logFile = f

	logger.Info("Logger initialized", "logPath", logPath)
	return nil
}

// Close closes the log file if one was opened.
func Close() error {
	if logFile != nil {
		return logFile.Close()
	}
	return nil
}

// Debug logs a message at debug level.
func Debug(msg string, keyvals ...any) {
	logger.Debug(msg, keyvals...)
}

// Info logs a message at info level.
func Info(msg string, keyvals ...any) {
	logger.Info(msg, keyvals...)
}

// Warn logs a message at warn level.
func Warn(msg string, keyvals ...any) {
	logger.Warn(msg, keyvals...)
}

// Error logs a message at error level.
func Error(msg string, keyvals ...any) {
	logger.Error(msg, keyvals...)
}

// Fatal logs a message at fatal level.
func Fatal(msg string, keyvals ...any) {
	logger.Fatal(msg, keyvals...)
}
