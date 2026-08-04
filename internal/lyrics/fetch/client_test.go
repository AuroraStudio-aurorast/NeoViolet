package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, h http.Handler) (*Client, string) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, 5*time.Second, false, nil)
	return c, srv.URL
}

func TestClientGet(t *testing.T) {
	var gotURL, gotUA string
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":1,"trackName":"Shelter","artistName":"Porter Robinson & Madeon","duration":219,"syncedLyrics":"[00:01.00]hi"}`))
	}))

	track, err := c.Get(context.Background(), TrackMeta{Title: "Shelter", Artist: "Porter Robinson/Madeon", Album: "Shelter", Duration: 219.01})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if track.ID != 1 || track.TrackName != "Shelter" {
		t.Errorf("Get() = %+v", track)
	}
	if !strings.Contains(gotURL, "track_name=Shelter") || !strings.Contains(gotURL, "duration=219") {
		t.Errorf("Get() URL = %q, want encoded params", gotURL)
	}
	if !strings.HasPrefix(gotUA, "NEOVIOLET v") {
		t.Errorf("User-Agent = %q, want NEOVIOLET prefix", gotUA)
	}
}

func TestClientGetNotFound(t *testing.T) {
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":404,"name":"TrackNotFound","message":"Failed to find specified track"}`))
	}))
	if _, err := c.Get(context.Background(), TrackMeta{Title: "X", Artist: "Y"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestClientGetRateLimitedRetry(t *testing.T) {
	var attempts atomic.Int32
	rl := &RateLimit{}
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":1,"trackName":"T","artistName":"A","duration":10,"syncedLyrics":"[00:01.00]x"}`))
	}))
	c.rateLimit = rl

	start := time.Now()
	if _, err := c.Get(context.Background(), TrackMeta{Title: "T", Artist: "A"}); err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if attempts.Load() != 2 {
		t.Errorf("attempts = %d, want 2 (one retry)", attempts.Load())
	}
	if time.Since(start) < time.Second {
		t.Error("Get() returned before Retry-After elapsed")
	}
}

func TestClientGetRateLimitedExhausted(t *testing.T) {
	rl, err := LoadRateLimit(t.TempDir())
	if err != nil {
		t.Fatalf("LoadRateLimit() error: %v", err)
	}
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	c.rateLimit = rl

	_, err = c.Get(context.Background(), TrackMeta{Title: "T", Artist: "A"})
	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("Get() error = %v, want RateLimitError", err)
	}
	if rle.RetryAfter != time.Second {
		t.Errorf("RetryAfter = %v, want 1s", rle.RetryAfter)
	}
	// cooldown must be persisted on the shared rate limit state
	if !rl.Blocked(c.baseURL) {
		t.Error("provider not marked rate limited after exhaustion")
	}
}

func TestClientContentTypeRejected(t *testing.T) {
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html>proxy error</html>`))
	}))
	if _, err := c.Get(context.Background(), TrackMeta{Title: "T", Artist: "A"}); !errors.Is(err, ErrUnexpectedResponse) {
		t.Errorf("Get() error = %v, want ErrUnexpectedResponse", err)
	}
}

func TestClientResponseTooLarge(t *testing.T) {
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(make([]byte, maxFetchResponse+1024))
	}))
	if _, err := c.Get(context.Background(), TrackMeta{Title: "T", Artist: "A"}); !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("Get() error = %v, want ErrResponseTooLarge", err)
	}
}

func TestClientSearch(t *testing.T) {
	var gotURL string
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":1,"trackName":"T","artistName":"A","duration":10}]`))
	}))

	items, err := c.Search(context.Background(), "", TrackMeta{Title: "T", Artist: "A"})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(items) != 1 || items[0].ID != 1 {
		t.Errorf("Search() = %+v", items)
	}
	if !strings.Contains(gotURL, "/api/search?") || !strings.Contains(gotURL, "track_name=T") {
		t.Errorf("Search() URL = %q", gotURL)
	}
}

func TestClientSearchQueryPriority(t *testing.T) {
	var gotURL string
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	if _, err := c.Search(context.Background(), "query", TrackMeta{Title: "T", Artist: "A"}); err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if !strings.Contains(gotURL, "q=query") || strings.Contains(gotURL, "track_name=") {
		t.Errorf("Search() URL = %q, q must take precedence", gotURL)
	}
}

func TestClientOfflineRefused(t *testing.T) {
	// Port 1 on loopback is not listening anywhere; the dial fails
	// immediately with ECONNREFUSED, which must map to ErrOffline.
	c := NewClient("http://127.0.0.1:1", 5*time.Second, false, nil)
	start := time.Now()
	_, err := c.Get(context.Background(), TrackMeta{Title: "T", Artist: "A"})
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("Get() error = %v, want ErrOffline", err)
	}
	if time.Since(start) > time.Second {
		t.Error("offline detection should return quickly, not wait for timeout")
	}
}

func TestClientThrottle(t *testing.T) {
	var hits atomic.Int32
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":1,"trackName":"T","artistName":"A","duration":10,"syncedLyrics":"[00:01.00]x"}`))
	}))

	start := time.Now()
	if _, err := c.Get(context.Background(), TrackMeta{Title: "T", Artist: "A"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), TrackMeta{Title: "T", Artist: "A"}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Errorf("hits = %d, want 2", hits.Load())
	}
	if time.Since(start) < minRequestGap {
		t.Error("two requests did not respect the minimum gap")
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct{ in string; want time.Duration }{
		{"", defaultRetryAfter},
		{"3", 3 * time.Second},
		{"-1", defaultRetryAfter},
		{"abc", defaultRetryAfter},
	}
	for _, tt := range tests {
		if got := parseRetryAfter(tt.in); got != tt.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestClampDuration(t *testing.T) {
	tests := []struct{ in float64; want int }{
		{0, 0},
		{219.01, 219},
		{0.4, 1},
		{5000, 3600},
	}
	for _, tt := range tests {
		if got := clampDuration(tt.in); got != tt.want {
			t.Errorf("clampDuration(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
