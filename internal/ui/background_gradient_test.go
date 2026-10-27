package ui

import (
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
	for _, hex := range []string{"#af87ff", "#60b5a6", "#df9b67", "#b86782", "#485b80", "#000000", "#ffffff", "#ffff00"} {
		t.Run(hex, func(t *testing.T) {
			main := mustColorful(t, hex)
			r, g, b, _ := main.RGBA()
			mainLum := luminance(color.RGBA{R: uint8((r >> 8) & 0xff), G: uint8((g >> 8) & 0xff), B: uint8((b >> 8) & 0xff), A: 255})
			original := (mainLum + 0.05) / 0.05
			limit := original * 0.95
			if original >= 4.5 {
				limit = math.Max(4.5, original*0.8)
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
