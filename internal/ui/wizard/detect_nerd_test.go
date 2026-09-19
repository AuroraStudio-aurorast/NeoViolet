package wizard

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/charmbracelet/x/term"
)

func TestMatchNerdFont(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"nerd font family", "JetBrainsMono Nerd Font", true},
		{"nerd font family lowercase", "JetBrainsMono nerd font", true},
		{"concatenated nerd font", "JetBrainsMono NerdFont", true},
		{"family with nerd suffix", "Hack Nerd Font Mono", true},
		{"standalone NF", "CaskaydiaCove NF", true},
		{"bare NF", "NF", true},
		{"multiple spaces between the words", "Hack Nerd  Font", true},
		{"uppercase", "JETBRAINSMONO NERD FONT", true},
		{"plain family", "JetBrains Mono", false},
		{"NF inside word", "CaskaydiaCove NNF", false},
		{"nf inside word lowercase", "information", false},
		{"NF as a word prefix", "NFinity", false},
		{"NF as a word suffix", "CONF", false},
		{"family that only says font", "Some Font", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		if got := MatchNerdFont(tc.in); got != tc.want {
			t.Errorf("MatchNerdFont(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestExtractOSC50Font(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"BEL terminated", []byte("\x1b]50;JetBrainsMono Nerd Font\x07"), "JetBrainsMono Nerd Font"},
		{"ST terminated", []byte("\x1b]50;Monospace\x1b\\"), "Monospace"},
		{"surrounding whitespace trimmed", []byte("\x1b]50; JetBrains Mono \x07"), "JetBrains Mono"},
		{"xterm leading-dash format", []byte("\x1b]50;-*-fixed-medium-r-*-*-18-*\x07"), "fixed"},
		{"xterm format underscores become spaces", []byte("\x1b]50;-*-DejaVu_Sans_Mono-regular-*\x07"), "DejaVu Sans Mono"},
		{"fontconfig style cut at colon", []byte("\x1b]50;JetBrains Mono:style=Regular\x07"), "JetBrains Mono"},
		{"too few dashes keeps the raw name", []byte("\x1b]50;-fixed\x07"), "-fixed"},
		{"no terminator", []byte("\x1b]50;JetBrains Mono"), ""},
		{"trailing bytes after the sequence", []byte("\x1b]50;Monospace\x07leftover"), "Monospace"},
		{"leading bytes before the sequence", []byte("noise\x1b]50;Monospace\x07"), "Monospace"},
		{"first sequence wins when two are present", []byte("\x1b]50;First\x07\x1b]50;Second\x1b\\"), "First"},
		{"no osc50 sequence", []byte("hello world"), ""},
		{"empty response", []byte("\x1b]50;\x07"), ""},
	}
	for _, tc := range cases {
		if got := extractOSC50Font(tc.in); got != tc.want {
			t.Errorf("extractOSC50Font(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Only the matching case is deterministic here: a value without a Nerd Font
// marker does not return early, so detection continues into the fastfetch,
// system font index and OSC 50 probes, whose answers depend on the machine
// running the tests.
func TestDetectNerdFontHonoursTheEnvHint(t *testing.T) {
	t.Setenv("NEOVIOLET_FONT", "JetBrainsMono Nerd Font")
	if !detectNerdFont() {
		t.Error("detectNerdFont() = false with a Nerd Font in NEOVIOLET_FONT")
	}
}

// The OSC 50 query writes an escape sequence to stdout and reads stdin, so it
// is only safe to exercise when stdin is not a terminal.
func TestQueryOSC50FontWithoutATerminal(t *testing.T) {
	if term.IsTerminal(os.Stdin.Fd()) {
		t.Skip("stdin is a terminal; the OSC 50 query would write to stdout")
	}
	if got := queryOSC50Font(); got != "" {
		t.Errorf("queryOSC50Font() = %q, want empty without a terminal", got)
	}
}

// queryFastfetch shells out to fastfetch, so the decode path it depends on is
// pinned here instead: the shape below is what
// `fastfetch --json --structure TerminalFont` emits, and the guards in
// queryFastfetch only make sense if it maps onto these pointers.
func TestFastfetchResultDecodesTheFontName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"full result",
			`[{"type":"TerminalFont","result":{"font":{"name":"JetBrainsMono Nerd Font"}}}]`,
			"JetBrainsMono Nerd Font",
		},
		{"empty font name", `[{"result":{"font":{"name":""}}}]`, ""},
	}
	for _, tc := range cases {
		var results []fastfetchResult
		if err := json.Unmarshal([]byte(tc.in), &results); err != nil {
			t.Fatalf("%s: unmarshal failed: %v", tc.name, err)
		}
		if len(results) != 1 {
			t.Fatalf("%s: decoded %d results, want 1", tc.name, len(results))
		}
		if results[0].Result == nil || results[0].Result.Font == nil {
			t.Fatalf("%s: font pointer chain is nil", tc.name)
		}
		if got := results[0].Result.Font.Name; got != tc.want {
			t.Errorf("%s: name = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The nil and empty shapes the queryFastfetch guards exist to survive.
func TestFastfetchResultHandlesMissingFields(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantLen int
		wantNil bool // Result is nil
		wantNoF bool // Result is set but Font is nil
	}{
		{"empty array", `[]`, 0, false, false},
		{"no result key", `[{}]`, 1, true, false},
		{"null result", `[{"result":null}]`, 1, true, false},
		{"result without font", `[{"result":{}}]`, 1, false, true},
		{"null font", `[{"result":{"font":null}}]`, 1, false, true},
	}
	for _, tc := range cases {
		var results []fastfetchResult
		if err := json.Unmarshal([]byte(tc.in), &results); err != nil {
			t.Fatalf("%s: unmarshal failed: %v", tc.name, err)
		}
		if len(results) != tc.wantLen {
			t.Fatalf("%s: decoded %d results, want %d", tc.name, len(results), tc.wantLen)
		}
		if tc.wantLen == 0 {
			continue
		}
		if tc.wantNil && results[0].Result != nil {
			t.Errorf("%s: Result = %+v, want nil", tc.name, results[0].Result)
		}
		if tc.wantNoF && (results[0].Result == nil || results[0].Result.Font != nil) {
			t.Errorf("%s: Font should be nil", tc.name)
		}
	}

	// Malformed output must be an error, which queryFastfetch discards.
	var results []fastfetchResult
	if err := json.Unmarshal([]byte("not json"), &results); err == nil {
		t.Error("malformed fastfetch output should fail to decode")
	}
}
