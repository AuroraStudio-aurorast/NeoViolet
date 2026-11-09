package ui

import (
	"fmt"
	"strconv"
	"strings"
)

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

func (p *pageState) renderPlaylists(c *pageCanvas, focused bool) {
	if p.saved {
		p.renderCollections(c, focused)
		return
	}
	w, h := c.width, len(c.rows)
	c.pair(0, 1, w-2, "Session queue", "[ Queue ]  Saved")
	c.dim(1, 1, fmt.Sprintf("%d tracks / %s · sample sequence", len(p.queue), pageTotal(p.queue)))
	if len(p.queue) == 0 {
		c.text(4, 3, "Your sample queue is empty")
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
			c.dim(18, x, "Sample content")
		}
	}
	stride := 3
	if h < 14 {
		stride = 1
	}
	slots := max(1, (h-5)/stride)
	cursor := min(p.cursors[1], len(p.queue)-1)
	start := max(0, cursor-slots+1)
	for n, id := range p.queue[start:min(len(p.queue), start+slots)] {
		i, y := start+n, 4+n*stride
		track := pageTracks[id]
		marker := "○"
		if id == p.playing {
			marker = "◆"
		}
		c.label(y, 2, marker)
		c.pair(y, 5, listW-5, fmt.Sprintf("%02d  %s", i+1, track.title), track.duration)
		if stride > 1 {
			c.dim(y+1, 2, "│")
			state := "Queued"
			if id == p.playing {
				state = "Playing in sample"
			}
			c.dim(y+1, 9, truncateLine(state+" / "+track.artist, max(0, listW-10)))
			if i+1 < len(p.queue) {
				c.dim(y+2, 2, "│")
			}
		}
		if i == cursor && focused {
			c.highlight(y, 1, listW)
		}
	}
	c.dim(h-1, 1, "Sample data · v Queue/Saved · J/K Order · d Remove · u Undo")
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
