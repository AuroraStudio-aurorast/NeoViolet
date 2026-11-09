package ui

import (
	"fmt"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
)

func (m *Model) renderCollections(c *pageCanvas, focused bool) {
	p, state := m.pages, &m.playlists
	w, h := c.width, len(c.rows)
	c.pair(0, 1, w-2, "Saved playlists", "Queue  [ Saved ]")
	switch {
	case state.loading || !state.loaded:
		c.dim(3, 2, "Loading playlists…")
		return
	case state.err != "":
		c.label(3, 2, "Cannot read playlists.json")
		c.dim(5, 2, truncateLine(browserDisplayText(state.err), w-4))
		c.dim(h-1, 1, "Repair file · r Retry · v Queue")
		return
	case len(state.lists) == 0:
		c.dim(1, 1, "0 playlists")
		c.text(4, 2, "No saved playlists yet")
		c.dim(6, 2, "c Create · s Save session queue")
		c.dim(h-1, 1, "c Create · s Save queue · r Refresh · v Queue")
		return
	}
	status := fmt.Sprintf("%d playlists · %d/%d selected", len(state.lists), p.collection+1, len(state.lists))
	if state.save != nil {
		status = "Saving playlist…"
	}
	c.dim(1, 1, status)
	controls := "c New · e Rename · d Delete · r Reload"
	if p.detail {
		controls = "A Add queue · J/K Order · r Reload"
	}
	c.dim(2, 1, controls)
	current := m.selectedPlaylist()
	wide := w >= 80 && h >= 14
	if wide || !p.detail {
		listW := w - 3
		if wide {
			listW = 23
		}
		stride := 3
		if h < 14 {
			stride = 1
		}
		slots := max(1, (h-5)/stride)
		start := max(0, p.collection-slots+1)
		for n, list := range state.lists[start:min(len(state.lists), start+slots)] {
			i, y := start+n, 4+n*stride
			c.text(y, 2, truncateLine(browserDisplayText(list.Name), listW-1))
			if stride > 1 {
				c.dim(y+1, 3, truncateLine(fmt.Sprintf("%d tracks / %s", len(list.Tracks), savedClock(list.Tracks)), listW-2))
			}
			if i == p.collection && focused && !p.detail {
				c.highlight(y, 1, listW)
			}
		}
		if !wide {
			c.dim(h-1, 1, "Enter Open · P Play · a Append · v Queue")
			return
		}
	}
	x := 2
	if wide {
		x = 28
		c.divider(25, 3, h-4)
	}
	cw := w - x - 1
	c.label(3, x, truncateLine(browserDisplayText(current.Name), cw))
	c.dim(4, x, fmt.Sprintf("%d tracks / %s", len(current.Tracks), savedClock(current.Tracks)))
	top := 6
	if h < 14 {
		top = 5
	}
	if len(current.Tracks) == 0 {
		c.text(top, x, "Empty playlist")
		c.dim(top+2, x, "A Add session queue")
	}
	slots := max(1, h-top-3)
	start := max(0, p.savedTrack-slots+1)
	for n, track := range current.Tracks[start:min(len(current.Tracks), start+slots)] {
		i, y := start+n, top+n
		title := browserDisplayText(track.Title)
		if title == "" {
			title = browserDisplayText(track.Path)
		}
		right := trackClock(track)
		if state.unavailable[track.Path] != "" {
			right = "Missing"
		}
		c.pair(y, x, cw, fmt.Sprintf("%02d  %s", i+1, title), right)
		if i == p.savedTrack && p.detail && focused {
			c.highlight(y, x, cw)
		}
	}
	if p.detail && len(current.Tracks) > 0 {
		track := current.Tracks[min(p.savedTrack, len(current.Tracks)-1)]
		path := browserDisplayText(track.Path)
		if reason := state.unavailable[track.Path]; reason != "" {
			path = reason + ": " + path
		}
		c.dim(h-2, x, truncateLine(path, cw))
	}
	hint := "Enter Details · P Play · a Append · v Queue"
	if p.detail {
		hint = "Enter Play · d Remove · Esc Back"
	}
	c.dim(h-1, 1, hint)
}
func savedClock(tracks []library.Track) string {
	var duration time.Duration
	unknown := false
	for _, track := range tracks {
		duration += track.Duration
		if track.Duration == 0 {
			unknown = true
		}
	}
	result := fmt.Sprintf("%d:%02d", int(duration.Minutes()), int(duration.Seconds())%60)
	if unknown {
		result += " + ?"
	}
	return result
}
