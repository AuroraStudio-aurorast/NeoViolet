package lyrics

import (
	"errors"
	"io"
)

// maxLyricSize is the maximum lyric file size we'll read into memory (1 MB).
// Files larger than this are rejected with ErrLyricTooLarge to prevent OOM.
const maxLyricSize = 1 * 1024 * 1024

// ErrLyricTooLarge is returned when a lyric file exceeds maxLyricSize.
var ErrLyricTooLarge = errors.New("lyrics file too large (>1MB)")

type LyricParser interface {
	FindSidecar(audioPath string) string
	Parse(r io.Reader, sourcePath string) (*LyricsData, error)
}

// readAllWithLimit reads from r up to maxLyricSize+1 bytes.
// If the content exceeds maxLyricSize, it returns ErrLyricTooLarge.
func readAllWithLimit(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxLyricSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxLyricSize {
		return nil, ErrLyricTooLarge
	}
	return data, nil
}
