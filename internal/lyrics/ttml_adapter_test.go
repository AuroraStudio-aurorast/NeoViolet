package lyrics

import (
	"slices"
	"strings"
	"testing"
	"time"

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

// ttmlSampleDoc parses ttmlAMLLSample the way the adapter's caller will:
// keyless <p> elements are kept (spec D3), because the hand-written parser
// accepted them too and most non-AMLL TTML has no itunes:key.
func ttmlSampleDoc(t *testing.T) *amllttml.Document {
	t.Helper()
	doc, err := amllttml.ParseReader(strings.NewReader(ttmlAMLLSample),
		amllttml.WithMissingLineKey(amllttml.MissingKeyKeep))
	if err != nil {
		t.Fatalf("amllttml.ParseReader() error: %v", err)
	}
	return doc
}

// joinWordText concatenates the word fragments of a line.
func joinWordText(words []WordFragment) string {
	var b strings.Builder
	for _, w := range words {
		b.WriteString(w.Text)
	}
	return b.String()
}

// TestTTML_AdapterPath pins that the adapter passes the caller's path through.
// Format is not the adapter's business: registry.go is its only writer.
func TestTTML_AdapterPath(t *testing.T) {
	doc, err := amllttml.ParseReader(strings.NewReader(ttmlAMLLSample))
	if err != nil {
		t.Fatalf("amllttml.ParseReader() error: %v", err)
	}

	data := ttmlToData(doc, "song.ttml")
	if data.Path != "song.ttml" {
		t.Errorf("Path = %q, want %q", data.Path, "song.ttml")
	}
}

// TestTTML_AdapterMapsSample pins the line projection of ttmlAMLLSample: one
// line per <p> (keyless included), Parts in [original, background, translation]
// order, Text joined with " | ", words from the original segment only, and the
// library's derived times.
func TestTTML_AdapterMapsSample(t *testing.T) {
	data := ttmlToData(ttmlSampleDoc(t), "song.ttml")

	if len(data.Lines) != 3 {
		t.Fatalf("lines = %d, want 3 (the keyless <p> is kept)", len(data.Lines))
	}

	// Line 1 is keyed and plain: no x-bg (so Line.Background is nil - reading
	// .Text without a nil check would panic here) and no translation, hence no
	// second segment and Parts stays nil.
	first := data.Lines[0]
	if first.Parts != nil {
		t.Errorf("line 1 Parts = %q, want nil", first.Parts)
	}
	if first.Text != "I could find you" {
		t.Errorf("line 1 Text = %q, want %q", first.Text, "I could find you")
	}
	if len(first.Words) != 4 {
		t.Fatalf("line 1 words = %d, want 4", len(first.Words))
	}
	if got := joinWordText(first.Words); got != first.Text {
		t.Errorf("line 1 words join = %q, want the line text %q", got, first.Text)
	}
	// WordFragment.Time is the word's own begin, absolute in the document.
	if first.Words[0].Time != time.Second {
		t.Errorf("line 1 word 1 Time = %v, want 1s", first.Words[0].Time)
	}
	if want := 3*time.Second + 250*time.Millisecond; first.Words[3].Time != want {
		t.Errorf("line 1 word 4 Time = %v, want %v", first.Words[3].Time, want)
	}
	if first.Time != time.Second || first.End != 4*time.Second {
		t.Errorf("line 1 window = [%v,%v], want [1s,4s]", first.Time, first.End)
	}
	if first.Agent != "v1" {
		t.Errorf("line 1 Agent = %q, want %q", first.Agent, "v1")
	}

	// Line 2 carries all three segments; the order is fixed by spec D6.
	second := data.Lines[1]
	wantParts := []string{"Ooh, I found you", "ooh", "我找到了你"}
	if !slices.Equal(second.Parts, wantParts) {
		t.Errorf("line 2 Parts = %q, want %q", second.Parts, wantParts)
	}
	if want := strings.Join(wantParts, " | "); second.Text != want {
		t.Errorf("line 2 Text = %q, want %q", second.Text, want)
	}
	if len(second.Words) != 4 {
		t.Fatalf("line 2 words = %d, want 4 (original segment only)", len(second.Words))
	}
	// The words tile Parts[0]: the background vocal "ooh" and the translation
	// are display text without word timing.
	//
	// This fixture only contains timed spans, so the tiling is strict here. Real
	// corpus files are not always that tidy: 5 of 717085 lines expose the words
	// as a PREFIX of the display text (a bare text node next to a span, upstream
	// INV-9, spec §5). Task 4's contract assertion is consequently written in the
	// looser prefix form for the registered parser; the strict form here is what
	// the sample is built to satisfy, not a contradiction.
	if got := joinWordText(second.Words); got != second.Parts[0] {
		t.Errorf("line 2 words join = %q, want Parts[0] %q", got, second.Parts[0])
	}
	for _, w := range second.Words {
		if strings.Contains(w.Text, "ooh") || strings.Contains(w.Text, "我找到了你") {
			t.Errorf("line 2 word %q comes from a non-original segment", w.Text)
		}
	}
	if second.Time != 4*time.Second || second.End != 7*time.Second {
		t.Errorf("line 2 window = [%v,%v], want [4s,7s]", second.Time, second.End)
	}

	// Line 3 has no itunes:key; the adapter must not lose it.
	third := data.Lines[2]
	if third.Parts != nil {
		t.Errorf("line 3 Parts = %q, want nil", third.Parts)
	}
	if third.Agent != "v2" {
		t.Errorf("line 3 Agent = %q, want %q", third.Agent, "v2")
	}
	if third.Time != 7*time.Second || third.End != 10*time.Second {
		t.Errorf("line 3 window = [%v,%v], want [7s,10s]", third.Time, third.End)
	}
}

// TestTTML_AdapterMapsSampleMetadata pins the metadata projection of
// ttmlAMLLSample, including the two deliberate behaviour changes: Properties
// keeps the FIRST value of a repeated key, and Creator is wired to the AMLL
// author meta.
func TestTTML_AdapterMapsSampleMetadata(t *testing.T) {
	data := ttmlToData(ttmlSampleDoc(t), "song.ttml")

	if data.Title != "Sample Song" {
		t.Errorf("Title = %q, want %q", data.Title, "Sample Song")
	}
	if data.Artist != "Alice" {
		t.Errorf("Artist = %q, want %q", data.Artist, "Alice")
	}
	if data.Album != "Sample Album" {
		t.Errorf("Album = %q, want %q", data.Album, "Sample Album")
	}
	// The fixture declares no ttmlAuthorGithubLogin meta, so Creator stays empty
	// here; the field is wired, this sample just cannot exercise it.
	if data.Creator != "" {
		t.Errorf("Creator = %q, want empty", data.Creator)
	}

	if got := data.Properties["musicName"]; got != "Sample Song" {
		t.Errorf("Properties[musicName] = %q, want %q", got, "Sample Song")
	}
	if got := data.Properties["album"]; got != "Sample Album" {
		t.Errorf("Properties[album] = %q, want %q", got, "Sample Album")
	}
	// "artists" appears twice (Alice, Bob): the first value wins, where the
	// hand-written parser's map kept the last one ("Bob", spec §6 item 3).
	if got := data.Properties["artists"]; got != "Alice" {
		t.Errorf("Properties[artists] = %q, want %q (first value wins)", got, "Alice")
	}

	// Neither agent declares a ttm:name, so the names come from the artists meta
	// in agent document order.
	if got := data.Agents["v1"]; got != "Alice" {
		t.Errorf("Agents[v1] = %q, want %q", got, "Alice")
	}
	if got := data.Agents["v2"]; got != "Bob" {
		t.Errorf("Agents[v2] = %q, want %q", got, "Bob")
	}
}

// TestTTML_AdapterEndFromEffectiveInterval pins the End sentinel. Both <p>
// elements declare a begin and no end:
//
//   - the first has a word without its own end, so nothing bounds the line and
//     End stays 0 - the unbounded sentinel ActiveLines' per-line rule relies on;
//   - the second has a word that ends, so the library's EffectiveInterval widens
//     the declared (endless) interval to the word's end (spec §6 item 5).
func TestTTML_AdapterEndFromEffectiveInterval(t *testing.T) {
	const sample = `<tt xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div>
      <p begin="00:01.000"><span begin="00:01.000">unbounded</span></p>
      <p begin="00:03.000"><span begin="00:03.000" end="00:04.500">bounded</span></p>
    </div>
  </body>
</tt>`

	doc, err := amllttml.ParseReader(strings.NewReader(sample),
		amllttml.WithMissingLineKey(amllttml.MissingKeyKeep))
	if err != nil {
		t.Fatalf("amllttml.ParseReader() error: %v", err)
	}
	data := ttmlToData(doc, "song.ttml")
	if len(data.Lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(data.Lines))
	}

	unbounded := data.Lines[0]
	if unbounded.Time != time.Second {
		t.Errorf("unbounded Time = %v, want 1s", unbounded.Time)
	}
	if unbounded.End != 0 {
		t.Errorf("unbounded End = %v, want 0 (the sentinel must survive the conversion)", unbounded.End)
	}

	bounded := data.Lines[1]
	if bounded.Time != 3*time.Second {
		t.Errorf("bounded Time = %v, want 3s", bounded.Time)
	}
	if want := 4*time.Second + 500*time.Millisecond; bounded.End != want {
		t.Errorf("bounded End = %v, want the word's end %v", bounded.End, want)
	}
}

