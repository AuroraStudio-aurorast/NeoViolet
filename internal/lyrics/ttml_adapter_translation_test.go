package lyrics

import (
	"slices"
	"testing"
)

// The fixtures below isolate the translation seam of keyless lines. The shape is
// the one real files use: an inline <span ttm:role="x-translation"> next to the
// original text, and/or a head-side
// <iTunesMetadata><translations><translation><text for="key"> block.

const ttmlTranslationHead = `xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata" xmlns:itunes="http://music.apple.com/lyric-ttml-internal"`

// ttmlKeylessInlineTranslationSample is a bilingual line without itunes:key.
const ttmlKeylessInlineTranslationSample = `<tt ` + ttmlTranslationHead + ` xml:lang="en">
  <body>
    <div>
      <p begin="00:01.000" end="00:04.000">Hello<span ttm:role="x-translation">你好</span></p>
    </div>
  </body>
</tt>`

// ttmlKeylessHeadTranslationSample has two keyless lines and a head-side
// <text> WITHOUT a for attribute. Such an entry is not addressable by key - the
// library's own aux lookup skips it for every key that is not "".
const ttmlKeylessHeadTranslationSample = `<tt ` + ttmlTranslationHead + ` xml:lang="en">
  <head>
    <metadata>
      <iTunesMetadata>
        <translations>
          <translation xml:lang="zh-CN" type="subtitle"><text>GLOBAL</text></translation>
        </translations>
      </iTunesMetadata>
    </metadata>
  </head>
  <body>
    <div>
      <p begin="00:01.000" end="00:02.000">Hello</p>
      <p begin="00:03.000" end="00:04.000">World</p>
    </div>
  </body>
</tt>`

// ttmlKeyedHeadTranslationSample is the same file with keys on both sides.
const ttmlKeyedHeadTranslationSample = `<tt ` + ttmlTranslationHead + ` xml:lang="en">
  <head>
    <metadata>
      <iTunesMetadata>
        <translations>
          <translation xml:lang="zh-CN" type="subtitle"><text for="L1">你好</text></translation>
        </translations>
      </iTunesMetadata>
    </metadata>
  </head>
  <body>
    <div>
      <p begin="00:01.000" end="00:04.000" itunes:key="L1">Hello</p>
    </div>
  </body>
</tt>`

// ttmlKeyedInlineTranslationSample is a bilingual keyed line with no head block.
const ttmlKeyedInlineTranslationSample = `<tt ` + ttmlTranslationHead + ` xml:lang="en">
  <body>
    <div>
      <p begin="00:01.000" end="00:04.000" itunes:key="L1">Hello<span ttm:role="x-translation">行内</span></p>
    </div>
  </body>
</tt>`

// ttmlKeyedDoubleSourceTranslationSample supplies a translation for the same line
// from both an inline span and the head block, with different texts so the two
// sources are distinguishable in the assertion. Upstream flags this shape with
// W-DOUBLE-SOURCE-AUX rather than forbidding it, and the reference tooling
// appends both.
const ttmlKeyedDoubleSourceTranslationSample = `<tt ` + ttmlTranslationHead + ` xml:lang="en">
  <head>
    <metadata>
      <iTunesMetadata>
        <translations>
          <translation xml:lang="zh-CN" type="subtitle"><text for="L1">头部</text></translation>
        </translations>
      </iTunesMetadata>
    </metadata>
  </head>
  <body>
    <div>
      <p begin="00:01.000" end="00:04.000" itunes:key="L1">Hello<span ttm:role="x-translation">行内</span></p>
    </div>
  </body>
</tt>`

// ttmlKeylessBackgroundTranslationSample translates a keyless line from a span
// nested inside its x-bg background vocal: the line's own translation list is
// empty there, the background's is not.
const ttmlKeylessBackgroundTranslationSample = `<tt ` + ttmlTranslationHead + ` xml:lang="en">
  <body>
    <div>
      <p begin="00:01.000" end="00:04.000">Hello<span ttm:role="x-bg" begin="00:02.000" end="00:03.000"><span begin="00:02.000" end="00:03.000">ooh</span><span ttm:role="x-translation">呜</span></span></p>
    </div>
  </body>
</tt>`

