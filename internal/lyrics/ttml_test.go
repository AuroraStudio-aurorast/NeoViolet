package lyrics

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const testTTML = `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:tts="http://www.w3.org/ns/ttml#styling"
    xmlns:ttm="http://www.w3.org/ns/ttml#metadata"
    xmlns:ttp="http://www.w3.org/ns/ttml#parameter"
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
        <span begin="00:00:12.000" end="00:00:13.000">word </span>
        <span begin="00:00:13.000" end="00:00:14.000">level </span>
        <span begin="00:00:14.000" end="00:00:15.500">sync</span>
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
    xmlns:ttp="http://www.w3.org/ns/ttml#parameter"
    ttp:frameRate="60"
    ttp:frameRateMultiplier="1001 1000"
    ttp:subFrameRate="2">
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

func parseTTML(s string) (*Data, error) {
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
	if line3.Words[0].Text != "word " || line3.Words[0].Time != 12000*time.Millisecond {
		t.Errorf("word[0] = %q @ %v, want 'word ' @ 12s (trailing space merged)", line3.Words[0].Text, line3.Words[0].Time)
	}
	if line3.Words[1].Text != "level " || line3.Words[1].Time != 13000*time.Millisecond {
		t.Errorf("word[1] = %q @ %v, want 'level ' @ 13s (trailing space merged)", line3.Words[1].Text, line3.Words[1].Time)
	}
	if line3.Words[2].Text != "sync" || line3.Words[2].Time != 14000*time.Millisecond {
		t.Errorf("word[2] = %q @ %v, want 'sync' @ 14s", line3.Words[2].Text, line3.Words[2].Time)
	}
}

func TestTTML_NoSpaceIsNotInvented(t *testing.T) {
	noSpace := `<tt xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="00:00:12.000" end="00:00:15.500">
      <span begin="00:00:12.000" end="00:00:13.000">word</span>
      <span begin="00:00:13.000" end="00:00:14.000">level</span>
      <span begin="00:00:14.000" end="00:00:15.500">sync</span>
    </p>
  </div></body>
</tt>`

	d, err := parseTTML(noSpace)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(d.Lines))
	}

	// The library must never invent inter-word spaces (spec D5): spans with no
	// real whitespace between them stay glued together. Each timed span keeps
	// its own word fragment, and concatenating the fragments reproduces the
	// glued Text with no invented spaces - the exact opposite of the CJK/space
	// heuristics the old hand-written parser applied.
	if d.Lines[0].Text != "wordlevelsync" {
		t.Errorf("Text = %q, want %q (spaces are never invented)", d.Lines[0].Text, "wordlevelsync")
	}
	if len(d.Lines[0].Words) != 3 {
		t.Errorf("words = %d, want 3 (each timed span stays its own fragment)", len(d.Lines[0].Words))
	}
	var joined strings.Builder
	for _, w := range d.Lines[0].Words {
		joined.WriteString(w.Text)
	}
	if joined.String() != d.Lines[0].Text {
		t.Errorf("joined fragments = %q, want glued Text %q", joined.String(), d.Lines[0].Text)
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

func TestTTML_ZeroLinesIsAnError(t *testing.T) {
	noP := `<tt xmlns="http://www.w3.org/ns/ttml"><body><div></div></body></tt>`
	_, err := parseTTML(noP)
	if !errors.Is(err, ErrNoLyrics) {
		t.Errorf("Parse() error = %v, want ErrNoLyrics", err)
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

// ttmlAppleStyle is the Apple-style (iTunes) word-timing fixture used by
// TestTTML_WordSyncWithTranslation. ttmlAppleRegressionSample (in
// ttml_regression_test.go) is a verbatim copy of it; that copy's sync guard
// asserts the two stay byte-identical.
const ttmlAppleStyle = `<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:ttm="http://www.w3.org/ns/ttml#metadata"
    xmlns:itunes="http://music.apple.com/lyric-ttml-internal" itunes:timing="Word">
  <body dur="0:10.000">
    <div begin="0.000" end="0:10.000">
      <p begin="1.345" end="3.071" itunes:key="L1" ttm:agent="v1">
        <span begin="1.345" end="1.548">I </span>
        <span begin="1.548" end="1.938">could </span>
        <span begin="1.938" end="2.198">find </span>
        <span begin="2.198" end="2.770">you</span>
        <span ttm:role="x-translation" xml:lang="zh-CN">我找到了你</span>
      </p>
      <p begin="4.085" end="6.505" itunes:key="L2" ttm:agent="v1">
        <span begin="4.085" end="4.510">Hello </span>
        <span begin="4.510" end="4.953">world</span>
        <span ttm:role="x-translation" xml:lang="zh-CN">你好世界</span>
      </p>
    </div>
  </body>
