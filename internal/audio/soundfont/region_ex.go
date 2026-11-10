package soundfont

import "math"

func (o *oscillator) startByRegion(data []int16, region regionPair) {
	sampleRate := region.instrument.Sample.SampleRate
	loopMode := region.getSampleModes()
	sampleStart := region.getSampleStart()
	sampleEnd := region.getSampleEnd()
	startLoop := region.getSampleStartLoop()
	endLoop := region.getSampleEndLoop()
	rootKey := region.getRootKey()
	coarseTune := region.getCoarseTune()
	fineTune := region.getFineTune()
	scaleTuning := region.getScaleTuning()

	o.start(data, loopMode, sampleRate, sampleStart, sampleEnd, startLoop, endLoop, rootKey, coarseTune, fineTune, scaleTuning)
}

func (env *volumeEnvelope) startByRegion(region regionPair, key int32, _ int32) {
	// If the release time is shorter than 10 ms, it will be clamped to 10 ms to avoid pop noise.

	delay := region.getDelayVolumeEnvelope()
	attack := region.getAttackVolumeEnvelope()
	hold := region.getHoldVolumeEnvelope() * calcKeyNumberToMultiplyingFactor(region.getKeyNumberToVolumeEnvelopeHold(), key)
	decay := region.getDecayVolumeEnvelope() * calcKeyNumberToMultiplyingFactor(region.getKeyNumberToVolumeEnvelopeDecay(), key)
	sustain := calcDecibelsToLinear(-region.getSustainVolumeEnvelope())
	release := float32(math.Max(float64(region.getReleaseVolumeEnvelope()), 0.01))

	env.start(delay, attack, hold, decay, sustain, release)
}

func (env *modulationEnvelope) startByRegion(region regionPair, key int32, _ int32) {

	delay := region.getDelayModulationEnvelope()
	attack := region.getAttackModulationEnvelope()
	hold := region.getHoldModulationEnvelope() * calcKeyNumberToMultiplyingFactor(region.getKeyNumberToModulationEnvelopeHold(), key)
	decay := region.getDecayModulationEnvelope() * calcKeyNumberToMultiplyingFactor(region.getKeyNumberToModulationEnvelopeDecay(), key)
	sustain := 1 - region.getSustainModulationEnvelope()/100
	release := region.getReleaseModulationEnvelope()

	env.start(delay, attack, hold, decay, sustain, release)
}

func (lfo *lfo) startVibrato(region regionPair, _ int32, _ int32) {
	lfo.start(region.getDelayVibratoLfo(), region.getFrequencyVibratoLfo())
}

func (lfo *lfo) startModulation(region regionPair, _ int32, _ int32) {
	lfo.start(region.getDelayModulationLfo(), region.getFrequencyModulationLfo())
}
