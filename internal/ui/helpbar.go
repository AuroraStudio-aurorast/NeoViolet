package ui

import "charm.land/bubbles/v2/key"

// shortHelpBindings describes the actions available in the current region.
// These are display copies: their concise labels never change the key map.
func shortHelpBindings(m *Model) []key.Binding {
	focus := keys.CycleFocus
	focus.SetHelp(focus.Help().Key, "focus")
	command := keys.Command
	command.SetHelp(command.Help().Key, "command")
	bindings := []key.Binding{focus}

	switch m.UI.Focus {
	case FocusTabBar:
		tabs := keys.TabNext
		tabs.SetHelp("←/→", "tabs")
		bindings = append(bindings, tabs)
	case FocusContent:
		tabs := keys.TabNext
		tabs.SetHelp("[/]", "tabs")
		bindings = append(bindings, tabs)
	case FocusFooter:
		if m.Audio.Player != nil {
			seek := keys.SeekForward
			seek.SetHelp("←/→", "seek")
			bindings = append(bindings, seek)
		}
		volume := keys.VolumeUp
		volume.SetHelp("+/−", "volume")
		bindings = append(bindings, volume)
	}

	if m.Audio.Player != nil {
		play := keys.Play
		if m.Audio.IsPlaying {
			play = keys.Pause
		}
		bindings = append(bindings, play)
	}
	bindings = append(bindings, command)
	if !m.isGUI() {
		bindings = append(bindings, keys.Quit)
	}
	return bindings
}
