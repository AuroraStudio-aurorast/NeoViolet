package audio

import (
	"fmt"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/speaker"
)

var initSpeakerOnce sync.Once
var initSpeakerErr error

type Player struct {
	mu           sync.Mutex
	streamer     beep.StreamSeekCloser
	ctrl         *beep.Ctrl
	volume       *effects.Volume
	format       beep.Format
	isPaused     bool
	isPlaying    bool
	file         io.Closer
	path         string
	linearVolume float64
	title        string
	artist       string
	decoder      *FormatDecoder
	tagReader    *MetadataReader
}

func NewPlayer() *Player {
	return NewPlayerWithDeps(NewFormatDecoder(), NewMetadataReader())
}

func NewPlayerWithDeps(decoder *FormatDecoder, tagReader *MetadataReader) *Player {
	return &Player{
		isPaused:     true,
		isPlaying:    false,
		linearVolume: 1.0,
		decoder:      decoder,
		tagReader:    tagReader,
	}
}

func ensureSpeakerInit(sampleRate beep.SampleRate) error {
	initSpeakerOnce.Do(func() {
		initSpeakerErr = speaker.Init(sampleRate, sampleRate.N(time.Second/10))
	})
	return initSpeakerErr
}

func (p *Player) Open(path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isPlaying {
		speaker.Clear()
		p.isPlaying = false
	}
	if p.streamer != nil {
		if p.file != nil {
			p.file.Close()
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open audio file: %w", err)
	}

	streamer, format, err := p.decoder.Decode(file, path)
	if err != nil {
		file.Close()
		return err
	}

	if err := ensureSpeakerInit(format.SampleRate); err != nil {
		file.Close()
		return fmt.Errorf("speaker init failed: %w", err)
	}

	p.streamer = streamer
	p.format = format
	p.file = file
	p.isPaused = true
	p.isPlaying = false
	p.path = path

	p.ctrl = &beep.Ctrl{
		Streamer: streamer,
		Paused:   true,
	}

	p.volume = &effects.Volume{
		Streamer: p.ctrl,
		Base:     2,
		Silent:   false,
	}

	p.applyLinearVolumeLocked()

	p.readTags(path)

	return nil
}

func (p *Player) applyLinearVolumeLocked() {
	if p.volume == nil {
		return
	}

	p.mu.Unlock()
	defer p.mu.Lock()

	speaker.Lock()
	defer speaker.Unlock()
	if p.linearVolume <= 0.0 {
		p.volume.Silent = true
		p.volume.Volume = 0
	} else {
		p.volume.Silent = false
		exponent := math.Log2(p.linearVolume)
		p.volume.Volume = exponent
	}
}

func (p *Player) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.streamer == nil || p.volume == nil {
		return fmt.Errorf("player not initialized")
	}

	speaker.Lock()
	p.ctrl.Paused = false
	speaker.Unlock()

	if p.isPlaying {
		speaker.Clear()
		p.isPlaying = false
	}

	speaker.Play(p.volume)
	p.isPlaying = true
	p.isPaused = false
	return nil
}

func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctrl == nil {
		return
	}
	speaker.Lock()
	p.ctrl.Paused = true
	speaker.Unlock()
	p.isPaused = true
}

func (p *Player) Resume() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctrl == nil {
		return
	}
	speaker.Lock()
	p.ctrl.Paused = false
	speaker.Unlock()
	p.isPaused = false
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	speaker.Clear()
	p.isPlaying = false
	p.isPaused = true
}

func (p *Player) Toggle() {
	if p.isPaused || !p.isPlaying {
		p.Play()
	} else {
		p.Pause()
	}
}

func (p *Player) Seek(position time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.streamer == nil || p.path == "" {
		return nil
	}

	targetSamples := int(position.Seconds() * float64(p.format.SampleRate))
	if targetSamples < 0 {
		targetSamples = 0
	}
	totalSamples := p.streamer.Len()
	if targetSamples > totalSamples {
		targetSamples = totalSamples
	}

	wasPlaying := p.isPlaying && !p.isPaused
	oldLinearVolume := p.linearVolume

	if p.file != nil {
		p.file.Close()
	}
	speaker.Clear()

	file, err := os.Open(p.path)
	if err != nil {
		return fmt.Errorf("reopen for seek: %w", err)
	}

	newStreamer, newFormat, err := p.decoder.Decode(file, p.path)
	if err != nil {
		file.Close()
		return fmt.Errorf("decode for seek: %w", err)
	}

	if newFormat.SampleRate != p.format.SampleRate {
		if err := ensureSpeakerInit(newFormat.SampleRate); err != nil {
			newStreamer.Close()
			file.Close()
			return fmt.Errorf("speaker reinit failed: %w", err)
		}
	}

	if err := newStreamer.Seek(targetSamples); err != nil {
		newStreamer.Close()
		file.Close()
		return fmt.Errorf("seek to %v: %w", position, err)
	}

	p.streamer = newStreamer
	p.format = newFormat
	p.file = file

	p.ctrl = &beep.Ctrl{
		Streamer: p.streamer,
		Paused:   true,
	}
	p.volume = &effects.Volume{
		Streamer: p.ctrl,
		Base:     2,
		Silent:   false,
	}
	p.linearVolume = oldLinearVolume
	p.applyLinearVolumeLocked()

	if wasPlaying {
		speaker.Lock()
		p.ctrl.Paused = false
		speaker.Unlock()
		speaker.Play(p.volume)
		p.isPlaying = true
		p.isPaused = false
	} else {
		p.isPlaying = false
		p.isPaused = true
	}

	return nil
}

func (p *Player) SetVolume(vol float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if vol < 0 {
		vol = 0
	}
	if vol > 1 {
		vol = 1
	}
	p.linearVolume = vol
	p.applyLinearVolumeLocked()
}

func (p *Player) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.linearVolume
}

func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isPlaying && !p.isPaused
}

func (p *Player) Duration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.streamer == nil {
		return 0
	}
	return p.format.SampleRate.D(p.streamer.Len())
}

func (p *Player) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.streamer == nil {
		return 0
	}
	return p.format.SampleRate.D(p.streamer.Position())
}

func (p *Player) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	speaker.Clear()
	if p.file != nil {
		p.file.Close()
		p.file = nil
	}
	p.streamer = nil
	p.ctrl = nil
	p.volume = nil
	p.isPaused = true
	p.isPlaying = false
	return nil
}

func (p *Player) Format() beep.Format {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.format
}

func (p *Player) Path() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.path
}

func (p *Player) Title() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.title
}

func (p *Player) Artist() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.artist
}

var ErrUnsupportedFormat = &UnsupportedFormatError{}

type UnsupportedFormatError struct{}

func (e *UnsupportedFormatError) Error() string {
	return "unsupported audio format"
}
