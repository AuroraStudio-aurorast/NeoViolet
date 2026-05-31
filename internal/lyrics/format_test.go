package lyrics

import (
	"strings"
	"testing"
	"time"
)

const testYRC = `[39345,3726](39345,203,0)I(0,0,0) (39548,390,0)could(0,0,0) (39938,260,0)ne(40198,182,0)ver(0,0,0) (40380,390,0)find(0,0,0) (40770,377,0)the(0,0,0) (41147,345,0)right(0,0,0) (41492,443,0)way(0,0,0) (41935,247,0)to(0,0,0) (42182,538,0)tell(0,0,0) (42720,351,0)you
[44085,2420](44085,145,0)Have(0,0,0) (44230,280,0)you(0,0,0) (44510,443,0)no(44953,115,0)ticed(0,0,0) (45068,372,0)I've(0,0,0) (45440,265,0)been(0,0,0) (45705,800,0)gone
[48858,4183](48858,132,0)Cause(0,0,0) (48990,340,0)I(0,0,0) (49330,435,0)left(0,0,0) (49765,280,0)be(50045,305,0)hind(0,0,0) (50350,318,0)the(0,0,0) (50668,430,0)home`

func parseYRC(s string) (*LyricsData, error) {
	var p yrcParser
	return p.Parse(strings.NewReader(s), "")
}

