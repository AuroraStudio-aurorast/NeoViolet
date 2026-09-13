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
	// Populated by TTML parser from <ttm:agent> + <amll:meta key="artists">.
	Agents map[string]string

	// Properties stores extended metadata (e.g. "ncmMusicId", "musicName").
	// Populated by TTML parser from <amll:meta> elements.
	Properties map[string]string

	// AgentFilter restricts ActiveLines() to a single agent.
	// "" means show all (default).
	AgentFilter string
}

// ActiveLines returns all lines that are active at the given elapsed time.
//
// For lines with End > 0 (TTML): active when begin <= t < end.
// For lines with End == 0 (all other formats): uses CurrentLine() legacy behavior,
// returning at most one line whose Time is the greatest <= elapsed.
//
// When AgentFilter is set, only lines matching that agent are returned.
func (d *Data) ActiveLines(elapsed time.Duration) []LyricLine {
	if len(d.Lines) == 0 {
		return nil
	}

	// Phase 1: collect End-bounded active lines
	var active []LyricLine
	anyBounded := false
	for _, line := range d.Lines {
		if line.End > 0 {
			anyBounded = true
			if line.Time <= elapsed && elapsed < line.End {
				active = append(active, line)
			}
		}
	}

	// Phase 2: if no bounded lines matched, fall back to legacy behavior
	if !anyBounded {
		idx := d.CurrentLine(elapsed)
		if idx >= 0 {
			active = append(active, d.Lines[idx])
		}
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
