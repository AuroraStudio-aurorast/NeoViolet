// Package streamcore provides shared buffer management for audio format streamers.
package streamcore

// Core provides shared buffer management implementation for audio format
// streamers (mp2stream, opusstream, alacstream). Embed this struct to
// eliminate duplicated Stream inner-loop, Close, Err, and buffer-reset code.
type Core struct {
	Pos           int       // sample position within Buf
	CurrentSample int       // absolute sample position within the stream
	Buf           []float64 // current decoded buffer (interleaved stereo)
	BufSamples    int       // number of sample frames in Buf
	NumChannels   int       // number of audio channels
	TotalSamples  int       // total samples in stream
	Closed        bool
}

// CopyToOutput copies frames from the internal buffer to the output [][2]float64
// slice. It returns the number of frames copied and advances Pos and CurrentSample.
func (sc *Core) CopyToOutput(samples [][2]float64, totalNeeded, totalFilled int) int {
	framesToCopy := MinInt(totalNeeded-totalFilled, sc.BufSamples-sc.Pos)
	for i := 0; i < framesToCopy; i++ {
		srcIdx := (sc.Pos + i) * 2
		samples[totalFilled+i][0] = sc.Buf[srcIdx]
		if sc.NumChannels > 1 && srcIdx+1 < len(sc.Buf) {
			samples[totalFilled+i][1] = sc.Buf[srcIdx+1]
		} else {
			samples[totalFilled+i][1] = sc.Buf[srcIdx]
		}
	}
	sc.Pos += framesToCopy
	sc.CurrentSample += framesToCopy
	return framesToCopy
}

// ResetBuffer clears the decoded buffer state.
func (sc *Core) ResetBuffer() {
	sc.Buf = nil
	sc.BufSamples = 0
	sc.Pos = 0
}

// Err implements beep.StreamSeekCloser.Err — always nil for these streamers.
func (sc *Core) Err() error { return nil }

// Close marks the streamer as closed. Implements io.Closer.
func (sc *Core) Close() error { sc.Closed = true; return nil }

// MinInt returns the minimum of two integers.
func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}