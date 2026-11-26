package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func TestPlaybackModeFooterUsesCurrentModeAndTheme(t *testing.T) {
	themes := []struct {
		name  string
		icons IconSet
		glyph []string
	}{
		{name: "emoji", icons: EmojiIcons, glyph: []string{"📋", "🔂", "🔀"}},
		{name: "nerd", icons: NerdIcons, glyph: []string{"\U000f0411", "\U000f0458", "\U000f049d"}},
		{name: "unicode", icons: FallbackIcons, glyph: []string{"→", "↻1", "⇄"}},
	}
	for _, theme := range themes {
		for i, mode := range []string{config.PlaybackOrder, config.PlaybackRepeatOne, config.PlaybackShuffle} {
			t.Run(theme.name+"/"+mode, func(t *testing.T) {
				m := lyricFooterModel()
				m.Config.PlaybackMode = mode
				m.pages.draft.PlaybackMode = config.PlaybackShuffle
				m.Icons = theme.icons
				handleResize(m, tea.WindowSizeMsg{Width: 80, Height: 24})
				rows := strings.Split(stripANSI(renderFooter(m, m.layoutPlan())), "\n")
				controls := rows[3]
				want := theme.glyph[i] + " " + playbackModeLabel(mode)
				if !strings.Contains(controls, want) {
					t.Fatalf("controls %q do not contain %q", controls, want)
				}
				if strings.Index(controls, theme.glyph[i]) < strings.Index(controls, theme.icons.Next) {
					t.Fatal("mode indicator must follow transport controls")
				}
			})
		}
	}
}

func TestPlaybackModeFooterFitsThemesSizesAndWideVolume(t *testing.T) {
	for _, icons := range []IconSet{EmojiIcons, NerdIcons, FallbackIcons} {
		for _, size := range [][2]int{{120, 32}, {80, 24}, {68, 17}} {
			for _, width := range []int{16, 55, 120} {
				t.Run(fmt.Sprintf("%dx%d/volume%d/%s", size[0], size[1], width, icons.Volume), func(t *testing.T) {
					m := lyricFooterModel()
					m.Icons = icons
					m.Config.PlaybackMode = config.PlaybackRepeatOne
					m.Components.VolumeBar.SetWidth(width)
					m.UI.Focus = FocusFooter
					m.Audio.Volume = 1
					handleResize(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
					plan := m.layoutPlan()
					footer := renderFooter(m, plan)
					if lipgloss.Width(footer) != size[0] || lipgloss.Height(footer) != plan.FooterRows {
						t.Fatalf("footer is %dx%d, expected %dx%d", lipgloss.Width(footer), lipgloss.Height(footer), size[0], plan.FooterRows)
					}
					if !strings.Contains(stripANSI(footer), "100%") {
						t.Fatal("narrow footer lost the volume percentage")
					}
					frame := renderMainView(m).Content
					if lipgloss.Width(frame) != size[0] || lipgloss.Height(frame) != size[1] {
						t.Fatal("mode/volume row overflowed the whole frame")
					}
				})
			}
		}
	}
}

func TestPlaybackModeFooterKeepsNoTrackAndLyricsGeometry(t *testing.T) {
	for _, loaded := range []bool{false, true} {
		for _, lyrics := range []bool{false, true} {
			m := lyricFooterModel()
			if !loaded {
				m.Audio.Player = nil
			}
			m.Audio.ShowLyrics = lyrics
			m.Config.PlaybackMode = config.PlaybackShuffle
			handleResize(m, tea.WindowSizeMsg{Width: 68, Height: 17})
			plan := m.layoutPlan()
			footer := renderFooter(m, plan)
			if !strings.Contains(stripANSI(footer), "Shuffle") || lipgloss.Height(footer) != plan.FooterRows {
				t.Fatal("idle/lyric state hid the mode or changed the footer geometry")
			}
		}
	}
}

func TestPlaybackFooterSanitizesWideVolumeFill(t *testing.T) {
	m := lyricFooterModel()
	cfg := m.Config.Clone()
	cfg.VolumeBar.Fill = []string{"界", "界"}
	cfg.VolumeBar.Width = 40
	m.applySavedSettings(cfg, nil)
	m.Audio.Volume = 1
	handleResize(m, tea.WindowSizeMsg{Width: 68, Height: 17})
	plan := m.layoutPlan()
	footer := renderFooter(m, plan)
	if lipgloss.Width(footer) != plan.Width || lipgloss.Height(footer) != plan.FooterRows {
		t.Fatalf("footer is %dx%d, want %dx%d", lipgloss.Width(footer), lipgloss.Height(footer), plan.Width, plan.FooterRows)
	}
	if !strings.Contains(stripANSI(footer), "100%") {
		t.Fatal("footer lost the volume percentage")
	}
}
