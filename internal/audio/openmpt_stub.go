//go:build !openmpt

package audio

import (
	"fmt"
	"image"
	"time"

	"github.com/gopxl/beep/v2"
)

type OpenmptPlayer struct{}

func NewOpenmptPlayer(path string, sampleRate beep.SampleRate) (*OpenmptPlayer, error) {
	return nil, fmt.Errorf("openmpt not compiled (install libopenmpt and rebuild with -tags openmpt)")
}

func (p *OpenmptPlayer) Stream(samples [][2]float64) (int, bool) { return 0, false }
func (p *OpenmptPlayer) Err() error                              { return nil }
func (p *OpenmptPlayer) Play() error                             { return nil }
func (p *OpenmptPlayer) Pause()                                  {}
func (p *OpenmptPlayer) Stop()                                   {}
func (p *OpenmptPlayer) Seek(time.Duration) error                { return nil }
func (p *OpenmptPlayer) SetVolume(float64)                       {}
func (p *OpenmptPlayer) Volume() float64                         { return 0 }
func (p *OpenmptPlayer) Duration() time.Duration                 { return 0 }
func (p *OpenmptPlayer) Position() time.Duration                 { return 0 }
func (p *OpenmptPlayer) Close() error                            { return nil }
func (p *OpenmptPlayer) Title() string                           { return "" }
func (p *OpenmptPlayer) Artist() string                          { return "" }
func (p *OpenmptPlayer) CoverImage() image.Image                 { return nil }
func (p *OpenmptPlayer) Format() beep.Format                     { return beep.Format{} }
func (p *OpenmptPlayer) Streamer() SynthStreamer                 { return nil }
