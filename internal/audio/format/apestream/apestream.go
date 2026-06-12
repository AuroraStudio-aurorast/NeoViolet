// Package apestream provides a beep.StreamSeekCloser for APE (Monkey's Audio)
// format using one of several subprocess backends (apecli, ffmpeg, mac).
package apestream

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

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

// ---------------------------------------------------------------------------
// Streamer (wraps any Backend, implements beep.StreamSeekCloser)
// ---------------------------------------------------------------------------

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
		if _, err := os.Stat(env); err == nil {
			return env
		}
		return ""
	}

	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		candidate := filepath.Join(dir, "apecli")
		if runtime.GOOS == "windows" {
			candidate += ".exe"
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		// Also check tools/apecli/target/release/ relative to project root
		// (common during development)
		devCandidate := filepath.Join(dir, "tools", "apecli", "target", "release", "apecli")
		if runtime.GOOS == "windows" {
			devCandidate += ".exe"
		}
		if _, err := os.Stat(devCandidate); err == nil {
			return devCandidate
		}
	}

	if path, err := exec.LookPath("apecli"); err == nil {
		return path
	}

	return ""
}

// --- Streamer methods ---

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

// ---------------------------------------------------------------------------
// apecli pipe backend
// ---------------------------------------------------------------------------

type apeCLIBackend struct {
	binary string
	path   string // original .ape file path
	cmd    *exec.Cmd
	stdout io.ReadCloser
	mu     sync.Mutex
	// Decoded PCM data flows through this channel from a read goroutine.
	chunkChan chan pcmChunk
	// Internal buffer for unconsumed PCM between Read calls.
	readBuf []float64
	// Buffer conversion state.
	bytesPerSample int
	numChannels    int
	// Signalled when the read goroutine exits.
	done chan struct{}
	// Closed to signal the readLoop to stop (e.g. during Seek).
	cancel chan struct{}
}

type pcmChunk struct {
	data []float64
}

// 28-byte binary header from apecli.
type apeCLIHeader struct {
	Magic         [4]byte // "APEP"
	HeaderSize    uint32  // 28
	SampleRate    uint32
	Channels      uint16
	BitsPerSample uint16
	TotalSamples  uint64
	BlockAlign    uint16
	Reserved      uint16
}

func newApeCLIBackend(binary string) *apeCLIBackend {
	return &apeCLIBackend{
		binary: binary,
	}
}

func (b *apeCLIBackend) Name() string { return "apecli" }

func (b *apeCLIBackend) Open(path string, seekSamples int) (*StreamInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.path = path

	args := []string{path}
	if seekSamples >= 0 {
		args = append([]string{"--seek", fmt.Sprintf("%d", seekSamples)}, args...)
	}

	return b.startProcess(args)
}

func (b *apeCLIBackend) startProcess(args []string) (*StreamInfo, error) {
	cmd := exec.Command(b.binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("apecli stdout pipe: %w", err)
	}

	// Capture stderr for diagnostics.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("apecli stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("apecli start: %w", err)
	}

	b.cmd = cmd
	b.stdout = stdout

	// Read stderr in background for debugging.
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, stderr)
		if buf.Len() > 0 {
			logger.Debug("apecli stderr", "msg", buf.String())
		}
	}()

	// Read the 28-byte header with timeout.
	var hdr apeCLIHeader
	if err := readWithTimeout(stdout, &hdr, 30*time.Second); err != nil {
		b.kill()
		return nil, fmt.Errorf("apecli read header: %w", err)
	}

	if string(hdr.Magic[:]) != "APEP" {
		b.kill()
		return nil, fmt.Errorf("apecli: bad header magic %q", string(hdr.Magic[:]))
	}
	if hdr.SampleRate < 8000 || hdr.SampleRate > 384000 {
		b.kill()
		return nil, fmt.Errorf("apecli: invalid sample rate %d", hdr.SampleRate)
	}
	if hdr.Channels < 1 || hdr.Channels > 2 {
		b.kill()
		return nil, fmt.Errorf("apecli: unsupported channels %d", hdr.Channels)
	}
	switch hdr.BitsPerSample {
	case 8, 16, 24, 32:
	default:
		b.kill()
		return nil, fmt.Errorf("apecli: unsupported bits-per-sample %d", hdr.BitsPerSample)
	}

	b.bytesPerSample = int(hdr.BitsPerSample) / 8
	b.numChannels = int(hdr.Channels)

	// Create fresh channels and start the read goroutine. Channels are created
	// here (not at the top) so they only exist when a readLoop is active.
	b.chunkChan = make(chan pcmChunk, 4)
	b.done = make(chan struct{})
	b.cancel = make(chan struct{})
	go b.readLoop()

	return &StreamInfo{
		SampleRate:    int(hdr.SampleRate),
		Channels:      int(hdr.Channels),
		BitsPerSample: int(hdr.BitsPerSample),
		TotalSamples:  int(hdr.TotalSamples),
	}, nil
}

