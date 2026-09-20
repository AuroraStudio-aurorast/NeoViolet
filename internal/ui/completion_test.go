package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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

	// Clearing the input must clear the candidates and the cached segment.
	ti.SetValue("")
	syncCompletion(m)
	if len(m.completionCandidates) != 0 || m.completionIndex != -1 || m.completionSeg != (segment{}) {
		t.Errorf("after clearing input: candidates = %d, index = %d, seg = %+v",
			len(m.completionCandidates), m.completionIndex, m.completionSeg)
	}
}

// The cursor can rest in the whitespace between two words, or before the first
// one. The segment is then the empty interval at the cursor, and no word after
// the cursor may count as a previous segment.
func TestCompletionContextInWhitespace(t *testing.T) {
	ctx := completionContextAt("lrc  switch", 4)
	if ctx.Seg != (segment{Index: 1, Prefix: "", Start: 4, End: 4}) {
		t.Errorf("segment = %+v, want the empty segment after \"lrc\"", ctx.Seg)
	}
	if len(ctx.Before) != 1 || ctx.Before[0] != "lrc" {
		t.Errorf("Before = %v, want [lrc]", ctx.Before)
	}
	cands := value(t, candidatesFor(ctx))
	if len(cands) != 7 || cands[0] != "on" {
		t.Errorf("candidates = %v, want the 7 subcommands", cands)
	}

	ctx = completionContextAt(" lrc", 0)
	if ctx.Seg.Index != 0 || ctx.Seg.Start != 0 || ctx.Seg.End != 0 {
		t.Errorf("segment before the first word = %+v, want index 0 at the cursor", ctx.Seg)
	}
	if len(ctx.Before) != 0 {
		t.Errorf("Before = %v, want none: no word precedes the cursor", ctx.Before)
	}
}

// Completion must split on the same whitespace parseInvocation does
// (unicode.IsSpace), so an NBSP-separated line completes like a plain space.
func TestCompletionSplitsOnUnicodeSpace(t *testing.T) {
	ctx := completionContextAt("lrc\u00a0s", 5)
	if ctx.Seg != (segment{Index: 1, Prefix: "s", Start: 4, End: 5}) {
		t.Errorf("segment = %+v", ctx.Seg)
	}
	if got := strings.Join(value(t, candidatesFor(ctx)), ","); got != "switch" {
		t.Errorf("candidates = %q, want \"switch\"", got)
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

func commandModeModel(t *testing.T, value string) *Model {
	t.Helper()
	m := setupModel()
	m.UI.Mode = ModeCommand
	m.Components.CommandInput.Focus()
	m.Components.CommandInput.SetValue(value)
	m.Components.CommandInput.CursorEnd()
	syncCompletion(m)
	return m
}

func TestTabCyclesAndWritesTheCandidate(t *testing.T) {
	m := commandModeModel(t, "lrc s")
	updated, _ := handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(*Model)
	if got := m.Components.CommandInput.Value(); got != "lrc switch" {
		t.Fatalf("value = %q, want \"lrc switch\"", got)
	}
	if m.completionIndex < 0 {
		t.Error("completionIndex = -1 after tab, want a selection")
	}
	if got := m.Components.CommandInput.Position(); got != len("lrc switch") {
		t.Errorf("cursor = %d, want %d", got, len("lrc switch"))
	}
}

func TestCtrlNAndCtrlPCycle(t *testing.T) {
	m := commandModeModel(t, "lrc ")
	// Seven candidates starting at -1: ctrl+n selects 0 (on), ctrl+n again moves to
	// off, ctrl+p goes back to on.
	updated, _ := handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	m = updated.(*Model)
	if got := m.Components.CommandInput.Value(); got != "lrc on" {
		t.Fatalf("after ctrl+n: value = %q, want \"lrc on\"", got)
	}
	updated, _ = handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	m = updated.(*Model)
	if got := m.Components.CommandInput.Value(); got != "lrc off" {
		t.Fatalf("after second ctrl+n: value = %q, want \"lrc off\"", got)
	}
	updated, _ = handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	m = updated.(*Model)
	if got := m.Components.CommandInput.Value(); got != "lrc on" {
		t.Fatalf("after ctrl+p: value = %q, want \"lrc on\"", got)
	}
}

// Cycling must wrap and must not grow the value.
func TestTabWrapsWithoutGrowingTheValue(t *testing.T) {
	m := commandModeModel(t, "lrc panel ")
	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		updated, _ := handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyTab})
		m = updated.(*Model)
		value := m.Components.CommandInput.Value()
		if len(value) > len("lrc panel auto") {
			t.Fatalf("value grew to %q", value)
		}
		seen[value] = true
	}
	if len(seen) != 3 {
		t.Errorf("cycled through %d distinct values, want 3 (%v)", len(seen), seen)
	}
}

