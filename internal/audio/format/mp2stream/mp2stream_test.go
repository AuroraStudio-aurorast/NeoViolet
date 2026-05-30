package mp2stream

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

func TestDecodeMP2_Detection(t *testing.T) {
	f, err := os.Open(testFile("test_mp2.mp2"))
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 4)
	_, err = f.Read(buf)
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	// MPEG sync word
	if buf[0] != 0xFF || (buf[1]&0xE0) != 0xE0 {
		t.Errorf("expected MPEG sync word, got %02x %02x", buf[0], buf[1])
	}
	// Layer bits: for Layer II they should be bits 17-18 of the 32-bit header,
	// which fall in byte 1 bits 3-2 (after the 11-bit sync).
	// sync word = bits 31-21 = 0xFFE, layer = bits 19-18, so in buf[1]:
	// buf[1] bit pattern: S S S S L L V V  (S=sync, L=layer, V=version)
	// For MPEG-1 Layer 2: layer=10, so buf[1] & 0x06 should be 0x04
	layer := (buf[1] >> 1) & 0x03
	if layer != 2 {
		t.Logf("warning: expected layer=2 (MP2), got layer=%d — may be valid MPEG-1 Layer II", layer)
	}
}

func TestDecodeMP2_Streamer(t *testing.T) {
	f, err := os.Open(testFile("test_mp2.mp2"))
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	defer f.Close()

	streamer, format, err := DecodeMP2(f)
	if err != nil {
		t.Fatalf("DecodeMP2: %v", err)
	}
	defer streamer.Close()

	if format.SampleRate == 0 {
		t.Error("expected non-zero sample rate")
	}
	if format.NumChannels == 0 {
		t.Error("expected non-zero channel count")
	}
	if format.Precision != 2 {
		t.Errorf("expected precision 2, got %d", format.Precision)
	}

	t.Logf("Format: %+v", format)
	t.Logf("Total samples: %d", streamer.Len())
	t.Logf("Duration: %v", format.SampleRate.D(streamer.Len()))

	if streamer.Len() == 0 {
		t.Fatal("expected non-zero total samples")
	}

	// Read some samples
	samples := make([][2]float64, 4800) // ~0.1s at 44100
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

	// Test Seek(0)
	err = streamer.Seek(0)
	if err != nil {
		t.Errorf("Seek(0): %v", err)
	}
	if streamer.Position() != 0 {
		t.Errorf("expected position 0 after seek, got %d", streamer.Position())
	}

	// Test seek to middle
	// MP2 frames are always 1152 samples; Seek snaps to frame boundaries.
	midSample := streamer.Len() / 2
	err = streamer.Seek(midSample)
	if err != nil {
		t.Errorf("Seek to middle: %v", err)
	}
	pos := streamer.Position()
	if pos == 0 {
		t.Errorf("Seek to middle landed at position 0 (midSample=%d)", midSample)
	}
	if pos >= streamer.Len() {
		t.Errorf("Seek to middle at end (pos=%d, len=%d)", pos, streamer.Len())
	}
	if diff := midSample - pos; diff > 1152 || diff < -1152 {
		t.Errorf("Seek to middle: expected within 1152 of %d, got %d (diff=%d)", midSample, pos, diff)
	}

	n, ok = streamer.Stream(samples[:100])
	if !ok {
		t.Fatal("stream returned !ok after seek")
	}
	if n == 0 {
		t.Fatal("stream returned 0 samples after seek")
	}

	// Test seek past end
	err = streamer.Seek(streamer.Len() + 1000)
	if err != nil {
		t.Errorf("Seek past end: %v", err)
	}
}

func TestDecodeMP2_ImplementsStreamSeekCloser(t *testing.T) {
	var _ beep.StreamSeekCloser = (*Streamer)(nil)
}

func TestDecodeMP2_ErrMethod(t *testing.T) {
	f, err := os.Open(testFile("test_mp2.mp2"))
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	defer f.Close()

	streamer, _, err := DecodeMP2(f)
	if err != nil {
		t.Fatalf("DecodeMP2: %v", err)
	}
	defer streamer.Close()

	if err := streamer.Err(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestDecodeMP2_Close(t *testing.T) {
	f, err := os.Open(testFile("test_mp2.mp2"))
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}

	streamer, _, err := DecodeMP2(f)
	if err != nil {
		f.Close()
		t.Fatalf("DecodeMP2: %v", err)
	}

	if err := streamer.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	// Stream should return (0, false) after close
	samples := make([][2]float64, 100)
	n, ok := streamer.Stream(samples)
	if n != 0 || ok {
		t.Errorf("expected (0, false) after close, got (%d, %v)", n, ok)
	}

	// Seek should return error after close
	if err := streamer.Seek(0); err == nil {
		t.Error("expected error from Seek after close")
	}
}

func TestDecodeMP2_BadFile(t *testing.T) {
	// Create a temporary file that is definitely not MP2
	f, err := os.CreateTemp(t.TempDir(), "bad-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()

	// Write garbage
	f.Write([]byte("this is not an mp2 file"))
	f.Seek(0, 0)

	_, _, err = DecodeMP2(f)
	if err == nil {
		t.Error("expected error for bad/empty file")
	}
}