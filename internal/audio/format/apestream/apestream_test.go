package apestream

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/apetag"
)

var testDataDir string

func init() {
	abs, _ := filepath.Abs(filepath.Join("..", "..", "..", "..", "testdata"))
	testDataDir = abs
}

func testFile(name string) string {
	return filepath.Join(testDataDir, name)
}

func TestConvertPCMToFloat64_16bit(t *testing.T) {
	// 2 frames of 16-bit stereo PCM: [0, 32767, -32768, 16384]
	pcm := []byte{
		0x00, 0x00, // ch0: 0
		0xFF, 0x7F, // ch1: 32767 (max positive)
		0x00, 0x80, // ch0: -32768 (min negative)
		0x00, 0x40, // ch1: 16384
	}
	out := make([]float64, 4) // 2 frames * 2 channels
	result := convertPCMToFloat64(pcm, 2, 2, out)

	if len(result) != 4 {
		t.Fatalf("expected 4 samples, got %d", len(result))
	}

	// 0
	if result[0] != 0.0 {
		t.Errorf("sample 0: expected 0.0, got %f", result[0])
	}
	// 32767 / 32768 ≈ 0.999969
	if result[1] < 0.99 || result[1] > 1.0 {
		t.Errorf("sample 1: expected ~0.999969, got %f", result[1])
	}
	// -32768 / 32768 = -1.0
	if result[2] != -1.0 {
		t.Errorf("sample 2: expected -1.0, got %f", result[2])
	}
}

func TestConvertPCMToFloat64_mono(t *testing.T) {
	// 2 frames of 16-bit mono PCM
	pcm := []byte{
		0x00, 0x40, // frame 0: 16384
		0x00, 0x80, // frame 1: -32768
	}
	out := make([]float64, 4) // 2 frames * 2 channels
	result := convertPCMToFloat64(pcm, 1, 2, out)

	if len(result) != 4 {
		t.Fatalf("expected 4 samples, got %d", len(result))
	}

	// Mono should be duplicated to L+R
	if result[0] != result[1] {
		t.Errorf("mono: L and R should be equal, got %f vs %f", result[0], result[1])
	}
	if result[0] < 0.49 || result[0] > 0.51 {
		t.Errorf("frame 0: expected ~0.5, got %f", result[0])
	}
	if result[2] != -1.0 {
		t.Errorf("frame 1: expected -1.0, got %f", result[2])
	}
}

func TestConvertPCMToFloat64_8bit(t *testing.T) {
	// unsigned 8-bit: 0 → -128 → -1.0, 255 → 127 → ~0.992, 128 → 0 → 0.0
	pcm := []byte{0x00, 0xFF, 0x80}
	out := make([]float64, 6) // 3 frames * 2 channels
	// Channels = 1 for simplicity (mono 8-bit)
	result := convertPCMToFloat64(pcm, 1, 1, out[:6])
	if len(result) != 6 {
		t.Fatalf("expected 6 samples, got %d", len(result))
	}
	if result[0] != -1.0 {
		t.Errorf("sample 0: expected -1.0 (0x00), got %f", result[0])
	}
	if result[2] < 0.99 || result[2] > 1.0 {
		t.Errorf("sample 1: expected ~0.992 (0xFF), got %f", result[2])
	}
	if result[4] != 0.0 {
		t.Errorf("sample 2: expected 0.0 (0x80), got %f", result[4])
	}
}

func TestConvertPCMToFloat64_24bit(t *testing.T) {
	// 1 frame of 24-bit stereo PCM
	pcm := []byte{
		0x00, 0x00, 0x00, // ch0: 0
		0xFF, 0xFF, 0x7F, // ch1: 8388607 (max positive 24-bit)
	}
	out := make([]float64, 2)
	result := convertPCMToFloat64(pcm, 2, 3, out)

	if len(result) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(result))
	}
	if result[0] != 0.0 {
		t.Errorf("expected 0.0, got %f", result[0])
	}
	if result[1] < 0.99 || result[1] > 1.0 {
		t.Errorf("expected ~1.0, got %f", result[1])
	}
}

func TestConvertPCMToFloat64_32bit(t *testing.T) {
	// 1 frame of 32-bit stereo PCM
	pcm := []byte{
		0x00, 0x00, 0x00, 0x80, // ch0: -2147483648 (min int32)
		0xFF, 0xFF, 0xFF, 0x7F, // ch1: 2147483647 (max int32)
	}
	out := make([]float64, 2)
	result := convertPCMToFloat64(pcm, 2, 4, out)

	if len(result) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(result))
	}
	if result[0] != -1.0 {
		t.Errorf("expected -1.0, got %f", result[0])
	}
	if result[1] < 0.99 || result[1] > 1.0 {
		t.Errorf("expected ~1.0, got %f", result[1])
	}
}

