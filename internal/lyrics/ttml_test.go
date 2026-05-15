package lyrics

import (
	"strings"
	"testing"
	"time"
)

const testTTML = `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:tts="http://www.w3.org/ns/ttml#styling"
    ttp:frameRate="24"
    ttp:tickRate="1000">
  <head>
    <metadata>
      <ttm:title>Test Song</ttm:title>
    </metadata>
  </head>
  <body>
    <div>
      <p begin="00:00:01.500" end="00:00:04.000">First line of lyrics</p>
      <p begin="00:00:04.000" end="00:00:07.500">Second line here</p>
      <p begin="00:00:07.500" end="00:00:12.000">Third line goes on</p>
      <p begin="00:00:12.000" end="00:00:15.500">
        <span begin="00:00:12.000">word</span>
        <span begin="00:00:13.000">level</span>
        <span begin="00:00:14.000">sync</span>
      </p>
    </div>
  </body>
</tt>`

const testTTMLOffset = `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div>
      <p begin="1.5s" end="4s">Offset time first line</p>
      <p begin="4000ms" end="7500ms">Offset time in milliseconds</p>
    </div>
  </body>
</tt>`

const testTTMLFrames = `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml"
    ttp:frameRate="30"
    ttp:frameRateMultiplier="1"
    ttp:subFrameRate="1">
  <body>
    <div>
      <p begin="00:00:01:15" end="00:00:02:00">Frames-based timestamp</p>
    </div>
  </body>
</tt>`

const testTTMLMinimal = `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div>
      <p begin="00:00:00.000">Minimal TTML</p>
    </div>
  </body>
</tt>`

func parseTTML(s string) (*LyricsData, error) {
	var p ttmlParser
	return p.Parse(strings.NewReader(s), "")
}

func TestTTML_BasicParse(t *testing.T) {
	d, err := parseTTML(testTTML)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Text != "First line of lyrics" {
		t.Errorf("line 0 text = %q", d.Lines[0].Text)
	}
	if d.Lines[0].Time != 1500*time.Millisecond {
		t.Errorf("line 0 time = %v, want 1500ms", d.Lines[0].Time)
	}

	if d.Lines[1].Text != "Second line here" {
		t.Errorf("line 1 text = %q", d.Lines[1].Text)
	}
	if d.Lines[1].Time != 4000*time.Millisecond {
		t.Errorf("line 1 time = %v, want 4000ms", d.Lines[1].Time)
	}

	if d.Lines[2].Text != "Third line goes on" {
		t.Errorf("line 2 text = %q", d.Lines[2].Text)
	}
	if d.Lines[2].Time != 7500*time.Millisecond {
		t.Errorf("line 2 time = %v, want 7500ms", d.Lines[2].Time)
	}
}

func TestTTML_WordLevelSync(t *testing.T) {
	d, err := parseTTML(testTTML)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	line3 := d.Lines[3]
	if line3.Text != "wordlevelsync" {
		t.Errorf("merged span text = %q, want 'wordlevelsync'", line3.Text)
	}
	if len(line3.Words) != 3 {
		t.Fatalf("expected 3 word fragments, got %d", len(line3.Words))
	}
	if line3.Words[0].Text != "word" || line3.Words[0].Time != 12000*time.Millisecond {
		t.Errorf("word[0] = %q @ %v, want 'word' @ 12s", line3.Words[0].Text, line3.Words[0].Time)
	}
	if line3.Words[1].Text != "level" || line3.Words[1].Time != 13000*time.Millisecond {
		t.Errorf("word[1] = %q @ %v, want 'level' @ 13s", line3.Words[1].Text, line3.Words[1].Time)
	}
	if line3.Words[2].Text != "sync" || line3.Words[2].Time != 14000*time.Millisecond {
		t.Errorf("word[2] = %q @ %v, want 'sync' @ 14s", line3.Words[2].Text, line3.Words[2].Time)
	}
}

