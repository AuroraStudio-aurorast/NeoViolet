package lyrics

import (
	"strings"
	"testing"
)

// TestTTML_RegressionTable pins the new (go-amll-ttml-parser) readings of the
// six existing fixtures against the old hand-written parser's readings. The old
// readings were measured by running the old parser at commit ace5995 (the last
// commit where "ttml" still registered it), twice: once on today's fixture text
// and once on the pre-rewrite text, so a fixture edit cannot masquerade as a
// parser change.
//
// Every difference from the old parser is annotated "旧值 → 新值" and classified
// as either a fixture change (three fixtures were deliberately rewritten: added
// xmlns:ttm/xmlns:ttp, real inter-word spaces, span end attributes, and the
// non-default frameRate=60 sentinel), a declared parser change, or both.
//
// The bare-text difference (a <p> with no timed <span> now maps to ONE word
// fragment - the whole line text at the line's begin - instead of zero
// fragments) is a deliberate change of this renovation. It follows directly
// from mapping Words straight to the library's Line.Words: the library
// synthesises one word per untimed text run, which is what makes C6 ("Words
// must tile the main text") hold for bare-text lines.

// regWord is one expected word fragment reading, in milliseconds.
type regWord struct {
	timeMs int64
	text   string
}

// regLine is the expected reading of one line: Time/End (ms), Text, and the
// word fragments.
type regLine struct {
	timeMs int64
	endMs  int64
	text   string
	words  []regWord
}

// assertRegLines checks len(d.Lines) and each line's exact readings.
func assertRegLines(t *testing.T, d *Data, want []regLine) {
	t.Helper()
	if len(d.Lines) != len(want) {
		t.Fatalf("len(Lines) = %d, want %d", len(d.Lines), len(want))
	}
	for i, w := range want {
		if i >= len(d.Lines) {
			break
		}
		l := d.Lines[i]
		if got := l.Time.Milliseconds(); got != w.timeMs {
			t.Errorf("line %d Time = %dms, want %dms", i, got, w.timeMs)
		}
		if got := l.End.Milliseconds(); got != w.endMs {
			t.Errorf("line %d End = %dms, want %dms", i, got, w.endMs)
		}
		if l.Text != w.text {
			t.Errorf("line %d Text = %q, want %q", i, l.Text, w.text)
		}
		if len(l.Words) != len(w.words) {
			t.Fatalf("line %d len(Words) = %d, want %d", i, len(l.Words), len(w.words))
		}
		for j, ww := range w.words {
			if got := l.Words[j].Time.Milliseconds(); got != ww.timeMs {
				t.Errorf("line %d Words[%d].Time = %dms, want %dms", i, j, got, ww.timeMs)
			}
			if l.Words[j].Text != ww.text {
				t.Errorf("line %d Words[%d].Text = %q, want %q", i, j, l.Words[j].Text, ww.text)
			}
		}
	}
}

