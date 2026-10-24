package soundfont

import "math"

type modulationEnvelope struct {
	synthesizer          *Synthesizer
	attackSlope          float64
	decaySlope           float64
	releaseSlope         float64
	attackStartTime      float64
	holdStartTime        float64
	decayStartTime       float64
	decayEndTime         float64
	releaseEndTime       float64
	releaseStartTime     float64
	sustainLevel         float32
	releaseLevel         float32
	processedSampleCount int32
	stage                int32
	value                float32
}

func newModulationEnvelope(s *Synthesizer) *modulationEnvelope {
	result := new(modulationEnvelope)
	result.synthesizer = s
	return result
}

func (env *modulationEnvelope) start(delay float32, attack float32, hold float32, decay float32, sustain float32, release float32) {
	env.attackSlope = 1 / float64(attack)
	env.decaySlope = 1 / float64(decay)
	env.releaseSlope = 1 / float64(release)

	env.attackStartTime = float64(delay)
	env.holdStartTime = env.attackStartTime + float64(attack)
	env.decayStartTime = env.holdStartTime + float64(hold)

	env.decayEndTime = env.decayStartTime + float64(decay)
	env.releaseEndTime = float64(release)

	env.sustainLevel = calcClamp(sustain, 0, 1)
	env.releaseLevel = 0

	env.processedSampleCount = 0
	env.stage = envDelay
	env.value = 0

	env.process(0)
}

func (env *modulationEnvelope) updateRates(region regionPair, key int32) {
	env.attackStartTime = float64(region.getDelayModulationEnvelope())
	attack := float64(region.getAttackModulationEnvelope())
	env.attackSlope = 1 / attack
	env.holdStartTime = env.attackStartTime + attack
	env.decayStartTime = env.holdStartTime + float64(region.getHoldModulationEnvelope()*calcKeyNumberToMultiplyingFactor(region.getKeyNumberToModulationEnvelopeHold(), key))
	decay := float64(region.getDecayModulationEnvelope() * calcKeyNumberToMultiplyingFactor(region.getKeyNumberToModulationEnvelopeDecay(), key))
	env.decaySlope = 1 / decay
	env.decayEndTime = env.decayStartTime + decay
	env.sustainLevel = calcClamp(1-region.getSustainModulationEnvelope()/100, 0, 1)
	duration := float64(region.getReleaseModulationEnvelope())
	env.releaseSlope = 1 / duration
	if env.stage == envRelease {
		env.releaseEndTime = env.releaseStartTime + duration
	} else {
		env.releaseEndTime = duration
	}
}

func (env *modulationEnvelope) release() {
	env.stage = envRelease
	env.releaseStartTime = float64(env.processedSampleCount) / float64(env.synthesizer.SampleRate)
	env.releaseEndTime += env.releaseStartTime
	env.releaseLevel = env.value
}

func (env *modulationEnvelope) process(sampleCount int32) bool {
	env.processedSampleCount += sampleCount
	currentTime := float64(env.processedSampleCount) / float64(env.synthesizer.SampleRate)

	for env.stage <= envHold {
		var endTime float64
		switch env.stage {
		case envDelay:
			endTime = env.attackStartTime
		case envAttack:
			endTime = env.holdStartTime
		case envHold:
			endTime = env.decayStartTime
		default:
			panic("invalid envelope stage")
		}

		if currentTime < endTime {
			break
		}
		env.stage++
	}

	switch env.stage {
	case envDelay:
		env.value = 0
		return true
	case envAttack:
		env.value = float32(env.attackSlope * (currentTime - env.attackStartTime))
		return true
	case envHold:
		env.value = 1
		return true
	case envDecay:
		env.value = float32(math.Max(env.decaySlope*(env.decayEndTime-currentTime), float64(env.sustainLevel)))
		return env.value > nonAudible
	case envRelease:
		env.value = float32(math.Max(float64(env.releaseLevel)*float64(env.releaseSlope)*(env.releaseEndTime-currentTime), 0))
		return env.value > nonAudible
	default:
		panic("invalid envelope stage.")
	}
}
