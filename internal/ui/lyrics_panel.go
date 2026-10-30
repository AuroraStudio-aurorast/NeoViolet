package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// This file holds the panel lyric display mode: a bordered box to the right of
// the content area showing the current line, with the surrounding lines around
// it. lyrics_one_line.go is the single-row counterpart.
//
// This file is the panel surface: how the box and its rows are drawn. Which
// lines the panel shows, how they are styled, and how many rows of them fit is
// decided by lyrics_panel_window.go and lyrics_panel_format.go; the wrap limit
// lives in panelDefaultMaxWrapRows there.

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
		if text == "Fetching lyrics" && innerH > 0 && innerW > 0 {
			rows = append(rows, strings.Split(renderFetchingLyrics(m, innerW, innerH), "\n")...)
		} else {
			rows = append(rows, panelEmptyStateRows(m, text, innerH, innerW)...)
		}
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
		return "No track loaded", true
	case m.Audio.Lyrics == nil && m.LyricsFetching:
		return "Fetching lyrics", true
	case m.Audio.Lyrics == nil:
		return "No lyrics for this track", true
	case len(m.Audio.Lyrics.VisibleLines()) == 0:
		return "No lyrics for this track", true
	default:
		return "", false
	}
}

func panelEmptyStateRows(m *Model, title string, height, width int) []string {
	rows := panelPlaceholderRows("", height, width)
	if height <= 0 || width <= 0 {
		return rows
	}
	action := "Try :lrc switch online"
	if m.Audio.Player == nil {
		action = "Use :open <path> to load audio"
	}
	titleRows := strings.Split(lipgloss.Wrap(title, width, ""), "\n")
	actionRows := strings.Split(lipgloss.Wrap(action, width, ""), "\n")
	titleCount := len(titleRows)
	block := append([]string{}, titleRows...)
	block = append(block, actionRows...)
	start := max(0, (height-len(block))/2)
	for i, line := range block {
		if start+i >= height {
			break
		}
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#d9d9d9"))
		if i >= titleCount {
			style = style.Foreground(newInteractionPalette(m.Accent).prompt)
		}
		rows[start+i] = centerLine(style.Render(line), width)
	}
	return rows
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

// panelCurrentStyle styles the current line: white and bold, and
// deliberately not italic (the panel repaints on every tick).
func panelCurrentStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true)
}

// panelCountdownStyle keeps the waiting indicator in the track's accent.
func panelCountdownStyle(m *Model) lipgloss.Style {
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
		s = ansi.Truncate(s, width, "…")
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
