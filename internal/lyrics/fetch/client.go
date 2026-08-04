package fetch

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/version"
)

const (
	maxFetchResponse   = 2 << 20 // 2 MiB response cap
	minRequestGap      = 200 * time.Millisecond
	dialTimeout        = 3 * time.Second
	maxRateLimitRetries = 1
	defaultRetryAfter  = 2 * time.Second
	maxRedirects       = 5
)

// Error values returned by the fetch package.
var (
	ErrNotFound           = errors.New("fetch: track not found")
	ErrNoMatch            = errors.New("fetch: no matching candidate")
	ErrRateLimited        = errors.New("fetch: rate limited")
	ErrOffline            = errors.New("fetch: no internet connection")
	ErrUnexpectedResponse = errors.New("fetch: unexpected response")
	ErrResponseTooLarge   = errors.New("fetch: response too large")
)

// RateLimitError carries the server-provided cooldown after retries are spent.
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string {
	return "fetch: rate limited, retry in " + e.RetryAfter.String()
}

func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// TrackMeta is the local metadata used to query a provider.
type TrackMeta struct {
	Title    string
	Artist   string
	Album    string
	Duration float64 // seconds; 0 means unknown
}

// Client talks to a single LRCLIB-compatible provider. It is bound to its
// base URL so cooldown and throttling stay provider-scoped.
type Client struct {
	baseURL    string
	httpClient *http.Client
	rateLimit  *RateLimit // may be nil (tests)
	mu         sync.Mutex
	lastReqAt  time.Time
}

// NewClient builds a client for baseURL. timeout of 0 means no overall timeout.
func NewClient(baseURL string, timeout time.Duration, insecureTLS bool, rateLimit *RateLimit) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext
	if insecureTLS {
		//nolint:gosec // explicit opt-in via config; warned about at config load
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("stopped after 5 redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("redirect to non-http(s) scheme")
			}
			return nil
		},
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
		rateLimit:  rateLimit,
	}
}

// throttle enforces the >=200ms gap between requests of this client.
func (c *Client) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if since := time.Since(c.lastReqAt); since < minRequestGap {
		time.Sleep(minRequestGap - since)
	}
	c.lastReqAt = time.Now()
}

// do sends a request, honoring 429 with one Retry-After-bounded retry.
func (c *Client) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	c.throttle()
	req.Header.Set("User-Agent", version.UserAgent())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if isOfflineErr(err) {
			return nil, ErrOffline
		}
		return nil, fmt.Errorf("fetch request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			wait := parseRetryAfter(resp.Header.Get("Retry-After"))
			logger.Warn("lyrics fetch rate limited", "base_url", c.baseURL, "retry_after", wait)
			resp.Body.Close()

			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			c.throttle()
			resp2, err2 := c.httpClient.Do(req)
			if err2 != nil {
				if isOfflineErr(err2) {
					return nil, ErrOffline
				}
				return nil, fmt.Errorf("fetch retry: %w", err2)
			}
			if resp2.StatusCode == http.StatusTooManyRequests {
				wait2 := parseRetryAfter(resp2.Header.Get("Retry-After"))
				resp2.Body.Close()
				if c.rateLimit != nil {
					if err := c.rateLimit.SetRateLimited(c.baseURL, wait2); err != nil {
						logger.Warn("lyrics fetch: persist rate limit failed", "err", err)
					}
				}
				return nil, &RateLimitError{RetryAfter: wait2}
			}
			if resp2.StatusCode != http.StatusOK {
				return nil, c.errFromResponse(resp2)
			}
			return resp2, nil
		}
		return nil, c.errFromResponse(resp)
	}
	return resp, nil
}

// errFromResponse converts a non-200 response into an error.
func (c *Client) errFromResponse(resp *http.Response) error {
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxFetchResponse))
	var api APIError
	if json.Unmarshal(body, &api) == nil && api.Code != 0 {
		if resp.StatusCode == http.StatusNotFound {
			return ErrNotFound
		}
		return fmt.Errorf("%w: %s (status %d)", ErrUnexpectedResponse, api.Message, resp.StatusCode)
	}
	return fmt.Errorf("%w: status %d", ErrUnexpectedResponse, resp.StatusCode)
}

// readJSON validates and decodes a 200 response body.
func (c *Client) readJSON(resp *http.Response, v any) error {
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return fmt.Errorf("%w: content-type %q", ErrUnexpectedResponse, ct)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchResponse+1))
	if err != nil {
		return err
	}
	if len(body) > maxFetchResponse {
		return ErrResponseTooLarge
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%w: decode: %v", ErrInvalidPayload, err)
	}
	return nil
}

// Get queries /api/get with exact metadata.
func (c *Client) Get(ctx context.Context, meta TrackMeta) (Track, error) {
	params := url.Values{}
	params.Set("artist_name", meta.Artist)
	params.Set("track_name", meta.Title)
	if meta.Album != "" {
		params.Set("album_name", meta.Album)
	}
	if secs := clampDuration(meta.Duration); secs > 0 {
		params.Set("duration", strconv.Itoa(secs))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/get?"+params.Encode(), nil)
	if err != nil {
		return Track{}, err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return Track{}, err
	}
	var t Track
	if err := c.readJSON(resp, &t); err != nil {
		return Track{}, err
	}
	return t, nil
}

// Search queries /api/search. q takes precedence; otherwise structured
// metadata is used.
func (c *Client) Search(ctx context.Context, q string, meta TrackMeta) ([]Track, error) {
	params := url.Values{}
	switch {
	case q != "":
		params.Set("q", q)
	default:
		if meta.Title != "" {
			params.Set("track_name", meta.Title)
		}
		if meta.Artist != "" {
			params.Set("artist_name", meta.Artist)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/search?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	var items []Track
	if err := c.readJSON(resp, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// GetByID fetches a track by absolute LRCLIB id.
func (c *Client) GetByID(ctx context.Context, id int64) (Track, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/get/%d", c.baseURL, id), nil)
	if err != nil {
		return Track{}, err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return Track{}, err
	}
	var t Track
	if err := c.readJSON(resp, &t); err != nil {
		return Track{}, err
	}
	return t, nil
}

// parseRetryAfter parses a Retry-After header in seconds, falling back to 2s.
func parseRetryAfter(s string) time.Duration {
	if s == "" {
		return defaultRetryAfter
	}
	secs, err := strconv.Atoi(s)
	if err != nil || secs <= 0 {
		return defaultRetryAfter
	}
	return time.Duration(secs) * time.Second
}

// clampDuration clamps to the LRCLIB-required 1-3600s range; 0 stays 0.
func clampDuration(d float64) int {
	if d <= 0 {
		return 0
	}
	secs := int(math.Round(d))
	if secs < 1 {
		return 1
	}
	if secs > 3600 {
		return 3600
	}
	return secs
}

// isOfflineErr reports whether err indicates no connectivity, so the caller
// can skip waiting for the full timeout.
func isOfflineErr(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	if errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}
