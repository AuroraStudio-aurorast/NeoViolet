package lyrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// parseSPL runs the parser over an inline sample and fails the test on a parse error;
// a case that must fail calls splParser.Parse itself.
func parseSPL(t *testing.T, src string) *Data {
	t.Helper()
	var p splParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return d
}

// wordsTileText concatenates a line's fragments, which C6 requires to equal the line's
// display text.
func wordsTileText(l LyricLine) string {
	var sb strings.Builder
	for _, w := range l.Words {
		sb.WriteString(w.Text)
	}
	return sb.String()
}

// TestSPL_FindSidecarOnlyReadsSpl pins that ".spl" is this parser's only extension:
// an ".lrc" beside the audio still belongs to lrcParser, so two parsers never claim
// one file.
func TestSPL_FindSidecarOnlyReadsSpl(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.mp3")
	for _, name := range []string{"song.mp3", "song.lrc"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var p splParser
	if got := p.FindSidecar(audio); got != "" {
		t.Errorf("FindSidecar with only an .lrc beside it = %q, want \"\"", got)
	}

	spl := filepath.Join(dir, "song.spl")
	if err := os.WriteFile(spl, []byte("[00:01.00]Hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := p.FindSidecar(audio); got != spl {
		t.Errorf("FindSidecar = %q, want %q", got, spl)
	}
}

// TestSPL_PlainLinesAndMetadata pins the line skeleton: a leading stamp starts a
// line, the body is its text, and a "[key:value]" header fills Data instead of
// becoming a lyric line of its own.
func TestSPL_PlainLinesAndMetadata(t *testing.T) {
	const src = "[ti:My Song]\n" +
		"[ar:Singer]\n" +
		"[al:Album]\n" +
		"[offset:+300]\n" +
		"; a comment line\n" +
		"\n" +
		"[00:01.00]Hello\n" +
		"[00:05.00]   Bye   \n"

	d := parseSPL(t, src)

	if d.Title != "My Song" || d.Artist != "Singer" || d.Album != "Album" {
		t.Errorf("metadata = %q/%q/%q, want My Song/Singer/Album", d.Title, d.Artist, d.Album)
	}
	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2 (headers, comments and blank lines are not lyrics)", len(d.Lines))
	}

	// [offset:+300] precedes both lines, so both shift by 300ms.
	wantTimes := []time.Duration{1300 * time.Millisecond, 5300 * time.Millisecond}
	wantTexts := []string{"Hello", "Bye"}
	for i, line := range d.Lines {
		if line.Time != wantTimes[i] {
			t.Errorf("Lines[%d].Time = %v, want %v", i, line.Time, wantTimes[i])
		}
		if line.Text != wantTexts[i] {
			t.Errorf("Lines[%d].Text = %q, want %q (the body is trimmed)", i, line.Text, wantTexts[i])
		}
		if got := wordsTileText(line); got != line.Text {
			t.Errorf("Lines[%d].Words %q do not tile Text %q", i, got, line.Text)
		}
		if len(line.Words) != 1 || line.Words[0].Time != line.Time {
			t.Errorf("Lines[%d].Words = %+v, want one fragment at the line's own Time", i, line.Words)
		}
	}
}

// TestSPL_RepeatedStamps pins SPL's repeat syntax: adjacent leading stamps carry one
// text between them, so three stamps produce three lines.
func TestSPL_RepeatedStamps(t *testing.T) {
	d := parseSPL(t, "[00:01.00][00:02.00][00:03.00]Again\n")

	if len(d.Lines) != 3 {
		t.Fatalf("len(Lines) = %d, want 3", len(d.Lines))
	}
	for i, line := range d.Lines {
		want := time.Duration(i+1) * time.Second
		if line.Time != want {
			t.Errorf("Lines[%d].Time = %v, want %v", i, line.Time, want)
		}
		if line.Text != "Again" {
			t.Errorf("Lines[%d].Text = %q, want %q", i, line.Text, "Again")
		}
	}
}

// TestSPL_OffsetShiftsLinesAfterIt pins that the offset is read at line-construction
// time, so "[offset:]" shifts only the lines after it, the rule LRC and LYS follow.
func TestSPL_OffsetShiftsLinesAfterIt(t *testing.T) {
	d := parseSPL(t, "[00:01.00]Before\n[offset:250]\n[00:03.00]After\n")

	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2", len(d.Lines))
	}
	if d.Lines[0].Time != time.Second {
		t.Errorf("Lines[0].Time = %v, want 1s (it precedes the offset)", d.Lines[0].Time)
	}
	if d.Lines[1].Time != 3250*time.Millisecond {
		t.Errorf("Lines[1].Time = %v, want 3250ms", d.Lines[1].Time)
	}
	if got := d.Lines[1].Words[0].Time; got != 3250*time.Millisecond {
		t.Errorf("Lines[1].Words[0].Time = %v, want 3250ms (fragments shift with their line)", got)
	}
}

