package ui

import (
	"sort"
	"strings"

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
// A blank line gets no suggestions on purpose: textinput keeps matched
// suggestions across Reset/SetValue and only recomputes them from the list it
// already holds, so a stale entry would make <tab> on an empty command line
// complete a command the user never typed.
func syncGhostSuggestions(m *Model) {
	ti := &m.Components.CommandInput
	if strings.TrimSpace(ti.Value()) == "" {
		ti.SetSuggestions(nil)
		return
	}
	ti.SetSuggestions(ghostSuggestions())
}

// segment describes the whitespace-delimited word the cursor is in: the text
// before the cursor inside it, and the [Start,End) interval it replaces.
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
	Path  bool   // true for filesystem paths (three-tier rendering, spec §7.3)
}

// completionContextAt splits value on whitespace while tracking offsets, then
// reports the segment the cursor sits in. Offsets are rune indexes, matching
// textinput's Position()/SetCursor().
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
		if runes[i] == ' ' || runes[i] == '\t' {
			i++
			continue
		}
		start := i
		for i < len(runes) && runes[i] != ' ' && runes[i] != '\t' {
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
	}

	before := make([]string, 0, len(fields))
	for _, f := range fields {
		before = append(before, f.text)
	}
	return completionContext{
		Seg:    segment{Index: len(fields), Prefix: "", Start: pos, End: pos},
		Before: before,
	}
}

// candidatesFor resolves the segment to its candidate source (spec §7.2).
// Previous segments are matched exactly: ":lrc sw " lists nothing rather than
// guessing what "sw" meant.
func candidatesFor(ctx completionContext) []candidate {
	switch ctx.Seg.Index {
	case 0:
		return filterCandidates(commandCandidates(), ctx.Seg.Prefix)
	case 1:
		spec, ok := commandLookup(ctx.Before[0])
		if !ok || spec.Name != "lrc" {
			return nil
		}
		return filterCandidates(lrcSubcommandCandidates(), ctx.Seg.Prefix)
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
// <enter> (spec §7.4 / D-note L4).
func syncCompletion(m *Model) {
	syncGhostSuggestions(m)

	ti := &m.Components.CommandInput
	if m.UI.Mode != ModeCommand || strings.TrimSpace(ti.Value()) == "" {
		m.completionCandidates = nil
		m.completionIndex = -1
		return
	}
	ctx := completionContextAt(ti.Value(), ti.Position())
	m.completionSeg = ctx.Seg
	m.completionCandidates = candidatesFor(ctx)
	m.completionIndex = -1
}
