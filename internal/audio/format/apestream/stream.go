package apestream

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/gopxl/beep/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/streamcore"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// StreamInfo carries the audio parameters of a decoded APE file.
type StreamInfo struct {
	SampleRate    int
	Channels      int
	BitsPerSample int
	TotalSamples  int
}

// Backend is the interface each subprocess decoder must implement.
//
// Implementations must tolerate Seek and Close while Read is blocked on the
// decoder, and Read must never wait forever once the decoder has stopped
// producing samples.
type Backend interface {
	// Name returns a human-readable identifier for the backend.
	Name() string
	// Open opens an APE file and prepares the stream. seekSamples < 0 means
	// start from the beginning.
	Open(path string, seekSamples int) (*StreamInfo, error)
	// Read fills p with interleaved float64 samples (L/R pairs). Returns the
	// number of frames written. It is called from a single goroutine and may
	// block until the decoder produces more samples.
	Read(p []float64) (int, error)
	// Seek seeks to the given sample position.
	Seek(samples int) error
	// Close releases all resources and kills any subprocess.
	Close() error
}

// Streamer (wraps any Backend, implements beep.StreamSeekCloser)

// Streamer implements beep.StreamSeekCloser for APE-encoded audio.
//
// opMu serialises Stream, Seek and Close: those three touch the shared decode
// buffer, while Position and Len read atomic counters and never block. beep
// already serialises them for playback (speaker.Clear waits for the in-flight
// Stream), but the streamer also owns a decoder subprocess and a reader
// goroutine, so it does not rely on the caller for its own consistency.
type Streamer struct {
	streamcore.Core
	backend Backend
	path    string
	opMu    sync.Mutex
}

// Decode opens an APE file and returns a beep.StreamSeekCloser. It tries each
// available backend in priority order: apecli (Rust binary), ffmpeg, mac.
func Decode(_ *os.File, path string) (*Streamer, beep.Format, error) {
	backends, problems := probeBackends()
	if len(backends) == 0 {
		return nil, beep.Format{}, noBackendError(problems)
	}

	var (
		streamer *Streamer
		info     *StreamInfo
		err      error
	)

	for _, b := range backends {
		logger.Debug("ape: trying backend", "name", b.Name())
		info, err = b.Open(path, -1)
		if err == nil {
			streamer = &Streamer{
				Core: streamcore.Core{
					TotalSamples: info.TotalSamples,
					NumChannels:  info.Channels,
					BufSamples:   0,
					Pos:          0,
				},
				backend: b,
				path:    path,
			}
			logger.Debug("ape: using backend", "name", b.Name())
			return streamer, beep.Format{
				SampleRate:  beep.SampleRate(info.SampleRate),
				NumChannels: info.Channels,
				Precision:   info.BitsPerSample / 8,
			}, nil
		}
		logger.Debug("ape: backend failed", "name", b.Name(), "err", err)
	}

	return nil, beep.Format{}, fmt.Errorf("ape: all backends failed: %w", err)
}

// noBackendError explains that no decoder is available. problems, when present,
// names decoders that were found but could not be used, so the hint does not
// send the user looking for something they already have.
func noBackendError(problems []string) error {
	const hint = "install ffmpeg or mac, or build apecli (requires cargo)"
	if len(problems) == 0 {
		return fmt.Errorf("ape: no decoder backend found — %s", hint)
	}
	return fmt.Errorf("ape: no decoder backend found (%s) — %s", strings.Join(problems, "; "), hint)
}

// sanitizeFileArg prefixes a user-supplied file path with "./" when it
// begins with "-", preventing it from being interpreted as a command-line
// flag by subprocess backends (ffmpeg, ffprobe, apecli, mac).
func sanitizeFileArg(path string) string {
	if strings.HasPrefix(path, "-") {
		return "./" + path
	}
	return path
}

// probeBackends discovers available backends in priority order. The returned
// problems explain any backend that was found but could not be used.
func probeBackends() ([]Backend, []string) {
	var (
		backends []Backend
		problems []string
	)

	// 1. apecli — built from tools/apecli/, searched in PATH and next to binary
	binary, apeProblems := findApeCLI()
	problems = append(problems, apeProblems...)
	if binary != "" {
		backends = append(backends, newApeCLIBackend(binary))
	}

	// 2. ffmpeg
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		backends = append(backends, newFFmpegBackend())
	}

	// 3. mac (official Monkey's Audio CLI)
	if _, err := exec.LookPath("mac"); err == nil {
		backends = append(backends, &macBackend{})
	}

	return backends, problems
}

// Streamer methods

// Stream fills the output buffer with decoded APE samples.
func (s *Streamer) Stream(samples [][2]float64) (int, bool) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	if s.Closed.Load() {
		return 0, false
	}

	totalNeeded := len(samples)
	totalFilled := 0

	for totalFilled < totalNeeded {
		if s.Pos < s.BufSamples {
			totalFilled += s.CopyToOutput(samples, totalNeeded, totalFilled)
			continue
		}

		// Buffer exhausted — read more from the backend
		s.Buf = s.bufPool(len(samples) * 2) // allocate enough for one read
		n, err := s.backend.Read(s.Buf)
		if n > 0 {
			s.BufSamples = n
			s.Pos = 0
			continue
		}
		if err != nil && err != io.EOF {
			return totalFilled, false
		}
		// Real EOF
		if totalFilled == 0 {
			return 0, false
		}
		return totalFilled, true
	}

	return totalFilled, true
}

// bufPool returns a []float64 of at most the given size, reusing a pooled
// buffer when possible.
func (s *Streamer) bufPool(n int) []float64 {
	const maxBufFrames = 4096
	if n > maxBufFrames*2 {
		n = maxBufFrames * 2
	}
	return make([]float64, n)
}

// Seek moves the stream position to the given sample.
func (s *Streamer) Seek(samples int) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	if s.Closed.Load() {
		return fmt.Errorf("apestream: streamer is closed")
	}
	if samples < 0 {
		samples = 0
	}
	if samples > s.TotalSamples {
		samples = s.TotalSamples
	}

	if err := s.backend.Seek(samples); err != nil {
		return fmt.Errorf("apestream seek: %w", err)
	}

	s.CurrentSample.Store(int64(samples))
	s.ResetBuffer()
	return nil
}

// Len returns the total number of samples in the stream.
func (s *Streamer) Len() int { return s.TotalSamples }

// Position returns the current sample position.
func (s *Streamer) Position() int { return int(s.CurrentSample.Load()) }

// Close closes the streamer and releases backend resources, killing any
// decoder subprocess and stopping its reader goroutine.
func (s *Streamer) Close() error {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	if s.Closed.Load() {
		return nil
	}
	s.Closed.Store(true)
	return s.backend.Close()
}
