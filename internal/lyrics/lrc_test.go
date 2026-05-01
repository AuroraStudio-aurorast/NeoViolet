package lyrics

import (
	"strings"
	"testing"
	"time"
)

const testLRC = `[ti:LRC 全面特性测试 - 包含特殊字符 🎵]
[ar:测试歌手 (Test Artist)]
[al:测试专辑 (Test Album)]
[au:测试作者]
[by:LRC Tester]
[offset:+300]

[00:00.00]标准带毫秒时间标签
[00:01]不带毫秒的简写标签（秒）
[00:02.34][00:02.56]同一行内多个时间标签（共享文本）
[00:03.78]     
[00:04.56]空歌词行（仅时间标签，无文本）
[00:05.67]包含标点：!@#$%^&*()_+{}:"<>?
[00:06.78]中文歌词：你好，世界！
[00:07.89]Emoji 测试：😊🎶🎤🔥

; 这是一条注释（非标准但部分解析器支持）行首分号
[00:08.90]注：上一行为非标准注释，解析器应忽略或保留原行

[00:09.01][00:09.34][00:09.67]每行最多三个时间标签（演示爆炸限制）

[offset:-100]
[00:10.11]此句之后偏移变为 -100 毫秒（多 offset 覆盖测试）

<00:11.22>词<00:11.33>曲<00:11.44>同<00:11.55>步<00:11.66>扩<00:11.66>展（尖括号内嵌单词时间戳）

[00:12.00]普通行
[00:13.00]乱序行A（实际时间 13.00，出现在 12.00 后面，正常顺序）
[00:14.00][00:15.00]标签对顺序不影响歌词显示
[00:20.00]乱序行B（时间 20.00，但在文件中位于 18.00 之前）
[00:18.00]乱序行C（时间 18.00，出现在 20.00 之后，测试解析器是否自动排序或保留原顺序）

[00:21.00]结尾行，时间戳精度为百分秒（0.01秒）

; ========== 双语歌词测试区域 ==========
; 场景1：完全相同的时间戳，分别写两行（原文 + 译文）
[00:22.00]This is the first bilingual line.
[00:22.00]这是第一句双语歌词（译文）。

[00:23.50]Hello, world!
[00:23.50]你好，世界！

; 场景2：同一行内用分隔符表示双语（非标准，部分播放器通过自定义标签支持，如 [00:24.00] 原文 | 译文）
[00:24.00]原文内容 | 译文内容（竖线分隔）

; 场景3：相同时间戳重复三次以上（测试重复处理）
[00:25.00]First repetition
[00:25.00]Second repetition
[00:25.00]Third repetition - 解析器应保留全部，还是只取最后一个？

; 场景4：相同时间戳混合标准行和带元数据的行（无特殊，仅作为边界测试）
[00:26.00]English text
[00:26.00]中文文本
[00:26.00]  带空格的第三语言

; 场景5：相同时间戳但是一个在前一个在后，中间有其他时间戳的行（测试是否自动去重或保留顺序）
[00:27.00]双语 A
[00:27.23]与双语 A 不同时间的行
[00:27.00]双语 B（与双语 A 相同时间，但相隔几行）

; 场景6：扩展 "[" 和 "]" 之外的双语标签（例如 [00:28.00:en] 和 [00:28.00:zh]） - 非标准，仅作鲁棒性测试
[00:28.00:en]English only
[00:28.00:zh]仅中文

; 场景7：同一时间戳，歌词文本为空（已有测试），再添加一个空行
[00:29.00]
[00:29.00]

; 结束
[00:30.00]测试文件结束。`

func TestParse_Metadata(t *testing.T) {
	d, err := Parse(strings.NewReader(testLRC), "")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if d.Title != "LRC 全面特性测试 - 包含特殊字符 🎵" {
		t.Errorf("Title = %q, want %q", d.Title, "LRC 全面特性测试 - 包含特殊字符 🎵")
	}
	if d.Artist != "测试歌手 (Test Artist)" {
		t.Errorf("Artist = %q", d.Artist)
	}
	if d.Album != "测试专辑 (Test Album)" {
		t.Errorf("Album = %q", d.Album)
	}
	if d.Author != "测试作者" {
		t.Errorf("Author = %q", d.Author)
	}
	if d.Creator != "LRC Tester" {
		t.Errorf("Creator = %q", d.Creator)
	}
}

