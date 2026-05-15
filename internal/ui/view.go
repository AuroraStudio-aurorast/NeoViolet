package ui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	minWidth      = 68
	minHeight     = 17
	tabsHeight    = 3
	footerHeight  = 6
	helpHeight    = 1
	contentOffset = tabsHeight + footerHeight + helpHeight
)

var appLayoutStyle = lipgloss.NewStyle()

func renderMainView(m *Model) tea.View {
	if m.Loading {
		return tea.NewView(loadingStyle.Render("Loading..."))
	}

	if m.UI.Width < minWidth || m.UI.Height < minHeight {
		msg := fmt.Sprintf("Terminal too small (%dx%d), resize to at least %dx%d",
			m.UI.Width, m.UI.Height, minWidth, minHeight)
		return tea.NewView(warnStyle.Width(m.UI.Width).Height(m.UI.Height).Align(lipgloss.Center, lipgloss.Center).Render(msg))
	}

	header := renderTabs(m)
	content := renderContent(m)
	footer := renderFooter(m)
	help := renderHelp(m)

	layout := appLayoutStyle.
		Width(m.UI.Width).
		Height(m.UI.Height)

	view := tea.NewView(layout.Render(
		header + "\n" + content + "\n" + footer + "\n" + help,
	))
	view.AltScreen = true

	if m.Audio.CurrentSong != "" {
		song := m.Audio.CurrentSong
		if m.Audio.Artist != "" && m.Audio.Artist != "Unknown Artist" {
			view.WindowTitle = "NeoViolet | " + song + " - " + m.Audio.Artist
		} else {
			view.WindowTitle = "NeoViolet | " + song
		}
	} else {
		view.WindowTitle = "NeoViolet"
	}

	return view
}

func renderTabs(m *Model) string {
	tabIcons := []string{m.Icons.Home, m.Icons.Playlist, m.Icons.Effects, m.Icons.Settings}
	var tabs []string

	for i, name := range m.UI.Tabs {
		prefix := tabIcons[i]
		tabContent := " " + prefix
		if prefix != "" {
			tabContent += " "
		}
		tabContent += name + " "
		if i == m.UI.ActiveTab {
			accented := activeTabStyle.Copy().BorderForeground(lipgloss.Color(accentOrDefault(m.Accent, "57")))
			tabs = append(tabs, accented.Width(m.UI.tabWidth).Render(tabContent))
		} else {
			s := tabStyle
			if m.UI.Focus == FocusTabBar {
				s = s.Copy().BorderForeground(lipgloss.Color(accentFocusOrDefault(m.Accent, "15")))
			}
			tabs = append(tabs, s.Width(m.UI.tabWidth).Render(tabContent))
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

	s := contentStyle.Copy()
	if m.UI.Focus == FocusContent {
		s = s.BorderForeground(lipgloss.Color(accentFocusOrDefault(m.Accent, "15")))
	}

	return s.
		Width(m.UI.Width).
		Height(m.UI.Height - contentOffset).
		Render(content)
}

func renderFooter(m *Model) string {
	icon := m.Icons.Play
	if m.Audio.IsPlaying {
		icon = m.Icons.Pause
	}

	var songLine string
	if m.Audio.Player != nil {
		artist := m.Audio.Artist
		if artist == "" || artist == "Unknown Artist" {
			songLine = fmt.Sprintf("%s  %s", icon, m.Audio.CurrentSong)
		} else {
			songLine = fmt.Sprintf("%s  %s - %s", icon, m.Audio.CurrentSong, artist)
		}
	} else {
		songLine = fmt.Sprintf("%s  No audio loaded", m.Icons.Music)
	}
	songLine = truncateLine(songLine, m.UI.Width-4)

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

	volumeLabel := footerTextStyle.Render(m.Icons.Volume + " ")
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
			lyricRow = lyricStyle.Copy().
				Foreground(lipgloss.Color(accentOrDefault(m.Accent, "141"))).
				Width(maxWidth).Render(visible)
		} else {
			lyricRow = lyricStyle.Copy().
				Foreground(lipgloss.Color(accentOrDefault(m.Accent, "141"))).
				Width(maxWidth).Render(lyricLine)
		}
	}

	// Combine all lines vertically
	elements := []string{songLine, progressLine, volumeLine}
	if lyricLine != "" {
		elements = append(elements, lyricRow)
	}
	content := lipgloss.JoinVertical(lipgloss.Top, elements...)

	s := footerStyle.Copy()
	if m.UI.Focus == FocusFooter {
		s = s.BorderForeground(lipgloss.Color(accentFocusOrDefault(m.Accent, "15")))
	}

	return s.Width(m.UI.Width).Render(content)
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

func truncateLine(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	width := 0
	for i, r := range runes {
		cw := lipgloss.Width(string(r))
		if width+cw > maxWidth-1 {
			return string(runes[:i]) + "…"
		}
		width += cw
	}
	return s
}

func renderHelp(m *Model) string {
	if m.UI.Mode == ModeCommand {
		return inputStyle.Render(m.Icons.Command + m.Components.CommandInput.View())
	}

	if m.Error.Message != "" && m.Error.Visible {
		return errorStyle.Width(m.UI.Width).Render(" " + m.Error.Message)
	}

	return m.Components.Help.View(keys)
}
