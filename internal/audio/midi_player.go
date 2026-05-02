package audio

import (
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/jpodeszfa/go-meltysynth/meltysynth"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

const midiBlockSize = 512

type MidiPlayer struct {
	mu sync.Mutex

	synthesizer *meltysynth.Synthesizer
	sequencer   *meltysynth.MidiFileSequencer
	midiFile    *meltysynth.MidiFile
	sampleRate  beep.SampleRate

	renderBufL []float32
	renderBufR []float32
	renderPos  int
	renderLen  int

	seekTarget int
	seeking    bool

	duration    time.Duration
	elapsed     time.Duration
	isPlaying   bool
	isPaused    bool
	closed      bool
	finished    bool
	volumeScale float64
}

func NewMidiPlayer(midiPath, sfPath string, soundFont *meltysynth.SoundFont, sampleRate beep.SampleRate) (*MidiPlayer, *meltysynth.SoundFont, error) {
	logger.Info("Creating MIDI player", "sampleRate", sampleRate)
	var sf *meltysynth.SoundFont
	if soundFont != nil {
		sf = soundFont
	} else {
		sfFile, err := os.Open(sfPath)
		if err != nil {
			return nil, nil, fmt.Errorf("open soundfont: %w", err)
		}
		sf, err = meltysynth.NewSoundFont(sfFile)
		sfFile.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("load soundfont: %w", err)
		}
	}

	settings := meltysynth.NewSynthesizerSettings(int32(sampleRate))
	synthesizer, err := meltysynth.NewSynthesizer([]*meltysynth.SoundFont{sf}, settings)
	if err != nil {
		return nil, nil, fmt.Errorf("create synthesizer: %w", err)
	}
	synthesizer.MasterVolume = 1.0

	midFile, err := os.Open(midiPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open midi file: %w", err)
	}
	midiFile, err := meltysynth.NewMidiFile(midFile)
	midFile.Close()
	if err != nil {
		return nil, nil, fmt.Errorf("parse midi file: %w", err)
	}

	sequencer := meltysynth.NewMidiFileSequencer(synthesizer)
	sequencer.Play(midiFile, false)

	return &MidiPlayer{
		synthesizer: synthesizer,
		sequencer:   sequencer,
		midiFile:    midiFile,
		sampleRate:  sampleRate,
		duration:    midiFile.GetLength(),
		renderBufL:  make([]float32, midiBlockSize),
		renderBufR:  make([]float32, midiBlockSize),
		isPaused:    true,
		volumeScale: 1.0,
	}, sf, nil
}

func softClip(x float64) float64 {
	if x > 3 {
		return 1.0
	}
	if x < -3 {
		return -1.0
	}
	return math.Tanh(x)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (p *MidiPlayer) Stream(samples [][2]float64) (n int, ok bool) {
	p.mu.Lock()

	if p.closed || p.finished {
		p.mu.Unlock()
		return 0, false
	}
	if p.isPaused && !p.seeking {
		p.mu.Unlock()
		for i := range samples {
			samples[i] = [2]float64{}
		}
		return len(samples), true
	}

	vs := p.volumeScale
	elapsed := p.elapsed
	duration := p.duration

	if p.seeking {
		p.handleSeek(samples, vs, &elapsed, duration)
		return len(samples), true
	}

	p.mu.Unlock()

	p.fillSamples(samples, vs, &elapsed, duration)

	p.mu.Lock()
	p.elapsed = elapsed
	p.mu.Unlock()

	return len(samples), true
}

func (p *MidiPlayer) handleSeek(samples [][2]float64, vs float64, elapsed *time.Duration, duration time.Duration) {
	target := p.seekTarget
	p.seeking = false
	p.mu.Unlock()

	p.sequencer.Stop()
	p.synthesizer.Reset()
	newSeq := meltysynth.NewMidiFileSequencer(p.synthesizer)
	newSeq.Play(p.midiFile, false)

	seekBlock := int(p.sampleRate)
	bufL := make([]float32, seekBlock)
	bufR := make([]float32, seekBlock)
	for rendered := 0; rendered < target; {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return
		}
		if p.seeking {
			target = p.seekTarget
			p.seeking = false
			p.mu.Unlock()
			p.sequencer.Stop()
			p.synthesizer.Reset()
			newSeq = meltysynth.NewMidiFileSequencer(p.synthesizer)
			newSeq.Play(p.midiFile, false)
			rendered = 0
			continue
		}
		p.mu.Unlock()

		blk := seekBlock
		if remain := target - rendered; remain < blk {
			blk = remain
		}
		newSeq.Render(bufL[:blk], bufR[:blk])
		rendered += blk
	}

	p.mu.Lock()
	p.sequencer = newSeq
	p.renderPos = 0
	p.renderLen = 0
	if p.seeking || p.isPaused {
		p.mu.Unlock()
		for i := range samples {
			samples[i] = [2]float64{}
		}
		return
	}
	p.mu.Unlock()

	p.fillSamples(samples, vs, elapsed, duration)

	p.mu.Lock()
	p.elapsed = *elapsed
	p.mu.Unlock()
}

