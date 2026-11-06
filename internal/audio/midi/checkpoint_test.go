package midi

import (
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

func TestRendererCheckpointExactContinuation(t *testing.T) {
	a := append(miditest.Event(0, 0x90, 60, 100), miditest.Event(100, 0xB0, 64, 127)...)
	a = append(a, miditest.Event(50, 0x80, 60, 0)...)
	a = append(a, miditest.Event(200, 0xB0, 64, 0)...)
	a = append(a, miditest.Event(100, 255, 47, 0)...)
	b := append(miditest.Event(0, 255, 33, 1, 1), miditest.Event(0, 0x90, 67, 110)...)
	b = append(b, miditest.Event(200, 0xE0, 70, 80)...)
	b = append(b, miditest.Event(250, 0x80, 67, 0)...)
	b = append(b, miditest.Event(0, 255, 47, 0)...)
	r, err := NewRenderer(timeline(t, a, b), testFont(t))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = r.Render(make([]float32, 30001), make([]float32, 30001))
	c := r.Clone()
	for range 10 {
		l, rr, x, y := make([]float32, 4097), make([]float32, 4097), make([]float32, 4097), make([]float32, 4097)
		n, e := r.Render(l, rr)
		m, f := c.Render(x, y)
		if n != m || e != f {
			t.Fatal("checkpoint changed scheduler/EOF")
		}
		for i := range n {
			if l[i] != x[i] || rr[i] != y[i] {
				t.Fatalf("port checkpoint changed PCM frame%d", i)
			}
		}
	}
}

func TestCheckpointCacheNearestAndBudget(t *testing.T) {
	tr := append(miditest.Event(0, 0x90, 60, 100), miditest.Event(10000, 0x80, 60, 0)...)
	tr = append(tr, miditest.Event(0, 255, 47, 0)...)
	r, err := NewRenderer(timeline(t, tr), testFont(t))
	if err != nil {
		t.Fatal(err)
	}
	cost := r.StateBytes()
	cache := NewCheckpoints(r.timeline.SampleRate, cost*3)
	if cache.Restore(100) != nil {
		t.Fatal("unindexed target returned state")
	}
	for range 30 {
		_, _ = r.Render(make([]float32, 44100), make([]float32, 44100))
		cache.Capture(r)
		stats := cache.Stats()
		if stats.Bytes > stats.Budget || stats.Count > 3 {
			t.Fatal("checkpoint budget exceeded")
		}
	}
	if cache.Stats().Count == 0 || cache.Stats().IntervalFrames <= 5*44100 {
		t.Fatal("budget did not sparsify checkpoints")
	}
	c := cache.Restore(r.PositionFrames())
	if c == nil || c.PositionFrames() > r.PositionFrames() || c.PositionFrames() == 0 {
		t.Fatal("no usable checkpoint")
	}
	at := c.PositionFrames()
	_, _ = c.Render(make([]float32, 129), make([]float32, 129))
	if again := cache.Restore(at); again == nil || again.PositionFrames() != at {
		t.Fatal("restoring mutated cached snapshot")
	}
	tiny := NewCheckpoints(44100, 1)
	tiny.Capture(r)
	if tiny.Stats().Bytes != 0 || tiny.Restore(r.PositionFrames()) != nil {
		t.Fatal("oversized checkpoint retained")
	}
}
