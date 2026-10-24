package synth

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/midi"
)

func TestMIDIIntegrationCheckpointPlayer(t *testing.T) {
	if testing.Short() {
		t.Skip("local integration")
	}
	file, bank := os.Getenv("NEOVIOLET_MIDI_TEST_FILE"), os.Getenv("NEOVIOLET_SF2_TEST_FILE")
	if file == "" || bank == "" || os.Getenv("NEOVIOLET_MIDI_CHECKPOINTS") == "" {
		t.Skip("set local assets and NEOVIOLET_MIDI_CHECKPOINTS=1")
	}
	p, _, err := NewMidiPlayer(file, bank, nil, 44100)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if e := p.Close(); e != nil {
			t.Error(e)
		}
	}()
	start := time.Now()
	if err = p.Play(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Minute)
	for p.CheckpointStats().IndexedFrames < p.total {
		if time.Now().After(deadline) {
			t.Fatal("automatic index timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
	p.Pause()
	t.Logf("player automatic index %v cache=%+v", time.Since(start), p.CheckpointStats())
	p.mu.Lock()
	factory := p.factory
	p.mu.Unlock()
	ref, err := factory()
	if err != nil {
		t.Fatal(err)
	}
	l, r := make([]float32, 512), make([]float32, 512)
	for _, pos := range []time.Duration{0, 17552 * time.Millisecond, 24800 * time.Millisecond, 85 * time.Second, 236576 * time.Millisecond, 257552 * time.Millisecond, 267160 * time.Millisecond} {
		target := midi.DurationFrames(pos, 44100, p.total)
		for ref.PositionFrames() < target {
			n := int(min(int64(len(l)), target-ref.PositionFrames()))
			if _, err = ref.Render(l[:n], r[:n]); err != nil && err != io.EOF {
				t.Fatal(err)
			}
		}
		if _, err = ref.Render(l, r); err != nil && err != io.EOF {
			t.Fatal(err)
		}
		p.Pause()
		start = time.Now()
		seekReady(t, p, pos)
		p.mu.Lock()
		frames := p.seekReplayFrames
		p.mu.Unlock()
		elapsed := time.Since(start)
		if err = p.Play(); err != nil {
			t.Fatal(err)
		}
		out := make([][2]float64, 512)
		p.Stream(out)
		for i := range out {
			if out[i][0] != clamp(float64(l[i]), -1, 1) || out[i][1] != clamp(float64(r[i]), -1, 1) {
				t.Fatalf("player checkpoint mismatch target%v frame%d", pos, i)
			}
		}
		t.Logf("player seek %v cost=%v replay=%v", pos, elapsed, midi.FramesDuration(frames, 44100))
	}
}
