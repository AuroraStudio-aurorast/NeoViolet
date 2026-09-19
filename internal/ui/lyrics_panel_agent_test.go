package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

// playerAgentModel is a panel model whose lines carry agents. Each spec is
// "agent|text"; an empty agent means the parser gave that line no agent (common
// in TTML files that declare ttm:agent only where the singer changes). The line
// at cur is playing.
func playerAgentModel(t *testing.T, cur int, specs ...string) *Model {
	t.Helper()
	m := panelModel(t, 0)
	lines := make([]lyrics.LyricLine, 0, len(specs))
	for i, spec := range specs {
		agent, text, _ := strings.Cut(spec, "|")
		lines = append(lines, lyrics.LyricLine{
			Time:  time.Duration(i) * 5 * time.Second,
			Text:  text,
			Agent: agent,
		})
	}
	m.Audio.Lyrics = &lyrics.Data{
		Format: "ttml",
		Agents: map[string]string{"v1": "Taylor Swift", "v2": "Brendon Urie"},
		Lines:  lines,
	}
	m.Audio.Elapsed = lines[cur].Time
	m.Audio.UpdateLyricIndex()
	return m
}

// panelTexts renders the panel window and returns the text of each non-blank
// row in display order.
func panelTexts(t *testing.T, m *Model) []string {
	t.Helper()
	plan := m.layoutPlan()
	out := make([]string, 0, plan.PanelInnerH)
	for _, r := range panelWindow(m, plan) {
		if text := panelRowText(r); strings.TrimSpace(text) != "" {
			out = append(out, text)
		}
	}
	return out
}

// equalRows fails with the whole row list so a mismatch reads as a diff.
func equalRows(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %q, want %q", got, want)
		}
	}
}

