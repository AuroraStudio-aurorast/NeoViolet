package lyrics

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	amllttml "github.com/WhatDamon/go-amll-ttml-parser"
)

// The tests in this file pin the TTML edge and error paths the happy-path
// suites do not reach: keyless and empty-key lines, resource-limit errors,
// diagnostics, unbounded and dur-derived intervals, registry fallback, and
// degenerate documents.

func TestTTML_KeylessFileYieldsLines(t *testing.T) {
	const sample = `<tt xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div>
      <p begin="00:01.000" end="00:02.000">keyless line</p>
    </div>
  </body>
</tt>`

	// Library side: the <p> carries no itunes:key, so its Key is "".
	doc := ttmlParseDoc(t, sample)
	if len(doc.Lines) != 1 || doc.Lines[0].Key != "" {
		t.Fatalf("lib lines = %d, key = %q, want one keyless line (the premise)",
			len(doc.Lines), doc.Lines[0].Key)
	}

	// Consumer side (spec D3): a keyless line is still a lyric line, so the
	// adapter keeps it. WithMissingLineKey(MissingKeyKeep) is the only thing
	// that makes this hold; Drop would empty the file.
	d, err := parseTTML(sample)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil (a keyless line is a line)", err)
	}
	if len(d.Lines) != 1 || d.Lines[0].Text != "keyless line" {
		t.Errorf("lines = %v, want the single keyless line", d.Lines)
	}
}

func TestTTML_ExplicitEmptyKeyLineIsKept(t *testing.T) {
	const sample = `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:itunes="http://music.apple.com/lyric-ttml-internal">
  <body>
    <div>
      <p begin="00:01.000" end="00:02.000" itunes:key="">empty-key line</p>
    </div>
  </body>
</tt>`

	doc := ttmlParseDoc(t, sample)
	if len(doc.Lines) != 1 || doc.Lines[0].Key != "" {
		t.Fatalf("lib lines = %d, key = %q, want one empty-key line (the premise)",
			len(doc.Lines), doc.Lines[0].Key)
	}
	// The published library flags the explicit empty key; only the code's
	// presence is pinned (§1.2), not the count, wording or position.
	if !hasTTMLDiagnosticCode(doc, amllttml.CodeEmptyKey) {
		t.Errorf("expected an %s diagnostic for the explicit empty key, got %v",
			amllttml.CodeEmptyKey, doc.Diagnostics())
	}

	d, err := parseTTML(sample)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil (an explicit empty key is kept)", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(d.Lines))
	}
	if d.Lines[0].Time != time.Second {
		t.Errorf("Time = %v, want 1s", d.Lines[0].Time)
	}
	if d.Lines[0].Text != "empty-key line" {
		t.Errorf("Text = %q, want %q", d.Lines[0].Text, "empty-key line")
	}
}

func TestTTML_TooLargeMatchesErrLyricTooLarge(t *testing.T) {
	// This assertion merges two gates: readAllWithLimit (the primary 1 MB
	// ceiling) and the library's WithMaxBytes limit mapped back to the sentinel
	// by ttmlParseError. A single run cannot tell them apart - the primary gate
	// fires first - which is why the mutation evidence in §1.3 relaxes the
	// primary gate to prove the second gate and its mapping are also live.
	_, err := parseTTML(strings.Repeat("a", maxLyricSize+1))
	if !errors.Is(err, ErrLyricTooLarge) {
		t.Errorf("Parse() error = %v, want ErrLyricTooLarge", err)
	}
}

func TestTTML_DiagnosticsDoNotFail(t *testing.T) {
	// The frames fixture carries an Error-severity diagnostic (bad-time-syntax,
	// plus clock-frames-unsupported). The premise assertion below proves it is
	// really error-level, so the nil-error assertion is not vacuous.
	doc := ttmlParseDoc(t, testTTMLFrames)
	if !doc.Diagnostics().HasErrors() {
		t.Fatalf("fixture has no Error-severity diagnostic (the premise)")
	}

	d, err := parseTTML(testTTMLFrames)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil (diagnostics never fail the parse)", err)
	}
	if len(d.Lines) != 1 || d.Lines[0].Text != "Frames-based timestamp" {
		t.Errorf("lines = %v, want the single frames line to survive", d.Lines)
	}
}