func TestTTML_RegressionTable(t *testing.T) {
	t.Run("testTTMLOffset", func(t *testing.T) {
		// Text unchanged (this fixture was not rewritten). The only difference is
		// len(Words): 0 -> 1 (Words maps straight to the library's Line.Words, so a
		// bare-text line becomes one synthesised word). Time/End identical.
		d, err := parseTTML(testTTMLOffset)
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}
		assertRegLines(t, d, []regLine{
			{1500, 4000, "Offset time first line", []regWord{{1500, "Offset time first line"}}},
			{4000, 7500, "Offset time in milliseconds", []regWord{{4000, "Offset time in milliseconds"}}},
		})
	})

	t.Run("testTTMLMinimal", func(t *testing.T) {
		// Text unchanged. len(Words) 0 -> 1 (the same bare-text synthesis).
		// End stays 0: a begin-only line has no end attribute and no words to
		// widen the interval, so it remains unbounded (the End derivation has
		// nothing to widen from here; the old parser also read End=0).
		d, err := parseTTML(testTTMLMinimal)
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}
		assertRegLines(t, d, []regLine{
			{0, 0, "Minimal TTML", []regWord{{0, "Minimal TTML"}}},
		})
	})

	t.Run("apple", func(t *testing.T) {
		// The shared ttmlAppleStyle fixture, so this row re-runs the source test on
		// exactly the same bytes. Two differences, both expected:
		//   1. Text: "I could find you" -> "I could find you | 我找到了你"
		//      (the inline x-translation is no longer dropped; it becomes
		//      Parts[1]).
		//   2. Word text: "I" -> "I " etc. This is the fixture rewrite (real
		//      inter-word spaces written into the span text) carried through
		//      EndsWithSpace by the library; the word TIMES are identical.
		// Word COUNT is unchanged (4 and 2) - no bare-text synthesis here.
		d, err := parseTTML(ttmlAppleStyle)
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}
		assertRegLines(t, d, []regLine{
			{
				1345, 3071, "I could find you | 我找到了你",
				[]regWord{
					{1345, "I "},
					{1548, "could "},
					{1938, "find "},
					{2198, "you"},
				},
			},
			{
				4085, 6505, "Hello world | 你好世界",
				[]regWord{
					{4085, "Hello "},
					{4510, "world"},
				},
			},
		})
	})

	t.Run("testTTML", func(t *testing.T) {
		// Lines 0-2: Text/Time/End unchanged; len(Words) 0 -> 1 (bare-text
		// synthesis).
		// Line 3: the fixture rewrite (real spaces in span text + span end
		// attributes) meets the no-invented-spaces guarantee. Net readings:
		//   - Text stays "word level sync" (old parser invented the spaces with
		//     needsSpace, the library keeps the fixture's real spaces), so the
		//     TEXT is identical but the cause differs.
		//   - Word text "word" -> "word " (fixture's real trailing space).
		//   - Word count 3 -> 3 (unchanged; the span end attributes keep three
		//     timed words instead of degrading into one synthesised word).
		d, err := parseTTML(testTTML)
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}
		assertRegLines(t, d, []regLine{
			{1500, 4000, "First line of lyrics", []regWord{{1500, "First line of lyrics"}}},
			{4000, 7500, "Second line here", []regWord{{4000, "Second line here"}}},
			{7500, 12000, "Third line goes on", []regWord{{7500, "Third line goes on"}}},
			{
				12000, 15500, "word level sync",
				[]regWord{
					{12000, "word "},
					{13000, "level "},
					{14000, "sync"},
				},
			},
		})
	})

	t.Run("testTTMLAgents", func(t *testing.T) {
		// Same two differences as apple's words, without the translation:
		//   - Text stays "I promise" (the fixture rewrite writes the real space;
		//     the old parser invented it, the library keeps it, so the text is
		//     unchanged for a different reason). No double space: the library
		//     never invents one.
		//   - Word text "I" -> "I " (fixture's real trailing space).
		// Word counts and all Time/End are unchanged.
		d, err := parseTTML(testTTMLAgents)
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}
		assertRegLines(t, d, []regLine{
			{0, 2593, "I promise", []regWord{{0, "I "}, {223, "promise"}}},
			{3490, 5848, "I know", []regWord{{3490, "I "}, {3553, "know"}}},
			{58854, 61239, "I know", []regWord{{58854, "I "}, {59025, "know"}}},
			{166447, 167814, "I'm the", []regWord{{166447, "I'm "}, {166572, "the"}}},
			{170728, 172328, "eeh", []regWord{{170728, "eeh"}}},
		})
	})

	t.Run("testTTMLFrames", func(t *testing.T) {
		// EXPECTED difference: the library does not implement the hh:mm:ss:ff
		// frame clock, so Time is 0 and the End cannot be derived (0), while the
		// old parser read 1500ms/2000ms from the old frameRate=30 text. The
		// rewritten fixture's frameRate=60 sentinel additionally makes the OLD
		// parser fail outright (frameRateMultiplier="1001 1000" is not an int).
		// The line survives with its text; len(Words) is 1 via the bare-text
		// synthesis.
		d, err := parseTTML(testTTMLFrames)
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}
		assertRegLines(t, d, []regLine{
			{0, 0, "Frames-based timestamp", []regWord{{0, "Frames-based timestamp"}}},
		})
	})
}