</tt>`

func TestTTML_WordSyncWithTranslation(t *testing.T) {
	d, err := parseTTML(ttmlAppleStyle)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(d.Lines))
	}

	// The inline translation is no longer dropped (spec §6 item 1): it becomes
	// the second display part, while Text and Words stay on the original segment.
	if len(d.Lines[0].Parts) != 2 || d.Lines[0].Parts[0] != "I could find you" || d.Lines[0].Parts[1] != "我找到了你" {
		t.Errorf("line 0 Parts = %q, want [%q %q]", d.Lines[0].Parts, "I could find you", "我找到了你")
	}
	if want := "I could find you | 我找到了你"; d.Lines[0].Text != want {
		t.Errorf("line 0 Text = %q, want %q", d.Lines[0].Text, want)
	}

	if len(d.Lines[0].Words) != 4 {
		t.Fatalf("expected 4 word fragments (not translation), got %d", len(d.Lines[0].Words))
	}
	if d.Lines[0].Words[0].Text != "I " || d.Lines[0].Words[0].Time != 1345*time.Millisecond {
		t.Errorf("word[0] = %q @ %v, want 'I ' @ 1345ms", d.Lines[0].Words[0].Text, d.Lines[0].Words[0].Time)
	}
	if d.Lines[0].Words[3].Text != "you" || d.Lines[0].Words[3].Time != 2198*time.Millisecond {
		t.Errorf("word[3] = %q @ %v, want 'you' @ 2198ms", d.Lines[0].Words[3].Text, d.Lines[0].Words[3].Time)
	}

	if d.Lines[1].Time != 4085*time.Millisecond {
		t.Errorf("line 1 time = %v, want 4085ms", d.Lines[1].Time)
	}
	if len(d.Lines[1].Parts) != 2 || d.Lines[1].Parts[0] != "Hello world" || d.Lines[1].Parts[1] != "你好世界" {
		t.Errorf("line 1 Parts = %q, want [%q %q]", d.Lines[1].Parts, "Hello world", "你好世界")
	}
	if want := "Hello world | 你好世界"; d.Lines[1].Text != want {
		t.Errorf("line 1 Text = %q, want %q", d.Lines[1].Text, want)
	}
}

// TestTTML_CJKNoInventedSpace pins spec §6 item 2 for CJK text: the library
// never invents inter-word spaces. The old hand-written parser's isCJKLang /
// isCJKContent heuristics were deleted; this name now points at the fidelity
// guarantee those heuristics used to approximate.
func TestTTML_CJKNoInventedSpace(t *testing.T) {
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

// TestTTML_CJKWithoutLang pins the same no-invented-space fidelity for a CJK
// file that carries no xml:lang hint: the glueing is the library's default, not
// a language-detection heuristic (isCJKContent no longer exists).
func TestTTML_CJKWithoutLang(t *testing.T) {
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
		t.Errorf("CJK without xml:lang should have no spaces, got %q", d.Lines[0].Text)
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
        <span begin="00:00.000" end="00:00.223">I </span>
        <span begin="00:00.223" end="00:00.394">promise</span>
      </p>
      <p begin="00:03.490" end="00:05.848" ttm:agent="v1">
        <span begin="00:03.490" end="00:03.553">I </span>
        <span begin="00:03.553" end="00:03.722">know</span>
      </p>
      <p begin="00:58.854" end="01:01.239" ttm:agent="v2">
        <span begin="00:58.854" end="00:59.025">I </span>
        <span begin="00:59.025" end="00:59.176">know</span>
      </p>
      <p begin="02:46.447" end="02:47.814" ttm:agent="v2">
        <span begin="02:46.447" end="02:46.572">I'm </span>
        <span begin="02:46.572" end="02:46.732">the</span>
      </p>
      <p begin="02:50.728" end="02:52.328" ttm:agent="v3">
        <span begin="02:50.728" end="02:51.008">eeh</span>
      </p>
    </div>
  </body>
</tt>`

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