func (p *MidiPlayer) fillSamples(samples [][2]float64, vs float64, elapsed *time.Duration, duration time.Duration) {
	for i := range samples {
		if *elapsed >= duration {
			p.mu.Lock()
			p.finished = true
			p.isPlaying = false
			p.mu.Unlock()
			for j := i; j < len(samples); j++ {
				samples[j] = [2]float64{}
			}
			return
		}

		if p.renderPos >= p.renderLen {
			for j := range p.renderBufL {
				p.renderBufL[j] = 0
				p.renderBufR[j] = 0
			}
			p.sequencer.Render(p.renderBufL, p.renderBufR)
			p.renderPos = 0
			p.renderLen = midiBlockSize
		}
		l := float64(p.renderBufL[p.renderPos]) * vs
		r := float64(p.renderBufR[p.renderPos]) * vs
		p.renderPos++

		samples[i][0] = softClip(l)
		samples[i][1] = softClip(r)
		*elapsed += time.Second / time.Duration(p.sampleRate)
	}
}

func (p *MidiPlayer) Err() error { return nil }

func (p *MidiPlayer) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	logger.Debug("MIDI player play")
	p.isPaused = false
	p.isPlaying = true
	p.finished = false
	return nil
}

func (p *MidiPlayer) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	logger.Debug("MIDI player pause")
	p.isPaused = true
}

func (p *MidiPlayer) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	logger.Debug("MIDI player stop")
	p.isPaused = true
	p.isPlaying = false
	p.finished = false
	p.seeking = false
	p.elapsed = 0
}

func (p *MidiPlayer) Toggle() {
	p.mu.Lock()
	defer p.mu.Unlock()
	logger.Debug("MIDI player toggle", "wasPaused", p.isPaused)
	if p.isPaused || !p.isPlaying {
		p.isPaused = false
		p.isPlaying = true
		p.finished = false
	} else {
		p.isPaused = true
	}
}

func (p *MidiPlayer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	logger.Debug("MIDI player close")
	p.closed = true
	p.seeking = false
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

func (p *MidiPlayer) SetVolume(vol float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volumeScale = clamp(vol, 0, 1)
}

func (p *MidiPlayer) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.volumeScale
}

func (p *MidiPlayer) Format() beep.Format {
	return beep.Format{SampleRate: p.sampleRate, NumChannels: 2, Precision: 4}
}

func (p *MidiPlayer) Path() string { return "" }

func (p *MidiPlayer) Artist() string { return "" }

func (p *MidiPlayer) Seek(pos time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	targetSample := int(pos.Seconds() * float64(p.sampleRate))
	if targetSample < 0 {
		targetSample = 0
	}

	logger.Debug("MIDI seek", "position", pos, "targetSample", targetSample)

	p.seekTarget = targetSample
	p.seeking = true
	p.elapsed = pos
	p.finished = false
	return nil
}

func (p *MidiPlayer) Open(path string) error {
	return fmt.Errorf("midi player does not support Open, use NewMidiPlayer")
}
