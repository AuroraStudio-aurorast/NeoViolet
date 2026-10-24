package midi

import (
	"bytes"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

func FuzzParseSMF(f *testing.F) {
	f.Add(miditest.SMF(0, 120, []byte{0, 0x90, 60, 100, 120, 0x80, 60, 0, 0, 255, 47, 0}))
	f.Add([]byte("MThd\x00\x00\x00\x06\x00\x01\x00\x01\x00\x78MTrk\xff\xff\xff\xff"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			t.Skip()
		}
		s, _, e := parse(bytes.NewReader(b), parseLimits{1 << 20, 10000, 65536})
		if e == nil {
			_, _ = s.Compile(44100)
		}
	})
}
