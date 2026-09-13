package ui

import (
	"strings"
	"testing"
	"time"

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
		Width:        config.DefaultPanelWidth,
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
	lines := strings.Split(renderFooter(m), "\n")
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

			out := renderFooter(m)
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
