package synth

import (
	"errors"
	"io"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/midi"
)

// cancelSeekLocked invalidates every draft before dropping state. Never join
// a worker with mu held: publication and cancellation need the same mutex.
func (p *MidiPlayer) cancelSeekLocked() {
	p.seekGeneration++
	if p.seekCancel != nil {
		close(p.seekCancel)
		p.seekCancel = nil
	}
	if p.pending && p.seekReady != nil {
		close(p.seekReady)
	}
	p.pending = false
}

// Seek requests a bounded, cancellable background rebuild; the newest request wins.
func (p *MidiPlayer) Seek(pos time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errors.New("MIDI player closed")
	}
	p.cancelSeekLocked()
	// #nosec G115 -- player sample rate was validated by the MIDI constructor.
	p.seekTarget = midi.DurationFrames(pos, int32(p.sampleRate), p.total)
	p.position = p.seekTarget
	p.finished = false
	p.renderErr = nil
	p.pending = true
	p.seekCancel = make(chan struct{})
	p.seekReady = make(chan struct{})
	p.wakeWorkerLocked()
	return nil
}

// seekWorker is the sole background rendering owner. It indexes at low priority
// between requests; a seek preempts indexing at the next 512-frame boundary.
func (p *MidiPlayer) seekWorker() {
	defer close(p.workerDone)
	var index *midi.Renderer
	blocks := 0
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return
		}
		cache, factory, total := p.checkpoints, p.factory, p.total
		if p.pending {
			gen, target, cancel := p.seekGeneration, p.seekTarget, p.seekCancel
			p.mu.Unlock()
			p.rebuildSeek(gen, target, cancel, factory, cache)
			// A seek may have advanced the cache much farther than the old indexer.
			index = nil
			continue
		}
		indexing := p.indexing && cache != nil
		p.mu.Unlock()
		if !indexing || cache.Stats().IndexedFrames >= total {
			index = nil
			select {
			case <-p.workerStop:
				return
			case <-p.seekWake:
			}
			continue
		}
		if index == nil {
			index = cache.Restore(total)
			if index == nil {
				r, err := factory()
				if err != nil {
					p.mu.Lock()
					p.indexing = false
					p.mu.Unlock()
					continue
				}
				index, _ = r.(*midi.Renderer)
				if index == nil {
					p.mu.Lock()
					p.indexing = false
					p.mu.Unlock()
					continue
				}
			}
		}
		var left, right [midiBlockSize]float32
		n, err := index.Render(left[:], right[:])
		if (err != nil && err != io.EOF) || n == 0 {
			p.mu.Lock()
			p.indexing = false
			p.mu.Unlock()
			index = nil
			continue
		}
		cache.Capture(index)
		blocks++
		if blocks%32 == 0 {
			// Yield CPU to live playback, but wake immediately for seeks and Close.
			timer := time.NewTimer(time.Millisecond)
			select {
			case <-p.workerStop:
				timer.Stop()
				return
			case <-p.seekWake:
				timer.Stop()
			case <-timer.C:
			}
		}
	}
}

func (p *MidiPlayer) rebuildSeek(gen uint64, target int64, cancel <-chan struct{}, factory rendererFactory, cache *midi.Checkpoints) {
	var draft midiRenderer
	if cache != nil {
		if r := cache.Restore(target); r != nil {
			draft = r
		}
	}
	var err error
	if draft == nil {
		draft, err = factory()
	}
	var left, right [midiBlockSize]float32
	var replayed int64
	cancelled := false
	for err == nil && draft.PositionFrames() < target {
		select {
		case <-cancel:
			cancelled = true
		default:
		}
		if cancelled {
			break
		}
		count := int(min(int64(midiBlockSize), target-draft.PositionFrames()))
		n, e := draft.Render(left[:count], right[:count])
		replayed += int64(n)
		if e != nil && e != io.EOF {
			err = e
			break
		}
		if n == 0 {
			err = io.ErrNoProgress
			break
		}
		if cache != nil {
			if r, ok := draft.(*midi.Renderer); ok {
				cache.Capture(r)
			}
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || gen != p.seekGeneration || !p.pending || cancelled {
		return
	}
	p.seekReplayFrames = replayed
	if err != nil {
		p.renderErr = err
		p.isPlaying = false
		p.renderer = nil
	} else {
		p.renderer = draft
		p.position = draft.PositionFrames()
		p.finished = p.position >= p.total
		if p.finished {
			p.isPlaying = false
		}
	}
	p.pending = false
	close(p.seekReady)
}