func TestTTML_UnboundedLineStaysUnbounded(t *testing.T) {
	const sample = `<tt xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div>
      <p begin="00:01.000" end="00:02.000">bounded</p>
      <p begin="00:03.000">unbounded</p>
    </div>
  </body>
</tt>`

	d, err := parseTTML(sample)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(d.Lines))
	}
	// Premise: the file really mixes a bounded and an unbounded line.
	if d.Lines[0].End != 2*time.Second {
		t.Fatalf("bounded line End = %v, want 2s (the premise)", d.Lines[0].End)
	}
	// The begin-only line has no end and no words: End stays 0, the unbounded
	// sentinel. It must not be turned into a bounded value by the conversion.
	if d.Lines[1].End != 0 {
		t.Errorf("unbounded line End = %v, want 0", d.Lines[1].End)
	}
	if d.Lines[1].Time != 3*time.Second {
		t.Errorf("unbounded line Time = %v, want 3s", d.Lines[1].Time)
	}
	// Per-line rule: the last (unbounded) line never expires, even in a file
	// that also contains a bounded line. The old "global any-bounded" shape made
	// every unbounded line unreachable here.
	active := d.ActiveLines(3500 * time.Millisecond)
	if len(active) != 1 || active[0].Text != "unbounded" {
		t.Errorf("ActiveLines(3.5s) = %v, want the unbounded last line to stay active", active)
	}
}

func TestTTML_DurIsMinOfEndAndDur(t *testing.T) {
	// TTML1 §10.2.3: the computed end is min(dur, end-begin). The library owns
	// this resolution (spec D1); the adapter surfaces it through
	// EffectiveInterval, so the assertions below pin the library's math rather
	// than any Neoviolet dur code (there is none).
	cases := []struct {
		name    string
		xml     string
		wantEnd time.Duration
	}{
		{
			name:    "begin-plus-dur-no-end",
			xml:     `<tt xmlns="http://www.w3.org/ns/ttml"><body><div><p begin="00:01.000" dur="00:02.000">A</p></div></body></tt>`,
			wantEnd: 3 * time.Second,
		},
		{
			name:    "end-less-than-dur",
			xml:     `<tt xmlns="http://www.w3.org/ns/ttml"><body><div><p begin="00:01.000" end="00:02.000" dur="00:05.000">B</p></div></body></tt>`,
			wantEnd: 2 * time.Second,
		},
		{
			name:    "dur-less-than-end",
			xml:     `<tt xmlns="http://www.w3.org/ns/ttml"><body><div><p begin="00:01.000" end="00:05.000" dur="00:02.000">C</p></div></body></tt>`,
			wantEnd: 3 * time.Second,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := parseTTML(c.xml)
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if len(d.Lines) != 1 {
				t.Fatalf("lines = %d, want 1", len(d.Lines))
			}
			if d.Lines[0].Time != time.Second {
				t.Errorf("Time = %v, want 1s", d.Lines[0].Time)
			}
			if d.Lines[0].End != c.wantEnd {
				t.Errorf("End = %v, want %v", d.Lines[0].End, c.wantEnd)
			}
		})
	}
}

func TestTTML_PathAndFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "song.ttml"), []byte(testTTMLMinimal), 0o600); err != nil {
		t.Fatalf("write song.ttml: %v", err)
	}
	audioPath := filepath.Join(dir, "song.mp3")

	data, err := FindAndParse(audioPath, []string{"ttml"})
	if err != nil {
		t.Fatalf("FindAndParse error: %v", err)
	}
	if data == nil {
		t.Fatal("FindAndParse returned nil")
	}
	// Format is written by the registry alone (T1-B1); Path by the parser.
	if data.Format != "ttml" {
		t.Errorf("Format = %q, want %q (the registry writes it)", data.Format, "ttml")
	}
	if want := filepath.Join(dir, "song.ttml"); data.Path != want {
		t.Errorf("Path = %q, want %q (the parser writes it)", data.Path, want)
	}
}

func TestFindAndParsePreferred_EmptyTTMLFallsBackToLRC(t *testing.T) {
	const lrc = "[00:01.00]Hello from LRC\n"
	cases := []struct {
		name string
		ttml string
	}{
		{
			name: "no-paragraphs",
			ttml: `<tt xmlns="http://www.w3.org/ns/ttml"><body><div></div></body></tt>`,
		},
		{
			name: "all-empty-paragraphs",
			ttml: `<tt xmlns="http://www.w3.org/ns/ttml"><body><div><p begin="00:01.000" end="00:02.000"></p><p begin="00:03.000" end="00:04.000"></p></div></body></tt>`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "song.lrc"), []byte(lrc), 0o600); err != nil {
				t.Fatalf("write song.lrc: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "song.ttml"), []byte(c.ttml), 0o600); err != nil {
				t.Fatalf("write song.ttml: %v", err)
			}

			data, err := FindAndParsePreferred(filepath.Join(dir, "song.mp3"), []string{"ttml", "lrc"}, "ttml")
			if err != nil {
				t.Fatalf("FindAndParsePreferred error: %v", err)
			}
			if data == nil {
				t.Fatal("no lyrics found (expected LRC fallback)")
			}
			// The empty TTML must not shadow the real LRC: ErrNoLyrics lets the
			// registry fall through to the next preferred format.
			if data.Format != "lrc" {
				t.Errorf("Format = %q, want %q (empty TTML must fall back to LRC)", data.Format, "lrc")
			}
			if len(data.Lines) != 1 || data.Lines[0].Text != "Hello from LRC" {
				t.Errorf("lines = %v, want the LRC line", data.Lines)
			}
		})
	}
}

