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
	if line3.Text != "word level sync" {
		t.Errorf("merged span text = %q, want 'word level sync'", line3.Text)
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

func TestTTML_BareSeconds(t *testing.T) {
	bareSec := `<tt xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="39.345" end="43.071">I could never find the right way</p>
    <p begin="44.085" end="46.505">Have you noticed I've been gone</p>
  </div></body>
</tt>`

	d, err := parseTTML(bareSec)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Time != 39345*time.Millisecond {
		t.Errorf("39.345s = %v, want 39345ms", d.Lines[0].Time)
	}
	if d.Lines[0].Text != "I could never find the right way" {
		t.Errorf("line 0 text = %q", d.Lines[0].Text)
	}

	if d.Lines[1].Time != 44085*time.Millisecond {
		t.Errorf("44.085s = %v, want 44085ms", d.Lines[1].Time)
	}
}

func TestTTML_WordSyncWithTranslation(t *testing.T) {
	appleStyle := `<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:ttm="http://www.w3.org/ns/ttml#metadata"
    xmlns:itunes="http://music.apple.com/lyric-ttml-internal" itunes:timing="Word">
  <body dur="0:10.000">
    <div begin="0.000" end="0:10.000">
      <p begin="1.345" end="3.071" itunes:key="L1" ttm:agent="v1">
        <span begin="1.345" end="1.548">I</span>
        <span begin="1.548" end="1.938">could</span>
        <span begin="1.938" end="2.198">find</span>
        <span begin="2.198" end="2.770">you</span>
        <span ttm:role="x-translation" xml:lang="zh-CN">我找到了你</span>
      </p>
      <p begin="4.085" end="6.505" itunes:key="L2" ttm:agent="v1">
        <span begin="4.085" end="4.510">Hello</span>
        <span begin="4.510" end="4.953">world</span>
        <span ttm:role="x-translation" xml:lang="zh-CN">你好世界</span>
      </p>
    </div>
  </body>
</tt>`

	d, err := parseTTML(appleStyle)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Text != "I could find you" {
		t.Errorf("line 0 text = %q, want 'I could find you' (no translation)", d.Lines[0].Text)
	}
	if strings.Contains(d.Lines[0].Text, "我") {
		t.Error("translation text leaked into line 0")
	}

	if len(d.Lines[0].Words) != 4 {
		t.Fatalf("expected 4 word fragments (not translation), got %d", len(d.Lines[0].Words))
	}
	if d.Lines[0].Words[0].Text != "I" || d.Lines[0].Words[0].Time != 1345*time.Millisecond {
		t.Errorf("word[0] = %q @ %v", d.Lines[0].Words[0].Text, d.Lines[0].Words[0].Time)
	}
	if d.Lines[0].Words[3].Text != "you" || d.Lines[0].Words[3].Time != 2198*time.Millisecond {
		t.Errorf("word[3] = %q @ %v", d.Lines[0].Words[3].Text, d.Lines[0].Words[3].Time)
	}

	if d.Lines[1].Time != 4085*time.Millisecond {
		t.Errorf("line 1 time = %v, want 4085ms", d.Lines[1].Time)
	}
	if d.Lines[1].Text != "Hello world" {
		t.Errorf("line 1 text = %q, want 'Hello world'", d.Lines[1].Text)
	}
}

func TestTTML_PartialClockTime(t *testing.T) {
	partial := `<tt xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="1:01.643" end="1:03.071">Trust in me</p>
    <p begin="2:19.153" end="2:23.083">I'll give them shelter</p>
  </div></body>
</tt>`

	d, err := parseTTML(partial)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Time != 61643*time.Millisecond {
		t.Errorf("1:01.643 = %v, want 61643ms", d.Lines[0].Time)
	}
	if d.Lines[0].Text != "Trust in me" {
		t.Errorf("line 0 text = %q", d.Lines[0].Text)
	}

	if d.Lines[1].Time != 139153*time.Millisecond {
		t.Errorf("2:19.153 = %v, want 139153ms", d.Lines[1].Time)
	}
}

func TestTTML_MixedBareSecondsAndPartial(t *testing.T) {
	mixed := `<tt xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="39.345" end="43.071">Bare seconds</p>
    <p begin="1:01.643" end="1:03.071">Partial clock time</p>
  </div></body>
</tt>`

	d, err := parseTTML(mixed)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Time != 39345*time.Millisecond {
		t.Errorf("39.345 = %v, want 39345ms", d.Lines[0].Time)
	}
	if d.Lines[1].Time != 61643*time.Millisecond {
		t.Errorf("1:01.643 = %v, want 61643ms", d.Lines[1].Time)
	}
}

func TestTTML_CJKLangNoSpace(t *testing.T) {
	cjk := `<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:ttm="http://www.w3.org/ns/ttml#metadata"
    xml:lang="zh-CN">
  <body><div xml:lang="zh-CN">
    <p begin="1.345" end="3.071" ttm:agent="v1">
      <span begin="1.345" end="1.548">` + "我" + `</span>
      <span begin="1.548" end="1.938">` + "找到" + `</span>
      <span begin="1.938" end="2.198">` + "了" + `</span>
      <span begin="2.198" end="2.770">` + "你" + `</span>
    </p>
  </div></body>
</tt>`

	d, err := parseTTML(cjk)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(d.Lines))
	}

	if d.Lines[0].Text != `我找到了你` {
		t.Errorf("CJK text should have no spaces between words, got %q", d.Lines[0].Text)
	}
	if len(d.Lines[0].Words) != 4 {
		t.Fatalf("expected 4 word fragments, got %d", len(d.Lines[0].Words))
	}
}

