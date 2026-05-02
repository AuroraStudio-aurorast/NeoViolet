package ui

type IconSet struct {
	Play     string
	Pause    string
	Next     string
	Prev     string
	Volume   string
	Command  string
	Home     string
	Playlist string
	Effects  string
	Settings string
	Search   string
	Music    string
}

var NerdIcons = IconSet{
	Play:     "\uf04b",
	Pause:    "\uf04c",
	Next:     "\uf050",
	Prev:     "\uf04a",
	Volume:   "\uf028",
	Command:  ":",
	Home:     "\uf46d",
	Playlist: "\uf0ca",
	Effects:  "\uf0eb",
	Settings: "\uf013",
	Search:   "\uf002",
	Music:    "\uf001",
}

var FallbackIcons = IconSet{
	Play:     "\u25b6",
	Pause:    "\u23f8",
	Next:     "\u23ed",
	Prev:     "\u23ee",
	Volume:   "VOL",
	Command:  ":",
	Home:     "",
	Playlist: "",
	Effects:  "",
	Settings: "",
	Search:   "?",
	Music:    "",
}

var EmojiIcons = IconSet{
	Play:     "\u25b6\ufe0f",
	Pause:    "\u23f8\ufe0f",
	Next:     "\u23ed\ufe0f",
	Prev:     "\u23ee\ufe0f",
	Volume:   "\U0001f50a",
	Command:  ":",
	Home:     "\U0001f3e0",
	Playlist: "\U0001f4cb",
	Effects:  "\u2728",
	Settings: "\u2699\ufe0f",
	Search:   "\U0001f50d",
	Music:    "\U0001f3b5",
}
