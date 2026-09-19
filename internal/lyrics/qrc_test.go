package lyrics

import (
	"strings"
	"testing"
	"time"
)

func TestQRC_HeaderDurationBecomesEnd(t *testing.T) {
	const src = "[1000,2000]Hello(1000,500) (1500,500)world\n[3500,2000]Bye(3500,500)\n"
	var p qrcParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2", len(d.Lines))
	}
	if d.Lines[0].Time != 1000*time.Millisecond || d.Lines[0].End != 3000*time.Millisecond {
		t.Errorf("Lines[0] = [%v, %v), want [1000ms, 3000ms)", d.Lines[0].Time, d.Lines[0].End)
	}
	if d.Lines[1].Time != 3500*time.Millisecond || d.Lines[1].End != 5500*time.Millisecond {
		t.Errorf("Lines[1] = [%v, %v), want [3500ms, 5500ms)", d.Lines[1].Time, d.Lines[1].End)
	}
}

func TestQRC_ZeroDurationStaysUnbounded(t *testing.T) {
	const src = "[1000,0]Hello(1000,500)\n"
	var p qrcParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d.Lines[0].End != 0 {
		t.Errorf("End = %v, want 0 (a malformed duration must not invent an end)", d.Lines[0].End)
	}
}

func TestQRC_HeaderMetadata(t *testing.T) {
	const src = "[ti:Title]\n[ar:Artist]\n[al:Album]\n[au:Author]\n[by:Creator]\n[offset:250]\n" +
		"[1000,2000]Hello(1000,500)\n"
	var p qrcParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d.Title != "Title" || d.Artist != "Artist" || d.Album != "Album" || d.Author != "Author" || d.Creator != "Creator" {
		t.Errorf("metadata = %q/%q/%q/%q/%q", d.Title, d.Artist, d.Album, d.Author, d.Creator)
	}
	if d.Offset != 250 {
		t.Errorf("Offset = %d, want 250", d.Offset)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1 (metadata lines are not lyrics)", len(d.Lines))
	}
	if d.Lines[0].Time != 1250*time.Millisecond || d.Lines[0].End != 3250*time.Millisecond {
		t.Errorf("Lines[0] = [%v, %v), want [1250ms, 3250ms)", d.Lines[0].Time, d.Lines[0].End)
	}
	if d.Lines[0].Words[0].Time != 1250*time.Millisecond {
		t.Errorf("Words[0].Time = %v, want 1250ms (offset applies to word times too)", d.Lines[0].Words[0].Time)
	}
}

func TestQRC_TimestampHeaderIsNotMetadata(t *testing.T) {
	// "[1000,2000]" has a comma but no colon, so it must never reach the metadata branch.
	const src = "[1000,2000]Hello(1000,500)\n"
	var p qrcParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d.Title != "" || d.Offset != 0 {
		t.Errorf("Title = %q, Offset = %d, want both empty", d.Title, d.Offset)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
}

func TestQRC_OffsetAfterLyricLineAppliesToEarlierLine(t *testing.T) {
	// The deliberate difference between the two passes: pass 1 reads every metadata
	// header before the lyric lines, so an [offset:] applies to a line even when it
	// stands after that line (LRC's semantics apply it only to what follows).
	const src = "[1000,2000]Hello(1000,500)\n[offset:250]\n"
	var p qrcParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	if d.Lines[0].Time != 1250*time.Millisecond || d.Lines[0].End != 3250*time.Millisecond {
		t.Errorf("Lines[0] = [%v, %v), want [1250ms, 3250ms)", d.Lines[0].Time, d.Lines[0].End)
	}
	if d.Lines[0].Words[0].Time != 1250*time.Millisecond {
		t.Errorf("Words[0].Time = %v, want 1250ms", d.Lines[0].Words[0].Time)
	}
}

func TestQRC_SkipsGarbageAndEmptyBodyLines(t *testing.T) {
	// Covers the skeleton's skip paths: no '[', no ']', a non-integer line header,
	// and an empty body.
	const src = "no bracket here\n" +
		"[1000,2000\n" +
		"[2000,1000]\n" +
		"[notnum,500]Body\n" +
		"[5000,1000]Hi(5000,500)\n"
	var p qrcParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1 (only the valid line survives)", len(d.Lines))
	}
	if d.Lines[0].Time != 5000*time.Millisecond || d.Lines[0].Text != "Hi" {
		t.Errorf("Lines[0] = %v %q, want 5000ms %q", d.Lines[0].Time, d.Lines[0].Text, "Hi")
	}
}

func TestQRC_CRLFLinesKeepTextAndWordsClean(t *testing.T) {
	// A CRLF file's trailing \r must never reach Text/Words (the local form of C2 and
	// C6).
	const src = "[1000,2000]Hello(1000,500)\r\n[3000,2000]Bye(3000,500)\r\n"
	var p qrcParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2", len(d.Lines))
	}
	for i, line := range d.Lines {
		if strings.ContainsAny(line.Text, "\r\n") {
			t.Errorf("Lines[%d].Text = %q, must not contain \\r or \\n", i, line.Text)
		}
		var sb strings.Builder
		for _, w := range line.Words {
			if strings.ContainsAny(w.Text, "\r\n") {
				t.Errorf("Lines[%d].Words has fragment %q with \\r or \\n", i, w.Text)
			}
			sb.WriteString(w.Text)
		}
		if sb.String() != line.Text {
			t.Errorf("Lines[%d].Words %q do not tile Text %q", i, sb.String(), line.Text)
		}
	}
	if d.Lines[0].Text != "Hello" {
		t.Errorf("Lines[0].Text = %q, want %q", d.Lines[0].Text, "Hello")
	}
	if d.Lines[0].End != 3000*time.Millisecond {
		t.Errorf("Lines[0].End = %v, want 3000ms", d.Lines[0].End)
	}
}

func TestQRC_ParseSetsPath(t *testing.T) {
	var p qrcParser
	d, err := p.Parse(strings.NewReader("[1000,2000]Hi(1000,500)\n"), "dir/song.qrc")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d.Path != "dir/song.qrc" {
		t.Errorf("Path = %q, want %q", d.Path, "dir/song.qrc")
	}
}
