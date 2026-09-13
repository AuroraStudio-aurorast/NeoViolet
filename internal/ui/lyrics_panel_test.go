package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

// panelModel is a 100x24 model with a 32-column panel, the smallest terminal
// where the panel appears with default settings.
func panelModel(t *testing.T, contextLines int) *Model {
	t.Helper()
	m := lyricFooterModel()
	m.UI.Width, m.UI.Height = 100, 24
	m.Audio.Player = &mockPlayer{}
	m.Config.Lyrics.Panel = config.LyricsPanelConfig{
		Mode:         config.PanelModeAuto,
		Width:        config.DefaultPanelWidth,
		ContextLines: contextLines,
	}
	m.panelMode = config.PanelModeAuto
	return m
}

// panelRowText is the plain text of a panel row.
func panelRowText(r panelRow) string {
	var b strings.Builder
	for _, s := range r.spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

// panelSongs returns a panel model whose lyrics have n single-row lines, with
// the line at index cur playing. Line i reads "line 01", "line 02", ...
func panelSongs(t *testing.T, contextLines, n, cur int) *Model {
	t.Helper()
	m := panelModel(t, contextLines)
	lines := make([]lyrics.LyricLine, n)
	for i := range lines {
		lines[i] = lyrics.LyricLine{
			Time: time.Duration(i) * 5 * time.Second,
			Text: fmt.Sprintf("line %02d", i+1),
		}
	}
	m.Audio.Lyrics = &lyrics.Data{Lines: lines}
	m.Audio.Elapsed = lines[cur].Time
	m.Audio.UpdateLyricIndex()
	return m
}

// panelAnchor is the row the current line group starts on: one third of the
// inner height, i.e. row 4 of the 13-row box the default panel has at 100x24.
func panelAnchor(innerH int) int { return innerH / 3 }

// panelRowWidths asserts the hard contract that no row exceeds the box.
func panelRowWidths(t *testing.T, rows []panelRow, innerW int) {
	t.Helper()
	for i, r := range rows {
		if got := spansWidth(r.spans); got > innerW {
			t.Errorf("row %d spans width = %d, want <= %d", i, got, innerW)
		}
	}
}

func TestRenderLyricsPanel_BoxGeometry(t *testing.T) {
	m := panelModel(t, 2)
	plan := m.layoutPlan()
	if !plan.PanelShown {
		t.Fatalf("panel must be shown at 100x24: %+v", plan)
	}
	if plan.PanelWidth != 32 || plan.PanelInnerW != 28 || plan.PanelInnerH != 13 {
		t.Fatalf("panel geometry = {%d %d %d}, want {32 28 13}",
			plan.PanelWidth, plan.PanelInnerW, plan.PanelInnerH)
	}

	out := renderLyricsPanel(m, plan)
	lines := strings.Split(out, "\n")
	if len(lines) != plan.ContentHeight {
		t.Fatalf("panel rows = %d, want %d", len(lines), plan.ContentHeight)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w != plan.PanelWidth {
			t.Errorf("row %d width = %d, want %d (%q)", i, w, plan.PanelWidth, line)
		}
	}
}

// A wrapped context line above the current one must keep its two halves in
// order: the upward walk prepends whole lines, it does not reverse flat rows.
func TestPanelWindow_WrappedContextLineKeepsRowOrder(t *testing.T) {
	m := panelSongs(t, 0, 40, 20)
	// Make the line directly above the current one wrap to two rows.
	m.Audio.Lyrics.Lines[19].Text = strings.Repeat("字", 30) // 60 cells > 2*28
	m.Audio.Elapsed = m.Audio.Lyrics.Lines[20].Time
	m.Audio.UpdateLyricIndex()

	plan := m.layoutPlan()
	rows := panelWindow(m, plan)

	if got := panelRowText(rows[0]); got != "line 18" {
		t.Errorf("row 0 = %q, want the farthest context line", got)
	}
	if got := panelRowText(rows[1]); got != "line 19" {
		t.Errorf("row 1 = %q, want the line before the wrapped one", got)
	}
	if got := panelRowText(rows[2]); got != strings.Repeat("字", 14) {
		t.Errorf("row 2 = %q, want the first half of the wrapped context line", got)
	}
	if got := panelRowText(rows[3]); got != strings.Repeat("字", 13)+"…" {
		t.Errorf("row 3 = %q, want the second half, ellipsis included", got)
	}
	if got := panelRowText(rows[4]); got != "line 21" {
		t.Errorf("row 4 = %q, want the current line", got)
	}
	panelRowWidths(t, rows, plan.PanelInnerW)
}

// A song that is shorter than the box keeps its anchor instead of being pulled
// to the bottom, and only its tail stays blank.
func TestPanelWindow_ShortSongKeepsItsAnchor(t *testing.T) {
	m := panelSongs(t, 0, 5, 4)
	plan := m.layoutPlan()
	rows := panelWindow(m, plan)

	for i, want := range []string{"line 01", "line 02", "line 03", "line 04"} {
		if got := panelRowText(rows[i]); got != want {
			t.Errorf("row %d = %q, want %q", i, got, want)
		}
	}
	if got := panelRowText(rows[4]); got != "line 05" {
		t.Errorf("row 4 = %q, want the current line on the anchor", got)
	}
	for i := 5; i < plan.PanelInnerH; i++ {
		if got := strings.TrimSpace(panelRowText(rows[i])); got != "" {
			t.Errorf("row %d = %q, want blank", i, got)
		}
	}
}

// With a per-side context cap the window stays deliberately compact: the
// current line keeps its one-third anchor and the remaining rows stay blank.
func TestPanelWindow_ContextCapKeepsCompactWindow(t *testing.T) {
	m := panelSongs(t, 2, 40, 20)
	plan := m.layoutPlan()
	if plan.PanelInnerH != 13 {
		t.Fatalf("inner height = %d, want 13", plan.PanelInnerH)
	}
	anchor := panelAnchor(plan.PanelInnerH)
	rows := panelWindow(m, plan)

	if got := panelRowText(rows[anchor]); got != "line 21" {
		t.Errorf("row %d = %q, want the current line", anchor, got)
	}
	if got := panelRowText(rows[anchor-1]); got != "line 20" {
		t.Errorf("row %d = %q, want one context line above", anchor-1, got)
	}
	if got := panelRowText(rows[anchor-2]); got != "line 19" {
		t.Errorf("row %d = %q, want the second context line above", anchor-2, got)
	}
	if got := panelRowText(rows[anchor-3]); strings.TrimSpace(got) != "" {
		t.Errorf("row %d = %q, want blank (two lines is the cap)", anchor-3, got)
	}
	if got := panelRowText(rows[anchor+1]); got != "line 22" {
		t.Errorf("row %d = %q, want one context line below", anchor+1, got)
	}
	if got := panelRowText(rows[anchor+2]); got != "line 23" {
		t.Errorf("row %d = %q, want the second context line below", anchor+2, got)
	}
	for _, i := range []int{0, anchor - 3, anchor + 3, plan.PanelInnerH - 1} {
		if got := panelRowText(rows[i]); strings.TrimSpace(got) != "" {
			t.Errorf("row %d = %q, want blank (context_lines=2 caps the window)", i, got)
		}
	}
	panelRowWidths(t, rows, plan.PanelInnerW)
}

// context_lines=0 (the default) means "no cap": every row that has a lyric line
// to show is filled. A song with fewer lines than the box has rows keeps a
// stable anchor and blanks only its tail.
func TestPanelWindow_NoCapFillsWhatItHas(t *testing.T) {
	m := panelModel(t, 0)
	plan := m.layoutPlan()
	rows := panelWindow(m, plan)

	if got := panelRowText(rows[0]); got != "我听见雨滴落在青青草地" {
		t.Errorf("row 0 = %q, want the first line", got)
	}
	if got := panelRowText(rows[1]); got != "可是我没有听见你的声音" {
		t.Errorf("row 1 = %q, want the current line", got)
	}
	if got := panelRowText(rows[2]); got != "认真 呼唤我姓名" {
		t.Errorf("row 2 = %q, want the last line", got)
	}
	for i := 3; i < plan.PanelInnerH; i++ {
		if got := strings.TrimSpace(panelRowText(rows[i])); got != "" {
			t.Errorf("row %d = %q, want blank", i, got)
		}
	}
}

// A song longer than the box fills every row: the current line one third down,
// as many lines above as that leaves room for, and the rest below.
func TestPanelWindow_FillsEveryRow(t *testing.T) {
	m := panelSongs(t, 0, 40, 20)
	plan := m.layoutPlan()
	rows := panelWindow(m, plan)
	if anchor := panelAnchor(plan.PanelInnerH); anchor != 4 {
		t.Fatalf("anchor = %d, want 4 for a 13-row box", anchor)
	}

	want := []string{
		"line 17", "line 18", "line 19", "line 20", // above the group
		"line 21", // the current line, on row 4
		"line 22", "line 23", "line 24", "line 25",
		"line 26", "line 27", "line 28", "line 29", // below
	}
	if len(want) != plan.PanelInnerH {
		t.Fatalf("fixture expects %d rows, box has %d", len(want), plan.PanelInnerH)
	}
	for i, w := range want {
		if got := panelRowText(rows[i]); got != w {
			t.Errorf("row %d = %q, want %q", i, got, w)
		}
	}
	panelRowWidths(t, rows, plan.PanelInnerW)
}

// At the start of a song there is nothing above the first line, so the window
// hugs the top and fills downwards instead of leaving a gap.
func TestPanelWindow_StartHugsTop(t *testing.T) {
	m := panelSongs(t, 0, 40, 0)
	plan := m.layoutPlan()
	rows := panelWindow(m, plan)

	if got := panelRowText(rows[0]); got != "line 01" {
		t.Errorf("row 0 = %q, want the current line at the top", got)
	}
	if got := panelRowText(rows[1]); got != "line 02" {
		t.Errorf("row 1 = %q, want the next line", got)
	}
	if got := panelRowText(rows[plan.PanelInnerH-1]); got != fmt.Sprintf("line %02d", plan.PanelInnerH) {
		t.Errorf("last row = %q, want the box filled", got)
	}
	panelRowWidths(t, rows, plan.PanelInnerW)
}

// At the end of a song there is nothing below the last line, so the window
// scrolls up and hugs the bottom rather than leaving two thirds of the box
// blank.
func TestPanelWindow_EndHugsBottom(t *testing.T) {
	m := panelSongs(t, 0, 40, 39)
	plan := m.layoutPlan()
	rows := panelWindow(m, plan)
	last := plan.PanelInnerH - 1

	if got := panelRowText(rows[last]); got != "line 40" {
		t.Errorf("last row = %q, want the current line at the bottom", got)
	}
	if got := panelRowText(rows[last-1]); got != "line 39" {
		t.Errorf("row %d = %q, want the previous line", last-1, got)
	}
	if got := panelRowText(rows[0]); got != fmt.Sprintf("line %02d", 40-plan.PanelInnerH+1) {
		t.Errorf("row 0 = %q, want the box filled from the top", got)
	}
	panelRowWidths(t, rows, plan.PanelInnerW)
}

// A long line wraps to two rows and both belong to the current group, so the
// group moves as a block around the anchor rather than splitting.
func TestPanelWindow_LongLineWrapsToTwoRows(t *testing.T) {
	m := panelModel(t, 2)
	m.Audio.Lyrics = &lyrics.Data{Lines: []lyrics.LyricLine{
		{Time: 0, Text: "我听见雨滴落在青青草地"},
		{Time: 5 * time.Second, Text: strings.Repeat("字", 30)}, // 60 cells > 2*28
	}}
	m.Audio.Elapsed = 5 * time.Second
	m.Audio.UpdateLyricIndex()

	plan := m.layoutPlan()
	rows := panelWindow(m, plan)
	anchor := panelAnchor(plan.PanelInnerH) // a capped window keeps the 1/3 anchor

	if got := panelRowText(rows[anchor]); got != strings.Repeat("字", 14) {
		t.Errorf("row %d = %q, want 14 glyphs", anchor, got)
	}
	// 2 rows of 28 cells cannot hold 60 cells: the second one is truncated and
	// gives up one cell so the ellipsis fits inside the box.
	if got := panelRowText(rows[anchor+1]); got != strings.Repeat("字", 13)+"…" {
		t.Errorf("row %d = %q, want 13 glyphs plus an ellipsis", anchor+1, got)
	}
	for _, i := range []int{anchor, anchor + 1} {
		if w := spansWidth(rows[i].spans); w > plan.PanelInnerW {
			t.Errorf("row %d width = %d, want <= %d", i, w, plan.PanelInnerW)
		}
	}
}

// A gap holds the previous line: the panel has no countdown dots, because the
// upcoming line is already visible as a context row.
func TestPanelWindow_GapHoldsPreviousLine(t *testing.T) {
	m := panelModel(t, 2)
	m.Audio.Lyrics = &lyrics.Data{Lines: []lyrics.LyricLine{
		{Time: 0, End: 5 * time.Second, Text: "first"},
		{Time: 12 * time.Second, Text: "next"},
	}}
	m.Audio.Elapsed = 10 * time.Second
	m.Audio.UpdateLyricIndex()

	plan := m.layoutPlan()
	rows := panelWindow(m, plan)
	anchor := panelAnchor(plan.PanelInnerH) // a capped window keeps the 1/3 anchor

	if got := panelRowText(rows[anchor]); got != "first" {
		t.Errorf("row %d = %q, want the previous line held through the gap", anchor, got)
	}
	if got := panelRowText(rows[anchor+1]); got != "next" {
		t.Errorf("row %d = %q, want the upcoming line", anchor+1, got)
	}
}

// Before the first line starts there is no current line: the window starts at
// the top of the box and nothing is highlighted.
func TestPanelWindow_BeforeFirstLine(t *testing.T) {
	m := panelModel(t, 0)
	m.Audio.Elapsed = 0
	m.Audio.Lyrics = &lyrics.Data{Lines: []lyrics.LyricLine{
		{Time: 5 * time.Second, Text: "first"},
		{Time: 10 * time.Second, Text: "second"},
	}}
	m.Audio.UpdateLyricIndex()

	plan := m.layoutPlan()
	rows := panelWindow(m, plan)
	if got := panelRowText(rows[0]); got != "first" {
		t.Errorf("row 0 = %q, want the first line", got)
	}
	if got := panelRowText(rows[1]); got != "second" {
		t.Errorf("row 1 = %q, want the second line", got)
	}
	if got := strings.TrimSpace(panelRowText(rows[2])); got != "" {
		t.Errorf("row 2 = %q, want blank", got)
	}
}

// Past the last line the last one stays on screen, so the panel does not go
// blank during the outro. A two-line song is shorter than the box, so the
// anchor is capped by the single line above it (see TestPanelWindow_EndHugsBottom
// for the long-song case, where the window scrolls to the bottom instead).
func TestPanelWindow_PastLastLine(t *testing.T) {
	m := panelModel(t, 0)
	m.Audio.Lyrics = &lyrics.Data{Lines: []lyrics.LyricLine{
		{Time: 0, Text: "first"},
		{Time: 5 * time.Second, Text: "last"},
	}}
	m.Audio.Elapsed = 20 * time.Second
	m.Audio.UpdateLyricIndex()

	plan := m.layoutPlan()
	rows := panelWindow(m, plan)
	if got := panelRowText(rows[1]); got != "last" {
		t.Errorf("row 1 = %q, want the last line", got)
	}
	if got := panelRowText(rows[0]); got != "first" {
		t.Errorf("row 0 = %q, want the previous line as context", got)
	}
}

func TestPanelWindow_KaraokeSplit(t *testing.T) {
	m := panelModel(t, 2)
	m.Audio.Lyrics = &lyrics.Data{Lines: []lyrics.LyricLine{{
		Time: 0,
		Text: "可是我没有听见你的声音",
		Words: []lyrics.WordFragment{
			{Time: 0, Text: "可是我没有"},
			{Time: 3 * time.Second, Text: "听见你的声音"},
		},
	}}}
	m.Audio.Elapsed = 2 * time.Second // the 3s word is still unplayed
	m.Audio.UpdateLyricIndex()

	plan := m.layoutPlan()
	rows := panelWindow(m, plan)
	anchor := panelAnchor(plan.PanelInnerH) // a capped window keeps the 1/3 anchor

	if got := panelRowText(rows[anchor]); got != "可是我没有听见你的声音" {
		t.Fatalf("row %d = %q", anchor, got)
	}
	spans := rows[anchor].spans
	if len(spans) != 2 {
		t.Fatalf("got %d spans, want 2 (played/unplayed)", len(spans))
	}
	if spans[0].Text != "可是我没有" || spans[1].Text != "听见你的声音" {
		t.Errorf("spans = %q / %q", spans[0].Text, spans[1].Text)
	}
	// lipgloss emits ANSI even under `go test`, so different styles must produce
	// different bytes. Comparing renders avoids depending on private style state.
	if spans[0].Style.Render("x") == spans[1].Style.Render("x") {
		t.Errorf("played and unplayed spans render identically: %q", spans[0].Style.Render("x"))
	}
}

// Word timings that do not tile the text fall back to a whole-line highlight
// instead of dropping or duplicating characters.
func TestPanelWindow_KaraokeFallsBackWhenWordsDoNotTile(t *testing.T) {
	m := panelModel(t, 2)
	m.Audio.Lyrics = &lyrics.Data{Lines: []lyrics.LyricLine{{
		Time:  0,
		Text:  "original text",
		Words: []lyrics.WordFragment{{Time: 0, Text: "unrelated"}},
	}}}
	m.Audio.Elapsed = time.Second
	m.Audio.UpdateLyricIndex()

	plan := m.layoutPlan()
	rows := panelWindow(m, plan)
	anchor := panelAnchor(plan.PanelInnerH) // a capped window keeps the 1/3 anchor
	if len(rows[anchor].spans) != 1 || panelRowText(rows[anchor]) != "original text" {
		t.Errorf("row = %+v, want a single whole-line span", rows[anchor].spans)
	}
}

func TestPanelPlaceholderText(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Model)
		want   func(*Model) string
		ok     bool
	}{
		{"no player", func(m *Model) { m.Audio.Player = nil },
			func(m *Model) string { return m.Icons.Music + " No track" }, true},
		{"fetching", func(m *Model) {
			m.Audio.Lyrics = nil
			m.Audio.ActiveLyricLines = nil
			m.LyricsFetching = true
		}, func(*Model) string { return "[Fetching lyrics...]" }, true},
		{"no lyrics", func(m *Model) {
			m.Audio.Lyrics = nil
			m.Audio.ActiveLyricLines = nil
		}, func(m *Model) string { return m.Icons.Music + " No lyrics" }, true},
		{"all lines blank", func(m *Model) {
			m.Audio.Lyrics = &lyrics.Data{Lines: []lyrics.LyricLine{{Time: 0, Text: " "}}}
		}, func(m *Model) string { return m.Icons.Music + " No lyrics" }, true},
		{"real lyrics", func(*Model) {}, func(*Model) string { return "" }, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := panelModel(t, 2)
			tc.mutate(m)
			got, ok := panelPlaceholderText(m)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if want := tc.want(m); got != want {
				t.Errorf("placeholder = %q, want %q", got, want)
			}
		})
	}
}

