package midi

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

func TestParseSMF(t *testing.T) {
	a := []byte{0, 0xFF, 0x21, 1, 1, 0, 0xC0, 80, 0, 0x90, 60, 100, 120, 60, 0, 0, 0xFF, 0x2F, 0}
	b := []byte{0, 0xC0, 39, 0, 0x90, 60, 80, 120, 0x80, 60, 0, 0, 0xFF, 0x2F, 0}
	s, _, err := Parse(bytes.NewReader(miditest.SMF(1, 120, a, b)))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Events) != 6 {
		t.Fatalf("events=%d", len(s.Events))
	}
	for i, w := range []struct {
		port, status uint8
		track        uint32
	}{{1, 0xC0, 0}, {1, 0x90, 0}, {0, 0xC0, 1}, {0, 0x90, 1}, {1, 0x90, 0}, {0, 0x80, 1}} {
		e := s.Events[i]
		if e.Port != w.port || e.Status != w.status || e.Track != w.track {
			t.Fatalf("event%d=%+v", i, e)
		}
	}
}

func TestParseSMFRejectsMalformed(t *testing.T) {
	for _, data := range [][]byte{nil, miditest.SMF(2, 120, []byte{0, 255, 47, 0}), miditest.SMF(0, 0, []byte{0, 255, 47, 0}), miditest.SMF(1, 120), miditest.SMF(0, 120, []byte{0, 60, 100}), miditest.SMF(0, 120, []byte{128, 128, 128, 128, 0, 255, 47, 0}), miditest.SMF(0, 120, []byte{0, 0x90, 60})} {
		if _, _, err := Parse(bytes.NewReader(data)); err == nil {
			t.Fatalf("accepted %x", data)
		}
	}
}

func TestRepairFinalTrackLength(t *testing.T) {
	raw := miditest.SMF(0, 120, []byte{0, 0xBF, 120, 0, 0, 0xFF, 0x2F, 0})
	binary.BigEndian.PutUint32(raw[18:22], 3)
	_, d, err := Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(d) != 1 || d[0].Code != "track_length_repaired" {
		t.Fatalf("diagnostics %+v", d)
	}
	bad := append(append([]byte(nil), raw...), 0)
	if _, _, err = Parse(bytes.NewReader(bad)); err == nil {
		t.Fatal("accepted garbage after repaired EOT")
	}
}

func TestSysExFragments(t *testing.T) {
	tr := []byte{0, 0xF0, 3, 0x7E, 0x7F, 9, 0, 0xF7, 2, 1, 0xF7, 0, 0xFF, 0x2F, 0}
	s, _, err := Parse(bytes.NewReader(miditest.SMF(0, 120, tr)))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Events) != 1 || !bytes.Equal(s.Events[0].Payload, []byte{0x7E, 0x7F, 9, 1, 0xF7}) {
		t.Fatalf("fragments %+v", s.Events)
	}
}

func TestCompileTempoRemainders(t *testing.T) {
	tr := []byte{0, 255, 81, 3, 7, 83, 0, 120, 0x90, 60, 100, 0, 255, 47, 0} // 480000 us/beat
	s, _, err := Parse(bytes.NewReader(miditest.SMF(0, 120, tr)))
	if err != nil {
		t.Fatal(err)
	}
	tl, err := s.Compile(44100)
	if err != nil {
		t.Fatal(err)
	}
	if tl.Events[0].Frame != 21168 || tl.Events[0].DispatchFrame != 21184 || tl.ContentFrames != 21168 || tl.TotalFrames != 109368 {
		t.Fatalf("timeline %+v", tl)
	}
}

func TestCompileSMPTE(t *testing.T) {
	for _, tc := range []struct {
		division uint16
		ticks    uint32
		want     int64
	}{{0xE728, 1000, 44100}, {0xE350, 2400, 44144}} { // -25*40, -29*80
		tr := append(miditest.Event(tc.ticks, 0x90, 60, 100), 0, 255, 47, 0)
		s, _, err := Parse(bytes.NewReader(miditest.SMF(0, tc.division, tr)))
		if err != nil {
			t.Fatal(err)
		}
		tl, err := s.Compile(44100)
		if err != nil {
			t.Fatal(err)
		}
		if tl.ContentFrames != tc.want {
			t.Fatalf("%x got%d want%d", tc.division, tl.ContentFrames, tc.want)
		}
	}
}
