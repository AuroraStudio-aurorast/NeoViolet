package ui

import "strings"

// ghostSuggestions is the list textinput filters for its inline ghost text:
// every command name and alias.
func ghostSuggestions() []string {
	names := make([]string, 0, len(commands)*2)
	for _, spec := range commands {
		names = append(names, spec.Name)
		names = append(names, spec.Aliases...)
	}
	return names
}

// syncGhostSuggestions keeps textinput's own suggestion list in step with the
// input line.
//
// A blank line gets no suggestions on purpose: textinput keeps matched
// suggestions across Reset/SetValue and only recomputes them from the list it
// already holds, so a stale entry would make <tab> on an empty command line
// complete a command the user never typed.
func syncGhostSuggestions(m *Model) {
	ti := &m.Components.CommandInput
	if strings.TrimSpace(ti.Value()) == "" {
		ti.SetSuggestions(nil)
		return
	}
	ti.SetSuggestions(ghostSuggestions())
}

// syncCompletion refreshes every piece of completion state that depends on the
// input line.
func syncCompletion(m *Model) {
	syncGhostSuggestions(m)
}
