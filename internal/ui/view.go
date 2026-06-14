package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
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
		// Braille dot spinner (npm-style): ⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		idx := m.loadingTick / 6 % len(frames)
		return tea.NewView(loadingStyle.Render(frames[idx] + " Loading..."))
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
				s = s.Copy().BorderForeground(lipgloss.Color("15"))
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
		s = s.BorderForeground(lipgloss.Color("15"))
	}

	// When lyrics are rendered in the footer, they take 1 row (footerHeight=6).
	// Without lyrics the footer is only 5 rows, so content gets that row back.
	offset := contentOffset
	if !(m.Audio.Lyrics != nil && m.Audio.ShowLyrics) {
		offset = contentOffset - 1
	}

	return s.
		Width(m.UI.Width).
		Height(m.UI.Height - offset).
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
			songLine = m.Audio.CurrentSong
		} else {
			songLine = fmt.Sprintf("%s - %s", m.Audio.CurrentSong, artist)
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
	availableWidth := m.UI.Width - 4
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

	// Lyric rendering: single or multiple lines joined by " | "
	var lyricText string
	maxWidth := m.UI.Width - 6

	if m.Audio.Lyrics != nil && m.Audio.ShowLyrics {
		if len(m.Audio.ActiveLyricLines) > 0 {
			active := m.Audio.ActiveLyricLines
			parts := make([]string, 0, len(active))
			for _, line := range active {
				parts = append(parts, m.Audio.Lyrics.LineDisplayText(line))
			}
			lyricText = strings.Join(parts, " | ")
		} else if m.Audio.LyricNextIndex >= 0 && m.Audio.LyricNextIndex < len(m.Audio.Lyrics.Lines) {
			// Show waiting dots if the total gap (previous line end to next line start)
			// exceeds 5 seconds, otherwise use a simple placeholder.
			if m.Audio.LyricGapDuration > 5*time.Second {
				// Dots animate in the last 3 seconds before the lyric begins.
				nextLine := m.Audio.Lyrics.Lines[m.Audio.LyricNextIndex]
				remaining := nextLine.Time - m.Audio.Elapsed
				secs := remaining.Seconds()
				dots := buildLyricCountdown(secs, m.Icons.LyricFilled, m.Icons.LyricEmpty)
				nextText := m.Audio.Lyrics.LineDisplayText(nextLine)
				lyricText = dots + "  " + nextText
			} else {
				lyricText = "-"
			}
		} else {
			// Past end or no upcoming lyric: show placeholder
			lyricText = "-"
		}
	}

	var lyricRows []string
	if lyricText != "" {
		lyricRows = append(lyricRows, renderSingleLyricLine(lyricText, maxWidth, m.Audio.LyricScrollOffset, m.Accent))
	}

	// Combine all lines vertically
	elements := []string{songLine, progressLine, volumeLine}
	for _, row := range lyricRows {
		elements = append(elements, row)
	}
	content := lipgloss.JoinVertical(lipgloss.Top, elements...)

	s := footerStyle.Copy()
	if m.UI.Focus == FocusFooter {
		s = s.BorderForeground(lipgloss.Color("15"))
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

// renderSingleLyricLine renders a single lyric line with marquee scroll support.
func renderSingleLyricLine(lineText string, maxWidth int, scrollOffset int, accent *accent.Accent) string {
	runes := []rune(lineText)
	displayWidth := lipgloss.Width(lineText)
	if displayWidth > maxWidth {
		start := scrollOffset
		runeStart := 0
		for i, w := 0, 0; i < len(runes) && w < start; i++ {
			w += lipgloss.Width(string(runes[i]))
			runeStart = i + 1
		}
		if runeStart >= len(runes) {
			runeStart = 0
		}
		runeEnd := runeStart
		for w := 0; runeEnd < len(runes); runeEnd++ {
			cw := lipgloss.Width(string(runes[runeEnd]))
			if w+cw > maxWidth {
				break
			}
			w += cw
		}
		visible := string(runes[runeStart:runeEnd])
		padWidth := maxWidth - lipgloss.Width(visible)
		if padWidth > 0 {
			visible += fmt.Sprintf("%*s", padWidth, "")
		}
		return lyricStyle.Copy().
			Foreground(lipgloss.Color(accentOrDefault(accent, "141"))).
			Width(maxWidth).Render(visible)
	}
	return lyricStyle.Copy().
		Foreground(lipgloss.Color(accentOrDefault(accent, "141"))).
		Width(maxWidth).Render(lineText)
}

// buildLyricCountdown returns a 3-dot countdown string based on seconds remaining
// until the next lyric line. Icon-theme-aware via filled/empty parameters.
// Only called when secs > 5 (long gap); the dots animate in the last 3 seconds:
//
//	s > 3s:  ○ ○ ○  (long wait, all empty)
//	2-3s:    ○ ○ ●  (one filled, countdown begins)
//	1-2s:    ○ ● ●  (two filled)
//	≤ 1s:    ● ● ●  (all filled, imminent)
func buildLyricCountdown(secs float64, filled, empty string) string {
	var a, b, c string
	switch {
	case secs > 3:
		a, b, c = empty, empty, empty
	case secs > 2:
		a, b, c = empty, empty, filled
	case secs > 1:
		a, b, c = empty, filled, filled
	default:
		a, b, c = filled, filled, filled
	}
	return a + " " + b + " " + c
}

func renderHelp(m *Model) string {
	if m.UI.Mode == ModeCommand {
		return inputStyle.Render(m.Icons.Command + m.Components.CommandInput.View())
	}

	if m.Info.Message != "" && m.Info.Visible {
		return infoStyle.Width(m.UI.Width).Render(" " + m.Info.Message)
	}

	if m.Error.Message != "" && m.Error.Visible {
		return errorStyle.Width(m.UI.Width).Render(" " + m.Error.Message)
	}

	return m.Components.Help.View(keys)
}
