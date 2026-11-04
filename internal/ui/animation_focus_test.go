package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestAnimationFocus_AnimLabelOwnsContentFocus(t *testing.T) {
	for _, fixture := range []string{animPlainFixture, animGateFixture} {
		m := animModel(t, fixture, 100, 30)
		m.UI.Focus = FocusFooter
		blurred := renderTabs(m)
		m.UI.Focus = FocusContent
		focused := renderTabs(m)
		if !strings.Contains(ansi.Strip(focused), " ANIM ") {
			t.Fatalf("%s: missing ANIM label", fixture)
		}
		if strings.Contains(ansi.Strip(focused), "NVAA") {
			t.Fatal("label uses the format name instead of ANIM")
		}
		if focused == blurred {
			t.Fatalf("%s: Content focus is not visible in ANIM label", fixture)
		}
		if ansi.Strip(focused) != ansi.Strip(blurred) {
			t.Fatal("focus changed header geometry")
		}
		if strings.Contains(ansi.Strip(focused), "━") {
			t.Fatal("a page tab claims the animation")
		}
		header := strings.Split(focused, "\n")[0]
		plain := ansi.Strip(header)
		prefix, _, found := strings.Cut(plain, " ANIM ")
		if !found {
			t.Fatal("ANIM is not on the navigation row")
		}
		start := lipgloss.Width(prefix)
		cells := frameBackgrounds(t, header)[0]
		for x, bg := range cells {
			if x >= start && x < start+6 {
				if !sameRGB(bg, newInteractionPalette(m.Accent).prompt) {
					t.Fatalf("ANIM cell %d lost focus colour", x)
				}
			} else if bg != nil {
				t.Fatalf("header cell %d outside ANIM has a background", x)
			}
		}
		handleNormalModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyTab})
		if got := renderTabs(m); got != blurred {
			t.Fatal("ANIM remained highlighted after focus moved")
		}
		if !m.animVisible() {
			t.Fatal("focus change closed animation")
		}
		m.Anim.Close()
		if strings.Contains(ansi.Strip(renderTabs(m)), "ANIM") {
			t.Fatal("ANIM label remained after closing animation")
		}
	}
}

func TestAnimationFocus_TabBarUsesLocalInkWithoutClaimingPage(t *testing.T) {
	m := animModel(t, animPlainFixture, 100, 30)
	m.UI.Focus = FocusFooter
	blurred := renderTabs(m)
	m.UI.Focus = FocusTabBar
	focused := renderTabs(m)
	if focused == blurred {
		t.Fatal("animation leaves Tab focus invisible")
	}
	if ansi.Strip(focused) != ansi.Strip(blurred) {
		t.Fatal("focus changes tab labels or geometry")
	}
	if strings.Contains(ansi.Strip(focused), "━") {
		t.Fatal("a tab claims the animation")
	}
	for y, row := range frameBackgrounds(t, focused) {
		for x, bg := range row {
			if bg != nil {
				t.Fatalf("Tab focus paints background at %d,%d", x, y)
			}
		}
	}
	plain, inks := warningForegrounds(t, focused)
	for _, name := range m.UI.Tabs {
		start := strings.Index(plain, name)
		if start < 0 {
			t.Fatalf("missing tab %q", name)
		}
		for i := range len(name) {
			if !sameRGB(inks[start+i], newInteractionPalette(m.Accent).prompt) {
				t.Fatalf("%s is not emphasized", name)
			}
		}
	}
}

func TestAnimationFocus_LeavesContentBytesUntouched(t *testing.T) {
	for _, fixture := range []string{animPlainFixture, animGateFixture} {
		m := animModel(t, fixture, 100, 30)
		p := m.layoutPlan()
		m.UI.Focus = FocusFooter
		blurred := renderContent(m, p)
		m.UI.Focus = FocusContent
		focused := renderContent(m, p)
		if focused != blurred {
			t.Fatalf("%s: focus changed animation pixels or gutters", fixture)
		}
		if lipgloss.Width(focused) != p.ContentWidth || lipgloss.Height(focused) != p.ContentHeight {
			t.Fatal("animation dimensions changed")
		}
		if raw, ok := animFrame(m, p); ok {
			for _, line := range strings.Split(raw, "\n") {
				if line != "" && !strings.Contains(focused, line) {
					t.Fatal("animation bytes changed")
				}
			}
		}
	}
}

func TestAnimationFocus_HeaderFitsEveryIconAtMinimumSize(t *testing.T) {
	for _, icons := range []IconSet{NerdIcons, FallbackIcons, EmojiIcons} {
		for _, size := range [][2]int{{68, 17}, {80, 24}, {120, 32}} {
			m := animModel(t, animPlainFixture, size[0], size[1])
			m.Icons = icons
			for _, focus := range []Focus{FocusTabBar, FocusContent, FocusFooter} {
				m.UI.Focus = focus
				tabs := renderTabs(m)
				lines := strings.Split(tabs, "\n")
				if len(lines) != 2 {
					t.Fatal("animation header no longer occupies two rows")
				}
				for _, line := range lines {
					if lipgloss.Width(line) != size[0] {
						t.Fatal("animation header does not fit terminal")
					}
				}
				plain := ansi.Strip(lines[0])
				if !strings.HasSuffix(plain, " ANIM   ") {
					t.Fatalf("ANIM not right-aligned: %q", plain)
				}
				for _, name := range m.UI.Tabs {
					if !strings.Contains(plain, name) {
						t.Fatalf("ANIM displaced tab %q", name)
					}
				}
				frame := m.View().Content
				if lipgloss.Width(frame) != size[0] || lipgloss.Height(frame) != size[1] {
					t.Fatal("ANIM changed frame dimensions")
				}
			}
		}
	}
}
