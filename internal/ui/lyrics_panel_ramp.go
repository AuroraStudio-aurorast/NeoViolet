package ui

import (
	"time"

	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

// lyricGrey is the colour unplayed lyric text uses: ANSI 245, the same value
// panelContextStyle carries. The gradient blend needs RGB, so the value is
// pinned as numbers rather than resolved through the terminal's colour profile.
// The scaling matches colorful.Hex exactly, i.e. byte * (1/255).
var lyricGrey = colorful.Color{
	R: float64(0x8a) * (1.0 / 255.0),
	G: float64(0x8a) * (1.0 / 255.0),
	B: float64(0x8a) * (1.0 / 255.0),
}

// lyricFallbackMain is ANSI 141, the emphasis colour accentOrDefault falls back
// to when the track has no cover.
var lyricFallbackMain = colorful.Color{
	R: float64(0xaf) * (1.0 / 255.0),
	G: float64(0x87) * (1.0 / 255.0),
	B: float64(0xff) * (1.0 / 255.0),
}

// lyricMain returns the emphasis colour as RGB. It mirrors accentOrDefault: the
// extracted accent when there is one, the ANSI 141 fallback otherwise. The
// fallback is resolved by hand because the blend needs RGB while
// accentOrDefault hands back the ANSI index as a string.
func lyricMain(a *accent.Accent) colorful.Color {
	if a == nil {
		return lyricFallbackMain
	}
	c, err := colorful.Hex(a.HexMain())
	if err != nil {
		return lyricFallbackMain
	}
	return c
}

// lyricRune is one display rune of a line with the cell it starts at and how
// many cells it covers.
type lyricRune struct {
	text  string
	x     int
	width int
}

// lyricRunes splits text into display runes, tracking the cell offset each one
// starts at. Offsets use lipgloss.Width, the same measure wrapSpans uses, so the
// two always agree about where a rune sits.
func lyricRunes(text string) []lyricRune {
	runes := make([]lyricRune, 0, len(text))
	x := 0
	for _, r := range text {
		s := string(r)
		w := lipgloss.Width(s)
		if w == 0 {
			// Zero-width runes (combining marks, joiners) occupy no cell: they ride
			// along with the rune before them instead of becoming their own span.
			// They must never become a rune of their own: a zero width would divide
			// the intensity by zero.
			if n := len(runes); n > 0 {
				runes[n-1].text += s
			}
			continue
		}
		runes = append(runes, lyricRune{text: s, x: x, width: w})
		x += w
	}
	return runes
}

// lyricSweep returns the emphasis front's position in display cells: how far the
// shading has reached into the line at elapsed. It is 0 before the first word and
// stops at a word's end, so a silence between words does not drag it forward.
func lyricSweep(line lyrics.LyricLine, elapsed time.Duration) float64 {
	current := -1
	for i, w := range line.Words {
		if w.Time <= elapsed {
			current = i
			continue
		}
		break
	}
	if current < 0 {
		return 0
	}

	start, width := 0, 0
	for i, w := range line.Words {
		span := lipgloss.Width(w.Text)
		if i < current {
			start += span
			continue
		}
		width = span
		break
	}

	dur := line.WordEnd(current) - line.Words[current].Time
	if dur <= 0 {
		// Unknown span: treat the word as finished rather than swept, so an
		// unbounded line keeps the whole-line highlight it renders today. This
		// guard must stay ahead of the division: a (0,0) filler fragment makes
		// WordEnd equal its own Time, and dividing by that span yields a NaN
		// front, which turns every colour on the line undefined.
		return float64(start + width)
	}
	return float64(start) + clampUnit(float64(elapsed-line.Words[current].Time)/float64(dur))*float64(width)
}

// lyricIntensity returns how far the front has crossed one rune, in [0,1]. The
// divisor is the rune's own width, which is what makes a rune reach full
// emphasis exactly when the front leaves it -- at any syllable speed.
func lyricIntensity(front float64, r lyricRune) float64 {
	return clampUnit((front - float64(r.x)) / float64(r.width))
}

// lyricShade blends the unplayed grey into the emphasis colour at t. The blend
// runs in CIELAB, the same space lipgloss.Blend1D drives the progress bar with.
func lyricShade(main colorful.Color, t float64) colorful.Color {
	switch {
	case t <= 0:
		return lyricGrey
	case t >= 1:
		return main
	default:
		return lyricGrey.BlendLab(main, t).Clamped()
	}
}

func clampUnit(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}

// lyricRamp styles a word-timed current line.
type lyricRamp struct {
	main    colorful.Color
	current lipgloss.Style
	context lipgloss.Style
}

// newLyricRamp collects the styling the ramp needs from the model.
func newLyricRamp(m *Model) lyricRamp {
	return lyricRamp{
		main:    lyricMain(m.Accent),
		current: panelCurrentStyle(m),
		context: panelContextStyle,
	}
}

// styleFor maps an emphasis level onto a style. The two ends reuse the finished
// styles verbatim, so a fully sung line and an unplayed one render exactly as
// they do today; only the runes mid-crossing get a shaded style.
func (r lyricRamp) styleFor(t float64) lipgloss.Style {
	switch {
	case t <= 0:
		return r.context
	case t >= 1:
		return r.current
	default:
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(lyricShade(r.main, t).Hex())).
			Bold(true)
	}
}

// spans shades a line: runes the front has passed keep the current style, runes
// it has not reached keep the context style, and the runes it is crossing shade
// from grey to the emphasis colour. Bold tracks the emphasis exactly -- a rune
// is bold whenever it has any emphasis at all.
//
// Adjacent runes that shade identically collapse into one span. Only the word
// being sung shades per rune, so a line costs about one span per rune of that
// word rather than one per cell, and the sung prefix and unplayed suffix each
// collapse to a single span.
func (r lyricRamp) spans(line lyrics.LyricLine, elapsed time.Duration) []styledSpan {
	front := lyricSweep(line, elapsed)
	out := make([]styledSpan, 0, len(line.Words)+2)

	prev, havePrev := 0.0, false
	for _, rn := range lyricRunes(line.Text) {
		t := lyricIntensity(front, rn)
		if havePrev && t == prev {
			out[len(out)-1].Text += rn.text
			continue
		}
		out = append(out, styledSpan{Text: rn.text, Style: r.styleFor(t)})
		prev, havePrev = t, true
	}
	return out
}
