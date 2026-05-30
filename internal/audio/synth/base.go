package synth

import (
	"image"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
)

// baseSynth provides shared state and method implementations for synth players
// (MIDI, tracker, OpenMPT). Embed this struct to eliminate duplicated control
// methods across all three players.
type baseSynth struct {
	mu          sync.Mutex
	sampleRate  beep.SampleRate
	duration    time.Duration
	elapsed     time.Duration
	isPlaying   bool
	isPaused    bool
	closed      bool
	finished    bool
	volumeScale float64
	title       string
	artist      string
}

func (b *baseSynth) Play() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.isPaused = false
	b.isPlaying = true
	b.finished = false
	return nil
}

func (b *baseSynth) Pause() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.isPaused = true
}

// baseStop sets common stop state. Call from overrides in specific players.
func (b *baseSynth) baseStop() {
	b.isPaused = true
	b.isPlaying = false
	b.finished = false
	b.elapsed = 0
}

func (b *baseSynth) Toggle() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.isPaused || !b.isPlaying {
		b.isPaused = false
		b.isPlaying = true
		b.finished = false
	} else {
		b.isPaused = true
	}
}

// baseClose sets common closed state. Call from overrides in specific players.
func (b *baseSynth) baseClose() {
	b.closed = true
}

func (b *baseSynth) Position() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.elapsed
}

func (b *baseSynth) Duration() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.duration
}

func (b *baseSynth) SetVolume(vol float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.volumeScale = clamp(vol, 0, 1)
}

func (b *baseSynth) Volume() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.volumeScale
}

func (b *baseSynth) Format() beep.Format {
	return beep.Format{SampleRate: b.sampleRate, NumChannels: 2, Precision: 4}
}

func (b *baseSynth) Streamer() Streamer { return nil }

func (b *baseSynth) Title() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.title
}

func (b *baseSynth) Artist() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.artist
}

func (b *baseSynth) CoverImage() image.Image { return nil }