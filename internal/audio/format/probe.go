package format

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
)

// ProbeInfo describes a local file without initializing output or synthesizers.
type ProbeInfo struct {
	Metadata
	Format   string
	Duration time.Duration
}

// Probe reads bounded container headers and text tags without initializing a
// playback decoder. Duration is zero when the headers do not establish it.
// Text metadata is best effort; invalid container headers, file errors and
// cancellation are returned to the scanner.
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
	stat, err := file.Stat()
	if err != nil {
		return result, err
	}
	r := &probeReader{ctx: ctx, file: file, size: stat.Size()}
	var header [1084]byte
	n, _ := r.readAt(header[:], 0)
	ext := probeFormat(header[:n])
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(path))
	}
	result.Format = strings.ToUpper(strings.TrimPrefix(ext, "."))
	valid := true
	switch ext {
	case ".wav":
		valid = probeWAV(r, &result)
	case ".flac":
		valid = probeFLAC(r, &result)
	case ".ogg", ".oga", ".opus":
		valid = probeOgg(r, &result)
	case ".m4a":
		valid = probeMP4(r, &result)
	case ".mp3", ".mp2":
		offset := probeID3(r, &result.Metadata)
		var sync [4]byte
		if r.at(sync[:], offset) && sync[0] == 255 && sync[1]&0xe0 == 0xe0 && sync[1]&6 == 4 {
			result.Format = "MP2"
		} else if ext == ".mp3" {
			valid = probeMP3(r, offset, &result)
		}
		probeTrailingTags(r, &result.Metadata)
	case ".ape":
		probeTrailingTags(r, &result.Metadata)
	case ".mid", ".midi":
		var err error
		if valid, err = probeMIDI(ctx, file, &result); err != nil {
			return result, err
		}
	default:
		if synth.OpenmptSupportsExtension(ext) {
			if err := probeTracker(ctx, file, stat.Size(), &result); err != nil {
				return result, err
			}
		}
	}
	if r.err != nil {
		return result, r.err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if !valid {
		return result, fmt.Errorf("probe %s: invalid or truncated header", ext)
	}
	return result, nil
}

// Limits apply to retained text and the number of records inspected, never to
// cover payload sizes: those are skipped with offsets, without reading them.
const probeTextLimit = 64 << 10
const probeRecordLimit = 4096

type probeReader struct {
	ctx       context.Context
	file      *os.File
	size      int64
	err       error
	bytes     int
	textBytes int
}

func (r *probeReader) readAt(buf []byte, offset int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(buf) > (4<<20)-r.bytes {
		return 0, io.EOF
	}
	r.bytes += len(buf)
	n, err := r.file.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		r.err = err
	}
	return n, err
}
func (r *probeReader) reserveText(n int64) bool {
	if n < 0 || n > (256<<10)-int64(r.textBytes) {
		return false
	}
	r.textBytes += int(n)
	return true
}
func (r *probeReader) at(buf []byte, offset int64) bool {
	if offset < 0 || offset > r.size || int64(len(buf)) > r.size-offset {
		return false
	}
	n, err := r.readAt(buf, offset)
	return n == len(buf) && err == nil
}
func probeDuration(samples, rate uint64) time.Duration {
	if rate == 0 || samples/rate > uint64(math.MaxInt64/int64(time.Second)) {
		return 0
	}
	seconds := samples / rate
	fraction := (samples % rate) * uint64(time.Second) / rate
	if seconds*uint64(time.Second) > uint64(math.MaxInt64)-fraction {
		return 0
	}
	// #nosec G115 -- the checks above bound the sum to math.MaxInt64.
	return time.Duration(seconds*uint64(time.Second) + fraction)
}
func probeSetText(m *Metadata, key, value string) {
	value = strings.TrimSpace(strings.TrimRight(value, "\x00"))
	switch strings.ToUpper(key) {
	case "TITLE", "TIT2", "TT2":
		m.Title = value
	case "ARTIST", "TPE1", "TP1":
		m.Artist = value
	case "ALBUM", "TALB", "TAL":
		m.Album = value
	}
}
func probeTextKey(key string) bool {
	switch strings.ToUpper(key) {
	case "TITLE", "ARTIST", "ALBUM", "TIT2", "TPE1", "TALB", "TT2", "TP1", "TAL":
		return true
	}
	return false
}

