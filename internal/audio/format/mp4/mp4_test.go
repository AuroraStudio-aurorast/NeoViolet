package mp4

import (
	"bytes"
	"os"
	"testing"
)

func TestReadBoxHeader(t *testing.T) {
	// 32-bit size + 'moov' type
	buf := bytes.NewReader([]byte{
		0x00, 0x00, 0x00, 0x18, // size = 24
		'm', 'o', 'o', 'v',
	})
	size, boxType, err := readBoxHeader(buf)
	if err != nil {
		t.Fatalf("readBoxHeader: %v", err)
	}
	if size != 24 || boxType != "moov" {
		t.Errorf("size=%d type=%q, want 24/moov", size, boxType)
	}
}

func TestIsContainer(t *testing.T) {
	for _, bt := range []string{"moov", "trak", "mdia", "minf", "stbl", "udta"} {
		if !isContainer(bt) {
			t.Errorf("isContainer(%q) = false, want true", bt)
		}
	}
	for _, bt := range []string{"mdat", "stsd", "free", "wide"} {
		if isContainer(bt) {
			t.Errorf("isContainer(%q) = true, want false", bt)
		}
	}
}

func TestFindALACTrackRealSample(t *testing.T) {
	f, err := os.Open("../../../../testdata/test_alac.m4a")
	if err != nil {
		t.Skipf("testdata missing: %v", err)
	}
	defer func() { _ = f.Close() }()

	d := NewDemuxer(f)
	track, err := d.FindALACTrack()
	if err != nil {
		t.Fatalf("FindALACTrack: %v", err)
	}
	if track.Channels == 0 || track.SampleCount == 0 || track.TimeScale == 0 {
		t.Errorf("track has zero fields: %+v", track)
	}
	if len(track.MagicCookie) == 0 {
		t.Error("ALAC cookie is empty")
	}
}
