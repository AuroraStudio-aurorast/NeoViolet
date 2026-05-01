package audio

import (
	"os"

	"github.com/dhowden/tag"
)

type MetadataReader struct{}

func NewMetadataReader() *MetadataReader {
	return &MetadataReader{}
}

type Metadata struct {
	Title  string
	Artist string
}

func (mr *MetadataReader) Read(path string) Metadata {
	file, err := os.Open(path)
	if err != nil {
		return Metadata{}
	}
	defer file.Close()

	metadata, err := tag.ReadFrom(file)
	if err != nil {
		return Metadata{}
	}

	return Metadata{
		Title:  metadata.Title(),
		Artist: metadata.Artist(),
	}
}

func (mr *MetadataReader) ReadWithFallback(path string, fallbackTitle, fallbackArtist string) Metadata {
	metadata := mr.Read(path)

	if metadata.Title == "" {
		metadata.Title = fallbackTitle
	}
	if metadata.Artist == "" {
		metadata.Artist = fallbackArtist
	}

	return metadata
}
