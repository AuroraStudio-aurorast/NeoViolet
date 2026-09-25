package ui

import (
	"math"
	"testing"
	"time"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
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

// lyricShade is the one curve both features read: the sweep at a rune's own
// progress, and a translated row at its midpoint. Only the midpoint is reachable
// through them, so the ends and the direction are pinned here -- a blend with its
// endpoints swapped, or one that ran backwards, would otherwise leave the whole
// suite green. The midpoint's value and hue are pinned through the row that uses
// it.
func TestLyricShade_RunsFromTheGreyUpToTheAccent(t *testing.T) {
	main := mustColorful(t, "#af87ff")

	// The ends are the finished colours themselves, not lookalikes.
	if got := lyricShade(main, 0); got != lyricGrey {
		t.Errorf("lyricShade at 0 = %s, want the unplayed grey %s", got.Hex(), lyricGrey.Hex())
	}
	if got := lyricShade(main, 1); got != main {
		t.Errorf("lyricShade at 1 = %s, want the emphasis colour %s", got.Hex(), main.Hex())
	}
	// styleFor clamps the same way, so an t outside [0,1] must not escape the ends.
	if got := lyricShade(main, -0.5); got != lyricGrey {
		t.Errorf("lyricShade at -0.5 = %s, want the unplayed grey", got.Hex())
	}
	if got := lyricShade(main, 1.5); got != main {
		t.Errorf("lyricShade at 1.5 = %s, want the emphasis colour", got.Hex())
	}

	// Direction, which the midpoint cannot show: three quarters of the way must be
	// further from the grey than a quarter. Swapping the two colours passed to the
	// blend would flip this while leaving the midpoint identical.
	_, greyC, _ := lyricGrey.Hcl()
	_, nearC, _ := lyricShade(main, 0.25).Hcl()
	_, farC, _ := lyricShade(main, 0.75).Hcl()
	_, mainC, _ := main.Hcl()
	if !(greyC <= nearC && nearC < farC && farC <= mainC) {
		t.Errorf("chroma is not monotonic from grey up to the accent: 0.25=%.2f 0.75=%.2f (grey=%.2f accent=%.2f)",
			nearC, farC, greyC, mainC)
	}
}

// The two ends of the ramp hand back the panel's finished styles rather than
// colours derived to look like them. That is what keeps a fully sung line and an
// unplayed one byte-identical to how they rendered before the sweep existed, and it
// stops the gradient's truecolor grey from reaching whole lines that the terminal
// would otherwise render through its own 245 profile.
func TestLyricRamp_EndsReuseThePanelStylesVerbatim(t *testing.T) {
	m := &Model{}
	r := newLyricRamp(m)

	if got, want := r.styleFor(0).Render("x"), panelContextStyle.Render("x"); got != want {
		t.Errorf("styleFor(0) renders %q, want panelContextStyle's %q", got, want)
	}
	if got, want := r.styleFor(1).Render("x"), panelCurrentStyle(m).Render("x"); got != want {
		t.Errorf("styleFor(1) renders %q, want panelCurrentStyle's %q", got, want)
	}
	// The clamped ends land on the same two styles.
	if got := r.styleFor(-1).Render("x"); got != panelContextStyle.Render("x") {
		t.Errorf("styleFor(-1) renders %q, want panelContextStyle", got)
	}
	if got := r.styleFor(2).Render("x"); got != panelCurrentStyle(m).Render("x") {
		t.Errorf("styleFor(2) renders %q, want panelCurrentStyle", got)
	}
}

// A translated row takes the emphasis colour halfway toward the grey the unplayed
// text uses, which is lyricShade read at translationShade. The hex values are
// pinned as literals rather than recomputed from the same expression, so changing
// the blend space, moving an endpoint or retuning the constant cannot pass
// unnoticed.
func TestLyricTranslationColour_IsTheMidpointTowardGrey(t *testing.T) {
	// No cover: lyricMain falls back to ANSI 141 (#af87ff).
	if got, want := lyricTranslationColour(nil).Hex(), "#9f89c4"; got != want {
		t.Errorf("cover-less translation colour = %q, want %q", got, want)
	}
	// With a cover the step follows the extracted accent instead.
	acc := &accent.Accent{Main: mustColorful(t, "#8a2be2")}
	if got, want := lyricTranslationColour(acc).Hex(), "#9163b7"; got != want {
		t.Errorf("accented translation colour = %q, want %q", got, want)
	}

	// The point of the blend is to lose saturation, not to fade out or to drift to
	// another colour: the hue stays and the chroma drops below the accent's. Blending
	// toward a neutral grey in CIELAB halves (a,b), so the hue is preserved exactly.
	main := mustColorful(t, "#af87ff")
	mid := lyricTranslationColour(nil)
	mainH, mainC, mainL := main.Hcl()
	midH, midC, midL := mid.Hcl()

	if diff := math.Abs(mainH - midH); diff > 1 {
		t.Errorf("hue moved %.1f degrees (%.1f -> %.1f), want the accent's hue kept", diff, mainH, midH)
	}
	if midC >= mainC {
		t.Errorf("chroma %.3f is not below the accent's %.3f: the row did not step back", midC, mainC)
	}
	if midL > mainL {
		t.Errorf("lightness rose %.3f -> %.3f, want the step to come from chroma, not a lift", mainL, midL)
	}
	if mid == lyricGrey {
		t.Error("translation colour equals the unplayed grey: the row would read as unsung")
	}
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
	for _, line := range []lyrics.LyricLine{
		{
			Text: "aa bb",
			Words: []lyrics.WordFragment{
				{Time: 0, End: 500 * time.Millisecond, Text: "aa "},
				{Time: 500 * time.Millisecond, End: 1 * time.Second, Text: "bb"},
			},
		},
		{
			// A zero-width rune opening the line has no base rune before it. It must
			// still reach the panel, so this pins the invariant at the span layer
			// rather than only inside lyricRunes.
			Text:  "\u200bhi",
			Words: []lyrics.WordFragment{{Time: 0, End: time.Second, Text: "\u200bhi"}},
		},
	} {
		ramp := rampForTest(t)
		for _, at := range []time.Duration{0, 250 * time.Millisecond, 750 * time.Millisecond, time.Second} {
			if got := spansText(ramp.spans(line, at)); got != line.Text {
				t.Errorf("%q at %v spans cover %q, want %q", line.Text, at, got, line.Text)
			}
		}
	}
}

// A zero-width rune must never be dropped: the panel would then render text the
// lyric data does not contain. It rides the rune before it, or the rune after it
// when it opens the line.
func TestLyricRamp_ZeroWidthRuneIsNotDropped(t *testing.T) {
	for _, text := range []string{"\u200bhi", "hi\u200b", "h\u200bi", "\u200b"} {
		var joined string
		for _, rn := range lyricRunes(text) {
			joined += rn.text
			if rn.width < 1 {
				t.Errorf("%q: rune %q has width %d, want at least 1", text, rn.text, rn.width)
			}
		}
		if joined != text {
			t.Errorf("lyricRunes(%q) covers %q, want the original text", text, joined)
		}
	}
}

// Text made only of zero-width runes leaves no base rune to ride. It must still
// produce one rune that claims a cell, so it is neither dropped nor divided by
// zero.
func TestLyricRamp_AllZeroWidthTextKeepsOneRune(t *testing.T) {
	runes := lyricRunes("\u200b")
	if len(runes) != 1 {
		t.Fatalf("got %d runes, want 1", len(runes))
	}
	if runes[0].x != 0 {
		t.Errorf("x = %d, want 0", runes[0].x)
	}
	if runes[0].width != 1 {
		t.Errorf("width = %d, want 1", runes[0].width)
	}
	if runes[0].text != "\u200b" {
		t.Errorf("text = %q, want %q", runes[0].text, "\u200b")
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
