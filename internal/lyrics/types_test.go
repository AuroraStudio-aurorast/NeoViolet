package lyrics

import (
	"strings"
	"testing"
	"time"
)

// A line that never went through a merge carries no Parts: Part(0) is the text
// itself, so every renderer can loop over parts without a special case.
func TestLyricLine_PartAccessors_PlainLine(t *testing.T) {
	plain := LyricLine{Text: "hello"}

	if got := plain.PartCount(); got != 1 {
		t.Errorf("PartCount() = %d, want 1", got)
	}
	if got := plain.Part(0); got != "hello" {
		t.Errorf("Part(0) = %q, want %q", got, "hello")
	}
	if plain.Parts != nil {
		t.Errorf("Parts = %v, want nil for a plain line", plain.Parts)
	}
}

// A merged line keeps Text as the single string the legacy consumers read
// (one_line footer, lyricSig, MPRIS) and exposes the source texts as Parts.
func TestLyricLine_PartAccessors_MergedLine(t *testing.T) {
	merged := LyricLine{Text: "hello | 你好", Parts: []string{"hello", "你好"}}

	if got := merged.PartCount(); got != 2 {
		t.Fatalf("PartCount() = %d, want 2", got)
	}
	for i, want := range merged.Parts {
		if got := merged.Part(i); got != want {
			t.Errorf("Part(%d) = %q, want %q", i, got, want)
		}
	}
	if got, want := strings.Join(merged.Parts, " | "), merged.Text; got != want {
		t.Errorf("Join(Parts, \" | \") = %q, want Text %q", got, want)
	}
}

func TestActiveLines_UnboundedAfterBoundedIsReachable(t *testing.T) {
	// The file has both bounded lines (A/B) and an unbounded last line (C). A
	// single global "does any line carry an end" switch would keep C out of the
	// candidate set forever, so the rule has to be decided per line.
	d := &Data{Lines: []LyricLine{
		{Time: 1 * time.Second, End: 2 * time.Second, Text: "A"},
		{Time: 1 * time.Second, End: 2 * time.Second, Text: "B"},
		{Time: 2 * time.Second, Text: "C"},
	}}

	cases := []struct {
		at   time.Duration
		want []string
	}{
		{500 * time.Millisecond, nil},
		{1 * time.Second, []string{"A", "B"}},
		{1999 * time.Millisecond, []string{"A", "B"}},
		{2 * time.Second, []string{"C"}},
		{30 * time.Second, []string{"C"}},
	}
	for _, tc := range cases {
		got := d.ActiveLines(tc.at)
		var texts []string
		for _, l := range got {
			texts = append(texts, l.Text)
		}
		if len(texts) != len(tc.want) {
			t.Fatalf("ActiveLines(%v) = %v, want %v", tc.at, texts, tc.want)
		}
		for i := range texts {
			if texts[i] != tc.want[i] {
				t.Errorf("ActiveLines(%v)[%d] = %q, want %q", tc.at, i, texts[i], tc.want[i])
			}
		}
	}
}

func TestActiveLines_SameTimeSiblingsDoNotCutEachOtherOff(t *testing.T) {
	// Sibling lines at the same Time (the two languages of one SMI <SYNC> <P>)
	// must be active together, so an unbounded line's right edge is the next line
	// with a **greater** Time, not simply the next line.
	d := &Data{Lines: []LyricLine{
		{Time: 1 * time.Second, Text: "v1"},
		{Time: 1 * time.Second, Text: "v2"},
		{Time: 3 * time.Second, Text: "next"},
	}}
	if got := len(d.ActiveLines(1 * time.Second)); got != 2 {
		t.Errorf("ActiveLines(1s) returned %d lines, want 2", got)
	}
	if got := len(d.ActiveLines(2 * time.Second)); got != 2 {
		t.Errorf("ActiveLines(2s) returned %d lines, want 2", got)
	}
	if got := len(d.ActiveLines(3 * time.Second)); got != 1 {
		t.Errorf("ActiveLines(3s) returned %d lines, want 1", got)
	}
}

