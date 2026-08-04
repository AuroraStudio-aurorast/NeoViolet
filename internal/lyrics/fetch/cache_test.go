package fetch

import (
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

func TestCacheFound(t *testing.T) {
	c := NewCache()
	data := &lyrics.LyricsData{}
	c.Store("sig1", CacheFound, data)

	state, got, ok := c.Lookup("sig1")
	if !ok || state != CacheFound || got != data {
		t.Errorf("Lookup() = (%v, %v, %v), want (CacheFound, data, true)", state, got, ok)
	}
}

func TestCacheNotFound(t *testing.T) {
	c := NewCache()
	c.Store("sig1", CacheNotFound, nil)

	state, _, ok := c.Lookup("sig1")
	if !ok || state != CacheNotFound {
		t.Errorf("Lookup() = (%v, _, %v), want (CacheNotFound, _, true)", state, ok)
	}
}

func TestCacheMiss(t *testing.T) {
	c := NewCache()
	if _, _, ok := c.Lookup("missing"); ok {
		t.Error("Lookup(missing) ok = true, want false")
	}
}

func TestCacheRateLimitedExpiry(t *testing.T) {
	c := NewCache()
	c.StoreRateLimited("sig1", 50*time.Millisecond)

	state, _, ok := c.Lookup("sig1")
	if !ok || state != CacheRateLimited {
		t.Fatalf("Lookup() = (%v, _, %v), want (CacheRateLimited, _, true)", state, ok)
	}

	time.Sleep(60 * time.Millisecond)
	if _, _, ok := c.Lookup("sig1"); ok {
		t.Error("Lookup() after cooldown expiry = ok, want miss")
	}
}

func TestCachePending(t *testing.T) {
	c := NewCache()
	c.Store("sig1", CachePending, nil)

	state, _, ok := c.Lookup("sig1")
	if !ok || state != CachePending {
		t.Errorf("Lookup() = (%v, _, %v), want (CachePending, _, true)", state, ok)
	}
}

func TestCacheClear(t *testing.T) {
	c := NewCache()
	c.Store("sig1", CacheNotFound, nil)
	c.Clear("sig1")
	if _, _, ok := c.Lookup("sig1"); ok {
		t.Error("Lookup() after Clear = ok, want miss")
	}
}

func TestCacheKeysIndependent(t *testing.T) {
	c := NewCache()
	c.Store("sig-a", CacheNotFound, nil)
	if _, _, ok := c.Lookup("sig-b"); ok {
		t.Error("sig-b should not be affected by sig-a entry")
	}
}
