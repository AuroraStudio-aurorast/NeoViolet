package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// spansText is the plain text of a span row.
func spansText(spans []styledSpan) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

// TestWrapSpans_NeverExceedsWidth is the property that keeps the panel from
// tearing: no matter the input, no row may be wider than the box's inner width.
func TestWrapSpans_NeverExceedsWidth(t *testing.T) {
	texts := []string{
		"",
		"abc",
		"我听见雨滴落在青青草地",
		"a我b●",
		"🎵🎵🎵",
		strings.Repeat("字", 30),
		"  ",
		"认真 呼唤我姓名",
	}
	widths := []int{-1, 0, 1, 2, 3, 7, 20, 28, 60}
	rowCaps := []int{-1, 0, 1, 2, 3}

	for _, width := range widths {
		for _, maxRows := range rowCaps {
			for _, text := range texts {
				spans := []styledSpan{{Text: text, Style: lipgloss.NewStyle()}}
				got := wrapSpans(spans, width, maxRows)

				if width <= 0 || maxRows <= 0 {
					if got != nil {
						t.Errorf("width=%d maxRows=%d: got %+v, want nil", width, maxRows, got)
					}
					continue
				}
				if len(got) > maxRows {
					t.Errorf("width=%d maxRows=%d text=%q: %d rows", width, maxRows, text, len(got))
				}
				for i, row := range got {
					if w := spansWidth(row); w > width {
						t.Errorf("width=%d maxRows=%d text=%q: row %d is %d cells: %q",
							width, maxRows, text, i, w, spansText(row))
					}
				}
			}
		}
	}
}

// Karaoke lines arrive as several spans; wrapping must respect span boundaries
// and keep the styles attached to their text.
func TestWrapSpans_MultipleSpansKeepStyles(t *testing.T) {
	played := lipgloss.NewStyle().Bold(true)
	rest := lipgloss.NewStyle().Italic(true)
	spans := []styledSpan{
		{Text: "可是我没有", Style: played}, // 10 cells
		{Text: "听见你的声音", Style: rest},  // 12 cells
	}

	got := wrapSpans(spans, 12, 2)
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	if text := spansText(got[0]); text != "可是我没有听" {
		t.Errorf("row 0 = %q, want %q", text, "可是我没有听")
	}
	if text := spansText(got[1]); text != "见你的声音" {
		t.Errorf("row 1 = %q, want %q", text, "见你的声音")
	}
	for i, row := range got {
		if w := spansWidth(row); w > 12 {
			t.Errorf("row %d width = %d, want <= 12", i, w)
		}
	}
}

func TestWrapSpans_TruncatesWithEllipsis(t *testing.T) {
	style := lipgloss.NewStyle()

	if got := wrapSpans([]styledSpan{{Text: "abcdef", Style: style}}, 3, 1); spansText(got[len(got)-1]) != "ab…" {
		t.Errorf("single-row truncation = %q, want %q", spansText(got[len(got)-1]), "ab…")
	}

	// 30 CJK glyphs = 60 cells; two rows of 28 cells cannot hold them all.
	got := wrapSpans([]styledSpan{{Text: strings.Repeat("字", 30), Style: style}}, 28, 2)
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	if text := spansText(got[1]); !strings.HasSuffix(text, "…") {
		t.Errorf("last row %q must end with an ellipsis", text)
	}
	if w := spansWidth(got[1]); w > 28 {
		t.Errorf("last row width = %d, want <= 28", w)
	}
}

// The last row must never be full-and-therefore-unmodified when there was
// leftover text: the ellipsis itself has to fit inside the width.
func TestWrapSpans_EllipsisFitsExactly(t *testing.T) {
	// 40 cells cannot fit in two rows of 10: the last row gives up one cell so
	// the ellipsis itself still fits inside the box.
	got := wrapSpans([]styledSpan{{Text: strings.Repeat("x", 40), Style: lipgloss.NewStyle()}}, 10, 2)
	last := got[len(got)-1]
	if w := spansWidth(last); w != 10 {
		t.Errorf("last row width = %d, want 10 (%q)", w, spansText(last))
	}
	if text := spansText(last); text != "xxxxxxxxx…" {
		t.Errorf("last row = %q, want %q", text, "xxxxxxxxx…")
	}
}

// Truncation assumes one dropped rune frees one cell, but a zero-width rune
// (combining mark, ZWJ, or a \n that slipped into a line) frees nothing.
// Correcting that must not cost a cell of the contract: the ellipsis has to
// keep fitting inside width.
func TestWrapSpans_ZeroWidthTailKeepsEllipsisInside(t *testing.T) {
	// "á" is a + U+0301: one cell of text, two runes, the last one zero cells.
	// A ZWJ and a \n that slipped into a line are zero-width too, so all three
	// shapes have to keep the ellipsis inside the width.
	texts := []string{"a\u0301b\u0301c", "a\u200db", "a\nb"}
	for _, width := range []int{1, 2, 3} {
		for _, text := range texts {
			got := wrapSpans([]styledSpan{{Text: text, Style: lipgloss.NewStyle()}}, width, 1)
			last := got[len(got)-1]
			if w := spansWidth(last); w > width {
				t.Errorf("width=%d text=%q: last row is %d cells: %q", width, text, w, spansText(last))
			}
		}
	}
}

// A CJK glyph is 2 cells: in a 1-column panel it simply cannot be drawn, and
// dropping it is the only way to keep the width contract.
func TestWrapSpans_DropsUnrepresentableRunes(t *testing.T) {
	got := wrapSpans([]styledSpan{{Text: "字", Style: lipgloss.NewStyle()}}, 1, 2)
	for i, row := range got {
		if w := spansWidth(row); w > 1 {
			t.Errorf("row %d width = %d, want <= 1", i, w)
		}
	}
}
