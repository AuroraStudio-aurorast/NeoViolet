package soundfont

import "math"

// updateModulation evaluates into voice-private storage. Envelopes retain their
// elapsed time; changing their rates never starts a new attack.
func (v *voice) updateModulation() {
	c := v.synthesizer.channels[v.channel]
	evaluateRegion(v.region, c, v.key, v.velocity, &v.modValues)
	r := v.region
	r.values = &v.modValues
	v.noteGain = calcDecibelsToLinear(-r.getInitialAttenuation() - 0.5*r.getInitialFilterQ())
	v.cutoff = r.getInitialFilterCutoffFrequency()
	v.resonance = calcDecibelsToLinear(r.getInitialFilterQ())
	v.vibLfoToPitch = 0.01 * float32(r.getVibratoLfoToPitch())
	v.modLfoToPitch = 0.01 * float32(r.getModulationLfoToPitch())
	v.modEnvToPitch = 0.01 * float32(r.getModulationEnvelopeToPitch())
	v.modLfoToCutoff = r.getModulationLfoToFilterCutoffFrequency()
	v.modEnvToCutoff = r.getModulationEnvelopeToFilterCutoffFrequency()
	v.dynamicCutoff = true
	v.modLfoToVolume = r.getModulationLfoToVolume()
	v.dynamicVolume = v.modLfoToVolume != 0
	v.instrumentPan = r.getPan()
	v.instrumentReverb = 0.01 * r.getReverbEffectsSend()
	v.instrumentChorus = 0.01 * r.getChorusEffectsSend()
	v.oscillator.tune = float32(r.getCoarseTune()) + 0.01*float32(r.getFineTune())
	// Recalculate phase boundaries against the original note-on time, never
	// reset processedSampleCount/stage. The current phase can only move forward.
	v.volEnv.attackStartTime = float64(r.getDelayVolumeEnvelope())
	attack := float64(r.getAttackVolumeEnvelope())
	v.volEnv.attackSlope = 1 / attack
	v.volEnv.holdStartTime = v.volEnv.attackStartTime + attack
	v.volEnv.decayStartTime = v.volEnv.holdStartTime + float64(r.getHoldVolumeEnvelope()*calcKeyNumberToMultiplyingFactor(r.getKeyNumberToVolumeEnvelopeHold(), v.key))
	v.volEnv.decaySlope = -9.226 / float64(r.getDecayVolumeEnvelope()*calcKeyNumberToMultiplyingFactor(r.getKeyNumberToVolumeEnvelopeDecay(), v.key))
	v.volEnv.releaseSlope = -9.226 / math.Max(0.01, float64(r.getReleaseVolumeEnvelope()))
	v.volEnv.sustainLevel = calcClamp(calcDecibelsToLinear(-r.getSustainVolumeEnvelope()), 0, 1)
	v.modEnv.updateRates(r, v.key)
	v.vibLfo.delay = float64(r.getDelayVibratoLfo())
	v.modLfo.delay = float64(r.getDelayModulationLfo())
	if f := r.getFrequencyVibratoLfo(); f > 0 {
		v.vibLfo.period = 1 / float64(f)
	}
	if f := r.getFrequencyModulationLfo(); f > 0 {
		v.modLfo.period = 1 / float64(f)
	}
}
