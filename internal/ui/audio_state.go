package ui

import (
	"math"
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
	if a.Lyrics == nil {
		a.LyricIndex = -1
		a.LyricScrollOffset = 0
		return
	}
	newIdx := a.Lyrics.CurrentLine(a.Elapsed)
	if newIdx != a.LastLyricIndex {
		a.LastLyricIndex = newIdx
		a.LyricScrollOffset = 0
		a.LyricScrollTick = 0
	}
	a.LyricIndex = newIdx
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
