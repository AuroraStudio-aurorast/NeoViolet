package ui

import (
	"fmt"
)

func (m *Model) renderPlaylists(c *pageCanvas, focused bool) {
	p := m.pages
	entries := m.session.entries
	if p.saved {
		m.renderCollections(c, focused)
		return
	}
	w, h := c.width, len(c.rows)
	c.pair(0, 1, w-2, "Session queue", "[ Queue ]  Saved")
	c.dim(1, 1, fmt.Sprintf("%d tracks / %s · in sequence", len(entries), queueClock(entries)))
	if len(entries) == 0 {
		c.text(4, 3, "Your session queue is empty")
		c.dim(6, 3, "s Save empty playlist · v Saved · u Undo")
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
