package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/WhatDamon/go-nvaa-codec/photosensitivity"
	"github.com/charmbracelet/x/ansi"
)

func TestWarningSummaryAttributesAPassingAnalysis(t *testing.T) {
	readings := gateReadings()
	readings.Assessment.Verdict = photosensitivity.VerdictPass
	readings.Assessment.GeneralFlashesPerSecond = 0
	readings.Assessment.MaxFlashAreaPermille = 0
	body := layoutWarnLines(warnSummary(readings, newAnimWarningPalette(nil)), 38, 10)
	words := strings.Join(strings.Fields(ansi.Strip(body)), " ")
	for _, want := range []string{
		"Our analysis: 0/s (limit 3/s).",
		"File check failed; our analysis passed.",
		"Esc Cancel animation", "Enter Play anyway",
	} {
		if !strings.Contains(words, want) {
			t.Errorf("narrow summary lost %q:\n%s", want, words)
		}
	}
}

func TestAnimationWarningsKeepRiskAndActionsAfterWrapping(t *testing.T) {
	for _, size := range [][2]int{{38, 9}, {38, 14}, {66, 9}, {78, 16}, {118, 25}} {
		for _, kind := range []string{"photo", "battery"} {
			m := animModel(t, animGateFixture, 80, 24)
			if kind == "battery" {
				m = batteryModel(t, animPlainFixture, 18)
				runAnim(m, invocation{})
			}
			plan := m.layoutPlan()
			plan.ContentInnerW, plan.ContentInnerH = size[0], size[1]
			body := renderAnimWarning(m, plan)
			want := []string{"Photosensitivity warning", "trigger seizures", "not playing yet"}
			if kind == "battery" {
				body = renderAnimPowerWarning(m, plan)
				want = []string{"Low battery", "18%", "20%", "drain the battery faster"}
			}
			plain := ansi.Strip(body)
			// Ignore wrapping and indentation when checking whether the message survived.
			words := strings.Join(strings.Fields(plain), " ")
			for _, text := range append(want, "Esc Cancel animation", "Enter Play anyway") {
				if !strings.Contains(words, text) {
					t.Errorf("%s %v lost %q:\n%s", kind, size, text, plain)
				}
			}
			if strings.Index(words, "Esc") > strings.Index(words, "Enter") {
				t.Errorf("%s %v puts the override before cancellation", kind, size)
			}
			if lipgloss.Height(body) != size[1] || lipgloss.Width(body) > size[0] {
				t.Errorf("%s %v overflowed: %dx%d", kind, size, lipgloss.Width(body), lipgloss.Height(body))
			}
			if kind == "photo" && size[0] >= 78 {
				for _, text := range []string{"the file says", "our analysis", "limit", "WCAG"} {
					if !strings.Contains(words, text) {
						t.Errorf("%v dropped detail despite available space: %s", size, text)
					}
				}
			}
		}
	}
}

func TestAnimationWarningReadingBlockStaysCenteredInWideContent(t *testing.T) {
	m := animModel(t, animGateFixture, 120, 32)
	m.Audio.ShowLyrics = false
	body := renderAnimWarning(m, m.layoutPlan())
	line := ansi.Strip(warningLineContaining(t, body, "warning"))
	left := len(line) - len(strings.TrimLeft(line, " "))
	if left < 25 || left > 35 {
		t.Errorf("wide warning is not centered as a reading block: left inset %d", left)
	}
	for _, row := range strings.Split(ansi.Strip(body), "\n") {
		if lipgloss.Width(strings.TrimLeft(row, " ")) > 58 {
			t.Errorf("reading line exceeds the block width: %q", row)
		}
	}
}
