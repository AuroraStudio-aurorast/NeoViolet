package fetch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	rateLimitFileVersion = 1
	rateLimitFileName    = "ratelimit.json"
)

type rateLimitFile struct {
	Version   int               `json:"version"`
	Providers map[string]string `json:"providers"` // baseURL → UTC RFC3339 deadline
}

// RateLimit persists provider-level 429 cooldowns. The cooldown is scoped to
// the provider base URL because LRCLIB rate limiting applies per server/IP,
// not per track; restarting must not lose the cooldown.
type RateLimit struct {
	mu    sync.Mutex
	file  string
	until map[string]time.Time
}

// LoadRateLimit loads cooldowns from dir/ratelimit.json. A missing or corrupt
// file yields an empty state, never an error.
func LoadRateLimit(dir string) (*RateLimit, error) {
	r := &RateLimit{
		file:  filepath.Join(dir, rateLimitFileName),
		until: make(map[string]time.Time),
	}
	data, err := os.ReadFile(r.file)
	if err != nil {
		return r, nil
	}
	var f rateLimitFile
	if err := json.Unmarshal(data, &f); err != nil {
		return r, nil
	}
	if f.Version != rateLimitFileVersion {
		return r, nil
	}
	now := time.Now()
	for base, ts := range f.Providers {
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil || !t.After(now) {
			continue
		}
		r.until[base] = t
	}
	return r, nil
}

// Blocked reports whether baseURL is still cooling down.
func (r *RateLimit) Blocked(baseURL string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.until[baseURL]
	if !ok {
		return false
	}
	if time.Now().After(t) {
		delete(r.until, baseURL)
		return false
	}
	return true
}

// Remaining returns the cooldown left for baseURL; <= 0 means it may request.
func (r *RateLimit) Remaining(baseURL string) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.until[baseURL]
	if !ok {
		return 0
	}
	d := time.Until(t)
	if d <= 0 {
		delete(r.until, baseURL)
		return 0
	}
	return d
}

// SetRateLimited records a cooldown for baseURL and persists it immediately.
func (r *RateLimit) SetRateLimited(baseURL string, retryAfter time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	r.until[baseURL] = now.Add(retryAfter)
	for b, t := range r.until { // prune expired keys on write
		if !t.After(now) {
			delete(r.until, b)
		}
	}
	return r.writeLocked()
}

// Clear removes the cooldown for baseURL (tests/manual reset).
func (r *RateLimit) Clear(baseURL string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.until, baseURL)
	return r.writeLocked()
}

func (r *RateLimit) writeLocked() error {
	f := rateLimitFile{Version: rateLimitFileVersion, Providers: make(map[string]string)}
	for b, t := range r.until {
		f.Providers[b] = t.UTC().Format(time.RFC3339)
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.file), 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	tmp := r.file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write ratelimit tmp: %w", err)
	}
	if err := os.Rename(tmp, r.file); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename ratelimit: %w", err)
	}
	return nil
}
