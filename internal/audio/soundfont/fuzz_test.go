package soundfont

import (
	"bytes"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

func FuzzLoadSoundFont(f *testing.F) {
	f.Add(miditest.SF2(nil))
	f.Add([]byte("RIFF\xff\xff\xff\xffsfbk"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			t.Skip()
		}
		sf, _, e := load(bytes.NewReader(b), loadLimits{1 << 20, 1 << 20})
		if e == nil {
			cfg := NewSynthesizerSettings(44100)
			cfg.EnableReverbAndChorus = false
			s, err := NewSynthesizer([]*SoundFont{sf}, cfg)
			if err != nil {
				t.Fatal(err)
			}
			s.NoteOn(0, 60, 100)
			s.Render(make([]float32, 128), make([]float32, 128))
		}
	})
}
