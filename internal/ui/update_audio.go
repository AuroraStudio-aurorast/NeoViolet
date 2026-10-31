package ui

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/anim"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

func (m *Model) finishAudioLoad() {
	m.pendingLoad.cancel()
	m.pendingLoad = nil
	m.Loading = false
	m.switchingTrack = false
	fmt.Fprint(os.Stdout, "\033[2K\r")
	fmt.Fprint(os.Stdout, "\033]9;4;0;0\a")
}

func handleAudioLoadFailed(m *Model, msg AudioLoadFailedMsg) (tea.Model, tea.Cmd) {
	if m.exiting || msg.Generation != m.loadGeneration {
		return m, nil
	}
	m.finishAudioLoad()
	m.Error.Set(msg.Message, msg.Timer)
	return m, nil
}

func handleAudioLoaded(m *Model, msg AudioLoadedMsg) (tea.Model, tea.Cmd) {
	if m.exiting || msg.Generation != m.loadGeneration {
		discardAudioLoaded(msg)
		return m, nil
	}
	if msg.load != nil && !msg.load.take() {
		return m, nil
	}
	m.finishAudioLoad()

	current := m.Audio.Player
	wasPlaying := current != nil && current.IsPlaying()
	if wasPlaying {
		current.Pause()
	}
	msg.Player.SetVolume(m.Audio.Volume)
	if err := msg.Player.Play(); err != nil {
		_ = msg.Player.Close()
		message := fmt.Sprintf("Failed to start playback: %v", err)
		if wasPlaying {
			if resumeErr := current.Play(); resumeErr != nil {
				message += fmt.Sprintf("; failed to resume current track: %v", resumeErr)
			}
			m.Audio.IsPlaying = current.IsPlaying()
		}
		m.Error.Set(message, 180)
		return m, nil
	}

	m.Audio.Close()
	m.Audio.Reset()
	m.Audio.LastSentLyricSig = ""
	m.Accent = nil
	m.activeGeneration = msg.Generation
	m.rebuildProgressBar()
	m.LyricsFetching = false
	m.fetchCmd = nil
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

	m.Audio.IsPlaying = true

	// Apply --seek flag: seek to initial position after playing starts
	if m.pendingSeek > 0 {
		seekPos := m.pendingSeek
		if seekPos > m.Audio.Duration {
			seekPos = m.Audio.Duration
		}
		logger.Info("Initial seek", "to", seekPos)
		_ = m.Audio.SeekPlayer(seekPos)
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
		} else if cmd := m.maybeFetchLyrics(msg.Path); cmd != nil {
			m.fetchCmd = cmd
		}
	}

	cmds := []tea.Cmd{m.loadTrackAnimation(msg.Path)}
	if m.fetchCmd != nil {
		cmds = append(cmds, m.fetchCmd)
		m.fetchCmd = nil
	}
	if m.Config.Accent.IsEnabled() {
		cmds = append(cmds, loadAccentCmd(msg.Player, m.activeGeneration))
	}
	if m.MediaCtl != nil {
		m.MediaCtl.Update(m.buildPlayState())
	}
	return m, tea.Batch(cmds...)
}

// handleLoadTrack prepares a replacement while the current track stays active.
func handleLoadTrack(m *Model, msg LoadTrackMsg) (tea.Model, tea.Cmd) {
	if m.exiting {
		return m, nil
	}
	logger.Info("Loading new track", "path", msg.Path)
	m.loadGeneration++
	m.pendingSeek = 0
	if m.Audio.Lyrics != nil {
		m.preferredLyricFormat = m.Audio.Lyrics.Format
	} else {
		m.preferredLyricFormat = ""
	}
	m.Loading = true
	m.loadingTick = 0
	m.switchingTrack = true
	return m, m.startAudioLoad(msg.Path)
}

// Sidecars follow accepted audio only: a failed replacement leaves the current
// animation (and its warnings) untouched.
func (m *Model) loadTrackAnimation(path string) tea.Cmd {
	follow := m.animVisible() || m.animAuto() || (m.Anim != nil && m.Anim.Loading)
	held := m.Anim != nil && m.Anim.Held()
	m.Anim.Close()
	m.animRequested = false
	if !follow {
		return nil
	}
	plan := m.layoutPlan()
	load := anim.Load{Path: path, Columns: plan.ContentInnerW, Lines: plan.ContentInnerH, GateMode: m.gateMode()}
	if reading, low := m.Anim.LowBattery(m.warnBelow()); low {
		if held {
			m.Anim.Hold(reading, load)
		} else {
			m.Info.Set(animSkippedMessage(reading.Percent), m.Config.Error.Duration)
		}
		return nil
	}
	return m.Anim.LoadFor(load)
}
