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

func (p *Player) Path() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.path
}

func (p *Player) Title() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isSynthActive() {
		return p.synthCtrl.Title()
	}
	return p.title
}

func (p *Player) Artist() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isSynthActive() {
		return p.synthCtrl.Artist()
	}
	return p.artist
}

func (p *Player) Album() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.album
}

func (p *Player) CoverImage() image.Image {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isSynthActive() {
		return p.synthCtrl.CoverImage()
	}
	return p.coverImage
}