func (b *apeCLIBackend) readLoop() {
	defer close(b.done)

	bps := b.bytesPerSample
	ch := b.numChannels
	const bufFrames = 4096
	bufSize := bufFrames * bps * ch
	rawBuf := make([]byte, bufSize)

	for {
		n, err := io.ReadFull(b.stdout, rawBuf)
		if n > 0 {
			validFrames := n / (bps * ch)
			pcmBuf := make([]float64, bufFrames*2)
			pcmBuf = convertPCMToFloat64(rawBuf[:n], ch, bps, pcmBuf)
			select {
			case b.chunkChan <- pcmChunk{data: pcmBuf[:validFrames*2]}:
			case <-b.cancel:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (b *apeCLIBackend) Read(p []float64) (int, error) {
	total := 0
	needed := len(p)

	// 1. Consume from internal buffer first.
	if len(b.readBuf) > 0 {
		n := copy(p, b.readBuf)
		b.readBuf = b.readBuf[n:]
		total += n
	}

	// 2. Read more chunks until p is full or EOF.
	for total < needed {
		chunk, ok := <-b.chunkChan
		if !ok {
			if total == 0 {
				return 0, io.EOF
			}
			return total / 2, nil
		}
		n := copy(p[total:], chunk.data)
		total += n
		// Save unconsumed remainder for next Read call.
		if n < len(chunk.data) {
			excess := make([]float64, len(chunk.data)-n)
			copy(excess, chunk.data[n:])
			b.readBuf = excess
		}
	}
	return total / 2, nil
}

func (b *apeCLIBackend) Seek(samples int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.kill()

	// Drain any buffered PCM and wait for the old readLoop to fully exit
	// before starting a new one. This prevents the old goroutine from
	// sending on or closing the new channel.
	if b.chunkChan != nil {
		for {
			select {
			case <-b.chunkChan:
			case <-b.done:
				goto stopped
			}
		}
	}
stopped:
	if b.chunkChan != nil {
		close(b.chunkChan)
	}

	_, err := b.startProcess([]string{"--seek", fmt.Sprintf("%d", samples), b.path})
	return err
}

func (b *apeCLIBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.kill()
	// Wait for the readLoop to exit before closing chunkChan, so we don't
	// race with a concurrent send on a full channel.
	if b.chunkChan != nil {
		for {
			select {
			case <-b.chunkChan:
			case <-b.done:
				goto closeCh
			}
		}
	}
closeCh:
	if b.chunkChan != nil {
		close(b.chunkChan)
	}
	return nil
}

func (b *apeCLIBackend) kill() {
	b.readBuf = nil
	if b.cancel != nil {
		close(b.cancel)
	}
	if b.cmd != nil && b.cmd.Process != nil {
		b.cmd.Process.Kill()
		b.cmd.Wait()
	}
	if b.stdout != nil {
		b.stdout.Close()
	}
	b.cmd = nil
	b.stdout = nil
}

// ---------------------------------------------------------------------------
// ffmpeg pipe backend
// ---------------------------------------------------------------------------

type ffmpegBackend struct {
	path   string // original .ape file path
	cmd    *exec.Cmd
	stdout io.ReadCloser
	mu     sync.Mutex
	// Pre-computed metadata (obtained via ffprobe).
	info         *StreamInfo
	chunkChan    chan pcmChunk
	readBuf      []float64
	done         chan struct{}
	cancel       chan struct{} // closed to signal readLoop to stop
}

func (b *ffmpegBackend) Name() string { return "ffmpeg" }

func (b *ffmpegBackend) Open(path string, seekSamples int) (*StreamInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.path = path

	return b.startProcess(path, seekSamples)
}

// probeFFmpegMetadata runs ffprobe to extract audio properties of the APE file.
func probeFFmpegMetadata(path string) (*StreamInfo, error) {
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		path,
	}
	cmd := exec.Command("ffprobe", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}

	var result struct {
		Streams []struct {
			CodecType    string `json:"codec_type"`
			SampleRate   string `json:"sample_rate"`
			Channels     int    `json:"channels"`
			BitsPerSample int   `json:"bits_per_sample"`
			Duration     string `json:"duration"`
			NbSamples    int64  `json:"nb_samples"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("ffprobe parse: %w", err)
	}

	for _, s := range result.Streams {
		if s.CodecType == "audio" {
			var sampleRate int
			fmt.Sscanf(s.SampleRate, "%d", &sampleRate)

			totalSamples := int(s.NbSamples)
			if totalSamples == 0 && s.Duration != "" {
				var duration float64
				fmt.Sscanf(s.Duration, "%f", &duration)
				totalSamples = int(duration*float64(sampleRate) + 0.5)
			}

			bps := s.BitsPerSample
			if bps == 0 {
				bps = 16 // ffmpeg always outputs 16-bit PCM
			}

			return &StreamInfo{
				SampleRate:    sampleRate,
				Channels:      s.Channels,
				BitsPerSample: bps,
				TotalSamples:  totalSamples,
			}, nil
		}
	}

	return nil, fmt.Errorf("ffprobe: no audio stream found")
}

func (b *ffmpegBackend) startProcess(path string, seekSamples int) (*StreamInfo, error) {
	// Probe metadata first.
	info, err := probeFFmpegMetadata(path)
	if err != nil {
		return nil, fmt.Errorf("ffmpeg probe: %w", err)
	}
	b.info = info

	// Build ffmpeg arguments.
	// We request s16le PCM output (ffmpeg's most compatible format) at the
	// file's native sample rate and channel count.
	args := []string{
		"-i", path,
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ac", fmt.Sprintf("%d", info.Channels),
		"-ar", fmt.Sprintf("%d", info.SampleRate),
	}
	// Seek support.
	if seekSamples >= 0 {
		seconds := float64(seekSamples) / float64(info.SampleRate)
		args = append(args, "-ss", fmt.Sprintf("%.6f", seconds))
	}
	args = append(args, "pipe:1")

	cmd := exec.Command("ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg start: %w", err)
	}

	b.cmd = cmd
	b.stdout = stdout

	// Capture stderr for diagnostics (ffmpeg logs there).
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, stderr)
		if buf.Len() > 0 {
			logger.Debug("ffmpeg stderr", "msg", buf.String())
		}
	}()

	// Create fresh channels and start the read goroutine.
	b.chunkChan = make(chan pcmChunk, 4)
	b.done = make(chan struct{})
	b.cancel = make(chan struct{})
	go b.readLoop()

	return info, nil
}

func (b *ffmpegBackend) readLoop() {
	defer close(b.done)

	ch := b.info.Channels
	if ch == 0 {
		ch = 2
	}
	const bufFrames = 4096
	frameBytes := 2 * ch // s16le = 2 bytes per sample
	rawBuf := make([]byte, bufFrames*frameBytes)

	for {
		n, err := io.ReadFull(b.stdout, rawBuf)
		if n > 0 {
			validFrames := n / frameBytes
			pcmBuf := make([]float64, bufFrames*2)
			pcmBuf = convertPCMToFloat64(rawBuf[:n], ch, 2, pcmBuf)
			select {
			case b.chunkChan <- pcmChunk{data: pcmBuf[:validFrames*2]}:
			case <-b.cancel:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (b *ffmpegBackend) Read(p []float64) (int, error) {
	total := 0
	needed := len(p)

	// 1. Consume from internal buffer first.
	if len(b.readBuf) > 0 {
		n := copy(p, b.readBuf)
		b.readBuf = b.readBuf[n:]
		total += n
	}

	// 2. Read more chunks until p is full or EOF.
	for total < needed {
		chunk, ok := <-b.chunkChan
		if !ok {
			if total == 0 {
				return 0, io.EOF
			}
			return total / 2, nil
		}
		n := copy(p[total:], chunk.data)
		total += n
		// Save unconsumed remainder for next Read call.
		if n < len(chunk.data) {
			excess := make([]float64, len(chunk.data)-n)
			copy(excess, chunk.data[n:])
			b.readBuf = excess
		}
	}
	return total / 2, nil
}

func (b *ffmpegBackend) Seek(samples int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.kill()

	// Drain any buffered PCM and wait for the old readLoop to fully exit.
	if b.chunkChan != nil {
		for {
			select {
			case <-b.chunkChan:
			case <-b.done:
				goto stoppedFfmpeg
			}
		}
	}
stoppedFfmpeg:
	if b.chunkChan != nil {
		close(b.chunkChan)
	}

	_, err := b.startProcess(b.path, samples)
	return err
}

func (b *ffmpegBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.kill()
	if b.chunkChan != nil {
		for {
			select {
			case <-b.chunkChan:
			case <-b.done:
				goto closeChFfmpeg
			}
		}
	}
closeChFfmpeg:
	if b.chunkChan != nil {
		close(b.chunkChan)
	}
	return nil
}

func (b *ffmpegBackend) kill() {
	b.readBuf = nil
	if b.cancel != nil {
		close(b.cancel)
	}
	if b.cmd != nil && b.cmd.Process != nil {
		b.cmd.Process.Kill()
		b.cmd.Wait()
	}
	if b.stdout != nil {
		b.stdout.Close()
	}
	b.cmd = nil
	b.stdout = nil
}

// ---------------------------------------------------------------------------
// mac (Monkey's Audio Console) backend — uses temp file
// ---------------------------------------------------------------------------

type macBackend struct {
	cmd     *exec.Cmd
	path    string // original .ape file path
	tempDir string
	tempWAV string
	pcm     []float64
	pos     int // current frame position in pcm
	info    *StreamInfo
	mu      sync.Mutex
}

func (b *macBackend) Name() string { return "mac" }

func (b *macBackend) Open(path string, seekSamples int) (*StreamInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.path = path

	return b.decodeToMemory(path, seekSamples)
}

func (b *macBackend) decodeToMemory(path string, seekSamples int) (*StreamInfo, error) {
	tempDir, err := os.MkdirTemp("", "neoviolet-mac-*")
	if err != nil {
		return nil, fmt.Errorf("mac temp dir: %w", err)
	}
	b.tempDir = tempDir

	tempWAV := filepath.Join(tempDir, "output.wav")

	// Decode APE → WAV using the official mac tool.
	cmd := exec.Command("mac", path, tempWAV, "-d")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("mac stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("mac start: %w", err)
	}

	// Capture stderr.
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, stderr)
		if buf.Len() > 0 {
			logger.Debug("mac stderr", "msg", buf.String())
		}
	}()

	if err := cmd.Wait(); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("mac decode: %w", err)
	}

	b.cmd = cmd
	b.tempWAV = tempWAV

	// Read the WAV file back.
	wavFile, err := os.Open(tempWAV)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("mac read wav: %w", err)
	}
	defer wavFile.Close()

	// Parse WAV header (44 bytes for standard PCM WAV).
	var header [44]byte
	if _, err := io.ReadFull(wavFile, header[:]); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("mac wav header: %w", err)
	}

	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("mac: invalid WAV file")
	}

	channels := int(binary.LittleEndian.Uint16(header[22:24]))
	sampleRate := int(binary.LittleEndian.Uint32(header[24:28]))
	bitsPerSample := int(binary.LittleEndian.Uint16(header[34:36]))
	dataSize := int(binary.LittleEndian.Uint32(header[40:44]))
	bytesPerSample := bitsPerSample / 8
	blockAlign := bytesPerSample * channels
	totalSamples := dataSize / blockAlign

	if channels < 1 || channels > 2 || sampleRate < 8000 || sampleRate > 384000 {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("mac: unexpected WAV params ch=%d sr=%d", channels, sampleRate)
	}

	b.info = &StreamInfo{
		SampleRate:    sampleRate,
		Channels:      channels,
		BitsPerSample: bitsPerSample,
		TotalSamples:  totalSamples,
	}

	// Read PCM data into memory.
	pcmRaw := make([]byte, dataSize)
	if _, err := io.ReadFull(wavFile, pcmRaw); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("mac read pcm: %w", err)
	}

	// Convert to float64.
	pcmBuf := make([]float64, totalSamples*2)
	b.pcm = convertPCMToFloat64(pcmRaw, channels, bytesPerSample, pcmBuf)

	// Handle seek: skip frames.
	if seekSamples > 0 {
		skipFrames := seekSamples
		if skipFrames > totalSamples {
			skipFrames = totalSamples
		}
		b.pcm = b.pcm[skipFrames*2:]
		// Adjust the total samples reported.
		b.info.TotalSamples = totalSamples - skipFrames
	}

	return b.info, nil
}

func (b *macBackend) Read(p []float64) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.pos >= len(b.pcm)/2 {
		return 0, io.EOF
	}

	available := len(b.pcm)/2 - b.pos
	framesNeeded := len(p) / 2
	if framesNeeded > available {
		framesNeeded = available
	}

	copy(p, b.pcm[b.pos*2:])
	b.pos += framesNeeded
	return framesNeeded, nil
}

func (b *macBackend) Seek(samples int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.cleanup(); err != nil {
		logger.Debug("mac cleanup during seek", "err", err)
	}

	_, err := b.decodeToMemory(b.path, samples)
	return err
}

func (b *macBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cleanup()
}

func (b *macBackend) cleanup() error {
	if b.cmd != nil && b.cmd.Process != nil {
		b.cmd.Process.Kill()
	}
	if b.tempDir != "" {
		return os.RemoveAll(b.tempDir)
	}
	return nil
}

// ---------------------------------------------------------------------------
// PCM conversion helpers
// ---------------------------------------------------------------------------

// convertPCMToFloat64 converts raw PCM bytes to float64 samples and stores
// them in the provided buffer (which must be large enough). Supports 8, 16,
// 24, and 32-bit little-endian PCM. Mono input is duplicated to stereo.
// The returned slice is the input buffer[:frames*2] for caller convenience.
func convertPCMToFloat64(pcm []byte, numChannels, bytesPerSample int, out []float64) []float64 {
	frameSize := bytesPerSample * numChannels
	if frameSize == 0 {
		return out
	}
	numFrames := len(pcm) / frameSize
	if numFrames*2 > len(out) {
		numFrames = len(out) / 2
	}
	out = out[:numFrames*2]

	for i := 0; i < numFrames; i++ {
		for ch := 0; ch < numChannels && ch < 2; ch++ {
			sampleStart := i*frameSize + ch*bytesPerSample
			var sample int32
			switch bytesPerSample {
			case 1:
				// unsigned 8-bit → signed (-128 to 127)
				sample = int32(pcm[sampleStart]) - 128
			case 2:
				sample = int32(int16(binary.LittleEndian.Uint16(pcm[sampleStart:])))
			case 3:
				sample = int32(pcm[sampleStart]) |
					int32(pcm[sampleStart+1])<<8 |
					int32(pcm[sampleStart+2])<<16
				if sample&0x800000 != 0 {
					sample |= ^0xffffff // sign extend
				}
			case 4:
				sample = int32(binary.LittleEndian.Uint32(pcm[sampleStart:]))
			}
			out[i*2+ch] = float64(sample) / float64(uint32(1)<<(bytesPerSample*8-1))
		}
		if numChannels == 1 {
			out[i*2+1] = out[i*2]
		}
	}

	return out
}

// readWithTimeout reads a binary value from r with a timeout.
func readWithTimeout(r io.Reader, v interface{}, timeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- binary.Read(r, binary.LittleEndian, v)
	}()
	select {
	case err := <-errCh:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("read timed out after %v", timeout)
	}
}