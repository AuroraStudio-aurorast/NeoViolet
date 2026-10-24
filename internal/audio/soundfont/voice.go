package soundfont

import "math"

const (
	voicePlaying          int32 = 0
	voiceReleaseRequested int32 = 1
	voiceReleased         int32 = 2
)

type voice struct {
	synthesizer *Synthesizer

	volEnv *volumeEnvelope
	modEnv *modulationEnvelope

	vibLfo *lfo
	modLfo *lfo

	oscillator *oscillator
	filter     *biQuadFilter

	block     []float32
	region    regionPair
	modValues [61]float32
	killed    bool

	// A sudden change in the mix gain will cause pop noise.
	// To avoid this, we save the mix gain of the previous block,
	// and smooth out the gain if the gap between the current and previous gain is too large.
	// The actual smoothing process is done in the WriteBlock method of the Synthesizer class.

	previousMixGainLeft  float32
	previousMixGainRight float32
	currentMixGainLeft   float32
	currentMixGainRight  float32

	previousReverbSend float32
	previousChorusSend float32
	currentReverbSend  float32
	currentChorusSend  float32

	exclusiveClass int32
	channel        int32
	key            int32
	velocity       int32

	noteGain float32

	cutoff    float32
	resonance float32

	vibLfoToPitch float32
	modLfoToPitch float32
	modEnvToPitch float32

	modLfoToCutoff int32
	modEnvToCutoff int32
	dynamicCutoff  bool

	modLfoToVolume float32
	dynamicVolume  bool

	instrumentPan    float32
	instrumentReverb float32
	instrumentChorus float32

	// Some instruments require fast cutoff change, which can cause pop noise.
	// This is used to smooth out the cutoff frequency.
	smoothedCutoff float32

	voiceState  int32
	voiceLength int32
}

func newVoice(s *Synthesizer) *voice {
	return &voice{
		synthesizer: s,
		volEnv:      newVolumeEnvelope(s),
		modEnv:      newModulationEnvelope(s),
		vibLfo:      newLfo(s),
		modLfo:      newLfo(s),
		oscillator:  newOscillator(s),
		filter:      newBiQuadFilter(s),
		block:       make([]float32, s.BlockSize),
	}
}

func (v *voice) start(region regionPair, channel int32, key int32, velocity int32, sf int) {
	v.exclusiveClass = region.getExclusiveClass()
	v.channel = channel
	v.key = key
	v.velocity = velocity

	v.region = region
	v.killed = false
	v.updateModulation()
	region.values = &v.modValues

	v.cutoff = region.getInitialFilterCutoffFrequency()
	v.resonance = calcDecibelsToLinear(region.getInitialFilterQ())

	v.vibLfoToPitch = 0.01 * float32(region.getVibratoLfoToPitch())
	v.modLfoToPitch = 0.01 * float32(region.getModulationLfoToPitch())
	v.modEnvToPitch = 0.01 * float32(region.getModulationEnvelopeToPitch())

	v.modLfoToCutoff = region.getModulationLfoToFilterCutoffFrequency()
	v.modEnvToCutoff = region.getModulationEnvelopeToFilterCutoffFrequency()
	v.dynamicCutoff = v.modLfoToCutoff != 0 || v.modEnvToCutoff != 0

	v.modLfoToVolume = region.getModulationLfoToVolume()
	v.dynamicVolume = v.modLfoToVolume > 0.05

	v.instrumentPan = calcClamp(region.getPan(), -50, 50)
	v.instrumentReverb = 0.01 * region.getReverbEffectsSend()
	v.instrumentChorus = 0.01 * region.getChorusEffectsSend()

	v.volEnv.startByRegion(region, key, velocity)
	v.modEnv.startByRegion(region, key, velocity)
	v.vibLfo.startVibrato(region, key, velocity)
	v.modLfo.startModulation(region, key, velocity)
	v.oscillator.startByRegion(v.synthesizer.SoundFont[sf].WaveData, region)
	v.filter.clearBuffer()
	v.filter.setLowPassFilter(v.cutoff, v.resonance)

	v.smoothedCutoff = v.cutoff

	v.voiceState = voicePlaying
	v.voiceLength = 0
}

