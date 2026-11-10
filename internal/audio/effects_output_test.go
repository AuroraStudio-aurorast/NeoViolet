package audio

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/speaker"
)

func TestEffectsPlayerCommonOutputAppliesMasterVolumeOnce(t *testing.T) {
	for _, synthetic := range []bool{false, true} {
		t.Run(map[bool]string{false: "decoded", true: "synth"}[synthetic], func(t *testing.T) {
			mixer := outputMixer(t)
			p, _ := outputPlayer(t)
			input := .125
			if synthetic {
				p.synthCtrl = &outputSynth{paused: true}
				p.synthActive = true
				input = .25
			}
			if err := p.SetEffects(EffectsSettings{PreampDB: -6.020599913279624}); err != nil {
				t.Fatal(err)
			}
			p.SetVolume(.5)
			if err := p.Play(); err != nil {
				t.Fatal(err)
			}
			var out [16][2]float64
			mixer.Stream(out[:])
			want := input * .25
			for _, v := range out {
				if math.Abs(v[0]-want) > 1e-12 || math.Abs(v[1]-want) > 1e-12 {
					t.Fatalf("output %v want %.8f", v, want)
				}
			}
		})
	}
}
func TestEffectsPlayerEditPauseSeekReplayPreservesPreset(t *testing.T) {
	mixer := outputMixer(t)
	p, _ := outputPlayer(t)
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	cfg := EffectsSettings{Mono: true, Pan: 1, Swap: true}
	if err := p.SetEffects(cfg); err != nil {
		t.Fatal(err)
	}
	samples := make([][2]float64, 2000)
	mixer.Stream(samples)
	if samples[1999] != [2]float64{.125, 0} {
		t.Fatal("live edit not applied")
	}
	p.Pause()
	mixer.Stream(samples)
	for _, v := range samples {
		if v != [2]float64{} {
			t.Fatal("paused processor leaked tail")
		}
	}
	if err := p.Seek(time.Second); err != nil {
		t.Fatal(err)
	}
	p.Resume()
	mixer.Stream(samples)
	if samples[1999] != [2]float64{.125, 0} || p.Effects() != cfg {
		t.Fatal("seek lost preset")
	}
	p.Stop()
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	mixer.Stream(samples)
	if samples[0] != [2]float64{.125, 0} {
		t.Fatal("replay lost preset")
	}
}
func TestEffectsPlayerSeekClearsFilterHistory(t *testing.T) {
	outputMixer(t)
	p := NewPlayer()
	source := &seekImpulse{fakeStream: fakeStream{total: 44100}, impulse: true}
	p.setupStreamer(source, beep.Format{SampleRate: 44100, NumChannels: 2, Precision: 2}, nopCloser{}, "impulse.wav")
	defer func() { _ = p.Close() }()
	if err := p.SetEffects(EffectsSettings{Gains: [5]float64{12}}); err != nil {
		t.Fatal(err)
	}
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	var out [16][2]float64
	p.output.Stream(out[:])
	source.impulse = false
	if err := p.Seek(time.Second / 2); err != nil {
		t.Fatal(err)
	}
	p.output.Stream(out[:])
	for _, v := range out {
		if v != [2]float64{} {
			t.Fatal("pre-seek filter history leaked")
		}
	}
}

type seekImpulse struct {
	fakeStream
	impulse bool
}

func (s *seekImpulse) Stream(out [][2]float64) (int, bool) {
	for i := range out {
		out[i] = [2]float64{}
		if s.impulse && s.pos == 0 {
			out[i] = [2]float64{1, 1}
		}
		s.pos++
	}
	return len(out), true
}
func TestEffectsControlUpdatesRaceWithSpeakerPull(t *testing.T) {
	outputMixer(t)
	p, _ := outputPlayer(t)
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	output := p.output
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		var out [64][2]float64
		for range 200 {
			speaker.Lock()
			output.Stream(out[:])
			speaker.Unlock()
		}
	}()
	for i := range 200 {
		if err := p.SetEffects(EffectsSettings{PreampDB: -float64(i % 12), Pan: float64(i%3) - 1}); err != nil {
			t.Fatal(err)
		}
		p.SetVolume(float64(i%100) / 100)
	}
	wg.Wait()
}

func TestEffectsRealTrackerUsesSamePCMChainAndSingleMasterGain(t *testing.T) {
	mixer := outputMixer(t)
	render := func(cfg EffectsSettings, volume float64) [][2]float64 {
		ctrl, err := synth.NewTrackerPlayer("synth/testdata/minimal.mod", ".mod", 44100)
		if err != nil {
			t.Fatal(err)
		}
		player := NewPlayer()
		player.synthActive = true
		player.synthCtrl = ctrl
		if err = player.SetEffects(cfg); err != nil {
			t.Fatal(err)
		}
		player.SetVolume(volume)
		if err = player.Play(); err != nil {
			t.Fatal(err)
		}
		out := make([][2]float64, 4096)
		mixer.Stream(out)
		if err = player.Close(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	reference := render(EffectsSettings{}, 1)
	processed := render(EffectsSettings{PreampDB: -6.020599913279624}, .5)
	energy := 0.
	for i, sample := range reference {
		for ch := range 2 {
			energy += sample[ch] * sample[ch]
			if math.Abs(processed[i][ch]-sample[ch]*.25) > 1e-12 {
				t.Fatalf("tracker master or preamp applied incorrectly at %d: %v vs %v", i, processed[i], sample)
			}
		}
	}
	if energy == 0 {
		t.Fatal("tracker fixture produced no audio")
	}
}

// R 100% must preserve the original right channel even for anti-phase stereo.
func TestEffectsPlayerPanEndpointKeepsAudibleRightChannelAndTransport(t *testing.T) {
	mixer := outputMixer(t)
	p := NewPlayer()
	source := &panStereoStream{fakeStream: fakeStream{total: 44100 * 20}}
	p.setupStreamer(source, beep.Format{SampleRate: 44100, NumChannels: 2, Precision: 2}, nopCloser{}, "anti-phase.wav")
	t.Cleanup(func() { _ = p.Close() })
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	var out [512][2]float64
	for step := 1; step <= 10; step++ {
		if err := p.SetEffects(EffectsSettings{Pan: float64(step) / 10}); err != nil {
			t.Fatal(err)
		}
		mixer.Stream(out[:])
	}
	if !p.IsPlaying() || p.Position() <= 0 {
		t.Fatal("pan adjustment stopped playback progression")
	}
	if ended, err := p.PlaybackEnd(); ended || err != nil {
		t.Fatal("pan endpoint was treated as EOF")
	}
	if got := out[len(out)-1]; got != [2]float64{0, -.25} {
		t.Fatalf("R 100%% lost original right-channel audio: %v", got)
	}
}

type panStereoStream struct{ fakeStream }

func (s *panStereoStream) Stream(out [][2]float64) (int, bool) {
	for i := range out {
		out[i] = [2]float64{.25, -.25}
		s.pos++
	}
	return len(out), true
}
