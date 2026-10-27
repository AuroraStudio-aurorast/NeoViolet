package ui

import (
	"image/color"
	"math"

	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
)

const (
	backgroundHeight = 0.62
	backgroundTint   = 0.16
	backgroundCurve  = 2.4
)

// backgroundGradient uses screen rows, so adjacent panels share one continuous
// wash. OSC 11 stays black; every visible cell carries its own row's colour.
func backgroundGradient(a *accent.Accent, height int) []color.RGBA {
	rows := make([]color.RGBA, max(height, 0))
	if height <= 1 {
		for i := range rows {
			rows[i] = color.RGBA{A: 255}
		}
		return rows
	}
	r, g, b, _ := lipgloss.Color(accentOrDefault(a, "#af87ff")).RGBA()
	main := color.RGBA{R: uint8((r >> 8) & 0xff), G: uint8((g >> 8) & 0xff), B: uint8((b >> 8) & 0xff), A: 255}
	mainLuminance := backgroundLuminance(main)
	base := (mainLuminance + 0.05) / 0.05
	target := base * 0.95
	if base >= 4.5 {
		target = max(4.5, base*0.8)
	}
	// Find one ceiling for the palette: capping every row at the same tint also
	// keeps quantisation from making the constrained bottom rows alternate shades.
	ceiling := backgroundTint
	for ceiling > 0 {
		bg := tintedBackground(main, ceiling)
		if (mainLuminance+0.05)/(backgroundLuminance(bg)+0.05) >= target {
			break
		}
		ceiling = max(0, ceiling-0.001)
	}
	for y := range rows {
		position := float64(y) / float64(height-1)
		p := clampUnit((position - (1 - backgroundHeight)) / backgroundHeight)
		tint := min(ceiling, backgroundTint*math.Pow(p, backgroundCurve))
		rows[y] = tintedBackground(main, tint)
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
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
}
