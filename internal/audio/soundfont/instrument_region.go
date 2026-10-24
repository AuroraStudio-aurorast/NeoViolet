package soundfont

import (
	"fmt"
)

// InstrumentRegion holds immutable generators, routes and validated sample addresses.
type InstrumentRegion struct {
	Sample                         *SampleHeader
	gs                             [61]int16
	valid                          bool
	modulators                     []modulator
	start, end, loopStart, loopEnd int32
}

func createInstrumentRegion(inst *Instrument, global *zone, local *zone, samples []*SampleHeader) (*InstrumentRegion, error) {
	result := new(InstrumentRegion)

	result.gs[genInitialFilterCutoffFrequency] = 13500
	result.gs[genDelayModulationLfo] = -12000
	result.gs[genDelayVibratoLfo] = -12000
	result.gs[genDelayModulationEnvelope] = -12000
	result.gs[genAttackModulationEnvelope] = -12000
	result.gs[genHoldModulationEnvelope] = -12000
	result.gs[genDecayModulationEnvelope] = -12000
	result.gs[genReleaseModulationEnvelope] = -12000
	result.gs[genDelayVolumeEnvelope] = -12000
	result.gs[genAttackVolumeEnvelope] = -12000
	result.gs[genHoldVolumeEnvelope] = -12000
	result.gs[genDecayVolumeEnvelope] = -12000
	result.gs[genReleaseVolumeEnvelope] = -12000
	result.gs[genKeyRange] = 0x7F00
	result.gs[genVelocityRange] = 0x7F00
	result.gs[genKeyNumber] = -1
	result.gs[genVelocity] = -1
	result.gs[genScaleTuning] = 100
	result.gs[genOverridingRootKey] = -1

	for i := 0; i < len(global.generators); i++ {
		result.setParameter(global.generators[i])
	}

	for i := 0; i < len(local.generators); i++ {
		result.setParameter(local.generators[i])
	}

	id := result.gs[genSampleID]
	if 0 > id || int(id) >= len(samples) {
		return nil, fmt.Errorf("the instrument %q contains an invalid sample id %d", inst.Name, id)
	}
	result.Sample = samples[id]
	result.modulators = mergeModulators(mergeModulators(defaultModulators, global.modulators), local.modulators)

	return result, nil
}

func createInstrumentRegions(inst *Instrument, zones []*zone, samples []*SampleHeader) ([]*InstrumentRegion, error) {
	var err error

	// Is the first one the global zone?
	if len(zones[0].generators) == 0 || zones[0].generators[len(zones[0].generators)-1].generatorType != genSampleID {

		// The first one is the global zone.
		global := zones[0]

		// The global zone is regarded as the base setting of subsequent zones.
		count := len(zones) - 1
		regions := make([]*InstrumentRegion, count)
		for i := 0; i < count; i++ {
			regions[i], err = createInstrumentRegion(inst, global, zones[i+1], samples)
			if err != nil {
				return nil, err
			}
		}
		return regions, nil
	}
	regions := make([]*InstrumentRegion, len(zones))
	for i := range zones {
		regions[i], err = createInstrumentRegion(inst, createEmptyZone(), zones[i], samples)
		if err != nil {
			return nil, err
		}
	}
	return regions, nil
}

func (region *InstrumentRegion) setParameter(param generator) {
	index := param.generatorType

	// Unknown generators should be ignored.
	if 0 <= int(index) && int(index) < len(region.gs) {
		// #nosec G115 -- SF2 generator amounts are signed 16-bit bit patterns by specification.
		region.gs[index] = int16(param.value)
	}
}

func (region *InstrumentRegion) contains(key int32, velocity int32) bool {
	containsKey := region.getKeyRangeStart() <= key && key <= region.getKeyRangeEnd()
	containsVelocity := region.getVelocityRangeStart() <= velocity && velocity <= region.getVelocityRangeEnd()
	return region.valid && containsKey && containsVelocity
}

func (region *InstrumentRegion) getSampleStart() int32 {
	return region.start
}

func (region *InstrumentRegion) getSampleEnd() int32 {
	return region.end
}

func (region *InstrumentRegion) getSampleStartLoop() int32 {
	return region.loopStart
}

func (region *InstrumentRegion) getSampleEndLoop() int32 {
	return region.loopEnd
}

func (region *InstrumentRegion) getKeyRangeStart() int32 {
	return int32(region.gs[genKeyRange]) & 0xFF
}

func (region *InstrumentRegion) getKeyRangeEnd() int32 {
	return (int32(region.gs[genKeyRange]) >> 8) & 0xFF
}

func (region *InstrumentRegion) getVelocityRangeStart() int32 {
	return int32(region.gs[genVelocityRange]) & 0xFF
}

func (region *InstrumentRegion) getVelocityRangeEnd() int32 {
	return (int32(region.gs[genVelocityRange]) >> 8) & 0xFF
}

func (region *InstrumentRegion) getSampleModes() int32 {
	if region.gs[genSampleModes] != 2 {
		return int32(region.gs[genSampleModes])
	}
	return loopNoLoop
}

func (region *InstrumentRegion) getExclusiveClass() int32 {
	return int32(region.gs[genExclusiveClass])
}

func (region *InstrumentRegion) getRootKey() int32 {
	if region.gs[genOverridingRootKey] != -1 {
		return int32(region.gs[genOverridingRootKey])
	}
	return int32(region.Sample.OriginalPitch)
}