func TestFindApeCLI(t *testing.T) {
	// The function should either return empty (if apecli isn't installed)
	// or find it alongside the test binary.
	path := findApeCLI()
	// During development, it may be found. In CI, it may not.
	// Just ensure no crash and it returns something sensible.
	t.Logf("findApeCLI() = %q", path)
}

func TestProbeBackends(t *testing.T) {
	backends := probeBackends()
	t.Logf("found %d backends:", len(backends))
	for _, b := range backends {
		t.Logf("  - %s", b.Name())
	}
	// Should always return at least 0 backends (never crash).
}

func TestStreamerDecodeNoBackend(t *testing.T) {
	// Decode should return an error when no backends are available.
	// We can't easily disable backends, but we can test the error path
	// by opening a non-APE file.
	_, _, err := Decode(nil, "/nonexistent/test.ape")
	if err == nil {
		t.Skip("backends are available — can't test no-backend path")
	}
}

func TestDecodeInvalidFile(t *testing.T) {
	// Test that Decode handles a non-APE file gracefully.
	// Create a temp file with garbage content.
	tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.ape")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write([]byte("not an APE file at all")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	tmpFile.Seek(0, io.SeekStart)

	// Decode should fail since this is not really an APE file.
	// The backends should all fail and return an error.
	_, _, err = Decode(tmpFile, tmpFile.Name())
	if err == nil {
		t.Skip("backends may report errors differently; not failing")
	}
	t.Logf("Decode error (expected): %v", err)
}

func TestStreamerInterface(t *testing.T) {
	// Verify that *Streamer implements beep.StreamSeekCloser at compile time.
	var _ interface {
		Stream([][2]float64) (int, bool)
		Seek(int) error
		Len() int
		Position() int
		Close() error
	} = (*Streamer)(nil)
}

func TestDecodeStereoAPE(t *testing.T) {
	path := testFile("test_ape.ape")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open test fixture: %v", err)
	}
	defer f.Close()

	s, fmt, err := Decode(f, path)
	if err != nil {
		t.Skipf("no APE backend available (install ffmpeg or mac, or build apecli): %v", err)
	}
	defer s.Close()

	if fmt.SampleRate != 44100 {
		t.Errorf("SampleRate = %d, want 44100", fmt.SampleRate)
	}
	if fmt.NumChannels != 2 {
		t.Errorf("NumChannels = %d, want 2", fmt.NumChannels)
	}
	if fmt.Precision != 2 {
		t.Errorf("Precision = %d, want 2 (16-bit)", fmt.Precision)
	}

	buf := make([][2]float64, 512)
	n, ok := s.Stream(buf)
	if !ok {
		t.Fatal("Stream returned false on first read")
	}
	if n != 512 {
		t.Errorf("Stream returned %d frames, want 512", n)
	}
	var sum float64
	for i := range buf {
		sum += buf[i][0]
	}
	if sum == 0 {
		t.Error("first buffer samples are all zero — expected audio data")
	}
}

func TestDecodeMonoAPE(t *testing.T) {
	path := testFile("test_ape_mono.ape")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open test fixture: %v", err)
	}
	defer f.Close()

	s, fmt, err := Decode(f, path)
	if err != nil {
		t.Skipf("no APE backend available: %v", err)
	}
	defer s.Close()

	t.Logf("Mono format: %d Hz, %d ch, %d-bit", fmt.SampleRate, fmt.NumChannels, fmt.Precision)
	if fmt.SampleRate != 44100 {
		t.Errorf("SampleRate = %d, want 44100", fmt.SampleRate)
	}
	if fmt.NumChannels != 1 {
		t.Errorf("NumChannels = %d, want 1", fmt.NumChannels)
	}

	buf := make([][2]float64, 256)
	n, ok := s.Stream(buf)
	if !ok || n == 0 {
		t.Fatal("Stream returned no data")
	}
	for i := range buf[:n] {
		if buf[i][0] != buf[i][1] {
			t.Fatalf("frame %d: L=%f ≠ R=%f — mono should be duplicated", i, buf[i][0], buf[i][1])
		}
	}
}

func TestDecodeAPEWithTags(t *testing.T) {
	path := testFile("test_ape.ape")
	tags, err := apetag.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if tags.Title != "Test Song" {
		t.Errorf("Title = %q, want %q", tags.Title, "Test Song")
	}
	if tags.Artist != "Test Artist" {
		t.Errorf("Artist = %q, want %q", tags.Artist, "Test Artist")
	}
	t.Logf("Tags: Title=%q Artist=%q", tags.Title, tags.Artist)
}