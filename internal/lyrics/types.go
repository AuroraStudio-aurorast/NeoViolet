package lyrics

import "time"

type WordFragment struct {
	Time time.Duration
	Text string
}

type LyricLine struct {
	Time  time.Duration
	Text  string
	Words []WordFragment
}

type LyricsData struct {
	Title   string
	Artist  string
	Album   string
	Author  string
	Creator string
	Offset  int
	Lines   []LyricLine
	Path    string
}

type TimestampGroup struct {
	IsMeta    bool
	MetaKey   string
	MetaValue string
	Time      time.Duration
}
