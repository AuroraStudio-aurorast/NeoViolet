package lyrics

import (
	"strings"
	"testing"
	"time"
)

const testSRT = `1
00:00:01,000 --> 00:00:04,000
First subtitle line

2
00:00:05,000 --> 00:00:08,500
Second subtitle text
Second line continues

3
00:00:10,000 --> 00:00:12,000
Third entry with
multiple
lines`

const testSRTDot = `1
00:00:01.500 --> 00:00:03.000
Dot-style timestamps`

func parseSRT(s string) (*LyricsData, error) {
	var p srtParser
	return p.Parse(strings.NewReader(s), "")
}

func TestSRT_BasicParse(t *testing.T) {
	d, err := parseSRT(testSRT)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(d.Lines))
	}

	// First entry: 00:00:01,000 --> 00:00:04,000
	if d.Lines[0].Time != 1*time.Second {
		t.Errorf("line 0 Time = %v, want 1s", d.Lines[0].Time)
	}
	if d.Lines[0].End != 4*time.Second {
		t.Errorf("line 0 End = %v, want 4s", d.Lines[0].End)
	}
	if d.Lines[0].Text != "First subtitle line" {
		t.Errorf("line 0 Text = %q, want 'First subtitle line'", d.Lines[0].Text)
	}

	// Second entry: 00:00:05,000 --> 00:00:08,500
	if d.Lines[1].Time != 5*time.Second {
		t.Errorf("line 1 Time = %v, want 5s", d.Lines[1].Time)
	}
	if d.Lines[1].End != 8500*time.Millisecond {
		t.Errorf("line 1 End = %v, want 8500ms", d.Lines[1].End)
	}
	if d.Lines[1].Text != "Second subtitle text\nSecond line continues" {
		t.Errorf("line 1 Text = %q", d.Lines[1].Text)
	}

	// Third entry: 00:00:10,000 --> 00:00:12,000 with multi-line text
	if d.Lines[2].Time != 10*time.Second {
		t.Errorf("line 2 Time = %v, want 10s", d.Lines[2].Time)
	}
	if d.Lines[2].End != 12*time.Second {
		t.Errorf("line 2 End = %v, want 12s", d.Lines[2].End)
	}
	expectedMulti := "Third entry with\nmultiple\nlines"
	if d.Lines[2].Text != expectedMulti {
		t.Errorf("line 2 Text = %q, want %q", d.Lines[2].Text, expectedMulti)
	}
}

func TestSRT_DotTimestamps(t *testing.T) {
	d, err := parseSRT(testSRTDot)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(d.Lines))
	}

	if d.Lines[0].Time != 1500*time.Millisecond {
		t.Errorf("Time = %v, want 1500ms", d.Lines[0].Time)
	}
	if d.Lines[0].End != 3*time.Second {
		t.Errorf("End = %v, want 3s", d.Lines[0].End)
	}
	if d.Lines[0].Text != "Dot-style timestamps" {
		t.Errorf("Text = %q", d.Lines[0].Text)
	}
}

func TestSRT_EmptyInput(t *testing.T) {
	_, err := parseSRT("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestSRT_SortedOutput(t *testing.T) {
	unsorted := `1
00:00:20,000 --> 00:00:25,000
later entry

2
00:00:10,000 --> 00:00:15,000
earlier entry`

	d, err := parseSRT(unsorted)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(d.Lines))
	}
	for i := 1; i < len(d.Lines); i++ {
		if d.Lines[i].Time < d.Lines[i-1].Time {
			t.Errorf("lines not sorted at index %d", i)
		}
	}
}

func TestSRT_Sidecar(t *testing.T) {
	ext := ".mp3"
	path := "/some/path/song.mp3"
	expected := path[:len(path)-len(ext)] + ".srt"
	if expected != "/some/path/song.srt" {
		t.Errorf("srt path = %q, want /some/path/song.srt", expected)
	}
}

func TestSRT_CurrentLine(t *testing.T) {
	d, err := parseSRT(testSRT)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if d.CurrentLine(0) != -1 {
		t.Error("should return -1 before first line (Time=1s)")
	}
	if d.CurrentLine(500*time.Millisecond) != -1 {
		t.Error("should return -1 at 500ms")
	}
	if d.CurrentLine(1*time.Second) != 0 {
		t.Error("should return 0 at 1s")
	}
	if d.CurrentLine(3*time.Second) != 0 {
		t.Error("should return 0 at 3s")
	}
	if d.CurrentLine(5*time.Second) != 1 {
		t.Error("should return 1 at 5s")
	}
	if d.CurrentLine(15*time.Second) != 2 {
		t.Error("should return 2 at 15s")
	}
}

