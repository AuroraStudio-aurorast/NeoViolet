package soundfont

import "math"

// Oscillator positions use 24 fractional bits, with validated sample addresses.
const fracBits int32 = 24
const fracUnit int64 = 1 << fracBits
const fpToSample float32 = float32(1) / float32(32768*fracUnit)

type oscillator struct {
	synthesizer                                                               *Synthesizer
	data                                                                      []int16
	loopMode, sampleRate, sampleStart, sampleEnd, startLoop, endLoop, rootKey int32
	tune, pitchChangeScale, sampleRateRatio, pitchOffset                      float32
	looping                                                                   bool
	positionFP                                                                int64
	valid                                                                     bool
}

func newOscillator(s *Synthesizer) *oscillator { return &oscillator{synthesizer: s} }
func (o *oscillator) start(data []int16, loopMode, sampleRate, start, end, startLoop, endLoop, rootKey, coarseTune, fineTune, scaleTuning int32) {
	o.data = data
	o.loopMode = loopMode
	o.sampleRate = sampleRate
	o.sampleStart = start
	o.sampleEnd = end
	o.startLoop = startLoop
	o.endLoop = endLoop
	o.rootKey = rootKey
	o.valid = start >= 0 && start < end && int64(end) <= int64(len(data)) && sampleRate > 0
	o.tune = float32(coarseTune) + 0.01*float32(fineTune)
	o.pitchChangeScale = 0.01 * float32(scaleTuning)
	o.sampleRateRatio = float32(sampleRate) / float32(o.synthesizer.SampleRate)
	o.looping = (loopMode == loopContinuous || loopMode == loopLoopUntilNoteOff) && start <= startLoop && startLoop < endLoop && endLoop <= end
	o.positionFP = int64(start) << fracBits
}
func (o *oscillator) release() {
	if o.loopMode == loopLoopUntilNoteOff {
		o.looping = false
	}
}
func (o *oscillator) process(block []float32, pitch float32) bool {
	change := o.pitchChangeScale*(pitch-float32(o.rootKey)) + o.tune + o.pitchOffset
	return o.fillBlock(block, float64(o.sampleRateRatio)*math.Pow(2, float64(change)/12))
}
func (o *oscillator) fillBlock(block []float32, ratio float64) bool {
	// Bound the conversion and addition; arbitrary non-finite/extreme modulation ends a voice.
	if !o.valid || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio <= 0 || ratio >= float64(int64(1)<<60)/float64(fracUnit) {
		clear(block)
		return false
	}
	step := max(int64(1), int64(float64(fracUnit)*ratio))
	end := int64(o.sampleEnd) << fracBits
	ls, le := int64(o.startLoop)<<fracBits, int64(o.endLoop)<<fracBits
	rendered := false
	for i := range block {
		if o.looping && o.positionFP >= le {
			o.positionFP = ls + (o.positionFP-ls)%(le-ls)
		}
		if o.positionFP >= end {
			clear(block[i:])
			return rendered
		}
		index := o.positionFP >> fracBits
		next := index + 1
		if o.looping && next >= int64(o.endLoop) {
			next = int64(o.startLoop)
		} else if next >= int64(o.sampleEnd) {
			next = index
		}
		x, y := int64(o.data[index]), int64(o.data[next])
		fraction := o.positionFP & (fracUnit - 1)
		block[i] = fpToSample * float32((x<<fracBits)+fraction*(y-x))
		rendered = true
		o.positionFP += step
	}
	return rendered
}
