// Package mp2stream provides a beep.StreamSeekCloser wrapper for MPEG-1 Audio Layer II (MP2).
package mp2stream

import (
	"fmt"
	"io"

	"github.com/gen2brain/mpeg"
	"github.com/gopxl/beep/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/streamcore"
)

// cachedFrame holds one decoded MP2 frame's audio samples.
type cachedFrame struct {
	data    []float64
	samples int
}

// Streamer implements beep.StreamSeekCloser for MP2-encoded audio.
type Streamer struct {
	streamcore.Core
	frames     []cachedFrame
	frameIndex int
}

// DecodeMP2 reads an MP2 audio stream and returns a beep.StreamSeekCloser.
func DecodeMP2(r io.ReadSeeker) (*Streamer, beep.Format, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, beep.Format{}, fmt.Errorf("mp2 seek: %w", err)
	}

	buf, err := mpeg.NewBuffer(r)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("mp2 create buffer: %w", err)
	}
	buf.SetLoadCallback(buf.LoadReaderCallback)

	audio := mpeg.NewAudio(buf)

	first := audio.Decode()
	if first == nil || !audio.HasHeader() {
		return nil, beep.Format{}, fmt.Errorf("mp2: no valid frame header found")
	}

	numChannels := audio.Channels()
	sampleRate := beep.SampleRate(audio.Samplerate())

	if numChannels == 0 {
		return nil, beep.Format{}, fmt.Errorf("mp2: zero channels")
	}
	if sampleRate == 0 {
		return nil, beep.Format{}, fmt.Errorf("mp2: zero sample rate")
	}

	frames := make([]cachedFrame, 0, 512)
	totalSamples := 0

	fr := toFrame(first, numChannels)
	totalSamples += fr.samples
	frames = append(frames, fr)

	for {
		s := audio.Decode()
		if s == nil || audio.HasEnded() {
			break
		}
		if len(s.Interleaved) == 0 {
			continue
		}
		fr := toFrame(s, numChannels)
		if fr.samples == 0 {
			continue
		}
		totalSamples += fr.samples
		frames = append(frames, fr)
	}

	if len(frames) == 0 {
		return nil, beep.Format{}, fmt.Errorf("mp2: no audio data found")
	}

	s := &Streamer{frames: frames}
	s.TotalSamples = totalSamples
	s.NumChannels = numChannels

	f := beep.Format{
		SampleRate:  sampleRate,
		NumChannels: numChannels,
		Precision:   2,
	}
	return s, f, nil
}

// toFrame converts an mpeg.Samples into a cachedFrame with float64 stereo pairs.
// The mpeg library already yields normalized float32 samples, so this only
// widens them to float64 (unlike streamcore.Int16ToFloat64).
func toFrame(s *mpeg.Samples, numChannels int) cachedFrame {
	in := s.Interleaved
	if len(in) == 0 {
		return cachedFrame{}
	}

	numSamples := len(in) / numChannels
	out := make([]float64, numSamples*2)

	for i := 0; i < numSamples; i++ {
		for ch := 0; ch < numChannels && ch < 2; ch++ {
			out[i*2+ch] = float64(in[i*numChannels+ch])
		}
		if numChannels == 1 {
			out[i*2+1] = out[i*2]
		}
	}

	return cachedFrame{
		data:    out,
		samples: numSamples,
	}
}

// Stream fills the output buffer with decoded MP2 samples.
func (s *Streamer) Stream(samples [][2]float64) (int, bool) {
	if s.Closed.Load() {
		return 0, false
	}

	totalNeeded := len(samples)
	totalFilled := 0

	for totalFilled < totalNeeded {
		if s.Pos < s.BufSamples {
			totalFilled += s.CopyToOutput(samples, totalNeeded, totalFilled)
			continue
		}

		if s.frameIndex >= len(s.frames) {
			if totalFilled == 0 {
				return 0, false
			}
			return totalFilled, true
		}

		fr := s.frames[s.frameIndex]
		s.Buf = fr.data
		s.BufSamples = len(fr.data) / 2
		s.Pos = 0
		s.frameIndex++
	}

	return totalFilled, true
}

// Seek moves the stream position to the given sample.
func (s *Streamer) Seek(samples int) error {
	if s.Closed.Load() {
		return fmt.Errorf("streamer is closed")
	}
	if samples < 0 {
		samples = 0
	}
	if samples > s.TotalSamples {
		samples = s.TotalSamples
	}

	accum := 0
	for i, fr := range s.frames {
		if accum+fr.samples > samples {
			s.frameIndex = i
			s.CurrentSample.Store(int64(accum))
			s.ResetBuffer()
			return nil
		}
		accum += fr.samples
	}

	s.frameIndex = len(s.frames)
	s.CurrentSample.Store(int64(s.TotalSamples))
	s.ResetBuffer()
	return nil
}
