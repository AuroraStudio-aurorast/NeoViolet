package lyrics

import "testing"

// contractCases holds one representative sample per registered parser. Adding a
// format means adding an entry: TestFormatContract asserts that every name in
// AvailableParsers() appears here.
//
// embedded's sample drives parseSYLT directly instead of embeddedParser.Parse:
// the latter needs a real audio container (io.ReadSeeker + tag.ReadFrom), which
// an inline byte string cannot provide. SYLT is the only path by which embedded
// produces multiple display lines (plain text splits on \n into separate lines,
// and LRC goes through lrcParser); container-level behaviour is covered by the
// real fixture in embedded_test.go.
var contractCases = map[string]contractCase{
	"embedded": {
		parse: func(t *testing.T) *Data {
			t.Helper()
			d := parseSYLT(syltBody(
				syltEntry{"first\nsecond", 1000},
				syltEntry{"third", 2000},
			))
			if d == nil {
				t.Fatal("parseSYLT returned nil")
			}
			return d
		},
		expect: expectation{hasParts: true},
	},
	"eslrc": {
		parse:  viaParser("eslrc", eslrcContractSample),
		expect: expectation{requireEnd: true, meta: true},
	},
	"lrc": {
		parse:  viaParser("lrc", lrcContractSample),
		expect: expectation{hasParts: true, translations: true, meta: true},
	},
	"lys": {
		parse: viaParser("lys", lysContractSample),
		// requireEnd stays false: End is derived from the last word, so falling
		// back to unbounded when word times are missing is an acceptable
		// degradation. The sample's End > 0 is asserted specifically by
		// TestLYS_EndIsLastWordEnd in format_test.go, so this false cannot turn
		// into a permanent green light.
	},
	"qrc": {
		parse:  viaParser("qrc", qrcContractSample),
		expect: expectation{requireEnd: true, meta: true},
	},
	"smi": {
		parse:  viaParser("smi", smiContractSample),
		expect: expectation{hasParts: true},
	},
	"srt": {
		parse:  viaParser("srt", srtContractSample),
		expect: expectation{hasParts: true},
	},
	"ttml": {
		parse:  viaParser("ttml", ttmlAMLLSample),
		expect: expectation{requireEnd: true, hasParts: true, translations: true, meta: true, looseTiling: true},
	},
	"yrc": {
		parse:  viaParser("yrc", yrcContractSample),
		expect: expectation{requireEnd: true, meta: true},
	},
}

const (
	lrcContractSample = "[ti:Contract]\n[ar:Tester]\n[offset:250]\n" +
		"[00:01.00]Hello\n" +
		"[00:01.00]你好\n" +
		"[00:05.00]Bye\n"

	qrcContractSample = "[ti:Contract]\n[ar:Tester]\n[offset:250]\n" +
		"[1000,2000]Hello(1000,500) (1500,500)world\n" +
		"[3000,2000]Bye(3000,500)\n" +
		// The platform's real shape: the space between words is written as a
		// degenerate (0,0) filler tuple that carries no time itself.
		"[5000,2000]I(5000,200) (0,0)could(5200,300) (0,0)not(5500,300)\n"

	yrcContractSample = "[ti:Contract]\n[ar:Tester]\n[offset:250]\n" +
		"[1000,2000](1000,500,0)Hello(1500,500,0) world\n" +
		"[3000,2000](3000,500,0)Bye\n" +
		// The same filler shape, with the tuples before the text.
		"[5000,2000](5000,200,0)I(0,0,0) (5200,300,0)could(0,0,0) (5500,300,0)not\n"

	// LYS line headers are [channel], and the body has the same shape as QRC's
	// (text before the timestamps).
	lysContractSample = "[0]Hello(1000,500) (1500,500)world\n" +
		"[2]Duet(3000,500)\n" +
		"[0]I(5000,200) (0,0)could(5200,300) (0,0)not(5500,300)\n"

	// Two <P Class=...> under one SYNC is the B shape (two LyricLines); <br> is the
	// A shape (Parts).
	smiContractSample = "<SMI><BODY>" +
		"<SYNC Start=1000><P Class=KRCC>first<br>second" +
		"<P Class=ENCC>uno<br>dos" +
		"<SYNC Start=2000><P Class=KRCC>last" +
		"</BODY></SMI>"

	srtContractSample = "1\n" +
		"00:00:01,000 --> 00:00:03,000\n" +
		"first line\nsecond line\n\n" +
		"2\n" +
		"00:00:05,000 --> 00:00:07,000\n" +
		"bye\n"
)

// eslrcContractSample = metadata header + the spec word-by-word sample (testESLRC
// in format_test.go:200). The header adds only [ti:]/[ar:] and no [offset:]: the
// exact semantics of offset are covered by the unit tests in format_test.go, so
// keeping the timings unchanged here lets this share one data set with testESLRC's
// existing assertions.
const eslrcContractSample = "[ti:Contract]\n[ar:Tester]\n" + testESLRC
