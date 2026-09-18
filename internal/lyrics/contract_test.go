package lyrics

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"
)

// contractCase 是一个格式的契约样例。
//
// pending 非空表示整条 case 挂起（不参与红绿），用于"已勘定但本轮搁置"的格式。
// 它是显式记账：TestFormatContract 的完备性断言仍然要求该条目存在，所以搁置是
// 记在账上的洞，不是静默跳过。当前没有任何格式挂起（TTML 在任务 4 已激活），
// 字段保留是因为 TestFormatContract 与 TestFormatContractGaps 都读它。
//
// known 列出该格式**当前已知未满足**的不变量/声明（值给出原因）：只有列出来的
// 才被跳过，未列出的必须为绿。与 pending 的区别只是粒度——它让"部分迁移"的格式
// 不必整条被无视。
type contractCase struct {
	parse   func(t *testing.T) *Data
	expect  expectation
	pending string
	known   map[string]string
}

// expectation 是对一个格式样例的规格级声明。它不是现状快照：这些值由
// 各格式的语义契约决定，实现与声明不一致时以实现为错。
type expectation struct {
	// requireEnd 声明该样例的**每一行**都必须 End > 0。
	// false 表示该格式允许（甚至必然）存在无界行。
	requireEnd bool
	// hasParts 声明该样例**必须**产出 Parts（至少一行）；
	// 未声明则断言**不**产出 Parts（每行 Parts == nil）。
	hasParts bool
	// meta 声明该样例的 [ti:]/[ar:] 头必须被解析进 Data（值为 exact 断言由
	// 各格式的单测负责）。
	meta bool
	// looseTiling 声明本格式的 Words 只保证铺满 Part(0) 的**前缀**（上游
	// INV-9：语料 5/717085 行的行尾裸文本不产词，且全部前缀锚定，故卡拉OK
	// 永不误定位）。只有 TTML 声明它，其余格式仍是严格相等。
	looseTiling bool
}

// viaParser 把"用注册表里的解析器解析一段内联文本"包成一个 parse 函数。
func viaParser(name, src string) func(t *testing.T) *Data {
	return func(t *testing.T) *Data {
		t.Helper()
		p, ok := parserMap[name]
		if !ok {
			t.Fatalf("parser %q is not registered", name)
		}
		d, err := p.Parse(strings.NewReader(src), "")
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if d == nil {
			t.Fatal("Parse returned nil Data with nil error")
		}
		return d
	}
}

// TestFormatContract 对每个已注册解析器跑一遍契约。
func TestFormatContract(t *testing.T) {
	for _, name := range AvailableParsers() {
		if _, ok := contractCases[name]; !ok {
			t.Errorf("parser %q has no contract case: add one to contract_cases_test.go", name)
		}
	}
	for _, name := range AvailableParsers() {
		c, ok := contractCases[name]
		if !ok {
			continue
		}
		t.Run(name, func(t *testing.T) {
			if c.pending != "" {
				t.Skipf("pending: %s", c.pending)
			}
			runContract(t, c.parse(t), c)
		})
	}
}

