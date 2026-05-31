package ui

import (
	"fmt"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"
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
	case AccentApplyMsg:
		return handleAccentApply(m, msg)
	case MediaCtlMsg:
		return handleMediaCtlCmd(m, msg)
	default:
		return m, nil
	}
}

func handleTick(m *Model) (tea.Model, tea.Cmd) {
	cmd := m.updatePlaybackState()
	m.Error.Tick()
	m.Info.Tick()

	// Push current playback state to OS media control layer (MPRIS on Linux)
	if m.MediaCtl != nil {
		m.MediaCtl.Update(m.buildPlayState())
	}

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

	// Apply --seek flag: seek to initial position after playing starts
	if m.pendingSeek > 0 {
		seekPos := m.pendingSeek
		if seekPos > m.Audio.Duration {
			seekPos = m.Audio.Duration
		}
		logger.Info("Initial seek", "to", seekPos)
		m.Audio.SeekPlayer(seekPos)
		m.pendingSeek = 0
	}

	if m.Config.Lyrics.Enabled {
		data, err := lyrics.FindAndParse(msg.Path, m.Config.Lyrics.FormatPriority)
		if err != nil {
			m.Error.Set(fmt.Sprintf("Failed to parse lyrics: %v", err), 180)
		} else if data != nil {
			m.Audio.Lyrics = data
			m.Audio.LyricIndex = -1
				m.Audio.ShowLyrics = true
		}
	}

	if m.Config.Accent.IsEnabled() {
		return m, loadAccentCmd(msg.Player)
	}

	// Push initial track metadata to OS media control layer
	if m.MediaCtl != nil {
		m.MediaCtl.Update(m.buildPlayState())
	}

	return m, nil
}

func handleAccentApply(m *Model, msg AccentApplyMsg) (tea.Model, tea.Cmd) {
	m.Accent = msg.Accent
	m.rebuildProgressBar()
	return m, nil
}

func handleMediaCtlCmd(m *Model, msg MediaCtlMsg) (tea.Model, tea.Cmd) {
	if m.Audio.Player == nil {
		return m, nil
	}

	switch msg.Command.Type {
	case mediactl.CmdPlayPause:
		m.togglePlayback()
	case mediactl.CmdPlay:
		if !m.Audio.Player.IsPlaying() {
			m.Audio.Player.Play()
			m.Audio.IsPlaying = true
		}
	case mediactl.CmdPause:
		if m.Audio.Player.IsPlaying() {
			m.Audio.Player.Pause()
			m.Audio.IsPlaying = false
		}
	case mediactl.CmdStop:
		m.Audio.Player.Pause()
		m.Audio.IsPlaying = false
		m.Audio.Player.Seek(0)
	case mediactl.CmdNext:
		// No tracklist — skip forward 10s as fallback
		m.Audio.SeekRelative(10 * time.Second)
	case mediactl.CmdPrev:
		m.Audio.SeekRelative(-10 * time.Second)
	case mediactl.CmdSeek:
		// MPRIS Seek offset is in microseconds
		offset := time.Duration(msg.Command.Value) * time.Microsecond
		m.Audio.SeekRelative(offset)
	case mediactl.CmdSetPosition:
		// MPRIS SetPosition is absolute position in microseconds
		pos := time.Duration(msg.Command.Value) * time.Microsecond
		m.Audio.SeekPlayer(pos)
	}

	return m, nil
}
