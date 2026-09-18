package lyrics

import (
	"strings"
	"time"

	amllttml "github.com/WhatDamon/go-amll-ttml-parser"
)

// ttmlToData maps an AMLL TTML document onto Neoviolet's Data. It is a pure
// mapping: no IO and no time arithmetic - the library has already resolved every
// interval, so the only conversion left is milliseconds to time.Duration.
//
// Data.Format is deliberately absent: registry.go sets it to the parser name for
// every format, and none of the other parsers writes it (Path, by contrast, is
// each parser's own responsibility).
func ttmlToData(doc *amllttml.Document, sourcePath string) *Data {
	data := &Data{
		Path:       sourcePath,
		Properties: ttmlProperties(doc),
		Agents:     ttmlAgents(doc),
	}
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
	// The library keeps document order, which need not be time order. The other
	// parsers all end up time-ordered (they sort, or their format is
	// timestamp-prefixed), and CurrentLine's binary search depends on it.
	sortLyricLines(data.Lines)
	return data
}

// ttmlLine maps one <p> onto a LyricLine. ok is false for a line with nothing to
// display (no text and no other display segment), which would render as a blank
// row; XML formatting turns real files full of those.
func ttmlLine(l *amllttml.Line, doc *amllttml.Document) (LyricLine, bool) {
	parts := ttmlParts(l, doc)

	text := l.Text
	if parts != nil {
		text = strings.Join(parts, " | ")
	}
	if text == "" && parts == nil {
		return LyricLine{}, false
	}

	// EffectiveInterval is the library's port of the reference
	// calculateTimeRange: the declared <p> interval widened to cover its words
	// and the background vocal. It is already in whole milliseconds.
	iv := l.EffectiveInterval()

	return LyricLine{
		Time:  millisToDuration(iv.BeginMillis()),
		End:   millisToDuration(iv.EndMillis()),
		Text:  text,
		Words: ttmlWords(l),
		Agent: l.AgentID,
		Parts: parts,
	}, true
}

// ttmlParts returns the display parts of a line in the fixed order
// [original, background vocal, translation], keeping only segments that exist
// and that differ from the ones already collected. partsOrNil then turns a
// single-segment result into nil, preserving "Parts != nil implies len >= 2".
//
// The translation is doc.TranslationsFor(key)[0]: the library merges the inline
// x-translation span and the head-side <text for="key"> block into one list with
// inline first, so the first entry is the inline one and a head-only file still
// yields its translation.
func ttmlParts(l *amllttml.Line, doc *amllttml.Document) []string {
	candidates := []string{l.Text}
	if l.Background != nil {
		candidates = append(candidates, l.Background.Text)
	}
	if tr := doc.TranslationsFor(l.Key); len(tr) > 0 {
		candidates = append(candidates, tr[0].Text)
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
	return partsOrNil(parts)
}

// ttmlWords maps the words of the ORIGINAL segment only: the background vocal
// and the translation are display parts without word timing, and the karaoke
// renderer highlights the original.
//
// Word.Text never contains a space; the library reports one in EndsWithSpace
// instead, so the trailing space is appended here. That is what makes the
// fragments concatenate back to the original text.
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
		// Word.Begin is already an absolute time.Duration from the document
		// start, so no unit conversion is involved.
		words = append(words, WordFragment{Time: w.Begin, Text: text})
	}
	if len(words) == 0 {
		return nil
	}
	return words
}

// millisToDuration converts a millisecond count - the unit Interval's
// BeginMillis/EndMillis report in - to a time.Duration. Zero stays zero: End == 0
// is the unbounded sentinel documented on LyricLine (a line without an end
// attribute must not become bounded by the conversion).
func millisToDuration(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

// ttmlProperties projects every <amll:meta> pair onto a map, keeping the FIRST
// value of a repeated key. Props keeps document order and duplicates, so the
// first match per key wins. That is a deliberate change from the hand-written
// parser, whose map was overwritten last-wins (spec §6 item 3).
//
// The map is non-nil even when the document has no <head> metadata, matching the
// hand-written parser and the SMI parser.
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

// ttmlAgents names every declared agent with the three-step fallback the
// hand-written parser had: the <ttm:name> child, else the i-th
// amll:meta key="artists" value, else the uppercased id ("v3" -> "V3").
//
// AgentOrder is the declarations' document order; the Agents map's iteration
// order is random and therefore unusable for the artists correspondence.
//
// The result is non-nil even when the document declares no agent, matching the
// hand-written parser.
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
