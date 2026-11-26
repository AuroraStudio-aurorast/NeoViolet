package format

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/gopxl/beep/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/apestream"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/mp2stream"
)

// Remote MP2 caches the entire decoded stream as stereo float64 samples.
const maxRemoteMP2PCMBytes = 256 * 1024 * 1024

// DecodeCachedAudio lets cancellation stop decoder setup. After setup, the
// returned stream owns its reader and no longer depends on the load context.
func (fd *Decoder) DecodeCachedAudio(ctx context.Context, r io.ReadSeekCloser, ext string) (beep.StreamSeekCloser, beep.Format, error) {
	if err := ctx.Err(); err != nil {
		return nil, beep.Format{}, err
	}
	var stream beep.StreamSeekCloser
	var spec beep.Format
	var err error
	ext = strings.ToLower(ext)
	if file, ok := r.(*os.File); ok && ext == ".ape" {
		// This decoder opens subprocesses by file path instead of reading r.
		var ape *apestream.Streamer
		ape, spec, err = apestream.DecodeContext(ctx, file.Name())
		if err == nil {
			stream = ape
		}
	} else {
		reader := &loadingAudioReader{ReadSeekCloser: r, ctx: ctx}
		if ext == ".mp2" {
			var mp2 *mp2stream.Streamer
			mp2, spec, err = mp2stream.DecodeMP2WithLimit(reader, maxRemoteMP2PCMBytes)
			if err == nil {
				stream = mp2
			}
		} else {
			stream, spec, err = fd.DecodeFromReader(reader, ext)
		}
		reader.ctx = nil
	}
	if cancelErr := ctx.Err(); cancelErr != nil {
		if stream != nil {
			_ = stream.Close()
		}
		return nil, beep.Format{}, cancelErr
	}
	return stream, spec, err
}

// Decoders can keep this reader for playback. Its context is cleared before
// the stream is handed off to the player.
type loadingAudioReader struct {
	io.ReadSeekCloser
	ctx context.Context
}

func (r *loadingAudioReader) Read(buf []byte) (int, error) {
	if r.ctx != nil {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
	}
	return r.ReadSeekCloser.Read(buf)
}

func (r *loadingAudioReader) Seek(offset int64, whence int) (int64, error) {
	if r.ctx != nil {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
	}
	return r.ReadSeekCloser.Seek(offset, whence)
}
