package audio

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/jpodeszfa/go-meltysynth/meltysynth"
)

const (
	midiBlockSize  = 512
	midiSampleRate = 44100
)

type MidiPlayer struct {
	mu          sync.Mutex
	synthesizer *meltysynth.Synthesizer
	sequencer   *meltysynth.MidiFileSequencer
	midiFile    *meltysynth.MidiFile

	renderBufL []float32
	renderBufR []float32
	renderPos  int
	renderLen  int

	pendingRender int // sample target when seeking; -1 = none

	duration  time.Duration
	elapsed   time.Duration
	isPlaying bool
	isPaused  bool
	closed    bool
	title     string
}

func NewMidiPlayer(midiPath, sfPath string) (*MidiPlayer, error) {
	sfFile, err := os.Open(sfPath)
	if err != nil {
		return nil, fmt.Errorf("open soundfont: %w", err)
	}
	soundFont, err := meltysynth.NewSoundFont(sfFile)
	sfFile.Close()
	if err != nil {
		return nil, fmt.Errorf("load soundfont: %w", err)
	}

	settings := meltysynth.NewSynthesizerSettings(midiSampleRate)
	synthesizer, err := meltysynth.NewSynthesizer([]*meltysynth.SoundFont{soundFont}, settings)
	if err != nil {
		return nil, fmt.Errorf("create synthesizer: %w", err)
	}
	synthesizer.MasterVolume = 0.3

	midFile, err := os.Open(midiPath)
	if err != nil {
		return nil, fmt.Errorf("open midi file: %w", err)
	}
	midiFile, err := meltysynth.NewMidiFile(midFile)
	midFile.Close()
	if err != nil {
		return nil, fmt.Errorf("parse midi file: %w", err)
	}

	sequencer := meltysynth.NewMidiFileSequencer(synthesizer)
	sequencer.Play(midiFile, false)

	return &MidiPlayer{
		synthesizer:   synthesizer,
		sequencer:     sequencer,
		midiFile:      midiFile,
		duration:      midiFile.GetLength(),
		renderBufL:    make([]float32, midiBlockSize),
		renderBufR:    make([]float32, midiBlockSize),
		pendingRender: -1,
		isPaused:      true,
	}, nil
}

func (p *MidiPlayer) Stream(samples [][2]float64) (n int, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return 0, false
	}
	if p.isPaused {
		for i := range samples {
			samples[i] = [2]float64{}
		}
		return len(samples), true
	}

	// Handle pending seek: render to target in background
	if p.pendingRender >= 0 {
		seekBlock := midiSampleRate
		buf := make([]float32, seekBlock)
		for rendered := 0; rendered < p.pendingRender; {
			n := seekBlock
			if remain := p.pendingRender - rendered; remain < n {
				n = remain
			}
			p.sequencer.Render(buf[:n], buf[:n])
			rendered += n
		}
		p.renderPos = 0
		p.renderLen = 0
		p.pendingRender = -1
	}

	for i := range samples {
		if p.renderPos >= p.renderLen {
			p.renderLen = midiBlockSize
			for j := range p.renderBufL {
				p.renderBufL[j] = 0
				p.renderBufR[j] = 0
			}
			p.sequencer.Render(p.renderBufL, p.renderBufR)
			p.renderPos = 0
		}
		samples[i][0] = float64(p.renderBufL[p.renderPos])
		samples[i][1] = float64(p.renderBufR[p.renderPos])
		if samples[i][0] > 0.99 {
			samples[i][0] = 0.99
		} else if samples[i][0] < -0.99 {
			samples[i][0] = -0.99
		}
		if samples[i][1] > 0.99 {
			samples[i][1] = 0.99
		} else if samples[i][1] < -0.99 {
			samples[i][1] = -0.99
		}
		p.renderPos++
	}
	p.elapsed += time.Duration(len(samples)) * time.Second / midiSampleRate
	return len(samples), true
}

func (p *MidiPlayer) Err() error { return nil }

func (p *MidiPlayer) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.isPaused = false
	p.isPlaying = true
	return nil
}

func (p *MidiPlayer) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.isPaused = true
}

func (p *MidiPlayer) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.isPaused = true
	p.isPlaying = false
	p.sequencer.Stop()
	p.renderPos = 0
	p.renderLen = 0
	p.elapsed = 0
}

func (p *MidiPlayer) Toggle() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isPaused || !p.isPlaying {
		p.isPaused = false
		p.isPlaying = true
	} else {
		p.isPaused = true
	}
}

func (p *MidiPlayer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.sequencer.Stop()
	return nil
}

func (p *MidiPlayer) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isPlaying && !p.isPaused
}

func (p *MidiPlayer) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.elapsed
}

func (p *MidiPlayer) Duration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.duration
}

func (p *MidiPlayer) SetVolume(vol float64) {}

func (p *MidiPlayer) Volume() float64 { return 1.0 }

func (p *MidiPlayer) Format() beep.Format {
	return beep.Format{SampleRate: midiSampleRate, NumChannels: 2, Precision: 4}
}

func (p *MidiPlayer) Path() string { return "" }

func (p *MidiPlayer) Title() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.title
}

func (p *MidiPlayer) Artist() string { return "" }

func (p *MidiPlayer) Seek(pos time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	targetSample := int(pos.Seconds() * midiSampleRate)
	if targetSample < 0 {
		targetSample = 0
	}

	// Reset sequencer and defer rendering to Stream (background goroutine)
	p.sequencer.Stop()
	p.synthesizer.Reset()
	p.sequencer = meltysynth.NewMidiFileSequencer(p.synthesizer)
	p.sequencer.Play(p.midiFile, false)

	p.renderPos = 0
	p.renderLen = 0
	p.elapsed = pos
	p.pendingRender = targetSample
	return nil
}

func (p *MidiPlayer) Open(path string) error {
	return fmt.Errorf("midi player does not support Open, use NewMidiPlayer")
}
