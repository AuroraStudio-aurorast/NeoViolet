package fetch

import (
	"sync"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

// CacheState is the per-signature result state held in the session cache.
type CacheState int

const (
	// CacheFound holds fetched lyrics.
	CacheFound CacheState = iota
	// CacheNotFound is a negative cache entry (track has no lyrics).
	CacheNotFound
	// CacheRateLimited mirrors the provider cooldown for quick lookups;
	// the authoritative check is rateLimit.Blocked (see ratelimit.go).
	CacheRateLimited
	// CachePending marks an in-flight request so concurrent lookups skip.
	CachePending
)

type cacheEntry struct {
	state            CacheState
	data             *lyrics.LyricsData
	at               time.Time
	rateLimitedUntil time.Time
}

// Cache is a session-scoped, per-signature lyrics cache.
type Cache struct {
	mu    sync.Mutex
	items map[string]cacheEntry
}

// NewCache returns an empty cache.
func NewCache() *Cache { return &Cache{items: make(map[string]cacheEntry)} }

// Lookup returns the state for sig. An expired rate-limited entry is treated
// as a miss so the track can be retried.
func (c *Cache) Lookup(sig string) (CacheState, *lyrics.LyricsData, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[sig]
	if !ok {
		return 0, nil, false
	}
	if e.state == CacheRateLimited && time.Now().After(e.rateLimitedUntil) {
		delete(c.items, sig)
		return 0, nil, false
	}
	return e.state, e.data, true
}

// Store records a result for sig.
func (c *Cache) Store(sig string, state CacheState, data *lyrics.LyricsData) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[sig] = cacheEntry{state: state, data: data, at: time.Now()}
}

// StoreRateLimited records a cooldown mirror for sig.
func (c *Cache) StoreRateLimited(sig string, retryAfter time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[sig] = cacheEntry{
		state:            CacheRateLimited,
		at:               time.Now(),
		rateLimitedUntil: time.Now().Add(retryAfter),
	}
}

// Clear removes sig, allowing a manual re-fetch to bypass negative entries.
func (c *Cache) Clear(sig string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, sig)
}
