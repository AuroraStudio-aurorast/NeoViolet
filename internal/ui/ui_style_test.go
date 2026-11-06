package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func TestUIStyle_ContentSpace(t *testing.T) {
	for _, tt := range []struct{ w, h, iw, ih int }{{80, 24, 78, 16}, {68, 17, 66, 9}, {120, 32, 82, 25}} {
		m := lyricFooterModel()
		m.Config.Lyrics.Panel.Width = config.PanelWidthAuto
		handleResize(m, tea.WindowSizeMsg{Width: tt.w, Height: tt.h})
		p := m.layoutPlan()
		if p.ContentInnerW != tt.iw || p.ContentInnerH != tt.ih {
			t.Errorf("%dx%d: content %dx%d, want %dx%d", tt.w, tt.h, p.ContentInnerW, p.ContentInnerH, tt.iw, tt.ih)
		}
	}
}

func TestUIStyle_FrameFitsAllIconsAndFocusStates(t *testing.T) {
	for _, icons := range []IconSet{NerdIcons, FallbackIcons, EmojiIcons} {
		for _, size := range [][2]int{{68, 17}, {80, 24}, {100, 28}, {120, 32}} {
			for _, focus := range []Focus{FocusTabBar, FocusContent, FocusFooter} {
				for _, volume := range []float64{0, .65, 1} {
					m := lyricFooterModel()
					m.Icons = icons
					m.UI.Focus = focus
					m.Audio.Volume = volume
					handleResize(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
					frame := renderMainView(m).Content
					if lipgloss.Width(frame) != size[0] || lipgloss.Height(frame) != size[1] {
						t.Fatalf("%dx%d focus%d volume%g: frame %dx%d", size[0], size[1], focus, volume, lipgloss.Width(frame), lipgloss.Height(frame))
					}
					tabs := renderTabs(m)
					if lipgloss.Height(tabs) != 2 {
						t.Fatal("tabs must occupy two rows")
					}
					plain := stripANSI(tabs)
					for _, name := range m.UI.Tabs {
						if !strings.Contains(plain, name) {
							t.Errorf("missing tab %q", name)
						}
					}
					if strings.ContainsAny(plain, "╭╰▸›") {
						t.Fatal("tabs have boxes or focus arrows")
					}
					if lipgloss.Height(renderFooter(m, m.layoutPlan())) != m.layoutPlan().FooterRows {
						t.Fatal("footer must fit its layout budget")
					}
				}
			}
		}
	}
}

func TestUIStyle_FocusChangesColorWithoutMovingContent(t *testing.T) {
	m := lyricFooterModel()
	handleResize(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	var tabs, content, footer []string
	for _, focus := range []Focus{FocusTabBar, FocusContent, FocusFooter} {
		m.UI.Focus = focus
		tabs = append(tabs, renderTabs(m))
		content = append(content, renderContent(m, m.layoutPlan()))
		footer = append(footer, renderFooter(m, m.layoutPlan()))
	}
	for _, frames := range [][]string{tabs, content, footer} {
		for _, s := range frames[1:] {
			if stripANSI(s) != stripANSI(frames[0]) {
				t.Fatal("focus moved or changed content")
			}
		}
	}
	if tabs[0] == tabs[1] || content[0] == content[1] || footer[1] == footer[2] {
		t.Fatal("focused region has no visible style change")
	}
	if strings.ContainsAny(stripANSI(content[1]), "│╭╰") {
		t.Fatal("content still has a surrounding box")
	}
}

func TestUIStyle_CompletionFitsReducedFooter(t *testing.T) {
	for _, size := range [][2]int{{68, 17}, {80, 24}, {120, 32}} {
		m := setupModel()
		handleResize(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		before := strings.Split(renderMainView(m).Content, "\n")
		handleNormalModeKeyPress(m, tea.KeyPressMsg{Code: ':', Text: ":"})
		p := m.layoutPlan()
		if got := completionRows(m, p); got != p.FooterRows {
			t.Fatalf("%v: list %d rows, footer %d", size, got, p.FooterRows)
		}
		after := strings.Split(renderMainView(m).Content, "\n")
		if len(after) != size[1] {
			t.Fatalf("%v: overlay changed height", size)
		}
		for y := 0; y < tabsHeight+p.ContentHeight; y++ {
			if before[y] != after[y] {
				t.Fatalf("overlay changed content row %d", y)
			}
		}
	}
}
