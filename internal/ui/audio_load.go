package ui

import (
	"fmt"
	"io"
	"os"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// audioLoad owns a prepared player until the UI accepts it. Cancellation also
// releases a result already queued in Bubble Tea, where it may never be handled.
type audioLoad struct {
	mu       sync.Mutex
	canceled bool
	player   audio.AudioPlayer
}

func (l *audioLoad) cancel() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.canceled = true
	player := l.player
	l.player = nil
	l.mu.Unlock()
	if player != nil {
		_ = player.Close()
	}
}

func (l *audioLoad) take() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.canceled {
		return false
	}
	l.player = nil
	return true
}

func (l *audioLoad) deliver(msg tea.Msg) tea.Msg {
	l.mu.Lock()
	if l.canceled {
		l.mu.Unlock()
		if loaded, ok := msg.(AudioLoadedMsg); ok {
			_ = loaded.Player.Close()
		}
		return nil
	}
	if loaded, ok := msg.(AudioLoadedMsg); ok {
		l.player = loaded.Player
		loaded.load = l
		msg = loaded
	}
	l.mu.Unlock()
	return msg
}

func (m *Model) startAudioLoad(path string) tea.Cmd {
	m.pendingLoad.cancel()
	load := &audioLoad{}
	m.pendingLoad = load
	generation := m.loadGeneration
	sfPath, backend := m.Config.SoundfontPath, m.Config.TrackerBackend
	return func() tea.Msg {
		load.mu.Lock()
		canceled := load.canceled
		load.mu.Unlock()
		if canceled {
			return nil
		}
		fmt.Fprint(os.Stdout, "\033]9;4;3;0\a")
		return load.deliver(loadAudio(path, sfPath, backend, generation))
	}
}

func discardAudioLoaded(msg AudioLoadedMsg) {
	if msg.load != nil {
		msg.load.cancel()
		return
	}
	if msg.Player != nil {
		_ = msg.Player.Close()
	}
}

func loadAudio(filePath, sfPath, trackerBackend string, generation int) (result tea.Msg) {
	logger.Info("Loading audio file", "path", filePath)
	player := audio.NewPlayer()
	defer func() {
		if _, loaded := result.(AudioLoadedMsg); !loaded {
			_ = player.Close()
		}
	}()
	if sfPath != "" {
		player.SetSoundfontPath(sfPath)
	}
	player.SetTrackerBackend(trackerBackend)

	// Stdin mode: load audio data from pipe
	if filePath == "-" {
		return loadAudioFromStdin(player, generation)
	}

	if err := player.Open(filePath); err != nil {
		logger.Error("Failed to load audio", "path", filePath, "err", err)
		return AudioLoadFailedMsg{Message: fmt.Sprintf("Failed to load audio: %v", err), Timer: 180, Generation: generation}
	}
	return AudioLoadedMsg{Player: player, Path: filePath, Generation: generation}
}

// loadAudioFromStdin reads audio data from os.Stdin and opens it via the player.
// os.Stdin is only read when it's a pipe (not a terminal); BubbleTea uses
// /dev/tty for terminal input so there is no conflict. Stdin is closed after
// reading to prevent accidental reuse.
func loadAudioFromStdin(player *audio.Player, generation int) tea.Msg {
	// Check if stdin is a pipe (not a terminal)
	info, err := os.Stdin.Stat()
	if err != nil {
		logger.Error("Failed to stat stdin", "err", err)
		return AudioLoadFailedMsg{Message: "Failed to read stdin", Timer: 120, Generation: generation}
	}
	if info.Mode()&os.ModeNamedPipe == 0 && info.Mode()&os.ModeCharDevice != 0 {
		return AudioLoadFailedMsg{
			Message:    "stdin is a terminal; pipe audio data or provide a file path",
			Timer:      180,
			Generation: generation,
		}
	}

	// Limit stdin read to 500 MB to prevent memory exhaustion from
	// accidentally piped large files or /dev/zero.
	const maxStdinSize = 500 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(os.Stdin, maxStdinSize+1))
	_ = os.Stdin.Close()
	if err != nil {
		logger.Error("Failed to read stdin", "err", err)
		return AudioLoadFailedMsg{Message: fmt.Sprintf("Failed to read stdin: %v", err), Timer: 120, Generation: generation}
	}
	if len(data) > maxStdinSize {
		return AudioLoadFailedMsg{
			Message:    "Stdin input exceeds 500 MB limit",
			Timer:      180,
			Generation: generation,
		}
	}

	if err := player.OpenReader("stdin", data); err != nil {
		logger.Error("Failed to load audio from stdin", "err", err)
		return AudioLoadFailedMsg{Message: fmt.Sprintf("Failed to load from stdin: %v", err), Timer: 180, Generation: generation}
	}

	return AudioLoadedMsg{Player: player, Path: "stdin", Generation: generation}
}
