package ui

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

// commandLineLayout splits the command row between the input and the right-hand
// notice, and reports whether the input's left edge needs an ellipsis and
// whether the notice still fits at all.
//
// Everything arrives as a value: this function reads no Model and no current
// input width, which is what keeps its result from oscillating.
func commandLineLayout(value, suggestion string, cursorAtEnd bool, notice string,
	fullWidth int) (inputWidth int, showEllipsis, showNotice bool) {
	pad := commandLinePad(value, suggestion, cursorAtEnd)
	showNotice = notice != ""
	wa := 0
	if showNotice {
		// The separating space is part of the reserved area.
		wa = 1 + lipgloss.Width(notice)
		if fullWidth-wa-pad < 1 {
			// No room for the notice: render the row without it.
			showNotice = false
			wa = 0
		}
	}

	inputWidth = fullWidth - wa - pad
	if inputWidth < 1 || lipgloss.Width(value) > inputWidth {
		showEllipsis = true
		inputWidth = fullWidth - wa - pad - 1
	}
	if inputWidth < 1 {
		inputWidth = 1
	}
	return inputWidth, showEllipsis, showNotice
}

// commandLinePad reports how many cells the textinput adds on top of its width:
// the cursor cell, plus the surplus of a ghost suggestion (it draws the whole
// suggestion while padding only for the value), plus one more cell when the
// cursor sits inside the window rather than at its end.
func commandLinePad(value, suggestion string, cursorAtEnd bool) int {
	extra := lipgloss.Width(suggestion) - lipgloss.Width(value)
	if extra < 0 {
		extra = 0
	}
	pad := extra
	if pad < 1 {
		pad = 1
	}
	if extra > 0 && !cursorAtEnd {
		pad++
	}
	return pad
}

// commandNotice derives the notice text. The count of a line that reached the
// limit wins over a refusal recorded earlier, because it describes the line
// itself rather than the last insertion attempt.
func commandNotice(m *Model) string {
	ti := &m.Components.CommandInput
	used := len([]rune(ti.Value()))
	if ti.CharLimit > 0 && used >= ti.CharLimit {
		return fmt.Sprintf("%d/%d", used, ti.CharLimit)
	}
	return m.UI.CommandNotice
}

// syncCommandInputWidth recomputes the input width from the current value,
// suggestion, cursor and notice, and pushes it into the textinput.
//
// bubbles does not recompute its window when only the width changes, so the
// cursor is pushed to the end (which always takes the recompute branch) and
// restored right after, before anything renders.
func syncCommandInputWidth(m *Model) {
	ti := &m.Components.CommandInput
	value := ti.Value()
	cursorAtEnd := ti.Position() >= len([]rune(value))
	width, _, _ := commandLineLayout(value, ti.CurrentSuggestion(), cursorAtEnd, commandNotice(m), m.UI.Width-1)

	pos := ti.Position()
	ti.SetWidth(width)
	ti.CursorEnd()
	ti.SetCursor(pos)
}
