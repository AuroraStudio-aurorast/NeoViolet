// Package alacstream provides a beep.StreamSeekCloser wrapper for ALAC audio.
package alacstream

import (
	"fmt"
	"io"

	"github.com/gopxl/beep/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/alac"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/mp4"
)

// Streamer implements beep.StreamSeekCloser for ALAC-encoded audio in M4A/MP4.
type Streamer struct {
	reader      io.ReadSeeker
	track       *mp4.ALACTrack
	decoder     *alac.Decoder
	sampleRate  beep.SampleRate
	numChannels int
	sampleSize  int

	currentSample int
	totalSamples  int
	pos           int

	buf        []float64
	bufSamples int

	closed bool
}

// DecodeM4A opens an M4A file and returns a beep.StreamSeekCloser for the ALAC track.
func DecodeM4A(reader io.ReadSeeker) (*Streamer, beep.Format, error) {
	dmx := mp4.NewDemuxer(reader)
	track, err := dmx.FindALACTrack()
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("m4a: find alac track: %w", err)
	}

	if track.SampleCount == 0 {
		return nil, beep.Format{}, fmt.Errorf("m4a: no samples in alac track")
	}

	decoder, err := alac.NewDecoder(track.MagicCookie)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("m4a: create alac decoder: %w", err)
	}
	decoder.SetNumChannels(int(track.Channels))

	var totalSamples int
	if track.DurationPerSample > 0 && track.TimeScale > 0 {
		totalSamples = int(track.DurationPerSample) * int(track.SampleCount)
	} else {
		totalSamples = int(decoder.MaxSamplesPerFrame) * int(track.SampleCount)
	}

	sampleRate := beep.SampleRate(track.SampleRate)
	if sampleRate == 0 {
		sampleRate = beep.SampleRate(decoder.SampleRate())
	}

	f := beep.Format{
		SampleRate:  sampleRate,
		NumChannels: int(track.Channels),
		Precision:   2,
	}

	return &Streamer{
		reader:        reader,
		track:         track,
		decoder:       decoder,
		sampleRate:    sampleRate,
		numChannels:   int(track.Channels),
		sampleSize:    int(track.SampleSize),
		currentSample: 0,
		totalSamples:  totalSamples,
	}, f, nil
}

// Stream reads audio samples into the provided buffer.
func (s *Streamer) Stream(samples [][2]float64) (int, bool) {
	if s.closed {
		return 0, false
	}

	totalNeeded := len(samples)
	totalFilled := 0

	for totalFilled < totalNeeded {
		if s.pos < s.bufSamples {
			framesToCopy := minInt(totalNeeded-totalFilled, s.bufSamples-s.pos)
			for i := 0; i < framesToCopy; i++ {
				srcIdx := (s.pos + i) * 2
				samples[totalFilled+i][0] = s.buf[srcIdx]
				if s.numChannels > 1 && srcIdx+1 < len(s.buf) {
					samples[totalFilled+i][1] = s.buf[srcIdx+1]
				} else {
					samples[totalFilled+i][1] = s.buf[srcIdx]
				}
			}
			s.pos += framesToCopy
			s.currentSample += framesToCopy
			totalFilled += framesToCopy
			continue
		}

		frameIndex := uint32(s.currentSample / int(s.decoder.MaxSamplesPerFrame))
		if int(frameIndex) >= len(s.track.SampleSizes) {
			if totalFilled == 0 {
				return 0, false
			}
			return totalFilled, true
		}

		pcm, err := s.decodeFrame(frameIndex)
		if err != nil || len(pcm) == 0 {
			if totalFilled == 0 {
				return 0, false
			}
			return totalFilled, true
		}

		s.buf = pcmToFloat64(pcm, s.numChannels, s.sampleSize)
		s.bufSamples = len(s.buf) / 2
		s.pos = 0
	}

	return totalFilled, true
}

// Err returns any error that occurred during streaming.
func (s *Streamer) Err() error {
	return nil
}

func (s *Streamer) decodeFrame(index uint32) ([]byte, error) {
	raw, err := s.track.ReadSample(s.reader, index)
	if err != nil {
		return nil, err
	}
	return s.decoder.Decode(raw), nil
}

// pcmToFloat64 converts little-endian PCM bytes to float64 samples in [-1, 1].
func pcmToFloat64(pcm []byte, numChannels, sampleSize int) []float64 {
	bytesPerSample := sampleSize / 8
	if bytesPerSample == 0 {
		bytesPerSample = 2
	}
	frameSize := bytesPerSample * numChannels
	if frameSize == 0 {
		return nil
	}
	numFrames := len(pcm) / frameSize
	out := make([]float64, numFrames*2)

	for i := 0; i < numFrames; i++ {
		for ch := 0; ch < numChannels && ch < 2; ch++ {
			sampleStart := i*frameSize + ch*bytesPerSample
			var sample int32
			switch bytesPerSample {
			case 1:
				sample = int32(pcm[sampleStart]) - 128
				sample = sample << 8
			case 2:
				lo := int16(pcm[sampleStart])
				hi := int16(pcm[sampleStart+1])
				sample = int32(lo | hi<<8)
			case 3:
				sample = int32(pcm[sampleStart]) | int32(pcm[sampleStart+1])<<8 | int32(pcm[sampleStart+2])<<16
				if sample&0x800000 != 0 {
					sample |= ^0xffffff
				}
			case 4:
				sample = int32(pcm[sampleStart]) | int32(pcm[sampleStart+1])<<8 |
					int32(pcm[sampleStart+2])<<16 | int32(pcm[sampleStart+3])<<24
			}
			out[i*2+ch] = float64(sample) / float64(int32(1)<<(sampleSize-1))
		}
		if numChannels == 1 {
			out[i*2+1] = out[i*2]
		}
	}
	return out
}

func (s *Streamer) Len() int {
	return s.totalSamples
}

func (s *Streamer) Position() int {
	return s.currentSample
}

func (s *Streamer) Seek(samples int) error {
	if s.closed {
		return fmt.Errorf("streamer is closed")
	}
	if samples < 0 {
		samples = 0
	}
	if samples > s.totalSamples {
		samples = s.totalSamples
	}
	s.currentSample = samples
	s.pos = 0
	s.buf = nil
	s.bufSamples = 0
	return nil
}

func (s *Streamer) Close() error {
	s.closed = true
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}