package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

var (
	seekFwdChordKey = key.NewBinding(key.WithKeys("ctrl+f"))
	seekBwdChordKey = key.NewBinding(key.WithKeys("ctrl+b"))
	arrowLeftKey    = key.NewBinding(key.WithKeys("left"))
	arrowRightKey   = key.NewBinding(key.WithKeys("right"))
)

// fullwidthRune maps fullwidth Unicode characters/CJK keyboard to their ASCII equivalents.
func fullwidthRune(r rune) rune {
	switch r {
	case '：', '；':
		return ':'
	case '［', '【', '「':
		return '['
	case '］', '】', '」':
		return ']'
	case '／':
		return '/'
	case '－':
		return '-'
	case '＋':
		return '+'
	case '＝':
		return '='
	case '＞', '》':
		return '>'
	case '＜', '《':
		return '<'
	case '？':
		return '?'
	case '＇', '‘', '’':
		return '\''
	case '＂', '“', '”':
		return '"'
	case '＾':
		return '^'
	case '～':
		return '~'
	case '＿':
		return '_'
	case '＠':
		return '@'
	case '＃':
		return '#'
	case '％':
		return '%'
	case '＆':
		return '&'
	case '＊':
		return '*'
	case '（':
		return '('
	case '）':
		return ')'
	}
	return r
}

// normalizedKey wraps a key string so that key.Matches sees a normalized version
// with fullwidth characters mapped to ASCII.
type normalizedKey struct{ raw string }

func (k normalizedKey) String() string {
	return strings.Map(fullwidthRune, k.raw)
}

// normMatch is like key.Matches but normalizes fullwidth characters first.
func normMatch(msg tea.KeyPressMsg, b ...key.Binding) bool {
	return key.Matches(normalizedKey{msg.String()}, b...)
}

func handleKeyPress(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.UI.Mode == ModeCommand {
		return handleCommandModeKeyPress(m, msg)
	}
	return handleNormalModeKeyPress(m, msg)
}