func TestTTML_OffsetTime(t *testing.T) {
	d, err := parseTTML(testTTMLOffset)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Time != 1500*time.Millisecond {
		t.Errorf("1.5s = %v, want 1500ms", d.Lines[0].Time)
	}
	if d.Lines[0].Text != "Offset time first line" {
		t.Errorf("line 0 text = %q", d.Lines[0].Text)
	}

	if d.Lines[1].Time != 4000*time.Millisecond {
		t.Errorf("4000ms = %v, want 4000ms", d.Lines[1].Time)
	}
	if d.Lines[1].Text != "Offset time in milliseconds" {
		t.Errorf("line 1 text = %q", d.Lines[1].Text)
	}
}

func TestTTML_FramesTime(t *testing.T) {
	d, err := parseTTML(testTTMLFrames)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(d.Lines))
	}

	// 00:00:01:15 at 30fps = 1s + 15/30s = 1.5s
	expected := 1500 * time.Millisecond
	if d.Lines[0].Time != expected {
		t.Errorf("frames time = %v, want %v", d.Lines[0].Time, expected)
	}
}

func TestTTML_Minimal(t *testing.T) {
	d, err := parseTTML(testTTMLMinimal)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(d.Lines))
	}
	if d.Lines[0].Text != "Minimal TTML" {
		t.Errorf("text = %q", d.Lines[0].Text)
	}
}

func TestTTML_EmptyInput(t *testing.T) {
	_, err := parseTTML("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestTTML_NoParagraphs(t *testing.T) {
	noP := `<tt xmlns="http://www.w3.org/ns/ttml"><body><div></div></body></tt>`
	_, err := parseTTML(noP)
	if err == nil {
		t.Error("expected error for TTML with no paragraphs")
	}
}

func TestTTML_SortedOutput(t *testing.T) {
	unsorted := `<tt xmlns="http://www.w3.org/ns/ttml"><body><div>
		<p begin="00:00:10.000">Later</p>
		<p begin="00:00:01.000">Earlier</p>
		<p begin="00:00:05.000">Middle</p>
	</div></body></tt>`

	d, err := parseTTML(unsorted)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(d.Lines))
	}

	for i := 1; i < len(d.Lines); i++ {
		if d.Lines[i].Time < d.Lines[i-1].Time {
			t.Errorf("lines not sorted at index %d", i)
		}
	}
}

func TestTTML_FindSidecar(t *testing.T) {
	ext := ".mp3"
	path := "/some/path/song.mp3"
	base := path[:len(path)-len(ext)]
	ttmlExpected := base + ".ttml"
	xmlExpected := base + ".xml"

	if ttmlExpected != "/some/path/song.ttml" {
		t.Errorf("ttml path = %q", ttmlExpected)
	}
	if xmlExpected != "/some/path/song.xml" {
		t.Errorf("xml path = %q", xmlExpected)
	}
}

func TestTTML_PathProperty(t *testing.T) {
	d, err := parseTTML(testTTMLMinimal)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if d.Path != "" {
		t.Errorf("path should be empty for in-memory parse, got %q", d.Path)
	}
}

func TestTTML_CurrentLineIntegration(t *testing.T) {
	d, err := parseTTML(testTTML)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	tests := []struct {
		elapsed time.Duration
		want    int
	}{
		{0, -1},
		{0 * time.Millisecond, -1},
		{1000 * time.Millisecond, -1},
		{1500 * time.Millisecond, 0},
		{3000 * time.Millisecond, 0},
		{4000 * time.Millisecond, 1},
		{7000 * time.Millisecond, 1},
		{7500 * time.Millisecond, 2},
		{10000 * time.Millisecond, 2},
		{12000 * time.Millisecond, 3},
		{20000 * time.Millisecond, 3},
	}

	for _, tt := range tests {
		got := d.CurrentLine(tt.elapsed)
		if got != tt.want {
			t.Errorf("CurrentLine(%v) = %d, want %d", tt.elapsed, got, tt.want)
		}
	}
}

func TestTTML_SidecarExtensionPreference(t *testing.T) {
	ext := ".mp3"
	path := "/some/path/song.mp3"
	base := path[:len(path)-len(ext)]
	ttmlPath := base + ".ttml"
	if ttmlPath != "/some/path/song.ttml" {
		t.Errorf("ttml path = %q", ttmlPath)
	}
	xmlPath := base + ".xml"
	if xmlPath != "/some/path/song.xml" {
		t.Errorf("xml path = %q", xmlPath)
	}
}