// ttmlKeylessDoubleInlineTranslationSample carries a translation both on the
// keyless line itself and on its background vocal's track.
const ttmlKeylessDoubleInlineTranslationSample = `<tt ` + ttmlTranslationHead + ` xml:lang="en">
  <body>
    <div>
      <p begin="00:01.000" end="00:04.000">Hello<span ttm:role="x-bg" begin="00:02.000" end="00:03.000"><span begin="00:02.000" end="00:03.000">ooh</span><span ttm:role="x-translation">呜</span></span><span ttm:role="x-translation">你好</span></p>
    </div>
  </body>
</tt>`

// TestTTML_AdapterKeylessInlineTranslationBecomesPart pins that a keyless line
// keeps its OWN inline translation.
//
// Why this needs its own test: the library indexes lines by key and skips empty
// keys, so Document.Line("") never resolves and TranslationsFor("") cannot reach
// the line's inline track. Reading the line directly is the only way to keep it.
func TestTTML_AdapterKeylessInlineTranslationBecomesPart(t *testing.T) {
	doc := ttmlParseDoc(t, ttmlKeylessInlineTranslationSample)
	// Premise: the translation IS on the line, it is just unreachable by key.
	if len(doc.Lines) != 1 || doc.Lines[0].Key != "" {
		t.Fatalf("expected one keyless line, got %d lines (key %q)", len(doc.Lines), doc.Lines[0].Key)
	}
	if got := len(doc.Lines[0].Translations); got != 1 {
		t.Fatalf("line inline translations = %d, want 1", got)
	}
	if got := len(doc.TranslationsFor("")); got != 0 {
		t.Fatalf("TranslationsFor(\"\") = %d entries, want 0 (the premise of this test)", got)
	}

	data := ttmlToData(doc, "song.ttml")
	if len(data.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(data.Lines))
	}
	want := []string{"Hello", "你好"}
	if !slices.Equal(data.Lines[0].Parts, want) {
		t.Errorf("Parts = %q, want %q (the keyless line's inline translation is dropped)", data.Lines[0].Parts, want)
	}
	if wantText := "Hello | 你好"; data.Lines[0].Text != wantText {
		t.Errorf("Text = %q, want %q", data.Lines[0].Text, wantText)
	}
}

// TestTTML_AdapterKeylessLineIgnoresHeadTranslationWithoutFor pins that a
// head-side <text> without a for attribute does not become the translation of
// every keyless line.
//
// The library filters head entries with it.For != key, and a missing for is "",
// which matches the "" key of a keyless line - so TranslationsFor("") hands the
// same entry to every one of them. Ignoring it keeps keyless files consistent
// with keyed ones, where such an entry is unreachable by any key. A head block
// that does not say which line it translates therefore displays nothing: an
// accepted limitation, not an oversight.
func TestTTML_AdapterKeylessLineIgnoresHeadTranslationWithoutFor(t *testing.T) {
	doc := ttmlParseDoc(t, ttmlKeylessHeadTranslationSample)
	if len(doc.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(doc.Lines))
	}
	// Premise: the library does offer that entry for the "" key - once per line.
	tr := doc.TranslationsFor("")
	if len(tr) != 1 || tr[0].For != "" || tr[0].Text != "GLOBAL" {
		t.Fatalf("TranslationsFor(\"\") = %+v, want the single for-less GLOBAL entry", tr)
	}

	data := ttmlToData(doc, "song.ttml")
	if len(data.Lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(data.Lines))
	}
	for i, line := range data.Lines {
		if line.Parts != nil {
			t.Errorf("line %d Parts = %q, want nil (a for-less head <text> translates no line)", i+1, line.Parts)
		}
		if line.Text != "Hello" && line.Text != "World" {
			t.Errorf("line %d Text = %q, want the original text only", i+1, line.Text)
		}
	}
}

// TestTTML_AdapterKeylessBackgroundTranslationBecomesPart pins the second of the
// two inline sources the keyless path reads: a translation living on the
// background vocal's track. The library's own lookup consults the line and then
// its background (auxFor), so the keyless path must too, or a file that
// translates only the echo loses that text.
func TestTTML_AdapterKeylessBackgroundTranslationBecomesPart(t *testing.T) {
	doc := ttmlParseDoc(t, ttmlKeylessBackgroundTranslationSample)
	// Premise: the translation sits on the background track, not on the line.
	if len(doc.Lines) != 1 || doc.Lines[0].Background == nil {
		t.Fatalf("expected one line with a background vocal, got %+v", doc.Lines)
	}
	if got := len(doc.Lines[0].Translations); got != 0 {
		t.Fatalf("line inline translations = %d, want 0 (the premise)", got)
	}
	if got := len(doc.Lines[0].Background.Translations); got != 1 {
		t.Fatalf("background translations = %d, want 1 (the premise)", got)
	}

	data := ttmlToData(doc, "song.ttml")
	if len(data.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(data.Lines))
	}
	want := []string{"Hello", "ooh", "呜"}
	if !slices.Equal(data.Lines[0].Parts, want) {
		t.Errorf("Parts = %q, want %q (the background translation is dropped)", data.Lines[0].Parts, want)
	}
}

