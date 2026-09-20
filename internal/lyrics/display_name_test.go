package lyrics

import "testing"

// Every registered parser needs a full name and the table may not name an
// unregistered parser: adding a format without its name turns this red.
func TestParserDisplayNameCoversEveryParser(t *testing.T) {
	for _, name := range AvailableParsers() {
		got := ParserDisplayName(name)
		if got == "" || got == name {
			t.Errorf("parser %q has no display name", name)
		}
	}
	if len(formatNames) != len(AvailableParsers()) {
		t.Errorf("formatNames has %d entries but %d parsers are registered",
			len(formatNames), len(AvailableParsers()))
	}
	for name := range formatNames {
		if _, ok := parserMap[name]; !ok {
			t.Errorf("formatNames contains %q, which is not a registered parser", name)
		}
	}
}

// An unregistered name is returned unchanged, so a caller never renders an
// empty description.
func TestParserDisplayNameUnknown(t *testing.T) {
	if got := ParserDisplayName("nope"); got != "nope" {
		t.Errorf("ParserDisplayName(nope) = %q, want \"nope\"", got)
	}
}

func TestParserDisplayNameKnown(t *testing.T) {
	if got := ParserDisplayName("qrc"); got != "QQ Music Word-for-Word Lyrics" {
		t.Errorf("ParserDisplayName(qrc) = %q", got)
	}
	if got := ParserDisplayName("embedded"); got != "Lyrics Embedded in Audio Tags" {
		t.Errorf("ParserDisplayName(embedded) = %q", got)
	}
}
