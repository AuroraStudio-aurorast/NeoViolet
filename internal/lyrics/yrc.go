package lyrics

import (
	"fmt"
	"io"
	"regexp"
)

var yrcWordRe = regexp.MustCompile(`\((\d+),(\d+),(\d+)\)([^(]*)`)

func init() {
	RegisterParser("yrc", &yrcParser{})
}

type yrcParser struct{}

func (p *yrcParser) FindSidecar(audioPath string) string {
	return findSidecarWithExt(audioPath, ".yrc")
}

func (p *yrcParser) Parse(r io.Reader, sourcePath string) (*Data, error) {
	data, err := readAllWithLimit(r)
	if err != nil {
		return nil, fmt.Errorf("read yrc: %w", err)
	}

	lyrics, err := parseWordTimedFile(data, "yrc", yrcGroups)
	if err != nil {
		return nil, err
	}
	lyrics.Path = sourcePath
	return lyrics, nil
}
