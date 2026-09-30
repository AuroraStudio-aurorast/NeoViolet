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

	// warnAcceptKey answers a warning gate. It is deliberately not the space
	// bar: space is this program's play/pause, and a warning that starts
	// flashing imagery on the same key that resumes music would train the wrong
	// reflex. It is bound here rather than in KeyMap because it belongs to one
	// transient question, not to the keyboard as a whole.
	warnAcceptKey = key.NewBinding(key.WithKeys("enter"))
)

func handleNormalModeKeyPress(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// ─── Tier 0: keys that navigate away close the animation ───
	//
	// The animation borrows the content area, which belongs to the active tab, so
	// it is not on a tab of its own and no tab claims it. Any key that means "go
	// to a tab" therefore closes it first, and escape closes it where it stands.
	// Nothing else is intercepted: play/pause, seeking, volume and the command
	// line all keep working while it plays.
	if m.animVisible() {
		switch {
		case (m.Anim.Gated() || m.Anim.Held()) && normMatch(msg, warnAcceptKey):
			// Enter is what lets the animation through. One gate has it loaded
			// but not started, the other has not read it at all; escape closes
			// either, as it does anywhere.
			return m, m.Anim.Approve()
		case normMatch(msg, keys.NormalMode):
			m.Anim.Close()
			return m, nil
		case normMatch(msg, keys.TabNext), normMatch(msg, keys.TabPrev):
			m.Anim.Close()
		case m.UI.Focus == FocusTabBar && (normMatch(msg, arrowLeftKey) || normMatch(msg, arrowRightKey)):
			m.Anim.Close()
		}
	}

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
