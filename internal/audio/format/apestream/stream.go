package apestream

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
type Backend interface {
	// Name returns a human-readable identifier for the backend.
	Name() string
	// Open opens an APE file and prepares the stream. seekSamples < 0 means
	// start from the beginning.
	Open(path string, seekSamples int) (*StreamInfo, error)
	// Read fills p with interleaved float64 samples (L/R pairs). Returns the
	// number of frames written.
	Read(p []float64) (int, error)
	// Seek seeks to the given sample position.
	Seek(samples int) error
	// Close releases all resources and kills any subprocess.
	Close() error
}

// Streamer (wraps any Backend, implements beep.StreamSeekCloser)

// Streamer implements beep.StreamSeekCloser for APE-encoded audio.
type Streamer struct {
	streamcore.Core
	backend Backend
	path    string
}

// Decode opens an APE file and returns a beep.StreamSeekCloser. It tries each
// available backend in priority order: apecli (Rust binary), ffmpeg, mac.
func Decode(file *os.File, path string) (*Streamer, beep.Format, error) {
	backends := probeBackends()
	if len(backends) == 0 {
		return nil, beep.Format{}, fmt.Errorf(
			"ape: no decoder backend found — install ffmpeg or mac, or build apecli (requires cargo)",
		)
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

// sanitizeFileArg prefixes a user-supplied file path with "./" when it
// begins with "-", preventing it from being interpreted as a command-line
// flag by subprocess backends (ffmpeg, ffprobe, apecli, mac).
func sanitizeFileArg(path string) string {
	if strings.HasPrefix(path, "-") {
		return "./" + path
	}
	return path
}

// probeBackends discovers available backends in priority order.
func probeBackends() []Backend {
	var backends []Backend

	// 1. apecli — built from tools/apecli/, searched in PATH and next to binary
	if binary := findApeCLI(); binary != "" {
		backends = append(backends, newApeCLIBackend(binary))
	}

	// 2. ffmpeg
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		backends = append(backends, &ffmpegBackend{})
	}

	// 3. mac (official Monkey's Audio CLI)
	if _, err := exec.LookPath("mac"); err == nil {
		backends = append(backends, &macBackend{})
	}

	return backends
}

// findApeCLI locates the apecli helper binary. Search order:
//  1. NEOVIOLET_APECLI env var
//  2. Next to the running executable
//  3. System PATH
func findApeCLI() string {
	if env := os.Getenv("NEOVIOLET_APECLI"); env != "" {
		// #nosec G703 -- env is a user-configured path to the apecli helper
		// binary; checking its existence is the documented config mechanism.
		if _, err := os.Stat(env); err == nil {
			return env
		}
		return ""
	}

	// Try next to executable, then dev build dir, then system PATH.
	// exec.LookPath handles platform-specific binary extensions (.exe on Windows).
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		for _, c := range []string{
			filepath.Join(dir, "apecli"),
			filepath.Join(dir, "tools", "apecli", "target", "release", "apecli"),
		} {
			if path, err := exec.LookPath(c); err == nil {
				return path
			}
		}
	}

	if path, err := exec.LookPath("apecli"); err == nil {
		return path
	}

	return ""
}

// Streamer methods

func (s *Streamer) Stream(samples [][2]float64) (int, bool) {
	if s.Closed {
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

func (s *Streamer) Seek(samples int) error {
	if s.Closed {
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

	s.CurrentSample = samples
	s.ResetBuffer()
	return nil
}

func (s *Streamer) Len() int { return s.TotalSamples }

func (s *Streamer) Position() int { return s.CurrentSample }

// Close closes the streamer and releases backend resources.
func (s *Streamer) Close() error {
	if s.Closed {
		return nil
	}
	s.Closed = true
	return s.backend.Close()
}
