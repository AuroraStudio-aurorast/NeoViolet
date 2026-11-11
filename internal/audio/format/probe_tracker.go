package format

import (
	"context"
	"io"
	"os"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
)

// Tracker probe loading happens in libopenmpt, so the whole module has to be
// resident. The cap keeps one pathological file from dominating a scan; a
// larger module keeps Duration unknown rather than blocking the library.
const trackerProbeLimit = 64 << 20

// probeTracker adds the duration of an OpenMPT module to result. It is best
// effort: an unsupported or malformed module leaves the zero duration that the
// scanner already understands, while context cancellation still stops the scan.
func probeTracker(ctx context.Context, file *os.File, size int64, result *ProbeInfo) error {
	if size <= 0 || size > trackerProbeLimit {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, r: file}, size))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil
	}
	if duration, ok := synth.OpenmptDuration(data); ok {
		result.Duration = duration
	}
	return nil
}
