package audio

import (
	"math"
	"testing"

	"github.com/gopxl/beep/v2"
)

type effectSignal struct {
	index, total    int
	frequency, rate float64
	impulse         bool
}

func (s *effectSignal) Stream(out [][2]float64) (int, bool) {
	n := min(len(out), s.total-s.index)
	for i := range out[:n] {
		value := math.Sin(2*math.Pi*s.frequency*float64(s.index)/s.rate) * .1
		if s.impulse {
			value = 0
			if s.index == 0 {
				value = .1
			}
		}
		out[i] = [2]float64{value, value}
		s.index++
	}
	return n, n > 0
}
func (s *effectSignal) Err() error { return nil }
func measureEffect(t *testing.T, rate beep.SampleRate, frequency float64, cfg EffectsSettings) float64 {
	t.Helper()
	source := &effectSignal{total: int(rate), rate: float64(rate), frequency: frequency}
	processor := newEffectsProcessor(source, rate, cfg)
	var out [257][2]float64
	sum := 0.
	count := 0
	for {
		n, ok := processor.Stream(out[:])
		for i, v := range out[:n] {
			if math.IsNaN(v[0]) || math.IsInf(v[0], 0) {
				t.Fatal("unstable EQ")
			}
			if source.index-n+i > int(rate)/2 {
				sum += v[0] * v[0]
				count++
			}
		}
		if !ok {
			break
		}
	}
	return math.Sqrt(sum / float64(count))
}
func TestEffectsPeakingBandsMatchKnownCenterGain(t *testing.T) {
	for i, frequency := range []float64{80, 250, 1000, 4000, 12000} {
		for _, gain := range []float64{-12, -6, 6, 12} {
			cfg := EffectsSettings{}
			cfg.Gains[i] = gain
			got := measureEffect(t, 48000, frequency, cfg) / measureEffect(t, 48000, frequency, EffectsSettings{})
			want := math.Pow(10, gain/20)
			if math.Abs(got-want) > want*.002 {
				t.Fatalf("band %v gain %v: ratio %.6f want %.6f", frequency, gain, got, want)
			}
		}
	}
}
func TestEffectsImpulseResponseIndependentOfBlockBoundaries(t *testing.T) {
	cfg := EffectsSettings{Gains: [5]float64{12, -12, 6, -6, 12}, PreampDB: -6}
	render := func(block int) [][2]float64 {
		s := newEffectsProcessor(&effectSignal{total: 2000, rate: 44100, impulse: true}, 44100, cfg)
		result := make([][2]float64, 0, 2000)
		buf := make([][2]float64, block)
		for {
			n, ok := s.Stream(buf)
			result = append(result, buf[:n]...)
			if !ok {
				break
			}
		}
		return result
	}
	one := render(1)
	for _, block := range []int{2, 7, 64, 257, 4096} {
		other := render(block)
		if len(other) != 2000 {
			t.Fatal("length changed")
		}
		for i := range one {
			if one[i] != other[i] {
				t.Fatalf("block %d differs at %d", block, i)
			}
		}
	}
}
func TestEffectsLowRateSkipsUnrepresentableBands(t *testing.T) {
	cfg := EffectsSettings{Gains: [5]float64{0, 0, 0, 12, -12}}
	got := effectSamples(t, cfg, [2]float64{0, 0})
	if got != [2]float64{} {
		t.Fatal("silence changed")
	}
	processor := newEffectsProcessor(&effectSource{frame: [2]float64{.3, -.2}, remaining: -1}, 8000, cfg)
	out := make([][2]float64, 10)
	processor.Stream(out)
	for _, v := range out {
		if v != [2]float64{.3, -.2} {
			t.Fatal("Nyquist/out-of-range band applied")
		}
	}
}
func TestEffectsTransitionsAreBoundedAndLatestPresetWins(t *testing.T) {
	processor := newEffectsProcessor(&effectSource{frame: [2]float64{.5, -.5}, remaining: -1}, 44100, EffectsSettings{})
	processor.desired = prepareEffects(EffectsSettings{PreampDB: -12, Mono: true}, 44100)
	var out [1][2]float64
	processor.Stream(out[:])
	if math.Abs(out[0][0]-.5) > .002 {
		t.Fatal("edit caused discontinuity")
	}
	for i := range 30 {
		processor.desired = prepareEffects(EffectsSettings{Pan: float64(i%3) - 1, Swap: i%2 == 0, Gains: [5]float64{12, -12, 6, -6, 12}}, 44100)
		processor.Stream(out[:])
		if math.IsNaN(out[0][0]) || math.Abs(out[0][0]) > 2 {
			t.Fatal("rapid update unstable")
		}
	}
	processor.desired = prepareEffects(EffectsSettings{Bypass: true}, 44100)
	buffer := make([][2]float64, 2000)
	processor.Stream(buffer)
	if buffer[len(buffer)-1] != [2]float64{.5, -.5} {
		t.Fatal("latest bypass not applied")
	}
	if allocations := testing.AllocsPerRun(50, func() { processor.Stream(buffer) }); allocations != 0 {
		t.Fatalf("callback allocates %v", allocations)
	}
}
func TestEffectsPanDoesNotOverloadFullScaleStereo(t *testing.T) {
	for _, pan := range []float64{-1, -.5, 0, .5, 1} {
		got := effectSamples(t, EffectsSettings{Pan: pan}, [2]float64{1, 1})
		for _, v := range got {
			if v > 1 || v < 0 {
				t.Fatalf("pan %v overloaded %v", pan, got)
			}
		}
	}
}

func TestEffectsActiveTransitionDoesNotAllocateInCallback(t *testing.T) {
	processor := newEffectsProcessor(&effectSource{frame: [2]float64{.1, .2}, remaining: -1}, 44100, EffectsSettings{Gains: [5]float64{12, -12, 12, -12, 12}})
	var buffer [257][2]float64
	presetA := prepareEffects(EffectsSettings{Gains: [5]float64{-12, 12, -12, 12, -12}, Pan: -1, Mono: true}, 44100)
	presetB := prepareEffects(EffectsSettings{PreampDB: -12, Swap: true}, 44100)
	allocations := testing.AllocsPerRun(50, func() {
		processor.desired = presetA
		processor.Stream(buffer[:])
		processor.desired = presetB
		processor.Stream(buffer[:])
	})
	if allocations != 0 {
		t.Fatalf("active filter/transition allocated %v", allocations)
	}
}
