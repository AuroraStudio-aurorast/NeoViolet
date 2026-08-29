package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func fetchOpts(t *testing.T, baseURL string, cache *Cache) Opts {
	return fetchOptsDir(t, baseURL, cache, t.TempDir())
}

func fetchOptsDir(t *testing.T, baseURL string, cache *Cache, dir string) Opts {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", dir)
	config.SetXDGConfig(true)
	t.Cleanup(func() { config.SetXDGConfig(false) })
	rl, err := LoadRateLimit(dir)
	if err != nil {
		t.Fatalf("LoadRateLimit() error: %v", err)
	}
	return Opts{
		BaseURL:   baseURL,
		Timeout:   5 * time.Second,
		Cache:     cache,
		RateLimit: rl,
	}
}

const shelterLyrics = "[00:01.00]long way from home\n[00:05.00]over the deep blue sea\n"

func trackJSON(id int64, artist string, dur float64, synced string) string {
	b, _ := json.Marshal(Track{
		ID:           id,
		TrackName:    "Shelter",
		ArtistName:   artist,
		AlbumName:    "Shelter",
		Duration:     dur,
		SyncedLyrics: synced,
	})
	return string(b)
}

func TestFetchLyricsGetHit(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(trackJSON(1, "Porter Robinson & Madeon", 219, shelterLyrics)))
	}))
	defer srv.Close()

	cache := NewCache()
	data, err := Lyrics(context.Background(),
		TrackMeta{Title: "Shelter", Artist: "Porter Robinson/Madeon", Album: "Shelter", Duration: 219},
		fetchOpts(t, srv.URL, cache))
	if err != nil {
		t.Fatalf("Lyrics() error: %v", err)
	}
	if data.Format != "lrclib" || data.Path != "lrclib://1" {
		t.Errorf("Format/Path = %q/%q, want lrclib/lrclib://1", data.Format, data.Path)
	}
	if len(data.Lines) != 2 {
		t.Errorf("lines = %d, want 2", len(data.Lines))
	}
	if hits != 1 {
		t.Errorf("server hits = %d, want 1", hits)
	}

	// second call for the same sig must hit the session cache (0 requests)
	sig := Sign("Shelter", "Porter Robinson/Madeon", "Shelter", 219)
	if state, _, ok := cache.Lookup(sig); !ok || state != CacheFound {
		t.Errorf("cache state = %v, %v, want CacheFound", state, ok)
	}
}

func TestFetchLyricsGet404SearchFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/get"):
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":404,"name":"TrackNotFound","message":"nope"}`))
		case strings.HasPrefix(r.URL.Path, "/api/search"):
			// two candidates; only the 2nd matches artists, with synced lyrics
			w.Write([]byte(`[` +
				trackJSON(10, "Someone Else", 219, shelterLyrics) + `,` +
				trackJSON(11, "Porter Robinson & Madeon", 221, shelterLyrics) + `]`))
		}
	}))
	defer srv.Close()

	data, err := Lyrics(context.Background(),
		TrackMeta{Title: "Shelter", Artist: "Porter Robinson/Madeon", Duration: 219},
		fetchOpts(t, srv.URL, NewCache()))
	if err != nil {
		t.Fatalf("Lyrics() error: %v", err)
	}
	if data.Path != "lrclib://11" {
		t.Errorf("Path = %q, want lrclib://11 (best candidate)", data.Path)
	}
}

func TestFetchLyricsNoMatchingCandidate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/get") {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.Write([]byte(`[` + trackJSON(10, "Completely Different", 100, shelterLyrics) + `]`))
		}
	}))
	defer srv.Close()

	_, err := Lyrics(context.Background(),
		TrackMeta{Title: "Shelter", Artist: "Porter Robinson/Madeon", Duration: 219},
		fetchOpts(t, srv.URL, NewCache()))
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("Lyrics() error = %v, want ErrNoMatch", err)
	}
}

func TestFetchLyricsPlainOnlyAbandoned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/get") {
			w.WriteHeader(http.StatusNotFound)
		} else {
			b, _ := json.Marshal(Track{ID: 10, TrackName: "Shelter", ArtistName: "Porter Robinson & Madeon", Duration: 219, PlainLyrics: "just text"})
			w.Write([]byte(`[` + string(b) + `]`))
		}
	}))
	defer srv.Close()

	_, err := Lyrics(context.Background(),
		TrackMeta{Title: "Shelter", Artist: "Porter Robinson/Madeon", Duration: 219},
		fetchOpts(t, srv.URL, NewCache()))
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("Lyrics() error = %v, want ErrNoMatch (plain-only)", err)
	}
}

func TestFetchLyricsInstrumental(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		b, _ := json.Marshal(Track{ID: 1, TrackName: "Shelter", ArtistName: "Porter Robinson & Madeon", Duration: 219, Instrumental: true})
		w.Write([]byte(string(b)))
	}))
	defer srv.Close()

	_, err := Lyrics(context.Background(),
		TrackMeta{Title: "Shelter", Artist: "Porter Robinson/Madeon", Duration: 219},
		fetchOpts(t, srv.URL, NewCache()))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Lyrics() error = %v, want ErrNotFound (instrumental)", err)
	}
}

func TestFetchLyricsDiskCacheHit(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(trackJSON(1, "Porter Robinson & Madeon", 219, shelterLyrics)))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	meta := TrackMeta{Title: "Shelter", Artist: "Porter Robinson/Madeon", Duration: 219}

	// first call populates the disk cache
	if _, err := Lyrics(context.Background(), meta, fetchOptsDir(t, srv.URL, NewCache(), cacheDir)); err != nil {
		t.Fatalf("first Lyrics() error: %v", err)
	}
	if hits != 1 {
		t.Fatalf("first call hits = %d, want 1", hits)
	}

	// second call with a fresh session cache must be served from disk
	data, err := Lyrics(context.Background(), meta, fetchOptsDir(t, srv.URL, NewCache(), cacheDir))
	if err != nil {
		t.Fatalf("second Lyrics() error: %v", err)
	}
	if hits != 1 {
		t.Errorf("second call hits = %d, want 1 (disk cache)", hits)
	}
	if data.Path != "lrclib://1" {
		t.Errorf("Path = %q, want lrclib://1", data.Path)
	}
}

func TestFetchLyricsInvalidPayloadNoCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":1,"trackName":"Shelter","artistName":"X","duration":10,"syncedLyrics":"not lrc at all"}`))
	}))
	defer srv.Close()

	cache := NewCache()
	_, err := Lyrics(context.Background(),
		TrackMeta{Title: "Shelter", Artist: "Porter Robinson/Madeon", Duration: 219},
		fetchOpts(t, srv.URL, cache))
	if err == nil {
		t.Fatal("Lyrics() error = nil, want invalid payload error")
	}
	// transient error must not poison the negative cache
	sig := Sign("Shelter", "Porter Robinson/Madeon", "", 219)
	if _, _, ok := cache.Lookup(sig); ok {
		t.Error("invalid payload cached a negative entry")
	}
}

func TestFetchLyricsOfflineNoCache(t *testing.T) {
	cache := NewCache()
	_, err := Lyrics(context.Background(),
		TrackMeta{Title: "T", Artist: "A", Duration: 10},
		Opts{BaseURL: "http://127.0.0.1:1", Timeout: 5 * time.Second, Cache: cache})
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("Lyrics() error = %v, want ErrOffline", err)
	}
	if _, _, ok := cache.Lookup(Sign("T", "A", "", 10)); ok {
		t.Error("offline result cached as negative")
	}
}