// ttmlSpaceBareSample is the bare-text half of the whitespace-normalisation
// fixture pair: a <p> with no timed <span>, whose text carries a U+3000
// ideographic space and a run of two ASCII spaces (spelled \u3000 and "  " so the
// escapes stay visible in source). Old parser: strings.TrimSpace(para.Text),
// which trims the ends only, so it read "A\u3000B  C" (at ace5995). New parser:
// "A B C" - the library folds every whitespace run to one half-width space.
const ttmlSpaceBareSample = "<tt xmlns=\"http://www.w3.org/ns/ttml\"><body><div>" +
	"<p begin=\"0s\">A\u3000B  C</p>" +
	"</div></body></tt>"

// ttmlSpaceSpanSample is the timed-span half of the same pair: the whitespace
// sits inside one <span>, so the line also has Words and the normalisation has
// to reach them as well (old parser: "A\u3000B  C" in both Text and Words[0];
// new parser: "A B C" in both).
const ttmlSpaceSpanSample = "<tt xmlns=\"http://www.w3.org/ns/ttml\"><body><div>" +
	"<p begin=\"0s\" end=\"1s\"><span begin=\"0s\" end=\"1s\">A\u3000B  C</span></p>" +
	"</div></body></tt>"

// TestTTML_RegressionTable_TitleSource supplements the regression table above by
// pinning the one metadata difference it does not cover: where Data.Title comes
// from. The hand-written parser only ever set Title from an amll:meta musicName
// property - its ttmlMetadata struct held nothing but Agents and AMLLs - so it
// never read <ttm:title>. The library reads the first non-empty <ttm:title>
// (a new Title source this renovation added) and
// exposes it as a musicName prop, the adapter takes Metadata.Titles[0], and
// testTTML therefore gains a title. Nothing else in the repo asserts this: the
// only <ttm:title> in a fixture is testTTML's (ttml_test.go), and the other title
// assertions (ttml_test.go, ttml_meta_test.go, ttml_adapter_test.go) all read
// data that came from amll:meta musicName.
//
// Old value -> new value per fixture, from the old parser's readings:
//
//	testTTML        Title ""      -> "Test Song"  new: <ttm:title> source
//	testTTMLAgents  Title "ME!"   -> "ME!"        unchanged: amll:meta musicName
//	testTTMLOffset  Title ""      -> ""           no <head>/<metadata> at all
//	testTTMLMinimal Title ""      -> ""           no <head>/<metadata> at all
//	testTTMLFrames  Title ""      -> ""           head has only ttp:* attributes
//	apple           Title ""      -> ""           xmlns:ttm declared, no <ttm:title>
//
// Artist/Album are asserted alongside where the fixture declares them (only
// testTTMLAgents); Creator is "" in both parsers for every fixture here, so it is
// not repeated. Pinning the empty expectations too is what makes this test fail
// if the library ever starts deriving a title from another source.
//
// Mutation evidence for this test is recorded in task-6.5-report.md: blanking the
// adapter's `data.Title = ttmlFirst(md.Titles)` line turns the testTTML case red
// and leaves the other five green.
func TestTTML_RegressionTable_TitleSource(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		title  string
		artist string
		album  string
	}{
		{"testTTML", testTTML, "Test Song", "", ""},
		{"testTTMLAgents", testTTMLAgents, "ME!", "Taylor Swift", "ME! (feat. Brendon Urie)"},
		{"testTTMLOffset", testTTMLOffset, "", "", ""},
		{"testTTMLMinimal", testTTMLMinimal, "", "", ""},
		{"testTTMLFrames", testTTMLFrames, "", "", ""},
		{"apple", ttmlAppleStyle, "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := parseTTML(c.src)
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if d.Title != c.title {
				t.Errorf("Title = %q, want %q", d.Title, c.title)
			}
			if d.Artist != c.artist {
				t.Errorf("Artist = %q, want %q", d.Artist, c.artist)
			}
			if d.Album != c.album {
				t.Errorf("Album = %q, want %q", d.Album, c.album)
			}
		})
	}
}

