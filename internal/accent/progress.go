package accent

import (
	"math"

	"github.com/lucasb-eyer/go-colorful"
)

// ProgressColors makes a restrained same-hue ramp from darker to lighter.
// Both endpoints lose chroma together until they fit sRGB, so Hex conversion
// cannot wrap an out-of-range channel into an unrelated colour.
func ProgressColors(main colorful.Color) (colorful.Color, colorful.Color) {
	h, c, l := main.Hcl()
	if math.IsNaN(h) {
		h = 285
	}
	c = math.Min(c*.75, .22)
	lo := math.Max(.45, math.Min(.65, l-.08))
	hi := math.Min(.82, lo+.16)
	for range 160 {
		a, b := colorful.Hcl(h, c, lo), colorful.Hcl(h, c, hi)
		if a.IsValid() && b.IsValid() {
			return a, b
		}
		c *= .95
	}
	return colorful.Hcl(h, 0, lo).Clamped(), colorful.Hcl(h, 0, hi).Clamped()
}
