package ui

import (
	"fmt"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

func input(mode string, showLyrics, oneLineVisible bool) layoutInput {
	return layoutInput{
		PanelMode:      mode,
		PanelWidth:     config.DefaultPanelWidth,
		ShowLyrics:     showLyrics,
		OneLineVisible: oneLineVisible,
	}
}

// TestComputeLayout_Invariants is the layout contract: whichever combination of
// size, mode and lyric state comes in, the regions must tile the terminal
// exactly once — no gap, no overlap.
func TestComputeLayout_Invariants(t *testing.T) {
	widths := []int{67, 68, 80, 90, 99, 100, 140, 200}
	heights := []int{16, 17, 18, 24, 40}
	modes := []string{config.PanelModeAuto, config.PanelModeOn, config.PanelModeOff}
	states := []struct {
		name       string
		showLyrics bool
		oneLine    bool
	}{
		{"lyrics visible", true, true},
		{"no lyrics row", true, false},
		{"lyrics off but fetching", false, true},
		{"nothing", false, false},
	}

	for _, w := range widths {
		for _, h := range heights {
			for _, mode := range modes {
				for _, st := range states {
					t.Run(fmt.Sprintf("%dx%d/%s/%s", w, h, mode, st.name), func(t *testing.T) {
						p := computeLayout(w, h, input(mode, st.showLyrics, st.oneLine))

						if got := p.ContentWidth + p.PanelWidth; got != w {
							t.Errorf("ContentWidth+PanelWidth = %d, want %d", got, w)
						}
						if got := tabsHeight + p.ContentHeight + p.FooterRows + helpHeight; got != h {
							t.Errorf("tabs+content+footer+help = %d, want %d", got, h)
						}
						if p.PanelShown {
							if p.PanelInnerW != p.PanelWidth-4 {
								t.Errorf("PanelInnerW = %d, want %d", p.PanelInnerW, p.PanelWidth-4)
							}
							if p.PanelInnerH != p.ContentHeight-2 {
								t.Errorf("PanelInnerH = %d, want %d", p.PanelInnerH, p.ContentHeight-2)
							}
							if p.LyricMode != lyricModePanel {
								t.Errorf("LyricMode = %d, want panel", p.LyricMode)
							}
							if p.OneLineLyricWidth != 0 {
								t.Errorf("OneLineLyricWidth = %d, want 0 while the panel is shown", p.OneLineLyricWidth)
							}
						} else {
							if p.PanelWidth != 0 || p.PanelInnerW != 0 || p.PanelInnerH != 0 {
								t.Errorf("hidden panel must have zero size: %+v", p)
							}
							if p.LyricMode != lyricModeOneLine {
								t.Errorf("LyricMode = %d, want one_line", p.LyricMode)
							}
							if p.OneLineLyricWidth != w-6 {
								t.Errorf("OneLineLyricWidth = %d, want %d", p.OneLineLyricWidth, w-6)
							}
						}
					})
				}
			}
		}
	}
}

// TestComputeLayout_MatchesToday pins the numbers the pre-panel layout produced,
// so the refactor cannot silently shift the content area by a row.
func TestComputeLayout_MatchesToday(t *testing.T) {
	cases := []struct {
		name        string
		w, h        int
		oneLine     bool
		wantContent int
		wantFooter  int
	}{
		{"80x24 with a lyric row", 80, 24, true, 14, 6},
		{"80x24 without lyrics", 80, 24, false, 15, 5},
		{"68x17 with a lyric row", 68, 17, true, 7, 6},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := computeLayout(tc.w, tc.h, input(config.PanelModeAuto, true, tc.oneLine))
			if p.PanelShown {
				t.Fatalf("panel must not appear at %dx%d with the default width", tc.w, tc.h)
			}
			if p.ContentHeight != tc.wantContent {
				t.Errorf("ContentHeight = %d, want %d", p.ContentHeight, tc.wantContent)
			}
			if p.FooterRows != tc.wantFooter {
				t.Errorf("FooterRows = %d, want %d", p.FooterRows, tc.wantFooter)
			}
		})
	}
}

func TestComputeLayout_Breakpoint(t *testing.T) {
	cases := []struct {
		panelWidth int
		width      int
		wantShown  bool
	}{
		{32, 99, false},
		{32, 100, true},
		{24, 91, false},
		{24, 92, true},
		{48, 115, false},
		{48, 116, true},
	}

	for _, tc := range cases {
		in := input(config.PanelModeAuto, true, true)
		in.PanelWidth = tc.panelWidth
		p := computeLayout(tc.width, 24, in)
		if p.PanelShown != tc.wantShown {
			t.Errorf("panelWidth=%d width=%d: PanelShown = %v, want %v", tc.panelWidth, tc.width, p.PanelShown, tc.wantShown)
		}
	}
}

