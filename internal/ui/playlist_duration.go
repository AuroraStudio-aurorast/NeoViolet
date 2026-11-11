package ui

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/playlist"
)

// fillPlaylistDurations repairs synthetic and APE entries saved before the
// probe knew their duration. Only zero-length local references are opened; the
// result remains in memory until the playlist is saved again.
func fillPlaylistDurations(lists []playlist.List) {
	decoder := format.NewDecoder()
	probed := map[string]time.Duration{}
	for i := range lists {
		tracks := lists[i].Tracks
		for j := range tracks {
			track := &tracks[j]
			if track.Duration > 0 || !probePlaylistPath(track.Path) {
				continue
			}
			if duration, ok := probed[track.Path]; ok {
				track.Duration = duration
				continue
			}
			var duration time.Duration
			if info, err := decoder.Probe(context.Background(), track.Path); err == nil {
				duration = info.Duration
			}
			probed[track.Path] = duration
			track.Duration = duration
		}
	}
}

func probePlaylistPath(path string) bool {
	if strings.Contains(path, "://") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".ape" || audio.IsSyntheticFormat(ext)
}
