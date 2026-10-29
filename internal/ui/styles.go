package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
)

func accentOrDefault(a *accent.Accent, fallback string) string {
	if a == nil {
		return fallback
	}
	return a.HexMain()
}

var (
	// Tabs share a baseline; the current page and focus get palette colours.
	tabStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	tabBaselineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	contentHeadingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	contentStyle        = lipgloss.NewStyle().Padding(0, 1)

	footerStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	footerTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	// Loading & warning styles
	loadingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#af87ff")).
			Bold(true)

	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	// Info/success styling (green text on dark background)
	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")).
			Background(lipgloss.Color("236"))

	// Lyric styling
	lyricStyle = lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("141"))

	// Lyrics panel (right-hand sidebar). The panel is not focusable, so its
	// border never highlights.
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	// panelContextStyle styles the lines surrounding the current one.
	panelContextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))
)
