package lyrics

import (
	"os"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

var (
	parserNames []string
	parserMap   = map[string]LyricParser{}
)

// RegisterParser registers a lyric parser under a format name.
func RegisterParser(name string, p LyricParser) {
	if _, exists := parserMap[name]; exists {
		logger.Warn("parser already registered", "name", name)
		return
	}
	parserMap[name] = p
	parserNames = append(parserNames, name)
}

// AvailableParsers returns the names of all registered lyric parsers.
func AvailableParsers() []string {
	result := make([]string, len(parserNames))
	copy(result, parserNames)
	return result
}

// FindAndParse locates and parses lyric sidecar files for an audio path.
func FindAndParse(audioPath string, priority []string) (*Data, error) {
	return findAndParseWithPreferred(audioPath, priority, "")
}

// FindAndParsePreferred is like FindAndParse but tries a single preferred format
// first (before falling back to the full priority list). If preferred is empty
// or the preferred parser is not found, it behaves identically to FindAndParse.
func FindAndParsePreferred(audioPath string, priority []string, preferred string) (*Data, error) {
	return findAndParseWithPreferred(audioPath, priority, preferred)
}

func findAndParseWithPreferred(audioPath string, priority []string, preferred string) (*Data, error) {
	order := priority
	if len(order) == 0 {
		order = parserNames
	}

	// If a preferred format is given, try it first
	if preferred != "" {
		if p, ok := parserMap[preferred]; ok {
			sidecar := p.FindSidecar(audioPath)
			if sidecar != "" {
				// #nosec G304 -- sidecar path is the user's own lyric file next to
				// their audio; opening it is the core feature.
				f, err := os.Open(sidecar)
				if err == nil {
					data, err := p.Parse(f, sidecar)
					_ = f.Close()
					if err == nil && data != nil {
						data.Format = preferred
						return data, nil
					}
					if err != nil {
						logger.Warn("lyric parse failed (preferred)", "format", preferred, "path", sidecar, "error", err)
					}
				} else {
					logger.Warn("open lyric file failed (preferred)", "format", preferred, "path", sidecar, "error", err)
				}
			}
		}
	}

	for _, name := range order {
		// Skip the preferred format in the main loop (already tried above)
		if name == preferred {
			continue
		}
		p, ok := parserMap[name]
		if !ok {
			continue
		}
		sidecar := p.FindSidecar(audioPath)
		if sidecar == "" {
			continue
		}
		// #nosec G304 -- see preferred-format path above.
		f, err := os.Open(sidecar)
		if err != nil {
			logger.Warn("open lyric file failed", "format", name, "path", sidecar, "error", err)
			continue
		}
		data, err := p.Parse(f, sidecar)
		_ = f.Close()
		if err != nil {
			logger.Warn("lyric parse failed", "format", name, "path", sidecar, "error", err)
			continue
		}
		if data == nil {
			continue
		}
		data.Format = name
		return data, nil
	}

	return nil, nil
}