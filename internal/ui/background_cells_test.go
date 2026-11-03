package ui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
)

// frameBackgrounds uses the ANSI decoder and its colour reader independently of
// paintBackground's SGR scan. Grapheme widths include blank and wide cells.
func frameBackgrounds(t *testing.T, frame string) [][]color.Color {
	t.Helper()
	rows := [][]color.Color{{}}
	var background color.Color
	parser := ansi.NewParser()
	var state byte
	for len(frame) > 0 {
		seq, width, n, next := ansi.DecodeSequence(frame, state, parser)
		if n == 0 {
			t.Fatal("ANSI decoder made no progress")
		}
		frame, state = frame[n:], next
		if strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m") {
			params := parser.Params()
			if len(params) == 0 {
				background = nil
			}
			for i := 0; i < len(params); i++ {
				v := params[i].Param(0)
				switch {
				case v == 0 || v == 49:
					background = nil
				case v >= 40 && v <= 47:
					background = ansi.BasicColor(v - 40)
				case v >= 100 && v <= 107:
					background = ansi.BasicColor(v - 100 + 8)
				case v == 38 || v == 48 || v == 58:
					var c color.Color
					read := ansi.ReadStyleColor(params[i:], &c)
					if read > 0 {
						if v == 48 {
							background = c
						}
						i += read - 1
					}
				}
			}
		}
		if seq == "\n" {
			rows = append(rows, []color.Color{})
		}
		for range width {
			rows[len(rows)-1] = append(rows[len(rows)-1], background)
		}
	}
	return rows
}

func sameRGB(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func TestPaintBackground_MixedResetsAndOwnBackgrounds(t *testing.T) {
	wash := []color.RGBA{{R: 1, G: 2, B: 3, A: 255}, {R: 28, G: 22, B: 41, A: 255}}
	for _, seq := range []string{
		"\x1b[m", "\x1b[0m", "\x1b[49m", "\x1b[0;1;38;2;49;0;22m",
		"\x1b[38;5;49;49m", "\x1b[49;38:2::0:49:22m", "\x1b[48;5;236;0;1m",
	} {
		t.Run(fmt.Sprintf("%q", seq), func(t *testing.T) {
			raw := "before" + seq + "中e\u0301 \n" + seq + "after  "
			painted := paintBackground(raw, wash)
			if ansi.Strip(raw) != ansi.Strip(painted) {
				t.Fatal("paint changed the text")
			}
			for y, row := range frameBackgrounds(t, painted) {
				for x, c := range row {
					if !sameRGB(c, wash[y]) {
						t.Fatalf("cell %d,%d = %v, want %v", x, y, c, wash[y])
					}
				}
			}
		})
	}
	for _, seq := range []string{"\x1b[0;48;5;236m", "\x1b[49;48:2::49:0:22m", "\x1b[40m", "\x1b[100m"} {
		t.Run(fmt.Sprintf("own %q", seq), func(t *testing.T) {
			raw := seq + "overlay\x1b[49;1mnormal"
			painted := frameBackgrounds(t, paintBackground(raw, wash))[0]
			original := frameBackgrounds(t, raw)[0]
			for x, c := range painted {
				want := color.Color(wash[0])
				if x < 7 {
					want = original[x]
				}
				if !sameRGB(c, want) {
					t.Fatalf("cell %d = %v, want %v", x, c, want)
				}
			}
		})
	}
}

func assertFrameBackgrounds(t *testing.T, m *Model) {
	t.Helper()
	raw := frameBackgrounds(t, renderMainView(m).Content)
	painted := frameBackgrounds(t, m.View().Content)
	wash := frameBackground(m)
	if len(painted) != m.UI.Height {
		t.Fatalf("frame height = %d, want %d", len(painted), m.UI.Height)
	}
	own := 0
	for y, row := range painted {
		if len(row) != m.UI.Width {
			t.Fatalf("row %d width = %d, want %d", y, len(row), m.UI.Width)
		}
		for x, c := range row {
			want := color.Color(wash[y])
			if raw[y][x] != nil {
				want = raw[y][x]
				own++
			}
			if !sameRGB(c, want) {
				t.Fatalf("cell %d,%d = %v, want %v", x, y, c, want)
			}
		}
	}
	if m.UI.Mode == ModeCommand && len(m.completionCandidates) > 0 && own == 0 {
		t.Fatal("test missed overlay backgrounds")
	}
}

func TestView_GradientCoversEveryCell(t *testing.T) {
	painted, _, _ := splitScreens(t)
	for name, m := range painted {
		t.Run(name, func(t *testing.T) {
			for _, hex := range []string{"#af87ff", "#60b5a6", "#485b80"} {
				m.Accent = &accent.Accent{Main: mustColorful(t, hex)}
				assertFrameBackgrounds(t, m)
			}
		})
	}
}

func TestView_GradientSurvivesResizeModeAndOverlayChanges(t *testing.T) {
	m := panelSongs(t, 0, 40, 20)
	for _, size := range [][2]int{{120, 30}, {100, 30}, {80, 24}, {68, 17}, {120, 30}} {
		m = sized(t, m, size[0], size[1])
		for _, hex := range []string{"#af87ff", "#df9b67"} {
			m.Accent = &accent.Accent{Main: mustColorful(t, hex)}
			assertFrameBackgrounds(t, m)
			m.UI.Mode = ModeCommand
			m.completionCandidates = []candidate{{Value: "load", Desc: "load a file"}, {Value: "lrc"}}
			assertFrameBackgrounds(t, m)
			m.UI.Mode = ModeNormal
			m.completionCandidates = nil
			assertFrameBackgrounds(t, m)
		}
	}
	m = sized(t, m, 60, 24)
	if m.View().BackgroundColor != nil || m.View().Content != renderMainView(m).Content {
		t.Fatal("too-small frame changed")
	}
}
