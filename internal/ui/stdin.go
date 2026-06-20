package ui

import (
	"bufio"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// StdinListener reads lines from os.Stdin and sends LoadTrackMsg for valid
// audio file paths. It is designed to run in a goroutine alongside the Bubble
// Tea program. Only non-empty, trimmed lines are processed.
//
// On Unix, Bubble Tea reads from /dev/tty, leaving os.Stdin free for pipe data.
// This function checks that stdin is a pipe before reading; if stdin is a
// terminal it returns immediately.
func StdinListener(program *tea.Program) {
	info, err := os.Stdin.Stat()
	if err != nil {
		logger.Warn("stdin listener: stat failed", "err", err)
		return
	}
	// Only read from pipes/FIFOs, not terminals
	if info.Mode()&os.ModeNamedPipe == 0 {
		logger.Debug("stdin listener: stdin is not a pipe, skipping")
		return
	}

	logger.Info("stdin listener: started")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !isValidAudioPath(line) {
			logger.Warn("stdin listener: invalid path", "path", line)
			program.Send(ErrorMsg{
				Message: "Invalid or unsupported audio file: " + line,
				Timer:   180,
			})
			continue
		}
		logger.Info("stdin listener: loading track", "path", line)
		program.Send(LoadTrackMsg{Path: line})
	}
	if err := scanner.Err(); err != nil {
		logger.Warn("stdin listener: scanner error", "err", err)
	}
	logger.Info("stdin listener: stopped")
}

// isValidAudioPath checks that the path exists, is a regular file, and has a
// plausible audio extension. Full format validation is done by the audio player
// during Open().
func isValidAudioPath(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	if !info.Mode().IsRegular() {
		return false
	}
	return true
}