// TestTTML_AdapterSortsByTime pins that the adapter re-establishes time order:
// the library keeps the document order of <p> elements (10s, 1s, 5s below), but
// the cross-format contract requires ascending Time and ActiveLines' per-line
// scan of unbounded lines assumes it.
func TestTTML_AdapterSortsByTime(t *testing.T) {
	const sample = `<tt xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div>
      <p begin="00:00:10.000">Later</p>
      <p begin="00:00:01.000">Earlier</p>
      <p begin="00:00:05.000">Middle</p>
    </div>
  </body>
</tt>`

	doc, err := amllttml.ParseReader(strings.NewReader(sample),
		amllttml.WithMissingLineKey(amllttml.MissingKeyKeep))
	if err != nil {
		t.Fatalf("amllttml.ParseReader() error: %v", err)
	}
	data := ttmlToData(doc, "song.ttml")
	if len(data.Lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(data.Lines))
	}
	for i := 1; i < len(data.Lines); i++ {
		if data.Lines[i].Time < data.Lines[i-1].Time {
			t.Errorf("lines not sorted at index %d: %v after %v",
				i, data.Lines[i].Time, data.Lines[i-1].Time)
		}
	}
}

// TestTTML_AdapterEmptyParagraphIsDropped pins the drop rule: a <p> with no
// text and no other display segment must not become a blank row.
func TestTTML_AdapterEmptyParagraphIsDropped(t *testing.T) {
	const sample = `<tt xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div>
      <p begin="00:01.000" end="00:02.000"></p>
      <p begin="00:03.000" end="00:04.000"><span begin="00:03.000" end="00:04.000">kept</span></p>
    </div>
  </body>
</tt>`

	doc, err := amllttml.ParseReader(strings.NewReader(sample),
		amllttml.WithMissingLineKey(amllttml.MissingKeyKeep))
	if err != nil {
		t.Fatalf("amllttml.ParseReader() error: %v", err)
	}
	// The library keeps the empty <p> as a line with an empty Text; dropping it is
	// the adapter's rule, so this count is what proves the rule fired.
	if len(doc.Lines) != 2 {
		t.Fatalf("parsed lines = %d, want 2", len(doc.Lines))
	}
	data := ttmlToData(doc, "song.ttml")
	if len(data.Lines) != 1 {
		t.Fatalf("lines = %d, want 1 (the empty <p> is dropped)", len(data.Lines))
	}
	if data.Lines[0].Text != "kept" {
		t.Errorf("Text = %q, want %q", data.Lines[0].Text, "kept")
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
