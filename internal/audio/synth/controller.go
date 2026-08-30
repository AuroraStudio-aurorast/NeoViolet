package synth

import (
	"image"
	"time"

	"github.com/gopxl/beep/v2"
)

// Streamer wraps a beep.Streamer with its output format.
type Streamer interface {
	beep.Streamer
	Format() beep.Format
}

// Controller is the playback interface implemented by synth backends.
type Controller interface {
	Play() error
	Pause()
	Stop()
	Seek(time.Duration) error
	SetVolume(float64)
	Volume() float64
	Duration() time.Duration
	Position() time.Duration
	Close() error
	Title() string
	Artist() string
	CoverImage() image.Image
	Streamer() Streamer
}
