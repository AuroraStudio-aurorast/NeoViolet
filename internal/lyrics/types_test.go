package lyrics

import (
	"strings"
	"testing"
)

// A line that never went through a merge carries no Parts: Part(0) is the text
// itself, so every renderer can loop over parts without a special case.
func TestLyricLine_PartAccessors_PlainLine(t *testing.T) {
	plain := LyricLine{Text: "hello"}

	if got := plain.PartCount(); got != 1 {
		t.Errorf("PartCount() = %d, want 1", got)
	}
	if got := plain.Part(0); got != "hello" {
		t.Errorf("Part(0) = %q, want %q", got, "hello")
	}
	if plain.Parts != nil {
		t.Errorf("Parts = %v, want nil for a plain line", plain.Parts)
	}
}

// A merged line keeps Text as the single string the legacy consumers read
// (one_line footer, lyricSig, MPRIS) and exposes the source texts as Parts.
func TestLyricLine_PartAccessors_MergedLine(t *testing.T) {
	merged := LyricLine{Text: "hello | 你好", Parts: []string{"hello", "你好"}}

	if got := merged.PartCount(); got != 2 {
		t.Fatalf("PartCount() = %d, want 2", got)
	}
	for i, want := range merged.Parts {
		if got := merged.Part(i); got != want {
			t.Errorf("Part(%d) = %q, want %q", i, got, want)
		}
	}
	if got, want := strings.Join(merged.Parts, " | "), merged.Text; got != want {
		t.Errorf("Join(Parts, \" | \") = %q, want Text %q", got, want)
	}
}
