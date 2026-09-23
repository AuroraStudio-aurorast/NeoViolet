package lyrics

// formatNames maps a registered parser name to its human-readable name. The
// wording matches the Lyrics File table in README.md; that table is
// documentation rather than a data source, so the two are kept in step by
// review, and display_name_test.go fails when a parser has no entry.
var formatNames = map[string]string{
	"embedded": "Lyrics Embedded in Audio Tags",
	"eslrc":    "Enhanced Synced Lyrics File",
	"lrc":      "Standard Synced Lyrics File",
	"lys":      "Lyricify Syllable File",
	"qrc":      "QQ Music Word-for-Word Lyrics",
	"smi":      "Synchronized Accessible Media Interchange",
	"srt":      "SubRip Text",
	"ttml":     "Timed Text Markup Language (AMLL-flavored)",
	"yrc":      "NetEase Cloud Music Word-for-Word Lyrics",
}

// ParserDisplayName returns the human-readable name of a lyric format. An
// unknown name is returned unchanged, so callers never render an empty
// description.
func ParserDisplayName(name string) string {
	if display, ok := formatNames[name]; ok {
		return display
	}
	return name
}
