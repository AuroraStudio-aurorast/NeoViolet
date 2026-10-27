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

func TestBackgroundGradient_ContrastAndMonotonicity(t *testing.T) {
	// Independent sRGB calculation, including byte quantisation. Do not derive
	// expected contrast from the production luminance or tint functions.
	luminance := func(c color.RGBA) float64 {
		result := 0.0
		for i, v := range []uint8{c.R, c.G, c.B} {
			x := float64(v) / 255
			vlin := x / 12.92
			if x > 0.04045 {
				vlin = math.Pow((x+0.055)/1.055, 2.4)
			}
			result += vlin * []float64{0.2126, 0.7152, 0.0722}[i]
		}
		return result
	}
	for _, hex := range []string{"#af87ff", "#60b5a6", "#df9b67", "#b86782", "#485b80", "#000000", "#ffffff", "#ffff00", "#558036", "#a8634e", "#876d6e"} {
		t.Run(hex, func(t *testing.T) {
			main := mustColorful(t, hex)
			r, g, b, _ := main.RGBA()
			mainLum := luminance(color.RGBA{R: uint8((r >> 8) & 0xff), G: uint8((g >> 8) & 0xff), B: uint8((b >> 8) & 0xff), A: 255})
			original := (mainLum + 0.05) / 0.05
			// All accents keep at least 80% of their contrast on black;
			// dark accents keep at least 95%.
			limit := original * 0.8
			if original <= 3 {
				limit = original * 0.95
			}
			for _, height := range []int{17, 24, 30, 60, 300} {
				previous := color.RGBA{A: 255}
				rows := backgroundGradient(&accent.Accent{Main: main}, height)
				for y, c := range rows {
					ratio := (mainLum + 0.05) / (luminance(c) + 0.05)
					if ratio+1e-9 < limit {
						t.Fatalf("height %d row %d contrast %.5f below %.5f", height, y, ratio, limit)
					}
					if c.R < previous.R || c.G < previous.G || c.B < previous.B {
						t.Fatalf("height %d row %d retreats %v -> %v", height, y, previous, c)
					}
					if float64(c.R) > math.Round(float64(r>>8)*0.16) || float64(c.G) > math.Round(float64(g>>8)*0.16) || float64(c.B) > math.Round(float64(b>>8)*0.16) {
						t.Fatalf("row %d exceeds 16%% tint: %v", y, c)
					}
					previous = c
				}
			}
		})
	}
}
