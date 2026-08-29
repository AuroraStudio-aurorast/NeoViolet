package cover

import (
	"bytes"
	"os"
	"testing"
)

// TestExtractFromFLAC extracts cover art from a real FLAC sample. The checked-in
// testdata samples do not embed cover art, so this test is skipped until a
// cover-bearing sample is added.
func TestExtractFromFLAC(t *testing.T) {
	f, err := os.Open("../../testdata/test_flac_with_metadata.flac")
	if err != nil {
		t.Skipf("testdata missing: %v", err)
	}
	defer func() { _ = f.Close() }()
	img, err := ExtractFromReader(f)
	if err != nil {
		t.Skipf("sample has no embedded cover art: %v", err)
	}
	if img == nil {
		t.Fatal("nil image")
	}
	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		t.Errorf("empty bounds: %v", img.Bounds())
	}
}

func TestExtractFromFileNonexistent(t *testing.T) {
	if _, err := ExtractFromFile("/nonexistent/file.flac"); err == nil {
		t.Error("ExtractFromFile on missing file should error")
	}
}

// TestExtractFromFileNoCover exercises the no-cover fallback path on a real
// audio file: dhowden/tag finds no picture, APEv2 has no cover data, and the
// function returns an error.
func TestExtractFromFileNoCover(t *testing.T) {
	img, err := ExtractFromFile("../../testdata/test_flac_with_metadata.flac")
	if err == nil {
		t.Fatalf("expected error for sample without cover art, got %v", img)
	}
}

// TestExtractFromReaderInvalidData verifies non-audio data is rejected.
func TestExtractFromReaderInvalidData(t *testing.T) {
	img, err := ExtractFromReader(bytes.NewReader([]byte("not audio data")))
	if err == nil {
		t.Fatalf("expected error for non-audio data, got %v", img)
	}
}
