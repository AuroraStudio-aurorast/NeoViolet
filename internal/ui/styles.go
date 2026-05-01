package ui

import "charm.land/lipgloss/v2"

var (
	// Tab styling
	tabStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		MarginRight(1).
		Width(14)

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

	// Input styling
	inputStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	// Error styling
	errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Bold(true)

	// Help styling
	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Padding(0, 1)
)