// TestSPL_MalformedLinesDropped pins that a line with no usable stamp is dropped
// rather than kept as text: a bare text line, an unrecognised colon header and a
// bracket that is not a stamp.
func TestSPL_MalformedLinesDropped(t *testing.T) {
	const src = "this line has no timestamp\n" +
		"[re:a tool tag]\n" +
		"[1000,2000]a QRC-shaped header\n" +
		"[abc]not a stamp\n" +
		"[00:01.00]Good\n"

	d := parseSPL(t, src)

	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1: %+v", len(d.Lines), d.Lines)
	}
	if d.Lines[0].Text != "Good" {
		t.Errorf("Lines[0].Text = %q, want %q", d.Lines[0].Text, "Good")
	}
}

// TestSPL_OutOfOrderLinesAreSorted pins that the file's order is not the display order:
// the panel walks Lines by time.
func TestSPL_OutOfOrderLinesAreSorted(t *testing.T) {
	d := parseSPL(t, "[00:03.00]Third\n[00:01.00]First\n[00:02.00]Second\n")

	want := []string{"First", "Second", "Third"}
	if len(d.Lines) != len(want) {
		t.Fatalf("len(Lines) = %d, want %d", len(d.Lines), len(want))
	}
	for i, line := range d.Lines {
		if line.Text != want[i] {
			t.Errorf("Lines[%d].Text = %q, want %q", i, line.Text, want[i])
		}
	}
}

// TestSPL_CRLFNeverLeaks pins that the one TrimSpace per line is the only clean point,
// so a CRLF file leaves no \r in Text or in a fragment.
func TestSPL_CRLFNeverLeaks(t *testing.T) {
	d := parseSPL(t, "[00:01.00]Hello\r\n[00:05.00]Bye\r\n")

	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2", len(d.Lines))
	}
	for i, line := range d.Lines {
		if strings.Contains(line.Text, "\r") {
			t.Errorf("Lines[%d].Text = %q, want no \\r", i, line.Text)
		}
		for _, w := range line.Words {
			if strings.Contains(w.Text, "\r") {
				t.Errorf("Lines[%d].Words contains \\r: %q", i, w.Text)
			}
		}
	}
}

// TestSPL_NoLyricLinesIsAnError pins the fall-through contract with the registry: a
// file that yields no displayable line reports an error, so FindAndParse moves on to
// the next preferred format.
func TestSPL_NoLyricLinesIsAnError(t *testing.T) {
	var p splParser
	for _, src := range []string{"", "; only a comment\n", "[ti:Title only]\n[00:01.00]\n"} {
		if d, err := p.Parse(strings.NewReader(src), ""); err == nil {
			t.Errorf("Parse(%q) = %+v, want an error", src, d)
		}
	}
}

