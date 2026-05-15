package accent

import (
	"fmt"
	"image"
	"math"

	"github.com/EdlinOrg/prominentcolor"
	"github.com/lucasb-eyer/go-colorful"
)

type Accent struct {
	Main      colorful.Color
	Focus     colorful.Color
	ProgressA colorful.Color
	ProgressB colorful.Color
	Lyric     colorful.Color
}

func (a Accent) IsDark() bool {
	l, _, _ := a.Main.Lab()
	return l < 50
}

func (a Accent) HexMain() string      { return a.Main.Hex() }
func (a Accent) HexFocus() string     { return a.Focus.Hex() }
func (a Accent) HexProgressA() string { return a.ProgressA.Hex() }
func (a Accent) HexProgressB() string { return a.ProgressB.Hex() }
func (a Accent) HexLyric() string     { return a.Lyric.Hex() }

func FromImage(img image.Image) (Accent, error) {
	args := prominentcolor.ArgumentNoCropping |
		prominentcolor.ArgumentSeedRandom |
		prominentcolor.ArgumentAverageMean

	colors, err := prominentcolor.KmeansWithAll(
		5, img, args, 80, prominentcolor.GetDefaultMasks(),
	)
	if err != nil {
		return Accent{}, fmt.Errorf("extract colors: %w", err)
	}

	if len(colors) == 0 {
		return Accent{}, fmt.Errorf("no dominant colors found")
	}

	seed, sat := selectSeed(colors)
	if sat < 0.02 {
		return Accent{}, fmt.Errorf("image is grayscale, no usable accent")
	}

	return derive(seed), nil
}

func selectSeed(items []prominentcolor.ColorItem) (colorful.Color, float64) {
	var seed colorful.Color
	maxSat := -1.0

	for _, item := range items {
		r := float64(item.Color.R) / 255
		g := float64(item.Color.G) / 255
		b := float64(item.Color.B) / 255
		c := colorful.Color{R: r, G: g, B: b}
		_, s, _ := c.Hsv()
		if s > maxSat {
			maxSat = s
			seed = c
		}
	}

	return seed, maxSat
}

func liftToVisible(c colorful.Color) colorful.Color {
	h, chroma, l := c.Hcl()

	if math.IsNaN(h) {
		return colorful.Hcl(260, 0.15, 0.5)
	}

	if chroma < 0.10 {
		chroma = 0.10 + chroma*0.3
	}
	chroma = math.Min(chroma, 0.45)

	if l < 0.20 {
		l = 0.20 + l*0.4
	}
	if l < 0.33 {
		l = 0.33 + (l-0.20)*0.4
	}
	l = math.Max(l, 0.35)
	l = math.Min(l, 0.75)

	return colorful.Hcl(h, chroma, l)
}

func derive(seed colorful.Color) Accent {
	main := liftToVisible(seed)
	h, chroma, l := main.Hcl()

	fL := math.Min(l*1.45, 0.92)
	fC := math.Min(chroma*1.2, 0.55)
	f := colorful.Hcl(h, fC, fL)

	paL := math.Min(l*1.25, 0.88)
	paC := math.Min(chroma*1.1, 0.5)
	pa := colorful.Hcl(h, paC, paL)

	pbL := math.Max(l*0.65, 0.25)
	pbC := math.Max(chroma*0.55, 0.05)
	pb := colorful.Hcl(h, pbC, pbL)

	lyL := math.Min(l*1.55, 0.93)
	lyC := math.Max(chroma*0.15, 0.02)
	ly := colorful.Hcl(h, lyC, lyL)

	return Accent{
		Main:      main,
		Focus:     f,
		ProgressA: pa,
		ProgressB: pb,
		Lyric:     ly,
	}
}