// TestFormatContractGaps 打印当前的缺口记账（各格式的"待迁移量"清单）。
// 它只打日志、从不断言，所以永远不会红；随着任务推进它会自动变短，
// 全部格式激活后（TTML 是最后一个）它应无输出。
func TestFormatContractGaps(t *testing.T) {
	for _, name := range AvailableParsers() {
		c, ok := contractCases[name]
		if !ok {
			t.Logf("GAP %-9s missing contract case", name)
			continue
		}
		if c.pending != "" {
			t.Logf("GAP %-9s pending     %s", name, c.pending)
			continue
		}
		for _, k := range sortedKeys(c.known) {
			t.Logf("GAP %-9s %-11s %s", name, k, c.known[k])
		}
	}
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func runContract(t *testing.T, d *Data, c contractCase) {
	t.Helper()
	skip := map[string]bool{}
	for _, k := range sortedKeys(c.known) {
		skip[k] = true
		t.Logf("known gap %s: %s", k, c.known[k])
	}

	runInvariants(t, d, c.expect, skip)
	runExpectations(t, d, c.expect, skip)
}

// wordsTile reports whether a line's Words tile its display text under the
// contract's C6 rule. The strict form requires ΣWords to equal some Part; when
// looseTiling is set, the fallback accepts ΣWords covering a non-empty prefix
// of Part(0) (upstream INV-9: a bare tail text node next to a timed span
// produces no word, so the words tile only a prefix of the display text). The
// strict form is always tried first because the loose form is a strictly weaker
// guarantee, and the sb.Len() > 0 guard keeps a line whose words concatenate to
// the empty string from passing (an empty string is a prefix of anything).
func wordsTile(l LyricLine, looseTiling bool) bool {
	var sb strings.Builder
	for _, w := range l.Words {
		sb.WriteString(w.Text)
	}
	for j := 0; j < l.PartCount(); j++ {
		if sb.String() == l.Part(j) {
			return true
		}
	}
	// The loose form accepts a strict prefix of the display text, which is the shape
	// the upstream INV-9 bare tail text produces. It deliberately cannot tell such a
	// prefix from a truncated word list - dropping the last fragment still leaves a
	// prefix - so word-list truncation is pinned by the per-format unit tests
	// (len(Words) assertions), not by this invariant.
	return looseTiling && sb.Len() > 0 && strings.HasPrefix(l.Part(0), sb.String())
}

func runInvariants(t *testing.T, d *Data, e expectation, skip map[string]bool) {
	t.Helper()

	// C8：Time 非降序且非负。
	if !skip["C8"] {
		var prev time.Duration
		for i, l := range d.Lines {
			if l.Time < 0 || (i > 0 && l.Time < prev) {
				t.Errorf("C8 line %d: Time %v out of order (previous %v)", i, l.Time, prev)
			}
			prev = l.Time
		}
	}

	for i, l := range d.Lines {
		tag := fmt.Sprintf("line %d (%q)", i, l.Text)

		// C1：Parts 非 nil 时至少 2 段。
		if !skip["C1"] && l.Parts != nil && len(l.Parts) < 2 {
			t.Errorf("C1 %s: len(Parts) = %d, want >= 2", tag, len(l.Parts))
		}

		// C2：Text 与每个 Part 都不含换行。
		if !skip["C2"] {
			if strings.ContainsAny(l.Text, "\n\r") {
				t.Errorf("C2 %s: Text contains a newline: %q", tag, l.Text)
			}
			for j := 0; j < l.PartCount(); j++ {
				if strings.ContainsAny(l.Part(j), "\n\r") {
					t.Errorf("C2 %s: Part(%d) contains a newline: %q", tag, j, l.Part(j))
				}
			}
		}

		// C3：无 Parts 时访问器退化到 Text。
		if !skip["C3"] && l.Parts == nil && (l.PartCount() != 1 || l.Part(0) != l.Text) {
			t.Errorf("C3 %s: PartCount() = %d, Part(0) = %q, Text = %q", tag, l.PartCount(), l.Part(0), l.Text)
		}

		// C5：有界行不能零长度或倒挂。
		if !skip["C5"] && l.End > 0 && l.End <= l.Time {
			t.Errorf("C5 %s: End %v <= Time %v", tag, l.End, l.Time)
		}

		// C6：Words 必须铺满某个 Part，否则面板静默退化为整行高亮。
		if !skip["C6"] && len(l.Words) > 0 {
			if !wordsTile(l, e.looseTiling) {
				var sb strings.Builder
				for _, w := range l.Words {
					sb.WriteString(w.Text)
				}
				t.Errorf("C6 %s: Words %q tile no Part of %v", tag, sb.String(), l.Parts)
			}
		}

		// C7：显示文本非空的行在它自己的 Time 时刻必须可达。
		if !skip["C7"] && strings.TrimSpace(l.Part(0)) != "" {
			saved := d.AgentFilter
			d.AgentFilter = ""
			active := d.ActiveLines(l.Time)
			d.AgentFilter = saved
			if len(active) == 0 {
				t.Errorf("C7 %s: not active at its own Time %v", tag, l.Time)
			}
		}

		// C4：访问器覆盖 [0, PartCount()) 且不 panic。
		if !skip["C4"] {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("C4 %s: Part() panicked: %v", tag, r)
					}
				}()
				if l.PartCount() < 1 {
					t.Errorf("C4 %s: PartCount() = %d, want >= 1", tag, l.PartCount())
				}
				for j := 0; j < l.PartCount(); j++ {
					_ = l.Part(j)
				}
			}()
		}
	}
}

func runExpectations(t *testing.T, d *Data, e expectation, skip map[string]bool) {
	t.Helper()

	if !skip["requireEnd"] && e.requireEnd {
		for i, l := range d.Lines {
			if l.End <= 0 {
				t.Errorf("requireEnd line %d (%q): End == 0, this format carries durations", i, l.Text)
			}
		}
	}

	if !skip["hasParts"] {
		if e.hasParts {
			found := false
			for _, l := range d.Lines {
				if l.Parts != nil {
					found = true
					break
				}
			}
			if !found {
				t.Error("hasParts: no line carried Parts")
			}
		} else {
			for i, l := range d.Lines {
				if l.Parts != nil {
					t.Errorf("no Parts expected, but line %d has %v", i, l.Parts)
				}
			}
		}
	}

	if !skip["meta"] && e.meta {
		if d.Title == "" {
			t.Error("meta: Title is empty, the sample carries [ti:]")
		}
		if d.Artist == "" {
			t.Error("meta: Artist is empty, the sample carries [ar:]")
		}
	}
}

