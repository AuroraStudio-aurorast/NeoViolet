package lyrics

import (
	"testing"
	"time"
)

// partsOf returns a line's display rows, with a plain line reported as its one
// row so a case can state what the panel draws either way.
func partsOf(l LyricLine) []string {
	if l.Parts == nil {
		return []string{l.Text}
	}
	return l.Parts
}

// TestSPL_TimestamplessTranslation pins the standard's other way of writing a
// translation: a lyric line followed by a line with no stamp of its own. It stays
// one lyric line that draws as two rows, and only the rows after the first are
// translations, which is what the panel styles them by.
func TestSPL_TimestamplessTranslation(t *testing.T) {
	d := parseSPL(t, "[05:20.22]Hello\nBonjour\n")

	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1: a translation is not a lyric line of its own", len(d.Lines))
	}
	line := d.Lines[0]
	if got, want := line.Text, "Hello | Bonjour"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
	if got := partsOf(line); len(got) != 2 || got[0] != "Hello" || got[1] != "Bonjour" {
		t.Errorf("Parts = %v, want [Hello Bonjour]", got)
	}
	if !d.TranslationsInParts {
		t.Error("TranslationsInParts = false, want true so the panel styles the second row")
	}
	// The words belong to the lyric line, not to the translation: they still tile
	// the first row rather than the joined text.
	if got := wordsTileText(line); got != "Hello" {
		t.Errorf("Words = %q, want them to tile the first row", got)
	}
	if line.Time != 5*time.Minute+20*time.Second+220*time.Millisecond {
		t.Errorf("Time = %v, want the lyric line's own stamp", line.Time)
	}
}

// TestSPL_MultiLineTranslation pins the standard's multi-line translation: every
// consecutive line without a stamp stays part of the same translation until
// another lyric line takes over.
func TestSPL_MultiLineTranslation(t *testing.T) {
	d := parseSPL(t, "[05:20.22]Hello\nBonjour\nHola\n")

	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	want := []string{"Hello", "Bonjour", "Hola"}
	got := partsOf(d.Lines[0])
	if len(got) != len(want) {
		t.Fatalf("Parts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Parts[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestSPL_TranslationFollowsTheLineAboveIt pins where a stamp-less line attaches:
// the standard shows that a lyric line ending the translation of the line above it
// becomes the anchor itself, so the text below it translates that line instead.
func TestSPL_TranslationFollowsTheLineAboveIt(t *testing.T) {
	d := parseSPL(t, "[00:01.00]First\n[00:02.00]Second\nTranslation\n")

	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2", len(d.Lines))
	}
	if got := partsOf(d.Lines[0]); len(got) != 1 || got[0] != "First" {
		t.Errorf("Lines[0].Parts = %v, want [First]", got)
	}
	if got := partsOf(d.Lines[1]); len(got) != 2 || got[1] != "Translation" {
		t.Errorf("Lines[1].Parts = %v, want the translation on the line above it", got)
	}
}

// TestSPL_TranslationBeforeAnyLyricIsDropped pins that a stamp-less line with no
// lyric line above it has nothing to translate and is not invented into a lyric.
func TestSPL_TranslationBeforeAnyLyricIsDropped(t *testing.T) {
	d := parseSPL(t, "loose text\n[00:01.00]Hello\n")

	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	if d.Lines[0].Text != "Hello" || d.Lines[0].Parts != nil {
		t.Errorf("Lines[0] = %q %v, want a plain line", d.Lines[0].Text, d.Lines[0].Parts)
	}
}

// TestSPL_EndMarkerDoesNotInterruptATranslation pins that an end-marker line stays
// what the standard calls it: not a lyric line, and therefore not a line that
// takes the translation below it away from the lyric it belongs to.
func TestSPL_EndMarkerDoesNotInterruptATranslation(t *testing.T) {
	d := parseSPL(t, "[00:01.00]Hello\n[00:02.00]\nBonjour\n")

	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	if got := partsOf(d.Lines[0]); len(got) != 2 || got[1] != "Bonjour" {
		t.Errorf("Parts = %v, want the translation to stay with the lyric line", got)
	}
	if d.Lines[0].End != 2*time.Second {
		t.Errorf("End = %v, want the end marker to still end the line", d.Lines[0].End)
	}
}

// TestSPL_RepeatLineTranslatesEveryCopy pins the reading of a translation that
// follows a repeat line: the repeated sentence is one lyric with one translation,
// and it is shown as many times as the sentence is.
func TestSPL_RepeatLineTranslatesEveryCopy(t *testing.T) {
	d := parseSPL(t, "[00:01.00][00:02.00]Hello\nBonjour\n")

	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2", len(d.Lines))
	}
	for i, line := range d.Lines {
		if got := partsOf(line); len(got) != 2 || got[1] != "Bonjour" {
			t.Errorf("Lines[%d].Parts = %v, want the translation on every copy", i, got)
		}
	}
}

// TestSPL_SameTimestampTranslation pins the standard's first way of writing a
// translation: two lines that carry the same stamp are one lyric line whose second
// row is the translation.
func TestSPL_SameTimestampTranslation(t *testing.T) {
	d := parseSPL(t, "[05:20.22]Hello\n[05:20.22]Bonjour\n")

	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1: the stamp pairs the two lines", len(d.Lines))
	}
	line := d.Lines[0]
	if got := partsOf(line); len(got) != 2 || got[0] != "Hello" || got[1] != "Bonjour" {
		t.Errorf("Parts = %v, want [Hello Bonjour]", got)
	}
	if got, want := line.Text, "Hello | Bonjour"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
	if line.Time != 5*time.Minute+20*time.Second+220*time.Millisecond {
		t.Errorf("Time = %v, want the shared stamp", line.Time)
	}
	if !d.TranslationsInParts {
		t.Error("TranslationsInParts = false, want true for the translation row")
	}
}

