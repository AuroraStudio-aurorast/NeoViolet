package format

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/flac"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/vorbis"
	"github.com/gopxl/beep/v2/wav"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

type FormatDecoder struct{}

func NewFormatDecoder() *FormatDecoder {
	return &FormatDecoder{}
}

// ---- Format probe infrastructure ----

// FormatProbe inspects raw bytes and reports whether it recognizes the format.
// It returns the extension and true on match; otherwise "", false.
type FormatProbe func(buf []byte, n int) (ext string, matched bool)

var (
	ftypProbes []FormatProbe // MP4 ftyp box content (m4a, etc.)
	oggProbes  []FormatProbe // Ogg container stream type (OpusHead, etc.)
	mpegProbes []FormatProbe // MPEG sync byte layer detection (MP2, etc.)
	id3Probes  []FormatProbe // Format behind an ID3 tag (MP2, etc.)
	magicProbes []FormatProbe // Raw leading magic bytes (APE, etc.)
)

func registerFTYPProbe(fn FormatProbe)  { ftypProbes = append(ftypProbes, fn) }
func registerOGGProbe(fn FormatProbe)   { oggProbes = append(oggProbes, fn) }
func registerMPEGProbe(fn FormatProbe)  { mpegProbes = append(mpegProbes, fn) }
func registerID3Probe(fn FormatProbe)   { id3Probes = append(id3Probes, fn) }
func registerMagicProbe(fn FormatProbe) { magicProbes = append(magicProbes, fn) }

// ---- Format registration ----

var formatTable []formatHandler

func registerFormat(h formatHandler) {
	formatTable = append(formatTable, h)
	for _, ext := range h.extensions {
		extLookup[ext] = &formatTable[len(formatTable)-1]
	}
}

// formatHandler describes how to decode one audio format from an io.Reader.
type formatHandler struct {
	extensions       []string
	decode           func(r io.Reader) (beep.StreamSeekCloser, beep.Format, error)
	decodeSeeker     func(r io.ReadSeeker) (beep.StreamSeekCloser, beep.Format, error)
	decodeReadCloser func(r io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error)
}

// extLookup maps a file extension (lower-case with dot) to its formatHandler.
var extLookup = make(map[string]*formatHandler)

func init() {
	// Register built-in (beep) formats.
	// Custom decoder formats (ALAC, Opus, MP2) are registered via init() in their own files.
	registerFormat(formatHandler{
		extensions: []string{".mp3"},
		decodeReadCloser: func(r io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) {
			return mp3.Decode(r)
		},
	})
	registerFormat(formatHandler{
		extensions: []string{".wav"},
		decode: func(r io.Reader) (beep.StreamSeekCloser, beep.Format, error) {
			return wav.Decode(r)
		},
	})
	registerFormat(formatHandler{
		extensions: []string{".flac"},
		decode: func(r io.Reader) (beep.StreamSeekCloser, beep.Format, error) {
			return flac.Decode(r)
		},
	})
	registerFormat(formatHandler{
		extensions: []string{".ogg", ".oga"},
		decodeReadCloser: func(r io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) {
			return vorbis.Decode(r)
		},
	})
}

