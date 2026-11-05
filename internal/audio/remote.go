package audio

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

func isURL(path string) bool {
	u, err := url.Parse(path)
	return err == nil && u.Hostname() != "" && (strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https"))
}

// RedactSource removes remote URL credentials, query parameters and fragments
// before a source is shown to the user or written to logs. Local paths pass
// through unchanged.
func RedactSource(source string) string {
	u, err := url.Parse(source)
	if err != nil || (!strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https")) {
		return source
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	return u.String()
}

func newRemoteHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 15 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.ExpectContinueTimeout = 1 * time.Second
	return &http.Client{Transport: transport}
}

type remoteReadSeeker struct {
	ctx          context.Context
	cancel       context.CancelFunc
	body         io.ReadCloser
	cacheFile    *os.File
	tempDir      string
	size         int64
	pos          int64
	written      int64
	err          error
	downloadDone bool
	closed       bool
	mu           sync.Mutex
	cond         *sync.Cond
}

func newRemoteReadSeeker(resp *http.Response) (*remoteReadSeeker, error) {
	ctx, cancel := context.WithCancel(context.Background())
	// Use a private temp directory so other local users cannot read cached audio.
	tempDir, err := os.MkdirTemp("", "neoviolet-*")
	if err != nil {
		_ = resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	f, err := os.CreateTemp(tempDir, "cache-*.tmp")
	if err != nil {
		_ = os.RemoveAll(tempDir)
		_ = resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("create temp cache: %w", err)
	}

	r := &remoteReadSeeker{
		ctx:       ctx,
		cancel:    cancel,
		body:      resp.Body,
		cacheFile: f,
		tempDir:   tempDir,
		size:      resp.ContentLength,
	}
	r.cond = sync.NewCond(&r.mu)
	go r.download()
	return r, nil
}

func (r *remoteReadSeeker) download() {
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		n, err := r.body.Read(buf)
		if n > 0 {
			r.mu.Lock()
			_, werr := r.cacheFile.WriteAt(buf[:n], r.written)
			r.written += int64(n)
			r.cond.Broadcast()
			r.mu.Unlock()
			if werr != nil {
				r.setError(fmt.Errorf("write cache: %w", werr))
				return
			}
		}
		if err == io.EOF {
			r.mu.Lock()
			r.size = r.written
			r.downloadDone = true
			r.cond.Broadcast()
			r.mu.Unlock()
			_ = r.body.Close()
			return
		}
		if err != nil {
			r.setError(fmt.Errorf("download: %w", err))
			return
		}
	}
}

func (r *remoteReadSeeker) setError(err error) {
	r.mu.Lock()
	r.err = err
	r.downloadDone = true
	r.cond.Broadcast()
	r.mu.Unlock()
	_ = r.body.Close()
}

func (r *remoteReadSeeker) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for r.pos >= r.written && !r.downloadDone && r.err == nil {
		r.cond.Wait()
	}

	if r.pos >= r.written {
		if r.err != nil {
			return 0, r.err
		}
		return 0, io.EOF
	}

	n := int64(len(p))
	if r.pos+n > r.written {
		n = r.written - r.pos
	}

	nn, err := r.cacheFile.ReadAt(p[:n], r.pos)
	r.pos += int64(nn)
	return nn, err
}

func (r *remoteReadSeeker) Seek(offset int64, whence int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.pos + offset
	case io.SeekEnd:
		for r.size < 0 && !r.downloadDone && r.err == nil {
			r.cond.Wait()
		}
		if r.err != nil {
			return 0, r.err
		}
		abs = r.size + offset
	}

	if abs < 0 {
		abs = 0
	}
	if r.size >= 0 && abs > r.size {
		abs = r.size
	}

	r.pos = abs
	return abs, nil
}

func (r *remoteReadSeeker) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()

	r.cancel()
	_ = r.body.Close()

	name := r.cacheFile.Name()
	_ = r.cacheFile.Close()
	// #nosec G703 -- name is a program-generated temp file path, not user input.
	_ = os.Remove(name)
	if r.tempDir != "" {
		// #nosec G703 -- tempDir is a program-generated temp dir, not user input.
		_ = os.RemoveAll(r.tempDir)
	}
	return nil
}

func (p *Player) openURL(urlStr string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	logger.Info("Opening remote audio", "url", RedactSource(urlStr))

	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	urlStr = u.String()

	ext := strings.ToLower(filepath.Ext(u.Path))

	client := newRemoteHTTPClient()
	// #nosec G704 -- fetching a user-supplied remote audio URL is the core
	// feature of remote playback; the URL comes from the user's own input.
	resp, err := client.Get(urlStr)
	if err != nil {
		return fmt.Errorf("HTTP GET: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	if ext == "" {
		ext = format.MIMETypeToExt(resp.Header.Get("Content-Type"))
	}
	if ext == "" {
		_ = resp.Body.Close()
		return fmt.Errorf("cannot detect audio format from URL or Content-Type")
	}

	if isSyntheticFormat(ext) {
		_ = resp.Body.Close()
		return fmt.Errorf("remote playback of %s format is not supported", ext)
	}

	// Release the previous track before opening the next one.
	_ = p.closeStreamer()

	rrs, err := newRemoteReadSeeker(resp)
	if err != nil {
		return err
	}

	streamer, format, err := p.decoder.DecodeFromReader(rrs, ext)
	if err != nil {
		_ = rrs.Close()
		return err
	}

	if err := ensureSpeakerInit(format.SampleRate); err != nil {
		_ = streamer.Close()
		_ = rrs.Close()
		return fmt.Errorf("speaker init failed: %w", err)
	}

	logger.Info("Remote audio opened", "url", RedactSource(urlStr), "format", format.SampleRate)

	p.setupStreamer(streamer, format, rrs, urlStr)

	display := RedactSource(urlStr)
	if safeURL, err := url.Parse(display); err == nil && safeURL.Scheme != "" {
		display = safeURL.Host + safeURL.RequestURI()
	}
	p.title = display
	p.artist = ""

	return nil
}
