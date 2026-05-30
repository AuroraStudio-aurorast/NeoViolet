package opusstream

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

func TestDecodeOpus_Detection(t *testing.T) {
	f, err := os.Open(testFile("test_opus.opus"))
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 37)
	_, err = f.Read(buf)
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	if string(buf[0:4]) != "OggS" {
		t.Errorf("expected OggS magic, got %q", string(buf[0:4]))
	}
	if string(buf[28:36]) != "OpusHead" {
		t.Errorf("expected OpusHead, got %q", string(buf[28:36]))
	}
}

func TestDecodeOpus_Streamer(t *testing.T) {
	f, err := os.Open(testFile("test_opus.opus"))
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	defer f.Close()

	streamer, format, err := DecodeOGG(f)
	if err != nil {
		t.Fatalf("DecodeOGG: %v", err)
	}
	defer streamer.Close()

	if format.SampleRate != 48000 {
		t.Errorf("expected sample rate 48000, got %d", format.SampleRate)
	}
	if format.NumChannels != 1 {
		t.Logf("expected mono, got %d", format.NumChannels)
	}

	t.Logf("Format: %+v", format)
	t.Logf("Total samples: %d", streamer.Len())
	t.Logf("Duration: %v", format.SampleRate.D(streamer.Len()))

	// Read some samples
	samples := make([][2]float64, 4800) // 0.1s
	n, ok := streamer.Stream(samples)
	if !ok {
		t.Fatal("stream returned !ok on first read")
	}
	if n == 0 {
		t.Fatal("stream returned 0 samples")
	}

	hasAudio := false
	checkLen := n
	if checkLen > 100 {
		checkLen = 100
	}
	for _, s := range samples[:checkLen] {
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

	n, ok = streamer.Stream(samples[:100])
	if !ok {
		t.Fatal("stream returned !ok after seek")
	}
	if n == 0 {
		t.Fatal("stream returned 0 samples after seek")
	}
}

func TestDecodeOpus_ImplementsStreamSeekCloser(t *testing.T) {
	var _ beep.StreamSeekCloser = (*Streamer)(nil)
}

func TestDecodeOpus_ErrMethod(t *testing.T) {
	f, err := os.Open(testFile("test_opus.opus"))
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	defer f.Close()

	streamer, _, err := DecodeOGG(f)
	if err != nil {
		t.Fatalf("DecodeOGG: %v", err)
	}
	defer streamer.Close()

	if err := streamer.Err(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}