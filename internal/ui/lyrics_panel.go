package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// This file holds the panel lyric display mode: a bordered box to the right of
// the content area showing the current line centred, with context lines above
// and below. lyrics_one_line.go is the single-row counterpart.
//
// This file is the panel surface: how the box and its rows are drawn. Which
// lines the panel shows and how they are styled lives in lyrics_panel_window.go.

// panelMaxWrapRows caps how many rows a single lyric line may occupy in the
// panel; longer text is truncated with an ellipsis.
const panelMaxWrapRows = 2

// panelRow is one writable row inside the panel; empty spans mean a blank row.
type panelRow struct {
	spans []styledSpan
}

// renderLyricsPanel renders the whole panel: a rounded box of exactly
// plan.ContentHeight rows and plan.PanelWidth columns.
func renderLyricsPanel(m *Model, plan layoutPlan) string {
	innerH, innerW := plan.PanelInnerH, plan.PanelInnerW
	rows := make([]string, 0, innerH)

	if text, ok := panelPlaceholderText(m); ok {
		rows = append(rows, panelPlaceholderRows(text, innerH, innerW)...)
	} else {
		for _, r := range panelWindow(m, plan) {
			rows = append(rows, renderPanelRow(r, innerW))
		}
		if len(rows) < innerH {
			rows = append(rows, panelPlaceholderRows("", innerH-len(rows), innerW)...)
		}
	}

	return panelStyle.
		Width(plan.PanelWidth).
		Height(plan.ContentHeight).
		Render(strings.Join(rows, "\n"))
}

// panelPlaceholderText returns the message shown when there is no lyric window
// to draw, or ok=false to render real lines instead.
func panelPlaceholderText(m *Model) (string, bool) {
	switch {
	case m.Audio.Player == nil:
		return m.Icons.Music + " No track", true
	case m.Audio.Lyrics == nil && m.LyricsFetching:
		return "[Fetching lyrics...]", true
	case m.Audio.Lyrics == nil:
		return m.Icons.Music + " No lyrics", true
	case len(m.Audio.Lyrics.VisibleLines()) == 0:
		return m.Icons.Music + " No lyrics", true
	default:
		return "", false
	}
}

// panelPlaceholderRows returns innerH blank rows with text centred in the middle
// row. An empty text is used to pad the tail of the window.
func panelPlaceholderRows(text string, innerH, innerW int) []string {
	if innerH <= 0 {
		return nil
	}
	rows := make([]string, innerH)
	for i := range rows {
		rows[i] = strings.Repeat(" ", maxInt(innerW, 0))
	}
	if text != "" {
		rows[innerH/2] = centerLine(text, innerW)
	}
	return rows
}

// panelCurrentStyle styles the current line: accent colour and bold, and
// deliberately not italic (the panel repaints on every tick).
func panelCurrentStyle(m *Model) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(accentOrDefault(m.Accent, "141"))).
		Bold(true)
}

// renderPanelRow renders one row to exactly innerW cells.
func renderPanelRow(r panelRow, innerW int) string {
	if innerW <= 0 {
		return ""
	}
	var b strings.Builder
	for _, s := range r.spans {
		if s.Text == "" {
			continue
		}
		b.WriteString(s.Style.Render(s.Text))
	}
	out := b.String()
	if w := lipgloss.Width(out); w < innerW {
		out += strings.Repeat(" ", innerW-w)
	}
	return out
}

// centerLine centres s in width cells, truncating if it does not fit.
func centerLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) > width {
		s = truncateLine(s, width)
	}
	gap := width - lipgloss.Width(s)
	left := gap / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", gap-left)
}

// maxInt returns the larger of a and b.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
