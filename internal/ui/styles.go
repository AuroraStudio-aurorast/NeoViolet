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
	// Tab styling
	tabStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			MarginRight(1)

	activeTabStyle = tabStyle.
			BorderForeground(lipgloss.Color("57")).
			Bold(true)

	// Content area styling
	contentStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1, 2).
			Width(80)

	// Footer styling
	footerStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	footerTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	// Loading & warning styles
	loadingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("57")).
			Bold(true)

	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	// Input styling
	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("57")).
			Bold(true)

	// Error styling
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Background(lipgloss.Color("236"))

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
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	// panelContextStyle styles the lines surrounding the current one.
	panelContextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	// Completion overlay rows. Every span carries the background so the overlay
	// stays opaque where it covers the content box: a nested style
	// without a background would reset it and let the content show through.
	completionRowStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("245"))

	completionDescStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("243"))

	completionSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("57")).
				Foreground(lipgloss.Color("231")).
				Bold(true)

	completionSelectedDescStyle = lipgloss.NewStyle().
					Background(lipgloss.Color("57")).
					Foreground(lipgloss.Color("153"))

	// Command line notice: used for the line-limit count and for a refused
	// insertion, both of which render inside the command row itself.
	commandNoticeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Bold(true)
)
