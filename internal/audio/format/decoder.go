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

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/alacstream"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/mp2stream"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/opusstream"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

type FormatDecoder struct{}

func NewFormatDecoder() *FormatDecoder {
	return &FormatDecoder{}
}

// formatHandler describes how to decode one audio format from an io.Reader.
type formatHandler struct {
	extensions       []string
	decode           func(r io.Reader) (beep.StreamSeekCloser, beep.Format, error)
	decodeSeeker     func(r io.ReadSeeker) (beep.StreamSeekCloser, beep.Format, error)
	decodeReadCloser func(r io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error)
}

var formatTable = []formatHandler{
	{
		extensions: []string{".mp2"},
		decodeSeeker: func(r io.ReadSeeker) (beep.StreamSeekCloser, beep.Format, error) {
			return mp2stream.DecodeMP2(r)
		},
	},
	{
		extensions: []string{".mp3"},
		decodeReadCloser: func(r io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) {
			return mp3.Decode(r)
		},
	},
	{
		extensions: []string{".wav"},
		decode: func(r io.Reader) (beep.StreamSeekCloser, beep.Format, error) {
			return wav.Decode(r)
		},
	},
	{
		extensions: []string{".flac"},
		decode: func(r io.Reader) (beep.StreamSeekCloser, beep.Format, error) {
			return flac.Decode(r)
		},
	},
	{
		extensions: []string{".ogg", ".oga"},
		decodeReadCloser: func(r io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) {
			return vorbis.Decode(r)
		},
	},
	{
		extensions: []string{".opus"},
		decodeSeeker: func(r io.ReadSeeker) (beep.StreamSeekCloser, beep.Format, error) {
			return opusstream.DecodeOGG(r)
		},
	},
	{
		extensions: []string{".m4a"},
		decodeSeeker: func(r io.ReadSeeker) (beep.StreamSeekCloser, beep.Format, error) {
			return alacstream.DecodeM4A(r)
		},
	},
}

// extLookup maps a file extension (lower-case with dot) to its formatHandler.
var extLookup map[string]*formatHandler

func init() {
	extLookup = make(map[string]*formatHandler)
	for i := range formatTable {
		h := &formatTable[i]
		for _, ext := range h.extensions {
			extLookup[ext] = h
		}
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
	if n < 4 {
		return "", fmt.Errorf("file too small to detect format")
	}

	switch {
	case n >= 17 && string(buffer[0:17]) == "Extended Module: ":
		logger.Debug("Detected format: XM", "path", file.Name())
		return ".xm", nil
	case n >= 4 && string(buffer[0:4]) == "IMPM":
		logger.Debug("Detected format: IT", "path", file.Name())
		return ".it", nil
	case n >= 48 && string(buffer[44:48]) == "SCRM":
		logger.Debug("Detected format: S3M", "path", file.Name())
		return ".s3m", nil
	case n >= 1084 && isMODSignature(string(buffer[1080:1084])):
		logger.Debug("Detected format: MOD", "path", file.Name())
		return ".mod", nil
	case n >= 3 && string(buffer[0:3]) == "ID3":
		if ext := detectMPEGBehindID3(buffer, n); ext != "" {
			logger.Debug("Detected format: MPEG behind ID3 -> "+ext, "path", file.Name())
			return ext, nil
		}
		logger.Debug("Detected format: MP3 (ID3)", "path", file.Name())
		return ".mp3", nil
	case n >= 2 && buffer[0] == 0xFF && (buffer[1]&0xE0) == 0xE0:
		layer := (buffer[1] >> 1) & 0x03
		if layer == 2 {
			logger.Debug("Detected format: MP2 (sync)", "path", file.Name())
			return ".mp2", nil
		}
		logger.Debug("Detected format: MP3 (sync)", "path", file.Name())
		return ".mp3", nil
	case n >= 12 && string(buffer[0:4]) == "RIFF" && string(buffer[8:12]) == "WAVE":
		logger.Debug("Detected format: WAV", "path", file.Name())
		return ".wav", nil
	case n >= 4 && string(buffer[0:4]) == "fLaC":
		logger.Debug("Detected format: FLAC", "path", file.Name())
		return ".flac", nil
	case n >= 4 && string(buffer[0:4]) == "OggS":
		if n >= 37 && string(buffer[28:36]) == "OpusHead" {
			logger.Debug("Detected format: Opus/OGG", "path", file.Name())
			return ".opus", nil
		}
		logger.Debug("Detected format: OGG/Vorbis", "path", file.Name())
		return ".ogg", nil
	case n >= 4 && string(buffer[0:4]) == "MThd":
		logger.Debug("Detected format: MIDI", "path", file.Name())
		return ".mid", nil
	case n >= 8 && string(buffer[4:8]) == "ftyp":
		ftype := string(buffer[8:12])
		if ftype == "M4A " || ftype == "mp42" || ftype == "isom" || ftype == "M4B" {
			logger.Debug("Detected format: M4A/ALAC", "path", file.Name(), "ftype", ftype)
			return ".m4a", nil
		}
		if n >= 16 {
			ftype2 := string(buffer[12:16])
			if ftype2 == "M4A " || ftype2 == "M4B" {
				logger.Debug("Detected format: M4A/ALAC", "path", file.Name(), "ftype", ftype2)
				return ".m4a", nil
			}
		}
		logger.Debug("Detected ftyp but not M4A", "path", file.Name(), "ftype", ftype)
		return "", fmt.Errorf("unknown ftyp container")
	default:
		if synth.OpenmptProbe(buffer[:n]) {
			logger.Debug("Detected format: tracker (openmpt probe)", "path", file.Name())
			return ".mod", nil
		}
		logger.Debug("Unknown audio format", "path", file.Name())
		return "", fmt.Errorf("unknown audio format")
	}
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
	if buf[pastID3] == 0xFF && (buf[pastID3+1]&0xE0) == 0xE0 {
		layer := (buf[pastID3+1] >> 1) & 0x03
		if layer == 2 {
			return ".mp2"
		}
	}
	return ".mp3"
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