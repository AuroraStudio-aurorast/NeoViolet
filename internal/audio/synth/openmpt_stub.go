//go:build !openmpt

package synth

import (
	"fmt"
	"image"
	"time"

	"github.com/gopxl/beep/v2"
)

// OpenmptPlayer is the no-op OpenMPT backend used when the library is not compiled in.
type OpenmptPlayer struct{}

// NewOpenmptPlayer always fails because OpenMPT support is not compiled in.
func NewOpenmptPlayer(_ string, _ beep.SampleRate) (*OpenmptPlayer, error) {
	return nil, fmt.Errorf("openmpt not compiled (install libopenmpt and rebuild with -tags openmpt)")
}

// Stream implements beep.Streamer.
func (p *OpenmptPlayer) Stream(_ [][2]float64) (int, bool) { return 0, false }

// Err always returns nil.
func (p *OpenmptPlayer) Err() error { return nil }

// Play is a no-op.
func (p *OpenmptPlayer) Play() error { return nil }

// Pause is a no-op.
func (p *OpenmptPlayer) Pause() {}

// Stop is a no-op.
func (p *OpenmptPlayer) Stop() {}

// Seek is a no-op.
func (p *OpenmptPlayer) Seek(time.Duration) error { return nil }

// SetVolume is a no-op.
func (p *OpenmptPlayer) SetVolume(float64) {}

// Volume returns 0.
func (p *OpenmptPlayer) Volume() float64 { return 0 }

// Duration returns 0.
func (p *OpenmptPlayer) Duration() time.Duration { return 0 }

// Position returns 0.
func (p *OpenmptPlayer) Position() time.Duration { return 0 }

// Close returns nil.
func (p *OpenmptPlayer) Close() error { return nil }

// Title returns an empty string.
func (p *OpenmptPlayer) Title() string { return "" }

// Artist returns an empty string.
func (p *OpenmptPlayer) Artist() string { return "" }

// Album returns an empty string.
func (p *OpenmptPlayer) Album() string { return "" }

// CoverImage returns nil.
func (p *OpenmptPlayer) CoverImage() image.Image { return nil }

// Format returns an empty beep.Format.
func (p *OpenmptPlayer) Format() beep.Format { return beep.Format{} }

// Streamer returns nil.
func (p *OpenmptPlayer) Streamer() Streamer { return nil }

// OpenmptProbe always reports no match.
func OpenmptProbe([]byte) bool { return false }

// OpenmptSupportedFormats returns no formats.
func OpenmptSupportedFormats() []string { return nil }
