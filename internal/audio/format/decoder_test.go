package format

import (
	"os"
	"testing"
)

func writeTempFile(t *testing.T, data []byte) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		t.Fatalf("Write: %v", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		f.Close()
		t.Fatalf("Seek: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestDetectFormatByMagic_mp3_id3(t *testing.T) {
	fd := NewFormatDecoder()
	f := writeTempFile(t, []byte("ID3some tag data here..."))
	ext, err := fd.DetectFormatByMagic(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ext != ".mp3" {
		t.Errorf("DetectFormatByMagic = %q, want .mp3", ext)
	}
}

func TestDetectFormatByMagic_mp3_sync(t *testing.T) {
	fd := NewFormatDecoder()
	f := writeTempFile(t, []byte{0xFF, 0xFB, 0x90, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	ext, err := fd.DetectFormatByMagic(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ext != ".mp3" {
		t.Errorf("DetectFormatByMagic = %q, want .mp3", ext)
	}
}

func TestDetectFormatByMagic_mp3_sync_no_second_byte(t *testing.T) {
	fd := NewFormatDecoder()
	f := writeTempFile(t, []byte{0xFF, 0x00, 0x00, 0x00})
	ext, err := fd.DetectFormatByMagic(f)
	if err == nil {
		t.Errorf("expected error for non-MP3 sync bytes, got %q", ext)
	}
}

func TestDetectFormatByMagic_mp2_sync(t *testing.T) {
	fd := NewFormatDecoder()
	f := writeTempFile(t, []byte{0xFF, 0xFC, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	ext, err := fd.DetectFormatByMagic(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ext != ".mp2" {
		t.Errorf("DetectFormatByMagic = %q, want .mp2", ext)
	}
}

func TestDetectFormatByMagic_mp2_id3(t *testing.T) {
	fd := NewFormatDecoder()
	// ID3v2 header + 2B tag body + MPEG-1 Layer II sync (0xFF 0xFC) at offset 12.
	data := append([]byte("ID3\x03\x00\x00\x00\x00\x00\x02"), 0x00, 0x00, 0xFF, 0xFC)
	f := writeTempFile(t, data)
	ext, err := fd.DetectFormatByMagic(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ext != ".mp2" {
		t.Errorf("DetectFormatByMagic = %q, want .mp2", ext)
	}
}

func TestDetectFormatByMagic_wav(t *testing.T) {
	fd := NewFormatDecoder()
	f := writeTempFile(t, []byte("RIFF\x00\x00\x00\x00WAVE"))
	ext, err := fd.DetectFormatByMagic(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ext != ".wav" {
		t.Errorf("DetectFormatByMagic = %q, want .wav", ext)
	}
}

func TestDetectFormatByMagic_flac(t *testing.T) {
	fd := NewFormatDecoder()
	f := writeTempFile(t, []byte("fLaC\x00\x00\x00\x00\x00\x00\x00\x00"))
	ext, err := fd.DetectFormatByMagic(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ext != ".flac" {
		t.Errorf("DetectFormatByMagic = %q, want .flac", ext)
	}
}

func TestDetectFormatByMagic_ogg(t *testing.T) {
	fd := NewFormatDecoder()
	f := writeTempFile(t, []byte("OggS\x00\x00\x00\x00\x00\x00\x00\x00"))
	ext, err := fd.DetectFormatByMagic(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ext != ".ogg" {
		t.Errorf("DetectFormatByMagic = %q, want .ogg", ext)
	}
}

func TestDetectFormatByMagic_midi(t *testing.T) {
	fd := NewFormatDecoder()
	f := writeTempFile(t, []byte("MThd\x00\x00\x00\x00\x00\x00\x00\x00"))
	ext, err := fd.DetectFormatByMagic(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ext != ".mid" && ext != ".midi" {
		t.Errorf("DetectFormatByMagic = %q, want .mid or .midi", ext)
	}
}

func TestDetectFormatByMagic_unknown(t *testing.T) {
	fd := NewFormatDecoder()
	f := writeTempFile(t, []byte("some unknown format data"))
	_, err := fd.DetectFormatByMagic(f)
	if err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestDetectFormatByMagic_too_short(t *testing.T) {
	fd := NewFormatDecoder()
	f := writeTempFile(t, []byte("abc"))
	_, err := fd.DetectFormatByMagic(f)
	if err == nil {
		t.Error("expected error for too-short file")
	}
}

func TestDetectFormatByMagic_empty(t *testing.T) {
	fd := NewFormatDecoder()
	f := writeTempFile(t, []byte{})
	_, err := fd.DetectFormatByMagic(f)
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestDetectFormatByMagic_ape(t *testing.T) {
	fd := NewFormatDecoder()
	f := writeTempFile(t, []byte("MAC \x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"))
	ext, err := fd.DetectFormatByMagic(f)
	if err != nil {
		t.Fatalf("unexpected error for APE: %v", err)
	}
	if ext != ".ape" {
		t.Errorf("DetectFormatByMagic = %q, want .ape", ext)
	}
}

func TestSupportedFormats(t *testing.T) {
	fd := NewFormatDecoder()
	formats := fd.SupportedFormats()
	expected := map[string]bool{".mp2": true, ".mp3": true, ".wav": true, ".flac": true, ".ogg": true, ".oga": true, ".opus": true, ".mid": true, ".midi": true, ".mod": true, ".xm": true, ".s3m": true, ".it": true, ".m4a": true, ".ape": true}
	if len(formats) != len(expected) {
		t.Fatalf("SupportedFormats() = %d items (%v), want %d", len(formats), formats, len(expected))
	}
	for _, f := range formats {
		if !expected[f] {
			t.Errorf("unexpected format %q", f)
		}
		delete(expected, f)
	}
	if len(expected) > 0 {
		t.Errorf("missing formats: %v", expected)
	}
}