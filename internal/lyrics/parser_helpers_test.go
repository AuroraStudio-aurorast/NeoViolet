package lyrics

import (
	"strings"
	"testing"
	"time"
)

func TestLRCFIELD(t *testing.T) {
	cases := []struct {
		content string
		key     string
		val     string
		ok      bool
	}{
		{"ti:Contract", "ti", "Contract", true},
		{"AR:  Someone  ", "ar", "Someone", true},
		{"offset:250", "offset", "250", true},
		{"1000,2000", "", "", false},
		{"00:39.345", "00", "39.345", true},
		{":noname", "", "", false},
		{"noColon", "", "", false},
	}
	for _, tc := range cases {
		key, val, ok := lrcField(tc.content)
		if ok != tc.ok || key != tc.key || val != tc.val {
			t.Errorf("lrcField(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.content, key, val, ok, tc.key, tc.val, tc.ok)
		}
	}
}

func TestApplyHeaderField(t *testing.T) {
	var d Data
	if !applyHeaderField(&d, "ti", "Contract") || d.Title != "Contract" {
		t.Errorf("ti: title=%q", d.Title)
	}
	if !applyHeaderField(&d, "ar", "Tester") || d.Artist != "Tester" {
		t.Errorf("ar: artist=%q", d.Artist)
	}
	if !applyHeaderField(&d, "al", "Album") || d.Album != "Album" {
		t.Errorf("al: album=%q", d.Album)
	}
	if !applyHeaderField(&d, "au", "Author") || d.Author != "Author" {
		t.Errorf("au: author=%q", d.Author)
	}
	if !applyHeaderField(&d, "by", "Creator") || d.Creator != "Creator" {
		t.Errorf("by: creator=%q", d.Creator)
	}
	if !applyHeaderField(&d, "offset", "250") || d.Offset != 250 {
		t.Errorf("offset: offset=%d", d.Offset)
	}
	// A later non-offset field must not reset the offset already read.
	if !applyHeaderField(&d, "ti", "X") || d.Offset != 250 {
		t.Errorf("offset must survive a later ti field: offset=%d", d.Offset)
	}
	if applyHeaderField(&d, "1000,2000", "x") {
		t.Error("an unknown key must report ok=false so callers fall through")
	}
	if applyHeaderField(&d, "offset", "notanumber") || d.Offset != 250 {
		t.Errorf("a malformed offset must report ok=false and leave the offset untouched: offset=%d", d.Offset)
	}
	if applyHeaderField(&d, "re", "x") {
		t.Error("an unknown metadata key must report ok=false")
	}
	// An empty key is not a metadata field: it must be silently ignored,
	// leaving any already-read offset untouched.
	if applyHeaderField(&d, "", "250") || d.Offset != 250 {
		t.Errorf("an empty key must be silently ignored: offset=%d", d.Offset)
	}
}

func TestShiftHelpers(t *testing.T) {
	if got := shiftTime(1000*time.Millisecond, 250*time.Millisecond); got != 1250*time.Millisecond {
		t.Errorf("shiftTime = %v, want 1250ms", got)
	}
	if got := shiftTime(100*time.Millisecond, -250*time.Millisecond); got != 0 {
		t.Errorf("shiftTime clamped = %v, want 0", got)
	}
	if got := shiftTime(5*time.Second, 0); got != 5*time.Second {
		t.Errorf("shiftTime with zero delta = %v, want 5s", got)
	}

	words := []WordFragment{{Time: 1000 * time.Millisecond, Text: "a"}, {Time: 100 * time.Millisecond, Text: "b"}}
	got := shiftWords(words, -250*time.Millisecond)
	if got[0].Time != 750*time.Millisecond || got[0].Text != "a" {
		t.Errorf("shiftWords[0] = %+v", got[0])
	}
	if got[1].Time != 0 || got[1].Text != "b" {
		t.Errorf("shiftWords[1] = %+v", got[1])
	}
	if words[0].Time != 1000*time.Millisecond {
		t.Error("shiftWords must not mutate its input")
	}

	if got := shiftWords(nil, 250*time.Millisecond); got != nil {
		t.Errorf("shiftWords(nil, delta) = %v, want nil", got)
	}
	if got := shiftWords(words, 0); got[0].Time != words[0].Time || got[1].Time != words[1].Time {
		t.Errorf("shiftWords with zero delta = %+v, want input unchanged", got)
	}
}

// A word whose End is unknown (0) must keep the zero sentinel through a shift:
// shifting it would silently turn "unknown" into a real, bogus end time.
func TestShiftWords_ShiftsEndButKeepsTheUnknownSentinel(t *testing.T) {
	words := []WordFragment{
		{Time: 1 * time.Second, End: 2 * time.Second, Text: "known"},
		{Time: 3 * time.Second, Text: "unknown"},
	}
	got := shiftWords(words, 500*time.Millisecond)

	if got[0].End != 2500*time.Millisecond {
		t.Errorf("got[0].End = %v, want 2.5s", got[0].End)
	}
	if got[1].End != 0 {
		t.Errorf("got[1].End = %v, want 0 (unknown must survive the shift)", got[1].End)
	}
}

// TestScanWordTimed pins the shared word-timed scanner QRC, YRC and LYS will
// run: QRC and LYS write each word before its (start,duration) tuple, YRC after
// its (start,duration,flag) tuple.
func TestScanWordTimed(t *testing.T) {
	qrc := wordTimedRe{re: qrcWordRe, text: 1, start: 2, duration: 3}
	yrc := wordTimedRe{re: yrcWordRe, text: 4, start: 1, duration: 2}

	cases := []struct {
		name  string
		body  string
		re    wordTimedRe
		text  string
		end   time.Duration
		words []string
		// starts, when set, pins the Time of each fragment in order.
		starts []time.Duration
		// ends, when set, pins the End of each fragment in order. A zero entry
		// means the fragment carries no end of its own.
		ends []time.Duration
		// start + wantStart pin the scan's Start; the flag is needed because 0 is
		// itself a legitimate Start.
		start     time.Duration
		wantStart bool
		// lineStart is the line's own start time; zero unless a case needs it.
		lineStart time.Duration
	}{
		{
			name:  "qrc text before each timestamp",
			body:  "Hello(1000,500) (1500,500)world",
			re:    qrc,
			text:  "Hello world",
			end:   2000 * time.Millisecond,
			words: []string{"Hello", " ", "world"},
		},
		{
			name:  "qrc trailing untimed text is kept",
			body:  "Hello(1000,500) (1500,500)world trailing",
			re:    qrc,
			text:  "Hello world trailing",
			end:   2000 * time.Millisecond,
			words: []string{"Hello", " ", "world trailing"},
		},
		{
			name:   "yrc leading untimed text is kept",
			body:   "lead (1000,500,0)Hello",
			re:     yrc,
			text:   "lead Hello",
			end:    1500 * time.Millisecond,
			words:  []string{"lead ", "Hello"},
			starts: []time.Duration{500 * time.Millisecond, 1000 * time.Millisecond},
			// The untimed head fragment has no end of its own: its end is the next
			// fragment's Time, which WordEnd resolves at read time. The word that
			// carried a duration keeps the end that duration implies.
			ends: []time.Duration{0, 1500 * time.Millisecond},
			// The body opens with untimed text, so the head fragment is timed at
			// the line start and Start is that fragment's Time.
			start:     500 * time.Millisecond,
			wantStart: true,
			lineStart: 500 * time.Millisecond,
		},
		{
			name:  "yrc text after each timestamp",
			body:  "(1000,500,0)Hello(1500,500,0) world trailing",
			re:    yrc,
			text:  "Hello world trailing",
			end:   2000 * time.Millisecond,
			words: []string{"Hello", " world trailing"},
		},
		{
			name:  "no timestamp at all",
			body:  "  plain text  ",
			re:    qrc,
			text:  "plain text",
			end:   0,
			words: []string{"plain text"},
			// A body with no timestamp keeps its whole text as a single fragment
			// timed at the line start (the no-match branch never sets End).
			starts:    []time.Duration{500 * time.Millisecond},
			start:     500 * time.Millisecond,
			wantStart: true,
			lineStart: 500 * time.Millisecond,
		},
		{
			name:   "degenerate 0,0 tuple carries the boundary",
			body:   "Hello(1000,500) (0,0)world",
			re:     qrc,
			text:   "Hello world",
			end:    1500 * time.Millisecond,
			words:  []string{"Hello", " ", "world"},
			starts: []time.Duration{1000 * time.Millisecond, 1500 * time.Millisecond, 1500 * time.Millisecond},
		},
		{
			// The platform's own shape: the space between two words is written as a
			// degenerate tuple next to the word tuple, and carries no time of its
			// own. Taking its 0 literally would put a fragment before the line's own
			// first word and break both the monotonic word timeline and the panel's
			// karaoke split.
			name:   "qrc space filler tuple carries the boundary",
			body:   "I(1000,200) (0,0)could(1200,300) (0,0)not(1500,300)",
			re:     qrc,
			text:   "I could not",
			end:    1800 * time.Millisecond,
			words:  []string{"I", " ", "could", " ", "not"},
			starts: []time.Duration{1000 * time.Millisecond, 1200 * time.Millisecond, 1200 * time.Millisecond, 1500 * time.Millisecond, 1500 * time.Millisecond},
			// I and could and not carry a duration; the two (0,0) space fillers do
			// not, so their End stays unknown and WordEnd falls back to the next
			// fragment's Time.
			ends: []time.Duration{1200 * time.Millisecond, 0, 1500 * time.Millisecond, 0, 1800 * time.Millisecond},
		},
		{
			// Same shape with YRC's polarity: the tuple precedes its text.
			name:   "yrc space filler tuple carries the boundary",
			body:   "(1000,200,0)I(0,0,0) (1200,300,0)could(0,0,0) (1500,300,0)not",
			re:     yrc,
			text:   "I could not",
			end:    1800 * time.Millisecond,
			words:  []string{"I", " ", "could", " ", "not"},
			starts: []time.Duration{1000 * time.Millisecond, 1200 * time.Millisecond, 1200 * time.Millisecond, 1500 * time.Millisecond, 1500 * time.Millisecond},
			// Same as the QRC case: only the tuples that carry a duration give
			// their fragment an end.
			ends: []time.Duration{1200 * time.Millisecond, 0, 1500 * time.Millisecond, 0, 1800 * time.Millisecond},
		},
		{
			name:   "only degenerate tuples leave no end",
			body:   "Hello(0,0)",
			re:     qrc,
			text:   "Hello",
			end:    0,
			words:  []string{"Hello"},
			starts: []time.Duration{500 * time.Millisecond},
			// A nonzero lineStart keeps this case from passing by accident: the
			// degenerate tuple carries no duration, so the line must stay
			// unbounded instead of inheriting lineStart as its End. The fragment
			// itself does inherit lineStart, because the tuple gave it no time.
			start:     500 * time.Millisecond,
			wantStart: true,
			lineStart: 500 * time.Millisecond,
		},
		{
			name:  "blank body scans to the zero value",
			body:  "   ",
			re:    qrc,
			text:  "",
			end:   0,
			words: nil,
		},
		{
			name:  "a timestamp carrying no text adds no fragment",
			body:  "(1000,500,0)",
			re:    yrc,
			text:  "",
			end:   0,
			words: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scanWordTimed(tc.body, tc.re, tc.lineStart)
			if got.Text != tc.text {
				t.Errorf("Text = %q, want %q", got.Text, tc.text)
			}
			if got.End != tc.end {
				t.Errorf("End = %v, want %v", got.End, tc.end)
			}
			// A nil expectation means the body held no fragment at all, so the
			// scan has to stay the zero value rather than an empty Words slice.
			if tc.words == nil && got.Words != nil {
				t.Errorf("Words = %v, want nil", got.Words)
			}
			var texts []string
			for _, w := range got.Words {
				texts = append(texts, w.Text)
			}
			if len(texts) != len(tc.words) {
				t.Fatalf("Words = %v, want %v", texts, tc.words)
			}
			for i := range tc.words {
				if texts[i] != tc.words[i] {
					t.Errorf("Words[%d] = %q, want %q", i, texts[i], tc.words[i])
				}
			}
			for i, want := range tc.starts {
				if got.Words[i].Time != want {
					t.Errorf("Words[%d].Time = %v, want %v", i, got.Words[i].Time, want)
				}
			}
			for i, want := range tc.ends {
				if got.Words[i].End != want {
					t.Errorf("Words[%d].End = %v, want %v", i, got.Words[i].End, want)
				}
			}
			if tc.wantStart && got.Start != tc.start {
				t.Errorf("Start = %v, want %v", got.Start, tc.start)
			}
			// Start is documented as Words[0].Time; LYS takes its line time
			// from it, so the two must not drift apart.
			if len(got.Words) > 0 && got.Start != got.Words[0].Time {
				t.Errorf("Start = %v, want Words[0].Time = %v", got.Start, got.Words[0].Time)
			}
			// The local form of C6: Words must tile Text.
			var sb strings.Builder
			for _, w := range got.Words {
				sb.WriteString(w.Text)
			}
			if sb.String() != got.Text {
				t.Errorf("Words %q do not tile Text %q", sb.String(), got.Text)
			}
		})
	}
}
