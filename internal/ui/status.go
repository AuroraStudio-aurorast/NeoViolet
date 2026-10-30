package ui

import (
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// renderStatus keeps feedback and its dismissal countdown in one row. The
// shrinking surface reveals the frame's gradient without changing text ink.
func renderStatus(m *Model, message *MessageState, failure bool) string {
	width := m.UI.Width
	if width <= 0 {
		return ""
	}
	label, ink := "Info", lipgloss.Color("#d9d9d9")
	if message.Success {
		label, ink = "Saved", lipgloss.Color("#8fd0a2")
	}
	if failure {
		label, ink = "Error", lipgloss.Color("#ffb4b4")
	}
	text := strings.Join(strings.Fields(message.Message), " ")
	row := " " + lipgloss.NewStyle().Foreground(ink).Bold(true).Render(label) +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#dedede")).Render("  "+text)
	row = ansi.Truncate(row, width, "…")
	row += strings.Repeat(" ", max(0, width-lipgloss.Width(row)))
	if message.duration <= 1 || message.Timer <= 0 {
		return row
	}
	rate := 30
	if m.Config != nil && m.Config.TickRate > 0 {
		rate = m.Config.TickRate
	}
	// Keep the empty endpoint visible for about 160 ms within the existing
	// lifetime. Short messages reserve at least one frame for the initial fill.
	hold := min(message.duration-1, int(math.Ceil(float64(rate)*.16)))
	remaining := clampUnit(float64(message.Timer-hold) / float64(message.duration-hold))
	filled := int(math.Round(float64(width) * remaining))
	if filled == 0 {
		return row
	}
	bg := interactionTone(backgroundAccent(m.Accent), .035, 1)
	if message.Success {
		bg = color.RGBA{R: 37, G: 58, B: 45, A: 255}
	}
	if failure {
		bg = color.RGBA{R: 66, G: 36, B: 43, A: 255}
	}
	// Snap the boundary before a wide grapheme so the sweep never cuts text.
	head := ansi.Truncate(row, filled, "")
	tail := ansi.Cut(row, lipgloss.Width(head), width)
	return paintBackground(head, []color.RGBA{bg}) + "\x1b[49m" + tail
}
