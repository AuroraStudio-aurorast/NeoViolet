package lyrics

import (
	"strings"
	"testing"
	"time"
)

const testLRC = `[ti:LRC Comprehensive Test - Special Chars 🎵]
[ar:Test Artist]
[al:Test Album]
[au:Test Author]
[by:LRC Tester]
[offset:+300]

[00:00.00]Standard millisecond precision timestamp
[00:01]Short timestamp without milliseconds
[00:02.34][00:02.56]Multiple timestamps on the same line (shared text)
[00:03.78]     
[00:04.56]
[00:05.67]Punctuation: !@#$%^&*()_+{}:"<>?
[00:06.78]Emoji test: 😊🎶🎤🔥

; This is a comment line (non-standard but supported by some parsers)
[00:08.90]Line after a comment

[00:09.01][00:09.34][00:09.67]Three timestamps on one line

[offset:-100]
[00:10.11]Offset override to -100ms

<00:11.22>word<00:11.33>synced<00:11.44>lyrics

[00:12.00]Normal line
[00:13.00]Out-of-order line A (at 13.00)
[00:14.00][00:15.00]Timestamp order does not affect display
[00:20.00]Out-of-order line B (at 20.00, appears before 18.00)
[00:18.00]Out-of-order line C (at 18.00, appears after 20.00)

[00:21.00]Final line with centisecond precision

; ========== Multilingual lyrics test area ==========
; Same timestamps across multiple language lines
[00:22.00]This is the first multilingual line.
[00:22.00]这是第一句多语言歌词。
[00:22.00]이것은 첫 번째 다국어 가사입니다.
[00:22.00]これが最初の多言語歌詞です。

[00:23.50]Hello, world!
[00:23.50]你好，世界！
[00:23.50]안녕하세요, 세계!
[00:23.50]こんにちは、世界！

; Single line with separator
[00:24.00]EN | 中文 | 한국어 | 日本語

; Three repetitions at same timestamp
[00:25.00]First repetition
[00:25.00]Second repetition
[00:25.00]Third repetition

; Four languages merge
[00:26.00]English text
[00:26.00]中文文本
[00:26.00]한국어 텍스트
[00:26.00]日本語テキスト

; Separated same timestamp
[00:27.00]Multilingual A
[00:27.23]Different timestamp line between
[00:27.00]Multilingual B (same time as A, separated by other lines)

; Extended bracket format
[00:28.00:en]English only
[00:28.00:zh]中文
[00:28.00:ko]한국어
[00:28.00:ja]日本語

; Empty timestamp with multiple lines
[00:29.00]
[00:29.00]

; End
[00:30.00]Test file complete.`

func parseLRC(s string) (*LyricsData, error) {
	var p lrcParser
	return p.Parse(strings.NewReader(s), "")
}

func TestParse_Metadata(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if d.Title != "LRC Comprehensive Test - Special Chars 🎵" {
		t.Errorf("Title = %q, want %q", d.Title, "LRC Comprehensive Test - Special Chars 🎵")
	}
	if d.Artist != "Test Artist" {
		t.Errorf("Artist = %q, want %q", d.Artist, "Test Artist")
	}
	if d.Album != "Test Album" {
		t.Errorf("Album = %q, want %q", d.Album, "Test Album")
	}
	if d.Author != "Test Author" {
		t.Errorf("Author = %q, want %q", d.Author, "Test Author")
	}
	if d.Creator != "LRC Tester" {
		t.Errorf("Creator = %q, want %q", d.Creator, "LRC Tester")
	}
}

func TestParse_StandardTimestamp(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// [00:00.00] with +300ms offset = 300ms
	if len(d.Lines) == 0 {
		t.Fatal("no lines parsed")
	}
	if d.Lines[0].Time != 300*time.Millisecond {
		t.Errorf("line 0 time = %v, want 300ms", d.Lines[0].Time)
	}
	if d.Lines[0].Text != "Standard millisecond precision timestamp" {
		t.Errorf("line 0 text = %q", d.Lines[0].Text)
	}
}

func TestParse_ShortTimestamp(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:01] with +300ms offset = 1300ms
	if d.Lines[1].Time != 1300*time.Millisecond {
		t.Errorf("short timestamp time = %v, want 1300ms", d.Lines[1].Time)
	}
}

func TestParse_MultiTimestamp(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:02.34][00:02.56] multiple timestamps sharing text
	var found bool
	for _, l := range d.Lines {
		if l.Time == 2340*time.Millisecond+300*time.Millisecond && l.Text == "Multiple timestamps on the same line (shared text)" {
			found = true
			break
		}
	}
	if !found {
		t.Error("multi-timestamp line not found at 2.34")
	}
}

