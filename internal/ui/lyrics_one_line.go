package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
)

// This file holds the one_line lyric display mode: a single footer row that
// marquee-scrolls when the line is wider than the footer. The code was moved
// out of view.go unchanged; lyrics_panel.go is the multi-line counterpart.

// renderOneLineLyrics renders the footer's single lyric row. ok is false when
// there is nothing to show, which is also what removes the row and gives its
// height back to the content area.
func renderOneLineLyrics(m *Model, maxWidth int) (string, bool) {
	text := oneLineLyricText(m)
	if text == "" {
		return "", false
	}
	return renderSingleLyricLine(text, maxWidth, m.Audio.LyricScrollOffset, m.Accent), true
}

// oneLineLyricText builds the text of the single lyric row: the active line(s)
// joined with " | ", a dot countdown while a long-awaited line is pending, or
// "-" as a placeholder. An empty return means "no row".
func oneLineLyricText(m *Model) string {
	if m.LyricsFetching && (m.Audio.Lyrics == nil || !m.Audio.ShowLyrics) {
		return "[Fetching lyrics...]"
	}
	if m.Audio.Lyrics == nil || !m.Audio.ShowLyrics {
		return ""
	}

	switch {
	case len(m.Audio.ActiveLyricLines) > 0:
		active := m.Audio.ActiveLyricLines
		parts := make([]string, 0, len(active))
		for _, line := range active {
			parts = append(parts, m.Audio.Lyrics.LineDisplayText(line))
		}
		return strings.Join(parts, " | ")

	case m.Audio.LyricNextIndex >= 0 && m.Audio.LyricNextIndex < len(m.Audio.Lyrics.Lines):
		next := m.Audio.Lyrics.Lines[m.Audio.LyricNextIndex]
		// Show waiting dots if the total gap (previous line end to next line
		// start) exceeds 5 seconds, otherwise use a simple placeholder. The dots
		// themselves animate only in the last 3 seconds.
		if m.Audio.LyricGapDuration > 5*time.Second {
			secs := (next.Time - m.Audio.Elapsed).Seconds()
			dots := buildLyricCountdown(secs, m.Icons.LyricFilled, m.Icons.LyricEmpty)
			return dots + "  " + m.Audio.Lyrics.LineDisplayText(next)
		}
		return "-"

	default:
		// Past the end, or no upcoming line.
		return "-"
	}
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
		return lyricStyle.
			Foreground(lipgloss.Color(accentOrDefault(accent, "141"))).
			Width(maxWidth).Render(visible)
	}
	return lyricStyle.
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
