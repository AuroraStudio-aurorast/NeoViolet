package fetch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

const cacheFileVersion = 1

const (
	ttlFound    = 30 * 24 * time.Hour
	ttlNotfound = 7 * 24 * time.Hour
)

// cacheFileNameRe matches our cache file names (<32 hex>.json) so cleanup
// never touches ratelimit.json or unrelated files.
var cacheFileNameRe = regexp.MustCompile(`^[0-9a-f]{32}\.json$`)

// CacheFile is one song's persistent cache record.
type CacheFile struct {
	Version      int       `json:"version"`
	Sig          string    `json:"sig"`
	Provider     string    `json:"provider"` // source BaseURL; mismatched entries are invalidated
	State        string    `json:"state"`    // "found" | "notfound" | "instrumental"
	TrackID      int64     `json:"track_id"`
	TrackName    string    `json:"track_name"`
	ArtistName   string    `json:"artist_name"`
	AlbumName    string    `json:"album_name"`
	Duration     int       `json:"duration"`
	FetchedAt    time.Time `json:"fetched_at"`
	SyncedLyrics string    `json:"synced_lyrics"`
	PlainLyrics  string    `json:"plain_lyrics"`
}

// cacheFileName returns the deterministic file name for a signature.
func cacheFileName(sig string) string {
	sum := sha256.Sum256([]byte(sig))
	return hex.EncodeToString(sum[:16]) + ".json"
}

// expired reports whether the record is past its TTL. Found lyrics live 30
// days; negative and instrumental records 7 days (LRCLIB may backfill tracks).
func (cf CacheFile) expired() bool {
	ttl := ttlFound
	if cf.State != "found" {
		ttl = ttlNotfound
	}
	return time.Now().After(cf.FetchedAt.Add(ttl))
}

func cacheFilePath(sig string) (string, error) {
	dir, err := config.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, cacheFileName(sig)), nil
}

// Load returns the cached record for sig. Corrupt, expired, version- or
// provider-mismatched files are removed and treated as a miss.
func Load(sig, provider string) (*CacheFile, bool) {
	path, err := cacheFilePath(sig)
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var cf CacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		os.Remove(path)
		return nil, false
	}
	if cf.Version != cacheFileVersion || cf.Sig != sig || cf.Provider != provider {
		os.Remove(path)
		return nil, false
	}
	if cf.expired() {
		os.Remove(path)
		return nil, false
	}
	return &cf, true
}

// Save writes the record atomically (tmp file then rename).
func Save(cf CacheFile) error {
	dir, err := config.CacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	path := filepath.Join(dir, cacheFileName(cf.Sig))
	data, err := json.Marshal(cf)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write cache tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename cache: %w", err)
	}
	return nil
}

// RemoveCache deletes the cache file for sig (used by :lrc refresh).
func RemoveCache(sig string) error {
	path, err := cacheFilePath(sig)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// CleanupExpired removes expired or corrupt cache files in dir (startup sweep).
// Non-cache files are left untouched.
func CleanupExpired(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		if e.IsDir() || !cacheFileNameRe.MatchString(e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cf CacheFile
		if err := json.Unmarshal(data, &cf); err != nil {
			os.Remove(path)
			removed++
			continue
		}
		if cf.Version != cacheFileVersion || cf.expired() {
			os.Remove(path)
			removed++
		}
	}
	return removed, nil
}
