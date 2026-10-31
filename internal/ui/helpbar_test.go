package ui

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

func TestHelpBar_NormalHintsRemainReadableOnTheFrameGradient(t *testing.T) {
	m := setupModel()
	m.Audio.Player = &mockPlayer{}
	for _, size := range [][2]int{{68, 17}, {80, 24}, {120, 32}} {
		m = sized(t, m, size[0], size[1])
		for _, focus := range []Focus{FocusTabBar, FocusContent, FocusFooter} {
			m.UI.Focus = focus
			row := renderHelp(m)
			plain, inks := warningForegrounds(t, row)
			bg := frameBackground(m)[m.UI.Height-1]
			for i, char := range plain {
				if char == ' ' {
					continue
				}
				if inks[i] == nil {
					t.Fatalf("hint at byte %d has no foreground", i)
				}
				fg := color.RGBAModel.Convert(inks[i]).(color.RGBA)
				if contrast := interactionContrast(fg, bg); contrast < 4.5 {
					t.Fatalf("%v focus %d: hint %q uses %v, contrast %.2f below 4.5",
						size, focus, string(char), fg, contrast)
				}
			}
			cells := frameBackgrounds(t, m.View().Content)
			for x, cell := range cells[m.UI.Height-1] {
				if !sameRGB(cell, bg) {
					t.Fatalf("help background broken at column %d: %v, want %v", x, cell, bg)
				}
			}
		}
	}
}

func TestHelpBar_FollowsFocus(t *testing.T) {
	m := setupModel()
	m.Audio.Player = &mockPlayer{}
	m.UI.Width = 160

	for _, tt := range []struct {
		focus  Focus
		want   []string
		absent []string
	}{
		{FocusTabBar, []string{"←/→ tabs", "tab focus", ": command"}, []string{"seek", "volume", "next track"}},
		{FocusContent, []string{"[/] tabs", "tab focus", ": command"}, []string{"select", "seek", "volume"}},
		{FocusFooter, []string{"←/→ seek", "+/− volume", "tab focus", ": command"}, []string{"select", "next track"}},
	} {
		m.UI.Focus = tt.focus
		got := stripANSI(renderHelp(m))
		for _, want := range tt.want {
			if !strings.Contains(got, want) {
				t.Errorf("focus %d: help %q does not contain %q", tt.focus, got, want)
			}
		}
		for _, absent := range tt.absent {
			if strings.Contains(got, absent) {
				t.Errorf("focus %d: help %q unexpectedly contains %q", tt.focus, got, absent)
			}
		}
	}
}

func TestHelpBar_FollowsPlaybackKey(t *testing.T) {
	m := setupModel()
	m.UI.Width = 160
	m.Audio.Player = &mockPlayer{}
	if got := stripANSI(renderHelp(m)); !strings.Contains(got, "space play") || strings.Contains(got, "pause") {
		t.Fatalf("paused help = %q", got)
	}

	handleNormalModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeySpace})
	if got := stripANSI(renderHelp(m)); !strings.Contains(got, "space pause") || strings.Contains(got, "space play") {
		t.Fatalf("playing help = %q", got)
	}

	handleNormalModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeySpace})
	if got := stripANSI(renderHelp(m)); !strings.Contains(got, "space play") || strings.Contains(got, "pause") {
		t.Fatalf("paused help after toggling = %q", got)
	}
}

func TestHelpBar_HidesUnavailableActions(t *testing.T) {
	m := setupModel()
	m.UI.Focus = FocusFooter
	m.UI.Width = 160
	got := stripANSI(renderHelp(m))
	for _, absent := range []string{"space", "seek", "next track", "prev track"} {
		if strings.Contains(got, absent) {
			t.Errorf("help without audio %q contains %q", got, absent)
		}
	}

	m.ipcServer = &ipc.Server{}
	if got := stripANSI(renderHelp(m)); strings.Contains(got, "ctrl+c") {
		t.Errorf("GUI help advertises disabled quit key: %q", got)
	}
	m.ipcServer = nil
	if got := stripANSI(renderHelp(m)); !strings.Contains(got, "ctrl+c quit") {
		t.Errorf("standalone help omits quit key: %q", got)
	}
}

func TestHelpBar_StaysOneRowAcrossResize(t *testing.T) {
	m := setupModel()
	m.Audio.Player = &mockPlayer{}
	m.Audio.IsPlaying = true
	for _, width := range []int{68, 80, 120, 68} {
		m.UI.Width = width
		for _, focus := range []Focus{FocusTabBar, FocusContent, FocusFooter} {
			m.UI.Focus = focus
			got := renderHelp(m)
			if lipgloss.Width(got) > width || strings.Contains(got, "\n") {
				t.Errorf("%d columns, focus %d: help does not fit one row: %q", width, focus, got)
			}
			plain := stripANSI(got)
			for _, want := range []string{"tab focus", ": command", "space pause"} {
				if !strings.Contains(plain, want) {
					t.Errorf("%d columns, focus %d: help hides %q: %q", width, focus, want, plain)
				}
			}
		}
	}
}
