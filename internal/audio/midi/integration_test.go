package midi

import (
	"io"
	"math"
	"os"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/soundfont"
)

func TestMIDIIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("local integration")
	}
	path, bank := os.Getenv("NEOVIOLET_MIDI_TEST_FILE"), os.Getenv("NEOVIOLET_SF2_TEST_FILE")
	if path == "" || bank == "" {
		t.Skip("set NEOVIOLET_MIDI_TEST_FILE and NEOVIOLET_SF2_TEST_FILE")
	}
	// #nosec G304,G703 -- opt-in test deliberately reads the local asset specified by the operator.
	f, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	s, d, e := Parse(f)
	_ = f.Close()
	if e != nil {
		t.Fatal(e)
	}
	tl, e := s.Compile(44100)
	if e != nil {
		t.Fatal(e)
	}
	ports := map[uint8]bool{}
	for _, x := range s.Events {
		ports[x.Port] = true
	}
	t.Logf("parse %v events=%d ports=%d content=%v total=%v diagnostics=%+v", time.Since(start), len(s.Events), len(ports), FramesDuration(tl.ContentFrames, 44100), FramesDuration(tl.TotalFrames, 44100), d)
	// #nosec G304,G703 -- opt-in test deliberately reads the local asset specified by the operator.
	f, e = os.Open(bank)
	if e != nil {
		t.Fatal(e)
	}
	sf, e := soundfont.NewSoundFont(f)
	_ = f.Close()
	if e != nil {
		t.Fatal(e)
	}
	ref, e := NewRenderer(tl, sf)
	if e != nil {
		t.Fatal(e)
	}
	cache := NewCheckpoints(44100, 64<<20)
	useCheckpoints := os.Getenv("NEOVIOLET_MIDI_CHECKPOINTS") != ""
	targets := []int64{0, 774043, 1093680, 3748500, 10433001, 11358043, 11781756}
	windows := map[int64][][2]float32{}
	l, r := make([]float32, 512), make([]float32, 512)
	start = time.Now()
	var clipped uint64
	index := 0
	for ref.PositionFrames() < tl.TotalFrames {
		pos := ref.PositionFrames()
		count := 512
		if index < len(targets) && pos < targets[index] {
			count = int(min(int64(count), targets[index]-pos))
		}
		if index < len(targets) && pos == targets[index] {
			count = 512
			windows[pos] = make([][2]float32, 0, 512)
			index++
		}
		n, err := ref.Render(l[:count], r[:count])
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
		for i := range n {
			if math.IsNaN(float64(l[i])) || math.IsInf(float64(l[i]), 0) || math.IsNaN(float64(r[i])) || math.IsInf(float64(r[i]), 0) {
				t.Fatalf("nonfinite frame%d", pos+int64(i))
			}
			if l[i] > 1 || l[i] < -1 || r[i] > 1 || r[i] < -1 {
				clipped++
			}
		}
		if _, ok := windows[pos]; ok {
			for i := range n {
				windows[pos] = append(windows[pos], [2]float32{l[i], r[i]})
			}
		}
		if n == 0 {
			t.Fatal("no progress")
		}
		if useCheckpoints {
			cache.Capture(ref)
		}
	}
	if useCheckpoints {
		t.Logf("checkpoint cache %+v single state=%d bytes", cache.Stats(), ref.StateBytes())
	}
	t.Logf("full render %v clipped(before user gain)=%d stats=%+v", time.Since(start), clipped, ref.Stats())
	for _, target := range targets {
		want, ok := windows[target]
		if !ok {
			continue
		}
		start = time.Now()
		var draft *Renderer
		if useCheckpoints {
			draft = cache.Restore(target)
		}
		if draft == nil {
			var err error
			draft, err = NewRenderer(tl, sf)
			if err != nil {
				t.Fatal(err)
			}
		}
		from := draft.PositionFrames()
		for draft.PositionFrames() < target {
			n := int(min(int64(512), target-draft.PositionFrames()))
			if _, err := draft.Render(l[:n], r[:n]); err != nil && err != io.EOF {
				t.Fatal(err)
			}
		}
		n, err := draft.Render(l[:len(want)], r[:len(want)])
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
		if n != len(want) {
			t.Fatal("seek window length")
		}
		for i, w := range want {
			if math.Abs(float64(l[i]-w[0])) > 1e-6 || math.Abs(float64(r[i]-w[1])) > 1e-6 {
				t.Fatalf("replay mismatch target%d frame%d", target, i)
			}
		}
		t.Logf("seek replay %v from %v took %v", FramesDuration(target, 44100), FramesDuration(from, 44100), time.Since(start))
	}
}
