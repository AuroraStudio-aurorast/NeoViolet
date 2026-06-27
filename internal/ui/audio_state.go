package ui

import (
	"fmt"
	"math"
	"sort"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

func (a *AudioState) TogglePlayback() {
	if a.Player == nil {
		return
	}
	if a.Player.IsPlaying() {
		logger.Debug("TogglePlayback: pause")
		a.Player.Pause()
		a.IsPlaying = false
	} else {
		logger.Debug("TogglePlayback: play")
		a.Player.Play()
		a.IsPlaying = true
	}
}

func (a *AudioState) Close() {
	if a.Player != nil {
		logger.Debug("AudioState.Close")
		a.Player.Close()
		a.Player = nil
	}
}

// Reset clears all audio state fields except Volume (which is preserved).
func (a *AudioState) Reset() {
	savedVolume := a.Volume
	a.Player = nil
	a.CurrentSong = ""
	a.Artist = ""
	a.Album = ""
	a.Progress = 0
	a.Duration = 0
	a.Elapsed = 0
	a.IsPlaying = false
	a.Lyrics = nil
	a.LyricIndex = -1
	a.LyricScrollOffset = 0
	a.LyricScrollTick = 0
	a.LastLyricIndex = 0
	a.ActiveLyricLines = nil
	a.lastActiveSig = ""
	a.LyricNextIndex = -1
	a.LyricGapDuration = 0
	a.Volume = savedVolume
}

func clampVolume(v float64) float64 {
	if v > 1.0 {
		return 1.0
	}
	if v < 0 {
		return 0
	}
	return math.Round(v*100) / 100
}

func (a *AudioState) AdjustVolume(delta float64) {
	a.Volume = clampVolume(a.Volume + delta)
	logger.Debug("AdjustVolume", "delta", delta, "newVolume", a.Volume)
	if a.Player != nil {
		a.Player.SetVolume(a.Volume)
	}
}

func (a *AudioState) SetVolume(vol float64) {
	a.Volume = clampVolume(vol)
	logger.Debug("SetVolume", "volume", a.Volume)
	if a.Player != nil {
		a.Player.SetVolume(a.Volume)
	}
}

func (a *AudioState) UpdatePosition() {
	if a.Player == nil {
		return
	}
	pos := a.Player.Position()
	dur := a.Player.Duration()

	a.Elapsed = pos
	if dur > 0 && a.Duration != dur {
		a.Duration = dur
	}
	if a.Duration > 0 {
		a.Progress = float64(pos) / float64(a.Duration)
		if a.Progress > 1.0 {
			a.Progress = 1.0
		}
		if a.Progress < 0 {
			a.Progress = 0
		}
	} else {
		a.Progress = 0
	}
}

func (a *AudioState) SeekPlayer(pos time.Duration) error {
	if a.Player == nil {
		return nil
	}
	logger.Debug("SeekPlayer", "position", pos)
	return a.Player.Seek(pos)
}

func (a *AudioState) SeekRelative(delta time.Duration) time.Duration {
	if a.Player == nil {
		return 0
	}
	current := a.Player.Position()
	newPos := current + delta
	if newPos < 0 {
		newPos = 0
	}
	if a.Duration > 0 && newPos > a.Duration {
		newPos = a.Duration
	}
	logger.Debug("SeekRelative", "delta", delta, "from", current, "to", newPos)
	a.Player.Seek(newPos)
	return newPos
}

func (a *AudioState) UpdateLyricIndex() {
	// Reset gap tracking state; set below if in a gap
	a.LyricNextIndex = -1
	a.LyricGapDuration = 0

	if a.Lyrics == nil {
		a.LyricIndex = -1
		a.ActiveLyricLines = nil
		a.LyricScrollOffset = 0
		return
	}

	// Use ActiveLines to get all active lyric lines at the current elapsed time
	active := a.Lyrics.ActiveLines(a.Elapsed)
	a.ActiveLyricLines = active

	// Compute signature for change detection
	sig := ""
	for _, l := range active {
		sig += fmt.Sprintf("%d|%s|", l.Time.Nanoseconds(), l.Text)
	}
	if sig != a.lastActiveSig {
		a.lastActiveSig = sig
		a.LyricScrollOffset = 0
		a.LyricScrollTick = 0
	}

	// If exactly one active line, find its index for marquee-scroll support.
	// Multiple active lines (overlapping) disable marquee scroll.
	if len(active) == 1 {
		for i, line := range a.Lyrics.Lines {
			if line.Time == active[0].Time && line.Text == active[0].Text {
				if i != a.LastLyricIndex {
					a.LastLyricIndex = i
					a.LyricScrollOffset = 0
					a.LyricScrollTick = 0
				}
				a.LyricIndex = i
				return
			}
		}
	}
	a.LyricIndex = -1

	// Gap detection: no active lines -> find the next upcoming lyric line
	if len(active) == 0 && len(a.Lyrics.Lines) > 0 {
		filterAgent := a.Lyrics.AgentFilter
		nextIdx := sort.Search(len(a.Lyrics.Lines), func(i int) bool {
			return a.Lyrics.Lines[i].Time > a.Elapsed
		})
		// Advance past the gap to the first line matching the agent filter
		for filterAgent != "" && nextIdx < len(a.Lyrics.Lines) && a.Lyrics.Lines[nextIdx].Agent != filterAgent {
			nextIdx++
		}
		a.LyricNextIndex = nextIdx

		// Total gap duration: from previous line's end (or Time if End==0) to
		// next line's Time. For song start (nextIdx==0), gap starts at 0.
		// Used by the view to decide dots (>5s) vs placeholder (<=5s).
		if nextIdx < len(a.Lyrics.Lines) {
			gapStart := time.Duration(0)
			if nextIdx > 0 {
				prev := a.Lyrics.Lines[nextIdx-1]
				gapStart = prev.End
				if gapStart <= 0 {
					gapStart = prev.Time
				}
			}
			a.LyricGapDuration = a.Lyrics.Lines[nextIdx].Time - gapStart
		}
	}
}

func (a *AudioState) AdvanceLyricScroll(scrollSpeed int, maxWidth int) {
	if a.Lyrics == nil || a.LyricIndex < 0 {
		return
	}
	text := a.Lyrics.Lines[a.LyricIndex].Text
	displayWidth := lipgloss.Width(text)
	if displayWidth <= maxWidth {
		return
	}
	a.LyricScrollTick++
	if a.LyricScrollTick < scrollSpeed {
		return
	}
	a.LyricScrollTick = 0
	a.LyricScrollOffset++
	if a.LyricScrollOffset > displayWidth-maxWidth+10 {
		a.LyricScrollOffset = 0
	}
}