package ui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

const historyFile = "history.txt"

// testMode disables disk I/O in history load/save functions during tests
// that don't specifically test persistence. Set to true for normal tests.
var testMode bool

// historyPath returns the full path to the history file.
func historyPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, historyFile), nil
}

// loadHistory reads command history from disk into the model.
// If the file doesn't exist, it returns silently.
func loadHistory(m *Model) {
	if testMode {
		return
	}
	path, err := historyPath()
	if err != nil {
		logger.Warn("Failed to resolve history path", "err", err)
		return
	}

	// #nosec G304 -- path is the resolved history file for this app.
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		logger.Warn("Failed to read history file", "path", path, "err", err)
		return
	}

	lines := strings.Split(string(data), "\n")

	// Filter out empty lines
	history := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			history = append(history, line)
		}
	}

	// Respect max — trim oldest entries from the front
	maxHistory := m.Config.CommandHistory.Max
	if maxHistory > 0 && len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}

	m.CommandHistory = history
	m.historyIndex = len(m.CommandHistory)

	logger.Debug("Command history loaded", "count", len(m.CommandHistory), "path", path)
}

// saveHistory writes command history to disk.
// Logs a warning on failure but never crashes the app.
func saveHistory(m *Model) {
	if testMode {
		return
	}
	path, err := historyPath()
	if err != nil {
		logger.Warn("Failed to resolve history path for save", "err", err)
		return
	}

	// Write history — newest at bottom
	data := strings.Join(m.CommandHistory, "\n") + "\n"

	// Ensure the directory exists
	dir := filepath.Dir(path)
	// #nosec G301 -- user config dir is intentionally world-readable (0755).
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Warn("Failed to create history directory", "dir", dir, "err", err)
		return
	}

	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		logger.Warn("Failed to write history file", "path", path, "err", err)
		return
	}
}
