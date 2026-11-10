package soundfont

import (
	"encoding/binary"
	"errors"
	"io"
)

func loadSamples(r io.ReadSeeker, end int64, sf *SoundFont, limits loadLimits) error {
	for {
		pos, e := r.Seek(0, io.SeekCurrent)
		if e != nil {
			return e
		}
		if pos == end {
			break
		}
		c, e := readChunk(r, end)
		if e != nil {
			return e
		}
		switch c.id {
		case "smpl":
			if sf.WaveData != nil {
				return errors.New("duplicate smpl")
			}
			if c.size == 0 || c.size%2 != 0 {
				return errors.New("invalid PCM length")
			}
			if c.size > limits.pcm {
				return errors.New("PCM resource limit exceeded")
			}
			sf.WaveData = make([]int16, int(c.size/2))
			var scratch [64 * 1024]byte
			for i := 0; i < len(sf.WaveData); {
				n := min(len(scratch)/2, len(sf.WaveData)-i)
				if _, e = io.ReadFull(r, scratch[:2*n]); e != nil {
					return e
				}
				for j := 0; j < n; j++ {
					// #nosec G115 -- PCM conversion preserves signed little-endian 16-bit sample bit patterns.
					sf.WaveData[i+j] = int16(binary.LittleEndian.Uint16(scratch[2*j : 2*j+2]))
				}
				i += n
			}
		case "sm24":
			sf.diagnose("sm24_ignored", "24-bit low bytes ignored; rendering 16-bit PCM")
		}
		if _, e = r.Seek(c.next, io.SeekStart); e != nil {
			return e
		}
	}
	if sf.WaveData == nil {
		return errors.New("missing smpl")
	}
	return nil
}
