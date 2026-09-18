package lyrics

import (
	"strings"
	"testing"

	amllttml "github.com/WhatDamon/go-amll-ttml-parser"
)

// ttmlAMLLSample is the AMLL-flavoured TTML fixture of the TTML renovation.
// It is shaped like a real AMLL file on purpose:
//
//   - the root declares every namespace it uses (no undeclared prefixes);
//   - <head> carries two ttm:agent declarations plus amll:meta
//     musicName/artists/album;
//   - line 1 is keyed (itunes:key) with word-level spans and real inter-word
//     spaces;
//   - line 2 is keyed and adds an x-bg background span (inner span carries both
//     begin and end: a begin-only x-bg yields zero words upstream) and an
//     inline ttm:role="x-translation" span next to the original text;
//   - line 3 has no itunes:key at all, so the sample also covers the keyless
//     path (upstream MissingKeyKeep).
//
// Inline <br/> is deliberately absent: a non-goal this round. See ttmlBrSample.
//
// Task 4 reuses this constant as the contract sample, so it must stay the only
// definition.
const ttmlAMLLSample = `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata" xmlns:itunes="http://music.apple.com/lyric-ttml-internal" xmlns:amll="http://www.example.com/ns/amll" xml:lang="en">
  <head>
    <metadata>
      <ttm:agent xml:id="v1" type="person"/>
      <ttm:agent xml:id="v2" type="person"/>
      <amll:meta key="musicName" value="Sample Song"/>
      <amll:meta key="artists" value="Alice"/>
      <amll:meta key="artists" value="Bob"/>
      <amll:meta key="album" value="Sample Album"/>
    </metadata>
  </head>
  <body>
    <div>
      <p begin="00:01.000" end="00:04.000" itunes:key="L1" ttm:agent="v1"><span begin="00:01.000" end="00:01.500">I</span> <span begin="00:01.500" end="00:02.500">could</span> <span begin="00:02.500" end="00:03.250">find</span> <span begin="00:03.250" end="00:04.000">you</span></p>
      <p begin="00:04.000" end="00:07.000" itunes:key="L2" ttm:agent="v1"><span begin="00:04.000" end="00:04.500">Ooh,</span> <span begin="00:04.500" end="00:05.250">I</span> <span begin="00:05.250" end="00:06.000">found</span> <span begin="00:06.000" end="00:07.000">you</span><span ttm:role="x-bg" begin="00:06.250" end="00:06.750"><span begin="00:06.250" end="00:06.750">ooh</span></span><span ttm:role="x-translation">我找到了你</span></p>
      <p begin="00:07.000" end="00:10.000" ttm:agent="v2"><span begin="00:07.000" end="00:08.000">no</span> <span begin="00:08.000" end="00:09.000">key</span> <span begin="00:09.000" end="00:10.000">here</span></p>
    </div>
  </body>
</tt>`

// ttmlBrSample probes inline <br/>, which spec §14.2 ruled a non-goal this
// round (no AMLL corpus file uses it). The reading is still pinned: the parser
// drops the element and concatenates its neighbours.
const ttmlBrSample = `<tt xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div>
      <p begin="00:01.000" end="00:03.000">first<br/>second</p>
    </div>
  </body>
</tt>`

// TestTTML_AdapterSketch pins what task 1 puts in ttmlToData: the mapping layer
// exists, Path comes from the caller, and Format matches the name the TTML
// parser is registered under. Task 2 fills in the rest.
func TestTTML_AdapterSketch(t *testing.T) {
	doc, err := amllttml.ParseReader(strings.NewReader(ttmlAMLLSample))
	if err != nil {
		t.Fatalf("amllttml.ParseReader() error: %v", err)
	}

	data := ttmlToData(doc, "song.ttml")
	if data.Path != "song.ttml" {
		t.Errorf("Path = %q, want %q", data.Path, "song.ttml")
	}
	if data.Format != ttmlFormat {
		t.Errorf("Format = %q, want %q", data.Format, ttmlFormat)
	}
}

// TestTTML_OldParserBaseline records what the hand-written parser in ttml.go
// makes of ttmlAMLLSample. These readings are the red evidence for tasks 3 and
// 4: the sample is AMLL-shaped, and the hand-written parser has no Parts, drops
// the x-translation span, loses the nested x-bg text behind an empty word, and
// cannot tile Text with Words (C6).
//
// TEMPORARY. Task 3 replaces the hand-written parser with
// go-amll-ttml-parser, so these readings change by design - rewrite or delete
// this test then; it is a record, not a regression guard.
func TestTTML_OldParserBaseline(t *testing.T) {
	joinWords := func(words []WordFragment) string {
		var b strings.Builder
		for _, w := range words {
			b.WriteString(w.Text)
		}
		return b.String()
	}

	d, err := parseTTML(ttmlAMLLSample)
	if err != nil {
		t.Fatalf("old parseTTML() error: %v", err)
	}
	if len(d.Lines) != 3 {
		t.Fatalf("old parser lines = %d, want 3 (the keyless <p> is parsed too)", len(d.Lines))
	}

	t.Logf("old meta: Title=%q Artist=%q Album=%q Creator=%q", d.Title, d.Artist, d.Album, d.Creator)
	t.Logf("old meta: Properties=%v Agents=%v", d.Properties, d.Agents)
	for i, l := range d.Lines {
		t.Logf("old line %d: Time=%v End=%v Agent=%q Parts=%v Text=%q",
			i, l.Time, l.End, l.Agent, l.Parts, l.Text)
		for j, w := range l.Words {
			t.Logf("old line %d word %d: Time=%v Text=%q", i, j, w.Time, w.Text)
		}
		t.Logf("old line %d: wordsJoin=%q Text=%q C6WordsTileText=%v",
			i, joinWords(l.Words), l.Text, joinWords(l.Words) == l.Text)
	}

	br, err := parseTTML(ttmlBrSample)
	if err != nil {
		t.Fatalf("old parseTTML(br sample) error: %v", err)
	}
	if len(br.Lines) != 1 {
		t.Fatalf("old parser br lines = %d, want 1", len(br.Lines))
	}
	t.Logf("old br probe: Text=%q words=%d", br.Lines[0].Text, len(br.Lines[0].Words))
	// The break is gone and the two halves are glued: <br/> is not modelled.
	if br.Lines[0].Text != "firstsecond" {
		t.Errorf("old br Text = %q, want %q", br.Lines[0].Text, "firstsecond")
	}
}