// detectFormatFromBuf runs the format-detection logic on a byte slice.
// It is the shared core used by both DetectFormatByMagic (from *os.File)
// and DetectFormatFromBytes (from a raw []byte).
func detectFormatFromBuf(buf []byte, n int, sourceName string) (string, error) {
	if n < 4 {
		return "", fmt.Errorf("file too small to detect format")
	}

	switch {
	case n >= 17 && string(buf[0:17]) == "Extended Module: ":
		logger.Debug("Detected format: XM", "source", sourceName)
		return ".xm", nil
	case n >= 4 && string(buf[0:4]) == "IMPM":
		logger.Debug("Detected format: IT", "source", sourceName)
		return ".it", nil
	case n >= 48 && string(buf[44:48]) == "SCRM":
		logger.Debug("Detected format: S3M", "source", sourceName)
		return ".s3m", nil
	case n >= 1084 && isMODSignature(string(buf[1080:1084])):
		logger.Debug("Detected format: MOD", "source", sourceName)
		return ".mod", nil
	case n >= 3 && string(buf[0:3]) == "ID3":
		if ext := detectMPEGBehindID3(buf, n); ext != "" {
			logger.Debug("Detected format: MPEG behind ID3 -> "+ext, "source", sourceName)
			return ext, nil
		}
		logger.Debug("Detected format: MP3 (ID3)", "source", sourceName)
		return ".mp3", nil
	case n >= 2 && buf[0] == 0xFF && (buf[1]&0xE0) == 0xE0:
		for _, p := range mpegProbes {
			if ext, ok := p(buf, n); ok {
				logger.Debug("Detected format: "+ext+" (sync)", "source", sourceName)
				return ext, nil
			}
		}
		logger.Debug("Detected format: MP3 (sync)", "source", sourceName)
		return ".mp3", nil
	case n >= 12 && string(buf[0:4]) == "RIFF" && string(buf[8:12]) == "WAVE":
		logger.Debug("Detected format: WAV", "source", sourceName)
		return ".wav", nil
	case n >= 4 && string(buf[0:4]) == "fLaC":
		logger.Debug("Detected format: FLAC", "source", sourceName)
		return ".flac", nil
	case n >= 4 && string(buf[0:4]) == "OggS":
		for _, p := range oggProbes {
			if ext, ok := p(buf, n); ok {
				logger.Debug("Detected format: "+ext, "source", sourceName)
				return ext, nil
			}
		}
		logger.Debug("Detected format: OGG/Vorbis", "source", sourceName)
		return ".ogg", nil
	case n >= 4 && string(buf[0:4]) == "MThd":
		logger.Debug("Detected format: MIDI", "source", sourceName)
		return ".mid", nil
	case n >= 8 && string(buf[4:8]) == "ftyp":
		for _, p := range ftypProbes {
			if ext, ok := p(buf, n); ok {
				logger.Debug("Detected format: "+ext, "source", sourceName)
				return ext, nil
			}
		}
		logger.Debug("Detected ftyp but unrecognized", "source", sourceName)
		return "", fmt.Errorf("unknown ftyp container")
	default:
		for _, p := range magicProbes {
			if ext, ok := p(buf, n); ok {
				logger.Debug("Detected format: "+ext+" (magic)", "source", sourceName)
				return ext, nil
			}
		}
		if synth.OpenmptProbe(buf[:n]) {
			logger.Debug("Detected format: tracker (openmpt probe)", "source", sourceName)
			return ".mod", nil
		}
		logger.Debug("Unknown audio format", "source", sourceName)
		return "", fmt.Errorf("unknown audio format")
	}
}

func (fd *FormatDecoder) DetectFormatByMagic(file *os.File) (string, error) {
	originalPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", err
	}
	defer file.Seek(originalPos, io.SeekStart)

	buffer := make([]byte, 1084)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}

	return detectFormatFromBuf(buffer, n, file.Name())
}

// DetectFormatFromBytes detects the audio format from a raw byte slice.
// Returns the file extension (e.g. ".mp3", ".flac") or an error.
func (fd *FormatDecoder) DetectFormatFromBytes(data []byte) (string, error) {
	buffer := make([]byte, 1084)
	n := copy(buffer, data)
	return detectFormatFromBuf(buffer, n, "stdin")
}

// detectMPEGBehindID3 skips past an ID3v2 header and checks the underlying
// MPEG frame to distinguish MP2 (Layer II) from MP3 (Layer III).
func detectMPEGBehindID3(buf []byte, n int) string {
	if n < 10 {
		return ""
	}
	tagSize := int(buf[6])<<21 | int(buf[7])<<14 | int(buf[8])<<7 | int(buf[9])
	pastID3 := 10 + tagSize
	if pastID3+2 > n {
		return ""
	}
	// Run id3Probes unconditionally (each probe decides if the data matches).
	for _, p := range id3Probes {
		if ext, ok := p(buf, n); ok {
			return ext
		}
	}
	// If no probe matched but we see MPEG sync, default to MP3.
	if buf[pastID3] == 0xFF && (buf[pastID3+1]&0xE0) == 0xE0 {
		return ".mp3"
	}
	return ""
}

