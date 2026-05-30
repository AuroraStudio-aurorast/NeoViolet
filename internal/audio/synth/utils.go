package synth

import "math"

// softClip applies a smooth tanh soft-clipping to prevent harsh distortion.
func softClip(x float64) float64 {
	if x > 3 {
		return 1.0
	}
	if x < -3 {
		return -1.0
	}
	return math.Tanh(x)
}

// clamp restricts v to the [lo, hi] range.
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}