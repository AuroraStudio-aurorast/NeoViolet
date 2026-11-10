package soundfont

import "fmt"

func validDestination(d uint16) bool {
	switch d {
	case 5, 6, 7, 8, 9, 10, 11, 13, 15, 16, 17, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 48, 51, 52, 56:
		return true
	}
	return false
}
func validateModulators(sf *SoundFont) {
	validate := func(list []modulator) {
		for _, m := range list {
			_, a := modSource(m.Source, &channel{}, 60, 100)
			_, b := modSource(m.AmountSource, &channel{}, 60, 100)
			if !a || !b || !validDestination(m.Destination) || (m.Transform != 0 && m.Transform != 2) {
				sf.diagnose("unsupported_modulator", fmt.Sprintf("source=%04x amountSource=%04x dest=%d transform=%d", m.Source, m.AmountSource, m.Destination, m.Transform))
			}
		}
	}
	for _, i := range sf.Instruments {
		for _, r := range i.Regions {
			validate(r.modulators)
		}
	}
	for _, p := range sf.Presets {
		for _, r := range p.Regions {
			validate(r.modulators)
		}
	}
}
func evaluateRegion(pair regionPair, c *channel, key, velocity int32, out *[61]float32) {
	for i := range out {
		out[i] = float32(pair.instrument.gs[i]) + float32(pair.preset.gs[i])
	}
	for _, list := range [][]modulator{pair.instrument.modulators, pair.preset.modulators} {
		for _, m := range list {
			if validDestination(m.Destination) {
				out[m.Destination] += float32(m.value(c, key, velocity))
			}
		}
	}
	for i := range out {
		lo, hi := generatorLimits(uint16(i))
		out[i] = calcClamp(out[i], lo, hi)
	}
}
func generatorLimits(g uint16) (float32, float32) {
	switch g {
	case 5, 6, 7, 10, 11:
		return -12000, 12000
	case 8:
		return 1500, 13500
	case 9:
		return 0, 960
	case 13:
		return -960, 960
	case 15, 16, 29:
		return 0, 1000
	case 17:
		return -500, 500
	case 21, 23, 25, 27, 33, 35:
		return -12000, 5000
	case 22, 24:
		return -16000, 4500
	case 26, 28, 30, 34, 36, 38:
		return -12000, 8000
	case 31, 32, 39, 40:
		return -1200, 1200
	case 37, 48:
		return 0, 1440
	case 51:
		return -120, 120
	// Pitch wheel's default initial pitch is represented in the cents summing
	// node. Its allowed excursion is larger than a static fineTune generator.
	case 52:
		return -12000, 12000
	case 56:
		return 0, 1200
	}
	return -32768, 32767
}