func TestTTML_InlineTranslationBeatsHeadTranslation(t *testing.T) {
	// ttmlKeyedDoubleSourceTranslationSample supplies the same keyed line a
	// translation from both an inline x-translation span (行内) and a head-side
	// <text for="L1"> (头部). The library's TranslationsFor returns inline
	// first, so the adapter's tr[0] is the inline one; Parts[1] must be it.
	d, err := parseTTML(ttmlKeyedDoubleSourceTranslationSample)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(d.Lines))
	}
	want := []string{"Hello", "行内"}
	if !slices.Equal(d.Lines[0].Parts, want) {
		t.Errorf("Parts = %q, want %q (inline translation beats the head-side one)", d.Lines[0].Parts, want)
	}
}

func TestTTML_AllEmptyParagraphsIsAnError(t *testing.T) {
	const sample = `<tt xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div>
      <p begin="00:01.000" end="00:02.000"></p>
      <p begin="00:03.000" end="00:04.000"></p>
    </div>
  </body>
</tt>`

	// The library keeps every <p>, but the adapter drops the ones that map to
	// nothing (ttmlLine ok=false). A file whose <p> elements are all empty must
	// therefore report ErrNoLyrics after mapping, or it would shadow a real
	// sidecar sitting next to it.
	_, err := parseTTML(sample)
	if !errors.Is(err, ErrNoLyrics) {
		t.Errorf("Parse() error = %v, want ErrNoLyrics", err)
	}
}

func TestTTML_AdapterZeroValueDocumentDoesNotPanic(t *testing.T) {
	// A hand-built zero-value Document has Metadata == nil. The adapter must
	// not dereference it: Properties and Agents stay non-nil (callers may look
	// up a key without a nil check), metadata fields come out empty, and no
	// lines are produced.
	doc := &amllttml.Document{}
	data := ttmlToData(doc, "song.ttml")
	if data.Path != "song.ttml" {
		t.Errorf("Path = %q, want %q", data.Path, "song.ttml")
	}
	if data.Properties == nil || len(data.Properties) != 0 {
		t.Errorf("Properties = %v, want non-nil and empty", data.Properties)
	}
	if data.Agents == nil || len(data.Agents) != 0 {
		t.Errorf("Agents = %v, want non-nil and empty", data.Agents)
	}
	if data.Title != "" || data.Artist != "" || data.Album != "" || data.Creator != "" {
		t.Errorf("metadata = [%q %q %q %q], want all empty", data.Title, data.Artist, data.Album, data.Creator)
	}
	if len(data.Lines) != 0 {
		t.Errorf("lines = %d, want 0", len(data.Lines))
	}
}

func TestTTML_UntimedBackgroundSpanStillContributesText(t *testing.T) {
	// The x-bg element itself carries no timing; only its inner span has a
	// begin. The background vocal's text must still be preserved as a display
	// part: the library keeps Background.Text in every shape, while
	// Background.Words is its own detail (left unasserted here).
	const sample = `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata">
  <body>
    <div>
      <p begin="00:01.000" end="00:04.000">原文<span ttm:role="x-bg"><span begin="00:02.000">ooh</span></span></p>
    </div>
  </body>
</tt>`

	doc := ttmlParseDoc(t, sample)
	if len(doc.Lines) != 1 || doc.Lines[0].Background == nil {
		t.Fatalf("lib lines = %+v, want one line with a background vocal (the premise)", doc.Lines)
	}

	data := ttmlToData(doc, "song.ttml")
	if len(data.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(data.Lines))
	}
	want := []string{"原文", "ooh"}
	if !slices.Equal(data.Lines[0].Parts, want) {
		t.Errorf("Parts = %q, want %q", data.Lines[0].Parts, want)
	}
	if wantText := "原文 | ooh"; data.Lines[0].Text != wantText {
		t.Errorf("Text = %q, want %q", data.Lines[0].Text, wantText)
	}
	if data.Lines[0].Part(0) != "原文" {
		t.Errorf("Part(0) = %q, want %q", data.Lines[0].Part(0), "原文")
	}
	if got := joinWordText(data.Lines[0].Words); got != data.Lines[0].Part(0) {
		t.Errorf("words tile %q, want Part(0) %q", got, data.Lines[0].Part(0))
	}
}
