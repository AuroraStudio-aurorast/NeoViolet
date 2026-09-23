package lyrics

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"
)

// contractCase is one format's contract sample.
//
// A non-empty pending suspends the whole case (it takes no part in red/green),
// for a format that has been scoped but is shelved this round. It is explicit
// bookkeeping: TestFormatContract's completeness assertion still requires the
// entry to exist, so a shelved format is a hole on the books rather than a
// silent skip. No format is pending today; the field stays because both
// TestFormatContract and TestFormatContractGaps read it.
//
// known lists the invariants/declarations this format is **currently known not
// to satisfy** (the value gives the reason): only the listed ones are skipped,
// everything else must be green. The difference from pending is only
// granularity - it lets a partially migrated format avoid being written off
// whole.
type contractCase struct {
	parse   func(t *testing.T) *Data
	expect  expectation
	pending string
	known   map[string]string
}

// expectation is a specification-level claim about one format sample. It is not
// a snapshot of current behaviour: these values follow from each format's
// semantic contract, so when the implementation disagrees with the declaration,
// the implementation is wrong.
type expectation struct {
	// requireEnd declares that **every line** of this sample must have End > 0.
	// false means the format allows (or necessarily has) unbounded lines.
	requireEnd bool
	// hasParts declares that this sample **must** produce Parts (on at least one
	// line); otherwise the assertion is that it does **not** (every line has
	// Parts == nil).
	hasParts bool
	// translations declares that this format's Parts after the first are
	// translations of the first, which is what lets the panel style them as
	// subordinate rows. It is a claim about the format's part semantics, so a
	// format whose parts are author line breaks must leave it false: tinting those
	// would render one sentence in two weights.
	translations bool
	// meta declares that this sample's [ti:]/[ar:] header must be parsed into
	// Data (exact values are asserted by the per-format unit tests).
	meta bool
	// looseTiling declares that this format's Words only guarantee covering a
	// **prefix** of Part(0) (upstream INV-9: on 5/717085 corpus lines a bare tail
	// text node produces no word, and every such case is prefix-anchored, so
	// karaoke never mispositions). Only TTML declares it; the other formats stay
	// strict.
	looseTiling bool
}

// viaParser wraps "parse an inline string with the registered parser" into a
// parse function.
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

// TestFormatContract runs the contract over every registered parser.
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

// TestFormatContractGaps prints the current gap accounting (each format's
// "still to migrate" list). It only logs and never asserts, so it can never go
// red; it shortens on its own as the work lands, and once every format is active
// it should print nothing.
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

// runInvariants checks the contract invariants every registered parser must
// satisfy. The ids are stable labels - the inline checks below and the
// per-format unit tests cite them by number - so the definitions live here.
//
//	C1  When Parts is non-nil it holds at least 2 parts; a single-part line has
//	    no reason to carry the slice and should degrade to Text instead (C3).
//	C2  Text and every Part are free of newlines. A raw \n is a hard line break
//	    for the renderers: it adds a row and breaks the frame height.
//	C3  With no Parts, the accessors degrade to Text: PartCount() == 1 and
//	    Part(0) == Text.
//	C4  The accessors cover [0, PartCount()), never panic, and PartCount() is at
//	    least 1.
//	C5  A bounded line is neither zero-length nor inverted: when End > 0 it must
//	    also hold that End > Time.
//	C6  Words tile a Part, so the panel's word-by-word highlighting cannot
//	    silently degrade to whole-line highlighting. wordsTile holds both the
//	    strict form and the loose one.
//	C7  A line whose display text is non-empty is reachable at its own Time:
//	    ActiveLines(Time) returns it once the agent filter is cleared.
//	C8  Time is non-negative and non-decreasing across Lines.
//	C9  Words are non-decreasing in Time. C6 only compares the concatenated
//	    text, so a reversed word timeline passes it; the panel splits Words into
//	    played/rest by time and re-concatenates each, so once the timeline is
//	    reversed played+rest no longer equals Text and word-by-word highlighting
//	    silently degrades to whole-line highlighting.
func runInvariants(t *testing.T, d *Data, e expectation, skip map[string]bool) {
	t.Helper()

	// C8: Time is non-negative and non-decreasing.
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

		// C1: Parts, when present, has at least 2 entries.
		if !skip["C1"] && l.Parts != nil && len(l.Parts) < 2 {
			t.Errorf("C1 %s: len(Parts) = %d, want >= 2", tag, len(l.Parts))
		}

		// C2: Text and every Part are newline-free.
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

		// C3: With no Parts the accessors degrade to Text.
		if !skip["C3"] && l.Parts == nil && (l.PartCount() != 1 || l.Part(0) != l.Text) {
			t.Errorf("C3 %s: PartCount() = %d, Part(0) = %q, Text = %q", tag, l.PartCount(), l.Part(0), l.Text)
		}

		// C5: A bounded line is neither zero-length nor inverted.
		if !skip["C5"] && l.End > 0 && l.End <= l.Time {
			t.Errorf("C5 %s: End %v <= Time %v", tag, l.End, l.Time)
		}

		// C6: Words tile a Part (see wordsTile).
		if !skip["C6"] && len(l.Words) > 0 {
			if !wordsTile(l, e.looseTiling) {
				var sb strings.Builder
				for _, w := range l.Words {
					sb.WriteString(w.Text)
				}
				t.Errorf("C6 %s: Words %q tile no Part of %v", tag, sb.String(), l.Parts)
			}
		}

		// C9: Words are non-decreasing in Time.
		if !skip["C9"] {
			for i := 1; i < len(l.Words); i++ {
				if l.Words[i].Time < l.Words[i-1].Time {
					t.Errorf("C9 %s: Words[%d] at %v is before Words[%d] at %v",
						tag, i, l.Words[i].Time, i-1, l.Words[i-1].Time)
				}
			}
		}

		// C7: A non-empty line is reachable at its own Time.
		if !skip["C7"] && strings.TrimSpace(l.Part(0)) != "" {
			saved := d.AgentFilter
			d.AgentFilter = ""
			active := d.ActiveLines(l.Time)
			d.AgentFilter = saved
			if len(active) == 0 {
				t.Errorf("C7 %s: not active at its own Time %v", tag, l.Time)
			}
		}

		// C4: The accessors cover [0, PartCount()) without panicking.
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

	if !skip["translations"] {
		if e.translations {
			if !d.TranslationsInParts {
				t.Error("translations: this format's parts are translations, but Data does not declare it")
			}
		} else if d.TranslationsInParts {
			t.Error("translations: Data declares translated parts, but this format's parts are line breaks")
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

// syltEntry is one (text, absolute-millisecond sync time) pair inside a SYLT
// frame.
type syltEntry struct {
	text string
	ms   uint32
}

// syltBody builds an ID3v2 SYLT frame body: encoding 0 (ISO-8859-1), language
// "eng", contentType 1 (lyrics), timeFormat 1 (absolute milliseconds), an empty
// content descriptor, then the (text, 4-byte big-endian sync time) pairs.
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

// TestSortedKeys pins the ordering the gap report and the skip table depend on:
// the report has to stay line-diffable from run to run. It also covers
// sortedKeys' empty, single-entry and insertion-sort branches.
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

// TestSyltBody pins the frame layout the embedded sample depends on: a 7-byte
// header (ISO-8859-1, "eng", contentType 1, timeFormat 1, an empty content
// descriptor), then each entry as NUL-terminated text + a 4-byte big-endian sync
// time. The header is asserted as bytes; the entries round-trip through parseSYLT.
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
