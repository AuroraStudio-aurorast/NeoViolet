package apestream

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// ffmpeg pipe backend

type ffmpegBackend struct {
	path   string // original .ape file path
	cmd    *exec.Cmd
	stdout io.ReadCloser
	mu     sync.Mutex
	// Pre-computed metadata (obtained via ffprobe).
	info      *StreamInfo
	chunkChan chan pcmChunk
	readBuf   []float64
	done      chan struct{}
	cancel    chan struct{} // closed to signal readLoop to stop
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
		"--",
		path,
	}
	// #nosec G204 -- ffprobe is a fixed binary name; args are a fixed flag set
	// plus the user's own audio path (with a "--" separator).
	cmd := exec.Command("ffprobe", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}

	var result struct {
		Streams []struct {
			CodecType     string `json:"codec_type"`
			SampleRate    string `json:"sample_rate"`
			Channels      int    `json:"channels"`
			BitsPerSample int    `json:"bits_per_sample"`
			Duration      string `json:"duration"`
			NbSamples     int64  `json:"nb_samples"`
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
		"-i", sanitizeFileArg(path),
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
