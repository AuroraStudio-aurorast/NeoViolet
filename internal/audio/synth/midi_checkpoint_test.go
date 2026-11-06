package synth

import (
	"bytes"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/midi"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/soundfont"
)

func checkpointPlayer(t *testing.T) *MidiPlayer {
	t.Helper()
	tr := append(miditest.Event(0, 0x90, 60, 100), miditest.Event(3000, 0x80, 60, 0)...)
	tr = append(tr, miditest.Event(0, 255, 47, 0)...)
	song, _, err := midi.Parse(bytes.NewReader(miditest.SMF(1, 120, tr)))
	if err != nil {
		t.Fatal(err)
	}
	tl, err := song.Compile(44100)
	if err != nil {
		t.Fatal(err)
	}
	sf, err := soundfont.NewSoundFont(bytes.NewReader(miditest.SF2(nil)))
	if err != nil {
		t.Fatal(err)
	}
	p, err := newMIDIPlayer(func() (midiRenderer, error) { return midi.NewRenderer(tl, sf) }, tl.TotalFrames, 44100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	})
	return p
}

func waitCheckpoint(t *testing.T, p *MidiPlayer, frame int64) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if p.CheckpointStats().IndexedFrames >= frame {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("background indexing failed to advance")
}
func seekReady(t *testing.T, p *MidiPlayer, pos time.Duration) {
	t.Helper()
	if err := p.Seek(pos); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	done := p.seekReady
	p.mu.Unlock()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("checkpoint seek blocked")
	}
	if err := p.Err(); err != nil {
		t.Fatal(err)
	}
}
func TestMidiCheckpointAutomaticIndexAndSeek(t *testing.T) {
	p := checkpointPlayer(t)
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	waitCheckpoint(t, p, 11*44100)
	if p.CheckpointStats().Count < 2 {
		t.Fatal("no automatic checkpoints")
	}
	p.Pause()
	seekReady(t, p, 11500*time.Millisecond)
	p.mu.Lock()
	replayed := p.seekReplayFrames
	p.mu.Unlock()
	if replayed >= 5*44100 {
		t.Fatalf("seek replayed the prefix: %d frames", replayed)
	}
	if p.IsPlaying() {
		t.Fatal("seek resumed paused playback")
	}
	// Use a fully independent, from-zero reference with the same controller flow.
	ref := checkpointPlayer(t)
	ref.mu.Lock()
	r := ref.renderer
	ref.mu.Unlock()
	target := int64(11500 * 44100 / 1000)
	l, rr := make([]float32, 512), make([]float32, 512)
	for r.PositionFrames() < target {
		n := int(min(int64(512), target-r.PositionFrames()))
		_, _ = r.Render(l[:n], rr[:n])
	}
	_, _ = r.Render(l, rr)
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	got := make([][2]float64, 512)
	p.Stream(got)
	for i := range got {
		if got[i][0] != float64(l[i]) || got[i][1] != float64(rr[i]) {
			t.Fatalf("checkpoint seek PCM differs frame%d", i)
		}
	}
	p.Stop()
	if p.Position() != 0 {
		t.Fatal("stop revived position")
	}
	seekReady(t, p, 11*time.Second)
	if p.CheckpointStats().Count == 0 {
		t.Fatal("stop discarded reusable cache")
	}
}
func TestMidiCheckpointZeroTemplateIsPrivate(t *testing.T) {
	p := checkpointPlayer(t)
	root, err := p.factory()
	if err != nil {
		t.Fatal(err)
	}
	initial := p.renderer.(*midi.Renderer)
	// Private reset state must be reusable after the original renderer advances.
	_, _ = initial.Render(make([]float32, 12345), make([]float32, 12345))
	fresh, err := p.factory()
	if err != nil {
		t.Fatal(err)
	}
	l, r, x, y := make([]float32, 512), make([]float32, 512), make([]float32, 512), make([]float32, 512)
	_, _ = root.Render(l, r)
	_, _ = fresh.Render(x, y)
	for i := range l {
		if l[i] != x[i] || r[i] != y[i] {
			t.Fatal("zero template advanced with playback")
		}
	}
}

func TestMidiCheckpointCloseReleasesCache(t *testing.T) {
	p := checkpointPlayer(t)
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	waitCheckpoint(t, p, 6*44100)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if p.CheckpointStats().Bytes != 0 {
		t.Fatal("closed player retained checkpoints")
	}
}
