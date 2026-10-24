package synth

import (
	"io"
	"sync/atomic"
	"testing"
	"time"
)

type frameRenderer struct{ pos int64 }

func (r *frameRenderer) PositionFrames() int64 { return r.pos }
func (r *frameRenderer) Render(l, rbuf []float32) (int, error) {
	for i := range l {
		l[i] = float32(r.pos+int64(i)) / 100000
		rbuf[i] = l[i]
	}
	r.pos += int64(len(l))
	return len(l), nil
}
func TestMidiSeekDoesNotBlockStream(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	p, e := newMIDIPlayer(func() (midiRenderer, error) {
		if calls.Add(1) == 2 {
			close(entered)
			<-release
		}
		return &frameRenderer{}, nil
	}, 100000, 44100)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = p.Close() }()
	_ = p.Play()
	if e = p.Seek(time.Second); e != nil {
		t.Fatal(e)
	}
	<-entered
	out := make([][2]float64, 128)
	if n, ok := p.Stream(out); n != 128 || !ok {
		t.Fatal("pending stream detached")
	}
	if p.Position() != time.Second {
		t.Fatal("requested position not visible")
	}
	for _, x := range out {
		if x != [2]float64{} {
			t.Fatal("pending old audio")
		}
	}
	close(release)
	p.mu.Lock()
	done := p.seekReady
	p.mu.Unlock()
	<-done
	if e = p.Err(); e != nil {
		t.Fatal(e)
	}
	p.Stream(out)
	if out[0][0] < 0.44 || out[0][0] > 0.45 {
		t.Fatalf("wrong resumed frame: %v", out[0])
	}
}
func TestMidiLatestSeekWins(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	p, e := newMIDIPlayer(func() (midiRenderer, error) {
		if calls.Add(1) == 2 {
			close(entered)
			<-release
		}
		return &frameRenderer{}, nil
	}, 100000, 44100)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = p.Close() }()
	_ = p.Play()
	_ = p.Seek(500 * time.Millisecond)
	<-entered
	_ = p.Seek(time.Second)
	_ = p.Seek(1500 * time.Millisecond)
	p.mu.Lock()
	done := p.seekReady
	p.mu.Unlock()
	close(release)
	<-done
	if p.Position() != 1500*time.Millisecond {
		t.Fatalf("old seek overwrote target %v", p.Position())
	}
	b := make([][2]float64, 1)
	p.Stream(b)
	if b[0][0] < 0.66 || b[0][0] > 0.67 {
		t.Fatalf("wrong target audio %v", b)
	}
	if calls.Load() != 3 {
		t.Fatalf("queued obsolete targets rendered: %d", calls.Load())
	}
}
func TestMidiSeekMatchesLinearRender(t *testing.T) {
	a, b := testMIDIPlayer(t), testMIDIPlayer(t)
	_ = a.Play()
	a.Stream(make([][2]float64, 12345))
	want := make([][2]float64, 4096)
	a.Stream(want)
	// Round up sub-frame duration so conversion truncates to precisely frame12345.
	pos := time.Duration((int64(12345)*int64(time.Second) + 44099) / 44100)
	if e := b.Seek(pos); e != nil {
		t.Fatal(e)
	}
	b.mu.Lock()
	done := b.seekReady
	b.mu.Unlock()
	<-done
	_ = b.Play()
	got := make([][2]float64, 4096)
	b.Stream(got)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("seek mismatch frame%d", i)
		}
	}
}
func TestMidiStopCloseCancelSeek(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	p, e := newMIDIPlayer(func() (midiRenderer, error) {
		if calls.Add(1) == 2 {
			close(entered)
			<-release
		}
		return &frameRenderer{}, nil
	}, 100000, 44100)
	if e != nil {
		t.Fatal(e)
	}
	_ = p.Seek(time.Second)
	<-entered
	p.Stop()
	close(release)
	if e = p.Close(); e != nil {
		t.Fatal(e)
	}
	_ = p.Close()
	if p.Position() != 0 || p.IsPlaying() {
		t.Fatal("cancelled state revived")
	}
	if p.Seek(0) == nil || p.Play() == nil {
		t.Fatal("closed control accepted")
	}
	if n, ok := p.Stream(make([][2]float64, 1)); n != 0 || ok {
		t.Fatal("closed stream retained")
	}
}

var _ midiRenderer = (*frameRenderer)(nil)
var _ = io.EOF
