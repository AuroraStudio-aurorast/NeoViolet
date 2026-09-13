package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// loadingFrames is the animated spinner shown during track loading.
var loadingFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var appLayoutStyle = lipgloss.NewStyle()

func renderMainView(m *Model) tea.View {
	if m.Loading && !m.switchingTrack {
		// Full-screen loading: only for initial startup load.
		// Runtime track switches keep the main UI visible.
		idx := m.loadingTick / 6 % len(loadingFrames)
		return tea.NewView(loadingStyle.Render(loadingFrames[idx] + " Loading..."))
	}

	if m.UI.Width < minWidth || m.UI.Height < minHeight {
		msg := fmt.Sprintf("Terminal too small (%dx%d), resize to at least %dx%d",
			m.UI.Width, m.UI.Height, minWidth, minHeight)
		return tea.NewView(warnStyle.Width(m.UI.Width).Height(m.UI.Height).Align(lipgloss.Center, lipgloss.Center).Render(msg))
	}

	plan := m.layoutPlan()

	header := renderTabs(m)
	content := renderContent(m, plan)
	if plan.PanelShown {
		content = lipgloss.JoinHorizontal(lipgloss.Top, content, renderLyricsPanel(m, plan))
	}
	footer := renderFooter(m, plan)
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
			accented := activeTabStyle.BorderForeground(lipgloss.Color(accentOrDefault(m.Accent, "57")))
			tabs = append(tabs, accented.Width(m.UI.tabWidth).Render(tabContent))
		} else {
			s := tabStyle
			if m.UI.Focus == FocusTabBar {
				s = s.BorderForeground(lipgloss.Color("15"))
			}
			tabs = append(tabs, s.Width(m.UI.tabWidth).Render(tabContent))
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func renderContent(m *Model, plan layoutPlan) string {
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

	s := contentStyle
	if m.UI.Focus == FocusContent {
		s = s.BorderForeground(lipgloss.Color("15"))
	}

	return s.
		Width(plan.ContentWidth).
		Height(plan.ContentHeight).
		Render(content)
}

func renderFooter(m *Model, plan layoutPlan) string {
	icon := m.Icons.Play
	if m.Audio.IsPlaying {
		icon = m.Icons.Pause
	}

	var songLine string
	if m.Audio.Player != nil {
		artist := m.Audio.Artist
		if artist == "" || artist == "Unknown Artist" {
			songLine = m.Audio.CurrentSong
		} else {
			songLine = fmt.Sprintf("%s - %s", m.Audio.CurrentSong, artist)
		}
	} else {
		songLine = fmt.Sprintf("%s  No audio loaded", m.Icons.Music)
	}
	songLine = truncateLine(songLine, plan.Width-4)

	timeDisplay := formatDuration(m.Audio.Elapsed) + " / " + formatDuration(m.Audio.Duration)

	// Set progress bar width based on available space
	timeWidth := lipgloss.Width(timeDisplay)
	pbWidth := plan.Width - 4 - timeWidth - 1
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

	whiteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	prevText := whiteStyle.Render(m.Icons.Prev)
	playPauseText := whiteStyle.Render(icon)
	nextText := whiteStyle.Render(m.Icons.Next)
	statusLine := lipgloss.JoinHorizontal(lipgloss.Left,
		prevText,
		" ",
		playPauseText,
		" ",
		nextText,
	)
	volumeLabel := footerTextStyle.Render(m.Icons.Volume + " ")
	volumeBar := m.Components.VolumeBar.ViewAs(m.Audio.Volume)
	volumeSection := lipgloss.JoinHorizontal(lipgloss.Left,
		volumeLabel,
		volumeBar,
	)

	// Combine play/pause icon (left) and volume section (right-aligned) on one line
	availableWidth := plan.Width - 4
	playWidth := lipgloss.Width(statusLine)
	volumeWidth := lipgloss.Width(volumeSection)
	spaceCount := availableWidth - playWidth - volumeWidth
	if spaceCount < 1 {
		spaceCount = 1
	}
	volumeLine := lipgloss.JoinHorizontal(lipgloss.Left,
		statusLine,
		strings.Repeat(" ", spaceCount),
		volumeSection,
	)

	// Lyric rendering (one_line mode): at most one row, marquee-scrolled. When
	// the panel is shown the footer has no lyric row at all.
	var lyricRows []string
	if plan.LyricMode == lyricModeOneLine {
		if text, ok := renderOneLineLyrics(m, plan.OneLineLyricWidth); ok {
			lyricRows = append(lyricRows, text)
		}
	}

	// Combine all lines vertically
	elements := []string{songLine, progressLine, volumeLine}
	elements = append(elements, lyricRows...)
	content := lipgloss.JoinVertical(lipgloss.Top, elements...)

	s := footerStyle
	if m.UI.Focus == FocusFooter {
		s = s.BorderForeground(lipgloss.Color("15"))
	}

	return s.Width(plan.Width).Render(content)
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

	if m.switchingTrack {
		idx := m.loadingTick / 6 % len(loadingFrames)
		return infoStyle.Width(m.UI.Width).Render(" " + loadingFrames[idx] + " Loading...")
	}

	if m.Info.Message != "" && m.Info.Visible {
		return infoStyle.Width(m.UI.Width).Render(" " + m.Info.Message)
	}

	if m.Error.Message != "" && m.Error.Visible {
		return errorStyle.Width(m.UI.Width).Render(" " + m.Error.Message)
	}

	return m.Components.Help.View(keys)
}
