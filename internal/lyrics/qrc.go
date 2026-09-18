package lyrics

import (
	"fmt"
	"io"
	"regexp"
)

var qrcWordRe = regexp.MustCompile(`([^(]+)\((\d+),(\d+)\)`)

func init() {
	RegisterParser("qrc", &qrcParser{})
}

type qrcParser struct{}

func (p *qrcParser) FindSidecar(audioPath string) string {
	return findSidecarWithExt(audioPath, ".qrc")
}

func (p *qrcParser) Parse(r io.Reader, sourcePath string) (*Data, error) {
	data, err := readAllWithLimit(r)
	if err != nil {
		return nil, fmt.Errorf("read qrc: %w", err)
	}

	lyrics, err := parseWordTimedFile(data, "qrc", qrcGroups)
	if err != nil {
		return nil, err
	}
	lyrics.Path = sourcePath
	return lyrics, nil
}