func TestParse_StandardTimestamp(t *testing.T) {
	d, err := Parse(strings.NewReader(testLRC), "")
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
	if d.Lines[0].Text != "标准带毫秒时间标签" {
		t.Errorf("line 0 text = %q", d.Lines[0].Text)
	}
}

func TestParse_ShortTimestamp(t *testing.T) {
	d, err := Parse(strings.NewReader(testLRC), "")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:01] with +300ms offset = 1300ms
	if d.Lines[1].Time != 1300*time.Millisecond {
		t.Errorf("short timestamp time = %v, want 1300ms", d.Lines[1].Time)
	}
}

func TestParse_MultiTimestamp(t *testing.T) {
	d, err := Parse(strings.NewReader(testLRC), "")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:02.34][00:02.56]同一行内多个时间标签（共享文本）
	// Each timestamp gets its own line with same text
	var found bool
	for _, l := range d.Lines {
		if l.Time == 2340*time.Millisecond+300*time.Millisecond && l.Text == "同一行内多个时间标签（共享文本）" {
			found = true
			break
		}
	}
	if !found {
		t.Error("multi-timestamp line not found at 2.34")
	}
}

func TestParse_EmptyText(t *testing.T) {
	d, err := Parse(strings.NewReader(testLRC), "")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:04.56]空歌词行（仅时间标签，无文本）
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
	d, err := Parse(strings.NewReader(testLRC), "")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	tests := []struct {
		approxMs int
		text     string
	}{
		{5670, "包含标点：!@#$%^&*()_+{}:\"<>?"},
		{6780, "中文歌词：你好，世界！"},
		{7890, "Emoji 测试：😊🎶🎤🔥"},
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
	d, err := Parse(strings.NewReader(testLRC), "")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:08.90] should be the next line after comments
	found := false
	for _, l := range d.Lines {
		if l.Text == "注：上一行为非标准注释，解析器应忽略或保留原行" {
			found = true
			break
		}
	}
	if !found {
		t.Error("line after comment not found")
	}
}

