package soundfont

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

// A short Read must not be confused with EOF by fixed-record parsing.
type shortSeeker struct{ *bytes.Reader }

func (s shortSeeker) Read(b []byte) (int, error) {
	if len(b) > 3 {
		b = b[:3]
	}
	return s.Reader.Read(b)
}

func TestSF2ChunkBounds(t *testing.T) {
	valid := miditest.SF2(nil)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"truncated", valid[:len(valid)-1]},
		{"short_riff", []byte("RIFF\xff\xff\xff\xffsfbk")},
		{"bad_bag", mutateSF2(valid, "ibag", 0, []byte{255, 255})},
		{"bad_instrument", mutateSF2(valid, "pgen", 2, []byte{255, 127})},
		{"bad_sample", mutateSF2(valid, "igen", 6, []byte{255, 127})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewSoundFont(bytes.NewReader(tc.data)); err == nil {
				t.Fatal("invalid SF2 accepted")
			}
		})
	}
}

func TestSF2ShortReadsAndUnknownChunks(t *testing.T) {
	data := miditest.SF2(nil)
	body := append([]byte(nil), data[8:]...)
	body = append(body, miditest.Chunk("JUNK", []byte{1, 2, 3})...)
	for _, r := range []io.ReadSeeker{shortSeeker{bytes.NewReader(data)}, bytes.NewReader(miditest.Chunk("RIFF", body))} {
		if _, err := NewSoundFont(r); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEmptyInstrumentPreservesIDs(t *testing.T) {
	data := miditest.SF2(&miditest.SF2Options{
		Instruments:       [][]miditest.Generator{{{Operator: 53, Amount: 0}}, {}, {{Operator: 53, Amount: 0}}},
		PresetInstruments: []uint16{0, 1, 2},
	})
	sf, err := NewSoundFont(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(sf.Instruments) != 3 || len(sf.Instruments[1].Regions) != 0 || len(sf.Instruments[2].Regions) != 1 {
		t.Fatal("empty instrument shifted references")
	}
	if sf.Presets[2].Regions[0].Instrument != sf.Instruments[2] {
		t.Fatal("preset references changed")
	}
}

func TestNoPlayableRegionsFails(t *testing.T) {
	data := miditest.SF2(&miditest.SF2Options{Instruments: [][]miditest.Generator{{}}})
	if _, err := NewSoundFont(bytes.NewReader(data)); err == nil {
		t.Fatal("unusable font accepted")
	}
}

// mutateSF2 changes one subchunk payload without involving the production loader.
func mutateSF2(data []byte, id string, offset int, value []byte) []byte {
	result := append([]byte(nil), data...)
	var walk func(int, int) bool
	walk = func(start, end int) bool {
		for p := start; p+8 <= end; {
			n := int(binary.LittleEndian.Uint32(result[p+4 : p+8]))
			if p+8+n > end {
				return false
			}
			if string(result[p:p+4]) == id {
				copy(result[p+8+offset:p+8+n], value)
				return true
			}
			if string(result[p:p+4]) == "LIST" && walk(p+12, p+8+n) {
				return true
			}
			p += 8 + n + n%2
		}
		return false
	}
	if !walk(12, len(result)) {
		panic("fixture subchunk missing: " + id)
	}
	return result
}
