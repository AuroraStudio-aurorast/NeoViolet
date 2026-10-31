package ui

import (
	"image/color"
	"math"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WhatDamon/go-nvaa-codec/player"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/anim"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/power"
)

// warningForegrounds reads the rendered ANSI through the dependency's parser,
// independently of the warning's styling code. The fixtures use ASCII text.
func warningForegrounds(t *testing.T, frame string) (string, map[int]color.Color) {
	t.Helper()
	parser := ansi.NewParser()
	var state byte
	var foreground color.Color
	var plain strings.Builder
	inks := map[int]color.Color{}
	for len(frame) > 0 {
		seq, width, n, next := ansi.DecodeSequence(frame, state, parser)
		if n == 0 {
			t.Fatal("ANSI decoder made no progress")
		}
		frame, state = frame[n:], next
		if strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m") {
			params := parser.Params()
			if len(params) == 0 {
				foreground = nil
			}
			for i := 0; i < len(params); i++ {
				v := params[i].Param(0)
				switch {
				case v == 0 || v == 39:
					foreground = nil
				case v >= 30 && v <= 37:
					foreground = ansi.BasicColor(v - 30)
				case v >= 90 && v <= 97:
					foreground = ansi.BasicColor(v - 90 + 8)
				case v == 38 || v == 48 || v == 58:
					var c color.Color
					if read := ansi.ReadStyleColor(params[i:], &c); read > 0 {
						if v == 38 {
							foreground = c
						}
						i += read - 1
					}
				}
			}
		}
		if width > 0 || seq == "\n" {
			for i := range len(seq) {
				inks[plain.Len()+i] = foreground
			}
			plain.WriteString(seq)
		}
	}
	return plain.String(), inks
}

func assertWarningInk(t *testing.T, frame, token string, want color.Color) {
	t.Helper()
	plain, inks := warningForegrounds(t, frame)
	start := strings.Index(plain, token)
	if start < 0 {
		t.Fatalf("missing warning text %q", token)
	}
	for i := range len(token) {
		if token[i] == ' ' || token[i] == '\n' {
			continue
		}
		if got := inks[start+i]; !sameRGB(got, want) {
			t.Fatalf("%q byte %d foreground = %v, want %v", token, i, got, want)
		}
	}
}

func TestAnimWarningTitleUsesRiskColour(t *testing.T) {
	m := animModel(t, animGateFixture, 100, 30)
	m.Accent = &accent.Accent{Main: mustColorful(t, "#62cc9b")}
	body := renderAnimWarning(m, m.layoutPlan())
	assertWarningInk(t, body, "Photosensitivity warning", color.RGBA{R: 255, G: 180, B: 180, A: 255})
	assertWarningInk(t, body, "The animation is not playing yet.", color.RGBA{R: 238, G: 238, B: 242, A: 255})
}

func TestBatteryWarningUsesWarmTitleAndCharge(t *testing.T) {
	m := batteryModel(t, animPlainFixture, 18)
	runAnim(m, invocation{})
	body := renderAnimPowerWarning(m, m.layoutPlan())
	warm := color.RGBA{R: 255, G: 208, B: 138, A: 255}
	assertWarningInk(t, body, "Low battery", warm)
	assertWarningInk(t, body, "18%", warm)
	assertWarningInk(t, body, "below 20%", color.RGBA{R: 181, G: 181, B: 190, A: 255})
}

func warningLineContaining(t *testing.T, body, text string) string {
	t.Helper()
	for _, row := range strings.Split(body, "\n") {
		if strings.Contains(ansi.Strip(row), text) {
			return row
		}
	}
	t.Fatalf("no row contains %q", text)
	return ""
}

