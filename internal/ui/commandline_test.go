package ui

import (
	"fmt"
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
	row := renderCommandLine(m)
	if strings.Contains(row, "256") {
		t.Fatalf("command row %q still shows a notice that cannot fit", row)
	}
	// Contrast against the same row built without a notice at all: a negative
	// assertion alone cannot tell the budget dropping it from the clamp eating it.
	without := commandModeModel(t, "abc")
	without.UI.Width = 8
	syncCommandInputWidth(without)
	if got, want := row, renderCommandLine(without); got != want {
		t.Fatalf("a dropped notice still changed the row:\n got: %q\nwant: %q", got, want)
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

func TestLimitCountAppearsAndDisappears(t *testing.T) {
	m := commandModeModel(t, "")
	key := tea.KeyPressMsg{Code: 'x', Text: "x"}
	for i := 0; i < m.Components.CommandInput.CharLimit; i++ {
		nm, _ := updateDispatcher(m, key)
		m = nm.(*Model)
	}
	if got := m.Components.CommandInput.Value(); len([]rune(got)) != m.Components.CommandInput.CharLimit {
		t.Fatalf("value length = %d, want %d", len([]rune(got)), m.Components.CommandInput.CharLimit)
	}
	row := renderCommandLine(m)
	count := fmt.Sprintf("%d/%d", m.Components.CommandInput.CharLimit, m.Components.CommandInput.CharLimit)
	// A suffix assertion, not a substring one: the notice sits at the end of the
	// row, so it is the first thing a too-narrow row loses. Contains would still
	// pass if only half of it survived.
	if !strings.HasSuffix(stripANSI(row), count) {
		t.Fatalf("command row %q does not end with %s", row, count)
	}
	if got := lipgloss.Width(row); got != m.UI.Width {
		t.Fatalf("command row width = %d, want %d", got, m.UI.Width)
	}

	nm, _ := updateDispatcher(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = nm.(*Model)
	if row := renderCommandLine(m); strings.Contains(row, count) {
		t.Fatalf("command row %q still shows %s after backspace", row, count)
	}
}

func TestCommandInputWidthFollowsTheCurrentLine(t *testing.T) {
	// The input width is derived from the value, the suggestion, the caret and
	// the notice. If any of them changes without a re-sync the input keeps the
	// old width and the assembled row overflows, so the clamp then eats whatever
	// sits at the end of the row: the notice or the caret.
	check := func(t *testing.T, m *Model) {
		t.Helper()
		ti := &m.Components.CommandInput
		want, _, _ := commandLineLayout(ti.Value(), ti.CurrentSuggestion(),
			ti.Position() >= len([]rune(ti.Value())), commandNotice(m), m.UI.Width-1)
		if got := ti.Width(); got != want {
			t.Fatalf("CommandInput.Width() = %d, want %d for a value of %d runes",
				got, want, len([]rune(ti.Value())))
		}
	}

	m := commandModeModel(t, "")
	limit := m.Components.CommandInput.CharLimit
	for i := 0; i < limit; i++ {
		nm, _ := updateDispatcher(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
		m = nm.(*Model)
	}
	check(t, m)

	nm, _ := updateDispatcher(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = nm.(*Model)
	check(t, m)
}

func TestCommandModeKeyClearsTheRefusalNotice(t *testing.T) {
	m := commandModeModel(t, "abc")
	setCommandNotice(m, "too long")
	nm, _ := updateDispatcher(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = nm.(*Model)
	if m.UI.CommandNotice != "" {
		t.Fatalf("CommandNotice = %q, want empty after a keypress", m.UI.CommandNotice)
	}
	if row := renderCommandLine(m); strings.Contains(row, "too long") {
		t.Fatalf("command row %q still shows the notice", row)
	}
}

func TestSyncCompletionResyncsTheWidthWhenTheLineEmpties(t *testing.T) {
	m := commandModeModel(t, "abc")
	setCommandNotice(m, "too long")
	if got := lipgloss.Width(renderCommandLine(m)); got != m.UI.Width {
		t.Fatalf("row width with a notice = %d, want %d", got, m.UI.Width)
	}
	// Leaving command mode drops the notice through setCommandNotice, which syncs
	// the width itself, so on its own this case cannot show that the sync before
	// syncCompletion's early return is load-bearing. The history case below does.
	nm, _ := updateDispatcher(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = nm.(*Model)
	if m.UI.CommandNotice != "" {
		t.Fatalf("CommandNotice = %q, want empty after leaving command mode", m.UI.CommandNotice)
	}
	if got, want := m.Components.CommandInput.Width(), m.UI.Width-2; got != want {
		t.Fatalf("CommandInput.Width() = %d, want %d", got, want)
	}
}

func TestLineEmptiedByHistoryRestoresTheUnsplitWidth(t *testing.T) {
	// "down" past the last history entry resets the line and writes no notice, so
	// only the width sync sitting before syncCompletion's early return can restore
	// the unsplit width: moving that sync below the early return leaves every
	// other width test green.
	m := commandModeModel(t, strings.Repeat("x", 120))
	if got := m.Components.CommandInput.Width(); got == m.UI.Width-2 {
		t.Fatalf("setup: width is already unsplit (%d), so the case cannot discriminate", got)
	}

	nm, _ := updateDispatcher(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = nm.(*Model)
	if got := m.Components.CommandInput.Value(); got != "" {
		t.Fatalf("the line was not emptied: %q", got)
	}
	if got, want := m.Components.CommandInput.Width(), m.UI.Width-2; got != want {
		t.Fatalf("CommandInput.Width() = %d, want %d after the line emptied", got, want)
	}
}

func TestNoticeLeavesEveryOtherRowUntouched(t *testing.T) {
	plain := commandModeModel(t, "abc")
	withNotice := commandModeModel(t, "abc")
	setCommandNotice(withNotice, "too long")

	plainRows := strings.Split(renderMainView(plain).Content, "\n")
	noticeRows := strings.Split(renderMainView(withNotice).Content, "\n")
	if len(plainRows) != len(noticeRows) {
		t.Fatalf("frame heights differ: %d vs %d", len(plainRows), len(noticeRows))
	}
	for i := range plainRows {
		if i == len(plainRows)-1 {
			continue // the command row itself is the one row allowed to differ
		}
		if plainRows[i] != noticeRows[i] {
			t.Fatalf("row %d differs:\n plain: %q\nnotice: %q", i, plainRows[i], noticeRows[i])
		}
	}
	if plainRows[len(plainRows)-1] == noticeRows[len(noticeRows)-1] {
		t.Fatal("the command row is identical with and without a notice")
	}
}

func TestRefusedInsertionShowsTheNotice(t *testing.T) {
	m := commandModeModel(t, "o") // a command-name prefix, so ghost text is on screen
	// A fresh model still carries NewModel's default tab width, which wraps the
	// tab row and makes the frame taller than UI.Height. Resize once so the
	// frame-height assertion below measures the geometry a real terminal gives.
	sized, _ := updateDispatcher(m, tea.WindowSizeMsg{Width: m.UI.Width, Height: m.UI.Height})
	m = sized.(*Model)
	m.Components.CommandInput.SetCursor(0)
	syncCompletion(m)
	before := m.Components.CommandInput.Value()
	nm, _ := updateDispatcher(m, tea.PasteMsg{Content: "/" + strings.Repeat("x", 300)})
	m = nm.(*Model)
	if got := m.Components.CommandInput.Value(); got != before {
		t.Fatalf("value = %q, want it unchanged (%q)", got, before)
	}
	row := renderCommandLine(m)
	if !strings.Contains(row, "too long") {
		t.Fatalf("command row %q does not show the refusal notice", row)
	}
	if got := lipgloss.Width(row); got != m.UI.Width {
		t.Fatalf("command row width = %d, want %d", got, m.UI.Width)
	}
	if got := lipgloss.Height(renderMainView(m).Content); got != m.UI.Height {
		t.Fatalf("frame height = %d, want %d", got, m.UI.Height)
	}
}

func TestInsertionClearsTheNotice(t *testing.T) {
	m := commandModeModel(t, "")
	setCommandNotice(m, "too long")
	nm, _ := updateDispatcher(m, tea.PasteMsg{Content: "/tmp/short.mp3"})
	m = nm.(*Model)
	if m.UI.CommandNotice != "" {
		t.Fatalf("CommandNotice = %q, want empty after a successful insertion", m.UI.CommandNotice)
	}
}

// The clear this case really pins is the one on a successful insertion, not
// handlePaste's own entry clear: the refusal writes the very same notice text,
// so the entry clear is unobservable with the current writers.
func TestPasteClearsAStaleNoticeWhenTheFirstPathIsRefused(t *testing.T) {
	m := commandModeModel(t, "")
	long := "/" + strings.Repeat("x", 300)
	nm, _ := updateDispatcher(m, tea.PasteMsg{Content: long + " /tmp/short.mp3"})
	m = nm.(*Model)
	if m.UI.CommandNotice != "" {
		t.Fatalf("CommandNotice = %q, want empty: a later path was inserted", m.UI.CommandNotice)
	}
}

func TestAcceptCompletionRefusedAtTheLimitShowsTheNotice(t *testing.T) {
	m := commandModeModel(t, "open ")
	m.Components.CommandInput.SetValue("open " + strings.Repeat("a", m.Components.CommandInput.CharLimit-6))
	m.Components.CommandInput.SetCursor(len([]rune(m.Components.CommandInput.Value())))
	syncCompletion(m)
	before := m.Components.CommandInput.Value()
	// A candidate long enough to overflow the line: no real directory entry can
	// reach the limit from this fixture, and without one the refusal branch below
	// is unreachable. The same injection the acceptCompletion overflow test uses.
	m.completionCandidates = []candidate{{Value: strings.Repeat("a", m.Components.CommandInput.CharLimit+4), Path: true}}
	acceptCompletion(m, 0)
	if got := m.Components.CommandInput.Value(); got != before {
		t.Fatalf("value = %q, want it unchanged", got)
	}
	if m.UI.CommandNotice != "too long" {
		t.Fatalf("CommandNotice = %q, want %q", m.UI.CommandNotice, "too long")
	}
	if row := renderCommandLine(m); !strings.Contains(row, "too long") {
		t.Fatalf("command row %q does not show the refusal notice", row)
	}
}
