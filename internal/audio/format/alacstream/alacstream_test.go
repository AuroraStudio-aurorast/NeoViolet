package alacstream

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gopxl/beep/v2"
)

var testDataDir string

func init() {
	abs, _ := filepath.Abs(filepath.Join("..", "..", "..", "..", "testdata"))
	testDataDir = abs
}

func testFile(name string) string {
	return filepath.Join(testDataDir, name)
}

func TestDecodeM4A_Detection(t *testing.T) {
	f, err := os.Open(testFile("test_alac.m4a"))
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 12)
	_, err = f.Read(buf)
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	if string(buf[4:8]) != "ftyp" {
		t.Errorf("expected ftyp box, got %q", string(buf[4:8]))
	}
}

func TestDecodeM4A_Streamer(t *testing.T) {
	f, err := os.Open(testFile("test_alac.m4a"))
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	defer f.Close()

	streamer, format, err := DecodeM4A(f)
	if err != nil {
		t.Fatalf("DecodeM4A: %v", err)
	}
	defer streamer.Close()

	if format.SampleRate != 44100 {
		t.Errorf("expected sample rate 44100, got %d", format.SampleRate)
	}
	if format.NumChannels != 1 {
		t.Logf("expected mono, got %d", format.NumChannels)
	}

	t.Logf("Format: %+v", format)
	t.Logf("Total samples: %d", streamer.Len())
	t.Logf("Duration: %v", format.SampleRate.D(streamer.Len()))

	samples := make([][2]float64, 4410)
	n, ok := streamer.Stream(samples)
	if !ok {
		t.Fatal("stream returned !ok on first read")
	}
	if n != 4410 {
		t.Errorf("expected 4410 samples, got %d", n)
	}

	hasAudio := false
	for _, s := range samples[:100] {
		if s[0] != 0 || s[1] != 0 {
			hasAudio = true
			break
		}
	}
	if !hasAudio {
		t.Error("expected audio data, got all zeros")
	}

	err = streamer.Seek(0)
	if err != nil {
		t.Errorf("Seek(0): %v", err)
	}
	if streamer.Position() != 0 {
		t.Errorf("expected position 0 after seek, got %d", streamer.Position())
	}

	midSample := streamer.Len() / 2
	err = streamer.Seek(midSample)
	if err != nil {
		t.Errorf("Seek to middle: %v", err)
	}
	if streamer.Position() != midSample {
		t.Errorf("expected position %d, got %d", midSample, streamer.Position())
	}

	n, ok = streamer.Stream(samples[:100])
	if !ok {
		t.Fatal("stream returned !ok after seek")
	}
	if n != 100 {
		t.Errorf("expected 100 samples after seek, got %d", n)
	}
}

func TestDecodeM4A_ImplementsStreamSeekCloser(t *testing.T) {
	var _ beep.StreamSeekCloser = (*Streamer)(nil)
}

func TestDecodeM4A_ErrMethod(t *testing.T) {
	f, err := os.Open(testFile("test_alac.m4a"))
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	defer f.Close()

	streamer, _, err := DecodeM4A(f)
	if err != nil {
		t.Fatalf("DecodeM4A: %v", err)
	}
	defer streamer.Close()

	if err := streamer.Err(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}