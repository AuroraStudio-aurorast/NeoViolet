package lyrics

import (
	"errors"
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
//     musicName/artists/album (musicName and album are deliberately duplicated
//     with DIFFERENT values so the first-wins mapping is discriminating) and the
//     author login;
//   - line 1 is keyed (itunes:key) with word-level spans and real inter-word
//     spaces;
//   - line 2 is keyed and adds an x-bg background span (the x-bg element itself
//     carries begin and end, so upstream keeps its inner word: Background.Words
//     == 1 - the C8 matrix's deciding factor is whether the x-bg element itself
//     has timing, not the inner span) and an inline ttm:role="x-translation"
//     span next to the original text;
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
      <amll:meta key="musicName" value="Second Title"/>
      <amll:meta key="artists" value="Alice"/>
      <amll:meta key="artists" value="Bob"/>
      <amll:meta key="album" value="Sample Album"/>
      <amll:meta key="album" value="Second Album"/>
      <amll:meta key="ttmlAuthorGithubLogin" value="SampleAuthor"/>
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
	return ttmlParseDoc(t, ttmlAMLLSample)
}

// ttmlParseDoc parses any fixture with the keyless policy the adapter's caller
// uses (spec D3: a <p> without itunes:key is still a lyric line).
func ttmlParseDoc(t *testing.T, sample string) *amllttml.Document {
	t.Helper()
	doc, err := amllttml.ParseReader(strings.NewReader(sample),
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
// author login meta (the hand-written parser never set Creator at all).
func TestTTML_AdapterMapsSampleMetadata(t *testing.T) {
	doc := ttmlSampleDoc(t)
	// Premise: the fixture really carries two DIFFERENT musicName and album
	// values. The library dedupes only identical values (metadata.go
	// dedupeStrings), so both survive into Titles/Albums; if the duplicates were
	// identical, first-wins and last-wins would be indistinguishable and the
	// Title/Album pins below would stay green under a last-wins regression.
	if got := doc.Metadata.Titles; !slices.Equal(got, []string{"Sample Song", "Second Title"}) {
		t.Fatalf("lib Titles = %q, want [%q %q] (the premise)", got, "Sample Song", "Second Title")
	}
	if got := doc.Metadata.Albums; !slices.Equal(got, []string{"Sample Album", "Second Album"}) {
		t.Fatalf("lib Albums = %q, want [%q %q] (the premise)", got, "Sample Album", "Second Album")
	}

	data := ttmlToData(doc, "song.ttml")

	if data.Title != "Sample Song" {
		t.Errorf("Title = %q, want %q", data.Title, "Sample Song")
	}
	if data.Artist != "Alice" {
		t.Errorf("Artist = %q, want %q", data.Artist, "Alice")
	}
	if data.Album != "Sample Album" {
		t.Errorf("Album = %q, want %q", data.Album, "Sample Album")
	}
	if data.Creator != "SampleAuthor" {
		t.Errorf("Creator = %q, want %q", data.Creator, "SampleAuthor")
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

// TestTTML_AdapterBackgroundWidensEnd pins the background-vocal half of the End
// derivation. The <p> declares 1s..2s while its background vocal runs to 5s: the
// library folds the x-bg interval into EffectiveInterval, so End is the background
// end. Reading the declared interval alone would report 2s here.
func TestTTML_AdapterBackgroundWidensEnd(t *testing.T) {
	const sample = `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata" xmlns:itunes="http://music.apple.com/lyric-ttml-internal">
  <body>
    <div>
      <p begin="1s" end="2s" itunes:key="L1">
        <span begin="1s" end="2s">main</span>
        <span ttm:role="x-bg" begin="1s" end="5s">ooh</span>
      </p>
    </div>
  </body>
</tt>`

	doc := ttmlParseDoc(t, sample)
	// Premise: the declared interval really stops at 2s, so the 5s asserted below
	// can only come from the background vocal.
	if got := doc.Lines[0].Interval.EndMillis(); got != 2000 {
		t.Fatalf("declared interval end = %dms, want 2000ms (the premise)", got)
	}

	data := ttmlToData(doc, "song.ttml")
	if len(data.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(data.Lines))
	}
	if got, want := data.Lines[0].End, 5*time.Second; got != want {
		t.Errorf("End = %v, want %v (the background vocal's end)", got, want)
	}
	if want := []string{"main", "ooh"}; !slices.Equal(data.Lines[0].Parts, want) {
		t.Errorf("Parts = %q, want %q", data.Lines[0].Parts, want)
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

// TestTTML_AdapterAgentNameBeatsArtistsMeta pins the newly added first step of
// the agent naming: a <ttm:name> child wins over the positional
// amll:meta key="artists" value. The hand-written parser read no <ttm:name> at
// all (its ttmlAgent struct carried only ID and Type), so whenever a file names
// its agents explicitly the displayed name changes with this renovation.
//
// Its own fixture on purpose: ttmlAMLLSample is the contract sample of tasks 3
// and 4, so its agents must keep exercising the artists-meta path.
func TestTTML_AdapterAgentNameBeatsArtistsMeta(t *testing.T) {
	const sample = `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata" xmlns:amll="http://www.example.com/ns/amll">
  <head>
    <metadata>
      <ttm:agent xml:id="v1" type="person"><ttm:name type="person">Real Name</ttm:name></ttm:agent>
      <amll:meta key="artists" value="Other"/>
    </metadata>
  </head>
  <body>
    <div><p begin="00:01.000" end="00:02.000" ttm:agent="v1">Hi</p></div>
  </body>
</tt>`

	doc := ttmlParseDoc(t, sample)
	// Premise: the library reports the explicit name, and the artists meta
	// disagrees with it.
	if got := doc.Agents["v1"].Name(); got != "Real Name" {
		t.Fatalf("lib Agent.Name() = %q, want %q", got, "Real Name")
	}

	data := ttmlToData(doc, "song.ttml")
	if got := data.Agents["v1"]; got != "Real Name" {
		t.Errorf("Agents[v1] = %q, want %q (ttm:name wins over the artists meta)", got, "Real Name")
	}
	// The artists meta is still read - it just does not name this agent.
	if data.Artist != "Other" {
		t.Errorf("Artist = %q, want %q", data.Artist, "Other")
	}
}

// TestTTML_AdapterHeadlessDocumentDefaults pins the values produced when a
// document has no <head> at all: the library reports that absence on
// Document.HeadPresent, and every metadata field must come out empty rather than
// crash. The lines themselves are unaffected.
func TestTTML_AdapterHeadlessDocumentDefaults(t *testing.T) {
	const sample = `<tt xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div><p begin="00:01.000" end="00:02.000">Solo</p></div>
  </body>
</tt>`

	doc := ttmlParseDoc(t, sample)
	// Premise: the source really has no <head>. Document.Metadata is not the
	// marker for that - the published library allocates it unconditionally, so it
	// is never nil here - HeadPresent is.
	if doc.HeadPresent {
		t.Fatal("lib HeadPresent = true, want false for a head-less document (the premise)")
	}

	data := ttmlToData(doc, "song.ttml")
	if data.Title != "" || data.Artist != "" || data.Album != "" || data.Creator != "" {
		t.Errorf("metadata = [%q %q %q %q], want all empty", data.Title, data.Artist, data.Album, data.Creator)
	}
	// Non-nil empty maps, like the hand-written and SMI parsers produce: callers
	// may look up a key without a nil check.
	if data.Properties == nil || len(data.Properties) != 0 {
		t.Errorf("Properties = %v, want non-nil and empty", data.Properties)
	}
	if data.Agents == nil || len(data.Agents) != 0 {
		t.Errorf("Agents = %v, want non-nil and empty", data.Agents)
	}
	if len(data.Lines) != 1 || data.Lines[0].Text != "Solo" {
		t.Fatalf("lines = %v, want the single \"Solo\" line", data.Lines)
	}
}

// TestTTML_AdapterLineWithEmptyOriginalButSegmentsIsKept pins the second half of
// the drop rule. A <p> whose original text is empty but which carries a
// background vocal and a translation still has something to show, so it must
// survive: dropping on the text alone would delete it.
func TestTTML_AdapterLineWithEmptyOriginalButSegmentsIsKept(t *testing.T) {
	const sample = `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata" xmlns:itunes="http://music.apple.com/lyric-ttml-internal">
  <body>
    <div>
      <p begin="00:01.000" end="00:04.000" itunes:key="L1"><span ttm:role="x-bg" begin="00:01.000" end="00:02.000"><span begin="00:01.000" end="00:02.000">ooh</span></span><span ttm:role="x-translation">译文</span></p>
    </div>
  </body>
</tt>`

	doc := ttmlParseDoc(t, sample)
	// Premise: the original segment really is empty here.
	if len(doc.Lines) != 1 || doc.Lines[0].Text != "" {
		t.Fatalf("lib line = %+v, want one line with an empty Text (the premise)", doc.Lines)
	}

	data := ttmlToData(doc, "song.ttml")
	if len(data.Lines) != 1 {
		t.Fatalf("lines = %d, want 1 (the line has display segments even without original text)", len(data.Lines))
	}
	want := []string{"ooh", "译文"}
	if !slices.Equal(data.Lines[0].Parts, want) {
		t.Errorf("Parts = %q, want %q", data.Lines[0].Parts, want)
	}
	if wantText := "ooh | 译文"; data.Lines[0].Text != wantText {
		t.Errorf("Text = %q, want %q", data.Lines[0].Text, wantText)
	}
}

// TestTTML_AdapterLoneDisplaySegmentIsKept covers the other half of the drop rule:
// a <p> whose original text is empty and which carries exactly ONE other display
// segment. A single segment folds back to Parts == nil, so a drop rule that looks
// at Parts cannot see it and would delete the line - and a file made only of such
// lines would report "no lyrics" and silently fall back to another format.
func TestTTML_AdapterLoneDisplaySegmentIsKept(t *testing.T) {
	head := `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata" xmlns:itunes="http://music.apple.com/lyric-ttml-internal"><body>`
	tail := `</body></tt>`
	cases := []struct{ name, body, wantText string }{
		{"translation only", `<p begin="1s" end="2s" itunes:key="L1"><span ttm:role="x-translation" xml:lang="zh">译文</span></p>`, "译文"},
		{"background only", `<p begin="1s" end="2s" itunes:key="L1"><span ttm:role="x-bg" begin="1s" end="2s">ooh</span></p>`, "ooh"},
		{"untimed background only", `<p begin="1s" end="2s" itunes:key="L1"><span ttm:role="x-bg">ooh</span></p>`, "ooh"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := ttmlParseDoc(t, head+c.body+tail)
			// Premise: the original segment really is empty, which is what makes the
			// lone segment the only thing the line has to show.
			if len(doc.Lines) != 1 || doc.Lines[0].Text != "" {
				t.Fatalf("lib line = %+v, want one line with an empty Text (the premise)", doc.Lines)
			}

			data := ttmlToData(doc, "song.ttml")
			if len(data.Lines) != 1 {
				t.Fatalf("lines = %d, want 1 (the lone segment is displayable)", len(data.Lines))
			}
			line := data.Lines[0]
			if line.Text != c.wantText {
				t.Errorf("Text = %q, want %q", line.Text, c.wantText)
			}
			if line.Parts != nil {
				t.Errorf("Parts = %q, want nil for a single segment", line.Parts)
			}
		})
	}

	// Negative control: a <p> with nothing to show is still dropped, so an empty
	// document keeps reporting "no lyrics" instead of gaining a blank line.
	if _, err := parseTTML(head + `<p begin="1s" end="2s" itunes:key="L1">  </p>` + tail); !errors.Is(err, ErrNoLyrics) {
		t.Errorf("Parse() error = %v, want ErrNoLyrics for a <p> with nothing to show", err)
	}
}

// TestTTML_AdapterDuplicateBackgroundSegmentCollapses pins a deliberate spec
// §4.2.1 decision: a segment is appended only when it differs from the ones
// already collected. A chorus echo whose background text equals the original
// therefore collapses into the original segment, and Parts stays nil instead of
// rendering the same text twice.
func TestTTML_AdapterDuplicateBackgroundSegmentCollapses(t *testing.T) {
	const sample = `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata" xmlns:itunes="http://music.apple.com/lyric-ttml-internal">
  <body>
    <div>
      <p begin="00:01.000" end="00:04.000" itunes:key="L1">echo<span ttm:role="x-bg" begin="00:02.000" end="00:03.000"><span begin="00:02.000" end="00:03.000">echo</span></span></p>
    </div>
  </body>
</tt>`

	doc := ttmlParseDoc(t, sample)
	// Premise: the background text duplicates the original one.
	if len(doc.Lines) != 1 || doc.Lines[0].Background == nil {
		t.Fatalf("lib lines = %+v, want one line with a background vocal", doc.Lines)
	}
	if got, want := doc.Lines[0].Background.Text, doc.Lines[0].Text; got != want {
		t.Fatalf("background text = %q, want it to equal the original %q (the premise)", got, want)
	}

	data := ttmlToData(doc, "song.ttml")
	if len(data.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(data.Lines))
	}
	if data.Lines[0].Parts != nil {
		t.Errorf("Parts = %q, want nil (the duplicate segment collapses into the original)", data.Lines[0].Parts)
	}
	if data.Lines[0].Text != "echo" {
		t.Errorf("Text = %q, want %q", data.Lines[0].Text, "echo")
	}
}

// TestTTML_BrIsNonGoal pins that inline <br/> stays a non-goal (spec §14.2):
// the parser drops the element and the derived text glues its neighbours, but
// the line survives and nothing panics. The hand-written parser behaved the
// same way, so this is a behaviour we are intentionally keeping rather than
// fixing.
func TestTTML_BrIsNonGoal(t *testing.T) {
	// Anchor guard: the fixture really carries an inline <br/> between the two
	// neighbours. Without this, swapping <br/> for another unknown element (say
	// <foo/>) keeps both the glue assertion and the unknown-element diagnostic
	// green while silently de-targeting the br contract. The neighbouring letters
	// are part of the anchor so the match cannot coincide with anything else.
	if !strings.Contains(ttmlBrSample, "first<br/>second") {
		t.Fatal(`ttmlBrSample no longer contains "first<br/>second": the br contract is de-targeted (the test would stay green if <br/> were swapped for any other unknown element)`)
	}

	d, err := parseTTML(ttmlBrSample)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(d.Lines))
	}
	if d.Lines[0].Text != "firstsecond" {
		t.Errorf("br Text = %q, want %q (the break is dropped and neighbours glue)", d.Lines[0].Text, "firstsecond")
	}
	// The library flags the dropped element (unknown-element, emitted at parse
	// time so it is visible on both Diags and Diagnostics()); only the code's
	// presence is pinned, not the count, wording or position.
	doc := ttmlParseDoc(t, ttmlBrSample)
	if !hasTTMLDiagnosticCode(doc, amllttml.CodeUnknownElement) {
		t.Errorf("expected an %s diagnostic for the dropped <br/>, got %v",
			amllttml.CodeUnknownElement, doc.Diagnostics())
	}
}
