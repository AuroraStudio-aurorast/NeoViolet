package streamcore

import (
	"testing"
)

func TestCopyToOutput(t *testing.T) {
	sc := &Core{
		Buf:          []float64{1, 2, 3, 4},
		BufSamples:   2,
		NumChannels:  2,
		TotalSamples: 2,
	}
	out := make([][2]float64, 4)
	n := sc.CopyToOutput(out, 4, 0)
	if n != 2 {
		t.Errorf("CopyToOutput returned %d, want 2", n)
	}
	if out[0] != [2]float64{1, 2} || out[1] != [2]float64{3, 4} {
		t.Errorf("out = %v", out[:2])
	}
	if sc.Pos != 2 || sc.CurrentSample != 2 {
		t.Errorf("Pos = %d, CurrentSample = %d after copy, want 2/2", sc.Pos, sc.CurrentSample)
	}
}

func TestCopyToOutputMono(t *testing.T) {
	// Buf is always laid out as interleaved stereo pairs; mono input is
	// duplicated into both channels by Int16ToFloat64 before reaching here.
	sc := &Core{
		Buf:         []float64{5, 5, 6, 6},
		BufSamples:  2,
		NumChannels: 1,
	}
	out := make([][2]float64, 2)
	n := sc.CopyToOutput(out, 2, 0)
	if n != 2 {
		t.Fatalf("CopyToOutput returned %d, want 2", n)
	}
	if out[0] != [2]float64{5, 5} || out[1] != [2]float64{6, 6} {
		t.Errorf("mono duplication failed: out = %v", out)
	}
}

func TestCopyToOutputWithOffset(t *testing.T) {
	// totalFilled > 0: CopyToOutput must start writing at samples[totalFilled]
	// and leave the already-filled prefix untouched.
	sc := &Core{
		Buf:          []float64{1, 2, 3, 4, 5, 6, 7, 8},
		BufSamples:   4,
		NumChannels:  2,
		TotalSamples: 4,
	}
	out := [][2]float64{{100, 100}, {200, 200}, {0, 0}, {0, 0}}
	n := sc.CopyToOutput(out, 4, 2)
	if n != 2 {
		t.Errorf("CopyToOutput returned %d, want 2", n)
	}
	if out[2] != [2]float64{1, 2} || out[3] != [2]float64{3, 4} {
		t.Errorf("copy did not start at samples[2]: out = %v", out)
	}
	if out[0] != [2]float64{100, 100} || out[1] != [2]float64{200, 200} {
		t.Errorf("pre-filled prefix was overwritten: out = %v", out[:2])
	}
	if sc.Pos != 2 || sc.CurrentSample != 2 {
		t.Errorf("Pos = %d, CurrentSample = %d after copy, want 2/2", sc.Pos, sc.CurrentSample)
	}
}

func TestCopyToOutputTruncated(t *testing.T) {
	// Output buffer smaller than the remaining data: framesToCopy must be
	// capped by totalNeeded-totalFilled so the copy count is limited.
	sc := &Core{
		Buf:          []float64{1, 2, 3, 4, 5, 6, 7, 8},
		BufSamples:   4,
		NumChannels:  2,
		TotalSamples: 4,
	}
	out := make([][2]float64, 1)
	n := sc.CopyToOutput(out, 1, 0)
	if n != 1 {
		t.Errorf("CopyToOutput returned %d, want 1 (capped by output capacity)", n)
	}
	if out[0] != [2]float64{1, 2} {
		t.Errorf("out[0] = %v, want {1 2}", out[0])
	}
	if sc.Pos != 1 || sc.CurrentSample != 1 {
		t.Errorf("Pos = %d, CurrentSample = %d after copy, want 1/1", sc.Pos, sc.CurrentSample)
	}
}

func TestCoreLifecycle(t *testing.T) {
	sc := &Core{TotalSamples: 100, CurrentSample: 42}
	if sc.Len() != 100 {
		t.Errorf("Len = %d, want 100", sc.Len())
	}
	if sc.Position() != 42 {
		t.Errorf("Position = %d, want 42", sc.Position())
	}
	sc.ResetBuffer()
	if sc.Buf != nil || sc.BufSamples != 0 || sc.Pos != 0 {
		t.Errorf("ResetBuffer: Buf = %v, BufSamples = %d, Pos = %d, want nil/0/0",
			sc.Buf, sc.BufSamples, sc.Pos)
	}
	if sc.CurrentSample != 42 {
		t.Errorf("CurrentSample = %d after ResetBuffer, want 42 (ResetBuffer does not reset stream position)",
			sc.CurrentSample)
	}
	if sc.Err() != nil {
		t.Error("Err should be nil")
	}
	sc.Close()
	if !sc.Closed {
		t.Error("Close should set Closed")
	}
}

func TestInt16ToFloat64(t *testing.T) {
	in := []int16{0, 1, -1, 32767, -32768}
	out := Int16ToFloat64(in, 1)
	want := []float64{
		0, 0,
		1.0 / 32768, 1.0 / 32768,
		-1.0 / 32768, -1.0 / 32768,
		32767.0 / 32768, 32767.0 / 32768,
		-1, -1,
	}
	if len(out) != len(want) {
		t.Fatalf("Int16ToFloat64 returned %d samples, want %d", len(out), len(want))
	}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("Int16ToFloat64[%d] = %v, want %v", i, out[i], want[i])
		}
	}
}

func TestInt16ToFloat64Stereo(t *testing.T) {
	in := []int16{1, 2, 3, 4}
	out := Int16ToFloat64(in, 2)
	want := []float64{1.0 / 32768, 2.0 / 32768, 3.0 / 32768, 4.0 / 32768}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("Int16ToFloat64 stereo[%d] = %v, want %v", i, out[i], want[i])
		}
	}
}

func TestMinInt(t *testing.T) {
	if got := MinInt(3, 5); got != 3 {
		t.Errorf("MinInt(3,5) = %d", got)
	}
	if got := MinInt(-1, -2); got != -2 {
		t.Errorf("MinInt(-1,-2) = %d", got)
	}
}
