package wizard

import (
	"image/color"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

func TestLogoMatchesReference(t *testing.T) {
	for _, tc := range []struct {
		name string
		dark bool
	}{
		{name: "dark", dark: true},
		{name: "light", dark: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want, err := os.ReadFile("testdata/logo-" + tc.name + ".golden")
			if err != nil {
				t.Fatal(err)
			}
			got := renderLogo(tc.dark)
			if got != string(want) {
				t.Error("logo differs from reference half-block rendering")
			}
			if lipgloss.Width(got) != 38 || lipgloss.Height(strings.TrimSuffix(got, "\n")) != 11 {
				t.Error("logo must occupy 38 columns and 11 rows")
			}
		})
	}
}

func TestLogoNoteRequestsBackground(t *testing.T) {
	note := newLogoNote()
	model, cmd := note.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	if model != note {
		t.Fatal("updates must retain the background-aware note")
	}
	if cmd == nil || cmd() != tea.RequestBackgroundColor() {
		t.Fatal("logo must request the terminal background color")
	}
	if _, repeat := note.Update(tea.WindowSizeMsg{Width: 80, Height: 40}); repeat != nil {
		t.Error("background query must only be sent once")
	}
	if !strings.Contains(note.View(), "38;2;255;255;255") ||
		strings.Contains(note.View(), "38;2;24;24;24") {
		t.Error("logo must use light text until a background response arrives")
	}
}

func TestLogoNoteFollowsTerminalBackground(t *testing.T) {
	note := newLogoNote()
	form := huh.NewForm(huh.NewGroup(note, huh.NewInput().Title("Name")))
	form.WithWidth(80).WithHeight(40)
	for _, tc := range []struct {
		name       string
		background color.Color
		want       string
	}{
		{name: "light", background: color.White, want: "38;2;24;24;24"},
		{name: "dark", background: color.Black, want: "38;2;255;255;255"},
		{name: "light again", background: color.White, want: "38;2;24;24;24"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			form.Update(tea.BackgroundColorMsg{Color: tc.background})
			got := form.View()
			if !strings.Contains(got, tc.want) {
				t.Errorf("logo did not switch text palette for %s background", tc.name)
			}
			if !strings.Contains(form.View(), "Welcome to NeoViolet") {
				t.Error("welcome note disappeared from the form")
			}
		})
	}
}
