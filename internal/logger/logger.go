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

func Init() error {
	tmpDir := os.TempDir()
	logPath := filepath.Join(tmpDir, "neoviolet.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
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

func Close() error {
	if logFile != nil {
		return logFile.Close()
	}
	return nil
}

func Debug(msg string, keyvals ...any) {
	logger.Debug(msg, keyvals...)
}

func Info(msg string, keyvals ...any) {
	logger.Info(msg, keyvals...)
}

func Warn(msg string, keyvals ...any) {
	logger.Warn(msg, keyvals...)
}

func Error(msg string, keyvals ...any) {
	logger.Error(msg, keyvals...)
}

func Fatal(msg string, keyvals ...any) {
	logger.Fatal(msg, keyvals...)
}

func Printf(format string, args ...any) {
	logger.Printf(format, args...)
}

func With(keyvals ...any) *log.Logger {
	return logger.With(keyvals...)
}
