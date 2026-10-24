package soundfont

type regionPair struct {
	preset     *PresetRegion
	instrument *InstrumentRegion
	values     *[61]float32
}

func newRegionPair(preset *PresetRegion, inst *InstrumentRegion) regionPair {
	var result regionPair

	result.preset = preset
	result.instrument = inst

	return result
}

func (region regionPair) getGeneratorValue(generatorType uint16) int32 {
	if region.values != nil {
		return int32(region.values[generatorType])
	}
	return int32(region.preset.gs[generatorType]) + int32(region.instrument.gs[generatorType])
}

func (region regionPair) getSampleStart() int32 {
	return region.instrument.getSampleStart()
}

func (region regionPair) getSampleEnd() int32 {
	return region.instrument.getSampleEnd()
}

func (region regionPair) getSampleStartLoop() int32 {
	return region.instrument.getSampleStartLoop()
}

func (region regionPair) getSampleEndLoop() int32 {
	return region.instrument.getSampleEndLoop()
}

func (region regionPair) getModulationLfoToPitch() int32 {
	return region.getGeneratorValue(genModulationLfoToPitch)
}

func (region regionPair) getVibratoLfoToPitch() int32 {
	return region.getGeneratorValue(genVibratoLfoToPitch)
}

func (region regionPair) getModulationEnvelopeToPitch() int32 {
	return region.getGeneratorValue(genModulationEnvelopeToPitch)
}

func (region regionPair) getInitialFilterCutoffFrequency() float32 {
	return calcCentsToHertz(float32(region.getGeneratorValue(genInitialFilterCutoffFrequency)))
}

func (region regionPair) getInitialFilterQ() float32 {
	return float32(0.1) * float32(region.getGeneratorValue(genInitialFilterQ))
}

func (region regionPair) getModulationLfoToFilterCutoffFrequency() int32 {
	return region.getGeneratorValue(genModulationLfoToFilterCutoffFrequency)
}

func (region regionPair) getModulationEnvelopeToFilterCutoffFrequency() int32 {
	return region.getGeneratorValue(genModulationEnvelopeToFilterCutoffFrequency)
}

func (region regionPair) getModulationLfoToVolume() float32 {
	return float32(0.1) * float32(region.getGeneratorValue(genModulationLfoToVolume))
}

func (region regionPair) getChorusEffectsSend() float32 {
	return float32(0.1) * float32(region.getGeneratorValue(genChorusEffectsSend))
}

func (region regionPair) getReverbEffectsSend() float32 {
	return float32(0.1) * float32(region.getGeneratorValue(genReverbEffectsSend))
}

func (region regionPair) getPan() float32 {
	return float32(0.1) * float32(region.getGeneratorValue(genPan))
}

func (region regionPair) getDelayModulationLfo() float32 {
	return calcTimecentsToSeconds(float32(region.getGeneratorValue(genDelayModulationLfo)))
}

func (region regionPair) getFrequencyModulationLfo() float32 {
	return calcCentsToHertz(float32(region.getGeneratorValue(genFrequencyModulationLfo)))
}

func (region regionPair) getDelayVibratoLfo() float32 {
	return calcTimecentsToSeconds(float32(region.getGeneratorValue(genDelayVibratoLfo)))
}

func (region regionPair) getFrequencyVibratoLfo() float32 {
	return calcCentsToHertz(float32(region.getGeneratorValue(genFrequencyVibratoLfo)))
}

func (region regionPair) getDelayModulationEnvelope() float32 {
	return calcTimecentsToSeconds(float32(region.getGeneratorValue(genDelayModulationEnvelope)))
}

func (region regionPair) getAttackModulationEnvelope() float32 {
	return calcTimecentsToSeconds(float32(region.getGeneratorValue(genAttackModulationEnvelope)))
}

func (region regionPair) getHoldModulationEnvelope() float32 {
	return calcTimecentsToSeconds(float32(region.getGeneratorValue(genHoldModulationEnvelope)))
}

func (region regionPair) getDecayModulationEnvelope() float32 {
	return calcTimecentsToSeconds(float32(region.getGeneratorValue(genDecayModulationEnvelope)))
}

func (region regionPair) getSustainModulationEnvelope() float32 {
	return float32(0.1) * float32(region.getGeneratorValue(genSustainModulationEnvelope))
}

func (region regionPair) getReleaseModulationEnvelope() float32 {
	return calcTimecentsToSeconds(float32(region.getGeneratorValue(genReleaseModulationEnvelope)))
}

func (region regionPair) getKeyNumberToModulationEnvelopeHold() int32 {
	return region.getGeneratorValue(genKeyNumberToModulationEnvelopeHold)
}

func (region regionPair) getKeyNumberToModulationEnvelopeDecay() int32 {
	return region.getGeneratorValue(genKeyNumberToModulationEnvelopeDecay)
}

func (region regionPair) getDelayVolumeEnvelope() float32 {
	return calcTimecentsToSeconds(float32(region.getGeneratorValue(genDelayVolumeEnvelope)))
}

func (region regionPair) getAttackVolumeEnvelope() float32 {
	return calcTimecentsToSeconds(float32(region.getGeneratorValue(genAttackVolumeEnvelope)))
}

func (region regionPair) getHoldVolumeEnvelope() float32 {
	return calcTimecentsToSeconds(float32(region.getGeneratorValue(genHoldVolumeEnvelope)))
}

func (region regionPair) getDecayVolumeEnvelope() float32 {
	return calcTimecentsToSeconds(float32(region.getGeneratorValue(genDecayVolumeEnvelope)))
}

func (region regionPair) getSustainVolumeEnvelope() float32 {
	return float32(0.1) * float32(region.getGeneratorValue(genSustainVolumeEnvelope))
}

func (region regionPair) getReleaseVolumeEnvelope() float32 {
	return calcTimecentsToSeconds(float32(region.getGeneratorValue(genReleaseVolumeEnvelope)))
}

func (region regionPair) getKeyNumberToVolumeEnvelopeHold() int32 {
	return region.getGeneratorValue(genKeyNumberToVolumeEnvelopeHold)
}

func (region regionPair) getKeyNumberToVolumeEnvelopeDecay() int32 {
	return region.getGeneratorValue(genKeyNumberToVolumeEnvelopeDecay)
}

func (region regionPair) getInitialAttenuation() float32 {
	return float32(0.1) * float32(region.getGeneratorValue(genInitialAttenuation))
}

func (region regionPair) getCoarseTune() int32 {
	return region.getGeneratorValue(genCoarseTune)
}

func (region regionPair) getFineTune() int32 {
	return region.getGeneratorValue(genFineTune) + int32(region.instrument.Sample.PitchCorrection)
}

func (region regionPair) getSampleModes() int32 {
	return region.instrument.getSampleModes()
}

func (region regionPair) getScaleTuning() int32 {
	return region.getGeneratorValue(genScaleTuning)
}

func (region regionPair) getExclusiveClass() int32 {
	return region.instrument.getExclusiveClass()
}

func (region regionPair) getRootKey() int32 {
	return region.instrument.getRootKey()
}
