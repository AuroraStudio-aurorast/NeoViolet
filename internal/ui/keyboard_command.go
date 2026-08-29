package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
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

	parts := strings.Fields(cmdText)
	if len(parts) == 0 {
		return m, nil
	}

	cmd := parts[0]
	var arg string
	if len(parts) > 1 {
		arg = parts[1]
	}

	switch cmd {
	case "w", "save":
		m.Config.DefaultVolume = m.Audio.Volume
		if err := m.Config.Save(); err != nil {
			m.Error.Set(fmt.Sprintf("Save failed: %v", err), m.Config.Error.Duration)
		}
		return m, nil

	case "wq":
		// Save config then quit gracefully.
		// In GUI mode, signal the wrapper to quit immediately (no dialog).
		m.Config.DefaultVolume = m.Audio.Volume
		if err := m.Config.Save(); err != nil {
			m.Error.Set(fmt.Sprintf("Save failed: %v", err), m.Config.Error.Duration)
		}
		if m.isGUI() {
			f := false
			_ = m.ipcServer.SendJSON(ipc.Message{Type: "quit", Dialog: &f})
		}
		m.cleanup()
		return m, tea.Quit

	case "quit", "q":
		// In GUI mode, request confirmation via the wrapper's close dialog
		// instead of quitting immediately. The wrapper may deny the quit
		// and keep the TUI running.
		if m.isGUI() {
			t := true
			_ = m.ipcServer.SendJSON(ipc.Message{Type: "quit", Dialog: &t})
			return m, nil
		}
		// Graceful quit with cleanup
		m.cleanup()
		return m, tea.Quit

	case "quit!", "q!", "wq!":
		// Force quit: no cleanup, exit with error code 1
		m.ExitCode = 1
		return m, tea.Quit

	case "p":
		m.togglePlayback()
		return m, nil

	case "vol":
		if arg == "" {
			m.Error.Set("Usage: vol <0.0-1.0>", m.Config.Error.Duration)
			return m, nil
		}
		vol, err := strconv.ParseFloat(arg, 64)
		if err != nil || vol < 0 || vol > 1.0 {
			m.Error.Set("Volume must be 0.0-1.0", m.Config.Error.Duration)
			return m, nil
		}
		vol = math.Round(vol*100) / 100
		m.Audio.Volume = vol
		if m.Audio.Player != nil {
			m.Audio.Player.SetVolume(vol)
		}
		m.Components.VolumeBar.SetPercent(vol)
		m.saveVolumeConfig()
		return m, nil

	case "seek":
		if m.Audio.Player == nil {
			m.Error.Set("No audio loaded", m.Config.Error.Duration)
			return m, nil
		}
		if arg == "" {
			m.Error.Set("Usage: seek <seconds>, seek <mm:ss>, seek <hh:mm:ss>, seek +<offset>, seek -<offset>", m.Config.Error.Duration)
			return m, nil
		}

		if strings.HasPrefix(arg, "+") || strings.HasPrefix(arg, "-") {
			rel, err := strconv.ParseFloat(arg, 64)
			if err != nil {
				m.Error.Set("Invalid seek offset", m.Config.Error.Duration)
				return m, nil
			}
			m.Audio.SeekRelative(time.Duration(rel * float64(time.Second)))
		} else if strings.Contains(arg, ":") {
			pos, err := parseClockTime(arg)
			if err != nil {
				m.Error.Set(err.Error(), m.Config.Error.Duration)
				return m, nil
			}
			if m.Audio.Duration > 0 && pos > m.Audio.Duration {
				pos = m.Audio.Duration
			}
			m.Audio.SeekPlayer(pos)
		} else {
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
			m.Audio.SeekPlayer(newPos)
		}
		return m, nil

	case "lrc", "lyric", "lyrics":
		return executeLrcCommand(m, parts)

	case "open", "load", "e":
		if len(parts) < 2 {
			m.Error.Set("Usage: open <path>", m.Config.Error.Duration)
			return m, nil
		}
		// Join remaining parts to support paths with spaces
		path := strings.Join(parts[1:], " ")
		if !isValidAudioPath(path) {
			m.Error.Set("Invalid or unsupported audio file: "+path, m.Config.Error.Duration)
			return m, nil
		}
		return handleLoadTrack(m, LoadTrackMsg{Path: path})

	default:
		m.Error.Set(fmt.Sprintf("Unknown command: %s", cmdText), m.Config.Error.Duration)
		return m, nil
	}
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
			return 0, fmt.Errorf("Invalid time, use <mm>:<ss> where ss < 60")
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
			return 0, fmt.Errorf("Invalid time, use <hh>:<mm>:<ss> where mm, ss < 60")
		}
		if hours < 0 {
			hours = 0
		}
		totalSeconds = hours*3600 + mins*60 + secs
	default:
		return 0, fmt.Errorf("Invalid time format, use <mm>:<ss> or <hh>:<mm>:<ss>")
	}
	return time.Duration(totalSeconds) * time.Second, nil
}

