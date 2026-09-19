package lyrics

import (
	"fmt"
	"strings"
	"time"
)

// WordFragment is a single timed word within a lyric line.
type WordFragment struct {
	Time time.Duration
	Text string
}

// LyricLine is a single timed lyric line, optionally with word fragments.
type LyricLine struct {
	Time  time.Duration
	End   time.Duration // 0 means unbounded (legacy format behavior)
	Text  string
	Words []WordFragment
	Agent string // agent ID (e.g. "v1", "v2") or "" for no agent

	// Parts holds the display sub-lines of an event that carries more than one
	// line of text: LRC merges same-timestamp entries and a SRT cue can have
	// several lines. nil means "plain line, Text is the only display source",
	// which keeps every parser and renderer that ignores sub-lines untouched.
	// Invariant: Parts != nil implies len(Parts) >= 2.
	Parts []string
}

// PartCount returns how many display rows this line has; always at least 1.
func (l LyricLine) PartCount() int {
	if len(l.Parts) == 0 {
		return 1
	}
	return len(l.Parts)
}

// Part returns the i-th display sub-line. i must be in [0, PartCount()); for a
// line without Parts every i returns Text, so callers can loop uniformly.
func (l LyricLine) Part(i int) string {
	if len(l.Parts) == 0 {
		return l.Text
	}
	return l.Parts[i]
}

// Data is the parsed lyric result returned by parsers.
type Data struct {
	Title   string
	Artist  string
	Album   string
	Author  string
	Creator string
	Offset  int
	Lines   []LyricLine
	Path    string
	Format  string // parser name that produced this data ("lrc", "ttml", etc.)

	// Agents maps agent ID to display name (e.g. "v1" -> "Taylor Swift").
	// Populated by the TTML parser from <ttm:agent> + <amll:meta key="artists">,
	// and by the SMI parser from <P Class=...>.
	Agents map[string]string

	// Properties stores extended metadata (e.g. "ncmMusicId", "musicName").
	// Populated by TTML parser from <amll:meta> elements.
	Properties map[string]string

	// AgentFilter restricts ActiveLines() to a single agent.
	// "" means show all (default).
	AgentFilter string
}

// activeLimits[i] is the Time of the first line after i with a greater Time,
// or -1 when there is none. One backward pass keeps the per-line rule of
// ActiveLines O(n) instead of a scan per unbounded line.
//
// The bound is "the next line with a greater Time" rather than "the next line"
// on purpose: lines sharing a Time (a SMI SYNC with several <P Class=...>
// children) must not cut each other off.
func activeLimits(lines []LyricLine) []time.Duration {
	limits := make([]time.Duration, len(lines))
	next := time.Duration(-1)
	for i := len(lines) - 1; i >= 0; i-- {
		limits[i] = next
		if i > 0 && lines[i].Time > lines[i-1].Time {
			next = lines[i].Time
		}
	}
	return limits
}

// ActiveLines returns all lines that are active at the given elapsed time.
//
// A line is bounded when End > Time: it is active for Time <= t < End.
// A line is unbounded when End == 0 or End <= Time: it is active from Time
// until the Time of the next line with a greater Time (the last line never
// expires). A zero-length or reversed interval is treated as unbounded because
// its Time <= t < End window is empty - the line would be unreachable - and
// because the End > 0 => End > Time invariant keeps producers from emitting
// that shape in the first place (ttmlLine normalises it).
//
// This is evaluated per line. Treating "the file contains at least one bounded
// line" as a global switch — the previous shape — made every unbounded line
// unreachable in a mixed file, which is exactly what a SMI file looks like
// (every SYNC bounded except the last one).
//
// When AgentFilter is set, only lines matching that agent are returned.
func (d *Data) ActiveLines(elapsed time.Duration) []LyricLine {
	if len(d.Lines) == 0 {
		return nil
	}

	limit := activeLimits(d.Lines)

	// Phase 1: collect active lines, each against its own window.
	var active []LyricLine
	for i, line := range d.Lines {
		if line.End > line.Time { // bounded: End must be strictly greater than Time
			if line.Time <= elapsed && elapsed < line.End {
				active = append(active, line)
			}
			continue
		}
		if line.Time > elapsed {
			continue
		}
		if limit[i] >= 0 && elapsed >= limit[i] {
			continue
		}
		active = append(active, line)
	}

	// Phase 3: apply agent filter
	if d.AgentFilter != "" && len(active) > 0 {
		var filtered []LyricLine
		for _, line := range active {
			if line.Agent == d.AgentFilter {
				filtered = append(filtered, line)
			}
		}
		return filtered
	}

	return active
}

// LineDisplayText returns the display text for a lyric line,
// including the agent prefix if applicable.
func (d *Data) LineDisplayText(line LyricLine) string {
	if line.Agent == "" {
		return line.Text
	}
	name := ""
	if d.Agents != nil {
		name = d.Agents[line.Agent]
	}
	if name == "" {
		name = strings.ToUpper(line.Agent)
	}
	return fmt.Sprintf("%s: %s", name, line.Text)
}

// VisibleLine couples a lyric line with its index in Data.Lines, so callers can
// map back to the original slice (the panel uses the index for karaoke and for
// locating the current group).
type VisibleLine struct {
	Index int
	Line  LyricLine
}

// VisibleLines returns the displayable lines in time order: AgentFilter applied
// and lines whose display text is blank dropped (LRC files commonly use empty
// lines as separators). The panel builds its context window from this; the
// one_line renderer keeps using ActiveLines/CurrentLine and is unaffected.
func (d *Data) VisibleLines() []VisibleLine {
	if d == nil || len(d.Lines) == 0 {
		return nil
	}

	out := make([]VisibleLine, 0, len(d.Lines))
	for i := range d.Lines {
		line := d.Lines[i]
		if d.AgentFilter != "" && line.Agent != d.AgentFilter {
			continue
		}
		if strings.TrimSpace(d.LineDisplayText(line)) == "" {
			continue
		}
		out = append(out, VisibleLine{Index: i, Line: line})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