func (v *voice) end() {
	if v.voiceState == voicePlaying {
		v.voiceState = voiceReleaseRequested
	}
}

func (v *voice) kill() {
	v.killed = true
}

func (v *voice) process() bool {
	if v.killed {
		return false
	}

	channelInfo := v.synthesizer.channels[v.channel]
	v.updateModulation()

	v.releaseIfNecessary(channelInfo)

	if !v.volEnv.process(v.synthesizer.BlockSize) {
		return false
	}

	v.modEnv.process(v.synthesizer.BlockSize)
	v.vibLfo.process()
	v.modLfo.process()

	vibPitchChange := v.vibLfoToPitch * v.vibLfo.value
	modPitchChange := v.modLfoToPitch*v.modLfo.value + v.modEnvToPitch*v.modEnv.value
	channelPitchChange := channelInfo.getTune()
	v.oscillator.pitchOffset = vibPitchChange + modPitchChange + channelPitchChange
	pitch := float32(v.key)
	if !v.oscillator.process(v.block, pitch) {
		return false
	}

	if v.dynamicCutoff {
		cents := float32(v.modLfoToCutoff)*v.modLfo.value + float32(v.modEnvToCutoff)*v.modEnv.value
		factor := calcCentsToMultiplyingFactor(cents)
		newCutoff := factor * v.cutoff

		// The cutoff change is limited within x0.5 and x2 to reduce pop noise.
		lowerLimit := 0.5 * v.smoothedCutoff
		upperLimit := 2 * v.smoothedCutoff
		v.smoothedCutoff = calcClamp(newCutoff, lowerLimit, upperLimit)

		v.filter.setLowPassFilter(v.smoothedCutoff, v.resonance)
	}
	v.filter.process(v.block)

	v.previousMixGainLeft = v.currentMixGainLeft
	v.previousMixGainRight = v.currentMixGainRight
	v.previousReverbSend = v.currentReverbSend
	v.previousChorusSend = v.currentChorusSend

	mixGain := v.noteGain * v.volEnv.value
	if v.dynamicVolume {
		decibels := v.modLfoToVolume * v.modLfo.value
		mixGain *= calcDecibelsToLinear(decibels)
	}

	angle := float32(math.Pi/200) * (v.instrumentPan + 50)
	switch {
	case angle <= 0:
		v.currentMixGainLeft = mixGain
		v.currentMixGainRight = 0
	case angle >= halfPi:
		v.currentMixGainLeft = 0
		v.currentMixGainRight = mixGain
	default:
		v.currentMixGainLeft = mixGain * float32(math.Cos(float64(angle)))
		v.currentMixGainRight = mixGain * float32(math.Sin(float64(angle)))
	}

	v.currentReverbSend = calcClamp(v.instrumentReverb, 0, 1)
	v.currentChorusSend = calcClamp(v.instrumentChorus, 0, 1)

	if v.voiceLength == 0 {
		v.previousMixGainLeft = v.currentMixGainLeft
		v.previousMixGainRight = v.currentMixGainRight
		v.previousReverbSend = v.currentReverbSend
		v.previousChorusSend = v.currentChorusSend
	}

	v.voiceLength += v.synthesizer.BlockSize

	return true
}

func (v *voice) releaseIfNecessary(channelInfo *channel) {
	if v.voiceLength < v.synthesizer.minimumVoiceDuration {
		return
	}

	if v.voiceState == voiceReleaseRequested && !channelInfo.holdPedal {
		v.volEnv.release()
		v.modEnv.release()
		v.oscillator.release()

		v.voiceState = voiceReleased
	}
}

func (v *voice) getPriority() float32 {
	if v.noteGain < nonAudible {
		return 0
	}
	return v.volEnv.priority
}