// TestTTML_RegressionTable_SpaceNormalization supplements the regression table
// with the second metadata-free difference it does not cover: the whitespace
// FORM of Text is normalised by the library, where the
// hand-written parser preserved inner whitespace verbatim (its bare-text branch
// used strings.TrimSpace(para.Text), which strips the ends only). A file written
// with an ideographic space or doubled spaces therefore displays them verbatim
// before this renovation and as a single half-width space after it.
//
// Both mapping paths are pinned because they are separate library code paths
// (bare text node vs. timed span) and Text is normalised on each.
//
// The span case additionally probes C6 - Words tile the display text - on this
// input, because the dangerous failure mode is not "Text keeps the U+3000" but
// "Text is folded while Words keeps \u3000": that would silently misalign word
// highlighting. A fold that reaches Text but not Words fails this test.
//
// This behaviour lives inside the library, so there is no adapter-level mutation
// that can turn it red (unlike the title-source test above); the corpus path is
// the assertion itself.
func TestTTML_RegressionTable_SpaceNormalization(t *testing.T) {
	t.Run("bare-text", func(t *testing.T) {
		assertSpaceFoldFixture(t, ttmlSpaceBareSample)
		d, err := parseTTML(ttmlSpaceBareSample)
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}
		if len(d.Lines) != 1 {
			t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
		}
		if got := d.Lines[0].Text; got != "A B C" {
			t.Errorf("Text = %q, want %q (U+3000 and the doubled space fold to one space)", got, "A B C")
		}
	})

	t.Run("timed-span", func(t *testing.T) {
		assertSpaceFoldFixture(t, ttmlSpaceSpanSample)
		d, err := parseTTML(ttmlSpaceSpanSample)
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}
		if len(d.Lines) != 1 {
			t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
		}
		line := d.Lines[0]
		if got := line.Text; got != "A B C" {
			t.Errorf("Text = %q, want %q (U+3000 and the doubled space fold to one space)", got, "A B C")
		}
		if len(line.Words) != 1 {
			t.Fatalf("len(Words) = %d, want 1 (the single timed span)", len(line.Words))
		}
		if got := line.Words[0].Text; got != "A B C" {
			t.Errorf("Words[0].Text = %q, want %q (the fold must reach Words too)", got, "A B C")
		}
		// C6 on a whitespace-heavy input: the words still tile the display text.
		if !wordsTile(line, false) {
			t.Errorf("C6: Words %q do not tile Text %q", joinWordText(line.Words), line.Text)
		}
	})
}

// assertSpaceFoldFixture guards the discriminating power of the test above: its
// Text == "A B C" assertion holds only because the fixture really carries a
// U+3000 and a doubled ASCII space for the library to fold. Swap the \u3000 for
// a plain space (or the doubled space for a single one) and the pin stays green
// while proving nothing. The anchors keep the neighbouring letters on purpose -
// a bare strings.Contains(src, "  ") would be trivially true on the XML
// attributes and indentation.
func assertSpaceFoldFixture(t *testing.T, src string) {
	t.Helper()
	if !strings.Contains(src, "A\u3000B") {
		t.Fatal("fixture no longer carries U+3000 + doubled space; this pin would silently prove nothing")
	}
	if !strings.Contains(src, "B  C") {
		t.Fatal("fixture no longer carries U+3000 + doubled space; this pin would silently prove nothing")
	}
}
