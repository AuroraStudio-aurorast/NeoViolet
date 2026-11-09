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
}

// Probe reads track metadata without playing the file.
type Probe func(context.Context, string) (Track, error)

// Scan walks roots without following directory symlinks, deduplicates paths,
// probes one file at a time, and preserves entries when metadata is unreadable.
// Cancellation is checked during traversal and before/after every probe.
func Scan(ctx context.Context, roots []string, include func(string) bool, probe Probe, progress func(int, int)) (Result, error) {
	result := Result{Tracks: []Track{}, Issues: []string{}}
	paths := []string{}
	seen := map[string]bool{}
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
				return nil
			}
			if !entry.Type().IsRegular() || !include(path) || seen[path] {
				return nil
			}
			seen[path] = true
			paths = append(paths, path)
			return nil
		})
		if err != nil {
			return result, err
		}
	}
	slices.SortFunc(paths, func(a, b string) int {
		if n := strings.Compare(strings.ToLower(a), strings.ToLower(b)); n != 0 {
			return n
		}
		return strings.Compare(a, b)
	})
	for i, path := range paths {
		if err := ctx.Err(); err != nil {
			return result, err
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
		result.Tracks = append(result.Tracks, track)
		if progress != nil && (i%32 == 0 || i+1 == len(paths)) {
			progress(i+1, len(paths))
		}
	}
	return result, nil
}
