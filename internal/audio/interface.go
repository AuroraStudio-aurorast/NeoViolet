// Package audio provides the playback engine and player abstractions.
package audio

import (
	"image"
	"time"

	"github.com/gopxl/beep/v2"
)

// AudioPlayer is the subset of the Player API consumed by the UI layer.
//
//nolint:revive // "Player" would collide with the concrete audio.Player struct.
type AudioPlayer interface {
	Open(path string) error
	Play() error
	Pause()
	Stop()
	Toggle()
	Seek(position time.Duration) error
	SetVolume(vol float64)
	Volume() float64
	IsPlaying() bool
	Duration() time.Duration
	Position() time.Duration
	Close() error
	Format() beep.Format
	Path() string
	Title() string
	Artist() string
	Album() string
	CoverImage() image.Image
}
