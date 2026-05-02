package audio

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
	if ext != ".mid" {
		t.Errorf("DetectFormatByMagic = %q, want .mid", ext)
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

func TestSupportedFormats(t *testing.T) {
	fd := NewFormatDecoder()
	formats := fd.SupportedFormats()
	expected := []string{".mp3", ".wav", ".flac", ".ogg", ".oga", ".mid"}
	if len(formats) != len(expected) {
		t.Fatalf("SupportedFormats() = %v, want %v", formats, expected)
	}
	for i, f := range formats {
		if f != expected[i] {
			t.Errorf("SupportedFormats()[%d] = %q, want %q", i, f, expected[i])
		}
	}
}
