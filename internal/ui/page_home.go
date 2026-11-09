package ui

import (
	"fmt"

	"github.com/charmbracelet/x/ansi"
)

type pageTrack struct{ title, artist, album, duration, format string }

var pageTracks = [...]pageTrack{
	{title: "Evening Sketch", artist: "Aurora", album: "After the Rain", duration: "3:42", format: "FLAC"},
	{title: "雨のち晴れ", artist: "藍色工房", album: "街の余白", duration: "4:18", format: "FLAC"},
	{title: "Glasshouse", artist: "Elena Vale", album: "Soft Focus", duration: "3:56", format: "MP3"},
	{title: "Night Bus", artist: "Northbound", album: "Homebound", duration: "5:06", format: "OPUS"},
	{title: "静かな街 / 長い名前の音楽", artist: "藍色工房", album: "街の余白", duration: "4:37", format: "ALAC"},
	{title: "Last Light", artist: "Aurora", album: "After the Rain", duration: "3:19", format: "WAV"},
	{title: "Paper Moon", artist: "Jun Sato", album: "Window Seat", duration: "4:02", format: "FLAC"},
	{title: "A Place to Return", artist: "Northbound", album: "Homebound", duration: "6:11", format: "FLAC"},
}
var pageScopes = [...]string{"All tracks", "FLAC", "MP3", "OPUS", "ALAC", "WAV"}

func (p *pageState) renderHome(c *pageCanvas, focused bool) {
	w, h := c.width, len(c.rows)
	wide := w >= 74 && h >= 14
	x := 1
	if wide {
		x = 21
	}
	listW := w - x - 1
	items := p.tracks()
	c.pair(0, 1, w-2, "Music library", pageCount(len(items), "tracks"))
	c.dim(1, x, "f Filter: "+p.filter)
	if p.filter == "" {
		c.dim(1, x, "f Filter: All tracks")
	}
	if wide {
		c.dim(3, 1, "Formats")
		for i, scope := range pageScopes {
			text := "  " + scope
			if i == p.scope {
				c.label(5+i, 1, "● "+scope)
			} else {
				c.dim(5+i, 1, text)
			}
		}
		c.divider(18, 3, h-4)
		if h >= 20 {
			c.dim(15, 2, "5 artists")
			c.dim(16, 2, "5 albums")
			c.label(18, 2, "o Open audio")
		}
		c.rule(2, x, listW)
		c.dim(3, x, "Title / Artist / Album")
	} else {
		c.label(2, 1, "←/→ Format: "+pageScopes[p.scope])
	}
	slots := max(1, (h-5)/2)
	cursor := max(0, min(p.cursors[0], len(items)-1))
	start := max(0, cursor-slots+1)
	for n, id := range items[start:min(len(items), start+slots)] {
		y := 4 + n*2
		track := pageTracks[id]
		marker := "  "
		if start+n == cursor {
			marker = "▸ "
		}
		if id == p.playing {
			marker += "♪ "
		}
		c.pair(y, x, listW, marker+track.title, track.duration)
		c.dim(y+1, x+3, ansi.Truncate(track.artist+" / "+track.album, max(0, listW-10), "…"))
		c.dim(y+1, x+listW-5, track.format)
		if start+n == cursor && focused {
			c.highlight(y, x, listW)
			c.highlight(y+1, x, listW)
		}
	}
	if len(items) == 0 {
		c.dim(4, x, "No matching sample tracks")
		c.label(6, x, "f Clear filter")
	}
	c.dim(h-1, 1, fmt.Sprintf("Sample data · %d tracks · f Filter · a Queue · o Browse · O Path", len(items)))
}
