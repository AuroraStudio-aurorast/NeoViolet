package lyrics

import (
	"strings"
	"testing"
	"time"
)

func TestYRC_HeaderDurationAndMetadata(t *testing.T) {
	const src = "[ti:Title]\n[ar:Artist]\n[offset:100]\n[1000,2000](1000,500,0)Hello(1500,500,0) world\n"
	var p yrcParser
	d, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d.Title != "Title" || d.Artist != "Artist" || d.Offset != 100 {
		t.Errorf("metadata = %q/%q offset=%d", d.Title, d.Artist, d.Offset)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	line := d.Lines[0]
	if line.Time != 1100*time.Millisecond || line.End != 3100*time.Millisecond {
		t.Errorf("Lines[0] = [%v, %v), want [1100ms, 3100ms)", line.Time, line.End)
	}
	if line.Text != "Hello world" {
		t.Errorf("Text = %q, want %q", line.Text, "Hello world")
	}
}

func TestYRC_ParseSetsPath(t *testing.T) {
	var p yrcParser
	d, err := p.Parse(strings.NewReader("[1000,2000](1000,500,0)Hi\n"), "dir/song.yrc")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d.Path != "dir/song.yrc" {
		t.Errorf("Path = %q, want %q", d.Path, "dir/song.yrc")
	}
}
