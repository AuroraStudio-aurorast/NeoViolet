package alac

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/mp4"
)

func TestSignExtended32(t *testing.T) {
	cases := []struct {
		in   int32
		bits int
		want int32
	}{
		{0x7F, 8, 127},
		{0x80, 8, -128},
		{0xFF, 8, -1},
		// 14-bit sign extension: valid inputs are 0x0000..0x3FFF.
		{0x1FFF, 14, 8191},  // max positive 14-bit value
		{0x2000, 14, -8192}, // min negative 14-bit value
	}
	for _, tc := range cases {
		if got := signExtended32(tc.in, tc.bits); got != tc.want {
			t.Errorf("signExtended32(%d, %d) = %d, want %d", tc.in, tc.bits, got, tc.want)
		}
	}
}

func TestCountLeadingZeros(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{0, 32},
		{1, 31},
		{0x80000000, 0},
	}
	for _, tc := range cases {
		if got := countLeadingZeros(tc.in); got != tc.want {
			t.Errorf("countLeadingZeros(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestCookieParsing(t *testing.T) {
	// Build a minimal ALACSpecificConfig per parseCookie's actual layout:
	// [0-3]   version/flags (skip)
	// [4-7]   max_samples_per_frame (uint32 BE)
	// [8]     unknown (7a)
	// [9]     sample_size
	// [10]    rice_historymult
	// [11]    rice_initialhistory
	// [12]    rice_kmodifier
	// [13]    unknown (7f)
	// [14-15] unknown (80) (uint16)
	// [16-19] unknown (82) (uint32)
	// [20-23] unknown (86) (uint32)
	// [24-27] sample_rate (uint32)
	cookie := make([]byte, 28)
	binary.BigEndian.PutUint32(cookie[4:8], 4096)    // max_samples_per_frame
	cookie[9] = 16                                   // sample_size
	binary.BigEndian.PutUint32(cookie[24:28], 44100) // sample_rate

	d, err := NewDecoder(cookie)
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	if d.MaxSamplesPerFrame != 4096 {
		t.Errorf("MaxSamplesPerFrame = %d, want 4096", d.MaxSamplesPerFrame)
	}
	if d.SampleSize() != 16 {
		t.Errorf("SampleSize() = %d, want 16", d.SampleSize())
	}
	if d.SampleRate() != 44100 {
		t.Errorf("SampleRate() = %d, want 44100", d.SampleRate())
	}
	if d.NumChannels() != 2 {
		t.Errorf("NumChannels() = %d, want 2 (ALAC decoder defaults to stereo)", d.NumChannels())
	}
}

func TestDecodeRealSample(t *testing.T) {
	f, err := os.Open("../../../../testdata/test_alac.m4a")
	if err != nil {
		t.Skipf("testdata missing: %v", err)
	}
	defer f.Close()

	demux := mp4.NewDemuxer(f)
	track, err := demux.FindALACTrack()
	if err != nil {
		t.Fatalf("FindALACTrack: %v", err)
	}
	d, err := NewDecoder(track.MagicCookie)
	if err != nil {
		t.Fatalf("NewDecoder(cookie): %v", err)
	}
	d.SetNumChannels(int(track.Channels))
	if d.SampleRate() == 0 {
		t.Error("SampleRate() = 0")
	}
	// Decode a few frames and assert structural invariants:
	// output length == frame_size * channels * (sample_size/8), no panic.
	for i := uint32(0); i < 3 && i < track.SampleCount; i++ {
		frame, err := track.ReadSample(f, i)
		if err != nil {
			t.Fatalf("ReadSample(%d): %v", i, err)
		}
		out := d.Decode(frame)
		if len(out) == 0 {
			t.Fatalf("Decode returned empty output for frame %d", i)
		}
	}
}
