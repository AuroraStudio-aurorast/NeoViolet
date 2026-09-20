package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func completionModel(t *testing.T, width, height int) *Model {
	t.Helper()
	m := setupModel()
	_, _ = handleResize(m, tea.WindowSizeMsg{Width: width, Height: height})
	m.UI.Mode = ModeCommand
	return m
}

// The overlay has min(candidates, 5, ContentHeight) rows and every row is
// avail wide.
func TestRenderCompletionShape(t *testing.T) {
	m := completionModel(t, 80, 24)
	plan := m.layoutPlan()
	m.completionCandidates = []candidate{
		{Value: "load", Desc: "<path>  Load an audio file"},
		{Value: "lrc", Desc: "<sub>  Lyrics control"},
	}
	got := renderCompletion(m, plan)

	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("rows = %d, want 2", len(lines))
	}
	avail := plan.ContentWidth - 6
	for _, line := range lines {
		if w := lipgloss.Width(line); w != avail {
			t.Errorf("row width = %d, want %d (%q)", w, avail, line)
		}
	}
}

func TestRenderCompletionEmptyWhenNoCandidates(t *testing.T) {
	m := completionModel(t, 80, 24)
	if got := renderCompletion(m, m.layoutPlan()); got != "" {
		t.Errorf("renderCompletion = %q, want empty", got)
	}
}

func TestRenderCompletionNormalModeIsEmpty(t *testing.T) {
	m := completionModel(t, 80, 24)
	m.UI.Mode = ModeNormal
	m.completionCandidates = []candidate{{Value: "lrc"}}
	if got := renderCompletion(m, m.layoutPlan()); got != "" {
		t.Errorf("renderCompletion in normal mode = %q, want empty", got)
	}
}

// Three degradation tiers: 1) value and description fit; 2) only the value
// fits; 3) the value itself is over-wide -> middle ellipsis for paths.
func TestCompletionRowThreeTiers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cand  candidate
		avail int
		want  string
	}{
		{"tier 1 keeps the description", candidate{Value: "lrc", Desc: "<sub>  Lyrics control"}, 40, "  lrc  <sub>  Lyrics control"},
		{"tier 2 drops the description", candidate{Value: "lrc", Desc: "<sub>  Lyrics control"}, 6, "  lrc"},
		{"tier 3 compacts a path", candidate{Value: "/Users/damon233/Music/Albums/VeryLong/name.mp3", Path: true}, 24, "  …/VeryLong/name.mp3"},
		{"tier 3 truncates a plain value", candidate{Value: "some-very-long-command-name"}, 10, "  some-ve…"},
	} {
		got := stripANSI(completionRow(tc.cand, false, tc.avail))
		if strings.TrimRight(got, " ") != tc.want {
			t.Errorf("%s: row = %q, want %q", tc.name, strings.TrimRight(got, " "), tc.want)
		}
		// Every row fills exactly avail cells. A wider row is cut by the canvas,
		// which loses text without an ellipsis, so the width is the real contract.
		if w := lipgloss.Width(got); w != tc.avail {
			t.Errorf("%s: row width = %d, want %d", tc.name, w, tc.avail)
		}
	}

	// The selected row keeps the same width, with a marker instead of padding.
	sel := stripANSI(completionRow(candidate{Value: "lrc", Desc: "<sub>  Lyrics control"}, true, 40))
	if w := lipgloss.Width(sel); w != 40 {
		t.Errorf("selected row width = %d, want 40", w)
	}
	if !strings.Contains(sel, "▸") {
		t.Errorf("selected row = %q, want a selection marker", sel)
	}
}

func TestCompactPath(t *testing.T) {
	for _, tc := range []struct {
		path  string
		avail int
		want  string
	}{
		{"/Users/me/Music/VeryLong/name.mp3", 62, "/Users/me/Music/VeryLong/name.mp3"},
		{"/Users/me/Music/VeryLong/name.mp3", 24, "…/VeryLong/name.mp3"},
		{"/Users/me/Music/VeryLong/name.mp3", 15, "…/name.mp3"},
		{"/a/very-long-file-name-here.mp3", 10, "/a/very-l…"},
	} {
		if got := compactPath(tc.path, tc.avail); got != tc.want {
			t.Errorf("compactPath(%q, %d) = %q, want %q", tc.path, tc.avail, got, tc.want)
		}
	}
}

// The scroll window centres on the selection and clamps to the list.
func TestCompletionWindowFollowsSelection(t *testing.T) {
	m := completionModel(t, 80, 24)
	m.completionCandidates = make([]candidate, 10)
	for i := range m.completionCandidates {
		m.completionCandidates[i] = candidate{Value: string(rune('a' + i))}
	}
	for _, tc := range []struct{ index, want int }{
		{-1, 0}, // no selection: the list starts at the top
		{0, 0},
		{2, 0}, // the selection reaches the centre before the window moves
		{3, 1},
		{4, 2},
		{9, 5}, // clamped to the last full window
	} {
		m.completionIndex = tc.index
		if got := completionWindowStart(m, 5); got != tc.want {
			t.Errorf("window start with selection %d of 10 = %d, want %d", tc.index, got, tc.want)
		}
	}
}

// stripANSI removes SGR sequences so assertions can compare plain text.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
