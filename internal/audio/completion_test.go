package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/gopxl/beep/v2/wav"
)

func completionWAVPlayer(t *testing.T, frames int) *Player {
	t.Helper()
	data := make([]byte, 44+frames*4)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8)) // #nosec G115 -- small test fixtures.
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 2)
	binary.LittleEndian.PutUint32(data[24:], 44100)
	binary.LittleEndian.PutUint32(data[28:], 44100*4)
	binary.LittleEndian.PutUint16(data[32:], 4)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(frames*4)) // #nosec G115 -- small test fixtures.
	for i := 44; i < len(data); i += 2 {
		binary.LittleEndian.PutUint16(data[i:], 4096)
	}
	stream, format, err := wav.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	p := NewPlayer()
	p.setupStreamer(stream, format, nopCloser{}, "finite.wav")
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestPlayerCompletionFollowsMixerDrain(t *testing.T) {
	for _, tc := range []struct {
		name      string
		frames    int
		resampled bool
	}{
		{"short", 1, false}, {"whole-block", 512, false},
		{"multiple-blocks", 513, false}, {"resampled", 513, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mixer := outputMixer(t)
			previousRate := speakerSampleRate
			speakerSampleRate = 44100
			if tc.resampled {
				speakerSampleRate = 48000
			}
			t.Cleanup(func() { speakerSampleRate = previousRate })
			p := completionWAVPlayer(t, tc.frames)
			if err := p.Play(); err != nil {
				t.Fatal(err)
			}
			var out [512][2]float64
			for pulls := 0; mixer.Len() > 0 && pulls < 8; pulls++ {
				mixer.Stream(out[:])
				ended, err := p.PlaybackEnd()
				if ended != (mixer.Len() == 0) || err != nil {
					t.Fatalf("mixer entries=%d, ended=%v, err=%v", mixer.Len(), ended, err)
				}
			}
			if mixer.Len() != 0 || p.IsPlaying() {
				t.Fatal("finite audio did not finish")
			}
		})
	}
}

func TestPlayerCompletionPreservesStreamError(t *testing.T) {
	mixer := outputMixer(t)
	p, _ := outputPlayer(t)
	want := errors.New("decoder failed")
	p.attachOutput(&effectSource{remaining: 0, err: want}, false)
	var out [8][2]float64
	mixer.Stream(out[:])
	if ended, err := p.PlaybackEnd(); !ended || !errors.Is(err, want) {
		t.Fatalf("ended=%v, err=%v", ended, err)
	}
}

func TestPlayerSeekAfterCompletionStaysStoppedUntilPlay(t *testing.T) {
	for _, synthetic := range []bool{false, true} {
		t.Run(map[bool]string{false: "decoded", true: "synth"}[synthetic], func(t *testing.T) {
			mixer := outputMixer(t)
			p := completionWAVPlayer(t, 8)
			var backend *outputSynth
			if synthetic {
				backend = &outputSynth{paused: true}
				p.synthCtrl, p.synthActive = backend, true
			}
			if err := p.Play(); err != nil {
				t.Fatal(err)
			}
			if backend != nil {
				backend.finished = true
			}
			var out [8][2]float64
			for range 2 {
				mixer.Stream(out[:])
			}
			if ended, _ := p.PlaybackEnd(); !ended || p.IsPlaying() {
				t.Fatal("fixture did not finish")
			}
			if err := p.Seek(0); err != nil {
				t.Fatal(err)
			}
			if p.IsPlaying() {
				t.Error("seek resumed finished playback")
			}
			mixer.Stream(out[:])
			for _, sample := range out {
				if sample != [2]float64{} {
					t.Fatal("seek produced sound before Play")
				}
			}
			if err := p.Play(); err != nil {
				t.Fatal(err)
			}
			mixer.Stream(out[:])
			if out[0] == [2]float64{} {
				t.Fatal("explicit Play did not resume after seek")
			}
		})
	}
}