func TestAnimWarningColoursOnlyExceededMeasurements(t *testing.T) {
	p := gateReadings()
	p.Declared.Verdict = player.VerdictPass
	p.Declared.General, p.Declared.Red, p.Declared.Area = 2, 1, 200
	p.Assessment.GeneralFlashesPerSecond = 5
	p.Assessment.RedFlashesPerSecond = 4
	p.Assessment.MaxFlashAreaPermille = 251 // rounds to 25%, but exceeds 250
	body := layoutWarnLines(warnDetail(p, newAnimWarningPalette(nil)), 64, 17)
	white := color.RGBA{R: 238, G: 238, B: 242, A: 255}
	risk := color.RGBA{R: 255, G: 180, B: 180, A: 255}
	muted := color.RGBA{R: 181, G: 181, B: 190, A: 255}
	declared := warningLineContaining(t, body, "the file says")
	assertWarningInk(t, declared, "2/s", white)
	assertWarningInk(t, declared, "1/s", white)
	assertWarningInk(t, declared, "20%", white)
	analysed := warningLineContaining(t, body, "  our analysis ")
	assertWarningInk(t, analysed, "our analysis", white)
	assertWarningInk(t, analysed, "5/s", risk)
	assertWarningInk(t, analysed, "4/s", risk)
	assertWarningInk(t, analysed, "25%", risk)
	limit := warningLineContaining(t, body, "  limit ")
	assertWarningInk(t, limit, "limit", muted)
	assertWarningInk(t, limit, "3/s", muted)
	assertWarningInk(t, limit, "25%", muted)
	assertWarningInk(t, body, "WCAG 2.2", muted)

	// Threshold values themselves are not excessive, and missing area is not a
	// measured zero. Non-default analysis area limits must remain authoritative.
	p.Declared.Verdict = player.VerdictFail
	p.Declared.General, p.Declared.Red, p.Declared.HasArea = 3, 3, false
	p.Assessment.GeneralFlashesPerSecond, p.Assessment.RedFlashesPerSecond = 3, 3
	p.Assessment.MaxFlashAreaPermille, p.Assessment.AreaThresholdPermille = 300, 300
	body = layoutWarnLines(warnDetail(p, newAnimWarningPalette(nil)), 64, 17)
	declared = warningLineContaining(t, body, "the file says")
	assertWarningInk(t, declared, "3/s", white)
	assertWarningInk(t, declared, "-", white)
	analysed = warningLineContaining(t, body, "  our analysis ")
	assertWarningInk(t, analysed, "3/s", white)
	assertWarningInk(t, analysed, "30%", white)
}

func TestAnimWarningMissingAnalysisAndCompactRate(t *testing.T) {
	p := gateReadings()
	p.Assessment = anim.Photosensitivity{}.Assessment
	p.Declared.HasArea = false
	palette := newAnimWarningPalette(nil)
	detail := layoutWarnLines(warnDetail(p, palette), 64, 17)
	if strings.Contains(ansi.Strip(detail), "our analysis ") {
		t.Error("missing analysis became a row of measured zeroes")
	}
	if !strings.Contains(ansi.Strip(detail), "Our analysis could not run on it.") {
		t.Error("missing-analysis explanation was lost")
	}
	compact := layoutWarnLines(warnCompact(p, palette), 62, 4)
	assertWarningInk(t, compact, "5", color.RGBA{R: 255, G: 180, B: 180, A: 255})
	assertWarningInk(t, compact, "limit 3/s", color.RGBA{R: 238, G: 238, B: 242, A: 255})
	if !strings.Contains(ansi.Strip(compact), "Esc") {
		t.Error("compact warning lost its way out")
	}
}

func TestBatteryWarningColoursItsChargeInBothLayouts(t *testing.T) {
	palette := newAnimWarningPalette(nil)
	reading := power.Status{Percent: 5, OnBattery: true, Known: true}
	for _, tc := range []struct {
		lines []warnLine
		h     int
	}{
		{lines: powerWarnDetail(reading, 20, palette), h: 17},
		{lines: powerWarnCompact(reading, 20, palette), h: 4},
	} {
		body := layoutWarnLines(tc.lines, 64, tc.h)
		assertWarningInk(t, body, "5%", color.RGBA{R: 255, G: 208, B: 138, A: 255})
		assertWarningInk(t, body, "below 20%", color.RGBA{R: 181, G: 181, B: 190, A: 255})
		if strings.Contains(ansi.Strip(body), "not playing yet") || strings.Contains(ansi.Strip(body), "WCAG") {
			t.Error("battery warning acquired photosensitivity-specific content")
		}
	}
}

