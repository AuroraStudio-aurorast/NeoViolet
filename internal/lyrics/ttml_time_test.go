package lyrics

import (
	"testing"
	"time"
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

	expected := 1500 * time.Millisecond
	if d.Lines[0].Time != expected {
		t.Errorf("frames time = %v, want %v", d.Lines[0].Time, expected)
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
