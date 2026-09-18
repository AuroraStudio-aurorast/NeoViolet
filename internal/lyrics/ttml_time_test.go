package lyrics

import (
	"strings"
	"testing"
	"time"

	amllttml "github.com/WhatDamon/go-amll-ttml-parser"
)

func TestTTML_OffsetTime(t *testing.T) {
	d, err := parseTTML(testTTMLOffset)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Time != 1500*time.Millisecond {
		t.Errorf("1.5s = %v, want 1500ms", d.Lines[0].Time)
	}
	if d.Lines[0].Text != "Offset time first line" {
		t.Errorf("line 0 text = %q", d.Lines[0].Text)
	}

	if d.Lines[1].Time != 4000*time.Millisecond {
		t.Errorf("4000ms = %v, want 4000ms", d.Lines[1].Time)
	}
	if d.Lines[1].Text != "Offset time in milliseconds" {
		t.Errorf("line 1 text = %q", d.Lines[1].Text)
	}
}

func TestTTML_FramesTime(t *testing.T) {
	d, err := parseTTML(testTTMLFrames)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(d.Lines))
	}

	// Frames (hh:mm:ss:ff) are a long-standing non-goal: the pinned library does
	// not implement the frame-count clock, so the line survives but its Time
	// stays 0 (every such line crowds at 0:00) and its text is not lost.
	if d.Lines[0].Time != 0 {
		t.Errorf("frames time = %v, want 0 (frames are unsupported)", d.Lines[0].Time)
	}
	if d.Lines[0].Text != "Frames-based timestamp" {
		t.Errorf("frames line text = %q, want 'Frames-based timestamp' (text must survive)", d.Lines[0].Text)
	}

	// The library flags the frame field instead of failing the parse: an
	// error-severity diagnostic exists in the public Diagnostics() surface (the
	// pinned version never stores bad-time-syntax in doc.Diags). Only existence
	// is pinned - no code, no count, no wording. When we bump to a version that
	// ships the dedicated clock-frames-unsupported code, tighten this to assert
	// that code alongside bad-time-syntax, with Time still 0.
	doc := ttmlParseDoc(t, testTTMLFrames)
	if !doc.Diagnostics().HasErrors() {
		t.Error("expected an error-severity diagnostic for the unsupported frame field")
	}
}

func TestTTML_BareSeconds(t *testing.T) {
	bareSec := `<tt xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="39.345" end="43.071">I could never find the right way</p>
    <p begin="44.085" end="46.505">Have you noticed I've been gone</p>
  </div></body>
</tt>`

	d, err := parseTTML(bareSec)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Time != 39345*time.Millisecond {
		t.Errorf("39.345s = %v, want 39345ms", d.Lines[0].Time)
	}
	if d.Lines[0].Text != "I could never find the right way" {
		t.Errorf("line 0 text = %q", d.Lines[0].Text)
	}

	if d.Lines[1].Time != 44085*time.Millisecond {
		t.Errorf("44.085s = %v, want 44085ms", d.Lines[1].Time)
	}
}

func TestTTML_PartialClockTime(t *testing.T) {
	partial := `<tt xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="1:01.643" end="1:03.071">Trust in me</p>
    <p begin="2:19.153" end="2:23.083">I'll give them shelter</p>
  </div></body>
</tt>`

	d, err := parseTTML(partial)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Time != 61643*time.Millisecond {
		t.Errorf("1:01.643 = %v, want 61643ms", d.Lines[0].Time)
	}
	if d.Lines[0].Text != "Trust in me" {
		t.Errorf("line 0 text = %q", d.Lines[0].Text)
	}

	if d.Lines[1].Time != 139153*time.Millisecond {
		t.Errorf("2:19.153 = %v, want 139153ms", d.Lines[1].Time)
	}
}

func TestTTML_MixedBareSecondsAndPartial(t *testing.T) {
	mixed := `<tt xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="39.345" end="43.071">Bare seconds</p>
    <p begin="1:01.643" end="1:03.071">Partial clock time</p>
  </div></body>
</tt>`

	d, err := parseTTML(mixed)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(d.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(d.Lines))
	}

	if d.Lines[0].Time != 39345*time.Millisecond {
		t.Errorf("39.345 = %v, want 39345ms", d.Lines[0].Time)
	}
	if d.Lines[1].Time != 61643*time.Millisecond {
		t.Errorf("1:01.643 = %v, want 61643ms", d.Lines[1].Time)
	}
}

// TestTTML_FrameRateSentinel proves the ttp:frameRate parameter on the root is
// really read: 50 frames at 60 fps is 833ms, whereas the ignored default of 30
// would give 1667ms.
func TestTTML_FrameRateSentinel(t *testing.T) {
	sample := `<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:ttp="http://www.w3.org/ns/ttml#parameter"
    ttp:frameRate="60">
  <body><div>
    <p begin="50f">Fifty frames</p>
  </div></body>
</tt>`

	d, err := parseTTML(sample)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 1 || d.Lines[0].Time != 833*time.Millisecond {
		t.Errorf("50f @ frameRate=60 = %v, want 833ms (default 30 would give 1667ms)", d.Lines[0].Time)
	}
	assertNoTTMLHygieneDiagnostics(t, sample)
}

// TestTTML_TickRateSentinel proves the ttp:tickRate parameter on the root is
// really read: 50 ticks at 100 ticks/s is 500ms, whereas the ignored default of
// one tick per second would give 50000ms.
func TestTTML_TickRateSentinel(t *testing.T) {
	sample := `<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:ttp="http://www.w3.org/ns/ttml#parameter"
    ttp:tickRate="100">
  <body><div>
    <p begin="50t">Fifty ticks</p>
  </div></body>
</tt>`

	d, err := parseTTML(sample)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(d.Lines) != 1 || d.Lines[0].Time != 500*time.Millisecond {
		t.Errorf("50t @ tickRate=100 = %v, want 500ms (default 1/s would give 50000ms)", d.Lines[0].Time)
	}
	assertNoTTMLHygieneDiagnostics(t, sample)
}

// assertNoTTMLHygieneDiagnostics asserts the fixture declares and places its ttp
// parameters correctly: zero attr-ns-fallback and zero ttp-not-on-root. It
// filters by code on purpose - a headless/keyless document always carries
// missing-head + line-no-key, so a zero-diagnostics assertion would be wrong.
//
// The pinned library defines both codes but never emits them for root ttp
// parameters (ttp-not-on-root has no emission points, and the root-attribute
// reader does not report attr-ns-fallback), so this assertion cannot fail today;
// it is a forward-looking guard that becomes real once we bump to the upstream
// commit that adds the detection (c2a4796).
func assertNoTTMLHygieneDiagnostics(t *testing.T, sample string) {
	t.Helper()
	doc, err := amllttml.ParseReader(strings.NewReader(sample),
		amllttml.WithMissingLineKey(amllttml.MissingKeyKeep))
	if err != nil {
		t.Fatalf("amllttml.ParseReader() error: %v", err)
	}
	for _, diag := range doc.Diagnostics() {
		if diag.Code == amllttml.CodeAttrNSFallback || diag.Code == amllttml.CodeTTPNotOnRoot {
			t.Errorf("unexpected hygiene diagnostic %s: %s", diag.Code, diag.Msg)
		}
	}
}
