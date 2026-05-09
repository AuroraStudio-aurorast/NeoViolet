package audio

import "github.com/AuroraStudio-aurorast/neoviolet/internal/cover"

func (p *Player) readTags(path string) {
	metadata := p.tagReader.Read(path)
	p.title = metadata.Title
	p.artist = metadata.Artist

	img, err := cover.ExtractFromFile(path)
	if err == nil {
		p.coverImage = img
	}
}
