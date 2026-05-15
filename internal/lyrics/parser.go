package lyrics

import "io"

type LyricParser interface {
	FindSidecar(audioPath string) string
	Parse(r io.Reader, sourcePath string) (*LyricsData, error)
}
