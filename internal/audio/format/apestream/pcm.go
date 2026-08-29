package apestream

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

// PCM conversion helpers

// convertPCMToFloat64 converts raw PCM bytes to float64 samples and stores
// them in the provided buffer (which must be large enough). Supports 8, 16,
// 24, and 32-bit little-endian PCM. Mono input is duplicated to stereo.
// The returned slice is the input buffer[:frames*2] for caller convenience.
func convertPCMToFloat64(pcm []byte, numChannels, bytesPerSample int, out []float64) []float64 {
	frameSize := bytesPerSample * numChannels
	if frameSize == 0 {
		return out
	}
	numFrames := len(pcm) / frameSize
	if numFrames*2 > len(out) {
		numFrames = len(out) / 2
	}
	out = out[:numFrames*2]

	for i := 0; i < numFrames; i++ {
		for ch := 0; ch < numChannels && ch < 2; ch++ {
			sampleStart := i*frameSize + ch*bytesPerSample
			var sample int32
			switch bytesPerSample {
			case 1:
				// unsigned 8-bit → signed (-128 to 127)
				sample = int32(pcm[sampleStart]) - 128
			case 2:
				// #nosec G115 -- uint16 bit-pattern reinterpreted as int16; bounded.
				sample = int32(int16(binary.LittleEndian.Uint16(pcm[sampleStart:])))
			case 3:
				sample = int32(pcm[sampleStart]) |
					int32(pcm[sampleStart+1])<<8 |
					int32(pcm[sampleStart+2])<<16
				if sample&0x800000 != 0 {
					sample |= ^0xffffff // sign extend
				}
			case 4:
				// #nosec G115 -- uint32 bit-pattern reinterpreted as int32; bounded.
				sample = int32(binary.LittleEndian.Uint32(pcm[sampleStart:]))
			}
			out[i*2+ch] = float64(sample) / float64(uint32(1)<<(bytesPerSample*8-1))
		}
		if numChannels == 1 {
			out[i*2+1] = out[i*2]
		}
	}

	return out
}

// readWithTimeout reads a binary value from r with a timeout.
func readWithTimeout(r io.Reader, v interface{}, timeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- binary.Read(r, binary.LittleEndian, v)
	}()
	select {
	case err := <-errCh:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("read timed out after %v", timeout)
	}
}
