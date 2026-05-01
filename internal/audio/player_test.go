package audio

import (
	"os"
	"testing"
)

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
