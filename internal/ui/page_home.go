package ui

import (
	"fmt"

	"github.com/charmbracelet/x/ansi"
)

func (m *Model) renderHome(c *pageCanvas, focused bool) {
	p := m.pages
	w, h := c.width, len(c.rows)
	wide := w >= 74 && h >= 14
	x := 1
	if wide {
		x = 21
	}
	listW := w - x - 1
	items := p.visible
	c.pair(0, 1, w-2, "Music library", pageCount(len(items), "tracks"))
	if focused {
		c.label(0, 1, "Music library")
	}
	c.dim(1, x, "f Filter: "+p.filter)
	if p.filter == "" {
		c.dim(1, x, "f Filter: All tracks")
	}
	if wide {
		c.dim(3, 1, "Formats")
		scopeRows := max(1, min(10, h-8))
		scopeStart := p.viewports.scopes.start(p.scope, len(p.scopes), scopeRows)
		for n, scope := range p.scopes[scopeStart:min(len(p.scopes), scopeStart+scopeRows)] {
			i := n + scopeStart
			text := "  " + scope
			if i == p.scope {
				c.label(5+n, 1, "● "+scope)
			} else {
				c.dim(5+n, 1, text)
			}
		}
		c.divider(18, 3, h-4)
		if h >= 20 {
			c.dim(15, 2, pageCount(p.artists, "artists"))
			c.dim(16, 2, pageCount(p.albums, "albums"))
			c.label(18, 2, "o Open audio")
		}
		c.rule(2, x, listW)
		c.dim(3, x, "Title / Artist / Album")
	} else {
		c.label(2, 1, "←/→ Format: "+p.scopes[p.scope])
	}
	slots := listCapacity(h, 4, 1, 2)
	cursor := max(0, min(p.cursors[0], len(items)-1))
	start := p.viewports.home.start(cursor, len(items), slots)
	for n, track := range items[start:min(len(items), start+slots)] {
		y := 4 + n*2
		marker := "  "
		if start+n == cursor {
			marker = "▸ "
		}
		if m.Audio.Player != nil && m.Audio.Player.Path() == track.Path {
			marker += "♪ "
		}
		c.pair(y, x, listW, marker+browserDisplayText(track.Title), trackClock(track))
		c.dim(y+1, x+3, ansi.Truncate(browserDisplayText(trackDescription(track)), max(0, listW-10), "…"))
		c.dim(y+1, x+listW-5, track.Format)
		if start+n == cursor && focused {
			c.highlight(y, x, listW)
			c.highlight(y+1, x, listW)
		}
	}
	if len(items) == 0 {
		c.dim(4, x, "No matching tracks")
		c.label(6, x, "f Clear filter")
	}
	status := fmt.Sprintf("%d tracks · Enter Play · f Filter · a Queue · r Rescan", len(items))
	if m.scanning {
		status = fmt.Sprintf("Scanning… %d / %d · previous results kept", m.scanDone, m.scanTotal)
	}
	if len(m.scanIssues) > 0 && !m.scanning {
		status = fmt.Sprintf("%d tracks · %d scan issues · r Retry", len(items), len(m.scanIssues))
	}
	if len(m.Config.Library.Roots) == 0 {
		c.text(4, x, "No music folders configured")
		c.label(6, x, "Settings → Music folders · o Open audio")
	}
	if len(m.libraryTracks) == 0 && m.scanning {
		c.text(4, x, "Scanning music folders…")
	}
	c.dim(h-1, 1, status)
}
