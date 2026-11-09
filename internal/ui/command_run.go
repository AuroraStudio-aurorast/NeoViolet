package ui

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// runSave implements ":w" / ":save".
func runSave(m *Model, _ invocation) (tea.Model, tea.Cmd) {
	return m, m.saveSettings(false)
}

// runSaveQuit implements ":wq". In GUI mode the wrapper is told to quit
// immediately, without its confirmation dialog.
func runSaveQuit(m *Model, _ invocation) (tea.Model, tea.Cmd) {
	return m, m.saveSettings(true)
}

// runQuit implements ":quit" / ":q". In GUI mode it asks the wrapper for
// confirmation for a loaded track or pending load. An idle session closes directly.
func runQuit(m *Model, _ invocation) (tea.Model, tea.Cmd) {
	if m.confirmSettingsQuit() {
		return m, nil
	}
	if m.ipcServer != nil {
		if err := m.requestGUIQuit(); err == nil {
			return m, nil
		}
	}
	m.cleanup()
	return m, tea.Quit
}

// runForceQuit implements ":quit!" / ":q!" / ":wq!": no cleanup, exit code 1.
func runForceQuit(m *Model, _ invocation) (tea.Model, tea.Cmd) {
	m.ExitCode = 1
	return m, tea.Quit
}

// runPlayPause implements ":p".
func runPlayPause(m *Model, _ invocation) (tea.Model, tea.Cmd) {
	m.togglePlayback()
	return m, nil
}

// runVolume implements ":vol <0.0-1.0>".
func runVolume(m *Model, inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.Parts) < 2 {
		m.Error.Set("Usage: vol <0.0-1.0>", m.Config.Error.Duration)
		return m, nil
	}
	vol, err := strconv.ParseFloat(inv.Parts[1], 64)
	if err != nil || vol < 0 || vol > 1.0 {
		m.Error.Set("Volume must be 0.0-1.0", m.Config.Error.Duration)
		return m, nil
	}
	vol = math.Round(vol*100) / 100
	m.setVolume(vol)
	return m, nil
}

// runSeek implements ":seek <seconds>|<mm:ss>|<hh:mm:ss>|+<offset>|-<offset>".
func runSeek(m *Model, inv invocation) (tea.Model, tea.Cmd) {
	if m.Audio.Player == nil {
		m.Error.Set("No audio loaded", m.Config.Error.Duration)
		return m, nil
	}
	if len(inv.Parts) < 2 {
		m.Error.Set("Usage: seek <seconds>, seek <mm:ss>, seek <hh:mm:ss>, seek +<offset>, seek -<offset>", m.Config.Error.Duration)
		return m, nil
	}
	arg := inv.Parts[1]

	switch {
	case strings.HasPrefix(arg, "+") || strings.HasPrefix(arg, "-"):
		rel, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			m.Error.Set("Invalid seek offset", m.Config.Error.Duration)
			return m, nil
		}
		m.Audio.SeekRelative(time.Duration(rel * float64(time.Second)))
	case strings.Contains(arg, ":"):
		pos, err := parseClockTime(arg)
		if err != nil {
			m.Error.Set(err.Error(), m.Config.Error.Duration)
			return m, nil
		}
		if m.Audio.Duration > 0 && pos > m.Audio.Duration {
			pos = m.Audio.Duration
		}
		_ = m.Audio.SeekPlayer(pos)
	default:
		seconds, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			m.Error.Set("Invalid seek position", m.Config.Error.Duration)
			return m, nil
		}
		newPos := time.Duration(seconds * float64(time.Second))
		if newPos < 0 {
			newPos = 0
		}
		if m.Audio.Duration > 0 && newPos > m.Audio.Duration {
			newPos = m.Audio.Duration
		}
		_ = m.Audio.SeekPlayer(newPos)
	}
	return m, nil
}

// runLrc implements ":lrc" and its aliases (see commands_lrc.go).
func runLrc(m *Model, inv invocation) (tea.Model, tea.Cmd) {
	return executeLrcCommand(m, inv.Parts)
}

// runOpen implements ":open [path]" / ":load" / ":e".
// Without a path, a capable attached GUI hosts the native audio picker.
func runOpen(m *Model, inv invocation) (tea.Model, tea.Cmd) {
	if inv.Rest == "" {
		if m.ipcAudioPicker && m.ipcSession != 0 {
			return m, m.requestAudioPicker()
		}
		m.Error.Set("Usage: open <path>", m.Config.Error.Duration)
		return m, nil
	}
	// Rest (not strings.Join(Parts[1:], " ")) so a file name with two adjacent
	// spaces survives; trailing whitespace is dropped because a dragged file
	// arrives as "path "; expandTilde so a completed "~/x.mp3" also opens.
	path := expandTilde(strings.TrimRightFunc(inv.Rest, unicode.IsSpace))
	if !isValidAudioPath(path) {
		m.Error.Set("Invalid or unsupported audio file: "+inv.Rest, m.Config.Error.Duration)
		return m, nil
	}
	return handleLoadTrack(m, LoadTrackMsg{Path: path})
}
