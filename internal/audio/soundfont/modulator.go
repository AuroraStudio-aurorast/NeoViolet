package soundfont

import (
	"encoding/binary"
	"math"
)

// SF2 2.04 sections 7.4/7.8, 8.2-8.4 and 9.5 define these records and
// identity/override rules. Amount is signed and is not part of route identity.
type modulator struct {
	Source, Destination     uint16
	Amount                  int16
	AmountSource, Transform uint16
}

func readModulators(b []byte) []modulator {
	mods := make([]modulator, len(b)/10-1)
	for i := range mods {
		v := b[i*10:]
		// #nosec G115 -- SF2 amount is a signed 16-bit field; preserve its wire bit pattern.
		mods[i] = modulator{binary.LittleEndian.Uint16(v), binary.LittleEndian.Uint16(v[2:]), int16(binary.LittleEndian.Uint16(v[4:])), binary.LittleEndian.Uint16(v[6:]), binary.LittleEndian.Uint16(v[8:])}
	}
	return mods
}
func mergeModulators(base, over []modulator) []modulator {
	out := append([]modulator(nil), base...)
	for _, m := range over {
		found := false
		for i, b := range out {
			if m.Source == b.Source && m.Destination == b.Destination && m.AmountSource == b.AmountSource && m.Transform == b.Transform {
				out[i] = m
				found = true
				break
			}
		}
		if !found {
			out = append(out, m)
		}
	}
	return out
}
func (m modulator) value(c *channel, key, velocity int32) float64 {
	src, ok := modSource(m.Source, c, key, velocity)
	if !ok {
		return 0
	}
	amount, ok := modSource(m.AmountSource, c, key, velocity)
	if !ok {
		return 0
	}
	v := src * float64(m.Amount) * amount
	if m.Transform == 2 {
		return math.Abs(v)
	}
	if m.Transform != 0 {
		return 0
	}
	return v
}

var defaultModulators = []modulator{
	{0x502, 48, 960, 0, 0}, {0x102, 8, -2400, 0, 0}, {13, 6, 50, 0, 0}, {0x81, 6, 50, 0, 0},
	{0x587, 48, 960, 0, 0}, {0x28A, 17, 500, 0, 0}, {0x58B, 48, 960, 0, 0},
	{0xDB, 16, 200, 0, 0}, {0xDD, 15, 200, 0, 0}, {0x20E, 52, 12700, 16, 0},
}
