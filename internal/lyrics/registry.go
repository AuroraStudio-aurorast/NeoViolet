package lyrics

import (
	"os"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

var (
	parserNames []string
	parserMap   = map[string]LyricParser{}
)

func RegisterParser(name string, p LyricParser) {
	if _, exists := parserMap[name]; exists {
		logger.Warn("parser already registered", "name", name)
		return
	}
	parserMap[name] = p
	parserNames = append(parserNames, name)
}

func AvailableParsers() []string {
	result := make([]string, len(parserNames))
	copy(result, parserNames)
	return result
}

func FindAndParse(audioPath string, priority []string) (*LyricsData, error) {
	order := priority
	if len(order) == 0 {
		order = parserNames
	}

	for _, name := range order {
		p, ok := parserMap[name]
		if !ok {
			continue
		}
		sidecar := p.FindSidecar(audioPath)
		if sidecar == "" {
			continue
		}
		f, err := os.Open(sidecar)
		if err != nil {
			logger.Warn("open lyric file failed", "format", name, "path", sidecar, "error", err)
			continue
		}
		defer f.Close()
		data, err := p.Parse(f, sidecar)
		if err != nil {
			logger.Warn("lyric parse failed", "format", name, "path", sidecar, "error", err)
			continue
		}
		return data, nil
	}

	return nil, nil
}
