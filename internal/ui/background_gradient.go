package ui

import (
	"image/color"
	"math"

	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
)

const (
	backgroundHeight = 0.62
	backgroundTint   = 0.16
	backgroundCurve  = 2.4
)

var backgroundReference = color.RGBA{R: 28, G: 22, B: 41, A: 255}

func backgroundAccent(a *accent.Accent) color.RGBA {
	r, g, b, _ := lipgloss.Color(accentOrDefault(a, "#af87ff")).RGBA()
	return color.RGBA{R: uint8((r >> 8) & 0xff), G: uint8((g >> 8) & 0xff), B: uint8((b >> 8) & 0xff), A: 255}
}

// normalizedBackground keeps the cover hue and limits its chroma while matching
// the accepted purple background's lightness. Out-of-gamut colours lose chroma,
// not hue, until they fit sRGB. go-colorful uses Lab values scaled by 1/100.
func normalizedBackground(main color.RGBA) color.RGBA {
	reference, _ := colorful.MakeColor(backgroundReference)
	light, _, _ := reference.Lab()
	return accentTone(main, light, 1)
}

// backgroundGradient uses screen rows, so adjacent panels share one continuous
// wash. Panel keeps the complete normalized ramp; OSC 11 remains black.
func backgroundGradient(a *accent.Accent, height int) []color.RGBA {
	rows := make([]color.RGBA, max(height, 0))
	main := backgroundAccent(a)
	bottom := normalizedBackground(main)
	for y := range rows {
		amount := 0.0
		if height > 1 {
			p := clampUnit((float64(y)/float64(height-1) - (1 - backgroundHeight)) / backgroundHeight)
			amount = math.Pow(p, backgroundCurve)
		}
		// Preserve the accepted purple ramp exactly, including its byte rounding.
		if main == (color.RGBA{R: 175, G: 135, B: 255, A: 255}) {
			rows[y] = tintedBackground(main, backgroundTint*amount)
		} else {
			rows[y] = tintedBackground(bottom, amount)
		}
	}
	return rows
}

// frameBackground darkens only the visible One-line lyric and its adjacent rows.
// The position comes from the same layout as the footer, including resize and
// automatic Panel fallback. Hidden lyrics leave the complete wash in place.
func frameBackground(m *Model) []color.RGBA {
	rows := backgroundGradient(m.Accent, m.UI.Height)
	plan := m.layoutPlan()
	if plan.LyricMode != lyricModeOneLine || !m.oneLineVisible() {
		return rows
	}
	main := backgroundAccent(m.Accent)
	mainLum := backgroundLuminance(main)
	base := (mainLum + .05) / .05
	transition := clampUnit((base - 3) / 4)
	transition = transition * transition * (3 - 2*transition)
	target := base * (.95 - .15*transition)
	lyricY := plan.Height - helpHeight - 2
	for y := max(0, lyricY-1); y <= min(len(rows)-1, lyricY+1); y++ {
		original := rows[y]
		scale := 1.0
		for scale > 0 && (mainLum+.05)/(backgroundLuminance(tintedBackground(original, scale))+.05) < target {
			scale = max(0, scale-.001)
		}
		weight := 1 - math.Abs(float64(y-lyricY))/2
		rows[y] = tintedBackground(original, 1-weight*(1-scale))
	}
	return rows
}

func tintedBackground(main color.RGBA, tint float64) color.RGBA {
	return color.RGBA{
		R: uint8(math.Round(float64(main.R) * tint)),
		G: uint8(math.Round(float64(main.G) * tint)),
		B: uint8(math.Round(float64(main.B) * tint)),
		A: 255,
	}
}

func backgroundLuminance(c color.RGBA) float64 {
	linear := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= .04045 {
			return s / 12.92
		}
		return math.Pow((s+.055)/1.055, 2.4)
	}
	return .2126*linear(c.R) + .7152*linear(c.G) + .0722*linear(c.B)
}
