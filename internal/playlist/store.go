// Package playlist persists named, ordered audio references independently of the library.
package playlist

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
)

// List has a stable identity independent of its editable name and queue entries.
type List struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Tracks []library.Track `json:"tracks"`
}
type document struct {
	Version int    `json:"version"`
	Lists   []List `json:"lists"`
}

// New validates and captures an independent list. Local references become absolute.
func New(name string, tracks []library.Track) (List, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return List{}, err
	}
	list := List{ID: hex.EncodeToString(id), Name: strings.TrimSpace(name), Tracks: slices.Clone(tracks)}
	for i := range list.Tracks {
		if err := validateReference(list.Tracks[i].Path); err != nil {
			return List{}, err
		}
		if !isURL(list.Tracks[i].Path) {
			path, err := filepath.Abs(list.Tracks[i].Path)
			if err != nil {
				return List{}, err
			}
			list.Tracks[i].Path = path
		}
	}
	return list, validate([]List{list})
}

// Clone returns a snapshot with independent track slices.
func Clone(lists []List) []List {
	result := slices.Clone(lists)
	for i := range result {
		result[i].Tracks = slices.Clone(result[i].Tracks)
	}
	return result
}

// Load treats absence as empty, but rejects unreadable or unsupported documents.
func Load(path string) ([]List, error) {
	// #nosec G304 -- caller supplies the resolved application playlist store path.
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []List{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc document
	if err = json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("read playlists: %w", err)
	}
	if doc.Version != 1 {
		return nil, fmt.Errorf("unsupported playlist version %d", doc.Version)
	}
	if err = validate(doc.Lists); err != nil {
		return nil, err
	}
	return doc.Lists, nil
}

// Save atomically replaces a complete snapshot after validation and syncing.
// The caller serializes writers; failed writes never publish partial JSON.
func Save(path string, lists []List) error {
	if err := validate(lists); err != nil {
		return err
	}
	data, err := json.MarshalIndent(document{Version: 1, Lists: lists}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".playlists-*")
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
	// #nosec G703 -- resolved application store path; temporary source is in the same directory.
	return os.Rename(name, path)
}
func validate(lists []List) error {
	ids := map[string]bool{}
	for _, list := range lists {
		if list.ID == "" || ids[list.ID] {
			return fmt.Errorf("invalid or duplicate playlist identity")
		}
		ids[list.ID] = true
		if strings.TrimSpace(list.Name) == "" || strings.ContainsFunc(list.Name, unicode.IsControl) {
			return fmt.Errorf("playlist name must be nonempty and contain no controls")
		}
		for _, track := range list.Tracks {
			if err := validateReference(track.Path); err != nil {
				return err
			}
			if !isURL(track.Path) && !filepath.IsAbs(track.Path) {
				return fmt.Errorf("playlist path must be absolute: %q", track.Path)
			}
		}
	}
	return nil
}
func isURL(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}
func validateReference(path string) error {
	if path == "" || path == "-" || path == "stdin" {
		return fmt.Errorf("stdin cannot be saved; open an audio file or URL")
	}
	if strings.Contains(path, "://") {
		u, err := url.Parse(path)
		if err != nil || !isURL(path) || u.Host == "" {
			return fmt.Errorf("playlist reference must be a local file or HTTP(S) URL")
		}
		return nil
	}
	base, parent := filepath.Base(path), filepath.Base(filepath.Dir(path))
	if strings.HasPrefix(base, "neoviolet-stdin-") || strings.HasPrefix(parent, "neoviolet-") && strings.HasPrefix(base, "cache-") && strings.HasSuffix(base, ".tmp") {
		return fmt.Errorf("temporary audio cannot be saved; use its original file or URL")
	}
	return nil
}
