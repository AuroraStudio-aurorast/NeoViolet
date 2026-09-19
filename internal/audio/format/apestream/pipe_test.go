package apestream

import (
	"errors"
	"io"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// TestPipeReadTimeoutAbortsWedgedDecoder covers a decoder that stops producing
// output without closing its pipe: Read must give up instead of blocking its
// caller (and, through beep's speaker lock, the whole player) forever, and the
// stuck subprocess must be reaped.
func TestPipeReadTimeoutAbortsWedgedDecoder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a subprocess that stays silent")
	}

	cmd := exec.Command("sleep", "60")
	stdout, err := startPipeCmd("sleep", cmd)
	if err != nil {
		t.Fatalf("start pipe: %v", err)
	}

	info := &StreamInfo{SampleRate: 44100, Channels: 2, BitsPerSample: 16}
	s := newPipeStream(cmd, stdout, info, 2, 2)
	s.readTimeout = 200 * time.Millisecond
	defer s.abort()

	done := make(chan error, 1)
	go func() {
		_, readErr := s.read(make([]float64, 1024))
		done <- readErr
	}()

	select {
	case readErr := <-done:
		if !errors.Is(readErr, io.EOF) {
			t.Errorf("read from a stalled decoder = %v, want io.EOF", readErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("read did not return after the decoder stalled")
	}

	if s.cmd.ProcessState == nil {
		t.Error("stalled decoder was not reaped")
	}
}
