package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics/fetch"
)

func handleCommandModeKeyPress(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case normMatch(msg, keys.Quit):
		if m.isGUI() {
			return m, nil
		}
		m.cleanup()
		return m, tea.Quit

	case normMatch(msg, keys.NormalMode):
		m.UI.Mode = ModeNormal
		m.UI.Focus = m.UI.SavedFocus
		m.Components.CommandInput.Reset()
		m.Components.CommandInput.Blur()
		m.historyIndex = len(m.CommandHistory)
		return m, nil

	default:
		keyStr := msg.String()
		switch keyStr {
		case "enter":
			return executeCommand(m)
		case "up":
			if len(m.CommandHistory) == 0 {
				return m, nil
			}
			if m.historyIndex > 0 {
				m.historyIndex--
			}
			m.Components.CommandInput.SetValue(m.CommandHistory[m.historyIndex])
			m.Components.CommandInput.CursorEnd()
			return m, nil
		case "down":
			if m.historyIndex >= len(m.CommandHistory)-1 {
				m.historyIndex = len(m.CommandHistory)
				m.Components.CommandInput.Reset()
				return m, nil
			}
			m.historyIndex++
			m.Components.CommandInput.SetValue(m.CommandHistory[m.historyIndex])
			m.Components.CommandInput.CursorEnd()
			return m, nil
		default:
			m.historyIndex = len(m.CommandHistory)
			var cmd tea.Cmd
			m.Components.CommandInput, cmd = m.Components.CommandInput.Update(msg)
			return m, cmd
		}
	}
}

func executeCommand(m *Model) (tea.Model, tea.Cmd) {
	cmdText := m.Components.CommandInput.Value()
	m.Components.CommandInput.Reset()
	m.UI.Mode = ModeNormal
	m.UI.Focus = m.UI.SavedFocus

	logger.Info("Command executed", "cmd", cmdText)

	// Save command to history: move to top if exists, cap at 50
	if cmdText != "" {
		for i, s := range m.CommandHistory {
			if s == cmdText {
				m.CommandHistory = append(m.CommandHistory[:i], m.CommandHistory[i+1:]...)
				break
			}
		}
		m.CommandHistory = append(m.CommandHistory, cmdText)
		if len(m.CommandHistory) > m.Config.CommandHistory.Max {
			m.CommandHistory = m.CommandHistory[1:]
		}
		saveHistory(m)
	}
	m.historyIndex = len(m.CommandHistory)

	inv, ok := parseInvocation(cmdText)
	if !ok {
		return m, nil
	}
	spec, ok := commandLookup(inv.Name)
	if !ok {
		m.Error.Set(fmt.Sprintf("Unknown command: %s", cmdText), m.Config.Error.Duration)
		return m, nil
	}
	return spec.Run(m, inv)
}

// parseClockTime parses a "mm:ss" or "hh:mm:ss" clock string into a duration.
func parseClockTime(s string) (time.Duration, error) {
	parts := strings.Split(s, ":")
	var totalSeconds int
	switch len(parts) {
	case 2:
		mins, err1 := strconv.Atoi(parts[0])
		secs, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || secs < 0 || secs >= 60 {
			return 0, fmt.Errorf("invalid time, use <mm>:<ss> where ss < 60")
		}
		if mins < 0 {
			mins = 0
		}
		totalSeconds = mins*60 + secs
	case 3:
		hours, err1 := strconv.Atoi(parts[0])
		mins, err2 := strconv.Atoi(parts[1])
		secs, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil || mins < 0 || mins >= 60 || secs < 0 || secs >= 60 {
			return 0, fmt.Errorf("invalid time, use <hh>:<mm>:<ss> where mm, ss < 60")
		}
		if hours < 0 {
			hours = 0
		}
		totalSeconds = hours*3600 + mins*60 + secs
	default:
		return 0, fmt.Errorf("invalid time format, use <mm>:<ss> or <hh>:<mm>:<ss>")
	}
	return time.Duration(totalSeconds) * time.Second, nil
}

// fetchLyricsManual implements :lrc switch online. Unlike auto-fetch it
// bypasses the negative cache but never the provider cooldown.
func (m *Model) fetchLyricsManual() tea.Cmd {
	baseURL := effectiveBaseURL(m.Config.Lyrics.Fetch.BaseURL)
	if m.fetchRateLimit.Blocked(baseURL) {
		m.Error.Set(fmt.Sprintf("Rate limited, retry in %s", shortDur(m.fetchRateLimit.Remaining(baseURL))), m.Config.Error.Duration)
		return nil
	}
	title, artist := m.Audio.CurrentSong, m.Audio.Artist
	if title == "" || artist == "" || artist == "Unknown Artist" {
		m.Error.Set("Cannot fetch online lyrics: missing track metadata", m.Config.Error.Duration)
		return nil
	}
	sig := m.currentSig()
	m.fetchCache.Clear(sig) // bypass negative cache on explicit request
	m.fetchCache.Store(sig, fetch.CachePending, nil)
	m.LyricsFetching = true

	meta := fetch.TrackMeta{Title: title, Artist: artist, Album: m.Audio.Album, Duration: m.Audio.Duration.Seconds()}
	return m.buildFetchCmd(meta, sig, baseURL)
}

// shortDur renders a duration as "3m 12s" or "45s".
func shortDur(d time.Duration) string {
	d = d.Round(time.Second)
	if mins := int(d.Minutes()); mins > 0 {
		return fmt.Sprintf("%dm %ds", mins, int(d.Seconds())%60)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}
