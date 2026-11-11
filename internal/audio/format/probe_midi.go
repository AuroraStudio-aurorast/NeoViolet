package format

import (
	"context"
	"io"
	"os"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/midi"
)

// A fixed rate is enough for a duration: the renderer's tail is two seconds at
// any supported rate, and only sub-frame tempo rounding could differ.
const midiProbeRate = 44100

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

// probeMIDI parses the SMF because its length is carried in the track events,
// not in a fixed-size header. Invalid data leaves the duration unknown so the
// scanner can still list the file; playback reports the parse error later.
func probeMIDI(ctx context.Context, file *os.File, result *ProbeInfo) (bool, error) {
	var magic [4]byte
	n, err := file.ReadAt(magic[:], 0)
	if err != nil && err != io.EOF {
		return false, err
	}
	if n != len(magic) || string(magic[:]) != "MThd" {
		return true, nil
	}
	song, _, err := midi.Parse(contextReader{ctx: ctx, r: file})
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return true, nil
	}
	duration, err := song.Duration(midiProbeRate)
	if err == nil {
		result.Duration = duration
	}
	return true, nil
}
