package soundfont

import (
	"math"
	"testing"
)

func TestModulatorSourceCurves(t *testing.T) {
	c := &channel{}
	c.reset()
	c.cc[11] = 64
	for _, tc := range []struct {
		src      uint16
		velocity int32
		want     float64
	}{
		{0, 64, 1}, {2, 127, 1}, {0x102, 127, 0}, {0x502, 127, 0}, {0x502, 0, 1}, {0x502, 64, 0.12400989457169573},
		{0x202, 64, 0}, {0xC02, 63, 0}, {0xC02, 64, 1},
	} {
		got, ok := modSource(tc.src, c, 60, tc.velocity)
		if !ok || math.Abs(got-tc.want) > 1e-6 {
			t.Fatalf("src%x vel%d got%g want%g", tc.src, tc.velocity, got, tc.want)
		}
	}
	if _, ok := modSource(0x1002, c, 60, 64); ok {
		t.Fatal("reserved curve accepted")
	}
}
func TestModulatorMerge(t *testing.T) {
	base := []modulator{{Source: 0x502, Destination: 48, Amount: 960}}
	global := []modulator{{Source: 0x502, Destination: 48, Amount: 500}}
	local := []modulator{{Source: 0x502, Destination: 48, Amount: 0}}
	got := mergeModulators(mergeModulators(base, global), local)
	if len(got) != 1 || got[0].Amount != 0 {
		t.Fatalf("zero amount did not override default: %+v", got)
	}
}
func TestModulatorAmountsAndTransform(t *testing.T) {
	c := &channel{}
	c.reset()
	c.cc[1] = 127
	m := modulator{Source: 0x81, Destination: 17, Amount: -500, AmountSource: 2, Transform: 2}
	if got := m.value(c, 60, 127); got != 500 {
		t.Fatalf("absolute product: %g", got)
	}
}
