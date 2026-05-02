package ui

import (
	"fmt"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

func updateDispatcher(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TickMsg:
		return handleTick(m)
	case tea.WindowSizeMsg:
		return handleResize(m, msg)
	case tea.KeyPressMsg:
		return handleKeyPress(m, msg)
	case progress.FrameMsg:
		return handleProgressFrame(m, msg)
	case VolumeMsg:
		return handleVolume(m, msg)
	case SeekMsg:
		return handleSeek(m, msg)
	case ErrorMsg:
		return handleError(m, msg)
	case AudioLoadedMsg:
		return handleAudioLoaded(m, msg)
	default:
		return m, nil
	}
}

func handleTick(m *Model) (tea.Model, tea.Cmd) {
	cmd := m.updatePlaybackState()
	m.Error.Tick()

	return m, tea.Batch(cmd, tea.Tick(time.Second/time.Duration(m.Config.TickRate), func(t time.Time) tea.Msg {
		return TickMsg{}
	}))
}

func handleResize(m *Model, msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.UI.Width = msg.Width
	m.UI.Height = msg.Height

	m.UI.tabWidth = (msg.Width - 4) / len(m.UI.Tabs)
	if m.UI.tabWidth > 20 {
		m.UI.tabWidth = 20
	}

	m.Components.CommandInput.SetWidth(msg.Width - 1)

	return m, nil
}

func handleProgressFrame(m *Model, msg progress.FrameMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.Components.ProgressBar, cmd = m.Components.ProgressBar.Update(msg)
	return m, cmd
}

func handleVolume(m *Model, msg VolumeMsg) (tea.Model, tea.Cmd) {
	if msg.Delta != 0 {
		m.Audio.AdjustVolume(msg.Delta)
	} else if msg.Level >= 0 && msg.Level <= 1.0 {
		m.Audio.SetVolume(msg.Level)
	}
	logger.Debug("Volume", "newVolume", m.Audio.Volume)
	m.Components.VolumeBar.SetPercent(m.Audio.Volume)
	return m, nil
}

func handleSeek(m *Model, msg SeekMsg) (tea.Model, tea.Cmd) {
	if m.Audio.Player == nil {
		return m, nil
	}

	currentPos := m.Audio.Player.Position()
	var newPos time.Duration
	if msg.Relative {
		newPos = currentPos + msg.Position
	} else {
		newPos = msg.Position
	}

	if newPos < 0 {
		newPos = 0
	}
	if newPos > m.Audio.Duration {
		newPos = m.Audio.Duration
	}

	logger.Debug("Seek", "from", currentPos, "to", newPos)
	if err := m.Audio.SeekPlayer(newPos); err != nil {
		m.Error.Set(fmt.Sprintf("Seek failed: %v", err), 120)
	}

	return m, nil
}

func handleError(m *Model, msg ErrorMsg) (tea.Model, tea.Cmd) {
	m.Loading = false
	m.Error.Set(msg.Message, msg.Timer)
	return m, nil
}

func handleAudioLoaded(m *Model, msg AudioLoadedMsg) (tea.Model, tea.Cmd) {
	m.Loading = false
	m.Audio.Player = msg.Player
	m.Audio.Duration = msg.Player.Duration()

	if msg.Player.Title() != "" {
		m.Audio.CurrentSong = msg.Player.Title()
		logger.Info("Audio loaded", "title", msg.Player.Title(), "artist", msg.Player.Artist())
	} else {
		m.Audio.CurrentSong = filepath.Base(msg.Path)
		logger.Info("Audio loaded (no tags)", "file", filepath.Base(msg.Path))
	}
	if msg.Player.Artist() != "" {
		m.Audio.Artist = msg.Player.Artist()
	} else {
		m.Audio.Artist = "Unknown Artist"
	}

	msg.Player.SetVolume(m.Audio.Volume)

	if err := msg.Player.Play(); err != nil {
		m.Error.Set(fmt.Sprintf("Failed to start playback: %v", err), 180)
	} else {
		m.Audio.IsPlaying = true
	}

	// Load LRC lyrics if available
	if m.Config.Lyrics.Enabled {
		lrcPath := lyrics.FindLRC(msg.Path)
		if lrcPath != "" {
			data, err := lyrics.ParseFile(lrcPath)
			if err != nil {
				m.Error.Set(fmt.Sprintf("Failed to parse lyrics: %v", err), 180)
			} else {
				m.Audio.Lyrics = data
				m.Audio.LyricIndex = -1
			}
		}
	}

	return m, nil
}
