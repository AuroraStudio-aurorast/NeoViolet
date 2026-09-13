package ui

import "charm.land/lipgloss/v2"

// styledSpan is a run of text with one style. The panel window is built from
// spans so a karaoke line can mix played and unplayed styling inside one row.
type styledSpan struct {
	Text  string
	Style lipgloss.Style
}

// spansWidth returns the display width of a span sequence in terminal cells.
func spansWidth(spans []styledSpan) int {
	w := 0
	for _, s := range spans {
		w += lipgloss.Width(s.Text)
	}
	return w
}

// wrapSpans breaks spans into rows of at most width display cells, at most
// maxRows rows.
//
// Every returned row is guaranteed to fit, so callers render rows without
// re-checking widths: runes wider than a whole row (a CJK glyph in a 1-column
// panel) are dropped, and text left over after the last allowed row is
// truncated with an ellipsis. width <= 0 or maxRows <= 0 yields nil.
func wrapSpans(spans []styledSpan, width, maxRows int) [][]styledSpan {
	if width <= 0 || maxRows <= 0 {
		return nil
	}

	rows := make([][]styledSpan, 0, maxRows)
	var row []styledSpan
	rowW := 0

	for _, sp := range spans {
		chunk := make([]rune, 0, len(sp.Text))
		chunkW := 0
		// flush moves the runes accumulated for this span into the row, keeping
		// them as one span so the style is applied once per row segment.
		flush := func() {
			if len(chunk) == 0 {
				return
			}
			row = append(row, styledSpan{Text: string(chunk), Style: sp.Style})
			rowW += chunkW
			chunk = chunk[:0]
			chunkW = 0
		}

		for _, r := range sp.Text {
			if r == '\n' || r == '\r' {
				// Panel rows are single terminal rows: a newline inside a cue
				// (multi-line SRT) would break the box, and lipgloss.Width
				// counts it as zero, so the row would also be mismeasured.
				r = ' '
			}
			rw := lipgloss.Width(string(r))
			if rw > width {
				continue // cannot be drawn in this width at all
			}
			if rowW+chunkW+rw > width {
				flush()
				if len(rows) == maxRows-1 {
					// This is the last row we may use: truncate the rest.
					return appendEllipsis(rows, row, width)
				}
				rows = append(rows, row)
				row = nil
				rowW = 0
			}
			chunk = append(chunk, r)
			chunkW += rw
		}
		flush()
	}

	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

// appendEllipsis closes the last usable row, dropping cells until the ellipsis
// itself fits inside width. dropLastCell removes one rune, which need not free a
// cell (a combining mark or ZWJ is zero cells wide), so keep dropping until the
// row is short enough instead of assuming one rune bought one cell.
func appendEllipsis(rows [][]styledSpan, row []styledSpan, width int) [][]styledSpan {
	for len(row) > 0 && spansWidth(row)+1 > width {
		row = dropLastCell(row)
	}
	style := lipgloss.NewStyle()
	if n := len(row); n > 0 {
		style = row[n-1].Style
	}
	if row = append(row, styledSpan{Text: "…", Style: style}); len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

// dropLastCell removes the final rune of the final span so an ellipsis fits.
func dropLastCell(row []styledSpan) []styledSpan {
	for len(row) > 0 {
		last := len(row) - 1
		runes := []rune(row[last].Text)
		if len(runes) == 0 {
			row = row[:last]
			continue
		}
		row[last].Text = string(runes[:len(runes)-1])
		return row
	}
	return row
}
