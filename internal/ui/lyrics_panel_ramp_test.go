package ui

import (
	"math"
	"testing"
	"time"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

// rampForTest builds the ramp the way a cover-less track gets it. panelCurrentStyle
// reads m.Accent, so it needs a non-nil model: a nil *Model panics instead of
// falling back to the ANSI colours.
func rampForTest(t *testing.T) lyricRamp {
	t.Helper()
	return lyricRamp{
		main:    mustColorful(t, "#af87ff"),
		current: panelCurrentStyle(&Model{}),
		context: panelContextStyle,
	}
}

func mustColorful(t *testing.T, hex string) colorful.Color {
	t.Helper()
	c, err := colorful.Hex(hex)
	if err != nil {
		t.Fatalf("colorful.Hex(%q): %v", hex, err)
	}
	return c
}

func closeEnough(got, want float64) bool {
	d := got - want
	return d < 0.001 && d > -0.001
}

func runeIntensityForTest(line lyrics.LyricLine, elapsed time.Duration, r lyricRune) float64 {
	return lyricIntensity(lyricSweep(line, elapsed), r)
}

// Bold marks exactly the runes the emphasis front has reached: the colour gets
// there and the weight follows it.
func TestLyricRamp_BoldTracksTheEmphasisRegion(t *testing.T) {
	line := lyrics.LyricLine{
		Text:  "abcd",
		Words: []lyrics.WordFragment{{Time: 0, End: 4 * time.Second, Text: "abcd"}},
	}
	ramp := rampForTest(t)
	for _, frac := range []float64{0, 0.25, 0.5, 0.75, 1} {
		elapsed := time.Duration(frac * float64(4*time.Second))
		front := lyricSweep(line, elapsed)
		for _, rn := range lyricRunes(line.Text) {
			want := lyricIntensity(front, rn) > 0
			if got := ramp.styleFor(lyricIntensity(front, rn)).GetBold(); got != want {
				t.Errorf("frac %v rune %q: bold = %v, want %v", frac, rn.text, got, want)
			}
		}
	}
}

// Whatever the front is doing, the spans must cover the line exactly once: a
// rune merged away by mistake would silently drop text from the panel.
func TestLyricRamp_SpansCoverTheLine(t *testing.T) {
	line := lyrics.LyricLine{
		Text: "aa bb",
		Words: []lyrics.WordFragment{
			{Time: 0, End: 500 * time.Millisecond, Text: "aa "},
			{Time: 500 * time.Millisecond, End: 1 * time.Second, Text: "bb"},
		},
	}
	ramp := rampForTest(t)
	for _, at := range []time.Duration{0, 250 * time.Millisecond, 750 * time.Millisecond, time.Second} {
		if got := spansText(ramp.spans(line, at)); got != line.Text {
			t.Errorf("at %v spans cover %q, want %q", at, got, line.Text)
		}
	}
}

// A silence between words freezes the front: the rendered line must not change
// while no word is being sung.
func TestLyricRamp_SilenceFreezesTheFront(t *testing.T) {
	line := lyrics.LyricLine{
		Text: "wait for",
		Words: []lyrics.WordFragment{
			{Time: 0, End: 400 * time.Millisecond, Text: "wait "},
			{Time: 1800 * time.Millisecond, End: 2000 * time.Millisecond, Text: "for"},
		},
	}
	ramp := rampForTest(t)
	first := ramp.spans(line, 400*time.Millisecond)
	for _, at := range []time.Duration{800 * time.Millisecond, 1400 * time.Millisecond, 1800 * time.Millisecond} {
		if got := ramp.spans(line, at); !sameSpans(got, first) {
			t.Errorf("line changed at %v during the silence:\n got %v\nwant %v", at, got, first)
		}
	}
}

// Without an End the word falls back to the next fragment's Time, so the front
// crawls across a silence -- the behaviour carrying End is meant to replace.
func TestLyricRamp_UnknownEndFallsBackToTheNextWord(t *testing.T) {
	line := lyrics.LyricLine{
		Text: "wait for",
		Words: []lyrics.WordFragment{
			{Time: 0, Text: "wait "},
			{Time: 1800 * time.Millisecond, End: 2000 * time.Millisecond, Text: "for"},
		},
	}
	early := lyricSweep(line, 400*time.Millisecond)
	late := lyricSweep(line, 1400*time.Millisecond)
	if !(late > early) {
		t.Fatalf("front did not advance without an End: %v then %v", early, late)
	}
}

// A word whose span collapses to zero must read as "already sung", never as a
// divisor: dividing by it yields NaN, and a NaN front makes every colour on the
// line undefined. The (0,0) filler tuples QRC/YRC write between words are the
// usual way to reach this state, so it is a normal input rather than a corner.
func TestLyricRamp_ZeroSpanWordIsAlreadySung(t *testing.T) {
	line := lyrics.LyricLine{
		Text:  "hm",
		Words: []lyrics.WordFragment{{Time: 0, Text: "hm"}},
	}
	// Pin that this fixture really does produce a zero span, so the test cannot
	// silently stop exercising the guard if WordEnd's fallbacks ever change.
	if end := line.WordEnd(0); end != line.Words[0].Time {
		t.Fatalf("WordEnd(0) = %v, want %v (a zero span)", end, line.Words[0].Time)
	}
	ramp := rampForTest(t)
	front := lyricSweep(line, 5*time.Second)
	if math.IsNaN(front) || math.IsInf(front, 0) {
		t.Fatalf("front = %v, want a finite number", front)
	}
	if got := spansText(ramp.spans(line, 5*time.Second)); got != line.Text {
		t.Errorf("spans cover %q, want %q", got, line.Text)
	}
}

// Only the word being sung shades per rune: the sung prefix and the unplayed
// suffix each collapse to one span, so a line costs about one span per rune of
// that word rather than one per cell.
func TestLyricRamp_MergesSpansOutsideTheCurrentWord(t *testing.T) {
	line := lyrics.LyricLine{
		Text: "hello world",
		Words: []lyrics.WordFragment{
			{Time: 0, End: 1 * time.Second, Text: "hello "},
			{Time: 1 * time.Second, End: 2 * time.Second, Text: "world"},
		},
	}
	// Ten runes, of which at most five can shade; a per-cell implementation would
	// produce eleven spans.
	spans := rampForTest(t).spans(line, 1500*time.Millisecond)
	if len(spans) > 6 {
		t.Errorf("got %d spans, want at most 6 (per-cell shading would give %d)",
			len(spans), len(lyricRunes(line.Text)))
	}
}

func sameSpans(a, b []styledSpan) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Text != b[i].Text || a[i].Style.Render("x") != b[i].Style.Render("x") {
			return false
		}
	}
	return true
}

// A rune's intensity is its progress through its own slice of the word, so the
// same fraction gives the same value whether the syllable is 20ms or 300ms long.
func TestLyricRamp_IntensityIsProgressThroughTheRune(t *testing.T) {
	for _, tc := range []struct {
		name    string
		perRune time.Duration
	}{
		{"fast", 20 * time.Millisecond},
		{"slow", 300 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line := lyrics.LyricLine{
				Text:  "abcd",
				Words: []lyrics.WordFragment{{Time: 0, End: 4 * tc.perRune, Text: "abcd"}},
			}
			// 2.5 runes in: the third rune is half shaded, the first two are full.
			elapsed := time.Duration(2.5 * float64(tc.perRune))
			want := []float64{1, 1, 0.5, 0}
			for i, rn := range lyricRunes(line.Text) {
				if got := runeIntensityForTest(line, elapsed, rn); !closeEnough(got, want[i]) {
					t.Errorf("rune %d intensity = %v, want %v", i, got, want[i])
				}
			}
		})
	}
}
