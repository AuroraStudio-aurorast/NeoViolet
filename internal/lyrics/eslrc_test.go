package lyrics

import (
	"strings"
	"testing"
	"time"
)

func TestESLRC_WordStartTimesFollowPreviousBracket(t *testing.T) {
	// F2：片段的起点是它**前面**那个 bracket，"[00:00.000]" 表示"沿用上一个
	// 已知边界"。旧实现取后一个 bracket（词的终点），整行逐字因此滞后一个词。
	d, err := parseESLRC(testESLRC)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	line0 := d.Lines[0]
	want := []struct {
		text string
		at   time.Duration
	}{
		{"I", 39345}, {" ", 39548}, {"could", 39548}, {" ", 39938},
		{"ne", 39938}, {"ver", 40198}, {" ", 40380}, {"find", 40380},
		{" ", 40770}, {"the", 40770}, {" ", 41147}, {"right", 41147},
		{" ", 41492}, {"way", 41492}, {" ", 41935}, {"to", 41935},
		{" ", 42182}, {"tell", 42182}, {" ", 42720}, {"you", 42720},
	}
	if len(line0.Words) != len(want) {
		t.Fatalf("len(Words) = %d, want %d", len(line0.Words), len(want))
	}
	for i, w := range want {
		if line0.Words[i].Text != w.text || line0.Words[i].Time != w.at*time.Millisecond {
			t.Errorf("Words[%d] = %q @ %v, want %q @ %v",
				i, line0.Words[i].Text, line0.Words[i].Time, w.text, w.at*time.Millisecond)
		}
	}
	if line0.End != 43071*time.Millisecond {
		t.Errorf("End = %v, want 43071ms", line0.End)
	}

	// C6 的本地形式：Words 必须铺满 Text（旧实现里空格不入 Words，铺不满）。
	var sb strings.Builder
	for _, w := range line0.Words {
		sb.WriteString(w.Text)
	}
	if sb.String() != line0.Text {
		t.Errorf("Words %q do not tile Text %q", sb.String(), line0.Text)
	}
}

