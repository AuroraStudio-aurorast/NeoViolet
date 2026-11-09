package ui

import (
	"fmt"
	"strconv"
	"strings"
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

type pageCollection struct {
	name, description string
	tracks            []int
}

var pageCollections = [...]pageCollection{
	{name: "After the Rain", description: "Soft voices for the end of a day", tracks: []int{0, 1, 2, 5}},
	{name: "Late train", description: "Long journeys, familiar windows", tracks: []int{3, 7, 0}},
	{name: "Quiet mornings", description: "A slower start", tracks: []int{6, 1, 5}},
}

func pageTotal(ids []int) string {
	var seconds int
	for _, id := range ids {
		parts := strings.Split(pageTracks[id].duration, ":")
		minutes, _ := strconv.Atoi(parts[0])
		remaining, _ := strconv.Atoi(parts[1])
		seconds += minutes*60 + remaining
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func (m *Model) renderPlaylists(c *pageCanvas, focused bool) {
	p := m.pages
	entries := m.session.entries
	if p.saved {
		p.renderCollections(c, focused)
		return
	}
	w, h := c.width, len(c.rows)
	c.pair(0, 1, w-2, "Session queue", "[ Queue ]  Saved")
	c.dim(1, 1, fmt.Sprintf("%d tracks / %s · in sequence", len(entries), queueClock(entries)))
	if len(entries) == 0 {
		c.text(4, 3, "Your session queue is empty")
		c.dim(6, 3, "u Undo removal · return to Home to browse")
		return
	}
	listW := w - 2
	if w >= 76 && h >= 14 {
		listW = w - 26
		x := listW + 4
		c.divider(listW+1, 3, h-4)
		c.dim(3, x, "Session rules")
		c.text(5, x, "In sequence")
		c.text(7, x, "Stop at end")
		c.dim(10, x, "d Remove entry")
		c.dim(11, x, "u Undo removal")
		c.dim(12, x, "J/K Move entry")
		if h >= 21 {
			c.label(16, x, "s Save as playlist")
			c.dim(18, x, "Files stay intact")
		}
	}
	stride := 3
	if h < 14 {
		stride = 1
	}
	slots := max(1, (h-5)/stride)
	cursor := min(p.cursors[1], len(entries)-1)
	start := max(0, cursor-slots+1)
	for n, entry := range entries[start:min(len(entries), start+slots)] {
		i, y := start+n, 4+n*stride
		track := entry.track
		marker := "○"
		if entry.id == m.session.current {
			marker = "◆"
		}
		c.label(y, 2, marker)
		c.pair(y, 5, listW-5, fmt.Sprintf("%02d  %s", i+1, browserDisplayText(track.Title)), trackClock(track))
		if stride > 1 {
			c.dim(y+1, 2, "│")
			state := "Queued"
			if entry.id == m.session.current {
				state = "Playing now"
			}
			if track.Artist != "" {
				state += " / " + browserDisplayText(track.Artist)
			}
			c.dim(y+1, 9, truncateLine(state, max(0, listW-10)))
			if i+1 < len(entries) {
				c.dim(y+2, 2, "│")
			}
		}
		if i == cursor && focused {
			c.highlight(y, 1, listW)
		}
	}
	c.dim(h-1, 1, "v Queue/Saved · J/K Order · d Remove · u Undo")
}

func (p *pageState) renderCollections(c *pageCanvas, focused bool) {
	w, h := c.width, len(c.rows)
	c.pair(0, 1, w-2, "Saved playlists", "Queue  [ Saved ]")
	c.dim(1, 1, "3 sample collections")
	wide := w >= 80 && h >= 14
	current := pageCollections[p.collection]
	if wide || !p.detail {
		stride := 4
		if h < 14 {
			stride = 1
		}
		for i, list := range pageCollections {
			y := 4 + i*stride
			cw := w - 3
			if wide {
				cw = 23
			}
			c.text(y, 2, truncateLine(list.name, cw))
			if h >= 14 {
				c.dim(y+1, 4, fmt.Sprintf("%d tracks / %s", len(list.tracks), pageTotal(list.tracks)))
			}
			if i == p.collection && focused {
				c.highlight(y, 1, cw)
			}
		}
		if !wide {
			c.dim(h-1, 1, "Enter Open collection · v Queue · sample data")
			return
		}
	}
	x := 2
	if wide {
		x = 28
		c.divider(25, 3, h-4)
	}
	cw := w - x - 1
	c.label(3, x, current.name)
	c.dim(4, x, fmt.Sprintf("%d tracks / %s", len(current.tracks), pageTotal(current.tracks)))
	top := 5
	if h >= 14 {
		c.text(6, x, current.description)
		top = 9
	}
	for i, id := range current.tracks[:min(len(current.tracks), max(0, h-top-1))] {
		track := pageTracks[id]
		c.pair(top+i, x, cw, fmt.Sprintf("%02d  %s", i+1, track.title), track.duration)
	}
	c.dim(h-1, 1, "Sample data · Esc Collections · a Append · v Queue")
}
