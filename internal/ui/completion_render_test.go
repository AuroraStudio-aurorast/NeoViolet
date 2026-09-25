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

// The overlay has min(candidates, 5, FooterRows) rows and every row is
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
	avail := plan.Width
	for _, line := range lines {
		if w := lipgloss.Width(line); w != avail {
			t.Errorf("row width = %d, want %d (%q)", w, avail, line)
		}
	}
}

// The overlay never grows past five rows, however many candidates there are.
func TestRenderCompletionCapsRowsAtFive(t *testing.T) {
	m := completionModel(t, 80, 24)
	plan := m.layoutPlan()
	m.completionCandidates = make([]candidate, 8)
	for i := range m.completionCandidates {
		m.completionCandidates[i] = candidate{Value: string(rune('a' + i))}
	}

	want := 5
	if got := completionRows(m, plan); got != want {
		t.Fatalf("completionRows with 8 candidates = %d, want %d", got, want)
	}
	if lines := strings.Split(renderCompletion(m, plan), "\n"); len(lines) != want {
		t.Errorf("rendered rows = %d, want %d", len(lines), want)
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
		// Every row fills exactly avail cells, because the tier checks pad it to
		// avail. The width is therefore the real contract: an over-wide row would
		// widen the composited block (the compositor sizes its canvas from the
		// union of the layer bounds) instead of being clipped, which is worse than
		// losing text with an ellipsis.
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

// A single segment of width avail-1 leaves no room for the "…/" prefix, so the
// path must fall back to truncation instead of overflowing by a cell.
func TestCompactPathNeverExceedsAvail(t *testing.T) {
	for _, avail := range []int{12, 24, 62} {
		for _, delta := range []int{0, 1, 2, 3} {
			path := "/dir/" + strings.Repeat("x", avail-delta)
			if got := compactPath(path, avail); lipgloss.Width(got) > avail {
				t.Errorf("compactPath(%q, %d) = %q, width %d exceeds avail", path, avail, got, lipgloss.Width(got))
			}
		}
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

// A Windows path splits on its own separators, so a long one still loses its
// left-hand directories and keeps the file name instead of being read as a
// single unbreakable name and truncated from the right.
func TestCompactPathWindows(t *testing.T) {
	windowsSeparators(t)
	const path = `C:\Users\me\Music\VeryLong\name.mp3`
	for _, tc := range []struct {
		avail int
		want  string
	}{
		{62, path},
		{24, `…\VeryLong\name.mp3`},
		{15, `…\name.mp3`},
	} {
		if got := compactPath(path, tc.avail); got != tc.want {
			t.Errorf("compactPath(%q, %d) = %q, want %q", path, tc.avail, got, tc.want)
		}
	}
	// Below the middle-ellipsis width the row falls back to truncation, which
	// keeps the drive and the start of the name.
	if got := compactPath(`C:\a\very-long-file-name-here.mp3`, 10); got != `C:\a\very…` {
		t.Errorf("compactPath narrow = %q, want %q", got, `C:\a\very…`)
	}
}

// A pathological single file name must still come back as exactly avail cells:
// nothing downstream clips an over-wide row (the compositor widens its canvas
// instead), so compactPath and truncateLine have to land inside the budget on
// their own.
func TestCompletionRowWidthIsExactlyAvail(t *testing.T) {
	m := completionModel(t, 80, 24)
	plan := m.layoutPlan()
	avail := plan.Width
	cand := candidate{Value: "/dir/" + strings.Repeat("x", avail-markerWidth-1), Path: true}
	for _, selected := range []bool{false, true} {
		if got := lipgloss.Width(completionRow(cand, selected, avail)); got != avail {
			t.Errorf("selected=%v: row width = %d, want exactly %d", selected, got, avail)
		}
	}
}

// A list that already fits needs no scrolling, and no candidates means no
// candidate block at all.
func TestCompletionWindowAndEmptyRender(t *testing.T) {
	m := completionModel(t, 80, 24)
	plan := m.layoutPlan()
	m.completionCandidates = []candidate{{Value: "lrc"}, {Value: "load"}}
	// A list that fits never scrolls: both the exact candidate count and a row
	// budget larger than the list must leave the window at the top (without the
	// guard the second shape would clamp to a negative start).
	for _, rows := range []int{2, 5} {
		for _, index := range []int{-1, 0, 1} {
			m.completionIndex = index
			if got := completionWindowStart(m, rows); got != 0 {
				t.Errorf("window start with %d candidates, %d rows and selection %d = %d, want 0",
					len(m.completionCandidates), rows, index, got)
			}
		}
	}

	m.completionCandidates = nil
	m.completionIndex = -1
	if got := completionRows(m, plan); got != 0 {
		t.Errorf("completionRows with no candidates = %d, want 0", got)
	}
	if got := renderCompletion(m, plan); got != "" {
		t.Errorf("renderCompletion with no candidates = %q, want empty", got)
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

// The budget for a value and its description is the row minus the selection
// marker, and the two-cell gap between them is part of it. A row may keep its
// description only while all of that still fits in avail.
func TestCompletionRowBudgetsTheGapAndMarker(t *testing.T) {
	// Exactly at the boundary: marker (2) + value (32) + gap (2) + description
	// (4) == avail, so the description still fits.
	const avail = 40
	row := stripANSI(completionRow(candidate{Value: strings.Repeat("v", 32), Desc: "desc"}, false, avail))
	if w := lipgloss.Width(row); w != avail {
		t.Errorf("boundary row width = %d, want %d", w, avail)
	}
	if got := strings.TrimRight(row, " "); !strings.HasSuffix(got, "desc") {
		t.Errorf("boundary row = %q, want it to keep the description", got)
	}

	// The description keeps its own style: with pad == 0 its span is exactly
	// completionDescStyle.Render(<desc>), so this fails if the row style is used
	// for it instead. This render is deliberately left un-stripped of its SGR
	// sequences, unlike the one above.
	styled := completionRow(candidate{Value: strings.Repeat("v", 32), Desc: "desc"}, false, avail)
	if !strings.Contains(styled, completionDescStyle.Render("desc")) {
		t.Errorf("boundary row = %q, want the description in its own style", styled)
	}

	// Two cells too wide once the marker and the gap count: the description has
	// to go so that the row stays inside avail.
	const narrow = 10
	tight := stripANSI(completionRow(candidate{Value: "lrcde", Desc: "Lyr"}, false, narrow))
	if w := lipgloss.Width(tight); w != narrow {
		t.Errorf("tight row width = %d, want %d", w, narrow)
	}
	if strings.Contains(tight, "Lyr") {
		t.Errorf("tight row = %q, want the description dropped", tight)
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

// The overlay only touches the footer band: the frame size is unchanged, the
// content box and the lyrics panel keep every row, and the command line keeps
// its text and its row.
func TestOverlayKeepsFrameSizeAndOtherRegions(t *testing.T) {
	m := completionModel(t, 80, 24)
	plan := m.layoutPlan()

	before := renderMainView(m).Content

	m.completionCandidates = []candidate{
		{Value: "load", Desc: "<path>  Load an audio file"},
		{Value: "lrc", Desc: "<sub>  Lyrics control"},
	}
	after := renderMainView(m).Content

	if got := lipgloss.Height(after); got != m.UI.Height {
		t.Errorf("frame height = %d, want %d", got, m.UI.Height)
	}
	if got := lipgloss.Width(after); got != m.UI.Width {
		t.Errorf("frame width = %d, want %d", got, m.UI.Width)
	}
	if !strings.Contains(after, "Load an audio file") {
		t.Error("the overlay is missing from the frame")
	}
	// The content block is never covered at all now, so its own text has to be
	// there byte for byte — the loop below is what checks that.
	if !strings.Contains(after, "[ Home ]") {
		t.Errorf("the content block's own text is gone from the overlaid frame: %q", after)
	}
	if before == after {
		t.Error("the frame did not change when candidates appeared")
	}
	// The overlay consumes no rows, so the frame keeps its line count.
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	if len(beforeLines) != len(afterLines) {
		t.Fatalf("frame line count changed: %d -> %d", len(beforeLines), len(afterLines))
	}
	// Everything outside the content block (header above it, footer and command
	// line below it) comes through byte for byte: the overlay only ever draws on
	// the content block, so nothing else may change, not even its styling. Both
	// frames go through the same layout pass at the same width, so comparing them
	// as they are is exact.
	// Everything outside the footer band — the whole content box included —
	// comes through byte for byte, styling and all. That is what the position
	// buys, so it is worth pinning as a comparison rather than by geometry.
	bandTop := plan.Height - plan.FooterRows
	for row := range afterLines {
		if row >= bandTop && row < plan.Height-helpHeight {
			continue // the footer band is what the overlay draws on
		}
		if afterLines[row] != beforeLines[row] {
			t.Errorf("row %d outside the footer band changed:\n got %q\nwant %q", row, afterLines[row], beforeLines[row])
		}
	}
}

// The list sits in the bottom rows of the footer band, directly above the
// command line, and covers nothing else: the command line keeps its own row and
// the content box keeps every row it had, its bottom border included.
func TestOverlaySitsInTheFooterBand(t *testing.T) {
	m := completionModel(t, 80, 24)
	plan := m.layoutPlan()
	before := renderMainView(m).Content

	m.completionCandidates = []candidate{{Value: "lrc", Desc: "<sub>  Lyrics control"}}
	after := renderMainView(m).Content

	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	if len(beforeLines) != len(afterLines) {
		t.Fatalf("frame line count changed: %d -> %d", len(beforeLines), len(afterLines))
	}

	// One candidate, one row, and that row is the last row above the command
	// line. Any other row count would make the anchor meaningless.
	overlayRow := -1
	for row := range afterLines {
		if strings.Contains(stripANSI(afterLines[row]), "Lyrics control") {
			if overlayRow >= 0 {
				t.Fatalf("the overlay spans more than one row: %d and %d", overlayRow, row)
			}
			overlayRow = row
		}
	}
	if want := plan.Height - helpHeight - 1; overlayRow != want {
		t.Fatalf("overlay row = %d, want %d", overlayRow, want)
	}

	// Everything the list does not cover is byte for byte what it was: the
	// content box keeps all of its rows, borders included, and the command line
	// keeps its own.
	for row := tabsHeight; row < tabsHeight+plan.ContentHeight; row++ {
		if afterLines[row] != beforeLines[row] {
			t.Errorf("content row %d changed with candidates:\n got %q\nwant %q",
				row, afterLines[row], beforeLines[row])
		}
	}
	bottomBorder := strings.TrimRight(stripANSI(beforeLines[tabsHeight+plan.ContentHeight-1]), " ")
	if !strings.HasPrefix(bottomBorder, "╰") || !strings.HasSuffix(bottomBorder, "╯") {
		t.Fatalf("the content box bottom border is not where this test looks: %q", bottomBorder)
	}
	if afterLines[plan.Height-1] != beforeLines[plan.Height-1] {
		t.Errorf("command row changed with candidates:\n got %q\nwant %q",
			afterLines[plan.Height-1], beforeLines[plan.Height-1])
	}
}

// The minimum legal terminal still shows the whole candidate list: the frame
// keeps its size, the content height does not squeeze the list, and the band
// stays inside the content box (62 of the 68 columns).
func TestOverlayFitsAtMinimumTerminalSize(t *testing.T) {
	m := completionModel(t, 68, 17)
	plan := m.layoutPlan()
	if plan.ContentWidth != 68 || plan.ContentHeight != 8 {
		t.Fatalf("content block = %dx%d, want 68x8", plan.ContentWidth, plan.ContentHeight)
	}

	before := renderMainView(m).Content

	// A passive list: the user has typed the subcommand name and has not moved
	// the selection yet, so the candidates come from the command table instead of
	// from a list written out here. setCommand leaves the cursor at the end of a
	// fresh input, which is the position the list is computed from.
	setCommand(m, "lrc ")
	syncCompletion(m)
	if m.completionIndex != -1 {
		t.Fatalf("completionIndex = %d, want -1: a list nobody selected from must not pre-select", m.completionIndex)
	}
	after := renderMainView(m).Content

	if got := lipgloss.Height(after); got != 17 {
		t.Errorf("frame height = %d, want 17", got)
	}
	if got := lipgloss.Width(after); got != 68 {
		t.Errorf("frame width = %d, want 68", got)
	}
	rows := completionRows(m, plan)
	if rows != 5 {
		t.Fatalf("completionRows = %d, want 5: the footer band must not squeeze the list", rows)
	}
	if start := completionWindowStart(m, rows); start != 0 {
		t.Fatalf("completion window starts at %d, want 0: nothing is selected, so the list shows its head", start)
	}
	first := m.completionCandidates[0].Desc
	last := m.completionCandidates[rows-1].Desc
	avail := plan.Width
	for _, line := range strings.Split(renderCompletion(m, plan), "\n") {
		if w := lipgloss.Width(line); w != avail {
			t.Errorf("overlay row width = %d, want exactly %d", w, avail)
		}
	}

	// The overlay consumes no rows and sits on the block's last rows, above the
	// bottom border: pin both ends of the band.
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	if len(beforeLines) != len(afterLines) {
		t.Fatalf("frame line count changed: %d -> %d", len(beforeLines), len(afterLines))
	}
	// The anchors come from the candidates themselves, so the test cannot go
	// stale when a description changes, and each must identify exactly one row:
	// a duplicate would make the row-number assertions meaningless.
	rowWith := func(needle string) int {
		found := -1
		for row := 0; row < plan.Height; row++ {
			if strings.Contains(stripANSI(afterLines[row]), needle) {
				if found >= 0 {
					t.Fatalf("the candidate text %q appears in more than one row", needle)
				}
				found = row
			}
		}
		if found < 0 {
			t.Fatalf("the candidate text %q is not in the band", needle)
		}
		return found
	}
	if got, want := rowWith(first), plan.Height-helpHeight-rows; got != want {
		t.Errorf("first candidate row = %d, want %d", got, want)
	}
	if got, want := rowWith(last), plan.Height-helpHeight-1; got != want {
		t.Errorf("last candidate row = %d, want %d", got, want)
	}
}

// tailCells returns the part of a rendered line that starts at the given cell
// offset. Slicing the raw string would cut escape sequences and mis-count wide
// characters, so this walks the runes and measures them with lipgloss, dropping
// every rune that still lies before the offset.
func tailCells(line string, cells int) string {
	var (
		out   []rune
		width int
	)
	for _, r := range stripANSI(line) {
		if width >= cells {
			out = append(out, r)
			continue
		}
		width += lipgloss.Width(string(r))
	}
	return string(out)
}

// The list never leaves the footer band, so the lyrics panel beside the
// content block comes through untouched: the 100x24 frame keeps its size and
// the panel's region is byte-for-byte identical with and without a list. The
// comparison is per row because a squeezed or overpainted panel shows up as one
// changed row.
func TestOverlayLeavesTheLyricsPanelUntouched(t *testing.T) {
	m := panelModel(t, 2)
	beforePlan := m.layoutPlan()
	if !beforePlan.PanelShown {
		t.Fatal("the lyrics panel is not shown: the test would compare nothing")
	}
	if m.UI.Width != 100 || m.UI.Height != 24 {
		t.Fatalf("model size = %dx%d, want 100x24", m.UI.Width, m.UI.Height)
	}
	// Command mode on both sides, so the only difference is the candidate list.
	m.UI.Mode = ModeCommand
	if rows := completionRows(m, beforePlan); rows != 0 {
		t.Fatalf("completionRows = %d before typing, want 0", rows)
	}
	before := renderMainView(m).Content

	setCommand(m, "lrc ")
	syncCompletion(m)
	afterPlan := m.layoutPlan()
	after := renderMainView(m).Content

	if rows := completionRows(m, afterPlan); rows == 0 {
		t.Fatal("no candidates: the overlay is missing from the frame")
	}
	anchor := m.completionCandidates[0].Desc
	if !strings.Contains(stripANSI(after), anchor) {
		t.Fatalf("the candidate text %q is not in the frame", anchor)
	}

	// The list spans the frame, not the content block beside the panel: a narrower
	// band would leave the right-hand columns of the footer band showing.
	for _, line := range strings.Split(renderCompletion(m, afterPlan), "\n") {
		if got := lipgloss.Width(line); got != afterPlan.Width {
			t.Errorf("candidate row width = %d, want the frame width %d", got, afterPlan.Width)
		}
	}

	if got := lipgloss.Height(after); got != 24 {
		t.Errorf("frame height = %d, want 24", got)
	}
	if got := lipgloss.Width(after); got != 100 {
		t.Errorf("frame width = %d, want 100", got)
	}
	if got, want := afterPlan.ContentHeight, beforePlan.ContentHeight; got != want {
		t.Errorf("content height = %d, want %d: a list must not take rows from the block", got, want)
	}
	if got, want := afterPlan.PanelInnerH, beforePlan.PanelInnerH; got != want {
		t.Errorf("panel inner height = %d, want %d", got, want)
	}

	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	if len(beforeLines) != len(afterLines) {
		t.Fatalf("frame line count changed: %d -> %d", len(beforeLines), len(afterLines))
	}
	panel := make([]string, 0, beforePlan.ContentHeight)
	for row := tabsHeight; row < tabsHeight+beforePlan.ContentHeight; row++ {
		want := tailCells(beforeLines[row], beforePlan.ContentWidth)
		if got := tailCells(afterLines[row], beforePlan.ContentWidth); got != want {
			t.Errorf("panel row %d changed with candidates:\n got %q\nwant %q", row, got, want)
		}
		panel = append(panel, want)
	}
	// The region has to carry the panel, otherwise both sides could be blank and
	// the row comparison above would pass without comparing anything.
	if rendered := strings.Join(panel, "\n"); !strings.Contains(rendered, "可是我没有听见你的声音") {
		t.Fatal("the panel region holds no lyric line: the comparison proves nothing")
	}
}
