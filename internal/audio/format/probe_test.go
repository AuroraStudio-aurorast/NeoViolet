package format

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProbeReadsDurationWithoutPlaybackAndCancels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "silent.WAV")
	header := make([]byte, 44+8000*2)
	copy(header, "RIFF")
	binary.LittleEndian.PutUint32(header[4:], 16036)
	copy(header[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(header[16:], 16)
	binary.LittleEndian.PutUint16(header[20:], 1)
	binary.LittleEndian.PutUint16(header[22:], 1)
	binary.LittleEndian.PutUint32(header[24:], 8000)
	binary.LittleEndian.PutUint32(header[28:], 16000)
	binary.LittleEndian.PutUint16(header[32:], 2)
	binary.LittleEndian.PutUint16(header[34:], 16)
	copy(header[36:], "data")
	binary.LittleEndian.PutUint32(header[40:], 16000)
	if err := os.WriteFile(path, header, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := NewDecoder().Probe(context.Background(), path)
	if err != nil || got.Duration != time.Second || got.Format != "WAV" {
		t.Fatalf("probe: %+v %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewDecoder().Probe(ctx, path)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("probe ignored cancellation: %v", err)
	}
}
