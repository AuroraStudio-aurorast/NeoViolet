package lyrics

import (
	"strings"
	"time"

	amllttml "github.com/WhatDamon/go-amll-ttml-parser"
)

// ttmlToData maps an AMLL TTML document onto Data. It is a pure mapping: no IO
// and no time arithmetic, since the library has already resolved every interval.
//
// Data.Format is deliberately absent: registry.go is its only writer.
func ttmlToData(doc *amllttml.Document, sourcePath string) *Data {
	data := &Data{
		Path:       sourcePath,
		Properties: ttmlProperties(doc),
		Agents:     ttmlAgents(doc),
	}
	// The guard is live, not dead code: Parse never returns nil Metadata, but a
	// hand-built zero-value Document reaches it.
	if md := doc.Metadata; md != nil {
		data.Title = ttmlFirst(md.Titles)
		data.Artist = ttmlFirst(md.Artists)
		data.Album = ttmlFirst(md.Albums)
		data.Creator = ttmlFirst(md.AuthorNames)
	}

	for _, l := range doc.Lines {
		line, ok := ttmlLine(l, doc)
		if !ok {
			continue
		}
		data.Lines = append(data.Lines, line)
	}
	// The library keeps document order, which need not be time order, and both the
	// cross-format contract and CurrentLine's binary search need ascending Time.
	sortLyricLines(data.Lines)
	return data
}

// ttmlLine maps one <p> onto a LyricLine. ok is false for a line with nothing to
// display at all, which would render as a blank row; XML formatting turns real
// files full of those.
func ttmlLine(l *amllttml.Line, doc *amllttml.Document) (LyricLine, bool) {
	segments := ttmlSegments(l, doc)
	if len(segments) == 0 {
		return LyricLine{}, false
	}

	// Two or more segments become parts; a lone segment is the whole display text.
	// For a <p> with no original text at all - only a background vocal, or only a
	// translation - that lone segment is what keeps the line from vanishing.
	parts := partsOrNil(segments)
	text := segments[0]
	if parts != nil {
		text = strings.Join(parts, " | ")
	}

	// EffectiveInterval is the library's port of the reference
	// calculateTimeRange: the declared <p> interval widened to cover its words and
	// its background vocal.
	iv := l.EffectiveInterval()
	startMs, endMs := iv.BeginMillis(), iv.EndMillis()
	if endMs <= startMs {
		// A zero-length or reversed interval has no usable duration: keeping the
		// library's value would make the per-line window Time <= t < End empty (the
		// line would never display) and break the End > 0 => End > Time invariant. One
		// real corpus file declares begin == end == 03:49.093
		// (ncm-lyrics/2158558246.ttml). End == 0 is the unbounded sentinel, which is
		// also where a missing end attribute lands.
		endMs = 0
	}

	return LyricLine{
		Time:  millisToDuration(startMs),
		End:   millisToDuration(endMs),
		Text:  text,
		Words: ttmlWords(l),
		Agent: l.AgentID,
		Parts: parts,
	}, true
}

// ttmlSegments returns the display segments of a line in the fixed order
// [original, background vocal, translation], dropping empty ones and later
// duplicates.
//
// The result is deliberately unfolded: partsOrNil collapses a single segment back
// to nil to preserve "Parts != nil implies len >= 2", so the caller still has to
// tell "nothing to show" apart from "one segment to show".
func ttmlSegments(l *amllttml.Line, doc *amllttml.Document) []string {
	candidates := []string{l.Text}
	if l.Background != nil {
		candidates = append(candidates, l.Background.Text)
	}
	if tr := ttmlTranslationText(l, doc); tr != "" {
		candidates = append(candidates, tr)
	}

	var parts []string
	for _, c := range candidates {
		if strings.TrimSpace(c) == "" {
			continue
		}
		seen := false
		for _, p := range parts {
			if p == c {
				seen = true
				break
			}
		}
		if !seen {
			parts = append(parts, c)
		}
	}
	return parts
}