// TestSPL_SameTimestampTranslationNeedNotBeAdjacent pins the grouping the standard
// allows: it says the translation may be written apart from its lyric line, so the
// file is grouped as a whole. LRC's merge only folds adjacent runs, which would
// leave the pair below as three lines.
func TestSPL_SameTimestampTranslationNeedNotBeAdjacent(t *testing.T) {
	d := parseSPL(t, "[00:01.00]Hello\n[00:02.00]Middle\n[00:01.00]Bonjour\n")

	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2", len(d.Lines))
	}
	if got := partsOf(d.Lines[0]); len(got) != 2 || got[1] != "Bonjour" {
		t.Errorf("Lines[0].Parts = %v, want the translation folded into its own stamp", got)
	}
	if d.Lines[1].Text != "Middle" || d.Lines[1].Parts != nil {
		t.Errorf("Lines[1] = %q %v, want a plain line", d.Lines[1].Text, d.Lines[1].Parts)
	}
}

// TestSPL_MainLineGovernsTheMergedLine pins which of two lines that share a stamp
// owns the merged one: the standard puts the lyric line first and the translation
// after it, so the words and the end marker of the line that comes first count, and
// the translation only adds a row.
func TestSPL_MainLineGovernsTheMergedLine(t *testing.T) {
	d := parseSPL(t, "[00:01.00]Hello[00:03.00]\n[00:01.00]Bonjour\n")

	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	line := d.Lines[0]
	if line.End != 3*time.Second {
		t.Errorf("End = %v, want the main line's 3s", line.End)
	}
	if got := wordsTileText(line); got != "Hello" {
		t.Errorf("Words = %q, want the main line's words", got)
	}
	if got := wordsTileText(line); got != line.Part(0) {
		t.Errorf("Words %q do not tile the first row %q", got, line.Part(0))
	}
}

// TestSPL_TranslationWrittenFirstLosesItsOwnEnd pins the cost of that rule, so it
// cannot change by accident: when the translation is the line written first, it is
// the one that governs, and the end marker sitting on the other line is not read as
// the line's end.
func TestSPL_TranslationWrittenFirstLosesItsOwnEnd(t *testing.T) {
	d := parseSPL(t, "[00:01.00]Bonjour\n[00:01.00]Hello[00:03.00]\n")

	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	if got := partsOf(d.Lines[0]); len(got) != 2 || got[0] != "Bonjour" {
		t.Errorf("Parts = %v, want the first line kept as the main row", got)
	}
	if d.Lines[0].End != 0 {
		t.Errorf("End = %v, want 0: the line written first carries no end", d.Lines[0].End)
	}
}
