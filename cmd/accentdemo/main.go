package main

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"math"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run ./cmd/accentdemo <image_path|--demo>")
		fmt.Println("  <image_path>  album cover image (jpeg/png)")
		fmt.Println("  --demo        generate synthetic gradient image for testing")
		os.Exit(1)
	}

	var img image.Image
	if os.Args[1] == "--demo" {
		fmt.Println("Using synthetic demo image (blue-purple gradient)")
		img = synthImage()
	} else {
		f, err := os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "open:", err)
			os.Exit(1)
		}
		defer f.Close()

		var err2 error
		img, _, err2 = image.Decode(f)
		if err2 != nil {
			fmt.Fprintln(os.Stderr, "decode:", err2)
			os.Exit(1)
		}
	}

	a, err := accent.FromImage(img)
	if err != nil {
		fmt.Fprintln(os.Stderr, "accent:", err)
		os.Exit(1)
	}

	fmt.Println()
	renderSwatches(a)
	fmt.Println()
	renderMockUI(a)
	fmt.Println()
}

func renderSwatches(a accent.Accent) {
		swatch := func(hex string, label string) string {
		return lipgloss.NewStyle().
			Background(lipgloss.Color(hex)).
			Width(16).Height(3).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color(contrastText(hex))).
			Bold(true).
			Render(label)
	}

	hex := func(hex string) string {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render(hex)
	}

	var cols []string
	cols = append(cols,
		lipgloss.JoinVertical(lipgloss.Left,
			swatch(a.HexMain(), "Main"),
			hex(a.HexMain()),
		),
		lipgloss.JoinVertical(lipgloss.Left,
			swatch(a.HexProgressA(), "Progress A"),
			hex(a.HexProgressA()),
		),
		lipgloss.JoinVertical(lipgloss.Left,
			swatch(a.HexProgressB(), "Progress B"),
			hex(a.HexProgressB()),
		),
		lipgloss.JoinVertical(lipgloss.Left,
			swatch(a.HexLyric(), "Lyric"),
			hex(a.HexLyric()),
		),
	)

	header := lipgloss.NewStyle().Bold(true).Render("Extracted Accent Colors")
	fmt.Println(header)
	fmt.Println()
	fmt.Println(lipgloss.JoinHorizontal(lipgloss.Top, cols...))

	if a.IsDark() {
		fmt.Printf("\n  tone: dark  (L* = %.0f)\n", lightness(a.Main))
	} else {
		fmt.Printf("\n  tone: light (L* = %.0f)\n", lightness(a.Main))
	}
}

func renderMockUI(a accent.Accent) {
	bg := lipgloss.NewStyle().Background(lipgloss.Color("#1a1a2e")).Width(88).Padding(0)

	header := lipgloss.NewStyle().Bold(true).Padding(0, 1).Render("UI Preview")
	fmt.Println(header)
	fmt.Println()

	section := func(title string, blocks ...string) string {
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Width(42).
			Padding(0, 1).
			Render(
				lipgloss.JoinVertical(lipgloss.Left,
					append([]string{
						lipgloss.NewStyle().Bold(true).PaddingBottom(1).Render(title),
					}, blocks...)...,
				),
			)
	}

	defaultColors := defaultSection()
	accentedColors := accentedSection(a)

	w := lipgloss.Width
	_ = w

	fmt.Println(lipgloss.JoinHorizontal(lipgloss.Top,
		bg.Render(section("Default (purple)", defaultColors...)),
		section("Accented", accentedColors...),
	))
}

func defaultSection() []string {
	var lines []string

	tabs := lipgloss.JoinHorizontal(lipgloss.Top,
		tabBlock("Home", "#57"),
	)
	lines = append(lines, tabs)

	pb := progressBarGradient(0.6, "#DA70D6", "#8A2BE2", 36)
	lines = append(lines, pb)

	vb := volumeBar(0.8, "#00FF00", "#FF0000", 36)
	lines = append(lines, vb)

	lyric := lipgloss.NewStyle().
		Foreground(lipgloss.Color("141")).
		Italic(true).
		Render("♪ And I'm feeling good...")
	lines = append(lines, lyric)

	return lines
}

func accentedSection(a accent.Accent) []string {
	var lines []string

	tabs := lipgloss.JoinHorizontal(lipgloss.Top,
		tabBlock("Home", a.HexMain()),
	)
	lines = append(lines, tabs)

	pb := progressBarGradient(0.6, a.HexProgressA(), a.HexProgressB(), 36)
	lines = append(lines, pb)

	vb := volumeBar(0.8, "#00FF00", "#FF0000", 36)
	lines = append(lines, vb)

	lyric := lipgloss.NewStyle().
		Foreground(lipgloss.Color(a.HexLyric())).
		Italic(true).
		Render("♪ And I'm feeling good...")
	lines = append(lines, lyric)

	return lines
}

func tabBlock(name string, active string) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(active)).
		Padding(0, 1).
		Bold(true)

	return style.Render(" " + name + " ")
}

func progressBarGradient(pct float64, from, to string, w int) string {
	cf, _ := colorful.Hex(from)
	ct, _ := colorful.Hex(to)

	fw := int(float64(w) * pct)
	if fw < 0 {
		fw = 0
	}
	if fw > w {
		fw = w
	}
	ew := w - fw

	var chars []string
	for i := 0; i < fw; i++ {
		t := 0.5
		if fw > 1 {
			t = float64(i) / float64(fw-1)
		}
		c := cf.BlendHcl(ct, t)
		chars = append(chars, lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex())).Render("█"))
	}
	filled := strings.Join(chars, "")

	unfilled := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(strings.Repeat("░", ew))
	pctStr := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(fmt.Sprintf(" %d%%", int(pct*100)))

	return filled + unfilled + pctStr
}

func volumeBar(pct float64, fill, empty string, w int) string {
	fw := int(float64(w-2) * pct)
	ew := w - 2 - fw
	if fw < 0 {
		fw = 0
	}
	if ew < 0 {
		ew = 0
	}

	filled := lipgloss.NewStyle().Foreground(lipgloss.Color(fill)).Render(strings.Repeat("▰", fw))
	unfilled := lipgloss.NewStyle().Foreground(lipgloss.Color(empty)).Render(strings.Repeat("▱", ew))
	pctStr := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(fmt.Sprintf(" %d%%", int(pct*100)))

	return "🔊 " + filled + unfilled + pctStr + "  "
}

func contrastText(bgHex string) string {
	c := hexToColorful(bgHex)
	l, _, _ := c.Lab()
	if l > 55 {
		return "#000000"
	}
	return "#ffffff"
}

func hexToColorful(hex string) colorful.Color {
	c, _ := colorful.Hex(hex)
	return c
}

func lightness(c colorful.Color) float64 {
	l, _, _ := c.Lab()
	return l
}

func synthImage() image.Image {
	w, h := 400, 400
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	bg := colorful.Hsv(250, 0.8, 0.6)
	fg := colorful.Hsv(200, 0.9, 0.75)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dist := math.Sqrt(
				math.Pow(float64(x-w/2)/float64(w/2), 2)+
					math.Pow(float64(y-h/2)/float64(h/2), 2),
			)
			t := math.Min(dist*1.2, 1.0)
			c := bg.BlendHcl(fg, 1-t)
			r, g, b := c.Clamped().RGB255()
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}
	return img
}