// ttmlTranslationText returns the translation segment of a line, or "".
//
// Keyed lines use the key-based lookup. Keyless ones have to read their own inline
// tracks: the library indexes lines by key and skips empty keys, so
// TranslationsFor("") cannot see them. That lookup does collect head-side <text>
// elements with no for attribute - they match the "" key, and one of them would
// translate every keyless line at once - so those are ignored here, as they are on
// the keyed path, where a for-less <text> matches no key either. A head block that
// does not name the line it translates therefore shows nothing, an accepted
// limitation of the format rather than of this mapping.
//
// Both paths put the inline translation first, so [0] is the inline one and a
// head-only file still yields its translation. Keyless lines read their inline
// tracks in the same order: the line's own, then its background vocal's.
func ttmlTranslationText(l *amllttml.Line, doc *amllttml.Document) string {
	if l.Key == "" {
		if len(l.Translations) > 0 {
			return l.Translations[0].Text
		}
		if l.Background != nil && len(l.Background.Translations) > 0 {
			return l.Background.Translations[0].Text
		}
		return ""
	}
	if tr := doc.TranslationsFor(l.Key); len(tr) > 0 {
		return tr[0].Text
	}
	return ""
}

// ttmlWords maps the words of the ORIGINAL segment only: the background vocal and
// the translation are display parts without word timing, and the karaoke renderer
// highlights the original.
//
// Word.Text never contains a space; the library reports one in EndsWithSpace
// instead, so the trailing space is appended here. That is what makes the
// fragments concatenate back to the display text.
func ttmlWords(l *amllttml.Line) []WordFragment {
	if len(l.Words) == 0 {
		return nil
	}
	words := make([]WordFragment, 0, len(l.Words))
	for _, w := range l.Words {
		if w.Text == "" {
			continue
		}
		text := w.Text
		if w.EndsWithSpace {
			text += " "
		}
		words = append(words, WordFragment{Time: w.Begin, Text: text})
	}
	if len(words) == 0 {
		return nil
	}
	return words
}

// millisToDuration converts the millisecond counts Interval reports into a
// time.Duration. Zero stays zero: End == 0 is the unbounded sentinel, and a line
// without an end attribute must not become bounded by the conversion.
func millisToDuration(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

// ttmlProperties projects every <amll:meta> pair onto a map, keeping the FIRST
// value of a repeated key: Props keeps document order and duplicates, so the first
// match wins. That is a deliberate change from the hand-written parser, whose map
// was overwritten last-wins.
//
// The map is non-nil even when the document has no <head> metadata, like the other
// parsers'.
func ttmlProperties(doc *amllttml.Document) map[string]string {
	props := make(map[string]string)
	if doc.Metadata == nil {
		return props
	}
	for _, p := range doc.Metadata.Props {
		if _, seen := props[p.Key]; seen {
			continue
		}
		props[p.Key] = p.Value
	}
	return props
}

// ttmlFirst returns the first element of a metadata projection, or "".
func ttmlFirst(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// ttmlAgents names every declared agent, preferring an explicit <ttm:name>, then
// the i-th amll:meta key="artists" value, then the uppercased id ("v3" -> "V3").
// The explicit name is new with this renovation; the other two steps are what the
// hand-written parser did.
//
// AgentOrder is the declarations' document order: the Agents map's iteration order
// is random and cannot be used for the artists correspondence. The result is
// non-nil even when no agent is declared.
func ttmlAgents(doc *amllttml.Document) map[string]string {
	var artists []string
	if doc.Metadata != nil {
		artists = doc.Metadata.Artists
	}
	agents := make(map[string]string, len(doc.AgentOrder))
	for i, id := range doc.AgentOrder {
		name := doc.Agents[id].Name()
		if name == "" && i < len(artists) {
			name = artists[i]
		}
		if name == "" {
			name = strings.ToUpper(id)
		}
		agents[id] = name
	}
	return agents
}