// TestWordsTileLoosePrefix pins the C6 loose form's discriminating power. The
// line reproduces the measured upstream INV-9 shape: a
// timed span followed by a bare tail text node, so the words tile only a strict
// prefix of the display text. Feeding it to the strict form must fail
// (ΣWords != Part(0)) and the loose form must accept it; a line whose words
// concatenate to the empty string must never pass the loose form.
func TestWordsTileLoosePrefix(t *testing.T) {
	// Same shape as the measured probe: Words == ["ユー"], Text == "ユー💀",
	// Parts stays nil so Part(0) degrades to Text per C3.
	prefix := LyricLine{
		Text:  "ユー💀",
		Words: []WordFragment{{Text: "ユー"}},
	}

	if wordsTile(prefix, false) {
		t.Error("strict form accepted a prefix-only tile: ΣWords != Part(0)")
	}
	if !wordsTile(prefix, true) {
		t.Error("loose form rejected a valid prefix-only tile")
	}

	// Discriminator: the sb.Len() > 0 guard. A concatenation that is empty is a
	// prefix of any Part, so without the guard it would pass.
	empty := LyricLine{
		Text:  "x",
		Words: []WordFragment{{Text: ""}},
	}
	if wordsTile(empty, true) {
		t.Error("loose form accepted empty words (empty string is a prefix of anything)")
	}
}

// syltEntry 是 SYLT frame 里的一对 (文本, 绝对毫秒同步时间)。
type syltEntry struct {
	text string
	ms   uint32
}

// syltBody 构造一个 ID3v2 SYLT frame body：encoding 0（ISO-8859-1）、
// language "eng"、contentType 1（lyrics）、timeFormat 1（绝对毫秒）、
// 空内容描述符，随后是若干 (文本, 4 字节大端同步时间) 对。
func syltBody(entries ...syltEntry) []byte {
	buf := []byte{0, 'e', 'n', 'g', 1, 1, 0}
	for _, e := range entries {
		buf = append(buf, e.text...)
		buf = append(buf, 0)
		var sync [4]byte
		binary.BigEndian.PutUint32(sync[:], e.ms)
		buf = append(buf, sync[:]...)
	}
	return buf
}

// TestSortedKeys 钉住缺口报告与跳过表依赖的顺序：报告要能被后续任务逐行 diff。
// 它同时覆盖 sortedKeys 的空表、单条与需要插入排序的分支。
func TestSortedKeys(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]string
		want string
	}{
		{"empty", nil, ""},
		{"single", map[string]string{"meta": "m"}, "meta"},
		{
			"unsorted",
			map[string]string{"requireEnd": "r", "C7": "7", "meta": "m", "hasParts": "h", "C6": "6"},
			"C6,C7,hasParts,meta,requireEnd",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(sortedKeys(tc.in), ","); got != tc.want {
				t.Errorf("sortedKeys(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSyltBody 钉住 embedded 样例依赖的 frame 布局：7 字节头（ISO-8859-1、"eng"、
// contentType 1、timeFormat 1、空内容描述符），其后每条是 NUL 结尾文本 + 4 字节大端
// 同步时间。头用字节字面量断言，条目则走 parseSYLT 往返核对。
func TestSyltBody(t *testing.T) {
	if got, want := syltBody(), []byte{0, 'e', 'n', 'g', 1, 1, 0}; !bytes.Equal(got, want) {
		t.Errorf("syltBody() = %v, want the bare 7-byte header %v", got, want)
	}

	d := parseSYLT(syltBody(syltEntry{"ab", 1000}, syltEntry{"c", 2}))
	if d == nil || len(d.Lines) != 2 {
		t.Fatalf("parseSYLT(syltBody(...)) = %v, want 2 lines", d)
	}
	if d.Lines[0].Text != "ab" || d.Lines[0].Time != 1000*time.Millisecond {
		t.Errorf("line 0 = %q @ %v, want %q @ 1s", d.Lines[0].Text, d.Lines[0].Time, "ab")
	}
	if d.Lines[1].Text != "c" || d.Lines[1].Time != 2*time.Millisecond {
		t.Errorf("line 1 = %q @ %v, want %q @ 2ms", d.Lines[1].Text, d.Lines[1].Time, "c")
	}
}
