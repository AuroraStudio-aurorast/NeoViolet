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
// A blank line gets no suggestions on purpose: textinput's prefix match accepts
// every candidate for "", so leaving the full list in place would make <tab> on
// an empty command line complete the first command.
func syncGhostSuggestions(m *Model) {
	ti := &m.Components.CommandInput
	if strings.TrimSpace(ti.Value()) == "" {
		ti.SetSuggestions(nil)
		return
	}
	ti.SetSuggestions(ghostSuggestions())
}

// syncCompletion refreshes every piece of completion state after the input line
// changed. Phase 1 only feeds the ghost text; the candidate list is layered on
// top of it.
func syncCompletion(m *Model) {
	syncGhostSuggestions(m)
}