func TestRenderLyricsPanel_PlaceholderIsCentred(t *testing.T) {
	m := panelModel(t, 2)
	m.Audio.Lyrics = nil
	m.Audio.ActiveLyricLines = nil

	plan := m.layoutPlan()
	out := renderLyricsPanel(m, plan)
	lines := strings.Split(out, "\n")

	mid := plan.ContentHeight / 2
	msg := m.Icons.Music + " No lyrics"
	want := strings.Repeat(" ", (plan.PanelInnerW-lipgloss.Width(msg))/2) + msg
	if !strings.Contains(lines[mid], want) {
		t.Errorf("row %d = %q, want a centred %q", mid, lines[mid], want)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w != plan.PanelWidth {
			t.Errorf("row %d width = %d, want %d", i, w, plan.PanelWidth)
		}
	}
}

// A tiny panel must not panic and must still respect its inner width.
func TestRenderLyricsPanel_ExtremeSizes(t *testing.T) {
	for _, innerH := range []int{1, 2} {
		for _, innerW := range []int{1, 4} {
			m := panelModel(t, 2)
			plan := layoutPlan{
				Width: 100, Height: 24, PanelShown: true,
				PanelWidth: innerW + 4, PanelInnerW: innerW, PanelInnerH: innerH,
				ContentWidth: 100 - innerW - 4, ContentHeight: innerH + 2,
				FooterRows: footerBaseRows, LyricMode: lyricModePanel,
			}
			out := renderLyricsPanel(m, plan)
			for i, line := range strings.Split(out, "\n") {
				if w := lipgloss.Width(line); w != plan.PanelWidth {
					t.Errorf("innerW=%d innerH=%d row %d width = %d, want %d",
						innerW, innerH, i, w, plan.PanelWidth)
				}
			}
		}
	}
}
