package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
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
	case LoadTrackMsg:
		return handleLoadTrack(m, msg)
	default:
		return m, nil
	}
}

func handleTick(m *Model) (tea.Model, tea.Cmd) {
	cmd := m.updatePlaybackState()
	m.Error.Tick()
	m.Info.Tick()

	if m.Loading {
		m.loadingTick++
	}

	// Drain IPC messages from GUI wrapper (non-blocking, one per tick)
	if m.ipcServer != nil {
		select {
		case msg := <-m.ipcServer.Incoming:
			if path, ok := parseIPCMessage(msg); ok {
				logger.Info("IPC: loading track", "path", path)
				_, loadCmd := handleLoadTrack(m, LoadTrackMsg{Path: path})
				// Batch the load command with the next tick so the
				// event loop keeps running — without the tick, the
				// progress bar and lyrics would freeze and new IPC
				// messages would never be processed.
				return m, tea.Batch(loadCmd,
					tea.Tick(time.Second/time.Duration(m.Config.TickRate),
						func(t time.Time) tea.Msg { return TickMsg{} },
					),
				)
			}
		default:
		}
	}

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
	// Ignore stale errors from a superseded track switch (Generation > 0 only)
	if msg.Generation > 0 && msg.Generation != m.loadGeneration {
		logger.Debug("Ignoring stale ErrorMsg", "msgGen", msg.Generation, "modelGen", m.loadGeneration)
		return m, nil
	}

	m.Loading = false
	m.switchingTrack = false
	// Clear loading line from normal screen
	fmt.Fprint(os.Stdout, "\033[2K\r")
	// Hide ConEmu progress bar
	fmt.Fprint(os.Stdout, "\033]9;4;0;0\a")
	m.Error.Set(msg.Message, msg.Timer)
	return m, nil
}

func handleAudioLoaded(m *Model, msg AudioLoadedMsg) (tea.Model, tea.Cmd) {
	// Ignore stale load results from a superseded track switch
	if msg.Generation != m.loadGeneration {
		logger.Debug("Ignoring stale AudioLoadedMsg", "msgGen", msg.Generation, "modelGen", m.loadGeneration)
		return m, nil
	}

	m.Loading = false
	m.switchingTrack = false
	// Clear loading line from normal screen
	fmt.Fprint(os.Stdout, "\033[2K\r")
	// Hide ConEmu progress bar
	fmt.Fprint(os.Stdout, "\033]9;4;0;0\a")
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
	if msg.Player.Album() != "" {
		m.Audio.Album = msg.Player.Album()
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
		preferred := m.preferredLyricFormat
		m.preferredLyricFormat = ""
		data, err := lyrics.FindAndParsePreferred(msg.Path, m.Config.Lyrics.FormatPriority, preferred)
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
		// Push new position to NowPlaying immediately so Control Center
		// doesn't briefly show the old position before the next tick.
		if m.MediaCtl != nil {
			m.MediaCtl.Update(m.buildPlayState())
		}
	case mediactl.CmdSetPosition:
		// MPRIS SetPosition is absolute position in microseconds
		pos := time.Duration(msg.Command.Value) * time.Microsecond
		m.Audio.SeekPlayer(pos)
		if m.MediaCtl != nil {
			m.MediaCtl.Update(m.buildPlayState())
		}
	}

	return m, nil
}

// handleLoadTrack switches to a new audio track at runtime.
// It releases the current player, preserves volume/playback settings,
// and synchronizes lyrics with a preference for the same format used
// by the previous track.
func handleLoadTrack(m *Model, msg LoadTrackMsg) (tea.Model, tea.Cmd) {
	logger.Info("Loading new track", "path", msg.Path)

	// Increment generation to invalidate any in-flight load from a previous track
	m.loadGeneration++

	// Remember the current lyrics format to try it first on the new track
	if m.Audio.Lyrics != nil && m.Audio.Lyrics.Format != "" {
		m.preferredLyricFormat = m.Audio.Lyrics.Format
	} else {
		m.preferredLyricFormat = ""
	}

	// Release current player
	m.Audio.Close()

	// Reset audio state but preserve volume
	savedVolume := m.Audio.Volume
	m.Audio.Player = nil
	m.Audio.CurrentSong = ""
	m.Audio.Artist = ""
	m.Audio.Album = ""
	m.Audio.Progress = 0
	m.Audio.Duration = 0
	m.Audio.Elapsed = 0
	m.Audio.IsPlaying = false
	m.Audio.Lyrics = nil
	m.Audio.LyricIndex = -1
	m.Audio.LyricScrollOffset = 0
	m.Audio.LyricScrollTick = 0
	m.Audio.LastLyricIndex = 0
	m.Audio.ActiveLyricLines = nil
	m.Audio.lastActiveSig = ""
	m.Audio.LyricNextIndex = -1
	m.Audio.LyricGapDuration = 0
	m.Audio.Volume = savedVolume

	// Reset accent
	m.Accent = nil
	m.rebuildProgressBar()

	m.Loading = true
	m.loadingTick = 0
	m.switchingTrack = true

	// Update media control with cleared metadata
	if m.MediaCtl != nil {
		m.MediaCtl.Update(m.buildPlayState())
	}

	return m, func() tea.Msg {
		fmt.Fprint(os.Stdout, "\033]9;4;3;0\a")
		return loadAudio(msg.Path, m.Config.SoundfontPath, m.Config.TrackerBackend, m.loadGeneration)
	}
}

// parseIPCMessage parses a JSON line received from the GUI via IPC.
// Returns the path to load and true for "open" messages, or empty/false.
func parseIPCMessage(line string) (string, bool) {
	var msg ipc.Message
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		logger.Debug("IPC: invalid JSON", "line", line, "err", err)
		return "", false
	}
	switch msg.Type {
	case "open":
		if msg.Path != "" {
			return msg.Path, true
		}
	default:
		logger.Debug("IPC: unhandled message type", "type", msg.Type)
	}
	return "", false
}
