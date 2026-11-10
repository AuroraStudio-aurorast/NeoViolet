// Package library scans user-selected music folders without playback ownership.
package library

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Track uses its absolute path as library identity, never as queue entry identity.
// A zero Duration means it is not known until the decoder opens for playback.
type Track struct {
	Path, Title, Artist, Album, Format string
	Duration                           time.Duration
}

// Result is a completed library snapshot with nonfatal per-path diagnostics.
type Result struct {
	Tracks []Track
	Issues []string
	Cache  *Cache
}

// Probe reads track metadata without playing the file.
type Probe func(context.Context, string) (Track, error)

// ScanOptions enables successful fingerprint reuse and bounded snapshot delivery.
type ScanOptions struct {
	Cache   *Cache
	Publish func([]Track)
}

// Scan discovers paths before probing and preserves entries with unreadable tags.
// It checks cancellation at traversal and probe boundaries and never follows directory symlinks.
func Scan(ctx context.Context, roots []string, include func(string) bool, probe Probe, progress func(int, int), options ...ScanOptions) (Result, error) {
	var opts ScanOptions
	if len(options) > 0 {
		opts = options[0]
	}
	result := Result{Tracks: []Track{}, Issues: []string{}, Cache: &Cache{Version: cacheVersion, Roots: rootKeys(roots), Entries: map[string]CachedTrack{}}}
	fingerprints := map[string]CachedTrack{}
	paths := []string{}
	seen := map[string]bool{}
	directories := map[string]bool{}
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		absolute, err := filepath.Abs(root)
		if err != nil {
			result.Issues = append(result.Issues, err.Error())
			continue
		}
		resolved, resolveErr := filepath.EvalSymlinks(absolute)
		if resolveErr != nil {
			result.Issues = append(result.Issues, resolveErr.Error())
			continue
		}
		info, statErr := os.Stat(resolved)
		if statErr != nil || !info.IsDir() {
			result.Issues = append(result.Issues, fmt.Sprintf("%s is not a readable directory", absolute))
			continue
		}
		absolute = resolved
		err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				result.Issues = append(result.Issues, fmt.Sprintf("%s: %v", path, walkErr))
				return nil
			}
			if entry.IsDir() {
				if directories[path] {
					return filepath.SkipDir
				}
				directories[path] = true
				return nil
			}
			if !entry.Type().IsRegular() || !include(path) || seen[path] {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				result.Issues = append(result.Issues, fmt.Sprintf("%s: %v", path, err))
				return nil
			}
			fingerprints[path] = CachedTrack{Size: info.Size(), Modified: info.ModTime().UnixNano()}
			seen[path] = true
			paths = append(paths, path)
			return nil
		})
		if err != nil {
			return result, err
		}
	}
	sortPaths(paths)
	result.Tracks = make([]Track, len(paths))
	valid := make([]bool, len(paths))
	for i, path := range paths {
		track := Track{Path: path, Title: filepath.Base(path), Format: strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))}
		if opts.Cache != nil && opts.Cache.Version == cacheVersion {
			cached, ok := opts.Cache.Entries[path]
			fingerprint := fingerprints[path]
			if ok && cached.Size == fingerprint.Size && cached.Modified == fingerprint.Modified {
				track = cached.Track
				result.Cache.Entries[path] = cached
				valid[i] = true
			}
		}
		result.Tracks[i] = track
	}
	if opts.Publish != nil {
		opts.Publish(slices.Clone(result.Tracks))
	}
	lastPublish := time.Now()
	for i, path := range paths {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if valid[i] {
			if progress != nil && (i%32 == 0 || i+1 == len(paths)) {
				progress(i+1, len(paths))
			}
			continue
		}
		track, err := probe(ctx, path)
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if err != nil {
			result.Issues = append(result.Issues, fmt.Sprintf("%s: %v", path, err))
		}
		track.Path = path
		if track.Title == "" {
			track.Title = filepath.Base(path)
		}
		if track.Format == "" {
			track.Format = strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))
		}
		result.Tracks[i] = track
		if err == nil {
			before := fingerprints[path]
			info, statErr := os.Stat(path)
			if statErr == nil && info.Size() == before.Size && info.ModTime().UnixNano() == before.Modified {
				before.Track = track
				result.Cache.Entries[path] = before
			}
		}
		if opts.Publish != nil && time.Since(lastPublish) >= 75*time.Millisecond && i+1 < len(paths) {
			opts.Publish(slices.Clone(result.Tracks))
			lastPublish = time.Now()
		}
		if progress != nil && (i%32 == 0 || i+1 == len(paths)) {
			progress(i+1, len(paths))
		}
	}
	return result, nil
}

func sortPaths(paths []string) {
	keys := make(map[string]string, len(paths))
	for _, path := range paths {
		keys[path] = strings.ToLower(path)
	}
	slices.SortFunc(paths, func(a, b string) int {
		if n := strings.Compare(keys[a], keys[b]); n != 0 {
			return n
		}
		return strings.Compare(a, b)
	})
}
