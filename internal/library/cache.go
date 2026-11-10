package library

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// Version 2 invalidates metadata stored with the old APE-over-ID3 precedence.
const cacheVersion = 2

var cacheWriteMu sync.Mutex

// CachedTrack pairs successful metadata with its source fingerprint.
type CachedTrack struct {
	Track    Track `json:"track"`
	Size     int64 `json:"size"`
	Modified int64 `json:"modified"`
}

// Cache is a rebuildable, versioned library snapshot. Failed probes are omitted.
type Cache struct {
	Version int                    `json:"version"`
	Roots   []string               `json:"roots"`
	Entries map[string]CachedTrack `json:"entries"`
}

func rootKeys(roots []string) []string {
	result := make([]string, 0, len(roots))
	for _, root := range roots {
		if absolute, err := filepath.Abs(root); err == nil {
			result = append(result, filepath.Clean(absolute))
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

// MatchesRoots prevents a snapshot for removed folders from appearing at startup.
func (c *Cache) MatchesRoots(roots []string) bool {
	return c != nil && c.Version == cacheVersion && slices.Equal(c.Roots, rootKeys(roots))
}

// Tracks returns an independent, ordered snapshot suitable for immediate display.
func (c *Cache) Tracks() []Track {
	paths := make([]string, 0, len(c.Entries))
	for path := range c.Entries {
		paths = append(paths, path)
	}
	sortPaths(paths)
	tracks := make([]Track, 0, len(paths))
	for _, path := range paths {
		tracks = append(tracks, c.Entries[path].Track)
	}
	return tracks
}

// LoadCache reads bounded, private application state; callers may discard errors.
func LoadCache(path string) (*Cache, error) {
	// #nosec G304 -- caller resolves this application cache path.
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	const maxBytes = 64 << 20
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("library cache exceeds size limit")
	}
	var cached Cache
	if err = json.NewDecoder(io.LimitReader(file, maxBytes)).Decode(&cached); err != nil {
		return nil, err
	}
	if cached.Version != cacheVersion || cached.Entries == nil {
		return nil, fmt.Errorf("unsupported library cache")
	}
	for path, entry := range cached.Entries {
		if !filepath.IsAbs(path) || path != entry.Track.Path || entry.Size < 0 {
			return nil, fmt.Errorf("invalid library cache entry")
		}
	}
	return &cached, nil
}

// SaveCache replaces only complete snapshots, serializing cancelled generations.
func SaveCache(ctx context.Context, path string, cached *Cache) error {
	cacheWriteMu.Lock()
	defer cacheWriteMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(cached)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".library-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer func() { _ = file.Close(); _ = os.Remove(name) }()
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// #nosec G703 -- source and destination are resolved application cache paths.
	return os.Rename(name, path)
}