// TestSPL_StampDigitGrammar pins the standard's per-field digit limits and its
// padding rule: the fraction is a string of digits, so a short one is padded on the
// right instead of being read as a count of milliseconds.
func TestSPL_StampDigitGrammar(t *testing.T) {
	tests := []struct {
		stamp string
		want  time.Duration
		valid bool
	}{
		// The standard's own correct forms.
		{"103:3.405", 103*time.Minute + 3*time.Second + 405*time.Millisecond, true},
		{"3:12.5", 3*time.Minute + 12*time.Second + 500*time.Millisecond, true},
		// "1" is 100ms and "02" is 20ms, not 1ms and 2ms.
		{"3:12.1", 3*time.Minute + 12*time.Second + 100*time.Millisecond, true},
		{"3:12.02", 3*time.Minute + 12*time.Second + 20*time.Millisecond, true},
		// Four to six digits carry sub-millisecond precision.
		{"3:12.450000", 3*time.Minute + 12*time.Second + 450*time.Millisecond, true},
		{"3:12.0004", 3*time.Minute + 12*time.Second + 400*time.Microsecond, true},
		// Shortest and longest legal width of each field, and a missing fraction.
		{"1:1", time.Minute + time.Second, true},
		{"02:02", 2*time.Minute + 2*time.Second, true},
		{"999:99", 999*time.Minute + 99*time.Second, true},
		{"3:12", 3*time.Minute + 12*time.Second, true},

		// The standard's own wrong forms, plus one field over its limit each.
		{"3:102.5", 0, false},
		{"1234:1.0", 0, false},
		{"3:12.1234567", 0, false},
		{"3:12.", 0, false},
		{"3:12.1.2", 0, false},
		{"3:12:5", 0, false},
		{"3:", 0, false},
		{"3", 0, false},
		{"", 0, false},
		{"a:b", 0, false},
	}

	for _, tc := range tests {
		got, err := parseSPLStamp(tc.stamp)
		if !tc.valid {
			if err == nil {
				t.Errorf("parseSPLStamp(%q) = %v, want an error", tc.stamp, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSPLStamp(%q) failed: %v", tc.stamp, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseSPLStamp(%q) = %v, want %v", tc.stamp, got, tc.want)
		}
	}
}

// TestSPL_StampGrammarIsEnforcedAtParse pins that the grammar reaches the line loop
// and not just the helper: the standard's wrong writings are dropped, so a file made
// of them reports no lyrics instead of rendering at a garbage time.
func TestSPL_StampGrammarIsEnforcedAtParse(t *testing.T) {
	const src = "(103:3.405)parentheses are not brackets\n" +
		"[3:102.5]seconds out of range\n" +
		"[1234:1.0]minutes out of range\n" +
		"[3:12.1234567]fraction out of range\n" +
		"[3:12.5]Good\n"

	d := parseSPL(t, src)

	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1: %+v", len(d.Lines), d.Lines)
	}
	if d.Lines[0].Text != "Good" || d.Lines[0].Time != 3*time.Minute+12*time.Second+500*time.Millisecond {
		t.Errorf("Lines[0] = %q @ %v, want Good @ 3m12.5s", d.Lines[0].Text, d.Lines[0].Time)
	}
}

// TestSPL_OutOfGrammarStampInBodyStaysText pins what happens to a stamp that is
// inside a line but outside the grammar: it stays text. The standard defines no
// recovery and deleting the bytes would drop part of the user's line; the repeat loop
// stops at it too, so the line is not repeated either.
func TestSPL_OutOfGrammarStampInBodyStaysText(t *testing.T) {
	d := parseSPL(t, "[00:01.00][3:102.5]txt\n")

	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1: %+v", len(d.Lines), d.Lines)
	}
	if d.Lines[0].Text != "[3:102.5]txt" {
		t.Errorf("Lines[0].Text = %q, want the bracket kept as text", d.Lines[0].Text)
	}
	if d.Lines[0].Time != time.Second {
		t.Errorf("Lines[0].Time = %v, want 1s", d.Lines[0].Time)
	}
}