// TestTTML_AdapterKeylessLineTranslationBeatsBackgroundTranslation pins the
// order of the two inline sources for a keyless line: the line's own translation
// first, its background vocal's only as the fallback - the same order the
// library's auxFor uses for keyed lines.
func TestTTML_AdapterKeylessLineTranslationBeatsBackgroundTranslation(t *testing.T) {
	doc := ttmlParseDoc(t, ttmlKeylessDoubleInlineTranslationSample)
	// Premise: both tracks carry a translation.
	if len(doc.Lines) != 1 || doc.Lines[0].Background == nil {
		t.Fatalf("expected one line with a background vocal, got %+v", doc.Lines)
	}
	if len(doc.Lines[0].Translations) != 1 || len(doc.Lines[0].Background.Translations) != 1 {
		t.Fatalf("translations: line=%d background=%d, want 1 and 1 (the premise)",
			len(doc.Lines[0].Translations), len(doc.Lines[0].Background.Translations))
	}

	data := ttmlToData(doc, "song.ttml")
	if len(data.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(data.Lines))
	}
	want := []string{"Hello", "ooh", "你好"}
	if !slices.Equal(data.Lines[0].Parts, want) {
		t.Errorf("Parts = %q, want %q (the line's own translation is the one shown)", data.Lines[0].Parts, want)
	}
}

// TestTTML_AdapterKeyedHeadTranslationBecomesPart pins the keyed head-side path:
// <text for="key"> is reachable through the library's keyed lookup.
func TestTTML_AdapterKeyedHeadTranslationBecomesPart(t *testing.T) {
	data := ttmlToData(ttmlParseDoc(t, ttmlKeyedHeadTranslationSample), "song.ttml")

	if len(data.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(data.Lines))
	}
	want := []string{"Hello", "你好"}
	if !slices.Equal(data.Lines[0].Parts, want) {
		t.Errorf("Parts = %q, want %q", data.Lines[0].Parts, want)
	}
	if wantText := "Hello | 你好"; data.Lines[0].Text != wantText {
		t.Errorf("Text = %q, want %q", data.Lines[0].Text, wantText)
	}
}

// TestTTML_AdapterKeyedInlineTranslationBecomesPart pins the keyed inline path,
// which the shared sample also covers through its second line.
func TestTTML_AdapterKeyedInlineTranslationBecomesPart(t *testing.T) {
	data := ttmlToData(ttmlParseDoc(t, ttmlKeyedInlineTranslationSample), "song.ttml")

	if len(data.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(data.Lines))
	}
	want := []string{"Hello", "行内"}
	if !slices.Equal(data.Lines[0].Parts, want) {
		t.Errorf("Parts = %q, want %q", data.Lines[0].Parts, want)
	}
}

// TestTTML_AdapterKeyedInlineTranslationWinsOverHead pins the priority rule:
// when a line is translated by both an inline span and the head block,
// the inline one is Parts[1]. The library returns inline first, so index 0 is
// the inline entry; nothing else in the suite would notice that order flipping.
func TestTTML_AdapterKeyedInlineTranslationWinsOverHead(t *testing.T) {
	doc := ttmlParseDoc(t, ttmlKeyedDoubleSourceTranslationSample)
	// Premise: both sources are present (the library deliberately keeps both).
	if got := len(doc.TranslationsFor("L1")); got != 2 {
		t.Fatalf("TranslationsFor(L1) = %d entries, want 2", got)
	}

	data := ttmlToData(doc, "song.ttml")
	if len(data.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(data.Lines))
	}
	want := []string{"Hello", "行内"}
	if !slices.Equal(data.Lines[0].Parts, want) {
		t.Errorf("Parts = %q, want %q (inline translation first)", data.Lines[0].Parts, want)
	}
	for _, p := range data.Lines[0].Parts {
		if p == "头部" {
			t.Errorf("Parts = %q contains the head-side translation, want the inline one", data.Lines[0].Parts)
		}
	}
}