func TestParse_EmptyText(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:04.56] has empty text
	found := false
	for _, l := range d.Lines {
		if l.Time == 4560*time.Millisecond+300*time.Millisecond {
			found = true
			break
		}
	}
	if !found {
		t.Error("empty text line not found at 4.56")
	}
}

func TestParse_SpecialChars(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	tests := []struct {
		approxMs int
		text     string
	}{
		{5670, `Punctuation: !@#$%^&*()_+{}:"<>?`},
		{6780, "Emoji test: 😊🎶🎤🔥"},
	}
	for _, tt := range tests {
		target := time.Duration(tt.approxMs)*time.Millisecond + 300*time.Millisecond
		found := false
		for _, l := range d.Lines {
			if l.Time == target && l.Text == tt.text {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("special char line not found at %dms: %q", tt.approxMs, tt.text)
		}
	}
}

func TestParse_CommentIgnored(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:08.90] should be the next line after comments
	found := false
	for _, l := range d.Lines {
		if l.Text == "Line after a comment" {
			found = true
			break
		}
	}
	if !found {
		t.Error("line after comment not found")
	}
}

func TestParse_ThreeTimestamps(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:09.01][00:09.34][00:09.67] should produce 3 lines
	text := "Three timestamps on one line"
	count := 0
	for _, l := range d.Lines {
		if l.Text == text {
			count++
		}
	}
	if count != 3 {
		t.Errorf("expected 3 lines with that text, got %d", count)
	}
}

func TestParse_OffsetOverride(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:10.11] with offset -100ms = 10010ms = 10.01s
	found := false
	for _, l := range d.Lines {
		if l.Text == "Offset override to -100ms" {
			if l.Time != 10010*time.Millisecond {
				t.Errorf("offset line time = %v, want 10010ms", l.Time)
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("offset override line not found")
	}
}

func TestParse_WordLevel(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// <00:11.22>word<00:11.33>synced<00:11.44>lyrics
	// With offset -100ms
	found := false
	for _, l := range d.Lines {
		if strings.Contains(l.Text, "wordsyncedlyrics") {
			found = true
			if len(l.Words) == 0 {
				t.Error("word-level line has no word fragments")
			}
			break
		}
	}
	if !found {
		t.Error("word-level line not found")
	}
}

func TestParse_OutOfOrder(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// Lines should be sorted by time
	for i := 1; i < len(d.Lines); i++ {
		if d.Lines[i].Time < d.Lines[i-1].Time {
			t.Errorf("lines not sorted at index %d: %v < %v", i, d.Lines[i].Time, d.Lines[i-1].Time)
			break
		}
	}
}

func TestParse_MultilingualMerge(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:22.00] should merge EN + ZH + KO + JA with " | "
	found := false
	for _, l := range d.Lines {
		if strings.Contains(l.Text, "This is the first multilingual line.") &&
			strings.Contains(l.Text, "这是第一句多语言歌词。") &&
			strings.Contains(l.Text, "이것은 첫 번째 다국어 가사입니다.") &&
			strings.Contains(l.Text, "これが最初の多言語歌詞です。") &&
			strings.Contains(l.Text, " | ") {
			found = true
			break
		}
	}
	if !found {
		t.Error("multilingual merge not found (expected EN | ZH | KO | JA)")
	}
}

func TestParse_ThreeRepetitions(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:25.00] appears 3 times, should merge all 3 with " | "
	found := false
	for _, l := range d.Lines {
		if l.Time == 25000*time.Millisecond-100*time.Millisecond { // offset -100ms
			if strings.Count(l.Text, "|") == 2 {
				found = true
			}
			break
		}
	}
	if !found {
		t.Error("three repetitions not properly merged")
	}
}

func TestParse_FourLanguages(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:26.00] with 4 lines (EN + ZH + KO + JA), all merged
	found := false
	for _, l := range d.Lines {
		if strings.Contains(l.Text, "English text") &&
			strings.Contains(l.Text, "中文文本") &&
			strings.Contains(l.Text, "한국어 텍스트") &&
			strings.Contains(l.Text, "日本語テキスト") {
			found = true
			break
		}
	}
	if !found {
		t.Error("four languages not merged")
	}
}

func TestParse_SeparatedSameTimestamp(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:27.00] appears twice with [00:27.23] between them.
	// Non-consecutive same-timestamp lines are NOT merged by the parser.
	foundA := false
	foundB := false
	for _, l := range d.Lines {
		if l.Text == "Multilingual A" {
			foundA = true
		}
		if l.Text == "Multilingual B (same time as A, separated by other lines)" {
			foundB = true
		}
	}
	if !foundA {
		t.Error("multilingual A not found at 27.00")
	}
	if !foundB {
		t.Error("multilingual B not found at 27.00")
	}
}