func TestAnimWarningForegroundsStayReadableWithoutAddingBackgrounds(t *testing.T) {
	blue := color.RGBA{R: 70, G: 83, B: 107, A: 255}
	for _, hex := range []string{"#af87ff", "#46536b", "#62cc9b", "#cb5264", "#8baeca", "#000000", "#ffffff"} {
		a := &accent.Accent{Main: mustColorful(t, hex)}
		palette := newAnimWarningPalette(a)
		body := layoutWarnLines(warnDetail(gateReadings(), palette), 64, 17)
		for y, row := range frameBackgrounds(t, body) {
			for x, bg := range row {
				if bg != nil {
					t.Fatalf("%s warning cell %d,%d acquired a background: %v", hex, x, y, bg)
				}
			}
		}
		plain, inks := warningForegrounds(t, body)
		bg := color.RGBA{R: 28, G: 22, B: 41, A: 255} // brightest global reference
		for i, c := range []byte(plain) {
			if c == ' ' || c == '\n' {
				continue
			}
			fg := inks[i]
			if fg == nil {
				t.Fatalf("%s warning text at %d has no explicit foreground", hex, i)
			}
			r, g, b, _ := fg.RGBA()
			y := testLuminance(color.RGBA{R: uint8((r >> 8) & 255), G: uint8((g >> 8) & 255), B: uint8((b >> 8) & 255), A: 255})
			z := testLuminance(bg)
			ratio := (math.Max(y, z) + .05) / (math.Min(y, z) + .05)
			if ratio < 4.5 {
				t.Fatalf("%s text at %d contrast %.2f below 4.5", hex, i, ratio)
			}
		}
		if hex == "#46536b" && sameRGB(palette.key.GetForeground(), blue) {
			t.Error("raw dark accent was not lifted for the warning keys")
		}
	}
	if sameRGB(newAnimWarningPalette(&accent.Accent{Main: mustColorful(t, "#46536b")}).key.GetForeground(),
		newAnimWarningPalette(&accent.Accent{Main: mustColorful(t, "#62cc9b")}).key.GetForeground()) {
		t.Error("key colour no longer follows the track accent")
	}
}

func TestBatteryApprovalStillStopsAtPhotosensitivityGate(t *testing.T) {
	m := batteryModel(t, animGateFixture, 18)
	_, cmd := runAnim(m, invocation{})
	if cmd != nil || !m.Anim.Held() || m.Anim.Loading {
		t.Fatal("low battery did not hold the load before reading")
	}
	renderAnimPowerWarning(m, m.layoutPlan())
	if m.Anim.Path() != "" {
		t.Fatal("rendering a battery warning read the sidecar")
	}
	_, cmd = handleNormalModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("battery approval no longer schedules its pending load")
	}
	loaded, ok := cmd().(anim.LoadedMsg)
	if !ok {
		t.Fatal("pending load returned an unexpected message")
	}
	handleAnimLoaded(m, loaded)
	if !m.Anim.Gated() || m.Anim.Held() || m.Anim.View() != "" {
		t.Fatal("battery approval bypassed the photosensitivity gate")
	}
	renderAnimWarning(m, m.layoutPlan())
	if !m.Anim.Gated() || m.Anim.View() != "" {
		t.Fatal("rendering a warning started its animation")
	}
	handleNormalModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.animVisible() {
		t.Fatal("escape did not cancel the warning")
	}
}

func TestAnimationWarningSpaceControlsMusicWithoutApproving(t *testing.T) {
	for _, kind := range []string{"photo", "battery"} {
		m := animModel(t, animGateFixture, 100, 30)
		if kind == "battery" {
			m = batteryModel(t, animGateFixture, 18)
			runAnim(m, invocation{})
		}
		player := &mockPlayer{}
		m.Audio.Player = player
		held, gated := m.Anim.Held(), m.Anim.Gated()
		handleNormalModeKeyPress(m, tea.KeyPressMsg{Code: ' ', Text: " "})
		if !player.IsPlaying() {
			t.Errorf("%s warning swallowed music play/pause", kind)
		}
		if m.Anim.Held() != held || m.Anim.Gated() != gated || m.Anim.View() != "" {
			t.Errorf("%s warning was approved by the space bar", kind)
		}
	}
}

func TestAnimWarningForegroundsDoNotChangeGUIDefaults(t *testing.T) {
	for _, kind := range []string{"photo", "battery"} {
		m := animModel(t, animGateFixture, 100, 30)
		if kind == "battery" {
			m = batteryModel(t, animGateFixture, 18)
			runAnim(m, invocation{})
			m = sized(t, m, 100, 30)
		}
		m.ipcServer = &ipc.Server{}
		view := m.View()
		if view.Content != renderMainView(m).Content || view.BackgroundColor != nil || view.ForegroundColor != nil {
			t.Errorf("%s warning changed GUI default colours", kind)
		}
		for y, row := range frameBackgrounds(t, view.Content) {
			for x, bg := range row {
				if bg != nil {
					t.Fatalf("%s GUI cell %d,%d acquired an explicit background", kind, x, y)
				}
			}
		}
	}
}