func (fd *FormatDecoder) Decode(file *os.File, path string) (beep.StreamSeekCloser, beep.Format, error) {
	detectedExt, err := fd.DetectFormatByMagic(file)
	if err != nil {
		detectedExt = filepath.Ext(path)
	}

	h := extLookup[detectedExt]
	if h == nil {
		return nil, beep.Format{}, ErrUnsupportedFormat
	}

	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		return nil, beep.Format{}, fmt.Errorf("%s seek: %w", detectedExt, seekErr)
	}

	var streamer beep.StreamSeekCloser
	var format beep.Format
	switch {
	case h.decodeSeeker != nil:
		streamer, format, err = h.decodeSeeker(file)
	case h.decodeReadCloser != nil:
		streamer, format, err = h.decodeReadCloser(file)
	default:
		streamer, format, err = h.decode(file)
	}

	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("decode %s: %w", detectedExt, err)
	}

	return streamer, format, nil
}

func (fd *FormatDecoder) DecodeFromReader(r io.Reader, ext string) (beep.StreamSeekCloser, beep.Format, error) {
	formatMime := strings.ToLower(ext)
	h := extLookup[formatMime]
	if h == nil {
		return nil, beep.Format{}, ErrUnsupportedFormat
	}

	var streamer beep.StreamSeekCloser
	var format beep.Format
	var err error

	switch {
	case h.decodeSeeker != nil:
		rsc, ok := r.(io.ReadSeeker)
		if !ok {
			return nil, beep.Format{}, fmt.Errorf("%s decode requires io.ReadSeeker", formatMime)
		}
		streamer, format, err = h.decodeSeeker(rsc)
	case h.decodeReadCloser != nil:
		rc, ok := r.(io.ReadCloser)
		if !ok {
			return nil, beep.Format{}, fmt.Errorf("%s decode requires io.ReadCloser", formatMime)
		}
		streamer, format, err = h.decodeReadCloser(rc)
	default:
		streamer, format, err = h.decode(r)
	}

	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("decode %s: %w", formatMime, err)
	}

	return streamer, format, nil
}

var modSignatures = map[string]bool{
	"M.K.": true,
	"M!K!": true,
	"FLT4": true,
	"FLT8": true,
	"4CHN": true,
	"6CHN": true,
	"8CHN": true,
	"16CN": true,
	"32CN": true,
	"CD81": true,
	"OKTA": true,
	"OCTA": true,
	"TDZ1": true,
	"TDZ2": true,
	"TDZ3": true,
}

func isMODSignature(sig string) bool {
	return modSignatures[sig]
}

// MIMETypeToExt maps a MIME type (e.g. "audio/mpeg") to a file extension.
func MIMETypeToExt(mime string) string {
	mime = strings.ToLower(mime)
	switch {
	case strings.Contains(mime, "audio/mpeg"), strings.Contains(mime, "audio/mp3"):
		return ".mp3"
	case strings.Contains(mime, "audio/flac"):
		return ".flac"
	case strings.Contains(mime, "audio/wav"), strings.Contains(mime, "audio/x-wav"), strings.Contains(mime, "audio/wave"):
		return ".wav"
	case strings.Contains(mime, "audio/ogg"), strings.Contains(mime, "audio/vorbis"):
		return ".ogg"
	}
	return ""
}

func (fd *FormatDecoder) SupportedFormats() []string {
	var exts []string
	for i := range formatTable {
		exts = append(exts, formatTable[i].extensions...)
	}
	exts = append(exts, ".mid", ".midi", ".mod", ".xm", ".s3m", ".it")
	return exts
}

var ErrUnsupportedFormat = &UnsupportedFormatError{}

type UnsupportedFormatError struct{}

func (e *UnsupportedFormatError) Error() string {
	return "unsupported audio format"
}