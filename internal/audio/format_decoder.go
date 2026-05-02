package audio

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

	buffer := make([]byte, 12)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}
	if n < 4 {
		return "", fmt.Errorf("file too small to detect format")
	}

	switch {
	case n >= 3 && string(buffer[0:3]) == "ID3":
		return ".mp3", nil
	case n >= 2 && buffer[0] == 0xFF && (buffer[1]&0xE0) == 0xE0:
		return ".mp3", nil
	case n >= 12 && string(buffer[0:4]) == "RIFF" && string(buffer[8:12]) == "WAVE":
		return ".wav", nil
	case n >= 4 && string(buffer[0:4]) == "fLaC":
		return ".flac", nil
	case n >= 4 && string(buffer[0:4]) == "OggS":
		return ".ogg", nil
	case n >= 4 && string(buffer[0:4]) == "MThd":
		return ".mid", nil
	default:
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

func (fd *FormatDecoder) SupportedFormats() []string {
	return []string{".mp3", ".wav", ".flac", ".ogg", ".oga", ".mid"}
}