// The label rule itself: the first line carrying an agent is labelled, and so is
// every later line whose agent differs from the previous line that carried one.
func TestPanelAgentLabels(t *testing.T) {
	cases := []struct {
		name   string
		agents []string
		want   []bool
	}{
		{"single agent repeated", []string{"v1", "v1", "v1"}, []bool{true, false, false}},
		{"singer changes back and forth", []string{"v1", "v1", "v2", "v1"}, []bool{true, false, true, true}},
		{"agentless line keeps the block", []string{"v1", "", "v1"}, []bool{true, false, false}},
		{"leading agentless lines", []string{"", "", "v2"}, []bool{false, false, true}},
		{"no agents at all", []string{"", ""}, []bool{false, false}},
		{"only the first line has an agent", []string{"v1", "", ""}, []bool{true, false, false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			visible := make([]lyrics.VisibleLine, len(tc.agents))
			for i, agent := range tc.agents {
				visible[i] = lyrics.VisibleLine{Index: i, Line: lyrics.LyricLine{Agent: agent}}
			}
			got := panelAgentLabels(visible)
			if len(got) != len(tc.want) {
				t.Fatalf("labels = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("labels = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// A single-singer file is labelled once for the whole panel instead of on every
// line: this is the noise the per-line prefix used to cause.
func TestPanelWindow_SingleSingerIsLabelledOnce(t *testing.T) {
	m := playerAgentModel(t, 0, "v1|line one", "v1|line two", "v1|line three")
	equalRows(t, panelTexts(t, m), []string{
		"Taylor Swift: line one",
		"line two",
		"line three",
	})
}

// The label follows a change of singer, and only there.
func TestPanelWindow_LabelFollowsSingerChange(t *testing.T) {
	m := playerAgentModel(t, 0, "v1|first", "v1|second", "v2|third", "v2|fourth")
	equalRows(t, panelTexts(t, m), []string{
		"Taylor Swift: first",
		"second",
		"Brendon Urie: third",
		"fourth",
	})
}

// An agentless line neither gets a label nor re-labels the singer it interrupts.
func TestPanelWindow_AgentlessLineDoesNotRelabel(t *testing.T) {
	m := playerAgentModel(t, 0, "v1|first", "|interlude", "v1|second")
	equalRows(t, panelTexts(t, m), []string{
		"Taylor Swift: first",
		"interlude",
		"second",
	})
}

// The label belongs to the line, not to each of its display parts: a bilingual
// line must not repeat it on the translation row (TTML Parts = [original,
// background, translation]).
func TestPanelWindow_MergedPartsLabelFirstPartOnly(t *testing.T) {
	m := panelModel(t, 0)
	m.Audio.Lyrics = &lyrics.Data{
		Format: "ttml",
		Agents: map[string]string{"v1": "Taylor Swift"},
		Lines: []lyrics.LyricLine{{
			Time:  0,
			Text:  "Hello world | 你好世界",
			Parts: []string{"Hello world", "你好世界"},
			Agent: "v1",
		}},
	}
	m.Audio.Elapsed = 0
	m.Audio.UpdateLyricIndex()

	equalRows(t, panelTexts(t, m), []string{
		"Taylor Swift: Hello world",
		"你好世界",
	})
}

// The label is decided over the whole visible sequence, not over the rendered
// window: the singer changes on a context line below the current group, and that
// line still has to carry the label. This is the path that goes through
// panelFormat.collectRows.
func TestPanelWindow_ContextLineKeepsItsLabel(t *testing.T) {
	m := playerAgentModel(t, 1, "v1|first", "v1|second", "v1|third", "v2|fourth")
	equalRows(t, panelTexts(t, m), []string{
		"Taylor Swift: first",
		"second",
		"third",
		"Brendon Urie: fourth",
	})
}

// The label is also decided above the current group: line 0 is above the group
// and its label must not be dropped by the upward walk.
func TestPanelWindow_LabelAboveTheGroupIsKept(t *testing.T) {
	m := playerAgentModel(t, 2, "v1|first", "v1|second", "v1|third")
	equalRows(t, panelTexts(t, m), []string{
		"Taylor Swift: first",
		"second",
		"third",
	})
}

// The label survives the karaoke split: the highlighted line still shows it once,
// ahead of the played/unplayed spans.
func TestPanelWindow_LabelWithKaraoke(t *testing.T) {
	m := panelModel(t, 2)
	m.Audio.Lyrics = &lyrics.Data{
		Format: "ttml",
		Agents: map[string]string{"v1": "Taylor Swift"},
		Lines: []lyrics.LyricLine{{
			Time:  0,
			Text:  "I could find you",
			Agent: "v1",
			Words: []lyrics.WordFragment{
				{Time: 0, Text: "I could "},
				{Time: 3 * time.Second, Text: "find you"},
			},
		}},
	}
	m.Audio.Elapsed = 2 * time.Second // the 3s word is still unplayed
	m.Audio.UpdateLyricIndex()

	plan := m.layoutPlan()
	rows := panelWindow(m, plan)
	anchor := panelAnchor(plan.PanelInnerH)

	// The label costs 14 columns, so this line wraps where the unlabelled one did
	// not; the group's rows together must still read as the labelled text.
	spans := rows[anchor].spans
	if len(spans) < 2 {
		t.Fatalf("row %d has %d spans, want the label plus the played text", anchor, len(spans))
	}
	if spans[0].Text != "Taylor Swift: " {
		t.Fatalf("first span = %q, want the label span", spans[0].Text)
	}
	current := panelCurrentStyle(m).Render("x")
	if got := spans[0].Style.Render("x"); got != current {
		t.Errorf("label style is not the current-line style (rendered %q, want %q)", got, current)
	}

	var joined strings.Builder
	for i := anchor; i < len(rows) && len(rows[i].spans) > 0; i++ {
		joined.WriteString(panelRowText(rows[i]))
	}
	if got := joined.String(); got != "Taylor Swift: I could find you" {
		t.Errorf("joined rows = %q, want the labelled line", got)
	}
}

// Only the panel changed: the one-line footer still labels the line it shows,
// because there the label is the only thing saying who is singing.
func TestOneLineLyricsStillLabelsAgent(t *testing.T) {
	m := playerAgentModel(t, 0, "v1|first", "v1|second")
	if got := oneLineLyricText(m); !strings.Contains(got, "Taylor Swift: first") {
		t.Errorf("one-line lyrics = %q, want the agent label kept", got)
	}
}
