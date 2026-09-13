package ui

import (
	"strings"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

// This file holds the panel window: which lines the panel shows and how each
// line is styled. lyrics_panel.go owns the surface (the box and row rendering);
// this file selects the current line group, builds the window around it, and
// splits highlighted lines into played/unplayed spans.
//
// How the window is laid out (anchoring, the context cap, wrapping) is decided
// by panelFormat in lyrics_panel_format.go.

// panelWindow builds exactly plan.PanelInnerH rows: the current line group
// placed by the format's anchor, with the neighbouring lines filling the rest
// of the box. By default (no context cap) every row that has a lyric line to
// show is filled; only the very start and end of a song, or a song with fewer
// lines than the box has rows, leaves blank rows.
func panelWindow(m *Model, plan layoutPlan) []panelRow {
	innerH, innerW := plan.PanelInnerH, plan.PanelInnerW
	rows := make([]panelRow, maxInt(innerH, 0))
	if innerH <= 0 || innerW <= 0 || m.Audio.Lyrics == nil {
		return rows
	}

	visible := m.Audio.Lyrics.VisibleLines()
	if len(visible) == 0 {
		return rows
	}

	format := panelFormatFor(m)
	first, last, highlight := panelCurrent(m, visible)

	cur := make([]panelRow, 0, len(visible))
	for pos := first; pos <= last; pos++ {
		cur = append(cur, panelLineRows(m, visible[pos].Line, innerW, format.MaxWrapRows, highlight)...)
	}
	if len(cur) == 0 {
		return rows
	}
	if len(cur) > innerH {
		cur = cur[:innerH]
	}

	// Gather both sides in display order: the anchor decides how much of each
	// survives, so an over-long side is trimmed back towards the group.
	above := format.collectRows(m, visible, first-1, -1, innerW, innerH)
	below := format.collectRows(m, visible, last+1, 1, innerW, innerH)

	anchor := format.anchorRows(innerH, len(cur), len(above), len(below))
	if len(above) > anchor {
		above = above[len(above)-anchor:] // keep the lines closest to the group
	}
	if room := innerH - anchor - len(cur); len(below) > room {
		below = below[:maxInt(room, 0)]
	}

	place(rows, anchor-len(above), above)
	place(rows, anchor, cur)
	place(rows, anchor+len(cur), below)
	return rows
}

// place copies rs into rows starting at start, skipping out-of-range rows.
func place(rows []panelRow, start int, rs []panelRow) {
	for i, r := range rs {
		if at := start + i; at >= 0 && at < len(rows) {
			rows[at] = r
		}
	}
}

// panelCurrent returns the positions in visible of the current line group.
// highlight is false before the first line starts: nothing has been sung yet,
// so the first line is shown unhighlighted at the top of the window.
func panelCurrent(m *Model, visible []lyrics.VisibleLine) (first, last int, highlight bool) {
	if active := m.Audio.ActiveLyricLines; len(active) > 0 {
		first, last = -1, -1
		for i := range visible {
			if lineIsActive(visible[i].Line, active) {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first >= 0 {
			return first, last, true
		}
	}

	// No active line: hold the last line that already started. In a gap the
	// previous line stays on screen (no countdown dots in the panel).
	best := -1
	for i := range visible {
		if visible[i].Line.Time > m.Audio.Elapsed {
			break
		}
		best = i
	}
	if best < 0 {
		return 0, 0, false
	}
	return best, best, true
}

// lineIsActive reports whether line is one of the active lines.
func lineIsActive(line lyrics.LyricLine, active []lyrics.LyricLine) bool {
	for _, a := range active {
		if a.Time == line.Time && a.Text == line.Text {
			return true
		}
	}
	return false
}

// panelLineRows renders one lyric line into at most maxRows panel rows.
func panelLineRows(m *Model, line lyrics.LyricLine, innerW, maxRows int, highlight bool) []panelRow {
	wrapped := wrapSpans(panelLineSpans(m, line, highlight), innerW, maxRows)
	rows := make([]panelRow, 0, len(wrapped))
	for _, spans := range wrapped {
		rows = append(rows, panelRow{spans: spans})
	}
	return rows
}

// panelLineSpans styles a lyric line. The highlighted line is split into
// played/unplayed spans when the format carries word timings that tile the text.
func panelLineSpans(m *Model, line lyrics.LyricLine, highlight bool) []styledSpan {
	current := panelCurrentStyle(m)
	if !highlight {
		return []styledSpan{{Text: m.Audio.Lyrics.LineDisplayText(line), Style: panelContextStyle}}
	}
	if len(line.Words) == 0 {
		return []styledSpan{{Text: m.Audio.Lyrics.LineDisplayText(line), Style: current}}
	}

	played, rest := splitWordsAt(line, m.Audio.Elapsed)
	if played+rest != line.Text {
		// Word timings do not tile the text (translations, stray fragments):
		// fall back to a whole-line highlight rather than losing characters.
		return []styledSpan{{Text: m.Audio.Lyrics.LineDisplayText(line), Style: current}}
	}

	spans := make([]styledSpan, 0, 3)
	if prefix := agentPrefix(m.Audio.Lyrics, line); prefix != "" {
		spans = append(spans, styledSpan{Text: prefix, Style: current})
	}
	if played != "" {
		spans = append(spans, styledSpan{Text: played, Style: current})
	}
	if rest != "" {
		spans = append(spans, styledSpan{Text: rest, Style: panelContextStyle})
	}
	if len(spans) == 0 {
		spans = append(spans, styledSpan{Text: line.Text, Style: current})
	}
	return spans
}

// splitWordsAt splits a line's text at the word fragment covering elapsed.
func splitWordsAt(line lyrics.LyricLine, elapsed time.Duration) (played, rest string) {
	for _, w := range line.Words {
		if w.Time <= elapsed {
			played += w.Text
			continue
		}
		rest += w.Text
	}
	return played, rest
}

// agentPrefix returns the "NAME: " prefix LineDisplayText adds, or "".
func agentPrefix(d *lyrics.Data, line lyrics.LyricLine) string {
	if line.Agent == "" {
		return ""
	}
	return strings.TrimSuffix(d.LineDisplayText(line), line.Text)
}
