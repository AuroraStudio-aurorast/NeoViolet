package synth

import (
	"errors"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/midi"
)

// Play resumes playback, rebuilding the initial renderer after Stop or EOF.
func (p *MidiPlayer) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errors.New("MIDI player closed")
	}
	if p.renderErr != nil {
		return p.renderErr
	}
	if !p.pending && (p.renderer == nil || p.finished) {
		r, err := p.factory()
		if err != nil {
			return err
		}
		p.renderer = r
		p.position = 0
	}
	p.isPlaying = true
	p.isPaused = false
	p.finished = false
	if p.checkpoints != nil {
		p.indexing = true
		p.wakeWorkerLocked()
	}
	return nil
}

// Pause freezes output without changing the synthesizer state.
func (p *MidiPlayer) Pause() { p.mu.Lock(); defer p.mu.Unlock(); p.isPaused = true }

// Toggle switches the current playback intent.
func (p *MidiPlayer) Toggle() {
	if p.IsPlaying() {
		p.Pause()
	} else {
		_ = p.Play()
	}
}

// ClippedSamples reports scalar output samples limited after user gain.
func (p *MidiPlayer) ClippedSamples() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.clippedSamples
}

// ReusableStream reports that EOF/Stop retain the registered silent stream.
func (p *MidiPlayer) ReusableStream() bool { return true }

// Position returns consumed frames or the newest pending seek target.
func (p *MidiPlayer) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	// #nosec G115 -- player sample rate was validated by the MIDI constructor.
	return midi.FramesDuration(p.position, int32(p.sampleRate))
}

// Duration includes the bounded release window.
func (p *MidiPlayer) Duration() time.Duration { p.mu.Lock(); defer p.mu.Unlock(); return p.duration }

// IsPlaying reports playback intent, including pending seeks.
func (p *MidiPlayer) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.closed && p.renderErr == nil && p.isPlaying && !p.isPaused
}

// Stop cancels pending seeks and drops all playback state.
func (p *MidiPlayer) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelSeekLocked()
	p.indexing = false
	if p.seekWake != nil {
		select {
		case p.seekWake <- struct{}{}:
		default:
		}
	}
	p.baseStop()
	p.renderer = nil
	p.position = 0
	p.renderErr = nil
}

// Close cancels rebuilding and waits outside the callback mutex; it is idempotent.
func (p *MidiPlayer) Close() error {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		p.isPlaying = false
		p.cancelSeekLocked()
		if p.workerStop != nil {
			close(p.workerStop)
		}
	}
	done := p.workerDone
	p.mu.Unlock()
	if done != nil {
		<-done
	}
	p.mu.Lock()
	p.renderer = nil
	p.factory = nil
	p.checkpoints = nil
	p.mu.Unlock()
	return nil
}