// TestSPL_WordMarkers pins the standard's word-by-word example:
//
//	[05:20.22]Hello[05:23.22]World[05:24.22]
//
// Each marker starts the text after it, so the stamp at the end of the line is both
// the final word's start and the line's end.
func TestSPL_WordMarkers(t *testing.T) {
	d := parseSPL(t, "[05:20.22]Hello[05:23.22]World[05:24.22]\n")

	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	line := d.Lines[0]
	want := []WordFragment{
		{Time: 5*time.Minute + 20*time.Second + 220*time.Millisecond, Text: "Hello"},
		{Time: 5*time.Minute + 23*time.Second + 220*time.Millisecond, Text: "World"},
	}
	if len(line.Words) != len(want) {
		t.Fatalf("Words = %+v, want %+v", line.Words, want)
	}
	for i, w := range want {
		if line.Words[i] != w {
			t.Errorf("Words[%d] = %+v, want %+v", i, line.Words[i], w)
		}
	}
	if line.Text != "HelloWorld" {
		t.Errorf("Text = %q, want the markers stripped", line.Text)
	}
	if got := wordsTileText(line); got != line.Text {
		t.Errorf("Words %q do not tile Text %q", got, line.Text)
	}
	if wantEnd := 5*time.Minute + 24*time.Second + 220*time.Millisecond; line.End != wantEnd {
		t.Errorf("End = %v, want %v", line.End, wantEnd)
	}
}

// TestSPL_ExplicitLineEndInline pins the standard's inline end marker: a stamp after
// the last text says where the line stops, so it does not run until the next one.
func TestSPL_ExplicitLineEndInline(t *testing.T) {
	d := parseSPL(t, "[05:20.22]Hello World[05:21.22]\n")

	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	line := d.Lines[0]
	if line.End-line.Time != time.Second {
		t.Errorf("line lasts %v, want the 1s the standard spells out", line.End-line.Time)
	}
	if len(line.Words) != 1 || line.Words[0].Text != "Hello World" {
		t.Errorf("Words = %+v, want one fragment for the whole line", line.Words)
	}
}

// TestSPL_ExplicitLineEndOnItsOwnLine pins the standard's separate end-marker line,
// including the case it calls out: sharing the next line's stamp is not a conflict.
func TestSPL_ExplicitLineEndOnItsOwnLine(t *testing.T) {
	d := parseSPL(t, "[05:20.22]Hello World\n[05:21.22]\n[05:21.22]Good day\n")

	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2 (an end marker is not a lyric)", len(d.Lines))
	}
	if got := d.Lines[0].End; got != 5*time.Minute+21*time.Second+220*time.Millisecond {
		t.Errorf("Lines[0].End = %v, want the marker's stamp", got)
	}
	if d.Lines[1].Text != "Good day" || d.Lines[1].End != 0 {
		t.Errorf("Lines[1] = %q ending %v, want \"Good day\" with no end", d.Lines[1].Text, d.Lines[1].End)
	}
}

// TestSPL_ImplicitLineEndIsNotStored pins the other half of the standard's line
// endings: the window to the next line is derived at display time, so writing it into
// End would invent a bound the file never stated — and C5 forbids an End not past
// Time.
func TestSPL_ImplicitLineEndIsNotStored(t *testing.T) {
	d := parseSPL(t, "[00:01.00]A\n[00:05.00]B\n")

	if d.Lines[0].End != 0 {
		t.Errorf("Lines[0].End = %v, want 0 (unbounded)", d.Lines[0].End)
	}
	if active := d.ActiveLines(2 * time.Second); len(active) != 1 || active[0].Text != "A" {
		t.Errorf("ActiveLines(2s) = %+v, want A: it lasts until B starts", active)
	}
	if active := d.ActiveLines(5 * time.Second); len(active) != 1 || active[0].Text != "B" {
		t.Errorf("ActiveLines(5s) = %+v, want B", active)
	}
}

