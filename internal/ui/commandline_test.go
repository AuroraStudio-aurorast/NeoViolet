package ui

import (
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const layoutFullWidth = 79 // m.UI.Width - 1 at the 80 column default

func TestCommandLineLayoutTable(t *testing.T) {
	long := strings.Repeat("x", 90)
	cases := []struct {
		name         string
		value        string
		suggestion   string
		cursorAtEnd  bool
		notice       string
		wantWidth    int
		wantEllipsis bool
	}{
		{"no notice short value", "abc", "", true, "", 78, false},
		{"no notice over wide", long, "", true, "", 77, true},
		{"ghost at end", "o", "open", true, "", 76, false},
		{"ghost mid line", "o", "open", false, "", 75, false},
		{"at limit over wide", long, "", true, "256/256", 69, true},
		{"refusal short value", "abc", "", true, "too long", 69, false},
		{"refusal with ghost at end", "o", "open", true, "too long", 67, false},
		{"refusal with ghost mid line", "o", "open", false, "too long", 66, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotWidth, gotEllipsis, gotNotice := commandLineLayout(
				tc.value, tc.suggestion, tc.cursorAtEnd, tc.notice, layoutFullWidth)
			if gotWidth != tc.wantWidth {
				t.Fatalf("inputWidth = %d, want %d", gotWidth, tc.wantWidth)
			}
			if gotEllipsis != tc.wantEllipsis {
				t.Fatalf("showEllipsis = %v, want %v", gotEllipsis, tc.wantEllipsis)
			}
			if gotNotice != (tc.notice != "") {
				t.Fatalf("showNotice = %v, want %v", gotNotice, tc.notice != "")
			}
		})
	}
}

func TestCommandLineLayoutNarrow(t *testing.T) {
	for _, fullWidth := range []int{1, 2, 3} {
		width, _, _ := commandLineLayout("abc", "", true, "", fullWidth)
		if width < 1 {
			t.Fatalf("fullWidth %d: inputWidth = %d, want >= 1", fullWidth, width)
		}
	}
}

func TestCommandLineLayoutDisplayWidth(t *testing.T) {
	// Three CJK runes take six cells; the layout must work in cells, not runes.
	width, ellipsis, _ := commandLineLayout("歌曲名", "", true, "", layoutFullWidth)
	if width != 78 || ellipsis {
		t.Fatalf("cjk value: inputWidth = %d ellipsis = %v, want 78 false", width, ellipsis)
	}
}

func TestCommandLineLayoutDropsUnfittingNotice(t *testing.T) {
	width, _, showNotice := commandLineLayout("abc", "", true, "256/256", 8)
	if showNotice {
		t.Fatal("showNotice = true, want false when the notice cannot fit")
	}
	plain, _, _ := commandLineLayout("abc", "", true, "", 8)
	if width != plain {
		t.Fatalf("inputWidth = %d, want %d (the no-notice width)", width, plain)
	}
}

func TestCommandInputWidthIsSetOnNewModel(t *testing.T) {
	// The width must be live right after construction: without a resize event
	// the textinput would keep the zero width it is created with.
	m := setupModel()
	if got, want := m.Components.CommandInput.Width(), m.UI.Width-2; got != want {
		t.Fatalf("CommandInput.Width() = %d, want %d", got, want)
	}
}

func TestCommandInputWidthFollowsResize(t *testing.T) {
	m := setupModel()
	nm, _ := updateDispatcher(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(*Model)
	if got, want := m.Components.CommandInput.Width(), m.UI.Width-2; got != want {
		t.Fatalf("after resize: CommandInput.Width() = %d, want %d", got, want)
	}
}

func TestRenderCommandLineWidthIsExactlyTheTerminalWidth(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"short value", "abc"},
		{"value that does not fit", strings.Repeat("x", 200)},
		{"ghost suggestion visible", "o"},
		{"value at the limit", strings.Repeat("x", 256)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := commandModeModel(t, tc.value)
			m.Components.CommandInput.SetCursor(len([]rune(tc.value)))
			syncCompletion(m)
			if got := lipgloss.Width(renderCommandLine(m)); got != m.UI.Width {
				t.Fatalf("command row width = %d, want %d", got, m.UI.Width)
			}
		})
	}
}

func TestRenderCommandLineShowsTheNotice(t *testing.T) {
	m := commandModeModel(t, "abc")
	setCommandNotice(m, "too long")
	if row := renderCommandLine(m); !strings.Contains(row, "too long") {
		t.Fatalf("command row %q does not contain the notice", row)
	}
	if got := lipgloss.Width(renderCommandLine(m)); got != m.UI.Width {
		t.Fatalf("command row width = %d, want %d", got, m.UI.Width)
	}
}

func TestRenderCommandLineShowsEllipsisWhenTheValueDoesNotFit(t *testing.T) {
	m := commandModeModel(t, strings.Repeat("x", 200))
	if row := renderCommandLine(m); !strings.HasPrefix(stripANSI(row), ":…") {
		t.Fatalf("command row %q does not start with the ellipsis marker", row)
	}
}

func TestRenderCommandLineDropsTheNoticeWhenItCannotFit(t *testing.T) {
	m := commandModeModel(t, "abc")
	m.UI.Width = 8
	setCommandNotice(m, "256/256")
	if row := renderCommandLine(m); strings.Contains(row, "256") {
		t.Fatalf("command row %q still shows a notice that cannot fit", row)
	}
}

func TestRenderCommandLineKeepsTheFrameHeight(t *testing.T) {
	build := func(notice string) *Model {
		m := commandModeModel(t, strings.Repeat("x", 200))
		// A fresh model still carries NewModel's default tab width, which wraps the
		// tab row and makes the frame taller than UI.Height. Resize once so the
		// frame-height assertion below measures the geometry a real terminal gives.
		sized, _ := updateDispatcher(m, tea.WindowSizeMsg{Width: m.UI.Width, Height: m.UI.Height})
		m = sized.(*Model)
		if notice != "" {
			setCommandNotice(m, notice)
		}
		return m
	}
	for _, notice := range []string{"", "too long", "256/256"} {
		m := build(notice)
		frame := renderMainView(m).Content
		if got := lipgloss.Height(frame); got != m.UI.Height {
			t.Fatalf("notice %q: frame height = %d, want %d", notice, got, m.UI.Height)
		}
	}
}

func TestRenderHelpDelegatesToRenderCommandLine(t *testing.T) {
	m := commandModeModel(t, "abc")
	setCommandNotice(m, "too long")
	if got, want := renderHelp(m), renderCommandLine(m); got != want {
		t.Fatalf("renderHelp = %q, want the command row %q", got, want)
	}
}