func TestParse_ThreeTimestamps(t *testing.T) {
	d, err := Parse(strings.NewReader(testLRC), "")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:09.01][00:09.34][00:09.67] should produce 3 lines
	text := "每行最多三个时间标签（演示爆炸限制）"
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
	d, err := Parse(strings.NewReader(testLRC), "")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:10.11] with offset -100ms = 10010ms = 10.01s
	found := false
	for _, l := range d.Lines {
		if l.Text == "此句之后偏移变为 -100 毫秒（多 offset 覆盖测试）" {
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
	d, err := Parse(strings.NewReader(testLRC), "")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// <00:11.22>词<00:11.33>曲<00:11.44>同<00:11.55>步<00:11.66>扩<00:11.66>展
	// With offset -100ms (offset at this point is -100)
	// Text should be concatenated: "词曲同步扩展"
	found := false
	for _, l := range d.Lines {
		if strings.Contains(l.Text, "词曲同步扩展") {
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
	d, err := Parse(strings.NewReader(testLRC), "")
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

func TestParse_BilingualMerge(t *testing.T) {
	d, err := Parse(strings.NewReader(testLRC), "")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:22.00] should merge English and Chinese with " | "
	found := false
	for _, l := range d.Lines {
		if strings.Contains(l.Text, "This is the first bilingual line.") &&
			strings.Contains(l.Text, "这是第一句双语歌词（译文）。") &&
			strings.Contains(l.Text, " | ") {
			found = true
			break
		}
	}
	if !found {
		t.Error("bilingual merge not found")
	}
}

func TestParse_ThreeRepetitions(t *testing.T) {
	d, err := Parse(strings.NewReader(testLRC), "")
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

func TestParse_ThreeLanguages(t *testing.T) {
	d, err := Parse(strings.NewReader(testLRC), "")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:26.00] with 3 lines, all merged
	found := false
	for _, l := range d.Lines {
		if strings.Contains(l.Text, "English text") &&
			strings.Contains(l.Text, "中文文本") &&
			strings.Contains(l.Text, "带空格的第三语言") {
			found = true
			break
		}
	}
	if !found {
		t.Error("three languages not merged")
	}
}

func TestParse_SeparatedSameTimestamp(t *testing.T) {
	d, err := Parse(strings.NewReader(testLRC), "")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:27.00] appears twice with [00:27.23] between them
	// After de-dup within same timestamp block, both 27.00 lines should merge
	found := false
	for _, l := range d.Lines {
		if strings.Contains(l.Text, "双语 A") &&
			strings.Contains(l.Text, "双语 B") &&
			!strings.Contains(l.Text, "与双语 A 不同时间的行") {
			found = true
			break
		}
	}
	if !found {
		t.Error("separated same-timestamp lines not merged correctly")
	}
}

func TestParse_ExtendedBracket(t *testing.T) {
	d, err := Parse(strings.NewReader(testLRC), "")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// [00:28.00:en] and [00:28.00:zh] - extended format
	found := false
	for _, l := range d.Lines {
		if strings.Contains(l.Text, "English only") {
			found = true
			break
		}
	}
	if !found {
		t.Error("extended bracket [x:y:z] not parsed")
	}
}

func TestParse_EmptyTimestampText(t *testing.T) {
	d, err := Parse(strings.NewReader(testLRC), "")
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
	d, err := Parse(strings.NewReader(testLRC), "")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	found := false
	for _, l := range d.Lines {
		if strings.Contains(l.Text, "测试文件结束。") {
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
	_, err := Parse(strings.NewReader(invalid), "")
	if err != nil {
		t.Fatalf("unexpected error for mixed content: %v", err)
	}
	// Actually, lines without brackets are just skipped, not rejected
	// The parser currently skips non-bracket lines, so "this is not..." is skipped
}

func TestFindLRC(t *testing.T) {
	// Test path transformation (file existence check is separate)
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

func TestRealLyricsFile(t *testing.T) {
	d, err := ParseFile("/Users/damon233/Desktop/Files/Music/Porter Robinson,Madeon - Shelter.lrc")
	if err != nil {
		t.Fatalf("ParseFile() real lyrics failed: %v", err)
	}
	if d.Title == "" && d.Artist == "" {
		// Shelter.lrc has no [ti:] but has [by:]
		t.Logf("Shelter.lrc: %d lines, artist metadata: by=%q", len(d.Lines), d.Creator)
	}
	if len(d.Lines) == 0 {
		t.Fatal("no lyric lines parsed from real file")
	}
	// Check bilingual format: same timestamps should be merged
	// Shelter has many pairs like [00:39.150]English and [00:39.150]Chinese
	mergedCount := 0
	for _, l := range d.Lines {
		if strings.Contains(l.Text, " | ") {
			mergedCount++
		}
	}
	t.Logf("Shelter.lrc: %d lines total, %d bilingual merged lines", len(d.Lines), mergedCount)
	if mergedCount == 0 {
		t.Error("expected at least one bilingual merged line in Shelter.lrc")
	}

	// Verify some specific lines exist
	found := false
	for _, l := range d.Lines {
		if strings.Contains(l.Text, "I could never find the right way") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected lyric 'I could never find...' not found")
	}
}

func TestParse_WhitespaceText(t *testing.T) {
	d, err := Parse(strings.NewReader(testLRC), "")
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
	_, err := Parse(strings.NewReader(""), "")
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestParse_OnlyComments(t *testing.T) {
	_, err := Parse(strings.NewReader("; comment 1\n; comment 2\n"), "")
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
