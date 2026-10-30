package ui

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

func TestCommandLineUsesAccentSurface(t *testing.T) {
	m := completionModel(t, 80, 24)
	m.Accent = &accent.Accent{Main: mustColorful(t, "#46536b")}
	m.Components.CommandInput.SetValue("lrc pan")
	m.Components.CommandInput.SetCursor(7)
	row := renderCommandLine(m)
	if !strings.Contains(row, "48;2;") {
		t.Fatal("command line has no explicit dynamic background")
	}
	if got := stripANSI(row); got[0] != ':' {
		t.Fatalf("command prompt moved: %q", got)
	}
	if got := lipgloss.Width(row); got != m.UI.Width {
		t.Fatalf("row width=%d want %d", got, m.UI.Width)
	}
}

func TestInteractionPaletteMatchesAcceptedDemo(t *testing.T) {
	p := newInteractionPalette(nil)
	for _, tc := range []struct {
		name string
		got  color.RGBA
		want color.RGBA
	}{
		{"command", p.command, color.RGBA{R: 40, G: 33, B: 53, A: 255}},
		{"selected", p.selected, color.RGBA{R: 64, G: 56, B: 78, A: 255}},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want accepted demo %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestInteractionSurfacesStayReadableAcrossAccents(t *testing.T) {
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	errorFG := color.RGBA{R: 255, G: 180, B: 180, A: 255}
	descFG := color.RGBA{R: 181, G: 181, B: 190, A: 255}
	for _, hex := range []string{"#af87ff", "#46536b", "#62cc9b", "#cb5264", "#8baeca", "#ffffff", "#000000"} {
		p := newInteractionPalette(&accent.Accent{Main: mustColorful(t, hex)})
		for _, tc := range []struct{ fg, bg color.RGBA }{
			{white, p.selected}, {errorFG, backgroundReference}, {descFG, p.command}, {p.prompt, p.command},
		} {
			// Test-side luminance does not call the palette's contrast helper.
			a, b := testLuminance(tc.fg), testLuminance(tc.bg)
			ratio := (math.Max(a, b) + 0.05) / (math.Min(a, b) + 0.05)
			if ratio < 4.5 {
				t.Errorf("%s: contrast %.3f below 4.5 for %v on %v", hex, ratio, tc.fg, tc.bg)
			}
		}
	}
}

func TestInteractionRowsAreOpaqueAcrossResizeAndAccentChanges(t *testing.T) {
	m := completionModel(t, 100, 24)
	m.Components.CommandInput.Focus()
	m.Components.CommandInput.SetValue("lrc pan")
	m.Components.CommandInput.SetSuggestions([]string{"lrc panel"})
	for _, size := range [][2]int{{100, 24}, {80, 24}, {68, 17}, {100, 30}} {
		m = sized(t, m, size[0], size[1])
		for _, hex := range []string{"#46536b", "#cb5264", "#62cc9b"} {
			m.Accent = &accent.Accent{Main: mustColorful(t, hex)}
			syncCommandInputWidth(m)
			command := renderCommandLine(m)
			bg := newInteractionPalette(m.Accent).command
			for x, cell := range frameBackgrounds(t, command)[0] {
				if !sameRGB(cell, bg) {
					t.Fatalf("command cell %d lost its surface: %v, want %v", x, cell, bg)
				}
			}
			m.completionCandidates = []candidate{
				{Value: "on", Desc: "Enable lyrics"},
				{Value: "switch", Desc: "Switch lyrics format"},
				{Value: strings.Repeat("中文目录/", 15) + "track.flac", Path: true},
			}
			m.completionIndex = 1
			rows := frameBackgrounds(t, renderCompletion(m, m.layoutPlan()))
			for y, row := range rows {
				if len(row) != m.UI.Width {
					t.Fatalf("candidate row %d has %d cells, want %d", y, len(row), m.UI.Width)
				}
				for x, cell := range row {
					if cell != nil {
						t.Fatalf("candidate cell %d,%d hides the background gradient", x, y)
					}
				}
			}
			assertFrameBackgrounds(t, m)
			m.UI.Mode = ModeNormal
			m.completionCandidates = nil
			assertFrameBackgrounds(t, m)
			m.UI.Mode = ModeCommand
		}
	}
}

func TestErrorKeepsOneRowAndHelpPriority(t *testing.T) {
	m := completionModel(t, 80, 24)
	m.UI.Mode = ModeNormal
	m.Error.Set(strings.Repeat("cannot open 中文文件 ", 12), 90)
	m.Error.Visible = true
	row := renderHelp(m)
	if lipgloss.Height(row) != 1 || lipgloss.Width(row) != m.UI.Width {
		t.Error("long error broke the help row geometry")
	}
	for y, line := range frameBackgrounds(t, row) {
		for x, bg := range line {
			if bg != nil {
				t.Fatalf("error cell %d,%d hides the gradient", x, y)
			}
		}
	}
	m.Info.Set("saved", 90)
	if got := stripANSI(renderHelp(m)); !strings.HasPrefix(got, " Info  saved") {
		t.Error("info no longer takes precedence over errors")
	}
	m.switchingTrack = true
	if !strings.Contains(stripANSI(renderHelp(m)), "Loading audio") {
		t.Error("loading no longer takes precedence over messages")
	}
	m.UI.Mode = ModeCommand
	if got := renderHelp(m); got != renderCommandLine(m) {
		t.Error("command line no longer takes precedence over loading")
	}
}

func TestInteractionPaletteLeavesGUIFrameDefaultsAlone(t *testing.T) {
	m := completionModel(t, 80, 24)
	m.ipcServer = &ipc.Server{}
	view := m.View()
	if view.Content != renderMainView(m).Content || view.BackgroundColor != nil || view.ForegroundColor != nil {
		t.Error("interaction surfaces changed the GUI background exclusion")
	}
}

func TestCommandRenderPreservesBlinkBetweenFrames(t *testing.T) {
	m := completionModel(t, 80, 24)
	ti := &m.Components.CommandInput
	ti.SetValue("seek")
	renderCommandLine(m) // install the palette once
	blink := ti.Focus()
	beforeBlink := ti.View()
	updated, _ := ti.Update(blink())
	*ti = updated
	if ti.View() == beforeBlink {
		t.Fatal("fixture did not advance the cursor blink")
	}
	before := ti.View()
	renderCommandLine(m)
	if after := ti.View(); after != before {
		t.Error("an unchanged palette reset the cursor blink while rendering")
	}
}

func TestCompletionUsesAccentForeground(t *testing.T) {
	m := completionModel(t, 80, 24)
	m.Accent = &accent.Accent{Main: mustColorful(t, "#62cc9b")}
	m.completionCandidates = []candidate{{Value: "lrc", Desc: "Lyrics control"}, {Value: "load", Desc: "Load audio"}}
	m.completionIndex = 1
	rows := strings.Split(renderCompletion(m, m.layoutPlan()), "\n")
	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2", len(rows))
	}
	for i, row := range rows {
		if strings.Contains(row, "48;2;") {
			t.Fatalf("candidate row %d has a background", i)
		}
		if got := lipgloss.Width(row); got != m.UI.Width {
			t.Fatalf("row %d width=%d want %d", i, got, m.UI.Width)
		}
	}
	assertWarningInk(t, rows[1], "load", newInteractionPalette(m.Accent).prompt)
}

func TestErrorUsesSoftRedForeground(t *testing.T) {
	m := completionModel(t, 80, 24)
	m.UI.Mode = ModeNormal
	m.Error.Set("Failed to load audio: file not found", 90)
	m.Error.Visible = true
	got := renderHelp(m)
	if !strings.Contains(got, "38;2;255;180;180") {
		t.Fatalf("error does not use soft red foreground: %q", got)
	}
	if !strings.Contains(stripANSI(got), m.Error.Message) {
		t.Fatal("error text changed")
	}
}
