package ui

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

// commandLineLayout splits the command row between the input, the two edge
// ellipses and the right-hand notice, and reports which of them still fit.
//
// Everything arrives as a value: this function reads no Model and no current
// input width, which is what keeps its result from oscillating.
//
// cursor is a rune index into value. The right edge is reported when the text
// from the cursor onwards is wider than the window the textinput can show,
// because that widget anchors its window on the cursor and grows it to the
// right.
func commandLineLayout(value, suggestion string, cursor int, notice string,
	fullWidth int) (inputWidth int, showLeft, showRight, showNotice bool) {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	cursorAtEnd := cursor >= len(runes)
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
		// A width proxy: this can show even when nothing was cut on the left.
		showLeft = true
		inputWidth = fullWidth - wa - pad - 1
	}
	// The comparison uses the width left after the left-hand cell was reserved;
	// the deduction happens after it. The window shows inputWidth+1 cells from
	// the cursor, so the right edge is cut only once the tail is wider than that.
	if !cursorAtEnd && lipgloss.Width(string(runes[cursor:])) > inputWidth+1 {
		showRight = true
		inputWidth--
	}
	if inputWidth < 1 {
		inputWidth = 1
	}
	return inputWidth, showLeft, showRight, showNotice
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
// itself rather than the last insertion attempt. A refusal is therefore
// superseded by that derived count in the rendered row once the line sits at
// the limit; the stored notice is left untouched, and the priority is
// deliberate.
func commandNotice(m *Model) string {
	ti := &m.Components.CommandInput
	used := len([]rune(ti.Value()))
	if ti.CharLimit > 0 && used >= ti.CharLimit {
		return fmt.Sprintf("%d/%d", used, ti.CharLimit)
	}
	return m.UI.CommandNotice
}

// setCommandNotice records the notice text and re-syncs the input width: the
// notice is part of the row budget, so a notice that appears without a value
// change still has to resize the input.
func setCommandNotice(m *Model, text string) {
	m.UI.CommandNotice = text
	syncCommandInputWidth(m)
}

// renderCommandLine assembles the whole command row: the prompt, the two
// optional edge ellipses, the input itself, and the right-hand notice.
//
// Only the prompt and the input carry the command style. The ellipses stay
// outside it, so they render in the terminal's default colour.
func renderCommandLine(m *Model) string {
	ti := &m.Components.CommandInput
	value := ti.Value()
	notice := commandNotice(m)
	_, showLeft, showRight, showNotice := commandLineLayout(
		value, ti.CurrentSuggestion(), ti.Position(), notice, m.UI.Width-1)

	row := inputStyle.Render(m.Icons.Command)
	if showLeft {
		// A width proxy: this can show even when nothing was cut on the left.
		row += "…"
	}
	row += inputStyle.Render(ti.View())
	if showRight {
		// A width proxy as well: it reports the tail the window cannot hold.
		row += "…"
	}
	if showNotice {
		// The leading space is the separator the budget reserved a cell for.
		row += commandNoticeStyle.Render(" " + notice)
	}
	// The clamp is the last step, once every span is closed: it only trims the
	// tail, and trimming keeps the escape sequences that follow the cut.
	return clampRowWidth(row, m.UI.Width)
}

// clampRowWidth trims an over-wide row down to width cells. It is the safety net
// under the budget: the budget predicts what the textinput will render, and this
// trims what it actually rendered. Trimming only ever shortens the tail, so a
// row that already fits comes back unchanged.
func clampRowWidth(row string, width int) string {
	return lipgloss.NewStyle().MaxWidth(width).Render(row)
}

// syncCommandInputWidth recomputes the input width from the current value,
// suggestion, cursor and notice, and pushes it into the textinput. The layout and
// the render must both see the same cursor, so the position is read once here.
//
// bubbles does not recompute its window when only the width changes, so the
// cursor is pushed to the end (which always takes the recompute branch) and
// restored right after, before anything renders.
func syncCommandInputWidth(m *Model) {
	ti := &m.Components.CommandInput
	value := ti.Value()
	pos := ti.Position()
	width, _, _, _ := commandLineLayout(
		value, ti.CurrentSuggestion(), pos, commandNotice(m), m.UI.Width-1)

	ti.SetWidth(width)
	ti.CursorEnd()
	ti.SetCursor(pos)
}