func handleCommandModeKeyPress(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case normMatch(msg, keys.Quit):
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

func handleNormalModeKeyPress(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// ─── Tier 1: Always-global keys ───
	switch {
	case normMatch(msg, keys.Quit):
		m.cleanup()
		return m, tea.Quit

	case normMatch(msg, keys.NormalMode):
		m.QuitConfirm = false
		return m, nil

	case normMatch(msg, keys.Play), normMatch(msg, keys.Pause):
		m.togglePlayback()
		return m, nil

	case normMatch(msg, keys.Command):
		m.UI.SavedFocus = m.UI.Focus
		m.UI.Mode = ModeCommand
		cmd := m.Components.CommandInput.Focus()
		return m, cmd

	case normMatch(msg, keys.CycleFocus):
		m.UI.Focus = (m.UI.Focus + 1) % 3
		return m, nil

	case normMatch(msg, keys.TabNext):
		m.UI.ActiveTab = (m.UI.ActiveTab + 1) % len(m.UI.Tabs)
		return m, nil

	case normMatch(msg, keys.TabPrev):
		m.UI.ActiveTab = (m.UI.ActiveTab - 1 + len(m.UI.Tabs)) % len(m.UI.Tabs)
		return m, nil

	case normMatch(msg, keys.Next):
		return m, nil

	case normMatch(msg, keys.Prev):
		return m, nil

	case normMatch(msg, seekFwdChordKey):
		m.Audio.SeekRelative(time.Duration(m.Config.SeekStep) * time.Second)
		return m, nil

	case normMatch(msg, seekBwdChordKey):
		m.Audio.SeekRelative(-time.Duration(m.Config.SeekStep) * time.Second)
		return m, nil
	}

	// ─── Tier 2: Focus-aware keys ───
	switch m.UI.Focus {
	case FocusTabBar:
		if normMatch(msg, arrowLeftKey) {
			m.UI.ActiveTab = (m.UI.ActiveTab - 1 + len(m.UI.Tabs)) % len(m.UI.Tabs)
			return m, nil
		}
		if normMatch(msg, arrowRightKey) {
			m.UI.ActiveTab = (m.UI.ActiveTab + 1) % len(m.UI.Tabs)
			return m, nil
		}

	case FocusContent:
		// No focus-specific keys yet

	case FocusFooter:
		switch {
		case normMatch(msg, arrowLeftKey):
			m.Audio.SeekRelative(-time.Duration(m.Config.SeekStep) * time.Second)
			return m, nil
		case normMatch(msg, arrowRightKey):
			m.Audio.SeekRelative(time.Duration(m.Config.SeekStep) * time.Second)
			return m, nil
		case normMatch(msg, keys.VolumeUp):
			m.adjustVolume(m.Config.VolumeStep)
			return m, nil
		case normMatch(msg, keys.VolumeDown):
			m.adjustVolume(-m.Config.VolumeStep)
			return m, nil
		}
	}

	// ─── Tier 3: Fallback quit confirmation ───
	if keyStr == "q" || keyStr == "Q" {
		if m.QuitConfirm {
			m.cleanup()
			return m, tea.Quit
		}
		m.QuitConfirm = true
		return m, nil
	}
	return m, nil
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
		// Save config then quit gracefully
		m.Config.DefaultVolume = m.Audio.Volume
		if err := m.Config.Save(); err != nil {
			m.Error.Set(fmt.Sprintf("Save failed: %v", err), m.Config.Error.Duration)
		}
		m.cleanup()
		return m, tea.Quit

	case "quit", "q":
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
			parts := strings.Split(arg, ":")
			var totalSeconds int
			switch len(parts) {
			case 2:
				mins, err1 := strconv.Atoi(parts[0])
				secs, err2 := strconv.Atoi(parts[1])
				if err1 != nil || err2 != nil || secs < 0 || secs >= 60 {
					m.Error.Set("Invalid time, use <mm>:<ss> where ss < 60", m.Config.Error.Duration)
					return m, nil
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
					m.Error.Set("Invalid time, use <hh>:<mm>:<ss> where mm, ss < 60", m.Config.Error.Duration)
					return m, nil
				}
				if hours < 0 {
					hours = 0
				}
				totalSeconds = hours*3600 + mins*60 + secs
			default:
				m.Error.Set("Invalid time format, use <mm>:<ss> or <hh>:<mm>:<ss>", m.Config.Error.Duration)
				return m, nil
			}
			newPos := time.Duration(totalSeconds) * time.Second
			if m.Audio.Duration > 0 && newPos > m.Audio.Duration {
				newPos = m.Audio.Duration
			}
			m.Audio.SeekPlayer(newPos)
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

	default:
		m.Error.Set(fmt.Sprintf("Unknown command: %s", cmdText), m.Config.Error.Duration)
		return m, nil
	}
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
			m.Error.Set("Usage: lrc switch <format> (e.g. lrc, ttml, qrc, embedded)", m.Config.Error.Duration)
			return m, nil
		}
		if m.Audio.Player == nil {
			m.Error.Set("No audio loaded", m.Config.Error.Duration)
			return m, nil
		}
		format := parts[2]
		data, err := lyrics.FindAndParse(m.Audio.Player.Path(), []string{format})
		if err != nil {
			m.Error.Set(fmt.Sprintf("Failed to parse %s lyrics: %v", format, err), m.Config.Error.Duration)
			return m, nil
		}
		if data == nil {
			m.Error.Set(fmt.Sprintf("No lyrics found for format: %s", format), m.Config.Error.Duration)
			return m, nil
		}
		m.Audio.Lyrics = data
		m.Audio.LyricIndex = -1
		m.Audio.ShowLyrics = true
		return m, nil

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

	default:
		m.Error.Set(fmt.Sprintf("Unknown lrc subcommand: %s (use on, off, switch, or agent)", subcmd), m.Config.Error.Duration)
		return m, nil
	}
}
