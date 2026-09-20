package ui

import (
	"strings"
	"testing"
)

func TestSegmentAt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		pos   int
		want  segment
	}{
		{"empty line", "", 0, segment{Index: 0, Prefix: "", Start: 0, End: 0}},
		{"first word", "lrc", 3, segment{Index: 0, Prefix: "lrc", Start: 0, End: 3}},
		{"partial first word", "lr", 2, segment{Index: 0, Prefix: "lr", Start: 0, End: 2}},
		{"second word", "lrc sw", 6, segment{Index: 1, Prefix: "sw", Start: 4, End: 6}},
		{"after trailing space", "lrc ", 4, segment{Index: 1, Prefix: "", Start: 4, End: 4}},
		{"third word", "lrc switch q", 12, segment{Index: 2, Prefix: "q", Start: 11, End: 12}},
		{"cursor mid-word", "lrc switch", 5, segment{Index: 1, Prefix: "s", Start: 4, End: 5}},
		{"two spaces", "lrc  sw", 7, segment{Index: 1, Prefix: "sw", Start: 5, End: 7}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := completionContextAt(tc.value, tc.pos).Seg; got != tc.want {
				t.Errorf("segment = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCompletionContextBefore(t *testing.T) {
	ctx := completionContextAt("lrc switch q", 12)
	if len(ctx.Before) != 2 || ctx.Before[0] != "lrc" || ctx.Before[1] != "switch" {
		t.Errorf("Before = %v, want [lrc switch]", ctx.Before)
	}
}

func TestCandidatesForCommandNames(t *testing.T) {
	got := value(t, candidatesFor(completionContextAt("l", 1)))
	want := []string{"load", "lrc", "lyric", "lyrics"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("candidates for \"l\" = %v, want %v", got, want)
	}
}

func TestCandidatesForLrcSubcommands(t *testing.T) {
	got := value(t, candidatesFor(completionContextAt("lrc ", 4)))
	if len(got) != 7 {
		t.Fatalf("candidates = %v, want the 7 subcommands", got)
	}
	if got[0] != "on" {
		t.Errorf("first subcommand = %q, want \"on\"", got)
	}
	// Aliases must reach the subcommand segment too.
	if got := value(t, candidatesFor(completionContextAt("lyrics s", 8))); strings.Join(got, ",") != "switch" {
		t.Errorf("candidates for \"lyrics s\" = %v, want [switch]", got)
	}
}

func TestCandidatesForLrcSwitchFormats(t *testing.T) {
	cands := candidatesFor(completionContextAt("lrc switch ", 11))
	if len(cands) != 10 {
		t.Fatalf("candidates = %d, want 10 (online + 9 parsers)", len(cands))
	}
	if cands[0].Value != "online" {
		t.Errorf("first candidate = %q, want \"online\"", cands[0].Value)
	}
	byValue := map[string]string{}
	for _, c := range cands {
		if c.Desc == "" {
			t.Errorf("candidate %q has no description", c.Value)
		}
		if strings.Contains(c.Desc, "EXPERIMENTAL") {
			t.Errorf("candidate %q leaks the EXPERIMENTAL marker", c.Value)
		}
		byValue[c.Value] = c.Desc
	}
	if byValue["qrc"] != "QQ Music Word-for-Word Lyrics" {
		t.Errorf("qrc desc = %q", byValue["qrc"])
	}
	if byValue["embedded"] != "Lyrics Embedded in Audio Tags" {
		t.Errorf("embedded must be offered as well, got %q", byValue["embedded"])
	}
	// Registration order, unsorted.
	if cands[1].Value != "embedded" {
		t.Errorf("second candidate = %q, want \"embedded\" (registration order)", cands[1].Value)
	}
}

func TestCandidatesForPanelAndAgent(t *testing.T) {
	if got := value(t, candidatesFor(completionContextAt("lrc panel o", 11))); strings.Join(got, ",") != "on,off" {
		t.Errorf("panel candidates = %v", got)
	}
	if got := value(t, candidatesFor(completionContextAt("lrc agent ", 10))); strings.Join(got, ",") != "all" {
		t.Errorf("agent candidates = %v", got)
	}
}

func TestCandidatesForUnknownSegments(t *testing.T) {
	for _, value := range []string{"vol ", "lrc nope ", "seek "} {
		if got := candidatesFor(completionContextAt(value, len(value))); len(got) != 0 {
			t.Errorf("candidates for %q = %v, want none", value, got)
		}
	}
}

func TestSyncCompletionResetsSelection(t *testing.T) {
	m := setupModel()
	ti := &m.Components.CommandInput
	m.UI.Mode = ModeCommand

	ti.SetValue("lrc s")
	ti.CursorEnd()
	syncCompletion(m)
	if len(m.completionCandidates) != 1 || m.completionCandidates[0].Value != "switch" {
		t.Fatalf("candidates = %+v, want [switch]", m.completionCandidates)
	}
	if m.completionIndex != -1 {
		t.Errorf("completionIndex = %d, want -1 (passive list must not preselect)", m.completionIndex)
	}
	if m.completionSeg != (segment{Index: 1, Prefix: "s", Start: 4, End: 5}) {
		t.Errorf("completionSeg = %+v", m.completionSeg)
	}

	// Clearing the input must clear the candidates.
	ti.SetValue("")
	syncCompletion(m)
	if len(m.completionCandidates) != 0 || m.completionIndex != -1 {
		t.Errorf("after clearing input: candidates = %d, index = %d", len(m.completionCandidates), m.completionIndex)
	}
}

func value(t *testing.T, cands []candidate) []string {
	t.Helper()
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.Value)
	}
	return out
}
