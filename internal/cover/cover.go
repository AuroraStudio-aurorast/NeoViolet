package cover

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/dhowden/tag"
)

func ExtractFromFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open for cover: %w", err)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, fmt.Errorf("read tags: %w", err)
	}

	pic := m.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return nil, nil
	}

	img, _, err := image.Decode(bytes.NewReader(pic.Data))
	if err != nil {
		return nil, fmt.Errorf("decode cover: %w", err)
	}

	return img, nil
}
