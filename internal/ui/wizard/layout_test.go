package wizard

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// A form taller than the terminal forces the inline renderer to scroll on
// every frame, so the welcome flow appears blank or flickers instead of settling.
func TestSetupFormFitsTerminal(t *testing.T) {
	selected := IconFallback
	var sfPath string
	form := newSetupForm(&selected, &sfPath, t.TempDir())
	_ = form.Init()
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 60, Height: 24}, {Width: 120, Height: 40}, {Width: 80, Height: 24}} {
		_, _ = form.Update(size)
		view := form.View().Content
		if !strings.Contains(view, "Select your favorite") || !strings.Contains(view, "Choose a SoundFont") {
			t.Errorf("%dx%d: setup choices are hidden", size.Width, size.Height)
		}
		if h := lipgloss.Height(view); h > size.Height-1 {
			t.Errorf("%dx%d: form height %d leaves no room for the inline cursor", size.Width, size.Height, h)
		}
		if w := lipgloss.Width(view); w > size.Width {
			t.Errorf("%dx%d: form width %d overflows", size.Width, size.Height, w)
		}
	}
}
