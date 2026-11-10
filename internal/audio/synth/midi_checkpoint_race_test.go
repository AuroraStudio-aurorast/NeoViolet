package synth

import (
	"sync"
	"testing"
	"time"
)

func TestMidiCheckpointSeekPreemptsIndexAndStop(t *testing.T) {
	p := checkpointPlayer(t)
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	// No wait for a completed index: requests race the actual background owner.
	for _, pos := range []time.Duration{11 * time.Second, 3 * time.Second, 12 * time.Second, 7 * time.Second} {
		if err := p.Seek(pos); err != nil {
			t.Fatal(err)
		}
	}
	p.mu.Lock()
	done := p.seekReady
	p.mu.Unlock()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("index starved seek")
	}
	if p.Position() != 7*time.Second {
		t.Fatal("obsolete indexed state published")
	}
	p.Stop()
	seekReady(t, p, 11*time.Second)
	p.mu.Lock()
	replayed := p.seekReplayFrames
	p.mu.Unlock()
	if replayed > 6*44100 {
		t.Fatal("prior replay did not build reusable checkpoints")
	}
	p.Stop()
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if p.Position() != 0 || p.CheckpointStats().Count != 0 || p.IsPlaying() {
		t.Fatal("stop/close retained active state")
	}
}

func TestMidiCheckpointConcurrentControlsAndClose(t *testing.T) {
	p := checkpointPlayer(t)
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for lane := range 3 {
		wg.Go(func() {
			for step := range 12 {
				switch lane {
				case 0:
					_ = p.Seek(time.Duration(step) * time.Second)
				case 1:
					p.Pause()
					_ = p.Play()
				case 2:
					p.Stream(make([][2]float64, 65))
					_ = p.CheckpointStats()
				}
			}
		})
	}
	wg.Wait()
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if p.CheckpointStats().Bytes != 0 || p.IsPlaying() {
		t.Fatal("closed index retained resources")
	}
}
