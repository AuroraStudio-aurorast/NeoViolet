package audio

import (
	"image"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/cover"
)

func (p *Player) readTags(path string) {
	metadata := p.tagReader.Read(path)
	p.title = metadata.Title
	p.artist = metadata.Artist
	p.album = metadata.Album

	img, err := cover.ExtractFromFile(path)
	if err == nil {
		p.coverImage = img
	}
}

// Path returns the path or URL of the currently loaded audio source.
func (p *Player) Path() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.path
}

// Title returns the track title of the currently loaded audio source.
func (p *Player) Title() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isSynthActive() {
		return p.synthCtrl.Title()
	}
	return p.title
}

// Artist returns the artist name of the currently loaded audio source.
func (p *Player) Artist() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isSynthActive() {
		return p.synthCtrl.Artist()
	}
	return p.artist
}

// Album returns the album name of the currently loaded audio source.
func (p *Player) Album() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.album
}

// CoverImage returns the embedded cover art of the loaded audio source, if any.
func (p *Player) CoverImage() image.Image {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isSynthActive() {
		return p.synthCtrl.CoverImage()
	}
	return p.coverImage
}
