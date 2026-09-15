package lyrics

import (
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
