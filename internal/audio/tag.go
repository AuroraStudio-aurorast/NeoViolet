package audio

func (p *Player) readTags(path string) {
	metadata := p.tagReader.Read(path)
	p.title = metadata.Title
	p.artist = metadata.Artist
}
