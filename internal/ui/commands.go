package ui

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// invocation is a parsed command line: the text as typed, the command name that
// was used (a canonical name or one of its aliases) and the arguments.
//
// Rest keeps everything after the first token verbatim so ":open My  File.mp3"
// reaches the filesystem with both spaces intact; strings.Fields cannot do that
// because it collapses whitespace runs.
type invocation struct {
	Text  string
	Name  string
	Parts []string
	Rest  string
}

// parseInvocation splits one command line. ok is false for a blank line.
func parseInvocation(cmdText string) (inv invocation, ok bool) {
	parts := strings.Fields(cmdText)
	if len(parts) == 0 {
		return invocation{}, false
	}
	// Rest is everything after the first token. Trim with the same whitespace
	// set strings.Fields splits on, so a leading NBSP or newline cannot
	// misalign the slice.
	trimmed := strings.TrimLeftFunc(cmdText, unicode.IsSpace)
	rest := ""
	if i := strings.IndexFunc(trimmed, unicode.IsSpace); i >= 0 {
		rest = strings.TrimLeftFunc(trimmed[i:], unicode.IsSpace)
	}
	return invocation{Text: cmdText, Name: parts[0], Parts: parts, Rest: rest}, true
}

// commandSpec is the single source of truth for one command: dispatch and the
// completion list both read from here, so a candidate can never describe a
// command the dispatcher would reject.
type commandSpec struct {
	Name    string
	Aliases []string
	Args    string // argument hint, e.g. "<path>"; empty when there are none
	Desc    string // one-line English description (completion list, right column)
	Run     func(m *Model, inv invocation) (tea.Model, tea.Cmd)
}

// hint is the right-hand column of a completion row: the argument hint first,
// then the description ("<path>  Load an audio file").
func (s commandSpec) hint() string {
	if s.Args == "" {
		return s.Desc
	}
	return s.Args + "  " + s.Desc
}

// commands lists every command. The zero index of the completion list is built
// from this table.
var commands = []commandSpec{
	{Name: "save", Aliases: []string{"w"}, Desc: "Save configuration", Run: runSave},
	{Name: "wq", Desc: "Save configuration and quit", Run: runSaveQuit},
	{Name: "quit", Aliases: []string{"q"}, Desc: "Quit", Run: runQuit},
	{
		Name:    "quit!",
		Aliases: []string{"q!", "wq!"},
		Desc:    "Force quit without cleanup",
		Run:     runForceQuit,
	},
	{Name: "p", Desc: "Toggle play/pause", Run: runPlayPause},
	{Name: "vol", Args: "<0.0-1.0>", Desc: "Set playback volume", Run: runVolume},
	{Name: "seek", Args: "<time|+/-offset>", Desc: "Seek within the current track", Run: runSeek},
	{
		Name:    "lrc",
		Aliases: []string{"lyric", "lyrics"},
		Args:    "<sub>",
		Desc:    "Lyrics control",
		Run:     runLrc,
	},
	{
		Name:    "open",
		Aliases: []string{"load", "e"},
		Args:    "<path>",
		Desc:    "Load an audio file",
		Run:     runOpen,
	},
}

// commandLookup resolves a typed name (canonical or alias) to its spec.
func commandLookup(name string) (commandSpec, bool) {
	for _, spec := range commands {
		if spec.Name == name {
			return spec, true
		}
		for _, alias := range spec.Aliases {
			if alias == name {
				return spec, true
			}
		}
	}
	return commandSpec{}, false
}
