package ui

import (
	"sort"
	"strings"
	"unicode"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

// ghostSuggestions is the list textinput filters for its inline ghost text:
// every command name and alias.
func ghostSuggestions() []string {
	names := make([]string, 0, len(commands)*2)
	for _, spec := range commands {
		names = append(names, spec.Name)
		names = append(names, spec.Aliases...)
	}
	return names
}

// syncGhostSuggestions keeps textinput's own suggestion list in step with the
// input line.
//
// This list feeds the inline ghost text, not the selection: in command mode
// <tab> is intercepted and cycles our own candidates. A blank line gets no
// suggestions on purpose: textinput filters the list it already holds instead of
// rebuilding it from the value, and an empty value is a prefix of every entry,
// so a stale list would show ghost text for a command the user has not started
// typing.
func syncGhostSuggestions(m *Model) {
	ti := &m.Components.CommandInput
	if strings.TrimSpace(ti.Value()) == "" {
		ti.SetSuggestions(nil)
		return
	}
	ti.SetSuggestions(ghostSuggestions())
}

// segment describes the stretch of the command line the cursor is completing:
// the text before the cursor inside the current word, and the [Start,End)
// interval a candidate replaces (empty when the cursor rests between words).
type segment struct {
	Index  int
	Prefix string
	Start  int
	End    int
}

// completionContext is everything the candidate sources need: the current
// segment plus the complete segments before it.
type completionContext struct {
	Seg    segment
	Before []string
}

// candidate is one selectable completion.
type candidate struct {
	Value string // the segment's full replacement value (never an increment)
	Desc  string // right-hand hint; empty for path candidates
	Path  bool   // true for filesystem paths, which render with the three-tier rules
}

// completionContextAt splits value on the whitespace parseInvocation uses
// (unicode.IsSpace) while tracking offsets, then reports the segment the cursor
// sits in: the word it is inside, or the empty interval at the cursor when it
// rests between words. Offsets are rune indexes, matching textinput's
// Position()/SetCursor().
func completionContextAt(value string, pos int) completionContext {
	runes := []rune(value)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}

	type field struct {
		text       string
		start, end int
	}
	var fields []field
	for i := 0; i < len(runes); {
		if unicode.IsSpace(runes[i]) {
			i++
			continue
		}
		start := i
		for i < len(runes) && !unicode.IsSpace(runes[i]) {
			i++
		}
		fields = append(fields, field{text: string(runes[start:i]), start: start, end: i})
	}

	for i, f := range fields {
		if pos >= f.start && pos <= f.end {
			before := make([]string, 0, i)
			for _, prev := range fields[:i] {
				before = append(before, prev.text)
			}
			return completionContext{
				Seg:    segment{Index: i, Prefix: string(runes[f.start:pos]), Start: f.start, End: pos},
				Before: before,
			}
		}
		// The cursor rests in the gap after this word (and before the next
		// one, if any): the completed segment is empty and starts at the cursor.
		if pos > f.end && (i+1 == len(fields) || pos < fields[i+1].start) {
			before := make([]string, 0, i+1)
			for _, prev := range fields[:i+1] {
				before = append(before, prev.text)
			}
			return completionContext{
				Seg:    segment{Index: i + 1, Prefix: "", Start: pos, End: pos},
				Before: before,
			}
		}
	}

	// The cursor sits before the first word: nothing precedes it.
	return completionContext{Seg: segment{Index: 0, Start: pos, End: pos}}
}

// candidatesFor resolves the segment to its candidate source.
// Previous segments are matched exactly: ":lrc sw " lists nothing rather than
// guessing what "sw" meant.
func candidatesFor(ctx completionContext) []candidate {
	switch ctx.Seg.Index {
	case 0:
		return filterCandidates(commandCandidates(), ctx.Seg.Prefix)
	case 1:
		spec, ok := commandLookup(ctx.Before[0])
		if !ok {
			return nil
		}
		switch spec.Name {
		case "lrc":
			return filterCandidates(lrcSubcommandCandidates(), ctx.Seg.Prefix)
		case "open":
			return pathCandidates(ctx.Seg.Prefix)
		}
		return nil
	case 2:
		spec, ok := commandLookup(ctx.Before[0])
		if !ok || spec.Name != "lrc" {
			return nil
		}
		switch ctx.Before[1] {
		case "switch":
			return filterCandidates(lrcFormatCandidates(), ctx.Seg.Prefix)
		case "panel":
			return filterCandidates(panelModeCandidates(), ctx.Seg.Prefix)
		case "agent":
			return filterCandidates(agentCandidates(), ctx.Seg.Prefix)
		}
		return nil
	}
	return nil
}

