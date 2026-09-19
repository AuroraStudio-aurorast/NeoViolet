package apestream

import (
	"runtime"
	"testing"
	"time"
)

// The decoder backends run a subprocess plus a reader goroutine, so these tests
// cover the concurrency contracts the streamer has to keep: playback must reach
// the end of the file, Seek and Close must work while Stream is running, and
// Position must be readable from another goroutine. Run with -race.

// streamUntilStopped pulls samples like the beep speaker does and reports once
// it is done, either because the stream ended or the caller closed stop.
func streamUntilStopped(s *Streamer, stop <-chan struct{}) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([][2]float64, 512)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, ok := s.Stream(buf); !ok {
				return
			}
		}
	}()
	return done
}

func openTestStreamer(t *testing.T, name string) *Streamer {
	t.Helper()
	s, _, err := Decode(nil, testFile(name))
	if err != nil {
		t.Skipf("no APE backend available (install ffmpeg or mac, or build apecli): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestStreamReachesEndOfFile guards the reader-goroutine lifetime: when the
// decoder stops producing samples, Stream must return instead of blocking on a
// channel nobody will ever send on again.
func TestStreamReachesEndOfFile(t *testing.T) {
	s := openTestStreamer(t, "test_ape.ape")

	type result struct{ frames int }
	done := make(chan result, 1)
	go func() {
		buf := make([][2]float64, 512)
		frames := 0
		for {
			n, ok := s.Stream(buf)
			frames += n
			if !ok {
				done <- result{frames: frames}
				return
			}
		}
	}()

	select {
	case r := <-done:
		t.Logf("decoded %d frames, streamer reports %d", r.frames, s.Len())
		if missing := s.Len() - r.frames; missing < 0 || missing > pipeBufFrames {
			t.Errorf("decoded %d frames, want %d (±%d)", r.frames, s.Len(), pipeBufFrames)
		}
	case <-time.After(20 * time.Second):
		buf := make([]byte, 1<<16)
		t.Fatalf("Stream never returned at end of file:\n%s", buf[:runtime.Stack(buf, true)])
	}
}

// TestSeekDuringStream seeks while another goroutine is streaming, the way the
// player does when the user drags the seek bar.
func TestSeekDuringStream(t *testing.T) {
	s := openTestStreamer(t, "test_ape.ape")

	stop := make(chan struct{})
	done := streamUntilStopped(s, stop)

	const target = 22050 // halfway through the fixture
	for i := 0; i < 5; i++ {
		if err := s.Seek(target); err != nil {
			t.Fatalf("Seek(%d): %v", target, err)
		}
		// Streaming advances the position concurrently, so allow one decoder
		// read of slack on the upper bound.
		if got := s.Position(); got < target || got > target+pipeBufFrames {
			t.Errorf("Position() = %d after Seek(%d), want between %d and %d",
				got, target, target, target+pipeBufFrames)
		}
	}

	close(stop)
	<-done
}

// TestPositionDuringStream reads Position from another goroutine, as the TUI
// does while drawing the progress bar.
func TestPositionDuringStream(t *testing.T) {
	s := openTestStreamer(t, "test_ape.ape")

	stop := make(chan struct{})
	done := streamUntilStopped(s, stop)
	defer func() {
		close(stop)
		<-done
	}()

	deadline := time.Now().Add(5 * time.Second)
	last := 0
	for last == 0 && time.Now().Before(deadline) {
		last = s.Position()
	}
	if last == 0 {
		t.Fatal("Position() never advanced while streaming")
	}

	for i := 0; i < 500; i++ {
		if pos := s.Position(); pos < last {
			t.Fatalf("Position() went backwards: %d then %d", last, pos)
		} else {
			last = pos
		}
	}
}

// TestCloseDuringStream closes the streamer while it is streaming. Close owns
// the decoder subprocess, so it must return and release the reader goroutine.
func TestCloseDuringStream(t *testing.T) {
	s := openTestStreamer(t, "test_ape.ape")

	stop := make(chan struct{})
	defer close(stop)
	done := streamUntilStopped(s, stop)

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stream did not return after Close")
	}

	// Close is idempotent, and a closed streamer reports end of stream.
	if err := s.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if n, ok := s.Stream(make([][2]float64, 16)); ok || n != 0 {
		t.Errorf("Stream on closed streamer = %d, %v; want 0, false", n, ok)
	}
}
