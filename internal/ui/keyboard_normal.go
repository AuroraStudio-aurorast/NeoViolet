package ui

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

var (
	seekFwdChordKey = key.NewBinding(key.WithKeys("ctrl+f"))
	seekBwdChordKey = key.NewBinding(key.WithKeys("ctrl+b"))
	arrowLeftKey    = key.NewBinding(key.WithKeys("left"))
	arrowRightKey   = key.NewBinding(key.WithKeys("right"))
)

func handleNormalModeKeyPress(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// ─── Tier 1: Always-global keys ───
	switch {
	case normMatch(msg, keys.Quit):
		if m.isGUI() {
			return m, nil
		}
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
		syncCompletion(m)
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
	if !m.isGUI() && (keyStr == "q" || keyStr == "Q") {
		if m.QuitConfirm {
			m.cleanup()
			return m, tea.Quit
		}
		m.QuitConfirm = true
		return m, nil
	}
	return m, nil
}
