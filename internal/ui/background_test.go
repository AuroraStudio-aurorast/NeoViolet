package ui

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// reset ends a styled span: the sequence that drops the background a line was
// wrapped in.
const reset = "\x1b[m"

func TestSgrBackground_IsDerivedFromTheColour(t *testing.T) {
	if got, want := backgroundSGR, "\x1b[48;2;0;0;0m"; got != want {
		t.Errorf("backgroundSGR = %q, want %q", got, want)
	}
	// The sequence follows the colour, so changing uiBackground cannot leave the
	// frame painting the old one.
	if got, want := sgrBackground(lipgloss.Color("#102030")), "\x1b[48;2;16;32;48m"; got != want {
		t.Errorf("sgrBackground(#102030) = %q, want %q", got, want)
	}
}

func TestPaintBackground_ReassertsWhereThePenLosesTheColour(t *testing.T) {
	tests := []struct {
		name  string
		frame string
		want  string
	}{
		{"nothing to paint", "", ""},
		{"the first line has no reset to hook onto", "hi", backgroundSGR + "hi"},
		{"every line break", "a\nb", backgroundSGR + "a\n" + backgroundSGR + "b"},
		{"a full reset", "a" + reset + "b", backgroundSGR + "a" + reset + backgroundSGR + "b"},
		{"a spelled-out reset", "a\x1b[0mb", backgroundSGR + "a\x1b[0m" + backgroundSGR + "b"},
		{"the default background", "a\x1b[49mb", backgroundSGR + "a\x1b[49m" + backgroundSGR + "b"},
		{"a trailing reset", "a" + reset, backgroundSGR + "a" + reset + backgroundSGR},
		{
			// A colour on its own does not lose the background, so repainting
			// after it would only add bytes for the parser to walk.
			"a foreground", "a\x1b[38;5;240mb", backgroundSGR + "a\x1b[38;5;240mb",
		},
		{
			// The completion overlay paints its own background on purpose.
			"a background of its own", "a\x1b[0;48;5;236mb", backgroundSGR + "a\x1b[0;48;5;236mb",
		},
		{"a sequence that is not SGR", "a\x1b[2Kb", backgroundSGR + "a\x1b[2Kb"},
		{"an unknown escape", "a\x1b]11;?\x07b", backgroundSGR + "a\x1b]11;?\x07b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := paintBackground(tt.frame); got != tt.want {
				t.Errorf("paintBackground(%q) = %q, want %q", tt.frame, got, tt.want)
			}
		})
	}
}

// The paint is a colour, not a rewrite: the text and the size come out as they
// were.
func TestPaintBackground_KeepsWhatTheFrameSays(t *testing.T) {
	for name, m := range everyScreen(t) {
		t.Run(name, func(t *testing.T) {
			raw := renderMainView(m).Content
			painted := paintBackground(raw)

			if ansi.Strip(painted) != ansi.Strip(raw) {
				t.Error("the paint changed the text")
			}
			if w, h := lipgloss.Width(painted), lipgloss.Height(painted); w != lipgloss.Width(raw) || h != lipgloss.Height(raw) {
				t.Errorf("the paint changed the size: %dx%d, want %dx%d", w, h, lipgloss.Width(raw), lipgloss.Height(raw))
			}
		})
	}
}

// Every frame the program gives the renderer for the alternate screen carries a
// background on every cell it draws, so no cell shows the terminal's own.
func TestView_LeavesNoCellToTheTerminalBackground(t *testing.T) {
	painted, _ := splitScreens(t)
	for name, m := range painted {
		t.Run(name, func(t *testing.T) {
			content := m.View().Content
			if holes := backgroundHoles(content); holes != 0 {
				t.Errorf("%d cells the frame draws would show the terminal background", holes)
			}
			// A frame that does not cover the screen leaves the cells it never
			// draws to the terminal just the same.
			if w, h := lipgloss.Width(content), lipgloss.Height(content); w != m.UI.Width || h != m.UI.Height {
				t.Errorf("the frame covers %dx%d, want the screen's %dx%d", w, h, m.UI.Width, m.UI.Height)
			}
		})
	}
}