func TestComputeLayout_ForcedPanelShrinks(t *testing.T) {
	cases := []struct {
		width       int
		wantPanel   int
		wantContent int
	}{
		{200, 32, 168},
		{100, 32, 68},
		{90, 24, 66},
		{68, 24, 44},
	}

	for _, tc := range cases {
		p := computeLayout(tc.width, 24, input(config.PanelModeOn, true, true))
		if !p.PanelShown {
			t.Fatalf("width=%d: mode=on must show the panel", tc.width)
		}
		if p.PanelWidth != tc.wantPanel {
			t.Errorf("width=%d: PanelWidth = %d, want %d", tc.width, p.PanelWidth, tc.wantPanel)
		}
		if p.ContentWidth != tc.wantContent {
			t.Errorf("width=%d: ContentWidth = %d, want %d", tc.width, p.ContentWidth, tc.wantContent)
		}
	}
}

func TestComputeLayout_OffNeverShows(t *testing.T) {
	p := computeLayout(200, 40, input(config.PanelModeOff, true, true))
	if p.PanelShown {
		t.Errorf("mode=off must never show the panel: %+v", p)
	}
}

// :lrc off gives the panel's columns back to the content area (design D2).
func TestComputeLayout_LyricsOffGivesColumnsBack(t *testing.T) {
	p := computeLayout(140, 36, input(config.PanelModeAuto, false, false))
	if p.PanelShown {
		t.Fatal("panel must give way when ShowLyrics is false")
	}
	if p.ContentWidth != 140 {
		t.Errorf("ContentWidth = %d, want 140", p.ContentWidth)
	}
	if p.FooterRows != footerBaseRows {
		t.Errorf("FooterRows = %d, want %d", p.FooterRows, footerBaseRows)
	}
}

func TestComputeLayout_Defensive(t *testing.T) {
	for _, tc := range []struct{ w, h int }{{0, 24}, {24, 0}, {-1, -1}, {80, -5}} {
		p := computeLayout(tc.w, tc.h, input(config.PanelModeAuto, true, true))
		if p.Width != 0 || p.Height != 0 {
			t.Errorf("%dx%d: want zero plan, got %+v", tc.w, tc.h, p)
		}
	}

	// Unknown mode behaves as auto; a zero/negative width falls back to the default.
	in := layoutInput{PanelMode: "bogus", PanelWidth: 0, ShowLyrics: true, OneLineVisible: true}
	p := computeLayout(100, 24, in)
	if !p.PanelShown || p.PanelWidth != config.DefaultPanelWidth {
		t.Errorf("bogus mode / zero width: %+v", p)
	}
}

// layoutPlan is the single adapter from Model state onto layoutInput.
func TestLayoutPlan_MapsModelState(t *testing.T) {
	m := setupModel()
	m.UI.Width, m.UI.Height = 100, 24
	m.Config.Lyrics.Panel.Width = config.DefaultPanelWidth
	m.Audio.ShowLyrics = true

	// A sparse test config leaves panelMode empty; it must behave as auto.
	m.panelMode = ""
	if plan := m.layoutPlan(); !plan.PanelShown {
		t.Errorf("empty panelMode must behave as auto at 100x24: %+v", plan)
	}

	m.panelMode = config.PanelModeOff
	if plan := m.layoutPlan(); plan.PanelShown {
		t.Errorf("mode=off must hide the panel: %+v", plan)
	}
}

// oneLineVisible reproduces the condition that used to decide whether the
// footer keeps its lyric row; all eight combinations must match the old
// renderFooter logic exactly.
func TestOneLineVisible_TruthTable(t *testing.T) {
	cases := []struct {
		hasLyrics bool
		show      bool
		fetching  bool
		want      bool
	}{
		{true, true, false, true},
		{true, true, true, true},
		{true, false, false, false},
		{true, false, true, true},
		{false, true, false, false},
		{false, true, true, true},
		{false, false, false, false},
		{false, false, true, true},
	}

	for _, tc := range cases {
		m := setupModel()
		if tc.hasLyrics {
			m.Audio.Lyrics = &lyrics.Data{}
		}
		m.Audio.ShowLyrics = tc.show
		m.LyricsFetching = tc.fetching
		if got := m.oneLineVisible(); got != tc.want {
			t.Errorf("lyrics=%v show=%v fetching=%v: got %v, want %v",
				tc.hasLyrics, tc.show, tc.fetching, got, tc.want)
		}
	}
}
