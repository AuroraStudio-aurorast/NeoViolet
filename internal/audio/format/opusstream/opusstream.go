// Package opusstream provides a beep.StreamSeekCloser wrapper for Opus audio in Ogg containers.
package opusstream

import (
	"fmt"
	"io"

	"github.com/gopxl/beep/v2"
	"github.com/pion/opus"
	"github.com/pion/opus/pkg/oggreader"
)

// cachedPacket holds one Opus packet's audio metadata.
type cachedPacket struct {
	raw     []byte
	samples int       // number of sample frames (per channel) in this packet
}

// Streamer implements beep.StreamSeekCloser for Opus-encoded audio in OGG.
type Streamer struct {
	packets      []cachedPacket
	totalSamples int
	numChannels  int
	sampleRate   beep.SampleRate

	decoder       opus.Decoder
	packetIndex   int // which packet we are decoding next
	currentSample int // absolute sample position within the stream (for Position())
	pos           int // sample position within curBuf
	buf           []float64
	bufSamples    int

	closed bool
}

// DecodeOGG opens an OGG file containing Opus audio and returns a beep.StreamSeekCloser.
func DecodeOGG(r io.ReadSeeker) (*Streamer, beep.Format, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, beep.Format{}, fmt.Errorf("opus seek: %w", err)
	}

	og, header, err := oggreader.NewWith(r)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("opus ogg header: %w", err)
	}

	if header.Channels == 0 {
		return nil, beep.Format{}, fmt.Errorf("opus: zero channels in header")
	}

	var (
		packets         []cachedPacket
		granuleSamples  uint64
		preSkip         = uint64(header.PreSkip)
	)

	for {
		packet, pageHeader, err := og.ParseNextPacket()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, beep.Format{}, fmt.Errorf("opus read packet: %w", err)
		}

		if len(packet) < 2 {
			continue
		}

		if pageHeader.GranulePosition > granuleSamples {
			granuleSamples = pageHeader.GranulePosition
		}

		// Decode size probe: use a throwaway decoder since we're just counting
		probe := opus.NewDecoder()
		out := make([]int16, 5760) // max 120ms at 48kHz
		n, decodeErr := probe.DecodeToInt16(packet, out)
		if decodeErr != nil || n <= 0 {
			continue
		}

		packets = append(packets, cachedPacket{
			raw:     packet,
			samples: n,
		})
	}

	if len(packets) == 0 {
		return nil, beep.Format{}, fmt.Errorf("opus: no audio packets found")
	}

	// Total samples from granule position minus pre-skip
	totalSamples := int(granuleSamples - preSkip)
	if totalSamples <= 0 {
		for _, p := range packets {
			totalSamples += p.samples
		}
	}

	decoder, err := opus.NewDecoderWithOutput(48000, int(header.Channels))
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("opus create decoder: %w", err)
	}

	channels := int(header.Channels)

	f := beep.Format{
		SampleRate:  beep.SampleRate(48000),
		NumChannels: channels,
		Precision:   2,
	}

	return &Streamer{
		packets:      packets,
		totalSamples: totalSamples,
		numChannels:  channels,
		sampleRate:   beep.SampleRate(48000),
		decoder:      decoder,
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
		// Consume from current decoded buffer
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

		// Decode next packet
		if s.packetIndex >= len(s.packets) {
			if totalFilled == 0 {
				return 0, false
			}
			return totalFilled, true
		}

		pkt := s.packets[s.packetIndex]
		out := make([]int16, pkt.samples*s.numChannels)
		_, err := s.decoder.DecodeToInt16(pkt.raw, out)
		if err != nil {
			s.packetIndex++
			continue
		}

		s.buf = int16ToFloat64(out, s.numChannels)
		s.bufSamples = len(s.buf) / 2
		s.pos = 0
		s.packetIndex++
	}

	return totalFilled, true
}

func int16ToFloat64(pcm []int16, numChannels int) []float64 {
	if len(pcm) == 0 {
		return nil
	}
	numFrames := len(pcm) / numChannels
	out := make([]float64, numFrames*2)

	for i := 0; i < numFrames; i++ {
		for ch := 0; ch < numChannels && ch < 2; ch++ {
			out[i*2+ch] = float64(pcm[i*numChannels+ch]) / 32768.0
		}
		if numChannels == 1 {
			out[i*2+1] = out[i*2]
		}
	}
	return out
}

// Len returns the total number of sample frames.
func (s *Streamer) Len() int {
	return s.totalSamples
}

// Position returns the current sample position.
func (s *Streamer) Position() int {
	return s.currentSample
}

// Seek seeks to the given sample position.
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

	s.decoder.Init(48000, s.numChannels)

	// Walk packets to find which one contains the target sample
	accum := 0
	for i, pkt := range s.packets {
		if accum+pkt.samples > samples {
			s.packetIndex = i
			s.currentSample = accum
			s.buf = nil
			s.bufSamples = 0
			s.pos = 0
			return nil
		}
		accum += pkt.samples
	}

	s.packetIndex = len(s.packets)
	s.currentSample = s.totalSamples
	s.buf = nil
	s.bufSamples = 0
	s.pos = 0
	return nil
}

func (s *Streamer) Err() error {
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