package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderTabs keeps labels at their natural cell widths. The active page stays
// marked when focus moves into content or playback. Animation borrows content,
// so its ANIM status replaces the page selection and owns Content focus.
func renderTabs(m *Model) string {
	palette := newInteractionPalette(m.Accent)
	animation := m.animVisible()
	animationFocus := animation && m.UI.Focus == FocusTabBar
	icons := []string{m.Icons.Home, m.Icons.Playlist, m.Icons.Effects, m.Icons.Settings}
	var labels, baselines []string
	for i, name := range m.UI.Tabs {
		label := " " + name + " "
		if icons[i] != "" {
			label = " " + icons[i] + " " + name + " "
		}
		style, lineStyle := tabStyle, tabBaselineStyle
		if animationFocus {
			style = style.Foreground(palette.prompt).Bold(true)
			lineStyle = lineStyle.Foreground(palette.prompt)
		}
		line := strings.Repeat("─", lipgloss.Width(label))
		if i == m.UI.ActiveTab && !animation {
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
	if animation {
		status := tabStyle
		if m.UI.Focus == FocusContent {
			status = status.Foreground(lipgloss.Color("#171020")).Background(palette.prompt).Bold(true)
		}
		label := status.Render(" ANIM ")
		header += strings.Repeat(" ", max(0, m.UI.Width-lipgloss.Width(header)-lipgloss.Width(label)-2)) + label + "  "
	}
	baseline := tabBaselineStyle.Render("─") + strings.Join(baselines, tabBaselineStyle.Render("──"))
	baseline += tabBaselineStyle.Render(strings.Repeat("─", max(0, m.UI.Width-lipgloss.Width(baseline))))
	return lipgloss.NewStyle().Width(m.UI.Width).Render(header) + "\n" + baseline
}
