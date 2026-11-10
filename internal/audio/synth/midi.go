package synth

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gopxl/beep/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/midi"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/soundfont"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

const midiBlockSize = 512

type midiRenderer interface {
	Render([]float32, []float32) (int, error)
	PositionFrames() int64
}
type rendererFactory func() (midiRenderer, error)

// MidiPlayer serializes live DSP access; background seek has its own renderer.
type MidiPlayer struct {
	baseSynth
	factory                rendererFactory
	renderer               midiRenderer
	position, total        int64
	renderBufL, renderBufR [midiBlockSize]float32
	renderErr              error
	clippedSamples         uint64
	pending                bool
	seekGeneration         uint64
	seekTarget             int64
	seekCancel             chan struct{}
	seekReady              chan struct{}
	seekWake               chan struct{}
	workerStop             chan struct{}
	workerDone             chan struct{}
	checkpoints            *midi.Checkpoints
	indexing               bool
	seekReplayFrames       int64
}

func newMIDIPlayer(factory rendererFactory, total int64, rate beep.SampleRate) (*MidiPlayer, error) {
	r, err := factory()
	if err != nil {
		return nil, err
	}
	p := &MidiPlayer{factory: factory, renderer: r, total: total}
	p.sampleRate = rate
	p.isPaused = true
	p.volumeScale = 1
	// #nosec G115 -- sample rate is validated to 16000..192000 before constructing the player.
	p.duration = midi.FramesDuration(total, int32(rate))
	if renderer, ok := r.(*midi.Renderer); ok {
		initial := renderer.Clone()
		p.factory = func() (midiRenderer, error) { return initial.Clone(), nil }
		// #nosec G115 -- sample rate validated by the MIDI constructor.
		p.checkpoints = midi.NewCheckpoints(int32(rate), 64<<20)
	}
	return p, nil
}

// NewMidiPlayer creates a MIDI player using an immutable shared SoundFont.
func NewMidiPlayer(midiPath, sfPath string, sf *soundfont.SoundFont, rate beep.SampleRate) (*MidiPlayer, *soundfont.SoundFont, error) {
	if rate < 16000 || rate > 192000 {
		return nil, nil, errors.New("unsupported MIDI sample rate")
	}
	if sf == nil {
		// #nosec G304,G703 -- opening the local SoundFont explicitly selected by the user is intentional.
		f, err := os.Open(sfPath)
		if err != nil {
			return nil, nil, fmt.Errorf("open soundfont: %w", err)
		}
		var diags []soundfont.Diagnostic
		sf, diags, err = soundfont.Load(f)
		_ = f.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("load soundfont: %w", err)
		}
		if len(diags) > 0 {
			logger.Info("SoundFont compatibility diagnostics", "items", diags)
		}
	}
	// #nosec G304,G703 -- opening the local MIDI file explicitly selected by the user is intentional.
	f, err := os.Open(midiPath)
	if err != nil {
		return nil, nil, err
	}
	song, diags, err := midi.Parse(f)
	_ = f.Close()
	if err != nil {
		return nil, nil, fmt.Errorf("parse midi: %w", err)
	}
	if len(diags) > 0 {
		logger.Info("MIDI compatibility diagnostics", "items", diags)
	}
	tl, err := song.Compile(int32(rate))
	if err != nil {
		return nil, nil, err
	}
	p, err := newMIDIPlayer(func() (midiRenderer, error) { return midi.NewRenderer(tl, sf) }, tl.TotalFrames, rate)
	if err == nil {
		if renderer, ok := p.renderer.(*midi.Renderer); ok {
			stats := renderer.Stats()
			if len(stats.Diagnostics) > 0 {
				logger.Info("MIDI render compatibility diagnostics", "items", stats.Diagnostics)
			}
		}
	}
	return p, sf, err
}

// Stream holds the state mutex for at most 512 output frames. It never replays
// a seek prefix, waits for a worker or reads files on the audio callback.
func (p *MidiPlayer) Stream(samples [][2]float64) (int, bool) {
	for offset := 0; offset < len(samples); {
		p.mu.Lock()
		if p.closed || p.renderErr != nil {
			p.mu.Unlock()
			return offset, false
		}
		count := min(len(samples)-offset, midiBlockSize)
		out := samples[offset : offset+count]
		clear(out)
		if !p.pending && !p.isPaused && p.isPlaying && p.renderer != nil {
			n, err := p.renderer.Render(p.renderBufL[:count], p.renderBufR[:count])
			for i := 0; i < n; i++ {
				l, r := float64(p.renderBufL[i])*p.volumeScale, float64(p.renderBufR[i])*p.volumeScale
				if l < -1 || l > 1 {
					p.clippedSamples++
				}
				if r < -1 || r > 1 {
					p.clippedSamples++
				}
				out[i][0] = clamp(l, -1, 1)
				out[i][1] = clamp(r, -1, 1)
			}
			p.position = p.renderer.PositionFrames()
			if err != nil && err != io.EOF {
				p.renderErr = err
				p.isPlaying = false
			}
			if err == io.EOF || p.position >= p.total {
				p.position = p.total
				p.finished = true
				p.isPlaying = false
			}
		}
		p.mu.Unlock()
		offset += count
	}
	return len(samples), true
}

// Err reports the last asynchronous rendering error.
func (p *MidiPlayer) Err() error { p.mu.Lock(); defer p.mu.Unlock(); return p.renderErr }

// Streamer returns the reusable MIDI output stream.
func (p *MidiPlayer) Streamer() Streamer { return p }

// Path is empty because the enclosing Player owns the file identity.
func (p *MidiPlayer) Path() string { return "" }

// Open is unsupported; construct a MIDI player with NewMidiPlayer.
func (p *MidiPlayer) Open(string) error { return errors.New("use NewMidiPlayer") }

// SetTitle sets the display title.
func (p *MidiPlayer) SetTitle(s string) { p.mu.Lock(); defer p.mu.Unlock(); p.title = s }

// SetArtist sets the display artist.
func (p *MidiPlayer) SetArtist(s string) { p.mu.Lock(); defer p.mu.Unlock(); p.artist = s }