// TestSPL_IgnoredWordMarkers pins the standard's recovery rule: a marker that is not
// past the previous one, or not inside the line, is ignored and the text it was meant
// to start stays where it was.
func TestSPL_IgnoredWordMarkers(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []WordFragment
	}{
		{
			// Not increasing: it is not past the line's own start.
			"marker at the line start",
			"[00:05.00]a[00:05.00]b\n",
			[]WordFragment{{Time: 5 * time.Second, Text: "ab"}},
		},
		{
			// Outside the line: it would start before the line did.
			"marker before the line start",
			"[00:05.00]a[00:03.00]b\n",
			[]WordFragment{{Time: 5 * time.Second, Text: "ab"}},
		},
		{
			// Not increasing: it would start before the previous marker did.
			"marker before the previous marker",
			"[00:01.00]a[00:03.00]b[00:02.00]c\n",
			[]WordFragment{
				{Time: time.Second, Text: "a"},
				{Time: 3 * time.Second, Text: "bc"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := parseSPL(t, tc.src)
			if len(d.Lines) != 1 {
				t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
			}
			got := d.Lines[0].Words
			if len(got) != len(tc.want) {
				t.Fatalf("Words = %+v, want %+v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("Words[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestSPL_WordMarkersAndRepeatLines pins the limitation the standard documents: the
// markers are read against each line's own start, so a later repeat cannot use them
// and falls back to one fragment for its whole text.
func TestSPL_WordMarkersAndRepeatLines(t *testing.T) {
	d := parseSPL(t, "[05:20.22][05:30.22]Hello[05:23.22]World[05:24.22]\n")

	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2", len(d.Lines))
	}
	first, second := d.Lines[0], d.Lines[1]
	if len(first.Words) != 2 || first.Words[1].Text != "World" {
		t.Errorf("Lines[0].Words = %+v, want the markers honoured", first.Words)
	}
	if first.End != 5*time.Minute+24*time.Second+220*time.Millisecond {
		t.Errorf("Lines[0].End = %v, want the trailing marker", first.End)
	}
	want := WordFragment{Time: 5*time.Minute + 30*time.Second + 220*time.Millisecond, Text: "HelloWorld"}
	if len(second.Words) != 1 || second.Words[0] != want {
		t.Errorf("Lines[1].Words = %+v, want %+v", second.Words, want)
	}
	if second.End != 0 {
		t.Errorf("Lines[1].End = %v, want 0: every marker precedes it", second.End)
	}
}

// TestSPL_EndMarkerThatIsNotAnEndIsIgnored pins that an end is stored only when it
// can be one: an end at or before the line's start would leave a line that never
// becomes active.
func TestSPL_EndMarkerThatIsNotAnEndIsIgnored(t *testing.T) {
	d := parseSPL(t, "[00:05.00]text[00:03.00]\n")

	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	if d.Lines[0].End != 0 {
		t.Errorf("End = %v, want 0 for a marker before the line started", d.Lines[0].End)
	}
	if active := d.ActiveLines(5 * time.Second); len(active) != 1 {
		t.Errorf("ActiveLines(5s) = %+v, want the line to be active", active)
	}
}

// TestSPL_OffsetShiftsTheLineEnd pins that an end marker is shifted like the line it
// ends, so the line keeps the duration the file gave it.
func TestSPL_OffsetShiftsTheLineEnd(t *testing.T) {
	d := parseSPL(t, "[offset:+300]\n[00:01.00]A[00:02.00]\n[00:05.00]B\n[00:06.00]\n")

	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2", len(d.Lines))
	}
	for i, want := range []struct{ at, end time.Duration }{
		{1300 * time.Millisecond, 2300 * time.Millisecond},
		{5300 * time.Millisecond, 6300 * time.Millisecond},
	} {
		if d.Lines[i].Time != want.at || d.Lines[i].End != want.end {
			t.Errorf("Lines[%d] = %v..%v, want %v..%v", i, d.Lines[i].Time, d.Lines[i].End, want.at, want.end)
		}
	}
}

// TestSPL_AngleBracketSugarMatchesThePlainForm pins the equivalence the standard
// states: the brackets say where a marker may appear, not what a marker means.
func TestSPL_AngleBracketSugarMatchesThePlainForm(t *testing.T) {
	plain := parseSPL(t, "[05:20.22]Hello[05:23.22]World[05:24.22]\n").Lines
	sugar := parseSPL(t, "[05:20.22]Hello<05:23.22>World[05:24.22]\n").Lines

	if len(plain) != 1 || len(sugar) != 1 {
		t.Fatalf("lines = %d plain, %d sugar, want 1 each", len(plain), len(sugar))
	}
	if plain[0].Time != sugar[0].Time || plain[0].End != sugar[0].End || plain[0].Text != sugar[0].Text {
		t.Errorf("sugar = %v..%v %q, want the plain form %v..%v %q",
			sugar[0].Time, sugar[0].End, sugar[0].Text, plain[0].Time, plain[0].End, plain[0].Text)
	}
	if len(plain[0].Words) != len(sugar[0].Words) {
		t.Fatalf("Words = %+v sugar, %+v plain", sugar[0].Words, plain[0].Words)
	}
	for i := range plain[0].Words {
		if plain[0].Words[i] != sugar[0].Words[i] {
			t.Errorf("Words[%d] = %+v, want %+v", i, sugar[0].Words[i], plain[0].Words[i])
		}
	}
}

// TestSPL_DelayedFirstWord pins what the angle brackets exist for: the row arrives
// at one time, its first word starts at a later one, and the row is current for the
// whole gap with nothing on it lit yet.
func TestSPL_DelayedFirstWord(t *testing.T) {
	d := parseSPL(t, "[05:20.22]<05:21.22>Hello<05:23.22>World[05:24.22]\n")

	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1: an angle marker must not read as a repeat", len(d.Lines))
	}
	line := d.Lines[0]
	want := []WordFragment{
		{Time: 5*time.Minute + 21*time.Second + 220*time.Millisecond, Text: "Hello"},
		{Time: 5*time.Minute + 23*time.Second + 220*time.Millisecond, Text: "World"},
	}
	if len(line.Words) != len(want) {
		t.Fatalf("Words = %+v, want %+v", line.Words, want)
	}
	for i, w := range want {
		if line.Words[i] != w {
			t.Errorf("Words[%d] = %+v, want %+v", i, line.Words[i], w)
		}
	}
	if got, want := line.Text, "HelloWorld"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
	if got, want := line.End, 5*time.Minute+24*time.Second+220*time.Millisecond; got != want {
		t.Errorf("End = %v, want %v", got, want)
	}

	// A half second into the row the first word has not started, so the line is
	// active while its word markers are all still ahead of the clock.
	elapsed := 5*time.Minute + 20*time.Second + 720*time.Millisecond
	active := d.ActiveLines(elapsed)
	if len(active) != 1 || active[0].Text != "HelloWorld" {
		t.Fatalf("ActiveLines(%v) = %+v, want the row to be current", elapsed, active)
	}
	if active[0].Words[0].Time <= elapsed {
		t.Errorf("Words[0].Time = %v, want a word that has not started at %v", active[0].Words[0].Time, elapsed)
	}
}

// TestSPL_AngleBracketEndMarker pins that the end marker may use angle brackets too,
// since it is not the line's own leading stamp either.
func TestSPL_AngleBracketEndMarker(t *testing.T) {
	d := parseSPL(t, "[00:01.00]Hello<00:02.00>\n")

	line := d.Lines[0]
	if line.End != 2*time.Second {
		t.Errorf("End = %v, want 2s", line.End)
	}
	if line.Text != "Hello" {
		t.Errorf("Text = %q, want the marker cut out", line.Text)
	}
	if len(line.Words) != 1 || line.Words[0].Time != time.Second {
		t.Errorf("Words = %+v, want one fragment at the line's own start", line.Words)
	}
}

// TestSPL_IgnoredAngleBracketMarkers pins that the sugar changes the brackets and
// nothing else: an angle marker is ignored under the same rules as a square one.
func TestSPL_IgnoredAngleBracketMarkers(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []WordFragment
	}{
		{"before the line start", "[00:05.00]a<00:03.00>b\n", []WordFragment{{Time: 5 * time.Second, Text: "ab"}}},
		{"at the line start", "[00:05.00]a<00:05.00>b\n", []WordFragment{{Time: 5 * time.Second, Text: "ab"}}},
		{"before the previous marker", "[00:01.00]a<00:03.00>b<00:02.00>c\n", []WordFragment{
			{Time: time.Second, Text: "a"},
			{Time: 3 * time.Second, Text: "bc"},
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := parseSPL(t, tc.src)
			if len(d.Lines) != 1 {
				t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
			}
			got := d.Lines[0].Words
			if len(got) != len(tc.want) {
				t.Fatalf("Words = %+v, want %+v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("Words[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}
