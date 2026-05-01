package ui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	minWidth  = 68
	minHeight = 17
)

func renderMainView(m *Model) tea.View {
	if m.Loading {
		loadingStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("57")).
			Bold(true)
		return tea.NewView(loadingStyle.Render("Loading..."))
	}

	if m.UI.Width < minWidth || m.UI.Height < minHeight {
		warnStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true).
			Width(m.UI.Width).
			Height(m.UI.Height).
			Align(lipgloss.Center, lipgloss.Center)
		msg := fmt.Sprintf("Terminal too small (%dx%d), resize to at least %dx%d",
			m.UI.Width, m.UI.Height, minWidth, minHeight)
		return tea.NewView(warnStyle.Render(msg))
	}

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

	return view
}

func renderTabs(m *Model) string {
	tabIcons := []string{Icons.Home, Icons.Playlist, Icons.Effects, Icons.Settings}
	var tabs []string

	for i, name := range m.UI.Tabs {
		prefix := tabIcons[i]
		tabContent := " " + prefix
		if prefix != "" {
			tabContent += " "
		}
		tabContent += name + " "
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
		Width(m.UI.Width).
		Height(m.UI.Height - 10).
		Render(content)
}

func renderFooter(m *Model) string {
	icon := Icons.Play
	if m.Audio.IsPlaying {
		icon = Icons.Pause
	}

	var songLine string
	if m.Audio.Player != nil {
		songLine = fmt.Sprintf("%s  %s - %s", icon, m.Audio.CurrentSong, m.Audio.Artist)
	} else {
		songLine = fmt.Sprintf("%s  No audio loaded", Icons.Music)
	}

	timeDisplay := formatDuration(m.Audio.Elapsed) + " / " + formatDuration(m.Audio.Duration)

	// Set progress bar width based on available space
	timeWidth := lipgloss.Width(timeDisplay)
	pbWidth := m.UI.Width - 4 - timeWidth - 1
	if pbWidth < 10 {
		pbWidth = 10
	}
	m.Components.ProgressBar.SetWidth(pbWidth)
	progressBar := m.Components.ProgressBar.ViewAs(m.Audio.Progress)

	// Combine progress bar and time display on one line
	progressLine := lipgloss.JoinHorizontal(lipgloss.Center,
		progressBar,
		" ",
		footerTextStyle.Render(timeDisplay),
	)

	volumeLabel := footerTextStyle.Render(Icons.Volume + " ")
	volumeBar := m.Components.VolumeBar.ViewAs(m.Audio.Volume)

	// Combine volume label and bar on one line
	volumeLine := lipgloss.JoinHorizontal(lipgloss.Left,
		volumeLabel,
		volumeBar,
	)

	// Current lyric line with marquee scroll
	var lyricLine string
	if m.Audio.Lyrics != nil && m.Audio.LyricIndex >= 0 && m.Audio.LyricIndex < len(m.Audio.Lyrics.Lines) {
		lyricLine = m.Audio.Lyrics.Lines[m.Audio.LyricIndex].Text
	}
	lyricStyle := lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("141"))
	maxWidth := m.UI.Width - 6

	lyricRow := ""
	if lyricLine != "" {
		runes := []rune(lyricLine)
		displayWidth := lipgloss.Width(lyricLine)
		if displayWidth > maxWidth {
			start := m.Audio.LyricScrollOffset
			// Find rune index corresponding to display offset
			runeStart := 0
			for i, w := 0, 0; i < len(runes) && w < start; i++ {
				w += lipgloss.Width(string(runes[i]))
				runeStart = i + 1
			}
			if runeStart >= len(runes) {
				runeStart = 0
			}
			// Accumulate runes up to maxWidth display cells
			runeEnd := runeStart
			for w := 0; runeEnd < len(runes); runeEnd++ {
				cw := lipgloss.Width(string(runes[runeEnd]))
				if w+cw > maxWidth {
					break
				}
				w += cw
			}
			visible := string(runes[runeStart:runeEnd])
			// Pad to exact display width
			padWidth := maxWidth - lipgloss.Width(visible)
			if padWidth > 0 {
				visible += fmt.Sprintf("%*s", padWidth, "")
			}
			lyricRow = lyricStyle.Width(maxWidth).Render(visible)
		} else {
			lyricRow = lyricStyle.Width(maxWidth).Render(lyricLine)
		}
	}

	// Combine all lines vertically
	elements := []string{songLine, progressLine, volumeLine}
	if lyricLine != "" {
		elements = append(elements, lyricRow)
	}
	content := lipgloss.JoinVertical(lipgloss.Top, elements...)

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
		return inputStyle.Render(Icons.Command + m.Components.CommandInput.View())
	}

	if m.Error.Message != "" && m.Error.Visible {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Background(lipgloss.Color("236"))
		return errorStyle.Width(m.UI.Width).Render(" " + m.Error.Message)
	}

	return m.Components.Help.View(keys)
}
