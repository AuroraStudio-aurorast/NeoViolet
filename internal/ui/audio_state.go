package ui

import "time"

func (a *AudioState) TogglePlayback() {
	if a.Player == nil {
		return
	}
	if a.Player.IsPlaying() {
		a.Player.Pause()
		a.IsPlaying = false
	} else {
		a.Player.Play()
		a.IsPlaying = true
	}
}

func (a *AudioState) Close() {
	if a.Player != nil {
		a.Player.Close()
		a.Player = nil
	}
}

func (a *AudioState) AdjustVolume(delta float64) {
	a.Volume += delta
	if a.Volume > 1.0 {
		a.Volume = 1.0
	}
	if a.Volume < 0 {
		a.Volume = 0
	}
	if a.Player != nil {
		a.Player.SetVolume(a.Volume)
	}
}

func (a *AudioState) SetVolume(vol float64) {
	a.Volume = vol
	if a.Volume > 1.0 {
		a.Volume = 1.0
	}
	if a.Volume < 0 {
		a.Volume = 0
	}
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
	a.Player.Seek(newPos)
	return newPos
}