func TestTTML_CJKAutoDetectNoSpace(t *testing.T) {
	cjkNoLang := `<tt xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="1.345" end="3.071">
      <span begin="1.345" end="1.548">` + "我" + `</span>
      <span begin="1.548" end="1.938">` + "找到" + `</span>
      <span begin="1.938" end="2.198">` + "了" + `</span>
      <span begin="2.198" end="2.770">` + "你" + `</span>
    </p>
  </div></body>
</tt>`

	d, err := parseTTML(cjkNoLang)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(d.Lines))
	}

	if d.Lines[0].Text != `我找到了你` {
		t.Errorf("auto-detected CJK should have no spaces, got %q", d.Lines[0].Text)
	}
}

const testTTMLAgents = `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:ttm="http://www.w3.org/ns/ttml#metadata"
    xmlns:amll="http://www.example.com/ns/amll">
  <head>
    <metadata>
      <ttm:agent type="person" xml:id="v1"/>
      <ttm:agent type="other" xml:id="v2"/>
      <ttm:agent type="person" xml:id="v3"/>
      <amll:meta key="musicName" value="ME!"/>
      <amll:meta key="artists" value="Taylor Swift"/>
      <amll:meta key="artists" value="Brendon Urie"/>
      <amll:meta key="album" value="ME! (feat. Brendon Urie)"/>
      <amll:meta key="ncmMusicId" value="1361348080"/>
    </metadata>
  </head>
  <body dur="03:08.002">
    <div>
      <p begin="00:00.000" end="00:02.593" ttm:agent="v1">
        <span begin="00:00.000" end="00:00.223">I</span>
        <span begin="00:00.223" end="00:00.394">promise</span>
      </p>
      <p begin="00:03.490" end="00:05.848" ttm:agent="v1">
        <span begin="00:03.490" end="00:03.553">I</span>
        <span begin="00:03.553" end="00:03.722">know</span>
      </p>
      <p begin="00:58.854" end="01:01.239" ttm:agent="v2">
        <span begin="00:58.854" end="00:59.025">I</span>
        <span begin="00:59.025" end="00:59.176">know</span>
      </p>
      <p begin="02:46.447" end="02:47.814" ttm:agent="v2">
        <span begin="02:46.447" end="02:46.572">I'm</span>
        <span begin="02:46.572" end="02:46.732">the</span>
      </p>
      <p begin="02:50.728" end="02:52.328" ttm:agent="v3">
        <span begin="02:50.728" end="02:51.008">eeh</span>
      </p>
    </div>
  </body>
</tt>`

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

func TestTTML_FullIntegration(t *testing.T) {
	d, err := parseTTML(testTTMLAgents)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if d.Title != "ME!" || d.Artist != "Taylor Swift" || d.Album != "ME! (feat. Brendon Urie)" {
		t.Errorf("metadata mismatch: Title=%q Artist=%q Album=%q", d.Title, d.Artist, d.Album)
	}

	if d.Agents["v1"] != "Taylor Swift" || d.Agents["v2"] != "Brendon Urie" || d.Agents["v3"] != "V3" {
		t.Errorf("Agents map unexpected: %v", d.Agents)
	}

	if len(d.Lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(d.Lines))
	}
	for i := 1; i < len(d.Lines); i++ {
		if d.Lines[i].Time < d.Lines[i-1].Time {
			t.Errorf("lines not sorted at index %d", i)
		}
	}

	if d.Lines[0].Agent != "v1" || d.Lines[0].Time != 0 || d.Lines[0].End != 2593*time.Millisecond {
		t.Errorf("line 0: agent=%q time=%v end=%v", d.Lines[0].Agent, d.Lines[0].Time, d.Lines[0].End)
	}

	if d.Lines[2].Agent != "v2" || d.Lines[2].Time != 58854*time.Millisecond || d.Lines[2].End != 61239*time.Millisecond {
		t.Errorf("line 2: agent=%q time=%v end=%v", d.Lines[2].Agent, d.Lines[2].Time, d.Lines[2].End)
	}

	active := d.ActiveLines(60 * time.Second)
	if len(active) != 1 || active[0].Agent != "v2" {
		t.Errorf("ActiveLines(60s) = %d lines, want 1 (v2)", len(active))
	}
}
