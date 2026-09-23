package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format"
)

// maxPathCandidates caps one directory listing: completion is a hint, and a
// huge directory must not turn a keystroke into a stall.
const maxPathCandidates = 200

// pathScan is the filesystem side of one ":open" completion: the directory to
// read, the fragment already typed inside it, and the literal prefix to echo
// back so candidates keep the shape the user typed (relative, absolute or "~").
type pathScan struct {
	dir      string
	fragment string
	typedDir string
}

// scanFor resolves a typed prefix to a directory listing request.
func scanFor(prefix string) pathScan {
	full := expandTilde(prefix)
	switch {
	case full == "":
		return pathScan{dir: "."}
	case strings.HasSuffix(prefix, "/"):
		return pathScan{dir: full, typedDir: prefix}
	default:
		// The fragment and the directory to scan both come from the literal
		// prefix: expanding "~" can produce a path longer than what the user
		// typed, which would index past the start of the prefix. A typed "."
		// is the fragment for dotfile completion, not a request to descend
		// into the current directory, because os.Stat resolves "." to the
		// directory itself and would swallow the fragment.
		base := filepath.Base(prefix)
		if base != "." {
			if info, err := os.Stat(full); err == nil && info.IsDir() {
				return pathScan{dir: full, typedDir: prefix + "/"}
			}
		}
		return pathScan{
			dir:      expandTilde(filepath.Dir(prefix)),
			fragment: base,
			typedDir: prefix[:len(prefix)-len(base)],
		}
	}
}

// playableExt reports whether an extension is something the player can open:
// a registered decoder or a synthetic (MIDI/tracker) format.
func playableExt(ext string) bool {
	ext = strings.ToLower(ext)
	return format.IsSupportedExt(ext) || audio.IsSyntheticFormat(ext)
}

// pathCandidates lists what could follow a typed path: every directory plus the
// files the player can open. A ReadDir failure yields no candidates and no
// error state -- completion is a hint, not validation.
func pathCandidates(prefix string) []candidate {
	scan := scanFor(prefix)
	entries, err := os.ReadDir(scan.dir)
	if err != nil {
		return nil
	}

	showHidden := strings.HasPrefix(scan.fragment, ".")
	out := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasPrefix(name, scan.fragment) {
			continue
		}
		isDir := entry.IsDir()
		if !isDir && !playableExt(filepath.Ext(name)) {
			continue
		}
		value := scan.typedDir + name
		if isDir {
			value += "/"
		}
		out = append(out, candidate{Value: value, Path: true})
	}

	sort.Slice(out, func(i, j int) bool {
		iDir := strings.HasSuffix(out[i].Value, "/")
		jDir := strings.HasSuffix(out[j].Value, "/")
		if iDir != jDir {
			return iDir
		}
		return out[i].Value < out[j].Value
	})
	if len(out) > maxPathCandidates {
		out = out[:maxPathCandidates]
	}
	return out
}