// The check above is only worth something if it catches an unpainted frame.
func TestBackgroundHoles_FindsThemInAFrameNothingHasPainted(t *testing.T) {
	m := everyScreen(t)["library"]

	raw := renderMainView(m).Content
	holes := backgroundHoles(raw)
	if holes == 0 {
		t.Fatal("no holes found in a frame that was never painted")
	}
	t.Logf("the unpainted library frame leaves %d cells to the terminal", holes)

	if left := backgroundHoles(paintBackground(raw)); left != 0 {
		t.Errorf("%d cells survived the paint", left)
	}
}

// The frames drawn on the shell's own screen come back exactly as renderMainView
// wrote them: nothing touches the terminal there, neither the cells nor its
// default colours.
func TestView_LeavesTheShellsScreenUntouched(t *testing.T) {
	_, shell := splitScreens(t)
	for name, m := range shell {
		t.Run(name, func(t *testing.T) {
			view := m.View()
			if raw := renderMainView(m).Content; view.Content != raw {
				t.Error("the frame was painted before the interface had the screen")
			}
			if view.BackgroundColor != nil || view.ForegroundColor != nil {
				t.Errorf("the terminal's colours were pinned to %v/%v on the shell's screen",
					view.BackgroundColor, view.ForegroundColor)
			}
		})
	}
}

// The frames on the alternate screen pin the terminal's defaults to the
// interface's colours, so the cells no frame writes -- what a resize clears --
// are the interface's colour too.
func TestView_PinsTheColoursOnTheAlternateScreen(t *testing.T) {
	painted, _ := splitScreens(t)
	for name, m := range painted {
		t.Run(name, func(t *testing.T) {
			view := m.View()
			if view.BackgroundColor != uiBackground {
				t.Errorf("BackgroundColor = %v, want %v", view.BackgroundColor, uiBackground)
			}
			if view.ForegroundColor != uiForeground {
				t.Errorf("ForegroundColor = %v, want %v", view.ForegroundColor, uiForeground)
			}
		})
	}
}

// splitScreens sorts the screens by where they are drawn, refusing to hand out an
// empty half: a check over nothing must not be able to pass.
func splitScreens(t *testing.T) (painted, shell map[string]*Model) {
	t.Helper()

	painted, shell = map[string]*Model{}, map[string]*Model{}
	for name, m := range everyScreen(t) {
		if m.View().AltScreen {
			painted[name] = m
			continue
		}
		shell[name] = m
	}
	if len(painted) == 0 || len(shell) == 0 {
		t.Fatalf("%d screens on the alternate screen and %d on the shell's, want both kinds", len(painted), len(shell))
	}
	return painted, shell
}

// everyScreen is one model per screen renderMainView can produce, so a check over
// them covers all of its return paths rather than the one a default model happens
// to be on.
func everyScreen(t *testing.T) map[string]*Model {
	t.Helper()

	screens := map[string]*Model{}

	library := sized(t, setupModel(), 100, 30)
	screens["library"] = library

	loading := sized(t, setupModel(), 100, 30)
	loading.Loading = true
	screens["loading"] = loading

	screens["terminal too small"] = sized(t, setupModel(), 20, 5)

	command := sized(t, setupModel(), 100, 30)
	command.UI.Mode = ModeCommand
	command.completionCandidates = []candidate{
		{Value: "load", Desc: "<path>  Load an audio file"},
		{Value: "lrc", Desc: "<sub>  Lyrics control"},
		{Value: "queue"},
	}
	screens["command line and completion overlay"] = command

	notice := sized(t, setupModel(), 100, 30)
	notice.Info.Set("saved", 30)
	screens["info message"] = notice

	failure := sized(t, setupModel(), 100, 30)
	failure.Error.Set("could not open the file", 30)
	screens["error message"] = failure

	screens["lyrics panel"] = sized(t, lyricFooterModel(), 80, 24)
	screens["animation"] = sized(t, animModel(t, animPlainFixture, 100, 30), 100, 30)

	return screens
}

