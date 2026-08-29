package lyrics

import (
	"testing"
	"time"
)

func TestTTML_ParseAgentOnParagraph(t *testing.T) {
	d, err := parseTTML(testTTMLAgents)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(d.Lines))
	}
	if d.Lines[0].Agent != "v1" {
		t.Errorf("line 0 agent = %q, want v1", d.Lines[0].Agent)
	}
	if d.Lines[2].Agent != "v2" {
		t.Errorf("line 2 agent = %q, want v2", d.Lines[2].Agent)
	}
	if d.Lines[4].Agent != "v3" {
		t.Errorf("line 4 agent = %q, want v3", d.Lines[4].Agent)
	}
	if d.Lines[0].End == 0 {
		t.Error("line 0 End should not be 0")
	}
}

func TestTTML_ParseHeadAgent(t *testing.T) {
	d, err := parseTTML(testTTMLAgents)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if d.Agents == nil {
		t.Fatal("Agents map is nil")
	}
	if d.Agents["v1"] != "Taylor Swift" {
		t.Errorf("v1 display name = %q, want Taylor Swift", d.Agents["v1"])
	}
	if d.Agents["v2"] != "Brendon Urie" {
		t.Errorf("v2 display name = %q, want Brendon Urie", d.Agents["v2"])
	}
}

func TestTTML_ExcessAgentFallback(t *testing.T) {
	d, err := parseTTML(testTTMLAgents)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if d.Agents["v3"] != "V3" {
		t.Errorf("excess agent v3 display name = %q, want V3", d.Agents["v3"])
	}
}

func TestTTML_NoArtistsMapping(t *testing.T) {
	noArtists := `<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:ttm="http://www.w3.org/ns/ttml#metadata">
  <head>
    <metadata>
      <ttm:agent type="person" xml:id="v1"/>
    </metadata>
  </head>
  <body><div>
    <p begin="00:00.000" end="00:01.000" ttm:agent="v1">Hello</p>
  </div></body>
</tt>`
	d, err := parseTTML(noArtists)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if d.Agents["v1"] != "V1" {
		t.Errorf("no-artist agent display name = %q, want V1", d.Agents["v1"])
	}
}

func TestTTML_ParseAMLLMeta(t *testing.T) {
	d, err := parseTTML(testTTMLAgents)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if d.Properties == nil {
		t.Fatal("Properties map is nil")
	}
	if d.Properties["musicName"] != "ME!" {
		t.Errorf("musicName = %q, want ME!", d.Properties["musicName"])
	}
	if d.Properties["ncmMusicId"] != "1361348080" {
		t.Errorf("ncmMusicId = %q, want 1361348080", d.Properties["ncmMusicId"])
	}
	if d.Properties["album"] != "ME! (feat. Brendon Urie)" {
		t.Errorf("album = %q, want ME! (feat. Brendon Urie)", d.Properties["album"])
	}
	if d.Title != "ME!" {
		t.Errorf("Title = %q, want ME!", d.Title)
	}
	if d.Artist != "Taylor Swift" {
		t.Errorf("Artist = %q, want Taylor Swift", d.Artist)
	}
	if d.Album != "ME! (feat. Brendon Urie)" {
		t.Errorf("Album = %q, want ME! (feat. Brendon Urie)", d.Album)
	}
}

func TestTTML_ParseEndTime(t *testing.T) {
	d, err := parseTTML(testTTMLAgents)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if d.Lines[0].End != 2593*time.Millisecond {
		t.Errorf("line 0 end = %v, want 2593ms", d.Lines[0].End)
	}
	if d.Lines[1].End != 5848*time.Millisecond {
		t.Errorf("line 1 end = %v, want 5848ms", d.Lines[1].End)
	}
}
