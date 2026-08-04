package ui

import (
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics/fetch"
)

func newFetchTestModel() *Model {
	cfg := config.DefaultConfig()
	m := NewModel("", &cfg)
	m.fetchCache = fetch.NewCache()
	m.fetchRateLimit = fetch.NewRateLimit()
	return m
}

func TestMaybeFetchLyricsPreconditions(t *testing.T) {
	m := newFetchTestModel()
	m.Audio.CurrentSong = "Shelter"
	m.Audio.Artist = "Porter Robinson"
	m.Audio.Duration = 219 * time.Second

	tests := []struct {
		name   string
		mutate func(*Model)
		want   bool
	}{
		{"all satisfied", func(m *Model) {}, true},
		{"fetch disabled", func(m *Model) {
			m.Config.Lyrics.Fetch.Enabled = false
		}, false},
		{"no online entry", func(m *Model) {
			m.Config.Lyrics.FormatPriority = []string{"embedded", "lrc"}
		}, false},
		{"unknown artist", func(m *Model) {
			m.Audio.Artist = "Unknown Artist"
		}, false},
		{"synthetic format", func(m *Model) {
			m.Audio.CurrentSong = "song.mid"
			m.Audio.Artist = "Porter Robinson"
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newFetchTestModel()
			m.Audio.CurrentSong = "Shelter"
			m.Audio.Artist = "Porter Robinson"
			m.Audio.Duration = 219 * time.Second
			tt.mutate(m)
			path := "/music/Shelter.mp3"
			if m.Audio.CurrentSong == "song.mid" {
				path = "/music/song.mid"
			}
			got := m.maybeFetchLyrics(path) != nil
			if got != tt.want {
				t.Errorf("maybeFetchLyrics() cmd = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMaybeFetchLyricsCacheGating(t *testing.T) {
	m := newFetchTestModel()
	m.Audio.CurrentSong = "Shelter"
	m.Audio.Artist = "Porter Robinson"
	m.Audio.Duration = 219 * time.Second
	sig := m.currentSig()

	// negative cache blocks the request
	m.fetchCache.Store(sig, fetch.CacheNotFound, nil)
	if cmd := m.maybeFetchLyrics("/music/Shelter.mp3"); cmd != nil {
		t.Error("maybeFetchLyrics() with negative cache = cmd, want nil")
	}

	// pending blocks duplicate requests
	m.fetchCache.Clear(sig)
	m.fetchCache.Store(sig, fetch.CachePending, nil)
	if cmd := m.maybeFetchLyrics("/music/Shelter.mp3"); cmd != nil {
		t.Error("maybeFetchLyrics() with pending = cmd, want nil")
	}

	// after clear it fetches again
	m.fetchCache.Clear(sig)
	if cmd := m.maybeFetchLyrics("/music/Shelter.mp3"); cmd == nil {
		t.Error("maybeFetchLyrics() after clear = nil, want cmd")
	}
}

func TestMaybeFetchLyricsRateLimitGate(t *testing.T) {
	m := newFetchTestModel()
	m.Audio.CurrentSong = "Shelter"
	m.Audio.Artist = "Porter Robinson"
	m.Audio.Duration = 219 * time.Second

	// Point the rate limit state at a writable temp dir.
	rl, err := fetch.LoadRateLimit(t.TempDir())
	if err != nil {
		t.Fatalf("LoadRateLimit: %v", err)
	}
	m.fetchRateLimit = rl

	if err := rl.SetRateLimited(config.DefaultBaseURL, time.Hour); err != nil {
		t.Fatalf("SetRateLimited: %v", err)
	}
	if cmd := m.maybeFetchLyrics("/music/Shelter.mp3"); cmd != nil {
		t.Error("maybeFetchLyrics() during cooldown = cmd, want nil")
	}
}

func TestHandleFetchLyricsResultStale(t *testing.T) {
	m := newFetchTestModel()
	m.Audio.CurrentSong = "Shelter"
	m.Audio.Artist = "Porter Robinson"
	m.Audio.Duration = 219 * time.Second
	m.LyricsFetching = true

	// result for a different track must not mount or clear the indicator
	other := fetch.Sign("Other Song", "Other Artist", "", 100)
	_, _ = handleFetchLyricsResult(m, FetchLyricsResultMsg{
		Data: &lyrics.LyricsData{Format: "lrclib"},
		Sig:  other,
	})
	if m.Audio.Lyrics != nil {
		t.Error("stale result mounted lyrics")
	}
	if !m.LyricsFetching {
		t.Error("stale result cleared LyricsFetching")
	}
}

func TestHandleFetchLyricsResultMount(t *testing.T) {
	m := newFetchTestModel()
	m.Audio.CurrentSong = "Shelter"
	m.Audio.Artist = "Porter Robinson"
	m.Audio.Duration = 219 * time.Second
	m.LyricsFetching = true

	data := &lyrics.LyricsData{Format: "lrclib", Path: "lrclib://1"}
	_, _ = handleFetchLyricsResult(m, FetchLyricsResultMsg{Data: data, Sig: m.currentSig()})
	if m.Audio.Lyrics != data {
		t.Error("result did not mount lyrics")
	}
	if !m.Audio.ShowLyrics {
		t.Error("ShowLyrics not set")
	}
	if m.LyricsFetching {
		t.Error("LyricsFetching not cleared after result")
	}
}

func TestHandleFetchLyricsResultError(t *testing.T) {
	m := newFetchTestModel()
	m.Audio.CurrentSong = "Shelter"
	m.Audio.Artist = "Porter Robinson"
	m.Audio.Duration = 219 * time.Second
	m.LyricsFetching = true
	sig := m.currentSig()

	// FetchLyrics stores the negative result itself; the handler must not
	// clear it, only drop the display and the fetching indicator.
	m.fetchCache.Store(sig, fetch.CacheNotFound, nil)
	_, _ = handleFetchLyricsResult(m, FetchLyricsResultMsg{Err: fetch.ErrNotFound, Sig: sig})
	if m.LyricsFetching {
		t.Error("LyricsFetching not cleared on error")
	}
	if _, _, ok := m.fetchCache.Lookup(sig); !ok {
		t.Error("negative cache entry cleared")
	}
}

func TestHandleFetchLyricsResultTransientClearsPending(t *testing.T) {
	m := newFetchTestModel()
	m.Audio.CurrentSong = "Shelter"
	m.Audio.Artist = "Porter Robinson"
	m.Audio.Duration = 219 * time.Second
	sig := m.currentSig()
	m.fetchCache.Store(sig, fetch.CachePending, nil)
	m.LyricsFetching = true

	_, _ = handleFetchLyricsResult(m, FetchLyricsResultMsg{Err: fetch.ErrOffline, Sig: sig})
	if _, _, ok := m.fetchCache.Lookup(sig); ok {
		t.Error("transient error left pending cache entry")
	}
}
