package lyrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// parseSPL runs the SPL parser over an inline sample. It fails the test on any
// parse error, so a case that must fail parses through splParser.Parse instead.
func parseSPL(t *testing.T, src string) *Data {
	t.Helper()
	var p splParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return d
}

// wordsTileText concatenates a line's fragments. The contract's C6 requires the
// result to equal the line's display text, so an SPL line whose words do not
// tile silently loses its word-by-word highlighting.
func wordsTileText(l LyricLine) string {
	var sb strings.Builder
	for _, w := range l.Words {
		sb.WriteString(w.Text)
	}
	return sb.String()
}

// TestSPL_FindSidecarOnlyReadsSpl pins the registration decision behind this
// parser: ".spl" is SPL's own extension, and an ".lrc" file beside the audio
// still belongs to lrcParser. Falling back to ".lrc" here would make two
// parsers claim one file and let parse order decide which reading wins.
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
// line, the body is the line's text, and a "[key:value]" header is applied to
// Data instead of turning into a lyric line of its own.
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

// TestSPL_RepeatedStamps pins SPL's repeat syntax: adjacent leading stamps carry
// one text between them, so three stamps produce three lines with the same text.
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

// TestSPL_OffsetShiftsLinesAfterIt pins that delta is read at line-construction
// time, so "[offset:]" shifts only the lines parsed after it — the rule LRC and
// LYS follow (QRC and YRC instead read every header in a first pass and shift the
// whole file, because SPL writes its headers inline among the lyrics).
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
// rather than kept as literal text: a bare text line, an unrecognised colon
// header and a bracket that is not a stamp at all.
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

// TestSPL_OutOfOrderLinesAreSorted pins that the file's order is not the display
// order: the panel walks Lines by time.
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

// TestSPL_CRLFNeverLeaks pins that the single TrimSpace per line is the only
// clean point, so a CRLF file leaves no \r in Text, Words or a fragment.
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

// TestSPL_NoLyricLinesIsAnError pins the fall-through contract with the registry:
// a file that yields no displayable line reports an error so FindAndParse moves
// on to the next preferred format instead of stopping on an empty file.
func TestSPL_NoLyricLinesIsAnError(t *testing.T) {
	var p splParser
	for _, src := range []string{"", "; only a comment\n", "[ti:Title only]\n[00:01.00]\n"} {
		if d, err := p.Parse(strings.NewReader(src), ""); err == nil {
			t.Errorf("Parse(%q) = %+v, want an error", src, d)
		}
	}
}
