package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Every command name and alias resolves and carries a Run: this guards that
// candidates and dispatch stay in sync.
func TestCommandTableIsResolvable(t *testing.T) {
	if len(commands) == 0 {
		t.Fatal("commands table is empty")
	}
	for _, spec := range commands {
		if spec.Name == "" || spec.Desc == "" || spec.Run == nil {
			t.Errorf("incomplete spec: %+v", spec)
		}
		names := append([]string{spec.Name}, spec.Aliases...)
		for _, name := range names {
			got, ok := commandLookup(name)
			if !ok {
				t.Fatalf("commandLookup(%q) not found", name)
			}
			if got.Name != spec.Name {
				t.Errorf("commandLookup(%q).Name = %q, want %q", name, got.Name, spec.Name)
			}
		}
	}
}

// An unknown command only sets an error and never calls a Run (a typo like
// ":w" would write config, ":q" would quit).
func TestUnknownCommandDoesNotRun(t *testing.T) {
	m := setupModel()
	setCommand(m, "nope")
	executeCommand(m)
	if got := m.Error.Message; got != "Unknown command: nope" {
		t.Errorf("error = %q, want %q", got, "Unknown command: nope")
	}
}

// hint is the right-hand text of a candidate row: the argument hint first, a
// one-line description after it.
func TestCommandHint(t *testing.T) {
	if got := commandLookup0(t, "p").hint(); got != "Toggle play/pause" {
		t.Errorf("p hint = %q", got)
	}
	if got := commandLookup0(t, "open").hint(); got != "<path>  Load an audio file" {
		t.Errorf("open hint = %q", got)
	}
}

func commandLookup0(t *testing.T, name string) commandSpec {
	t.Helper()
	spec, ok := commandLookup(name)
	if !ok {
		t.Fatalf("commandLookup(%q) not found", name)
	}
	return spec
}

// parseInvocation must keep the text after the first token verbatim (runs of
// spaces inside it must survive).
func TestParseInvocationKeepsRawRest(t *testing.T) {
	inv, ok := parseInvocation("open /a/b  c.mp3")
	if !ok {
		t.Fatal("parseInvocation returned ok = false")
	}
	if inv.Name != "open" {
		t.Errorf("Name = %q", inv.Name)
	}
	if inv.Rest != "/a/b  c.mp3" {
		t.Errorf("Rest = %q, want %q", inv.Rest, "/a/b  c.mp3")
	}
	if len(inv.Parts) != 3 {
		t.Errorf("Parts = %v, want 3 fields", inv.Parts)
	}
	if _, ok := parseInvocation("   "); ok {
		t.Error("blank input should not parse")
	}
}

// lrcSubcommands must cover dispatch completely: every entry resolves through
// lrcSubcommandLookup and the full subcommand set is present, so a candidate can
// never describe a subcommand the dispatcher would reject.
func TestLrcSubcommandTable(t *testing.T) {
	if len(lrcSubcommands) != 7 {
		t.Fatalf("lrcSubcommands has %d entries, want 7", len(lrcSubcommands))
	}
	seen := map[string]bool{}
	for _, sub := range lrcSubcommands {
		if sub.Name == "" || sub.Desc == "" || sub.Run == nil {
			t.Errorf("incomplete sub spec: %+v", sub)
		}
		if seen[sub.Name] {
			t.Errorf("duplicate subcommand %q", sub.Name)
		}
		seen[sub.Name] = true
		if _, ok := lrcSubcommandLookup(sub.Name); !ok {
			t.Errorf("lrcSubcommandLookup(%q) not found", sub.Name)
		}
	}
	for _, want := range []string{"on", "off", "switch", "refresh", "agent", "desktop", "panel"} {
		if !seen[want] {
			t.Errorf("subcommand %q missing from the table", want)
		}
	}
}

