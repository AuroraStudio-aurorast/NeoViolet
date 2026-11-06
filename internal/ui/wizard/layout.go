package wizard

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func centerForm(width, height int, text string) string {
	lines := strings.Split(text, "\n")
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	used := 0
	for _, line := range lines {
		used = max(used, lipgloss.Width(strings.TrimRight(ansi.Strip(line), " ")))
	}
	used = min(used, width)
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, used, "")
		lines[i] += strings.Repeat(" ", max(0, used-lipgloss.Width(lines[i])))
	}
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, strings.Join(lines, "\n"))
}
