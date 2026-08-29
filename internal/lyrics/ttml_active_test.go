package lyrics

import (
	"testing"
	"time"
)

func TestTTML_ActiveLinesBounded(t *testing.T) {
	d, err := parseTTML(testTTMLAgents)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	tests := []struct {
		elapsed time.Duration
		want    int
	}{
		{0, 1},                       // line 0 starts at 0
		{1500 * time.Millisecond, 1}, // line 0 active
		{2593 * time.Millisecond, 0}, // line 0 just ended (exclusive)
		{4000 * time.Millisecond, 1}, // only line 1
		{6000 * time.Millisecond, 0}, // between lines
	}

	for _, tt := range tests {
		active := d.ActiveLines(tt.elapsed)
		if len(active) != tt.want {
			t.Errorf("ActiveLines(%v) returned %d lines, want %d", tt.elapsed, len(active), tt.want)
		}
	}
}

func TestTTML_ActiveLinesOverlapping(t *testing.T) {
	overlap := `<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:ttm="http://www.w3.org/ns/ttml#metadata">
  <body><div>
    <p begin="00:10.000" end="00:15.000" ttm:agent="v1">First part</p>
    <p begin="00:12.000" end="00:18.000" ttm:agent="v2">Harmony part</p>
  </div></body>
</tt>`
	d, err := parseTTML(overlap)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	active := d.ActiveLines(11 * time.Second)
	if len(active) != 1 {
		t.Fatalf("at 11s expected 1 active line, got %d", len(active))
	}
	if active[0].Agent != "v1" {
		t.Errorf("at 11s active line agent = %q, want v1", active[0].Agent)
	}

	active = d.ActiveLines(13 * time.Second)
	if len(active) != 2 {
		t.Fatalf("at 13s expected 2 active lines, got %d", len(active))
	}

	active = d.ActiveLines(16 * time.Second)
	if len(active) != 1 {
		t.Fatalf("at 16s expected 1 active line, got %d", len(active))
	}
	if active[0].Agent != "v2" {
		t.Errorf("at 16s active line agent = %q, want v2", active[0].Agent)
	}
}

func TestTTML_ActiveLinesLegacy(t *testing.T) {
	noEnd := `<tt xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="00:01.000">First</p>
    <p begin="00:03.000">Second</p>
    <p begin="00:05.000">Third</p>
  </div></body>
</tt>`
	d, err := parseTTML(noEnd)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	for i, line := range d.Lines {
		if line.End != 0 {
			t.Errorf("line %d End = %v, want 0", i, line.End)
		}
	}

	active := d.ActiveLines(2 * time.Second)
	if len(active) != 1 {
		t.Fatalf("ActiveLines(2s) returned %d lines, want 1", len(active))
	}
	if active[0].Text != "First" {
		t.Errorf("active line text = %q, want First", active[0].Text)
	}

	active = d.ActiveLines(4 * time.Second)
	if len(active) != 1 {
		t.Fatalf("ActiveLines(4s) returned %d lines, want 1", len(active))
	}
	if active[0].Text != "Second" {
		t.Errorf("active line text = %q, want Second", active[0].Text)
	}
}

func TestTTML_ActiveLinesEmpty(t *testing.T) {
	d, err := parseTTML(testTTMLAgents)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// At exactly 0, line 0 is active (it has begin=0)
	// Use a duration before any active line
	// Note: line 0 is NOT active at 2593ms (end is exclusive)
	active := d.ActiveLines(2594 * time.Millisecond)
	if len(active) != 0 {
		t.Errorf("ActiveLines(2594ms) = %d, want 0", len(active))
	}
}

func TestTTML_ActiveLinesFilter(t *testing.T) {
	overlap := `<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:ttm="http://www.w3.org/ns/ttml#metadata">
  <head>
    <metadata>
      <ttm:agent type="person" xml:id="v1"/>
      <ttm:agent type="other" xml:id="v2"/>
    </metadata>
  </head>
  <body><div>
    <p begin="00:10.000" end="00:20.000" ttm:agent="v1">Lead vocal</p>
    <p begin="00:12.000" end="00:18.000" ttm:agent="v2">Harmony</p>
  </div></body>
</tt>`
	d, err := parseTTML(overlap)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	d.AgentFilter = "v1"
	active := d.ActiveLines(14 * time.Second)
	if len(active) != 1 {
		t.Fatalf("filtered ActiveLines returned %d lines, want 1", len(active))
	}
	if active[0].Agent != "v1" {
		t.Errorf("filtered active line agent = %q, want v1", active[0].Agent)
	}

	d.AgentFilter = "v2"
	active = d.ActiveLines(14 * time.Second)
	if len(active) != 1 {
		t.Fatalf("filtered v2 ActiveLines returned %d lines, want 1", len(active))
	}
	if active[0].Agent != "v2" {
		t.Errorf("filtered v2 active line agent = %q, want v2", active[0].Agent)
	}

	d.AgentFilter = ""
	active = d.ActiveLines(14 * time.Second)
	if len(active) != 2 {
		t.Fatalf("unfiltered ActiveLines returned %d lines, want 2", len(active))
	}
}

func TestTTML_ActiveLinesFilterNoMatch(t *testing.T) {
	d, err := parseTTML(testTTMLAgents)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	d.AgentFilter = "nonexistent"
	active := d.ActiveLines(1500 * time.Millisecond)
	if len(active) != 0 {
		t.Errorf("filtered with nonexistent agent returned %d lines, want 0", len(active))
	}
}

func TestTTML_LineDisplayTextWithAgent(t *testing.T) {
	d, err := parseTTML(testTTMLAgents)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	display := d.LineDisplayText(d.Lines[0])
	want := "Taylor Swift: I promise"
	if display != want {
		t.Errorf("LineDisplayText = %q, want %q", display, want)
	}

	display = d.LineDisplayText(d.Lines[4])
	want = "V3: eeh"
	if display != want {
		t.Errorf("LineDisplayText for v3 = %q, want %q", display, want)
	}
}

func TestTTML_LineDisplayTextNoAgent(t *testing.T) {
	d, err := parseTTML(testTTML)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	display := d.LineDisplayText(d.Lines[0])
	want := "First line of lyrics"
	if display != want {
		t.Errorf("LineDisplayText (no agent) = %q, want %q", display, want)
	}
}
