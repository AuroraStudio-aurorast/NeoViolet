package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAndClose(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	logPath := filepath.Join(os.TempDir(), "neoviolet.log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("log file was not created")
	}

	if err := Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestInfoWritesToFile(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	defer Close()

	Info("test message", "key", "value")

	logPath := filepath.Join(os.TempDir(), "neoviolet.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "test message") {
		t.Errorf("log file does not contain 'test message': %s", content)
	}
	if !strings.Contains(content, "key") || !strings.Contains(content, "value") {
		t.Errorf("log file does not contain key=value: %s", content)
	}
}

func TestDebugNilLogger(t *testing.T) {
	// Ensure no panic when logger is nil
	Debug("debug msg", "k", "v")
	Info("info msg", "k", "v")
	Warn("warn msg", "k", "v")
	Error("error msg", "k", "v")
	Printf("printf %s", "test")
	_ = With("key", "value")
}

func TestWithReturnsLogger(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	defer Close()

	l := With("module", "test")
	if l == nil {
		t.Error("With() returned nil")
	}
}

func TestInitCleansUpOldLog(t *testing.T) {
	// Init should append to existing log file, not overwrite it
	if err := Init(); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	Info("first message")
	Close()

	if err := Init(); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	Info("second message")
	Close()

	logPath := filepath.Join(os.TempDir(), "neoviolet.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "first message") {
		t.Error("log should contain first message (append mode)")
	}
	if !strings.Contains(content, "second message") {
		t.Error("log should contain second message")
	}
}