// commandCandidates lists every command name and alias, sorted by value.
func commandCandidates() []candidate {
	out := make([]candidate, 0, len(commands)*2)
	for _, spec := range commands {
		out = append(out, candidate{Value: spec.Name, Desc: spec.hint()})
		for _, alias := range spec.Aliases {
			out = append(out, candidate{Value: alias, Desc: spec.hint()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

// lrcSubcommandCandidates lists the ":lrc" subcommands (table order).
func lrcSubcommandCandidates() []candidate {
	out := make([]candidate, 0, len(lrcSubcommands))
	for _, sub := range lrcSubcommands {
		out = append(out, candidate{Value: sub.Name, Desc: sub.Desc})
	}
	return out
}

// lrcFormatCandidates lists the values ":lrc switch" accepts: the online fetch
// plus every registered parser, in registration order (no filtering, so a
// candidate can never be missing for an accepted value).
func lrcFormatCandidates() []candidate {
	names := lyrics.AvailableParsers()
	out := make([]candidate, 0, len(names)+1)
	out = append(out, candidate{Value: "online", Desc: "Fetch lyrics from the internet"})
	for _, name := range names {
		out = append(out, candidate{Value: name, Desc: lyrics.ParserDisplayName(name)})
	}
	return out
}

// panelModeCandidates lists the ":lrc panel" modes.
func panelModeCandidates() []candidate {
	return []candidate{
		{Value: config.PanelModeOn, Desc: "Always show the panel"},
		{Value: config.PanelModeOff, Desc: "Never show the panel"},
		{Value: config.PanelModeAuto, Desc: "Show the panel when it fits"},
	}
}

// agentCandidates lists the ":lrc agent" filters. Agent names come from the
// lyric files themselves, so only the "show everything" value can be offered.
func agentCandidates() []candidate {
	return []candidate{{Value: "all", Desc: "Show every agent"}}
}

// filterCandidates keeps the candidates whose value starts with prefix,
// case-insensitively, mirroring textinput's own matching rule.
func filterCandidates(cands []candidate, prefix string) []candidate {
	if prefix == "" {
		return cands
	}
	lower := strings.ToLower(prefix)
	out := make([]candidate, 0, len(cands))
	for _, c := range cands {
		if strings.HasPrefix(strings.ToLower(c.Value), lower) {
			out = append(out, c)
		}
	}
	return out
}

// syncCompletion refreshes the completion state after the input line changed.
// The selection always resets to -1: a passively shown list must not hijack
// <enter>, which keeps executing exactly what was typed.
func syncCompletion(m *Model) {
	syncGhostSuggestions(m)

	ti := &m.Components.CommandInput
	if m.UI.Mode != ModeCommand || strings.TrimSpace(ti.Value()) == "" {
		m.completionCandidates = nil
		m.completionIndex = -1
		m.completionSeg = segment{}
		return
	}
	ctx := completionContextAt(ti.Value(), ti.Position())
	m.completionSeg = ctx.Seg
	m.completionCandidates = candidatesFor(ctx)
	m.completionIndex = -1
}

// acceptCompletion writes the selected candidate into the input line: the text
// between the segment start and the cursor is replaced, so anything after the
// cursor survives.
//
// The candidate list and the segment stay as they are. One tab starts a round
// against what the user typed, and further tabs walk that round's candidates,
// each press replacing the text the previous one wrote: completing a directory
// must not swap the listing under the user's fingers. Only a keystroke that
// reaches the textinput (syncCompletion) recomputes everything.
func acceptCompletion(m *Model, index int) {
	if index < 0 || index >= len(m.completionCandidates) {
		return
	}
	value := m.completionCandidates[index].Value
	ti := &m.Components.CommandInput
	runes := []rune(ti.Value())
	seg := m.completionSeg
	if seg.Start < 0 || seg.End > len(runes) || seg.Start > seg.End {
		return
	}

	updated := string(runes[:seg.Start]) + value + string(runes[seg.End:])
	if len([]rune(updated)) > ti.CharLimit {
		return // never truncate silently: leave the input untouched
	}
	ti.SetValue(updated)
	end := seg.Start + len([]rune(value))
	ti.SetCursor(end)

	// The written value is the segment's text now: the next cycle replaces it.
	m.completionSeg = segment{Index: seg.Index, Prefix: value, Start: seg.Start, End: end}
	m.completionIndex = index
	syncGhostSuggestions(m)
}

// cycleCompletion moves the selection and writes it into the input line. delta
// is +1 for tab/ctrl+n and -1 for ctrl+p.
func cycleCompletion(m *Model, delta int) {
	total := len(m.completionCandidates)
	if total == 0 {
		return
	}
	index := m.completionIndex
	switch {
	case index < 0 && delta < 0:
		index = total - 1
	case index < 0:
		index = 0
	default:
		index = (index + delta + total) % total
	}
	acceptCompletion(m, index)
}
