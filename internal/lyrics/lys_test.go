package lyrics

import (
	"strings"
	"testing"
	"time"
)

// TestLYS_FirstWordStartingAtZero pins the lineStart fix: the old loop treated
// a first word at 0ms as "not yet set" and took the second word's start as the
// line start.
func TestLYS_FirstWordStartingAtZero(t *testing.T) {
	const src = "[0]Hello(0,500) (500,500)world\n"
	var p lysParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	if d.Lines[0].Time != 0 {
		t.Errorf("Time = %v, want 0", d.Lines[0].Time)
	}
	if d.Lines[0].End != 1000*time.Millisecond {
		t.Errorf("End = %v, want 1000ms", d.Lines[0].End)
	}
}

// TestLYS_EndIsLastWordEnd pins that End is the last word's (start+duration),
// not the line header (LYS has no duration in its header), and that agents come
// from the channel number.
func TestLYS_EndIsLastWordEnd(t *testing.T) {
	const src = "[0]Hello(1000,500) (1500,500)world\n[2]Duet(3000,500)\n"
	var p lysParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2", len(d.Lines))
	}
	if d.Lines[0].Time != 1000*time.Millisecond || d.Lines[0].End != 2000*time.Millisecond {
		t.Errorf("Lines[0] = [%v, %v), want [1000ms, 2000ms)", d.Lines[0].Time, d.Lines[0].End)
	}
	if d.Lines[1].End <= d.Lines[1].Time {
		t.Errorf("Lines[1].End = %v, want > Time %v", d.Lines[1].End, d.Lines[1].Time)
	}
	if d.Lines[0].Agent != "v1" || d.Lines[1].Agent != "v2" {
		t.Errorf("agents = %q/%q, want v1/v2", d.Lines[0].Agent, d.Lines[1].Agent)
	}
}

// TestLYS_CRLFEndToEnd pins that CRLF line endings never leak into Text or
// Words: the single TrimSpace clean point runs before SplitN.
func TestLYS_CRLFEndToEnd(t *testing.T) {
	const src = "[0]Hello(1000,500)\r\n[2]Bye(3000,500)\r\n"
	var p lysParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2", len(d.Lines))
	}
	wantText := []string{"Hello", "Bye"}
	wantAgent := []string{"v1", "v2"}
	for i, line := range d.Lines {
		if strings.Contains(line.Text, "\r") {
			t.Errorf("Lines[%d].Text contains \\r: %q", i, line.Text)
		}
		if line.Text != wantText[i] {
			t.Errorf("Lines[%d].Text = %q, want %q", i, line.Text, wantText[i])
		}
		if line.Agent != wantAgent[i] {
			t.Errorf("Lines[%d].Agent = %q, want %q", i, line.Agent, wantAgent[i])
		}
		var sb strings.Builder
		for _, w := range line.Words {
			if strings.Contains(w.Text, "\r") {
				t.Errorf("Lines[%d].Words contains \\r: %q", i, w.Text)
			}
			sb.WriteString(w.Text)
		}
		if sb.String() != line.Text {
			t.Errorf("Lines[%d].Words %q do not tile Text %q", i, sb.String(), line.Text)
		}
	}
}

// TestLYS_PlainTextWithoutTimings pins the deliberate behaviour change: a body
// with no (start,duration) tuple is kept as a whole line (Time = Start = 0,
// End = 0) instead of being dropped.
func TestLYS_PlainTextWithoutTimings(t *testing.T) {
	const src = "[0]plain text without timings"
	var p lysParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	line := d.Lines[0]
	if line.Text != "plain text without timings" {
		t.Errorf("Text = %q", line.Text)
	}
	if line.Time != 0 {
		t.Errorf("Time = %v, want 0", line.Time)
	}
	if line.End != 0 {
		t.Errorf("End = %v, want 0", line.End)
	}
	var sb strings.Builder
	for _, w := range line.Words {
		sb.WriteString(w.Text)
	}
	if sb.String() != line.Text {
		t.Errorf("Words %q do not tile Text %q", sb.String(), line.Text)
	}
}

// TestLYS_MetadataHeaderConsumed pins the metadata fallback: a "[ti:Title]"
// header is applied and skipped, never parsed as a channel lyric line.
func TestLYS_MetadataHeaderConsumed(t *testing.T) {
	const src = "[ti:My Song]\n[0]Hello(1000,500)\n"
	var p lysParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d.Title != "My Song" {
		t.Errorf("Title = %q, want %q", d.Title, "My Song")
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1 (metadata line must not become a lyric line)", len(d.Lines))
	}
	if d.Lines[0].Text != "Hello" {
		t.Errorf("Lines[0].Text = %q, want %q", d.Lines[0].Text, "Hello")
	}
}

// TestLYS_EmptyBodySkipped pins that an empty/whitespace body is skipped, so
// "[0]" alone never produces a lyric line.
func TestLYS_EmptyBodySkipped(t *testing.T) {
	const src = "[0]\n[0]Hello(1000,500)\n"
	var p lysParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1 (empty body must be skipped)", len(d.Lines))
	}
	if d.Lines[0].Text != "Hello" {
		t.Errorf("Lines[0].Text = %q, want %q", d.Lines[0].Text, "Hello")
	}
}

// TestLYS_UnrecognizedColonHeaderFallsThrough pins the R5 fall-through: a
// "[key:value]" head whose key is not metadata (e.g. "[re:x]") must NOT be
// swallowed as metadata; it falls through to channel parsing and keeps the line.
func TestLYS_UnrecognizedColonHeaderFallsThrough(t *testing.T) {
	const src = "[re:something]Hello(1000,500)\n"
	var p lysParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d.Title != "" {
		t.Errorf("Title = %q, want empty (unrecognised key must not be applied)", d.Title)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	if d.Lines[0].Text != "Hello" {
		t.Errorf("Lines[0].Text = %q, want %q", d.Lines[0].Text, "Hello")
	}
	if d.Lines[0].Agent != "v1" {
		t.Errorf("Lines[0].Agent = %q, want %q (Atoi fails -> channel 0)", d.Lines[0].Agent, "v1")
	}
}
