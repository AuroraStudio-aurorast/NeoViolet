package audio

import (
	"image"
	"time"

	"github.com/gopxl/beep/v2"
)

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