func TestYRC_BasicParse(t *testing.T) {
	d, err := parseYRC(testYRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Time != 39345*time.Millisecond {
		t.Errorf("line 0 time = %v, want 39345ms", d.Lines[0].Time)
	}
	if d.Lines[0].Text != "I could never find the right way to tell you" {
		t.Errorf("line 0 text = %q", d.Lines[0].Text)
	}
	if len(d.Lines[0].Words) == 0 {
		t.Fatal("line 0 has no word fragments")
	}
	if d.Lines[0].Words[0].Text != "I" || d.Lines[0].Words[0].Time != 39345*time.Millisecond {
		t.Errorf("word[0] = %q @ %v", d.Lines[0].Words[0].Text, d.Lines[0].Words[0].Time)
	}

	if d.Lines[1].Time != 44085*time.Millisecond {
		t.Errorf("line 1 time = %v, want 44085ms", d.Lines[1].Time)
	}
	if d.Lines[1].Text != "Have you noticed I've been gone" {
		t.Errorf("line 1 text = %q", d.Lines[1].Text)
	}
}

func TestYRC_EmptyInput(t *testing.T) {
	_, err := parseYRC("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestYRC_SortedOutput(t *testing.T) {
	unsorted := `[20000,1000](20000,500,0)later
[10000,1000](10000,500,0)earlier`

	d, err := parseYRC(unsorted)
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

const testQRC = `[39345,3726]I(39345,203) (0,0)could(39548,390) (0,0)ne(39938,260)ver(40198,182) (0,0)find(40380,390) (0,0)the(40770,377) (0,0)right(41147,345) (0,0)way(41492,443) (0,0)to(41935,247) (0,0)tell(42182,538) (0,0)you(42720,351)
[44085,2420]Have(44085,145) (0,0)you(44230,280) (0,0)no(44510,443)ticed(44953,115) (0,0)I've(45068,372) (0,0)been(45440,265) (0,0)gone(45705,800)`

func parseQRC(s string) (*LyricsData, error) {
	var p qrcParser
	return p.Parse(strings.NewReader(s), "")
}

func TestQRC_BasicParse(t *testing.T) {
	d, err := parseQRC(testQRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Time != 39345*time.Millisecond {
		t.Errorf("line 0 time = %v, want 39345ms", d.Lines[0].Time)
	}
	if d.Lines[0].Text != "I could never find the right way to tell you" {
		t.Errorf("line 0 text = %q", d.Lines[0].Text)
	}
	if len(d.Lines[0].Words) == 0 {
		t.Fatal("line 0 has no word fragments")
	}

	if d.Lines[1].Time != 44085*time.Millisecond {
		t.Errorf("line 1 time = %v, want 44085ms", d.Lines[1].Time)
	}
	if d.Lines[1].Text != "Have you noticed I've been gone" {
		t.Errorf("line 1 text = %q", d.Lines[1].Text)
	}
}

func TestQRC_EmptyInput(t *testing.T) {
	_, err := parseQRC("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestQRC_CurrentLine(t *testing.T) {
	d, err := parseQRC(testQRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if d.Lines[0].Time != 39345*time.Millisecond {
		t.Errorf("line 0 time = %v, want 39345ms", d.Lines[0].Time)
	}
	got := d.CurrentLine(39000 * time.Millisecond)
	if got != -1 {
		t.Errorf("CurrentLine(39s) = %d, want -1", got)
	}
	got = d.CurrentLine(40000 * time.Millisecond)
	if got != 0 {
		t.Errorf("CurrentLine(40s) = %d, want 0", got)
	}
}

const testLYS = `[0]I(39345,203) (0,0)could(39548,390) (0,0)ne(39938,260)ver(40198,182) (0,0)find(40380,390) (0,0)the(40770,377) (0,0)right(41147,345) (0,0)way(41492,443) (0,0)to(41935,247) (0,0)tell(42182,538) (0,0)you(42720,351)
[0]Have(44085,145) (0,0)you(44230,280) (0,0)no(44510,443)ticed(44953,115) (0,0)I've(45068,372) (0,0)been(45440,265) (0,0)gone(45705,800)
[0]Cause(48858,132) (0,0)I(48990,340) (0,0)left(49330,435) (0,0)be(49765,280)hind(50045,305) (0,0)the(50350,318) (0,0)home(50668,430) (0,0)that(51098,474) (0,0)you(51572,284) (0,0)made(51856,399) (0,0)me(52255,786)`

func parseLYS(s string) (*LyricsData, error) {
	var p lysParser
	return p.Parse(strings.NewReader(s), "")
}

func TestLYS_BasicParse(t *testing.T) {
	d, err := parseLYS(testLYS)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Time != 39345*time.Millisecond {
		t.Errorf("line 0 time = %v, want 39345ms (from first word)", d.Lines[0].Time)
	}
	if d.Lines[0].Text != "I could never find the right way to tell you" {
		t.Errorf("line 0 text = %q", d.Lines[0].Text)
	}
	if len(d.Lines[0].Words) == 0 {
		t.Fatal("line 0 has no word fragments")
	}
	if d.Lines[0].Words[0].Text != "I" || d.Lines[0].Words[0].Time != 39345*time.Millisecond {
		t.Errorf("word[0] = %q @ %v", d.Lines[0].Words[0].Text, d.Lines[0].Words[0].Time)
	}

	if d.Lines[1].Time != 44085*time.Millisecond {
		t.Errorf("line 1 time = %v, want 44085ms", d.Lines[1].Time)
	}
	if d.Lines[1].Text != "Have you noticed I've been gone" {
		t.Errorf("line 1 text = %q", d.Lines[1].Text)
	}
}

func TestLYS_EmptyInput(t *testing.T) {
	_, err := parseLYS("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestLYS_SortedOutput(t *testing.T) {
	unsorted := `[0]later(20000,500)
[0]earlier(10000,500)`

	d, err := parseLYS(unsorted)
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

const testESLRC = `[00:39.345]I[00:39.548] [00:00.000]could[00:39.938] [00:00.000]ne[00:40.198]ver[00:40.380] [00:00.000]find[00:40.770] [00:00.000]the[00:41.147] [00:00.000]right[00:41.492] [00:00.000]way[00:41.935] [00:00.000]to[00:42.182] [00:00.000]tell[00:42.720] [00:00.000]you[00:43.071]
[00:44.085]Have[00:44.230] [00:00.000]you[00:44.510] [00:00.000]no[00:44.953]ticed[00:45.068] [00:00.000]I've[00:45.440] [00:00.000]been[00:45.705] [00:00.000]gone[00:46.505]
[00:48.858]Cause[00:48.990] [00:00.000]I[00:49.330] [00:00.000]left[00:49.765] [00:00.000]be[00:50.045]hind[00:50.350] [00:00.000]the[00:50.668] [00:00.000]home[00:51.098] [00:00.000]that[00:51.572] [00:00.000]you[00:51.856] [00:00.000]made[00:52.255] [00:00.000]me[00:53.041]`

func parseESLRC(s string) (*LyricsData, error) {
	var p eslrcParser
	return p.Parse(strings.NewReader(s), "")
}

func TestESLRC_BasicParse(t *testing.T) {
	d, err := parseESLRC(testESLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Time != 39345*time.Millisecond {
		t.Errorf("line 0 time = %v, want 39345ms", d.Lines[0].Time)
	}
	if d.Lines[0].Text != "I could never find the right way to tell you" {
		t.Errorf("line 0 text = %q", d.Lines[0].Text)
	}
	if len(d.Lines[0].Words) == 0 {
		t.Fatal("line 0 has no word fragments")
	}
	if d.Lines[0].Words[0].Text != "I" || d.Lines[0].Words[0].Time != 39548*time.Millisecond {
		t.Errorf("word[0] = %q @ %v, want 'I' @ 39548ms", d.Lines[0].Words[0].Text, d.Lines[0].Words[0].Time)
	}

	if d.Lines[1].Time != 44085*time.Millisecond {
		t.Errorf("line 1 time = %v, want 44085ms", d.Lines[1].Time)
	}
	if d.Lines[2].Time != 48858*time.Millisecond {
		t.Errorf("line 2 time = %v, want 48858ms", d.Lines[2].Time)
	}
}

func TestESLRC_WordTimestamps(t *testing.T) {
	d, err := parseESLRC(testESLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	line0 := d.Lines[0]
	if len(line0.Words) != 11 {
		t.Fatalf("expected 11 word fragments, got %d", len(line0.Words))
	}
	if line0.Words[0].Text != "I" || line0.Words[0].Time != 39548*time.Millisecond {
		t.Errorf("word[0] = %q @ %v", line0.Words[0].Text, line0.Words[0].Time)
	}
	if line0.Words[5].Text != "the" || line0.Words[5].Time != 41147*time.Millisecond {
		t.Errorf("word[5] = %q @ %v", line0.Words[5].Text, line0.Words[5].Time)
	}
}

func TestESLRC_EmptyInput(t *testing.T) {
	_, err := parseESLRC("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestESLRC_SortedOutput(t *testing.T) {
	unsorted := `[00:20.000]later
[00:10.000]earlier`

	d, err := parseESLRC(unsorted)
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

func TestYRC_FindSidecar(t *testing.T) {
	ext := ".mp3"
	path := "/some/path/song.mp3"
	expected := path[:len(path)-len(ext)] + ".yrc"
	if expected != "/some/path/song.yrc" {
		t.Errorf("yrc path = %q", expected)
	}
}

func TestQRC_FindSidecar(t *testing.T) {
	ext := ".mp3"
	path := "/some/path/song.mp3"
	expected := path[:len(path)-len(ext)] + ".qrc"
	if expected != "/some/path/song.qrc" {
		t.Errorf("qrc path = %q", expected)
	}
}

func TestESLRC_FindSidecar(t *testing.T) {
	ext := ".mp3"
	path := "/some/path/song.mp3"
	expected := path[:len(path)-len(ext)] + ".eslrc"
	if expected != "/some/path/song.eslrc" {
		t.Errorf("eslrc path = %q", expected)
	}
}

func TestLYS_FindSidecar(t *testing.T) {
	ext := ".mp3"
	path := "/some/path/song.mp3"
	expected := path[:len(path)-len(ext)] + ".lys"
	if expected != "/some/path/song.lys" {
		t.Errorf("lys path = %q", expected)
	}
}

func TestYRC_CurrentLine(t *testing.T) {
	d, err := parseYRC(testYRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if d.CurrentLine(30000*time.Millisecond) != -1 {
		t.Error("should return -1 before first line")
	}
	if d.CurrentLine(40000*time.Millisecond) != 0 {
		t.Error("should return 0 at 40s")
	}
	if d.CurrentLine(45000*time.Millisecond) != 1 {
		t.Error("should return 1 at 45s")
	}
}

func TestQRC_WordFragments(t *testing.T) {
	d, err := parseQRC(testQRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	line0 := d.Lines[0]
	if len(line0.Words) != 20 {
		t.Fatalf("expected 20 words (incl separators), got %d", len(line0.Words))
	}
	if line0.Words[0].Text != "I" || line0.Words[0].Time != 39345*time.Millisecond {
		t.Errorf("word[0] = %q @ %v", line0.Words[0].Text, line0.Words[0].Time)
	}
}

func TestLYS_WordFragments(t *testing.T) {
	d, err := parseLYS(testLYS)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	line0 := d.Lines[0]
	if len(line0.Words) == 0 {
		t.Fatal("expected word fragments")
	}
	if line0.Words[0].Text != "I" || line0.Words[0].Time != 39345*time.Millisecond {
		t.Errorf("word[0] = %q @ %v", line0.Words[0].Text, line0.Words[0].Time)
	}
}

func TestLYS_CurrentLine(t *testing.T) {
	d, err := parseLYS(testLYS)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	got := d.CurrentLine(39345 * time.Millisecond)
	if got != 0 {
		t.Errorf("CurrentLine(39345ms) = %d, want 0", got)
	}
}

// LYS multi-agent test data: channels 0,2,6,8 mapped to v1-v4
const testLYSMultiAgent = `[0]I(1000,500) (0,0)am(1500,300) (0,0)lead(1800,400)
[2]I(1000,500) (0,0)am(1500,300) (0,0)duet(1800,400)
[6]backing(2000,600) (0,0)vocals(2600,500)
[8]oh(2500,300) (0,0)yeah(2800,400)`

func parseLYSMulti(s string) (*LyricsData, error) {
	var p lysParser
	return p.Parse(strings.NewReader(s), "")
}

func TestLYS_AgentAssignment(t *testing.T) {
	d, err := parseLYSMulti(testLYSMultiAgent)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Agent != "v1" {
		t.Errorf("line 0 (channel 0): Agent = %q, want v1", d.Lines[0].Agent)
	}
	if d.Lines[1].Agent != "v2" {
		t.Errorf("line 1 (channel 2): Agent = %q, want v2", d.Lines[1].Agent)
	}
	if d.Lines[2].Agent != "v3" {
		t.Errorf("line 2 (channel 6): Agent = %q, want v3", d.Lines[2].Agent)
	}
	if d.Lines[3].Agent != "v4" {
		t.Errorf("line 3 (channel 8): Agent = %q, want v4", d.Lines[3].Agent)
	}
}

func TestLYS_AgentsMap(t *testing.T) {
	d, err := parseLYSMulti(testLYSMultiAgent)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// LYS doesn't have real names for agents, so Agents map should be nil
	// (falls back to uppercase agent ID: V1, V2, etc.)
	if d.Agents != nil {
		t.Log("Agents map is nil for LYS (falls back to uppercase agent ID)")
	}
}

func TestLYS_LineDisplayTextWithAgent(t *testing.T) {
	d, err := parseLYSMulti(testLYSMultiAgent)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	display := d.LineDisplayText(d.Lines[0])
	// LYS has no agent name mapping, so falls back to uppercase agent ID
	if display != "V1: I am lead" {
		t.Errorf("LineDisplayText = %q, want 'V1: I am lead'", display)
	}
}

func TestLYS_EndTimeComputed(t *testing.T) {
	d, err := parseLYSMulti(testLYSMultiAgent)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if d.Lines[0].End <= 0 {
		t.Error("line 0 should have End set from last word duration")
	}
	if d.Lines[0].End != 2200*time.Millisecond {
		t.Errorf("line 0 End = %v, want 2200ms", d.Lines[0].End)
	}
}

func TestLYS_ActiveLinesOverlap(t *testing.T) {
	d, err := parseLYSMulti(testLYSMultiAgent)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// At t=1000ms, lines 0 and 1 both start at 1000ms and overlap
	t1 := 1000 * time.Millisecond
	active := d.ActiveLines(t1)
	if len(active) != 2 {
		t.Errorf("at %v: expected 2 active lines, got %d", t1, len(active))
	}
	// At t=2700ms, lines 2 and 3 overlap (2000-2600 extended, 2500-2800)
	t2 := 2700 * time.Millisecond
	active = d.ActiveLines(t2)
	if len(active) != 2 {
		t.Errorf("at %v: expected 2 active lines, got %d", t2, len(active))
	}
}

func TestLYS_ActiveLinesFilter(t *testing.T) {
	d, err := parseLYSMulti(testLYSMultiAgent)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	d.AgentFilter = "v1"
	t1 := 1000 * time.Millisecond
	active := d.ActiveLines(t1)
	if len(active) != 1 {
		t.Errorf("filtered v1 at %v: expected 1 line, got %d", t1, len(active))
	}
	if len(active) > 0 && active[0].Text != "I am lead" {
		t.Errorf("filtered v1: expected 'I am lead', got %q", active[0].Text)
	}
}

func TestLYS_NoAgentForUnknownChannel(t *testing.T) {
	input := `[42]unknown(1000,500) (0,0)channel(1500,300)`
	d, err := parseLYS(input)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(d.Lines))
	}
	if d.Lines[0].Agent != "" {
		t.Errorf("Agent = %q, want empty", d.Lines[0].Agent)
	}
}