func TestSRT_ActiveLines(t *testing.T) {
	d, err := parseSRT(testSRT)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Before first entry
	active := d.ActiveLines(500 * time.Millisecond)
	if len(active) != 0 {
		t.Errorf("at 0.5s: expected 0 active, got %d", len(active))
	}

	// First entry active (1s - 4s)
	active = d.ActiveLines(2 * time.Second)
	if len(active) != 1 {
		t.Fatalf("at 2s: expected 1 active, got %d", len(active))
	}
	if active[0].Text != "First subtitle line" {
		t.Errorf("at 2s: text = %q", active[0].Text)
	}

	// Between first and second entry (4s - 5s)
	active = d.ActiveLines(4500 * time.Millisecond)
	if len(active) != 0 {
		t.Errorf("at 4.5s: expected 0 active, got %d", len(active))
	}

	// Second entry active (5s - 8.5s)
	active = d.ActiveLines(6 * time.Second)
	if len(active) != 1 {
		t.Fatalf("at 6s: expected 1 active, got %d", len(active))
	}
	if active[0].Text != "Second subtitle text\nSecond line continues" {
		t.Errorf("at 6s: text = %q", active[0].Text)
	}

	// Past last entry (End=12s)
	active = d.ActiveLines(15 * time.Second)
	if len(active) != 0 {
		t.Errorf("at 15s: expected 0 active, got %d", len(active))
	}
}

func TestSRT_InvalidIndex(t *testing.T) {
	input := `notanumber
00:00:01,000 --> 00:00:04,000
some text

1
00:00:05,000 --> 00:00:08,000
valid entry`

	d, err := parseSRT(input)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("expected 1 line (skip invalid), got %d", len(d.Lines))
	}
	if d.Lines[0].Time != 5*time.Second {
		t.Errorf("Time = %v, want 5s", d.Lines[0].Time)
	}
}

func TestSRT_OnlyWhitespaceText(t *testing.T) {
	input := `1
00:00:01,000 --> 00:00:04,000


2
00:00:05,000 --> 00:00:08,000
real text`

	d, err := parseSRT(input)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(d.Lines))
	}
	if d.Lines[0].Text != "real text" {
		t.Errorf("Text = %q", d.Lines[0].Text)
	}
}

func TestSRT_WindowsLineEndings(t *testing.T) {
	input := "1\r\n00:00:01,000 --> 00:00:04,000\r\ntext\r\n\r\n2\r\n00:00:05,000 --> 00:00:08,000\r\nmore text"
	d, err := parseSRT(input)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(d.Lines))
	}
	if d.Lines[0].Time != 1*time.Second {
		t.Errorf("line 0 Time = %v, want 1s", d.Lines[0].Time)
	}
	if d.Lines[1].Time != 5*time.Second {
		t.Errorf("line 1 Time = %v, want 5s", d.Lines[1].Time)
	}
}

func TestSRT_FindSidecar(t *testing.T) {
	ext := ".mp3"
	path := "/some/path/song.mp3"
	expected := path[:len(path)-len(ext)] + ".srt"
	if expected != "/some/path/song.srt" {
		t.Errorf("srt path = %q", expected)
	}
}

func TestSRT_EntryWithEmptyTextLines(t *testing.T) {
	input := `1
00:00:01,000 --> 00:00:04,000
some text

2
00:00:05,000 --> 00:00:08,000
`

	d, err := parseSRT(input)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(d.Lines))
	}
	if d.Lines[0].Text != "some text" {
		t.Errorf("Text = %q, want 'some text'", d.Lines[0].Text)
	}
}

func TestSRT_MultipleBlankLines(t *testing.T) {
	input := `1
00:00:01,000 --> 00:00:04,000
first text


2
00:00:05,000 --> 00:00:08,000
second text`

	d, err := parseSRT(input)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(d.Lines))
	}
	if d.Lines[0].Text != "first text" {
		t.Errorf("line 0 Text = %q", d.Lines[0].Text)
	}
	if d.Lines[1].Text != "second text" {
		t.Errorf("line 1 Text = %q", d.Lines[1].Text)
	}
}