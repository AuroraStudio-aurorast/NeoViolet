package ui

import (
	"fmt"
	"image/color"
	"math"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
)

func TestBackgroundGradient_DefaultPalette(t *testing.T) {
	rows := backgroundGradient(nil, 30)
	if len(rows) != 30 {
		t.Fatalf("rows = %d, want 30", len(rows))
	}
	black := color.RGBA{A: 255}
	for y := 0; y <= 11; y++ {
		if rows[y] != black {
			t.Errorf("row %d = %v, want black", y, rows[y])
		}
	}
	if got, want := rows[29], (color.RGBA{R: 28, G: 22, B: 41, A: 255}); got != want {
		t.Errorf("bottom = %v, want %v", got, want)
	}
	if got := rows[20]; got.R == 0 || got.R >= rows[29].R {
		t.Errorf("middle = %v, want a faint intermediate tint", got)
	}
}

func TestBackgroundGradient_DegenerateHeights(t *testing.T) {
	for _, height := range []int{-1, 0, 1} {
		rows := backgroundGradient(nil, height)
		if len(rows) != max(height, 0) {
			t.Errorf("height %d gives %d rows", height, len(rows))
		}
		for _, c := range rows {
			if c != (color.RGBA{A: 255}) {
				t.Errorf("height %d = %v, want black", height, c)
			}
		}
	}
}

// Fixed accents from real covers exercise the old contrast cliff without the
// cover extractor's random seeds changing the test input.
func TestBackgroundGradient_RealCoverAccentsKeepTheirWash(t *testing.T) {
	for _, hex := range []string{"#558036", "#a8634e", "#886f6f", "#29818e"} {
		t.Run(hex, func(t *testing.T) {
			rows := backgroundGradient(&accent.Accent{Main: mustColorful(t, hex)}, 30)
			bottom := rows[len(rows)-1]
			if max(bottom.R, bottom.G, bottom.B) < 14 {
				t.Errorf("bottom %v lost the cover wash", bottom)
			}
			unique := map[color.RGBA]bool{}
			for _, c := range rows {
				unique[c] = true
			}
			if len(unique) < 10 {
				t.Errorf("only %d row colours, want a complete ramp", len(unique))
			}
		})
	}
}

func TestBackgroundGradient_ConstrainedRampHasNoClippedTail(t *testing.T) {
	rows := backgroundGradient(&accent.Accent{Main: mustColorful(t, "#485b80")}, 30)
	for y := 25; y < len(rows); y++ {
		if rows[y] == rows[y-1] {
			t.Errorf("rows %d/%d repeat %v: the bottom of the ramp was clipped", y-1, y, rows[y])
		}
	}
}

func TestBackgroundGradient_NearbyAccentsHaveNearbyBackgrounds(t *testing.T) {
	for _, hex := range []string{"#558036", "#a8634e", "#886f6f"} {
		main := mustColorful(t, hex)
		r, g, b := main.RGB255()
		previous := color.RGBA{}
		for delta := -3; delta <= 3; delta++ {
			// These fixed mid-range fixtures stay inside byte bounds.
			neighbour := fmt.Sprintf("#%02x%02x%02x", int(r)+delta, int(g)+delta, int(b)+delta)
			parsed := mustColorful(t, neighbour)
			rows := backgroundGradient(&accent.Accent{Main: parsed}, 30)
			bottom := rows[len(rows)-1]
			if delta > -3 {
				for i, v := range []uint8{bottom.R, bottom.G, bottom.B} {
					before := []uint8{previous.R, previous.G, previous.B}[i]
					if math.Abs(float64(v)-float64(before)) > 1 {
						t.Errorf("%s delta %d: background jumps %v -> %v", hex, delta, previous, bottom)
					}
				}
			}
			previous = bottom
		}
	}
}

func TestBackgroundGradient_NormalizedLightnessAndMonotonicity(t *testing.T) {
	for _, hex := range []string{"#af87ff", "#46536b", "#3b5b89", "#265c5a", "#564c94", "#b1623d", "#8baeca", "#098fc8", "#d19756", "#62cc9b", "#000000", "#ffffff", "#ffff00", "#ff0000", "#0000ff"} {
		t.Run(hex, func(t *testing.T) {
			for _, height := range []int{17, 24, 30, 60, 300} {
				rows := backgroundGradient(&accent.Accent{Main: mustColorful(t, hex)}, height)
				bottom := rows[len(rows)-1]
				// Accepted HTML target is 0.0098079; allow one-byte RGB rounding.
				if delta := math.Abs(testLuminance(bottom) - 0.00980790543); delta > 0.0006 {
					t.Errorf("%s height %d luminance %.6f, want near 0.009808", hex, height, testLuminance(bottom))
				}
				previous := color.RGBA{A: 255}
				for y, c := range rows {
					if c.R < previous.R || c.G < previous.G || c.B < previous.B {
						t.Fatalf("row %d retreats %v -> %v", y, previous, c)
					}
					previous = c
				}
			}
		})
	}
}

// Independent sRGB calculation for assertions over the bytes sent to terminals.
func testLuminance(c color.RGBA) float64 {
	result := 0.0
	for i, v := range []uint8{c.R, c.G, c.B} {
		x := float64(v) / 255
		linear := x / 12.92
		if x > 0.04045 {
			linear = math.Pow((x+0.055)/1.055, 2.4)
		}
		result += linear * []float64{0.2126, 0.7152, 0.0722}[i]
	}
	return result
}

func TestBackgroundGradient_MatchesAcceptedHTMLColours(t *testing.T) {
	cases := map[string]color.RGBA{
		"#af87ff": {R: 28, G: 22, B: 41, A: 255},
		"#46536b": {R: 24, G: 25, B: 28, A: 255},
		"#3b5b89": {R: 21, G: 26, B: 31, A: 255},
		"#265c5a": {R: 21, G: 26, B: 26, A: 255},
		"#564c94": {R: 25, G: 24, B: 34, A: 255},
		"#558036": {R: 22, G: 27, B: 19, A: 255},
		"#b1623d": {R: 33, G: 23, B: 19, A: 255},
		"#098fc8": {R: 7, G: 27, B: 37, A: 255},
	}
	for hex, want := range cases {
		rows := backgroundGradient(&accent.Accent{Main: mustColorful(t, hex)}, 30)
		if got := rows[len(rows)-1]; got != want {
			t.Errorf("%s bottom=%v, want HTML %v", hex, got, want)
		}
	}
}
