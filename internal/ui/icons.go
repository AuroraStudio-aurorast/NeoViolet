package ui

// IconSet holds the glyphs used for each UI element.
type IconSet struct {
	Play        string
	Pause       string
	Next        string
	Prev        string
	Volume      string
	Command     string
	Home        string
	Playlist    string
	Effects     string
	Settings    string
	Search      string
	Music       string
	LyricFilled string
	LyricEmpty  string
}

// NerdIcons is the Nerd Font icon set.
var NerdIcons = IconSet{
	Play:        "\uf04b",
	Pause:       "\uf04c",
	Next:        "\U000f04ad",
	Prev:        "\U000f04ae",
	Volume:      "\uf028",
	Command:     ":",
	Home:        "\uf46d",
	Playlist:    "\uf0ca",
	Effects:     "\uf0eb",
	Settings:    "\uf013",
	Search:      "\uf002",
	Music:       "\uf001",
	LyricFilled: "\u25cf",
	LyricEmpty:  "\u25cb",
}

// FallbackIcons is the basic-Unicode icon set used when no icon font is available.
var FallbackIcons = IconSet{
	Play:        "\u25b6",
	Pause:       "\u23f8",
	Next:        "\u23ed",
	Prev:        "\u23ee",
	Volume:      "VOL",
	Command:     ":",
	Home:        "",
	Playlist:    "",
	Effects:     "",
	Settings:    "",
	Search:      "?",
	Music:       "",
	LyricFilled: "\u25cf",
	LyricEmpty:  "\u25cb",
}

// EmojiIcons is the emoji icon set.
var EmojiIcons = IconSet{
	Play:        "\u25b6\ufe0f",
	Pause:       "\u23f8\ufe0f",
	Next:        "\u23ed\ufe0f",
	Prev:        "\u23ee\ufe0f",
	Volume:      "\U0001f50a",
	Command:     ":",
	Home:        "\U0001f3e0",
	Playlist:    "\U0001f4cb",
	Effects:     "\u2728",
	Settings:    "\u2699\ufe0f",
	Search:      "\U0001f50d",
	Music:       "\U0001f3b5",
	LyricFilled: "\U0001f7e2",
	LyricEmpty:  "\u26aa",
}
