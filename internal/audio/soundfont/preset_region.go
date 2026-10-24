package soundfont

import (
	"fmt"
)

// PresetRegion adds preset-level articulation to an instrument.
type PresetRegion struct {
	Instrument *Instrument
	gs         [61]int16
	modulators []modulator
}

func createPresetRegion(preset *Preset, global *zone, local *zone, instruments []*Instrument) (*PresetRegion, error) {
	result := new(PresetRegion)

	result.gs[genKeyRange] = 0x7F00
	result.gs[genVelocityRange] = 0x7F00

	for i := 0; i < len(global.generators); i++ {
		result.setParameter(global.generators[i])
	}

	for i := 0; i < len(local.generators); i++ {
		result.setParameter(local.generators[i])
	}

	id := result.gs[genInstrument]
	if 0 > id || int(id) >= len(instruments) {
		return nil, fmt.Errorf("the preset %q contains an invalid instrument id %d", preset.Name, id)
	}
	result.Instrument = instruments[id]
	result.modulators = mergeModulators(global.modulators, local.modulators)

	return result, nil
}

func createPresetRegions(preset *Preset, zones []*zone, instruments []*Instrument) ([]*PresetRegion, error) {

	var global *zone
	var err error

	// Is the first one the global zone?
	if len(zones[0].generators) == 0 || zones[0].generators[len(zones[0].generators)-1].generatorType != genInstrument {

		// The first one is the global zone.
		global = zones[0]

		// The global zone is regarded as the base setting of subsequent zones.
		count := len(zones) - 1
		regions := make([]*PresetRegion, count)
		for i := 0; i < count; i++ {
			regions[i], err = createPresetRegion(preset, global, zones[i+1], instruments)
			if err != nil {
				return nil, err
			}
		}
		return regions, nil
	}
	regions := make([]*PresetRegion, len(zones))
	for i := range zones {
		regions[i], err = createPresetRegion(preset, createEmptyZone(), zones[i], instruments)
		if err != nil {
			return nil, err
		}
	}
	return regions, nil
}

func (region *PresetRegion) setParameter(param generator) {

	index := param.generatorType

	// Unknown generators should be ignored.
	if 0 <= int(index) && int(index) < len(region.gs) {
		// #nosec G115 -- SF2 generator amounts are signed 16-bit bit patterns by specification.
		region.gs[index] = int16(param.value)
	}
}

func (region *PresetRegion) contains(key int32, velocity int32) bool {
	containsKey := region.getKeyRangeStart() <= key && key <= region.getKeyRangeEnd()
	containsVelocity := region.getVelocityRangeStart() <= velocity && velocity <= region.getVelocityRangeEnd()
	return containsKey && containsVelocity
}

func (region *PresetRegion) getKeyRangeStart() int32 {
	return int32(region.gs[genKeyRange]) & 0xFF
}

func (region *PresetRegion) getKeyRangeEnd() int32 {
	return (int32(region.gs[genKeyRange]) >> 8) & 0xFF
}

func (region *PresetRegion) getVelocityRangeStart() int32 {
	return int32(region.gs[genVelocityRange]) & 0xFF
}

func (region *PresetRegion) getVelocityRangeEnd() int32 {
	return (int32(region.gs[genVelocityRange]) >> 8) & 0xFF
}
