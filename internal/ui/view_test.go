package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

// lyricFooterModel is a model with lyrics loaded and UpdateLyricIndex already
// run, i.e. the state the update loop produces during playback.
func lyricFooterModel() *Model {
	m := setupModel()
	m.UI.Width, m.UI.Height = 80, 24
	m.Config.Lyrics.Panel = config.LyricsPanelConfig{
		Mode:         config.PanelModeAuto,
		Width:        testPanelWidth,
		ContextLines: config.DefaultPanelContextLines,
	}
	m.Audio.Lyrics = &lyrics.Data{Lines: []lyrics.LyricLine{
		{Time: 0, Text: "我听见雨滴落在青青草地"},
		{Time: 5 * time.Second, Text: "可是我没有听见你的声音"},
		{Time: 10 * time.Second, Text: "认真 呼唤我姓名"},
	}}
	m.Audio.ShowLyrics = true
	m.Audio.Elapsed = 5 * time.Second
	m.Audio.UpdateLyricIndex()
	return m
}

// footerLyricRow returns the plain text of the footer's lyric row, i.e. the row
// just above the bottom border.
func footerLyricRow(t *testing.T, m *Model) string {
	t.Helper()
	lines := strings.Split(renderFooter(m, m.layoutPlan()), "\n")
	if len(lines) < 2 {
		t.Fatalf("footer rendered %d rows", len(lines))
	}
	return lines[len(lines)-2]
}

func TestRenderFooter_LyricRowPresence(t *testing.T) {
	gapLines := &lyrics.Data{Lines: []lyrics.LyricLine{
		{Time: 0, End: 5 * time.Second, Text: "first"},
		{Time: 12 * time.Second, Text: "next"},
	}}
	shortGapLines := &lyrics.Data{Lines: []lyrics.LyricLine{
		{Time: 0, End: 5 * time.Second, Text: "first"},
		{Time: 8 * time.Second, Text: "next"},
	}}

	icons := setupModel().Icons

	cases := []struct {
		name     string
		mutate   func(*Model)
		wantRows int
		want     []string
		absent   []string
	}{
		{
			name:     "active line",
			mutate:   func(*Model) {},
			wantRows: 6,
			want:     []string{"可是我没有听见你的声音"},
		},
		{
			name:     "lyrics off drops the row",
			mutate:   func(m *Model) { m.Audio.ShowLyrics = false },
			wantRows: 5,
			absent:   []string{"可是我没有听见你的声音"},
		},
		{
			name: "no lyric data drops the row",
			mutate: func(m *Model) {
				m.Audio.Lyrics = nil
				m.Audio.ActiveLyricLines = nil
			},
			wantRows: 5,
		},
		{
			name: "fetching shows a placeholder row",
			mutate: func(m *Model) {
				m.Audio.Lyrics = nil
				m.Audio.ActiveLyricLines = nil
				m.LyricsFetching = true
			},
			wantRows: 6,
			want:     []string{"[Fetching lyrics...]"},
		},
		{
			name: "long gap shows countdown dots and the next line",
			mutate: func(m *Model) {
				m.Audio.Lyrics = gapLines
				m.Audio.Elapsed = 9500 * time.Millisecond
				m.Audio.UpdateLyricIndex()
			},
			wantRows: 6,
			want: []string{
				icons.LyricEmpty + " " + icons.LyricEmpty + " " + icons.LyricFilled,
				"next",
			},
		},
		{
			name: "short gap shows a placeholder dash",
			mutate: func(m *Model) {
				m.Audio.Lyrics = shortGapLines
				m.Audio.Elapsed = 6 * time.Second
				m.Audio.UpdateLyricIndex()
			},
			wantRows: 6,
			want:     []string{"-"},
			absent:   []string{"next"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := lyricFooterModel()
			tc.mutate(m)

			out := renderFooter(m, m.layoutPlan())
			if got := lipgloss.Height(out); got != tc.wantRows {
				t.Errorf("footer rows = %d, want %d", got, tc.wantRows)
			}
			if got := lipgloss.Width(out); got != m.UI.Width {
				t.Errorf("footer width = %d, want %d", got, m.UI.Width)
			}

			row := footerLyricRow(t, m)
			for _, want := range tc.want {
				if !strings.Contains(row, want) {
					t.Errorf("lyric row %q does not contain %q", row, want)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(row, absent) {
					t.Errorf("lyric row %q should not contain %q", row, absent)
				}
			}
		})
	}
}

// renderSingleLyricLine must always fill exactly maxWidth cells; the marquee is
// driven by an offset into the rune stream.
func TestRenderSingleLyricLine_Width(t *testing.T) {
	cases := []struct {
		text     string
		maxWidth int
		offset   int
		want     string
	}{
		{"short", 40, 0, "short"},
		{"我听见雨滴落在青青草地", 10, 0, "我听见雨滴"},
		{"我听见雨滴落在青青草地", 10, 5, "雨滴落在青"},
		{"我听见雨滴落在青青草地", 10, 100, "我听见雨滴"},
		{"", 10, 0, ""},
		{"a", 1, 0, "a"},
	}

	for _, tc := range cases {
		got := renderSingleLyricLine(tc.text, tc.maxWidth, tc.offset, nil)
		if w := lipgloss.Width(got); w != tc.maxWidth {
			t.Errorf("text=%q offset=%d: width = %d, want %d", tc.text, tc.offset, w, tc.maxWidth)
		}
		if !strings.Contains(got, tc.want) {
			t.Errorf("text=%q offset=%d: %q does not contain %q", tc.text, tc.offset, got, tc.want)
		}
	}
}

func TestBuildLyricCountdown(t *testing.T) {
	cases := []struct {
		secs float64
		want string
	}{
		{10, "○ ○ ○"},
		{3.1, "○ ○ ○"},
		{3, "○ ○ ●"},
		{2.5, "○ ○ ●"},
		{2, "○ ● ●"},
		{1.5, "○ ● ●"},
		{1, "● ● ●"},
		{0.2, "● ● ●"},
	}

	for _, tc := range cases {
		if got := buildLyricCountdown(tc.secs, "●", "○"); got != tc.want {
			t.Errorf("secs=%v: got %q, want %q", tc.secs, got, tc.want)
		}
	}
}

// renderContent must take its size from the plan: with the panel shown the
// content area is narrower by exactly the panel width.
func TestRenderContent_UsesPlanSize(t *testing.T) {
	for _, tc := range []struct {
		name  string
		width int
	}{
		{"no panel", 99},
		{"panel", 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := panelModel(t, 2)
			_, _ = handleResize(m, tea.WindowSizeMsg{Width: tc.width, Height: 24})
			plan := m.layoutPlan()

			out := renderContent(m, plan)
			if w := lipgloss.Width(out); w != plan.ContentWidth {
				t.Errorf("content width = %d, want %d", w, plan.ContentWidth)
			}
			if h := lipgloss.Height(out); h != plan.ContentHeight {
				t.Errorf("content rows = %d, want %d", h, plan.ContentHeight)
			}
			if plan.PanelShown != (tc.width == 100) {
				t.Fatalf("PanelShown = %v at %d columns", plan.PanelShown, tc.width)
			}
		})
	}
}

