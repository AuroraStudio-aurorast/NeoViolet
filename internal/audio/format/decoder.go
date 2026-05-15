package format

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/flac"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/vorbis"
	"github.com/gopxl/beep/v2/wav"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

type FormatDecoder struct{}

func NewFormatDecoder() *FormatDecoder {
	return &FormatDecoder{}
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
		logger.Debug("Detected format: MP3 (ID3)", "path", file.Name())
		return ".mp3", nil
	case n >= 2 && buffer[0] == 0xFF && (buffer[1]&0xE0) == 0xE0:
		logger.Debug("Detected format: MP3 (sync)", "path", file.Name())
		return ".mp3", nil
	case n >= 12 && string(buffer[0:4]) == "RIFF" && string(buffer[8:12]) == "WAVE":
		logger.Debug("Detected format: WAV", "path", file.Name())
		return ".wav", nil
	case n >= 4 && string(buffer[0:4]) == "fLaC":
		logger.Debug("Detected format: FLAC", "path", file.Name())
		return ".flac", nil
	case n >= 4 && string(buffer[0:4]) == "OggS":
		logger.Debug("Detected format: OGG", "path", file.Name())
		return ".ogg", nil
	case n >= 4 && string(buffer[0:4]) == "MThd":
		logger.Debug("Detected format: MIDI", "path", file.Name())
		return ".mid", nil
	default:
		logger.Debug("Unknown audio format", "path", file.Name())
		return "", fmt.Errorf("unknown audio format")
	}
}

func (fd *FormatDecoder) Decode(file *os.File, path string) (beep.StreamSeekCloser, beep.Format, error) {
	detectedExt, err := fd.DetectFormatByMagic(file)
	if err != nil {
		detectedExt = filepath.Ext(path)
	}

	var streamer beep.StreamSeekCloser
	var format beep.Format

	switch detectedExt {
	case ".mp3":
		streamer, format, err = mp3.Decode(file)
	case ".wav":
		streamer, format, err = wav.Decode(file)
	case ".flac":
		streamer, format, err = flac.Decode(file)
	case ".ogg", ".oga":
		streamer, format, err = vorbis.Decode(file)
	default:
		return nil, beep.Format{}, ErrUnsupportedFormat
	}

	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("decode %s: %w", detectedExt, err)
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

func (fd *FormatDecoder) SupportedFormats() []string {
	return []string{".mp3", ".wav", ".flac", ".ogg", ".oga", ".mid", ".mod", ".xm", ".s3m", ".it", ".mptm"}
}

var ErrUnsupportedFormat = &UnsupportedFormatError{}

type UnsupportedFormatError struct{}

func (e *UnsupportedFormatError) Error() string {
	return "unsupported audio format"
}
