package format

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProbeAPEDuration(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "test_ape.ape"))
	if err != nil {
		t.Fatal(err)
	}
	got := probeTestFile(t, "test_ape.ape", data)
	if got.Format != "APE" || got.Duration != 2*time.Second {
		t.Fatalf("got format=%q duration=%v, want APE and 2s", got.Format, got.Duration)
	}
}

func TestProbeAPEDurationBehindID3(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "test_ape.ape"))
	if err != nil {
		t.Fatal(err)
	}
	id3 := make([]byte, 10)
	copy(id3, []byte{'I', 'D', '3', 4, 0, 0})
	data = append(id3, data...)
	got := probeTestFile(t, "test_ape.ape", data)
	if got.Format != "APE" || got.Duration != 2*time.Second {
		t.Fatalf("got format=%q duration=%v, want APE and 2s", got.Format, got.Duration)
	}
}