// The whole frame must tile the terminal exactly: H rows of W cells, always.
// This is what catches gaps and overlap when a region changes size.
func TestRenderMainView_FrameInvariants(t *testing.T) {
	sizes := [][2]int{{68, 17}, {80, 24}, {99, 24}, {100, 24}, {140, 36}, {200, 40}}
	modes := []string{config.PanelModeAuto, config.PanelModeOn, config.PanelModeOff}
	states := []struct {
		name   string
		mutate func(*Model)
	}{
		{"lyrics", func(*Model) {}},
		{"lyrics off", func(m *Model) { m.Audio.ShowLyrics = false }},
		{"no lyrics", func(m *Model) {
			m.Audio.Lyrics = nil
			m.Audio.ActiveLyricLines = nil
		}},
		{"fetching", func(m *Model) {
			m.Audio.Lyrics = nil
			m.Audio.ActiveLyricLines = nil
			m.LyricsFetching = true
		}},
	}

	for _, size := range sizes {
		for _, mode := range modes {
			for _, st := range states {
				t.Run(fmt.Sprintf("%dx%d/%s/%s", size[0], size[1], mode, st.name), func(t *testing.T) {
					m := panelModel(t, 2)
					m.panelMode = mode
					st.mutate(m)

					// Feed the real resize path so tabWidth tracks the terminal,
					// exactly as the running app does. Setting UI.Width directly
					// would leave tabWidth at its default and wrap the tab row.
					_, _ = handleResize(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})

					plan := m.layoutPlan()
					lines := strings.Split(renderMainView(m).Content, "\n")
					if len(lines) != size[1] {
						t.Fatalf("frame rows = %d, want %d", len(lines), size[1])
					}
					for i, line := range lines {
						if w := lipgloss.Width(line); w != size[0] {
							t.Errorf("row %d width = %d, want %d", i, w, size[0])
						}
					}

					// Every body row carries one box per region: two rounded
					// top-left corners when the panel is shown, one otherwise.
					wantCorners := 1
					if plan.PanelShown {
						wantCorners = 2
					}
					topRow := lines[tabsHeight]
					if got := strings.Count(topRow, "╭"); got != wantCorners {
						t.Errorf("body top row has %d top-left corners, want %d", got, wantCorners)
					}
				})
			}
		}
	}
}
