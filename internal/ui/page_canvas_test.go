package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Replacing text that crosses a wide glyph must not change the canvas width.
func TestPageCanvasClipsWideNamesAndSelections(t *testing.T) {
	m := setupModel()
	c := newPageCanvas(12, 3, m)
	c.text(0, 0, "我听见雨滴落在青青草地")
	c.text(0, 3, "abc")
	c.pair(1, 1, 10, "very long label", "value")
	c.highlight(0, 1, 10)
	c.text(8, 0, "outside")
	for _, line := range strings.Split(c.String(), "\n") {
		if got := lipgloss.Width(line); got != 12 {
			t.Fatalf("width %d, want 12: %q", got, line)
		}
	}
}

// Enter/adjust/sample-save cannot change a real playing stream or its transport.
func TestPageSamplesLeaveTransportUntouched(t *testing.T) {
	m := setupModel()
	m.UI.Focus = FocusContent
	player := &mockPlayer{playing: true, volume: .4}
	m.Audio.Player = player
	m.Audio.Volume = .4
	m.Audio.IsPlaying = true
	for tab := range 3 {
		m.UI.ActiveTab = tab
		pageKey(m, tea.KeyEnter, "")
		for _, r := range "as+=r" {
			pageKey(m, r, string(r))
		}
		pageKey(m, tea.KeyEscape, "")
	}
	if player.playCalled || player.pauseCalled || player.seekCalled || player.closeCalled || player.volume != .4 || !player.playing {
		t.Fatalf("page preview touched playback: %+v", player)
	}
}

// Compact controls must replace, rather than paint over, the longer heading.
func TestPageEffectsCompactHeadingHasNoLeftoverText(t *testing.T) {
	m := lyricFooterModel()
	m.UI.Width = 68
	m.UI.Height = 17
	m.UI.ActiveTab = 2
	got := stripANSI(renderContent(m, m.layoutPlan()))
	if !strings.Contains(got, "EQ / dB") || strings.Contains(got, "equalizer") {
		t.Fatalf("compact heading contains old text: %q", got)
	}
}

func TestPageEffectsShortLayoutKeepsBypassSelectionVisible(t *testing.T) {
	m := setupModel()
	m.UI.Width = 80
	m.UI.Height = 21
	m.UI.ActiveTab = 2
	m.UI.Focus = FocusContent
	p := m.pages
	p.effect = 9
	plan := m.layoutPlan()
	c := newPageCanvas(plan.ContentInnerW, plan.ContentInnerH, m)
	p.renderEffects(c, true, m.effectsStatus())
	if len(c.selections) == 0 {
		t.Fatal("short effects layout hides the selected bypass")
	}
}

// Repeated in-place draws must not retain styles from discarded text.
func TestPageCanvasOverwritesHaveBoundedANSIOutput(t *testing.T) {
	c := newPageCanvas(80, 1, setupModel())
	for i := range 20 {
		c.dim(0, 2+i*3, "│")
	}
	got := c.String()
	if lipgloss.Width(got) != 80 || strings.Count(stripANSI(got), "│") != 20 {
		t.Fatal("overwrites lost visible cells")
	}
	if len(got) > 2048 {
		t.Fatalf("80-cell row grew to %d bytes", len(got))
	}
	if c.String() != got {
		t.Fatal("render mutated canvas")
	}
}
func TestEffectsFrameHasBoundedOutput(t *testing.T) {
	m := setupModel()
	m.UI.Width = 120
	m.UI.Height = 30
	m.UI.ActiveTab = 2
	m.UI.Focus = FocusContent
	for selected := range 10 {
		m.pages.effect = selected
		if frame := m.View().Content; len(frame) > 64*1024 {
			t.Fatalf("selection %d generated %d bytes", selected, len(frame))
		}
	}
}
func TestPageCanvasPreservesEmbeddedInputStylesAndWideCells(t *testing.T) {
	c := newPageCanvas(12, 1, setupModel())
	c.text(0, 0, "界e\u0301\x1b[1;31mA\x1b[0mB")
	c.text(0, 1, "x")
	c.highlight(0, 0, 5)
	got := c.String()
	if plain := stripANSI(got); plain != " xe\u0301AB       " {
		t.Fatalf("bad grapheme overwrite: %q", plain)
	}
	if !strings.Contains(got, "31") || !strings.Contains(got, "\x1b[") {
		t.Fatal("embedded style was lost")
	}
}
