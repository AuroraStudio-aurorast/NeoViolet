package fetch

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

// FetchOpts configures a FetchLyrics call.
type FetchOpts struct {
	BaseURL     string
	Timeout     time.Duration
	InsecureTLS bool
	Basic       bool // basic content validation tier
	Cache       *Cache
	RateLimit   *RateLimit
}

type fetcher struct {
	opts    FetchOpts
	client  *Client
	baseURL string
}

// FetchLyrics retrieves lyrics for meta from the provider at opts.BaseURL.
// On success it persists the result to the session and disk caches. Transient
// errors (offline, rate limited, bad payload) are returned without caching so
// a later attempt can succeed; "no lyrics" results are cached as negatives.
func FetchLyrics(ctx context.Context, meta TrackMeta, opts FetchOpts) (*lyrics.LyricsData, error) {
	f := &fetcher{opts: opts, baseURL: strings.TrimRight(opts.BaseURL, "/")}
	f.client = NewClient(f.baseURL, opts.Timeout, opts.InsecureTLS, opts.RateLimit)
	sig := Sign(meta.Title, meta.Artist, meta.Album, meta.Duration)

	// Disk cache first (0 requests).
	if cf, ok := Load(sig, f.baseURL); ok {
		if cf.State == "found" {
			if data, err := lyricsFromCacheFile(cf); err == nil {
				opts.Cache.Store(sig, CacheFound, data)
				return data, nil
			}
		}
		// negative or instrumental record: no lyrics
		opts.Cache.Store(sig, CacheNotFound, nil)
		return nil, ErrNotFound
	}

	track, err := f.client.Get(ctx, meta)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return f.searchFallback(ctx, meta, sig)
		}
		return nil, err // offline / rate limited / transient: no caching
	}

	return f.trackResult(ctx, track, meta, sig, true)
}

// searchFallback queries /api/search and picks the best validated candidate.
func (f *fetcher) searchFallback(ctx context.Context, meta TrackMeta, sig string) (*lyrics.LyricsData, error) {
	items, err := f.client.Search(ctx, "", meta)
	if err != nil {
		return nil, err
	}
	best := pickBest(items, meta)
	if best == nil {
		f.storeNegative(sig, "notfound")
		return nil, ErrNoMatch
	}
	return f.trackResult(ctx, *best, meta, sig, true)
}

// trackResult validates, parses and caches a matched track.
func (f *fetcher) trackResult(ctx context.Context, track Track, meta TrackMeta, sig string, cache bool) (*lyrics.LyricsData, error) {
	if IsInstrumental(track) {
		f.storeNegative(sig, "instrumental")
		return nil, ErrNotFound
	}
	if err := ValidateTrack(track, ValidateOpts{Basic: f.opts.Basic, ReqDuration: meta.Duration}); err != nil {
		return nil, err
	}
	data, err := lyricsFromTrack(track)
	if err != nil {
		return nil, err
	}
	if cache {
		f.storeFound(sig, track, data)
	}
	return data, nil
}

// storeFound persists a positive result to session and disk caches.
func (f *fetcher) storeFound(sig string, track Track, data *lyrics.LyricsData) {
	cf := CacheFile{
		Version:      cacheFileVersion,
		Sig:          sig,
		Provider:     f.baseURL,
		State:        "found",
		TrackID:      track.ID,
		TrackName:    track.TrackName,
		ArtistName:   track.ArtistName,
		AlbumName:    track.AlbumName,
		Duration:     int(track.Duration),
		FetchedAt:    time.Now(),
		SyncedLyrics: cleanControl(track.SyncedLyrics),
		PlainLyrics:  cleanControl(track.PlainLyrics),
	}
	_ = Save(cf) // disk failure degrades to memory-only
	f.opts.Cache.Store(sig, CacheFound, data)
}

// storeNegative persists a no-lyrics result (notfound or instrumental).
func (f *fetcher) storeNegative(sig, state string) {
	cf := CacheFile{
		Version:   cacheFileVersion,
		Sig:       sig,
		Provider:  f.baseURL,
		State:     state,
		FetchedAt: time.Now(),
	}
	_ = Save(cf)
	f.opts.Cache.Store(sig, CacheNotFound, nil)
}

// pickBest returns the candidate matching title and artists with the closest
// duration, preferring synced lyrics. Nil means no candidate passes.
func pickBest(items []Track, meta TrackMeta) *Track {
	type scored struct {
		track *Track
		delta float64
	}
	var pass []scored
	for i := range items {
		t := &items[i]
		if !MatchTrack(meta.Title, t.TrackName) || !MatchArtist(meta.Artist, t.ArtistName, true) {
			continue
		}
		if strings.TrimSpace(t.SyncedLyrics) == "" {
			continue // plain-only candidates are abandoned (D2)
		}
		delta := 0.0
		if meta.Duration > 0 {
			delta = t.Duration - meta.Duration
			if delta < 0 {
				delta = -delta
			}
		}
		pass = append(pass, scored{track: t, delta: delta})
	}
	if len(pass) == 0 {
		return nil
	}
	sort.Slice(pass, func(i, j int) bool { return pass[i].delta < pass[j].delta })
	return pass[0].track
}

// lyricsFromTrack parses a fetched track into LyricsData.
func lyricsFromTrack(t Track) (*lyrics.LyricsData, error) {
	parsed, err := lyrics.ParseLRC(strings.NewReader(cleanControl(t.SyncedLyrics)))
	if err != nil {
		return nil, fmt.Errorf("parse fetched lyrics: %w", err)
	}
	parsed.Format = "lrclib"
	parsed.Path = fmt.Sprintf("lrclib://%d", t.ID)
	if parsed.Title == "" {
		parsed.Title = t.TrackName
	}
	if parsed.Artist == "" {
		parsed.Artist = t.ArtistName
	}
	if parsed.Album == "" {
		parsed.Album = t.AlbumName
	}
	return parsed, nil
}

// lyricsFromCacheFile rebuilds LyricsData from a disk cache record.
func lyricsFromCacheFile(cf *CacheFile) (*lyrics.LyricsData, error) {
	parsed, err := lyrics.ParseLRC(strings.NewReader(cf.SyncedLyrics))
	if err != nil {
		return nil, err
	}
	parsed.Format = "lrclib"
	parsed.Path = fmt.Sprintf("lrclib://%d", cf.TrackID)
	if parsed.Title == "" {
		parsed.Title = cf.TrackName
	}
	if parsed.Artist == "" {
		parsed.Artist = cf.ArtistName
	}
	if parsed.Album == "" {
		parsed.Album = cf.AlbumName
	}
	return parsed, nil
}