// up/down still recall history: completion never touches them.
func TestHistoryKeysStillWorkInCommandMode(t *testing.T) {
	m := commandModeModel(t, "")
	m.CommandHistory = []string{"lrc on", "vol 0.5"}
	m.historyIndex = len(m.CommandHistory)

	updated, _ := handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(*Model)
	if got := m.Components.CommandInput.Value(); got != "vol 0.5" {
		t.Errorf("after up: value = %q, want the previous history entry", got)
	}
	if m.completionIndex != -1 {
		t.Errorf("completionIndex = %d after history recall, want -1", m.completionIndex)
	}
}

// A user keystroke resets the selection (otherwise enter would run a stale
// choice).
func TestTypingResetsSelection(t *testing.T) {
	m := commandModeModel(t, "lrc s")
	updated, _ := handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(*Model)
	if m.completionIndex < 0 {
		t.Fatal("expected a selection after tab")
	}
	updated, _ = handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = updated.(*Model)
	if m.completionIndex != -1 {
		t.Errorf("completionIndex = %d after typing, want -1", m.completionIndex)
	}
}

// Completing a directory writes it and keeps the round: the listing must not
// swap under the user, so a further tab walks that round's siblings.
func TestAcceptDirectoryKeepsTheRound(t *testing.T) {
	root := mkTree(t)
	m := commandModeModel(t, "open "+root+"/A")
	updated, _ := handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(*Model)
	if got := m.Components.CommandInput.Value(); got != "open "+root+"/Album/" {
		t.Fatalf("value = %q, want the Album directory", got)
	}
	if m.completionIndex != 0 {
		t.Errorf("completionIndex = %d after completing a directory, want 0", m.completionIndex)
	}
	// The round is still the parent's listing: completing the directory did not
	// swap the candidates for the (empty) directory's own contents.
	if len(m.completionCandidates) != 1 {
		t.Errorf("candidates = %d, want the round's 1 match", len(m.completionCandidates))
	}

	// A keystroke starts a new round against what is on the line now.
	updated, _ = handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = updated.(*Model)
	if m.completionIndex != -1 {
		t.Errorf("completionIndex = %d after typing, want -1", m.completionIndex)
	}
}

// A second tab replaces the previous candidate instead of appending to it: the
// round's segment end follows the text that was written.
func TestTabCyclesPathCandidatesWithoutGrowingTheValue(t *testing.T) {
	root := mkTree(t)
	m := commandModeModel(t, "open "+root+"/")
	updated, _ := handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(*Model)
	if got, want := m.Components.CommandInput.Value(), "open "+root+"/Album/"; got != want {
		t.Fatalf("first tab: value = %q, want %q", got, want)
	}

	updated, _ = handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(*Model)
	if got, want := m.Components.CommandInput.Value(), "open "+root+"/a.flac"; got != want {
		t.Fatalf("second tab: value = %q, want %q", got, want)
	}
	if m.completionIndex != 1 {
		t.Errorf("completionIndex = %d after the second tab, want 1", m.completionIndex)
	}
}
