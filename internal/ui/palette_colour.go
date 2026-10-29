package ui

import (
	"image/color"
	"math"

	"github.com/lucasb-eyer/go-colorful"
)

// accentTone shares the background hue/chroma policy across the frame and
// interaction surfaces. Callers choose Lab lightness (scaled by 1/100) and
// optionally reduce chroma; out-of-gamut colours lose chroma until they fit sRGB.
func accentTone(main color.RGBA, light, chromaScale float64) color.RGBA {
	seed, _ := colorful.MakeColor(tintedBackground(main, backgroundTint))
	reference, _ := colorful.MakeColor(backgroundReference)
	_, a, b := seed.Lab()
	_, ra, rb := reference.Lab()
	chroma := math.Hypot(a, b)
	scale := chromaScale
	if capChroma := math.Hypot(ra, rb); chroma > capChroma {
		scale *= capChroma / chroma
	}
	a, b = a*scale, b*scale
	for range 120 {
		c := colorful.Lab(light, a, b)
		if c.IsValid() {
			return paletteRGB(c)
		}
		a, b = a*.95, b*.95
	}
	return paletteRGB(colorful.Lab(light, 0, 0).Clamped())
}

func paletteRGB(c colorful.Color) color.RGBA {
	r, g, b := c.RGB255()
	return color.RGBA{R: r, G: g, B: b, A: 255}
}