// sized puts a model on a terminal of the given size the way the program itself
// does. A model handed a width and a height directly still carries the tab width
// it was built with, and a tab row too wide for the screen wraps and makes the
// frame taller for reasons that have nothing to do with what is being checked.
func sized(t *testing.T, m *Model, w, h int) *Model {
	t.Helper()

	reported, _ := updateDispatcher(m, tea.WindowSizeMsg{Width: w, Height: h})
	sized, ok := reported.(*Model)
	if !ok {
		t.Fatalf("a window size report produced %T, want *Model", reported)
	}
	return sized
}

// backgroundHoles counts the cells a frame draws while no SGR background is set,
// i.e. the ones that show the terminal's own colour. It reads the frame the way
// the renderer's parser does, and is written out here rather than shared with
// paintBackground so that the two have to agree about a frame instead.
func backgroundHoles(frame string) int {
	holes, painted := 0, false
	for i := 0; i < len(frame); {
		if end, params, ok := csiAt(frame, i); ok {
			if frame[end-1] == 'm' {
				painted = backgroundAfter(params, painted)
			}
			i = end
			continue
		}
		if frame[i] < 0x20 || frame[i] == 0x7f {
			i++ // a line break or another control byte draws no cell
			continue
		}
		if r, size := utf8.DecodeRuneInString(frame[i:]); r != utf8.RuneError || size > 1 {
			if !painted {
				holes++
			}
			i += size
			continue
		}
		i++
	}
	return holes
}

// csiAt reports whether a control sequence starts at i, along with its parameter
// bytes and the index just past it.
func csiAt(frame string, i int) (end int, params string, ok bool) {
	if i+1 >= len(frame) || frame[i] != 0x1b {
		return i, "", false
	}
	if frame[i+1] != '[' {
		// A two-byte escape, e.g. an OSC introducer: its payload is not ours to
		// read, and it draws no cell either way.
		return i + 2, "", true
	}
	j := i + 2
	for j < len(frame) && frame[j] >= 0x20 && frame[j] <= 0x3f {
		j++
	}
	if j >= len(frame) {
		return i, "", false
	}
	return j + 1, frame[i+2 : j], true
}

// backgroundAfter returns the background state after an SGR parameter list, given
// the state before it.
func backgroundAfter(params string, painted bool) bool {
	fields := strings.Split(params, ";")
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		if head, _, found := strings.Cut(field, ":"); found {
			// Sub-parameters are the extra values of the same attribute.
			switch head {
			case "48":
				painted = true
			case "", "0", "49":
				painted = false
			}
			continue
		}

		n, err := strconv.Atoi(field)
		if field == "" {
			n, err = 0, nil // an omitted parameter is a zero
		}
		if err != nil {
			continue // an intermediate byte, not a parameter
		}

		switch {
		case n == 48, n >= 40 && n <= 47, n >= 100 && n <= 107:
			painted = true
		case n == 0, n == 49:
			painted = false
		}
		// 38, 48 and 58 are followed by the values describing their colour, which
		// are not parameters of their own: 48;5;49 is a colour index, not a reset.
		if n == 38 || n == 48 || n == 58 {
			i += extendedParams(fields[i+1:])
		}
	}
	return painted
}

// extendedParams counts how many fields the colour of an extended parameter takes.
func extendedParams(fields []string) int {
	if len(fields) == 0 {
		return 0
	}
	switch fields[0] {
	case "5":
		return min(2, len(fields))
	case "2":
		return min(4, len(fields))
	}
	return 0
}