func TestActiveLines_RegressionUnboundedFilesAndBoundedGaps(t *testing.T) {
	// Two regression pins: an all-unbounded file still yields exactly one line
	// (LRC behaviour, unchanged), and an all-bounded file keeps its inter-line
	// gaps as gaps (a TTML line is not "stuck" to the previous one).
	allUnbounded := &Data{Lines: []LyricLine{
		{Time: 0, Text: "one"},
		{Time: 5 * time.Second, Text: "two"},
		{Time: 10 * time.Second, Text: "three"},
	}}
	for _, at := range []time.Duration{0, 4 * time.Second, 5 * time.Second, 9 * time.Second, 20 * time.Second} {
		if got := len(allUnbounded.ActiveLines(at)); got != 1 {
			t.Errorf("all-unbounded ActiveLines(%v) returned %d lines, want exactly 1", at, got)
		}
	}

	withGaps := &Data{Lines: []LyricLine{
		{Time: 0, End: 1 * time.Second, Text: "first"},
		{Time: 3 * time.Second, End: 4 * time.Second, Text: "second"},
	}}
	if got := withGaps.ActiveLines(2 * time.Second); len(got) != 0 {
		t.Errorf("bounded gap ActiveLines(2s) = %v, want empty", got)
	}
	if got := withGaps.ActiveLines(3 * time.Second); len(got) != 1 || got[0].Text != "second" {
		t.Errorf("bounded ActiveLines(3s) = %v, want [second]", got)
	}
}

func TestActiveLines_ZeroLengthLineIsReachable(t *testing.T) {
	// A line with End == Time carries no usable duration. Under the old
	// "bounded when End > 0" rule its window Time <= t < End was empty, so the
	// line was unreachable forever (corpus hit: an AMLL TTML <p> with begin ==
	// end). End <= Time is now treated as unbounded. Data is hand-built here so
	// the test pins ActiveLines alone, not any parser's output.
	const text = "啊"

	// 1. A lone zero-length line is reachable at its own Time and, being last,
	// is never closed.
	lone := &Data{Lines: []LyricLine{{Time: 10 * time.Second, End: 10 * time.Second, Text: text}}}
	if got := lone.ActiveLines(10 * time.Second); len(got) != 1 || got[0].Text != text {
		t.Errorf("ActiveLines(10s) = %v, want the zero-length line itself", got)
	}
	if got := lone.ActiveLines(10*time.Second + time.Millisecond); len(got) != 1 || got[0].Text != text {
		t.Errorf("ActiveLines(10s+1ms) = %v, want the zero-length line still active", got)
	}
	if got := lone.ActiveLines(9999 * time.Millisecond); got != nil {
		t.Errorf("ActiveLines(9.999s) = %v, want nil (before its Time)", got)
	}

	// 2. With a later line it has the unbounded semantics: the next greater Time
	// closes its window, so it must not stick.
	withNext := &Data{Lines: []LyricLine{
		{Time: 10 * time.Second, End: 10 * time.Second, Text: text},
		{Time: 20 * time.Second, Text: "next"},
	}}
	if got := withNext.ActiveLines(10*time.Second + time.Millisecond); len(got) != 1 || got[0].Text != text {
		t.Errorf("ActiveLines(10s+1ms) = %v, want the zero-length line", got)
	}
	if got := withNext.ActiveLines(20 * time.Second); len(got) != 1 || got[0].Text != "next" {
		t.Errorf("ActiveLines(20s) = %v, want only the next line", got)
	}

	// 3. Regression: a genuinely bounded line keeps its strict window, and an
	// End == 0 line keeps the plain unbounded rule.
	bounded := &Data{Lines: []LyricLine{
		{Time: 1 * time.Second, End: 2 * time.Second, Text: "bounded"},
		{Time: 5 * time.Second, Text: "unbounded"},
	}}
	if got := bounded.ActiveLines(2 * time.Second); len(got) != 0 {
		t.Errorf("ActiveLines(2s) = %v, want empty (a bounded line expires at End)", got)
	}
	if got := bounded.ActiveLines(5 * time.Second); len(got) != 1 || got[0].Text != "unbounded" {
		t.Errorf("ActiveLines(5s) = %v, want the unbounded line", got)
	}
}
