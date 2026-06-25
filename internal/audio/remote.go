package audio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/speaker"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format"
		"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

func isURL(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

func detectFormatFromContentType(ct string) string {
	return format.MIMETypeToExt(ct)
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
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	f, err := os.CreateTemp(tempDir, "cache-*.tmp")
	if err != nil {
		os.RemoveAll(tempDir)
		resp.Body.Close()
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
			r.body.Close()
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
	r.body.Close()
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
	r.body.Close()

	name := r.cacheFile.Name()
	r.cacheFile.Close()
	os.Remove(name)
	if r.tempDir != "" {
		os.RemoveAll(r.tempDir)
	}
	return nil
}

func (p *Player) openURL(urlStr string) error {
	logger.Info("Opening remote audio", "url", urlStr)

	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(u.Path))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(urlStr)
	if err != nil {
		return fmt.Errorf("HTTP GET: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	if ext == "" {
		ext = detectFormatFromContentType(resp.Header.Get("Content-Type"))
	}
	if ext == "" {
		resp.Body.Close()
		return fmt.Errorf("cannot detect audio format from URL or Content-Type")
	}

	if isSyntheticFormat(ext) {
		resp.Body.Close()
		return fmt.Errorf("remote playback of %s format is not supported", ext)
	}

	if p.isPlaying {
		speaker.Clear()
		p.isPlaying = false
	}
	if p.streamer != nil {
		if p.file != nil {
			p.file.Close()
		}
	}

	rrs, err := newRemoteReadSeeker(resp)
	if err != nil {
		return err
	}

	streamer, format, err := p.decoder.DecodeFromReader(rrs, ext)
	if err != nil {
		rrs.Close()
		return err
	}

	if err := ensureSpeakerInit(format.SampleRate); err != nil {
		rrs.Close()
		return fmt.Errorf("speaker init failed: %w", err)
	}

	ctrlStreamer := resampleIfNeeded(streamer, format)

	logger.Info("Remote audio opened", "url", urlStr, "format", format.SampleRate)

	p.streamer = streamer
	p.format = format
	p.file = rrs
	p.isPaused = true
	p.isPlaying = false
	p.path = urlStr

	p.ctrl = &beep.Ctrl{
		Streamer: ctrlStreamer,
		Paused:   true,
	}

	p.volume = &effects.Volume{
		Streamer: p.ctrl,
		Base:     2,
		Silent:   false,
	}

	p.applyLinearVolumeLocked()

	display := urlStr
	if u.Scheme != "" {
		display = u.Host + u.RequestURI()
	}
	p.title = display
	p.artist = ""

	return nil
}