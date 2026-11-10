package ui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

func (b *fileBrowser) render(c *pageCanvas) {
	w, h := c.width, len(c.rows)
	title := "Open audio"
	if b.purpose == "directory" {
		title = "Choose music folder"
	}
	if b.purpose == "soundfont" {
		title = "Choose SoundFont"
	}
	if b.purpose == "player_binary" {
		title = "Choose player executable"
	}
	status := fmt.Sprintf("%d items", len(b.entries))
	if b.loading {
		status = "Reading…"
	}
	c.pair(0, 1, w-2, title, status)
	path := b.directory
	if path == "" {
		path = b.target
	}
	c.dim(1, 1, ansi.Truncate(browserDisplayText(path), max(0, w-2), "…"))
	c.rule(2, 1, w-2)
	c.dim(3, 1, "Name")
	c.dim(3, max(1, w-12), "Type")
	available := listCapacity(h, 4, 2, 1)
	cursor := max(0, min(b.cursor, len(b.entries)-1))
	start := b.viewport.start(cursor, len(b.entries), available)
	for n, entry := range b.entries[start:min(len(b.entries), start+available)] {
		y := 4 + n
		marker := "  "
		if start+n == cursor {
			marker = "▸ "
		}
		name := entry.name
		kind := "audio"
		switch {
		case entry.directory:
			name += "/"
			kind = "folder"
		case b.purpose == "player_binary":
			kind = "program"
		case b.purpose == "soundfont":
			kind = ".sf2"
		}
		c.pair(y, 1, w-2, marker+browserDisplayText(name), kind)
		if start+n == cursor {
			c.highlight(y, 1, w-2)
		}
	}
	if len(b.entries) == 0 && !b.loading {
		c.dim(4, 1, "No matching files · Ctrl+L Change path")
	}
	switch {
	case b.err != "":
		c.label(h-2, 1, browserDisplayText(b.err))
	case b.purpose == "soundfont" || b.purpose == "player_binary":
		c.dim(h-2, 1, "Selection edits the draft; s saves after returning")
	default:
		c.dim(h-2, 1, "Enter opens the file; Esc keeps the current track")
	}
	hint := "Enter Choose · Backspace Up · Ctrl+L Path/URL · Esc Back"
	if b.purpose == "directory" {
		hint = "Enter Browse · s Select folder · Backspace Up · Esc Back"
	}
	c.dim(h-1, 1, hint)
}

func browserDisplayText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, text)
}
