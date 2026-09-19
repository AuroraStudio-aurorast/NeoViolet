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
	currentTime := visible[first].Line.Time
	lit := func(line lyrics.LyricLine) bool {
		return panelLineIsCurrent(m, line, currentTime, highlight)
	}

	// The agent label marks a change of singer, so it is decided once for the
	// whole visible sequence and shared by the current group and the context
	// lines; deciding it per rendered window would relabel the same singer
	// whenever the window scrolled.
	labels := panelAgentLabels(visible)

	cur := make([]panelRow, 0, len(visible))
	for pos := first; pos <= last; pos++ {
		cur = append(cur, panelLineRows(m, visible[pos].Line, innerW, format.MaxWrapRows, lit(visible[pos].Line), labels[pos])...)
	}
	if len(cur) == 0 {
		return rows
	}
	// A long wait for the next line gets the countdown on its own row, directly
	// above the line it counts down to, so the wait is visible without moving the
	// lyrics the eye is on. In a gap that puts it under the line that just ended;
	// before the first line of the song nothing is held yet, so the group already is
	// the line being waited for and the dots lead it instead. They are part of the
	// group rather than of the context below, which keeps them in that position
	// however the rest of the window is configured. highlight is false exactly when
	// nothing is being sung (a gap or the wait before the first line), so a sung
	// line never counts down.
	if !highlight {
		if text, ok := lyricCountdownDots(m); ok {
			dots := panelRow{spans: []styledSpan{{Text: text, Style: panelCurrentStyle(m)}}}
			if currentTime > m.Audio.Elapsed {
				cur = append([]panelRow{dots}, cur...)
			} else {
				cur = append(cur, dots)
			}
		}
	}
	if len(cur) > innerH {
		cur = cur[:innerH]
	}

	// Gather both sides in display order: the anchor decides how much of each
	// survives, so an over-long side is trimmed back towards the group.
	above := format.collectRows(m, visible, first-1, -1, innerW, innerH, lit, labels)
	below := format.collectRows(m, visible, last+1, 1, innerW, innerH, lit, labels)

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
// highlight is false when nothing is being sung: before the first line starts,
// and in a gap, where the panel holds the last line that already started without
// drawing it as the current one (it has ended).
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

	// No active line: hold the last line that already started, so the eye stays
	// where the lyrics were. Nothing is being sung right now, so the held line is
	// not drawn as current: it has ended. Only a line carrying an end can leave the
	// panel in this state, because an unbounded line stays active until the next one
	// starts, so this never dims a line a format considers still playing.
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
	return best, best, false
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

// panelLineIsCurrent reports whether a line is drawn as "current": one of the
// parser's active lines, or a visible line at the same instant as the current
// group. The second rule lights up simultaneous lines that arrive as separate
// events (bilingual ESLRC, overlapping spans) without merging them, and judging
// each line instead of the whole first..last range stops the window from
// lighting lines that merely sit between two active ones.
func panelLineIsCurrent(m *Model, line lyrics.LyricLine, currentTime time.Duration, hasCurrent bool) bool {
	if lineIsActive(line, m.Audio.ActiveLyricLines) {
		return true
	}
	return hasCurrent && line.Time == currentTime
}

// panelLineRows renders one lyric line into panel rows: one block per display
// part, each part wrapped to at most maxRows. Most lines have a single part, so
// the common path is exactly one wrapSpans call. showAgent is the label decision
// for the line as a whole, so only its first part carries the label.
func panelLineRows(m *Model, line lyrics.LyricLine, innerW, maxRows int, highlight, showAgent bool) []panelRow {
	if line.PartCount() == 1 {
		return wrapLineRows(panelLineSpans(m, line, highlight, showAgent), innerW, maxRows)
	}
	rows := make([]panelRow, 0, line.PartCount())
	for i := 0; i < line.PartCount(); i++ {
		if strings.TrimSpace(line.Part(i)) == "" {
			continue // a blank part is data, not a row
		}
		// The label belongs to the line, not to each of its parts: the
		// translation row of a bilingual line must not repeat "NAME: ".
		rows = append(rows, wrapLineRows(panelLineSpans(m, partLine(line, i), highlight, i == 0 && showAgent), innerW, maxRows)...)
	}
	return rows
}

// wrapLineRows wraps styled spans into panel rows.
func wrapLineRows(spans []styledSpan, innerW, maxRows int) []panelRow {
	wrapped := wrapSpans(spans, innerW, maxRows)
	rows := make([]panelRow, 0, len(wrapped))
	for _, s := range wrapped {
		rows = append(rows, panelRow{spans: s})
	}
	return rows
}

// partLine returns a copy of line whose Text is its i-th display part. Words,
// Agent, Time and End stay untouched: panelLineSpans falls back to a whole-line
// highlight when the word timings do not tile the text, so the first part of a
// merged line keeps its karaoke and the remaining parts highlight as a whole
// without any branching here.
func partLine(line lyrics.LyricLine, i int) lyrics.LyricLine {
	line.Text = line.Part(i)
	return line
}

// panelLineSpans styles a lyric line. The highlighted line is split into
// played/unplayed spans when the format carries word timings that tile the text.
// showAgent adds the "NAME: " label; the panel shows it only where the singer
// changes, so when it is due it stays out of the karaoke split.
func panelLineSpans(m *Model, line lyrics.LyricLine, highlight, showAgent bool) []styledSpan {
	current := panelCurrentStyle(m)
	if !highlight {
		return []styledSpan{{Text: panelLineText(m.Audio.Lyrics, line, showAgent), Style: panelContextStyle}}
	}
	if len(line.Words) == 0 {
		return []styledSpan{{Text: panelLineText(m.Audio.Lyrics, line, showAgent), Style: current}}
	}

	played, rest := splitWordsAt(line, m.Audio.Elapsed)
	if played+rest != line.Text {
		// Word timings do not tile the text (translations, stray fragments):
		// fall back to a whole-line highlight rather than losing characters.
		return []styledSpan{{Text: panelLineText(m.Audio.Lyrics, line, showAgent), Style: current}}
	}

	spans := make([]styledSpan, 0, 3)
	if showAgent {
		if prefix := agentPrefix(m.Audio.Lyrics, line); prefix != "" {
			spans = append(spans, styledSpan{Text: prefix, Style: current})
		}
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

// panelLineText returns the text the panel draws for one line: the full display
// text (agent label included) when the label is due, otherwise the line's own
// text. LineDisplayText itself is untouched, because the one-line footer still
// labels every line it shows.
func panelLineText(d *lyrics.Data, line lyrics.LyricLine, showAgent bool) string {
	if !showAgent {
		return line.Text
	}
	return d.LineDisplayText(line)
}

// panelAgentLabels marks the lines whose agent label the panel shows. The label
// marks a change of singer rather than repeating the same name on every line:
// the first line carrying an agent is labelled, and so is every later line whose
// agent differs from the previous line that carried one. A line without an agent
// is never labelled and never resets the block, so a single-singer file is
// labelled once and an agentless interlude does not re-label the same singer.
func panelAgentLabels(visible []lyrics.VisibleLine) []bool {
	labels := make([]bool, len(visible))
	last := ""
	for i := range visible {
		if agent := visible[i].Line.Agent; agent != "" && agent != last {
			labels[i] = true
			last = agent
		}
	}
	return labels
}
