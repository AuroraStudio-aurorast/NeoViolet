package audio

import (
	"os"
	"testing"
	"time"

	"github.com/gopxl/beep/v2"
)

func TestIsSyntheticFormat(t *testing.T) {
	// Core tracker/MIDI formats are always synthetic; openmpt-only extensions
	// (.mptm etc.) depend on build tags, so they are not asserted here.
	for _, ext := range []string{".mid", ".midi", ".mod", ".xm", ".s3m", ".it"} {
		if !IsSyntheticFormat(ext) {
			t.Errorf("IsSyntheticFormat(%q) = false, want true", ext)
		}
	}
	for _, ext := range []string{".mp3", ".flac", ".wav", ".ogg", ".ape"} {
		if IsSyntheticFormat(ext) {
			t.Errorf("IsSyntheticFormat(%q) = true, want false", ext)
		}
	}
}

func TestPlayerPlayPauseResume(t *testing.T) {
	if os.Getenv("TEST_AUDIO") == "" {
		t.Skip("Set TEST_AUDIO=1 and provide file via TEST_FILE to run audio tests")
	}
	filePath := os.Getenv("TEST_FILE")
	if filePath == "" {
		t.Fatal("TEST_FILE env var required")
	}

	p := NewPlayer()
	if p.IsPlaying() {
		t.Error("NewPlayer should not be playing")
	}

	if err := p.Open(filePath); err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if p.IsPlaying() {
		t.Error("After Open, player should not be playing yet")
	}

	if err := p.Play(); err != nil {
		t.Fatalf("Play failed: %v", err)
	}

	if !p.IsPlaying() {
		t.Error("After Play, player should be playing")
	}

	p.Pause()
	if p.IsPlaying() {
		t.Error("After Pause, player should not be playing")
	}

	if err := p.Play(); err != nil {
		t.Fatalf("Play (resume) failed: %v", err)
	}

	if !p.IsPlaying() {
		t.Error("After Play (resume), player should be playing")
	}

	p.Close()
	if p.IsPlaying() {
		t.Error("After Close, player should not be playing")
	}
}

func TestToggleLogic(t *testing.T) {
	p := NewPlayer()

	p.Toggle()

	if p.IsPlaying() {
		t.Error("Toggle on unopened player should be no-op")
	}
}

// fakeStream implements beep.StreamSeekCloser over a fixed-length silent buffer.
type fakeStream struct {
	pos   int
	total int
	rate  beep.SampleRate
	seeks []int // records seek positions for assertions
}

func (f *fakeStream) Stream(samples [][2]float64) (int, bool) {
	n := 0
	for i := range samples {
		if f.pos >= f.total {
			break
		}
		samples[i][0], samples[i][1] = 0, 0
		f.pos++
		n++
	}
	return n, f.pos < f.total
}

func (f *fakeStream) Err() error         { return nil }
func (f *fakeStream) Close() error       { return nil }
func (f *fakeStream) Len() int           { return f.total }
func (f *fakeStream) Position() int      { return f.pos }
func (f *fakeStream) Seek(pos int) error { f.seeks = append(f.seeks, pos); f.pos = pos; return nil }

func newFakePlayer(totalSamples int) (*Player, *fakeStream) {
	p := NewPlayer()
	f := &fakeStream{total: totalSamples, rate: beep.SampleRate(44100)}
	fm := beep.Format{SampleRate: 44100, NumChannels: 2, Precision: 2}
	p.setupStreamer(f, fm, nopCloser{}, "fake.wav", f)
	return p, f
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func TestPlayerVolumeClamping(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"zero", 0, 0},
		{"one", 1, 1},
		{"mid", 0.5, 0.5},
		{"negative clamped", -1, 0},
		{"over one clamped", 2, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewPlayer()
			p.SetVolume(tc.in)
			if got := p.Volume(); got != tc.want {
				t.Errorf("SetVolume(%v) -> Volume() = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestPlayerSetters(t *testing.T) {
	p := NewPlayer()
	p.SetSoundfontPath("/tmp/x.sf2")
	p.SetTrackerBackend("gotracker")
	if p.sfPath != "/tmp/x.sf2" {
		t.Errorf("sfPath = %q", p.sfPath)
	}
	if p.trackerBackend != "gotracker" {
		t.Errorf("trackerBackend = %q", p.trackerBackend)
	}
}

func TestPlayerOpenStatesViaSetupStreamer(t *testing.T) {
	p, f := newFakePlayer(44100 * 10) // 10s at 44.1kHz
	if p.Path() != "fake.wav" {
		t.Errorf("Path() = %q, want fake.wav", p.Path())
	}
	if want := time.Duration(10 * time.Second); p.Duration() != want {
		t.Errorf("Duration() = %v, want %v", p.Duration(), want)
	}
	if p.Position() != 0 {
		t.Errorf("Position() = %v, want 0", p.Position())
	}
	// Seek on unopened player must be a no-op, not an error
	if err := NewPlayer().Seek(5 * time.Second); err != nil {
		t.Errorf("Seek on unopened player: %v, want nil", err)
	}
	// Play on unopened player must return "player not initialized"
	if err := NewPlayer().Play(); err == nil {
		t.Error("Play on unopened player: want error")
	}
	_ = f
}

func TestPlayerToggleOnUnopened(t *testing.T) {
	p := NewPlayer()
	p.Toggle() // must not panic; must not start playing
	if p.IsPlaying() {
		t.Error("Toggle on unopened player should not start playback")
	}
}

func TestResampleIfNeeded(t *testing.T) {
	fm := beep.Format{SampleRate: 48000, NumChannels: 2, Precision: 2}
	// speakerSampleRate == 0 (never initialized in tests): no resample
	got := resampleIfNeeded(&fakeStream{}, fm)
	if _, ok := got.(*fakeStream); !ok {
		t.Error("resampleIfNeeded should return original streamer when speaker not initialized")
	}
}
