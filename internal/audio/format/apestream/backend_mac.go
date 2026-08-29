package apestream

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// mac (Monkey's Audio Console) backend — uses temp file

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