// A cursor permits skipping irrelevant metadata without copying its payload.
type probeInput interface {
	reserveText(int64) bool
	Read([]byte) (int, error)
	skip(int64) bool
}
type probeCursor struct {
	r        *probeReader
	pos, end int64
}

func (c *probeCursor) reserveText(n int64) bool { return c.r.reserveText(n) }
func (c *probeCursor) Read(p []byte) (int, error) {
	if c.pos >= c.end {
		return 0, io.EOF
	}
	p = p[:min(int64(len(p)), c.end-c.pos)]
	n, e := c.r.readAt(p, c.pos)
	c.pos += int64(n)
	return n, e
}
func (c *probeCursor) skip(n int64) bool {
	if n < 0 || n > c.end-c.pos {
		return false
	}
	c.pos += n
	return c.r.ctx.Err() == nil
}
func probeU32(r io.Reader) (uint32, bool) {
	var b [4]byte
	_, e := io.ReadFull(r, b[:])
	return binary.LittleEndian.Uint32(b[:]), e == nil
}
func probeComments(r probeInput, m *Metadata) {
	n, ok := probeU32(r)
	if !ok || !r.skip(int64(n)) {
		return
	}
	count, ok := probeU32(r)
	if !ok {
		return
	}
	for range min(count, probeRecordLimit) {
		size, ok := probeU32(r)
		if !ok {
			return
		}
		// Only a short prefix is needed to distinguish a text field from a picture.
		var prefix [32]byte
		n := min(size, uint32(len(prefix)))
		if _, err := io.ReadFull(r, prefix[:n]); err != nil {
			return
		}
		key, first, found := strings.Cut(string(prefix[:n]), "=")
		left := int64(size - n)
		if !found || !probeTextKey(key) || size > probeTextLimit || !r.reserveText(int64(size)) {
			if !r.skip(left) {
				return
			}
			continue
		}
		value := make([]byte, int(left))
		if _, err := io.ReadFull(r, value); err != nil {
			return
		}
		probeSetText(m, key, first+string(value))
	}
}

// Detection deliberately avoids the playback detector's libopenmpt fallback.
func probeFormat(b []byte) string {
	n := len(b)
	switch {
	case n >= 3 && string(b[:3]) == "ID3":
		return ".mp3"
	case n >= 2 && b[0] == 255 && b[1]&0xe0 == 0xe0:
		for _, p := range mpegProbes {
			if ext, ok := p(b, n); ok {
				return ext
			}
		}
		return ".mp3"
	case n >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WAVE":
		return ".wav"
	case n >= 4 && string(b[:4]) == "fLaC":
		return ".flac"
	case n >= 4 && string(b[:4]) == "OggS":
		for _, p := range oggProbes {
			if ext, ok := p(b, n); ok {
				return ext
			}
		}
		return ".ogg"
	case n >= 12 && string(b[4:8]) == "ftyp":
		for _, p := range ftypProbes {
			if ext, ok := p(b, n); ok {
				return ext
			}
		}
	case n >= 4 && string(b[:4]) == "MThd":
		return ".mid"
	case n >= 17 && string(b[:17]) == "Extended Module: ":
		return ".xm"
	case n >= 4 && string(b[:4]) == "IMPM":
		return ".it"
	case n >= 48 && string(b[44:48]) == "SCRM":
		return ".s3m"
	case n >= 1084 && isMODSignature(string(b[1080:1084])):
		return ".mod"
	}
	for _, p := range magicProbes {
		if ext, ok := p(b, n); ok {
			return ext
		}
	}
	return ""
}
