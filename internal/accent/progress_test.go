package accent

import (
	"math"
	"testing"

	"github.com/lucasb-eyer/go-colorful"
)

func TestProgressPalette_StaysInGamutAndSameHue(t *testing.T) {
	for h := 0.0; h < 360; h += 15 {
		for _, l := range []float64{.15, .35, .55, .75, .9} {
			seed := colorful.Hcl(h, .35, l).Clamped()
			a := derive(seed)
			for _, c := range []colorful.Color{a.ProgressA, a.ProgressB} {
				if !c.IsValid() {
					t.Fatalf("seed h%g l%g: out of sRGB %+v", h, l, c)
				}
			}
			ha, _, la := a.ProgressA.Hcl()
			hb, _, lb := a.ProgressB.Hcl()
			if la >= lb || lb-la > .17 {
				t.Fatalf("brightness is not a restrained rising ramp: %g -> %g", la, lb)
			}
			delta := math.Abs(ha - hb)
			delta = math.Min(delta, 360-delta)
			if delta > 1 {
				t.Fatalf("gradient changes hue by %g degrees", delta)
			}
		}
	}
}
