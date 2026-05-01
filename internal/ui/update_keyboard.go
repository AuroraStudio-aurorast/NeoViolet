package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

func handleKeyPress(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.UI.Mode == ModeCommand {
		return handleCommandModeKeyPress(m, msg)
	}
	return handleNormalModeKeyPress(m, msg)
}

func handleCommandModeKeyPress(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "enter":
		return executeCommand(m)
	case "escape", "esc":
		m.UI.Mode = ModeNormal
		m.Components.CommandInput.Reset()
		m.Components.CommandInput.Blur()
		m.historyIndex = len(m.CommandHistory)
		return m, nil
	case "ctrl+c", "ctrl+C", "ctrl+d", "ctrl+D":
		m.cleanup()
		return m, tea.Quit
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

func handleNormalModeKeyPress(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	
	switch key {
	case "q", "Q":
		if m.QuitConfirm {
			m.cleanup()
			return m, tea.Quit
		}
		m.QuitConfirm = true
		return m, nil
	case "esc", "Escape":
		m.QuitConfirm = false
		return m, nil
	case " ", "space":
		m.togglePlayback()
		return m, nil
	case ":", "/":
		m.UI.Mode = ModeCommand
		cmd := m.Components.CommandInput.Focus()
		return m, cmd
	case "+", "=":
		m.adjustVolume(0.1)
		return m, nil
	case "-", "_":
		m.adjustVolume(-0.1)
		return m, nil
	case ">", "l", "L":
		// Next track placeholder
		return m, nil
	case "<", "h", "H":
		// Previous track placeholder
		return m, nil
	case "tab", "Tab":
		m.UI.Focus = FocusContent
		return m, nil
	case "enter", "Enter":
		m.UI.Focus = FocusFooter
		return m, nil
	case "]", "n", "N":
		m.UI.ActiveTab = (m.UI.ActiveTab + 1) % len(m.UI.Tabs)
		return m, nil
	case "[", "p", "P":
		m.UI.ActiveTab = (m.UI.ActiveTab - 1 + len(m.UI.Tabs)) % len(m.UI.Tabs)
		return m, nil
	case "ctrl+f", "right", "Right":
		m.Audio.SeekRelative(5 * time.Second)
		return m, nil
	case "ctrl+b", "left", "Left":
		m.Audio.SeekRelative(-5 * time.Second)
		return m, nil
	case "ctrl+c", "ctrl+C":
		m.cleanup()
		return m, tea.Quit
	default:
		return m, nil
	}
}

func executeCommand(m *Model) (tea.Model, tea.Cmd) {
	cmdText := m.Components.CommandInput.Value()
	m.Components.CommandInput.Reset()
	m.UI.Mode = ModeNormal

	// Save command to history: move to top if exists, cap at 50
	if cmdText != "" {
		for i, s := range m.CommandHistory {
			if s == cmdText {
				m.CommandHistory = append(m.CommandHistory[:i], m.CommandHistory[i+1:]...)
				break
			}
		}
		m.CommandHistory = append(m.CommandHistory, cmdText)
		if len(m.CommandHistory) > 50 {
			m.CommandHistory = m.CommandHistory[1:]
		}
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
	case "quit", "q", "wq":
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
			m.Error.Set("Usage: vol <0.0-1.0>", 90)
			return m, nil
		}
		vol, err := strconv.ParseFloat(arg, 64)
		if err != nil || vol < 0 || vol > 1.0 {
			m.Error.Set("Volume must be 0.0-1.0", 90)
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
			m.Error.Set("No audio loaded", 90)
			return m, nil
		}
		if arg == "" {
			m.Error.Set("Usage: seek <seconds>, seek <mm:ss>, seek <hh:mm:ss>, seek +<offset>, seek -<offset>", 90)
			return m, nil
		}

		if strings.HasPrefix(arg, "+") || strings.HasPrefix(arg, "-") {
			rel, err := strconv.ParseFloat(arg, 64)
			if err != nil {
				m.Error.Set("Invalid seek offset", 90)
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
					m.Error.Set("Invalid time, use <mm>:<ss> where ss < 60", 90)
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
					m.Error.Set("Invalid time, use <hh>:<mm>:<ss> where mm, ss < 60", 90)
					return m, nil
				}
				if hours < 0 {
					hours = 0
				}
				totalSeconds = hours*3600 + mins*60 + secs
			default:
				m.Error.Set("Invalid time format, use <mm>:<ss> or <hh>:<mm>:<ss>", 90)
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
				m.Error.Set("Invalid seek position", 90)
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

	default:
		m.Error.Set(fmt.Sprintf("Unknown command: %s", cmdText), 90)
		return m, nil
	}
}