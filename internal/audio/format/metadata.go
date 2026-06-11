package format

import (
	"os"

	"github.com/dhowden/tag"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/apetag"
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
	// 1. Try dhowden/tag (supports MP3/ID3, FLAC, OGG, MP4, etc.).
	if m := readViaDhowden(path); m.Title != "" {
		return m
	}

	// 2. Fall back to APEv2 tag reader (for Monkey's Audio .ape files).
	if m := readViaAPEv2(path); m.Title != "" {
		return m
	}

	return Metadata{}
}

func readViaDhowden(path string) Metadata {
	file, err := os.Open(path)
	if err != nil {
		logger.Debug("Metadata read: open failed", "path", path, "err", err)
		return Metadata{}
	}
	defer file.Close()

	metadata, err := tag.ReadFrom(file)
	if err != nil {
		logger.Debug("Metadata read: dhowden/tag failed", "path", path, "err", err)
		return Metadata{}
	}

	m := Metadata{
		Title:  metadata.Title(),
		Artist: metadata.Artist(),
	}
	logger.Debug("Metadata read (dhowden)", "path", path, "title", m.Title, "artist", m.Artist)
	return m
}

func readViaAPEv2(path string) Metadata {
	tags, err := apetag.ParseFile(path)
	if err != nil {
		logger.Debug("Metadata read: APEv2 failed", "path", path, "err", err)
		return Metadata{}
	}
	m := Metadata{
		Title:  tags.Title,
		Artist: tags.Artist,
	}
	logger.Debug("Metadata read (APEv2)", "path", path, "title", m.Title, "artist", m.Artist)
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