// parseInvocation.Rest must share strings.Fields' notion of whitespace
// (unicode.IsSpace, not just " \t"), otherwise a leading NBSP or newline
// misaligns the slice and Rest keeps the whitespace.
func TestParseInvocationRestUsesFieldsWhitespace(t *testing.T) {
	inv, ok := parseInvocation("open\u00a0/a  b.mp3")
	if !ok {
		t.Fatal("parseInvocation returned ok = false")
	}
	if inv.Rest != "/a  b.mp3" {
		t.Errorf("Rest = %q, want %q", inv.Rest, "/a  b.mp3")
	}
	if inv, _ := parseInvocation("open  /a.mp3"); inv.Rest != "/a.mp3" {
		t.Errorf("Rest = %q, want %q", inv.Rest, "/a.mp3")
	}
	if inv, _ := parseInvocation("open"); inv.Rest != "" {
		t.Errorf("Rest = %q, want empty", inv.Rest)
	}
}

func TestCommandInputSuggestionWiring(t *testing.T) {
	m := setupModel()
	ti := m.Components.CommandInput
	if !ti.ShowSuggestions {
		t.Error("ShowSuggestions = false, want true")
	}
	if ti.CharLimit != 256 {
		t.Errorf("CharLimit = %d, want 256", ti.CharLimit)
	}
	if got := ti.KeyMap.AcceptSuggestion.Keys(); len(got) != 1 || got[0] != "tab" {
		t.Errorf("AcceptSuggestion keys = %v, want [tab]", got)
	}
	if got := ti.KeyMap.NextSuggestion.Keys(); len(got) != 1 || got[0] != "ctrl+n" {
		t.Errorf("NextSuggestion keys = %v, want [ctrl+n]", got)
	}
	if got := ti.KeyMap.PrevSuggestion.Keys(); len(got) != 1 || got[0] != "ctrl+p" {
		t.Errorf("PrevSuggestion keys = %v, want [ctrl+p]", got)
	}
}

// An empty input must not leave a match-everything suggestion list:
// textinput's prefix match is true for the empty string.
func TestSyncCompletionClearsEmptyInput(t *testing.T) {
	m := setupModel()
	ti := &m.Components.CommandInput

	ti.SetValue("vo")
	syncCompletion(m)
	if got := len(ti.MatchedSuggestions()); got == 0 {
		t.Error("matched suggestions = 0 for \"vo\", want the command names")
	}

	ti.SetValue("")
	syncCompletion(m)
	if got := ti.MatchedSuggestions(); len(got) != 0 {
		t.Errorf("empty input matched %v, want none", got)
	}

	// Guards existing behaviour: tab on an empty input must not complete
	// anything. The input is focused because entering command mode focuses it,
	// and textinput ignores every key while it is blurred.
	m.UI.Mode = ModeCommand
	m.Components.CommandInput.Focus()
	updated, _ := handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := updated.(*Model).Components.CommandInput.Value(); got != "" {
		t.Errorf("value after tab on an empty line = %q, want empty", got)
	}
}

// Ghost text: tab completes "vo" to "vol".
func TestGhostTextAcceptsSuggestion(t *testing.T) {
	m := setupModel()
	m.UI.Mode = ModeCommand
	m.Components.CommandInput.Focus()
	m.Components.CommandInput.SetValue("vo")
	syncCompletion(m)

	updated, _ := handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := updated.(*Model).Components.CommandInput.Value(); got != "vol" {
		t.Errorf("value = %q, want \"vol\"", got)
	}
}

// The typing path must refill the suggestion list on every keystroke: textinput
// only narrows the list it already holds, and entering command mode cleared it
// (the line was empty), so without a sync after Update no ghost text appears.
func TestGhostTextAppearsWhileTyping(t *testing.T) {
	m := setupModel()
	m.UI.Mode = ModeCommand
	m.Components.CommandInput.Focus()
	syncCompletion(m) // what entering command mode does: an empty line

	for _, r := range "vo" {
		updated, _ := handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(*Model)
	}
	if got := len(m.Components.CommandInput.MatchedSuggestions()); got == 0 {
		t.Fatal("no suggestion matched after typing \"vo\"")
	}

	updated, _ := handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := updated.(*Model).Components.CommandInput.Value(); got != "vol" {
		t.Errorf("value after tab = %q, want \"vol\"", got)
	}
}
