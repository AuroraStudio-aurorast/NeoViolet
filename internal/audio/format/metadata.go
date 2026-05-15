package format

import (
	"os"

	"github.com/dhowden/tag"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
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
		logger.Debug("Metadata read: open failed", "path", path, "err", err)
		return Metadata{}
	}
	defer file.Close()

	metadata, err := tag.ReadFrom(file)
	if err != nil {
		logger.Debug("Metadata read: tag parse failed", "path", path, "err", err)
		return Metadata{}
	}

	m := Metadata{
		Title:  metadata.Title(),
		Artist: metadata.Artist(),
	}
	logger.Debug("Metadata read", "path", path, "title", m.Title, "artist", m.Artist)
	return m
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
