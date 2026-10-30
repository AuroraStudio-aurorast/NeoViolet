package ui

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

func TestStatusMessagesKeepOneRowAndSemanticLabels(t *testing.T) {
	m := sized(t, setupModel(), 68, 17)
	for _, failure := range []bool{false, true} {
		m.Info, m.Error = &MessageState{}, &MessageState{}
		message, label := m.Info, "Info"
		if failure {
			message, label = m.Error, "Error"
		}
		message.Set(strings.Repeat("cannot open 中文文件\n", 12), 30)
		row := renderHelp(m)
		if lipgloss.Height(row) != 1 || lipgloss.Width(row) != 68 {
			t.Fatalf("status overflow: %dx%d", lipgloss.Width(row), lipgloss.Height(row))
		}
		if !strings.HasPrefix(stripANSI(row), " "+label+"  ") {
			t.Fatalf("missing category %s: %q", label, stripANSI(row))
		}
		for _, bg := range frameBackgrounds(t, row)[0] {
			if bg != nil {
				t.Fatal("status hides the gradient")
			}
		}
		message.Tick()
		if message.Timer != 29 {
			t.Fatal("status styling changed dismissal timing")
		}
	}
}

func TestCompletionSelectionUsesForegroundWithoutLeakingFooterText(t *testing.T) {
	m := completionModel(t, 80, 24)
	m.completionCandidates = []candidate{{Value: "lrc", Desc: "Lyrics control"}, {Value: "open", Desc: "Load audio"}}
	m.completionIndex = 1
	rows := strings.Split(renderCompletion(m, m.layoutPlan()), "\n")
	if !strings.HasPrefix(stripANSI(rows[1]), "> open") {
		t.Fatal("selected candidate is not marked")
	}
	assertWarningInk(t, rows[1], "open", newInteractionPalette(m.Accent).prompt)
	for _, cells := range frameBackgrounds(t, strings.Join(rows, "\n")) {
		for _, bg := range cells {
			if bg != nil {
				t.Fatal("completion has a row background")
			}
		}
	}
	m.Audio.CurrentSong = "This underlying song must be erased"
	m.Audio.Player = &mockPlayer{}
	plan := m.layoutPlan()
	frame := stripANSI(m.View().Content)
	lines := strings.Split(frame, "\n")
	for i, row := range rows {
		if lines[completionTop(plan, len(rows))+i] != stripANSI(row) {
			t.Fatal("completion left underlying footer text visible")
		}
	}
}

func TestLyricEmptyStatesKeepActionVisibleInNarrowPanels(t *testing.T) {
	for _, noTrack := range []bool{true, false} {
		m := panelModel(t, 2)
		m.Audio.Lyrics, m.Audio.ActiveLyricLines = nil, nil
		want := []string{"No lyrics for this track", ":lrc switch online"}
		if noTrack {
			m.Audio.Player = nil
			want = []string{"No track loaded", ":open <path>"}
		}
		for _, height := range []int{4, 10, 17} {
			plan := layoutPlan{PanelInnerW: 21, PanelWidth: 24, PanelInnerH: height, ContentHeight: height}
			out := renderLyricsPanel(m, plan)
			if lipgloss.Width(out) != 24 || lipgloss.Height(out) != height {
				t.Fatal("empty state changed panel dimensions")
			}
			words := strings.Join(strings.Fields(stripANSI(out)), " ")
			words = strings.ReplaceAll(words, " │ ", " ")
			for _, text := range want {
				if !strings.Contains(words, text) {
					t.Errorf("empty state lost %q: %s", text, words)
				}
			}
		}
	}
}

func TestSingleLineLyricsPreserveWordHighlightsAndMarqueeText(t *testing.T) {
	m := lyricFooterModel()
	line := lyrics.LyricLine{Time: 0, End: 4 * time.Second, Text: "Rain across the city", Words: []lyrics.WordFragment{
		{Time: 0, Text: "Rain "}, {Time: time.Second, Text: "across "},
		{Time: 2 * time.Second, Text: "the "}, {Time: 3 * time.Second, Text: "city"},
	}}
	m.Audio.Lyrics = &lyrics.Data{Lines: []lyrics.LyricLine{line}}
	m.Audio.Elapsed = 1500 * time.Millisecond
	m.Audio.UpdateLyricIndex()
	full, _ := renderOneLineLyrics(m, 40)
	assertWarningInk(t, full, "Rain", color.RGBA{R: 255, G: 255, B: 255, A: 255})
	assertWarningInk(t, full, "city", color.RGBA{R: 138, G: 138, B: 138, A: 255})
	m.Audio.LyricScrollOffset = 5
	short, _ := renderOneLineLyrics(m, 8)
	if stripANSI(short) != "across t" {
		t.Fatalf("marquee lost styled text: %q", stripANSI(short))
	}
	if strings.Contains(short, "[3m") {
		t.Fatal("lyrics retain the old italic styling")
	}
}

func TestSaveFeedbackDoesNotClassifyLaterInfoAsSuccess(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	m := sized(t, setupModel(), 80, 24)
	cfg := config.DefaultConfig()
	m.Config = &cfg
	runSave(m, invocation{})
	if got := stripANSI(renderHelp(m)); !strings.HasPrefix(got, " Saved  ") {
		t.Fatalf("save feedback = %q", got)
	}
	m.Info.Set("Lyrics enabled", 30)
	if got := stripANSI(renderHelp(m)); !strings.HasPrefix(got, " Info  ") {
		t.Fatalf("success classification leaked: %q", got)
	}
}
