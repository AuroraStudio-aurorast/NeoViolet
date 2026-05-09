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

	activeTabStyle = tabStyle.Copy().
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

	// Help styling
	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 1)

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

	// Lyric styling
	lyricStyle = lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("141"))
)
