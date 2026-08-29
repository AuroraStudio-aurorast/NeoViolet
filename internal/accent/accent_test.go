package accent

import (
	"image"
	"image/color"
	"testing"
)

func solidImage(c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < 4*4; i++ {
		img.Set(i%4, i/4, c)
	}
	return img
}

func TestFromImageDeterministic(t *testing.T) {
	a, err := FromImage(solidImage(color.RGBA{200, 60, 60, 255}))
	if err != nil {
		t.Fatalf("FromImage: %v", err)
	}
	if a.HexMain() == "" {
		t.Error("HexMain empty")
	}
	// Determinism: same input, same output
	b, _ := FromImage(solidImage(color.RGBA{200, 60, 60, 255}))
	if a != b {
		t.Error("FromImage not deterministic for identical input")
	}
}

func TestIsDark(t *testing.T) {
	// Dark but saturated (not grayscale) and above the black-mask threshold:
	// FromImage rejects grayscale images (sat < 0.02) and prominentcolor's
	// black mask removes pixels whose channels are all below 80 (8-bit).
	a, err := FromImage(solidImage(color.RGBA{30, 10, 100, 255}))
	if err != nil {
		t.Fatalf("FromImage: %v", err)
	}
	if !a.IsDark() {
		t.Error("dark image should be IsDark")
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
