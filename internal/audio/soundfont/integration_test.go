package soundfont

import (
	"math"
	"os"
	"runtime"
	"testing"
	"time"
)

// TestSoundFontIntegration is opt-in: never requires a user's bank in CI.
func TestSoundFontIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("local SoundFont integration")
	}
	path := os.Getenv("NEOVIOLET_SF2_TEST_FILE")
	if path == "" {
		t.Skip("set NEOVIOLET_SF2_TEST_FILE")
	}
	// #nosec G304,G703 -- opt-in test deliberately reads the local asset specified by the operator.
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	start := time.Now()
	sf, diags, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("load %s: %d samples, %d instruments, %d presets, %v; diagnostics %+v", path, len(sf.WaveData), len(sf.Instruments), len(sf.Presets), time.Since(start), diags)
	runtime.GC()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	t.Logf("GC heap %.1f MiB", float64(mem.HeapAlloc)/(1<<20))
	cfg := NewSynthesizerSettings(44100)
	cfg.MaximumPolyphony = 256
	l, r := make([]float32, 4096), make([]float32, 4096)
	silent := 0
	for _, p := range sf.Presets {
		s, e := NewSynthesizer([]*SoundFont{sf}, cfg)
		if e != nil {
			t.Fatal(e)
		}
		s.SelectPreset(0, p.BankNumber, p.PatchNumber, p.BankNumber >= 128)
		for _, key := range []int32{24, 48, 60, 72, 96} {
			s.NoteOn(0, key, 100)
		}
		s.Render(l, r)
		energy := float64(0)
		for i := range l {
			x, y := float64(l[i]), float64(r[i])
			if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
				t.Fatalf("nonfinite bank%d program%d", p.BankNumber, p.PatchNumber)
			}
			energy += x*x + y*y
		}
		if energy == 0 {
			silent++
			t.Logf("short silence bank%d program%d %s", p.BankNumber, p.PatchNumber, p.Name)
		}
	}
	t.Logf("five-key short sweep complete: %d presets; %d silent (not full key/velocity coverage)", len(sf.Presets), silent)
	if os.Getenv("NEOVIOLET_SF2_SWEEP") != "" {
		cfg.EnableReverbAndChorus = false
		s, err := NewSynthesizer([]*SoundFont{sf}, cfg)
		if err != nil {
			t.Fatal(err)
		}
		cases := 0
		for _, p := range sf.Presets {
			for key := int32(0); key < 128; key++ {
				for _, velocity := range []int32{1, 32, 64, 100, 127} {
					s.Reset()
					s.SelectPreset(0, p.BankNumber, p.PatchNumber, p.BankNumber >= 128)
					s.NoteOn(0, key, velocity)
					s.Render(l[:128], r[:128])
					for i := range 128 {
						if math.IsNaN(float64(l[i])) || math.IsInf(float64(l[i]), 0) || math.IsNaN(float64(r[i])) || math.IsInf(float64(r[i]), 0) {
							t.Fatalf("sweep nonfinite bank%d program%d key%d velocity%d", p.BankNumber, p.PatchNumber, key, velocity)
						}
					}
					cases++
				}
			}
		}
		t.Logf("all-key/5-velocity 128-frame safety sweep: %d cases", cases)
		for _, p := range sf.Presets {
			s.Reset()
			s.SelectPreset(0, p.BankNumber, p.PatchNumber, p.BankNumber >= 128)
			for _, key := range []int32{24, 60, 96} {
				s.NoteOn(0, key, 100)
			}
			s.ProcessMidiMessage(0, 0xE0, 127, 127)
			for range 32 {
				s.Render(l, r)
				for i := range l {
					if math.IsNaN(float64(l[i])) || math.IsInf(float64(l[i]), 0) || math.IsNaN(float64(r[i])) || math.IsInf(float64(r[i]), 0) {
						t.Fatalf("long bend nonfinite bank%d program%d", p.BankNumber, p.PatchNumber)
					}
				}
			}
		}
		t.Logf("long loop/bend spot check: %d presets x 131072 frames", len(sf.Presets))
	}
}
