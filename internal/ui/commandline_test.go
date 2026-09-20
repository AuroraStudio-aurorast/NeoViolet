package ui

import (
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
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
