package ui

import "github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"

// This file is the panel's format seam: every knob that shapes what the panel
// writes into its box lives here, so future format work (wrapping, current line
// position, context policy, blank filling) changes one small surface instead of
// the renderer. lyrics_panel_window.go asks a panelFormat which lines are in the
// window; lyrics_panel.go draws the box around them.

// panelFormat is the set of knobs the panel window obeys.
type panelFormat struct {
	// AnchorNum/AnchorDen place the current line group: the group starts
	// AnchorNum/AnchorDen of the inner height down the box (1/3 by default).
	AnchorNum int
	AnchorDen int

	// ContextLines caps how many lyric lines the window takes on each side of
	// the current group. 0 means no cap: every row the box has is filled with
	// lyrics when the song has enough of them.
	ContextLines int

	// MaxWrapRows caps how many rows one lyric line may occupy; longer text is
	// truncated with an ellipsis.
	MaxWrapRows int
}

const (
	panelAnchorNum          = 1 // rows above the current line: inner height / 3
	panelAnchorDen          = 3
	panelDefaultMaxWrapRows = 2 // rows one lyric line may wrap to
)

// panelFormatFor maps the panel configuration onto a format. It is the only
// place the window logic reads panel configuration from, so tests can build a
// format directly instead of going through the model.
func panelFormatFor(m *Model) panelFormat {
	return panelFormat{
		AnchorNum:    panelAnchorNum,
		AnchorDen:    panelAnchorDen,
		ContextLines: m.Config.Lyrics.Panel.ContextLines,
		MaxWrapRows:  panelDefaultMaxWrapRows,
	}
}

// anchorTarget returns the preferred number of rows above the current line
// group, fitted so the group always fits inside the box.
func (f panelFormat) anchorTarget(innerH, curRows int) int {
	if innerH <= 0 || curRows <= 0 || f.AnchorDen <= 0 {
		return 0
	}
	return fitAnchor(innerH*f.AnchorNum/f.AnchorDen, innerH, curRows)
}

// fitAnchor clamps a desired first row so a group of curRows rows fits inside
// the box, which is also what keeps the two callers below consistent.
func fitAnchor(target, innerH, curRows int) int {
	if maxStart := innerH - curRows; target > maxStart {
		target = maxStart
	}
	if target < 0 {
		target = 0
	}
	return target
}

// anchorRows returns the row where the current line group starts.
//
// aboveRows/belowRows are the rows the surrounding lines actually need, already
// capped by ContextLines when it is set. With a cap the window is deliberately
// compact: the group stays on its target row and the rest of the box stays
// blank. Without a cap the window fills the box, so a side that runs out of
// lines hands its rows to the other one — the view scrolls instead of leaving a
// gap. That only applies when the box could be filled at all: a song with fewer
// lines than rows keeps its stable anchor.
func (f panelFormat) anchorRows(innerH, curRows, aboveRows, belowRows int) int {
	target := f.anchorTarget(innerH, curRows)
	if innerH <= 0 || curRows <= 0 || f.ContextLines > 0 {
		return target
	}
	if aboveRows < target {
		target = aboveRows // start of the song: hug the top
	}
	if aboveRows+curRows+belowRows >= innerH {
		if room := innerH - target - curRows; belowRows < room {
			target = innerH - curRows - belowRows // end of the song: hug the bottom
		}
	}
	return fitAnchor(target, innerH, curRows)
}

// collectRows walks away from the current group in dir (+1 down, -1 up)
// collecting whole lines as rows, and stops when the per-side line cap is
// reached or the box has no rows left. Rows come back in display order: an
// upward walk prepends each line as a whole block, so the two rows of a wrapped
// line stay in order instead of being reversed.
//
// lit decides the style of each line: a line can be current without being part
// of the current group (simultaneous events at the same instant are separate
// lines to the parser), and vice versa, so the caller owns that judgement.
//
// The cap counts lyric lines while innerH bounds rendered rows, so a line that
// wraps to two rows costs two of the budget.
//
// labels is the per-line agent label decision, indexed like visible, so a context
// line that starts a new singer keeps its label.
func (f panelFormat) collectRows(m *Model, visible []lyrics.VisibleLine, start, dir, innerW, innerH int, lit func(lyrics.LyricLine) bool, labels []bool) []panelRow {
	rows := make([]panelRow, 0, maxInt(innerH, 0))
	for pos, lines := start, 0; pos >= 0 && pos < len(visible); pos, lines = pos+dir, lines+1 {
		if f.ContextLines > 0 && lines >= f.ContextLines {
			break
		}
		lineRows := panelLineRows(m, visible[pos].Line, innerW, f.MaxWrapRows, lit(visible[pos].Line), labels[pos])
		if dir < 0 {
			rows = append(lineRows, rows...)
		} else {
			rows = append(rows, lineRows...)
		}
		if len(rows) >= innerH {
			break
		}
	}
	return rows
}
