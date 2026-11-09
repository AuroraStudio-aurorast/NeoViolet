package format

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProbeInfo describes a local file without initializing output or synthesizers.
type ProbeInfo struct {
	Metadata
	Format   string
	Duration time.Duration
}

// Probe closes the decoder immediately after reading its known duration. APE
// and synthetic files retain unknown duration, avoiding subprocesses/soundfonts
// during scans. A context-aware reader bounds cancellation between decoder reads.
func (fd *Decoder) Probe(ctx context.Context, path string) (ProbeInfo, error) {
	result := ProbeInfo{Format: strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	// #nosec G304 -- path was discovered inside a user-configured music folder.
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer func() { _ = file.Close() }()
	reader := &probeReader{ctx: ctx, file: file}
	result.Metadata = NewMetadataReader().ReadFromSeeker(reader)
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if _, err = reader.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	header := make([]byte, 1084)
	n, _ := reader.Read(header)
	ext, detectErr := fd.DetectFormatFromBytes(header[:n])
	if detectErr != nil {
		ext = strings.ToLower(filepath.Ext(path))
	}
	if _, err = reader.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	if ext == ".ape" {
		result.Format = "APE"
		return result, nil
	}
	handler := extLookup[ext]
	if handler == nil {
		return result, nil
	}
	result.Format = strings.ToUpper(strings.TrimPrefix(ext, "."))
	stream, format, err := fd.DecodeFromReader(reader, ext)
	if err != nil {
		return result, err
	}
	defer func() { _ = stream.Close() }()
	if format.SampleRate > 0 && stream.Len() > 0 {
		result.Duration = format.SampleRate.D(stream.Len())
	}
	return result, ctx.Err()
}

type probeReader struct {
	ctx  context.Context
	file *os.File
}

func (r *probeReader) Read(buf []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.file.Read(buf)
}
func (r *probeReader) Seek(offset int64, whence int) (int64, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.file.Seek(offset, whence)
}

// Decoder ownership ends with the probe; the underlying file closes in Probe.
func (r *probeReader) Close() error { return nil }
