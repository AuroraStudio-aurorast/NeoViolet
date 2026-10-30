package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// renderStatus keeps transient feedback in one row without hiding the frame's
// gradient. Category words keep its meaning visible alongside the semantic ink.
func renderStatus(message *MessageState, width int, failure bool) string {
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
	return row + strings.Repeat(" ", max(0, width-lipgloss.Width(row)))
}
