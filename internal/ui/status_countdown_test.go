package ui

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
)

// statusCountdownCells checks the final rendered row, including the global wash.
func statusCountdownCells(t *testing.T, m *Model) int {
	t.Helper()
	rows := frameBackgrounds(t, m.View().Content)
	if len(rows) != m.UI.Height {
		t.Fatalf("frame height = %d, want %d", len(rows), m.UI.Height)
	}
	row := rows[len(rows)-1]
	if len(row) != m.UI.Width {
		t.Fatalf("help width = %d, want %d", len(row), m.UI.Width)
	}
	wash := frameBackground(m)
	base := wash[len(wash)-1]
	filled, ended := 0, false
	for x, bg := range row {
		if bg == nil {
			t.Fatalf("cell %d lost its background", x)
		}
		if sameRGB(bg, base) {
			ended = true
			continue
		}
		if ended {
			t.Fatalf("countdown background has a gap at cell %d", x)
		}
		filled++
	}
	return filled
}

func TestStatusCountdownDrainsBeforeDismissal(t *testing.T) {
	for _, kind := range []string{"info", "saved", "error"} {
		t.Run(kind, func(t *testing.T) {
			m := sized(t, setupModel(), 80, 24)
			m.Config.TickRate = 30
			message := m.Info
			if kind == "error" {
				message = m.Error
			}
			message.Set("Countdown test", 90)
			if kind == "saved" {
				message.SetSuccess("Countdown test", 90)
			}
			text := stripANSI(renderHelp(m))
			previous := 80
			for tick := 0; tick < 90; tick++ {
				filled := statusCountdownCells(t, m)
				if tick == 0 && filled != 80 {
					t.Fatalf("initial bar = %d cells, want 80", filled)
				}
				if tick == 45 && (filled < 35 || filled > 40) {
					t.Fatalf("halfway bar = %d cells, want about half", filled)
				}
				if filled > previous {
					t.Fatalf("bar grew at tick %d: %d > %d", tick, filled, previous)
				}
				if tick >= 85 && filled != 0 {
					t.Fatalf("bar did not reach zero before expiry: tick %d, cells %d", tick, filled)
				}
				if !message.Visible || stripANSI(renderHelp(m)) != text {
					t.Fatalf("message disappeared or shifted before expiry at tick %d", tick)
				}
				previous = filled
				message.Tick()
			}
			if message.Visible || message.Message != "" || statusCountdownCells(t, m) != 0 {
				t.Fatal("expired message left text or a countdown background")
			}
			if !strings.Contains(stripANSI(renderHelp(m)), "focus") {
				t.Fatal("expiry did not restore normal shortcuts")
			}
		})
	}
}

func TestStatusCountdownReplacementStartsFull(t *testing.T) {
	m := sized(t, setupModel(), 80, 24)
	m.Config.TickRate = 30
	m.Info.SetSuccess("Configuration saved", 90)
	for range 60 {
		m.Info.Tick()
	}
	if statusCountdownCells(t, m) >= 40 {
		t.Fatal("fixture did not advance the first countdown")
	}
	m.Info.Set("Lyrics enabled", 30)
	if got := statusCountdownCells(t, m); got != 80 {
		t.Fatalf("replacement did not reset the bar: %d cells", got)
	}
	if !strings.HasPrefix(stripANSI(renderHelp(m)), " Info  Lyrics enabled") {
		t.Fatal("replacement retained the previous message or success category")
	}
	for range 30 {
		m.Info.Tick()
	}
	if m.Info.Visible {
		t.Fatal("replacement retained the previous dismissal duration")
	}
}

func TestStatusCountdownEndHoldFollowsTickRate(t *testing.T) {
	for _, rate := range []int{10, 30, 60} {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			m := sized(t, setupModel(), 80, 24)
			m.Config.TickRate = rate
			m.Info.Set("Lyrics enabled", rate*3)
			// 160 ms rounded up to the next available frame.
			hold := int(math.Ceil(float64(rate) * .16))
			for range rate*3 - hold {
				m.Info.Tick()
			}
			if !m.Info.Visible || statusCountdownCells(t, m) != 0 {
				t.Fatal("message did not remain visible at the empty endpoint")
			}
			for range hold {
				m.Info.Tick()
			}
			if m.Info.Visible {
				t.Fatal("endpoint hold extended the existing dismissal timer")
			}
		})
	}
}

func TestStatusCountdownPreservesWideTextAndInk(t *testing.T) {
	m := sized(t, setupModel(), 68, 17)
	m.Config.TickRate = 30
	m.Error.Set("Cannot open 中文目录/e\u0301/🎵.flac", 90)
	text := stripANSI(renderHelp(m))
	for range 90 {
		row := renderHelp(m)
		if stripANSI(row) != text || lipgloss.Width(row) != 68 || lipgloss.Height(row) != 1 {
			t.Fatalf("countdown split a grapheme or changed geometry: %q", stripANSI(row))
		}
		assertWarningInk(t, row, "Error", color.RGBA{R: 255, G: 180, B: 180, A: 255})
		assertWarningInk(t, row, "Cannot open", color.RGBA{R: 222, G: 222, B: 222, A: 255})
		m.Error.Tick()
	}
}

func TestStatusCountdownSurvivesResizeAndThemeChanges(t *testing.T) {
	m := sized(t, setupModel(), 80, 24)
	m.Config.TickRate = 30
	m.Info.Set("Lyrics enabled", 90)
	for range 45 {
		m.Info.Tick()
	}
	for _, width := range []int{68, 100, 80} {
		m = sized(t, m, width, 24)
		for _, hex := range []string{"#af87ff", "#60b5a6", "#df9b67", "#ffffff", "#000000"} {
			m.Accent = &accent.Accent{Main: mustColorful(t, hex)}
			filled := statusCountdownCells(t, m)
			if filled < width/3 || filled > width/2 {
				t.Fatalf("%s at width %d: progress restarted or overflowed: %d", hex, width, filled)
			}
			bg := frameBackgrounds(t, m.View().Content)[23][0]
			r, g, b, _ := bg.RGBA()
			a := color.RGBA{R: uint8((r >> 8) & 255), G: uint8((g >> 8) & 255), B: uint8((b >> 8) & 255), A: 255}
			fg := color.RGBA{R: 222, G: 222, B: 222, A: 255}
			ratio := (testLuminance(fg) + .05) / (testLuminance(a) + .05)
			if ratio < 4.5 {
				t.Fatalf("%s: status text contrast = %.2f", hex, ratio)
			}
		}
	}
}

func TestStatusCountdownHandlesUntimedAndShortMessages(t *testing.T) {
	m := sized(t, setupModel(), 68, 17)
	for _, ticks := range []int{0, -1, 1, 2} {
		m.Info.Set("Lyrics enabled", ticks)
		for tick := 0; tick <= max(1, ticks); tick++ {
			if row := renderHelp(m); lipgloss.Width(row) > 68 || lipgloss.Height(row) != 1 {
				t.Fatal("short or untimed message broke row geometry")
			}
			m.Info.Tick()
		}
		if m.Info.Visible != (ticks <= 0) {
			t.Fatalf("timer %d changed dismissal behavior", ticks)
		}
		if ticks <= 0 && statusCountdownCells(t, m) != 0 {
			t.Fatal("untimed message shows a misleading countdown")
		}
	}
}
