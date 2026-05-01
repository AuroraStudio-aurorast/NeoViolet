package ui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func renderMainView(m *Model) tea.View {
	header := renderTabs(m)
	content := renderContent(m)
	footer := renderFooter(m)
	help := renderHelp(m)

	layout := lipgloss.NewStyle().
		Width(m.UI.Width).
		Height(m.UI.Height)

	view := tea.NewView(layout.Render(
		header + "\n" + content + "\n" + footer + "\n" + help,
	))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeAllMotion

	return view
}

func renderTabs(m *Model) string {
	var tabs []string

	for i, name := range m.UI.Tabs {
		tabContent := " " + name + " "
		if i == m.UI.ActiveTab {
			tabs = append(tabs, activeTabStyle.Width(m.UI.tabWidth).Render(tabContent))
		} else {
			tabs = append(tabs, tabStyle.Width(m.UI.tabWidth).Render(tabContent))
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func renderContent(m *Model) string {
	descriptions := map[int]string{
		0: "Browse your music library and manage the queue",
		1: "View and manage your playlists",
		2: "Adjust audio effects and enhancements",
		3: "Configure application settings",
	}

	content := fmt.Sprintf("[ %s ]\n\n%s",
		m.UI.Tabs[m.UI.ActiveTab],
		descriptions[m.UI.ActiveTab],
	)

	return contentStyle.
		Width(m.UI.Width - 2).
		Height(m.UI.Height - 10).
		Render(content)
}

func renderFooter(m *Model) string {
	icon := "▶"
	if m.Audio.IsPlaying {
		icon = "⏸"
	}

	songLine := fmt.Sprintf("%s  %s - %s", icon, m.Audio.CurrentSong, m.Audio.Artist)

	progressBar := m.Components.ProgressBar.ViewAs(m.Audio.Progress)
	timeDisplay := formatDuration(m.Audio.Elapsed) + " / " + formatDuration(m.Audio.Duration)
	
	// Combine progress bar and time display on one line
	progressLine := lipgloss.JoinHorizontal(lipgloss.Center,
		progressBar,
		" ",
		footerTextStyle.Render(timeDisplay),
	)

	volumeLabel := footerTextStyle.Render("VOL ")
	volumeBar := m.Components.VolumeBar.ViewAs(m.Audio.Volume)
	
	// Combine volume label and bar on one line
	volumeLine := lipgloss.JoinHorizontal(lipgloss.Left,
		volumeLabel,
		volumeBar,
	)

	// Combine all three lines vertically, then put them in one border
	content := lipgloss.JoinVertical(lipgloss.Top,
		songLine,
		progressLine,
		volumeLine,
	)

	return footerStyle.Width(m.UI.Width).Render(content)
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func renderHelp(m *Model) string {
	if m.UI.Mode == ModeCommand {
		inputStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("57")).
			Bold(true)
		return inputStyle.Render(":" + m.Components.CommandInput.View())
	}

	if m.Error.Message != "" && m.Error.Visible {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Background(lipgloss.Color("236"))
		return errorStyle.Width(m.UI.Width).Render(" " + m.Error.Message)
	}

	return m.Components.Help.View(keys)
}
