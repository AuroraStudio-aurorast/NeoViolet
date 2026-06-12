package cover

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"

	"github.com/dhowden/tag"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/apetag"
)

// ExtractFromFile extracts the cover art image from an audio file.
// Tries dhowden/tag first (MP3/FLAC/OGG/MP4), then falls back to APEv2
// tag parsing (Monkey's Audio .ape files).
func ExtractFromFile(path string) (image.Image, error) {
	// 1. Try dhowden/tag (supports most common formats).
	img, err := extractViaDhowden(path)
	if err == nil {
		return img, nil
	}

	// 2. Fall back to APEv2 for files with APEv2 tags (.ape).
	img, err = extractViaAPEv2(path)
	if err == nil {
		return img, nil
	}

	return nil, fmt.Errorf("cover: no cover art found")
}

func extractViaDhowden(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return extractCoverFromReader(f)
}

// ExtractFromReader extracts cover art from an io.ReadSeeker (e.g. bytes.Reader).
// Uses dhowden/tag to parse embedded cover art from MP3/FLAC/OGG/MP4 files.
func ExtractFromReader(r io.ReadSeeker) (image.Image, error) {
	return extractCoverFromReader(r)
}

func extractCoverFromReader(r io.ReadSeeker) (image.Image, error) {
	m, err := tag.ReadFrom(r)
	if err != nil {
		return nil, err
	}

	pic := m.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return nil, fmt.Errorf("no picture data")
	}

	img, _, err := image.Decode(bytes.NewReader(pic.Data))
	return img, err
}

func extractViaAPEv2(path string) (image.Image, error) {
	tags, err := apetag.ParseFile(path)
	if err != nil {
		return nil, err
	}
	if len(tags.CoverData) == 0 {
		return nil, fmt.Errorf("no APEv2 cover art")
	}

	img, _, err := image.Decode(bytes.NewReader(tags.CoverData))
	return img, err
}
