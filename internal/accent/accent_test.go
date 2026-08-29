package accent

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/EdlinOrg/prominentcolor"
	"github.com/lucasb-eyer/go-colorful"
)

func solidImage(c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < 4*4; i++ {
		img.Set(i%4, i/4, c)
	}
	return img
}

func TestIsDark(t *testing.T) {
	// Dark, saturated (sat 0.9 > 0.02) and above the black-mask threshold
	// (channels below 80 are masked out). liftToVisible lifts its low L* up to
	// the 0.35 floor, which is still below the 0.5 dark/light midpoint.
	dark, err := FromImage(solidImage(color.RGBA{30, 10, 100, 255}))
	if err != nil {
		t.Fatalf("FromImage(dark): %v", err)
	}
	if !dark.IsDark() {
		l, _, _ := dark.Main.Lab()
		t.Errorf("dark image: IsDark() = false, want true (Main L* = %v)", l)
	}

	// Light, saturated (sat 0.40 > 0.02) and below the white-mask threshold
	// (white mask removes pixels whose channels are all >= 192). Its L* (~0.69)
	// survives liftToVisible unchanged, so it stays above the 0.5 midpoint.
	// Note: {240,200,200} was rejected by FromImage's white mask, hence this
	// adjusted light input.
	light, err := FromImage(solidImage(color.RGBA{200, 160, 120, 255}))
	if err != nil {
		t.Fatalf("FromImage(light): %v", err)
	}
	if light.IsDark() {
		l, _, _ := light.Main.Lab()
		t.Errorf("light image: IsDark() = true, want false (Main L* = %v)", l)
	}
}

func TestFromImageGrayscaleError(t *testing.T) {
	// Mid-gray survives both the white and black masks, then selectSeed returns
	// zero saturation and FromImage must reject it as an unusable accent.
	if _, err := FromImage(solidImage(color.RGBA{128, 128, 128, 255})); err == nil {
		t.Error("FromImage(grayscale) = nil error, want non-nil")
	}
}

func TestSelectSeedMaxSaturation(t *testing.T) {
	items := []prominentcolor.ColorItem{
		{Color: prominentcolor.ColorRGB{R: 128, G: 128, B: 128}, Cnt: 100}, // gray, sat 0
		{Color: prominentcolor.ColorRGB{R: 120, G: 120, B: 200}, Cnt: 1},   // muted blue, sat 0.4
		{Color: prominentcolor.ColorRGB{R: 255, G: 0, B: 0}, Cnt: 1},       // pure red, sat 1.0
	}

	seed, sat := selectSeed(items)
	if math.Abs(sat-1.0) > 1e-9 {
		t.Fatalf("selectSeed sat = %v, want 1.0", sat)
	}
	if math.Abs(seed.R-1.0) > 1e-9 || math.Abs(seed.G) > 1e-9 || math.Abs(seed.B) > 1e-9 {
		t.Errorf("selectSeed seed = %+v, want pure red (highest saturation)", seed)
	}
}

func TestDeriveRanges(t *testing.T) {
	seeds := []colorful.Color{
		{R: 30.0 / 255, G: 10.0 / 255, B: 100.0 / 255},   // dark, saturated
		{R: 200.0 / 255, G: 160.0 / 255, B: 120.0 / 255}, // light, saturated
		{R: 0.5, G: 0.5, B: 0.5},                         // gray (zero chroma path)
	}

	for i, seed := range seeds {
		a := derive(seed)

		_, _, mainL := a.Main.Hcl()
		_, _, paL := a.ProgressA.Hcl()
		_, _, pbL := a.ProgressB.Hcl()
		_, _, lyL := a.Lyric.Hcl()

		// liftToVisible clamps Main L* to [0.35, 0.75].
		if mainL < 0.35 || mainL > 0.75 {
			t.Errorf("seed %d: Main L* = %v, want [0.35, 0.75]", i, mainL)
		}
		// derive: paL = min(mainL*1.25, 0.88).
		if paL < 0.4375 || paL > 0.88 {
			t.Errorf("seed %d: ProgressA L* = %v, want [0.4375, 0.88]", i, paL)
		}
		// derive: pbL = max(mainL*0.65, 0.25).
		if pbL < 0.25 || pbL > 0.4875 {
			t.Errorf("seed %d: ProgressB L* = %v, want [0.25, 0.4875]", i, pbL)
		}
		// derive: lyL = min(mainL*1.55, 0.93).
		if lyL < 0.5425 || lyL > 0.93 {
			t.Errorf("seed %d: Lyric L* = %v, want [0.5425, 0.93]", i, lyL)
		}
	}
}

func TestHexGetters(t *testing.T) {
	a, err := FromImage(solidImage(color.RGBA{200, 60, 60, 255}))
	if err != nil {
		t.Fatalf("FromImage: %v", err)
	}
	if len(a.HexMain()) != 7 { // "#RRGGBB"
		t.Errorf("HexMain = %q, want 7 chars", a.HexMain())
	}
	for _, f := range []func() string{a.HexProgressA, a.HexProgressB, a.HexLyric} {
		if len(f()) != 7 {
			t.Errorf("hex getter returned %q, want 7 chars", f())
		}
	}
}
