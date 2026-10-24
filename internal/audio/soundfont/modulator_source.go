package soundfont

import "math"

// modSource maps native MIDI data into the SF2 controller domain. MIDI 7-bit
// endpoints are normalized to 127 (pitch wheel keeps its exact 8192 center).
// Concave attenuation uses the 96dB range from SF2 section 8.4.1: at half
// velocity, 960*c is approximately 120cB, giving the intended squared gain.
func modSource(src uint16, c *channel, key, velocity int32) (float64, bool) {
	if src == 0 {
		return 1, true
	}
	index := src & 127
	curve := src >> 10
	if curve > 3 {
		return 0, false
	}
	var x float64
	if src&128 != 0 {
		x = float64(c.cc[index]) / 127
	} else {
		switch index {
		case 2:
			x = float64(velocity) / 127
		case 3:
			x = float64(key) / 127
		case 10:
			if key < 0 || key > 127 {
				return 0, false
			}
			x = float64(c.polyPressure[key]) / 127
		case 13:
			x = float64(c.pressure) / 127
		case 14:
			x = (float64(c.pitchBend) + 1) / 2
		case 16:
			x = float64(c.getPitchBendRange()) / 127
		default:
			return 0, false
		}
	}
	x = math.Max(0, math.Min(1, x))
	bipolar := src&512 != 0
	if bipolar {
		if src&128 == 0 && index == 14 {
			x = 2*x - 1
		} else {
			raw := x * 127
			if raw <= 64 {
				x = (raw - 64) / 64
			} else {
				x = (raw - 64) / 63
			}
		}
		if src&256 != 0 {
			x = -x
		}
	} else if src&256 != 0 {
		x = 1 - x
	}
	sign := float64(1)
	if x < 0 {
		sign = -1
		x = -x
	}
	switch curve {
	case 1:
		switch {
		case x <= 0:
			x = 0
		case x >= 1:
			x = 1
		default:
			x = math.Max(0, math.Min(1, -(40.0/96)*math.Log10(1-x)))
		}
	case 2:
		switch {
		case x <= 0:
			x = 0
		case x >= 1:
			x = 1
		default:
			x = 1 - math.Max(0, math.Min(1, -(40.0/96)*math.Log10(x)))
		}
	case 3:
		if bipolar || x >= 0.5 {
			x = 1
		} else {
			x = 0
		}
	}
	return sign * x, true
}
