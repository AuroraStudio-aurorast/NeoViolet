package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderTabs keeps labels at their natural cell widths. The active page stays
// marked when focus moves into content or playback; only its colours change.
func renderTabs(m *Model) string {
	palette := newInteractionPalette(m.Accent)
	icons := []string{m.Icons.Home, m.Icons.Playlist, m.Icons.Effects, m.Icons.Settings}
	var labels, baselines []string
	for i, name := range m.UI.Tabs {
		label := " " + name + " "
		if icons[i] != "" {
			label = " " + icons[i] + " " + name + " "
		}
		style, lineStyle := tabStyle, tabBaselineStyle
		line := strings.Repeat("─", lipgloss.Width(label))
		if i == m.UI.ActiveTab && !m.animVisible() {
			style = style.Foreground(palette.prompt).Background(palette.selected).Bold(true)
			lineStyle = lineStyle.Foreground(palette.prompt)
			line = strings.Repeat("━", lipgloss.Width(label))
			if m.UI.Focus == FocusTabBar {
				style = style.Foreground(lipgloss.Color("#171020")).Background(palette.prompt)
				lineStyle = lineStyle.Foreground(lipgloss.Color("#e8dcfa"))
			}
		}
		labels = append(labels, style.Render(label))
		baselines = append(baselines, lineStyle.Render(line))
	}
	header := " " + strings.Join(labels, "  ")
	baseline := tabBaselineStyle.Render("─") + strings.Join(baselines, tabBaselineStyle.Render("──"))
	baseline += tabBaselineStyle.Render(strings.Repeat("─", max(0, m.UI.Width-lipgloss.Width(baseline))))
	return lipgloss.NewStyle().Width(m.UI.Width).Render(header) + "\n" + baseline
}