func executeLrcCommand(m *Model, parts []string) (tea.Model, tea.Cmd) {
	if len(parts) < 2 {
		if m.Audio.Lyrics != nil && m.Audio.ShowLyrics {
			m.Info.Set("Lyrics: enabled", m.Config.Error.Duration)
		} else {
			m.Info.Set("Lyrics: disabled", m.Config.Error.Duration)
		}
		return m, nil
	}

	subcmd := parts[1]

	switch subcmd {
	case "on":
		if m.Audio.Lyrics != nil {
			m.Audio.ShowLyrics = true
			return m, nil
		}
		if m.Audio.Player == nil {
			m.Error.Set("No audio loaded", m.Config.Error.Duration)
			return m, nil
		}
		data, err := lyrics.FindAndParse(m.Audio.Player.Path(), m.Config.Lyrics.FormatPriority)
		if err != nil {
			m.Error.Set(fmt.Sprintf("Failed to load lyrics: %v", err), m.Config.Error.Duration)
			return m, nil
		}
		if data == nil {
			m.Error.Set("No lyrics found for current track", m.Config.Error.Duration)
			return m, nil
		}
		m.Audio.Lyrics = data
		m.Audio.LyricIndex = -1
		m.Audio.ShowLyrics = true
		return m, nil

	case "off":
		m.Audio.ShowLyrics = false
		return m, nil

	case "switch":
		if len(parts) < 3 {
			m.Error.Set(fmt.Sprintf("Usage: lrc switch <format> (available: %s)", strings.Join(lyrics.AvailableParsers(), ", ")), m.Config.Error.Duration)
			return m, nil
		}
		if m.Audio.Player == nil {
			m.Error.Set("No audio loaded", m.Config.Error.Duration)
			return m, nil
		}
		format := parts[2]
		if format == "online" {
			return m, m.fetchLyricsManual()
		}
		data, err := lyrics.FindAndParse(m.Audio.Player.Path(), []string{format})
		if err != nil {
			m.Error.Set(fmt.Sprintf("Failed to parse %s lyrics: %v", format, err), m.Config.Error.Duration)
			return m, nil
		}
		if data == nil {
			m.Error.Set(fmt.Sprintf("No lyrics found for format: %s (available: %s)", format, strings.Join(lyrics.AvailableParsers(), ", ")), m.Config.Error.Duration)
			return m, nil
		}
		m.Audio.Lyrics = data
		m.Audio.LyricIndex = -1
		m.Audio.ShowLyrics = true
		return m, nil

	case "refresh":
		// Bypass the disk cache (including negative "no lyrics" records) and
		// refetch. The session cache is cleared inside fetchLyricsManual.
		if m.Audio.Player == nil {
			m.Error.Set("No audio loaded", m.Config.Error.Duration)
			return m, nil
		}
		if err := fetch.RemoveCache(m.currentSig()); err != nil {
			m.Error.Set(fmt.Sprintf("Failed to clear lyrics cache: %v", err), m.Config.Error.Duration)
			return m, nil
		}
		m.Info.Set("Lyrics cache cleared, refetching", m.Config.Error.Duration)
		return m, m.fetchLyricsManual()

	case "agent":
		if len(parts) < 3 {
			if m.Audio.Lyrics != nil && m.Audio.Lyrics.AgentFilter != "" {
				m.Info.Set(fmt.Sprintf("Lyrics agent filter: %s", m.Audio.Lyrics.AgentFilter), m.Config.Error.Duration)
			} else {
				m.Info.Set("Lyrics agent filter: all", m.Config.Error.Duration)
			}
			return m, nil
		}
		if m.Audio.Lyrics == nil {
			m.Error.Set("No lyrics loaded", m.Config.Error.Duration)
			return m, nil
		}
		filter := parts[2]
		switch filter {
		case "all", "":
			m.Audio.Lyrics.AgentFilter = ""
			m.Info.Set("Lyrics: showing all agents", m.Config.Error.Duration)
		default:
			m.Audio.Lyrics.AgentFilter = filter
			m.Info.Set(fmt.Sprintf("Lyrics: filtering agent %s", filter), m.Config.Error.Duration)
		}
		m.Audio.LyricScrollOffset = 0
		m.Audio.LyricIndex = -1
		m.Audio.ActiveLyricLines = nil
		return m, nil

	case "desktop":
		m.DesktopLyricsEnabled = !m.DesktopLyricsEnabled
		t := m.DesktopLyricsEnabled
		if t {
			m.Info.Set("Desktop lyrics: enabled", m.Config.Error.Duration)
		} else {
			m.Info.Set("Desktop lyrics: disabled", m.Config.Error.Duration)
		}
		// Sync with GUI if connected
		if m.ipcServer != nil {
			_ = m.ipcServer.SendJSON(ipc.Message{
				Type:   "desktop_lyrics",
				Enable: &t,
			})
		}
		return m, nil

	default:
		m.Error.Set(fmt.Sprintf("Unknown lrc subcommand: %s (use on, off, switch, refresh, agent, or desktop)", subcmd), m.Config.Error.Duration)
		return m, nil
	}
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