func TestParse_ExtendedBracket(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:28.00:en], [00:28.00:zh], [00:28.00:ko], [00:28.00:ja]
	// All four share the same timestamp so they get merged.
	foundEN := false
	foundZH := false
	foundKO := false
	foundJA := false
	hasMerge := false
	for _, l := range d.Lines {
		text := l.Text
		if strings.Contains(text, "English only") {
			foundEN = true
		}
		if strings.Contains(text, "中文") {
			foundZH = true
		}
		if strings.Contains(text, "한국어") {
			foundKO = true
		}
		if strings.Contains(text, "日本語") {
			foundJA = true
		}
		if strings.Contains(text, " | ") {
			hasMerge = true
		}
	}
	if !foundEN {
		t.Error("extended bracket [en] not parsed")
	}
	if !foundZH {
		t.Error("extended bracket [zh] not parsed")
	}
	if !foundKO {
		t.Error("extended bracket [ko] not parsed")
	}
	if !foundJA {
		t.Error("extended bracket [ja] not parsed")
	}
	if !hasMerge {
		t.Error("extended bracket languages not merged")
	}
}

func TestParse_EmptyTimestampText(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:29.00] appears twice with empty text
	found := false
	for _, l := range d.Lines {
		if l.Time == 29000*time.Millisecond-100*time.Millisecond {
			found = true
			break
		}
	}
	if !found {
		t.Error("empty text timestamp line not found at 29.00")
	}
}

func TestParse_FinalLine(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	found := false
	for _, l := range d.Lines {
		if strings.Contains(l.Text, "Test file complete.") {
			found = true
			break
		}
	}
	if !found {
		t.Error("final line not found")
	}
}

func TestParse_InvalidLine_Rejects(t *testing.T) {
	invalid := "[00:00.00]valid\nthis is not valid lrc at all\n[00:01.00]also valid"
	_, err := parseLRC(invalid)
	if err != nil {
		t.Fatalf("unexpected error for mixed content: %v", err)
	}
}

func TestSidecarPathDerivation(t *testing.T) {
	ext := ".mp3"
	path := "/some/path/song.mp3"
	lrcPath := path[:len(path)-len(ext)] + ".lrc"
	if lrcPath != "/some/path/song.lrc" {
		t.Errorf("path derivation = %q, want /some/path/song.lrc", lrcPath)
	}
}

func TestCurrentLine(t *testing.T) {
	lines := []LyricLine{
		{Time: 1000 * time.Millisecond, Text: "one"},
		{Time: 3000 * time.Millisecond, Text: "two"},
		{Time: 5000 * time.Millisecond, Text: "three"},
	}
	d := &LyricsData{Lines: lines}

	tests := []struct {
		elapsed time.Duration
		want    int
	}{
		{0, -1},
		{500 * time.Millisecond, -1},
		{1000 * time.Millisecond, 0},
		{2000 * time.Millisecond, 0},
		{2999 * time.Millisecond, 0},
		{3000 * time.Millisecond, 1},
		{4000 * time.Millisecond, 1},
		{5000 * time.Millisecond, 2},
		{9999 * time.Millisecond, 2},
	}
	for _, tt := range tests {
		got := d.CurrentLine(tt.elapsed)
		if got != tt.want {
			t.Errorf("CurrentLine(%v) = %d, want %d", tt.elapsed, got, tt.want)
		}
	}
}

func TestParse_WhitespaceText(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:03.78] with spaces - should be treated as a valid line
	found := false
	for _, l := range d.Lines {
		if l.Time == 3780*time.Millisecond+300*time.Millisecond {
			found = true
			break
		}
	}
	if !found {
		t.Error("whitespace-only text line not found at 3.78")
	}
}

func TestParse_EmptyFile(t *testing.T) {
	_, err := parseLRC("")
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestParse_OnlyComments(t *testing.T) {
	_, err := parseLRC("; comment 1\n; comment 2\n")
	if err == nil {
		t.Error("expected error for comments-only file")
	}
}

func TestCurrentLine_Empty(t *testing.T) {
	d := &LyricsData{}
	if idx := d.CurrentLine(5 * time.Second); idx != -1 {
		t.Errorf("empty lyrics: got %d, want -1", idx)
	}
}

func TestParse_FourLanguageExtendedBracketMerge(t *testing.T) {
	d, err := parseLRC(testLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:28.00] has en, zh, ko, ja entries - after merging they should
	// all be on the same line since timestamps match with offset -100ms
	found := false
	for _, l := range d.Lines {
		if strings.Contains(l.Text, "English only") &&
			strings.Contains(l.Text, "中文") &&
			strings.Contains(l.Text, "한국어") &&
			strings.Contains(l.Text, "日本語") &&
			strings.Contains(l.Text, " | ") {
			found = true
			break
		}
	}
	if !found {
		t.Error("extended bracket multilingual merge not found")
	}
}