func TestESLRC_TrailingUntimedTextBecomesAFragment(t *testing.T) {
	// 尾段必须成为片段：它同时补上 Time 与 tiling，否则 "fly" 会粘在 "could"
	// 后面且铺不满（旧的 text = "I couldfly"）。空格片段也进入 Words，所以这里
	// 是 4 个片段（"I"、" "、"could"、"fly"），"fly" 在 Words[3]。
	const src = "[00:01.000]I[00:01.548] [00:00.000]could[00:01.938]fly"
	d, err := parseESLRC(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	line := d.Lines[0]
	if line.Text != "I couldfly" {
		t.Errorf("Text = %q, want %q", line.Text, "I couldfly")
	}
	if len(line.Words) != 4 {
		t.Fatalf("len(Words) = %d, want 4", len(line.Words))
	}
	if line.Words[3].Text != "fly" || line.Words[3].Time != 1938*time.Millisecond {
		t.Errorf("Words[3] = %q @ %v, want 'fly' @ 1938ms", line.Words[3].Text, line.Words[3].Time)
	}
	var sb strings.Builder
	for _, w := range line.Words {
		sb.WriteString(w.Text)
	}
	if sb.String() != line.Text {
		t.Errorf("Words %q do not tile Text %q", sb.String(), line.Text)
	}
}

func TestESLRC_HeaderMetadata(t *testing.T) {
	const src = "[ti:Title]\n[ar:Artist]\n[offset:0]\n[00:01.000]Hello[00:01.500] world"
	d, err := parseESLRC(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if d.Title != "Title" || d.Artist != "Artist" {
		t.Errorf("metadata = %q/%q, want Title/Artist", d.Title, d.Artist)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1 (metadata lines are not lyrics)", len(d.Lines))
	}
}

func TestESLRC_LineWithoutWordBracketsStaysUnbounded(t *testing.T) {
	// LRC 形状的 .eslrc（仓库 fixture 就是这种）：Text 仍 trim、Words 仍为 nil、
	// End 仍为 0。这是可接受降级，不是缺陷。
	const src = "[00:01.00]  first line  \n[00:05.00]second line\n"
	d, err := parseESLRC(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2", len(d.Lines))
	}
	if d.Lines[0].Text != "first line" {
		t.Errorf("Text = %q, want %q", d.Lines[0].Text, "first line")
	}
	if d.Lines[0].Words != nil || d.Lines[0].End != 0 {
		t.Errorf("Words = %v, End = %v, want nil/0", d.Lines[0].Words, d.Lines[0].End)
	}
}

func TestESLRC_CRLFLineEndings(t *testing.T) {
	// R2：骨架只有循环顶部一次 TrimSpace，CRLF 的 \r 不应漏进 Text/Words。
	const src = "[00:01.000]I[00:01.548] [00:00.000]could[00:01.938]fly[00:02.000]\r\n" +
		"[00:03.000]Hi[00:03.500]\r\n"
	d, err := parseESLRC(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2", len(d.Lines))
	}
	for i, line := range d.Lines {
		if strings.Contains(line.Text, "\r") {
			t.Errorf("line %d Text %q contains \\r", i, line.Text)
		}
		for j, w := range line.Words {
			if strings.Contains(w.Text, "\r") {
				t.Errorf("line %d Words[%d] %q contains \\r", i, j, w.Text)
			}
		}
		var sb strings.Builder
		for _, w := range line.Words {
			sb.WriteString(w.Text)
		}
		if sb.String() != line.Text {
			t.Errorf("line %d Words %q do not tile Text %q", i, sb.String(), line.Text)
		}
		if line.End <= 0 {
			t.Errorf("line %d End = %v, want > 0", i, line.End)
		}
	}
}

func TestESLRC_OffsetShiftsTimes(t *testing.T) {
	// R4：offset 必须平移 Time/Words/End，而不是只写进 d.Offset。
	const src = "[offset:250]\n[00:01.000]Hello[00:01.500] world[00:02.000]\n"
	d, err := parseESLRC(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	line := d.Lines[0]
	if line.Time != 1250*time.Millisecond {
		t.Errorf("Time = %v, want 1250ms", line.Time)
	}
	if line.Text != "Hello world" {
		t.Errorf("Text = %q, want %q", line.Text, "Hello world")
	}
	if len(line.Words) != 2 {
		t.Fatalf("len(Words) = %d, want 2", len(line.Words))
	}
	if line.Words[0].Time != 1250*time.Millisecond {
		t.Errorf("Words[0].Time = %v, want 1250ms", line.Words[0].Time)
	}
	if line.Words[1].Time != 1750*time.Millisecond {
		t.Errorf("Words[1].Time = %v, want 1750ms", line.Words[1].Time)
	}
	if line.End != 2250*time.Millisecond {
		t.Errorf("End = %v, want 2250ms", line.End)
	}
}

func TestESLRC_LRCLineWithOffsetShiftsTime(t *testing.T) {
	// 同类缺口：applyHeaderField 会写 d.Offset，但 !timed（LRC 形状）出口
	// 以前直接用 lineStart，不读 d.Offset。混合文件 + [offset:] 时一部分行
	// 平移、一部分不平移，sortLyricLines 会按混合基准排序 → 行序错乱。
	// 这条断言钉住：LRC 形状行的 Time 也走 shiftTime，且 Words 仍为 nil、
	// End 仍为 0（R6 冻结的旧行为不能变）。
	const src = "[offset:250]\n[00:01.00]Hello\n"
	d, err := parseESLRC(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	line := d.Lines[0]
	if line.Time != 1250*time.Millisecond {
		t.Errorf("Time = %v, want 1250ms", line.Time)
	}
	if line.Words != nil {
		t.Errorf("Words = %v, want nil", line.Words)
	}
	if line.End != 0 {
		t.Errorf("End = %v, want 0", line.End)
	}
}

func TestESLRC_WordBracketEqualToLineStartHasNoEnd(t *testing.T) {
	// R7：prevBoundary > lineStart 的"假"侧。词括号等于行首时，末边界没有
	// 越过 lineStart，End 应为 0（与 timed == false 的整行出口是不同分支）。
	const src = "[00:01.000]I[00:01.000]you"
	d, err := parseESLRC(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	line := d.Lines[0]
	if line.Text != "Iyou" {
		t.Errorf("Text = %q, want %q", line.Text, "Iyou")
	}
	if len(line.Words) != 2 {
		t.Fatalf("len(Words) = %d, want 2", len(line.Words))
	}
	if line.End != 0 {
		t.Errorf("End = %v, want 0", line.End)
	}
}
