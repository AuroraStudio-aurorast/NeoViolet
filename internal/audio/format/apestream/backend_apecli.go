package apestream

import (
	"fmt"
	"os/exec"
	"strconv"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// apecli pipe backend

// apeCLIBackend decodes APE through the apecli helper (tools/apecli), which
// streams APEP-framed PCM on stdout. Process lifetime is handled by pipeOwner.
type apeCLIBackend struct {
	pipeOwner
	binary string
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
	b := &apeCLIBackend{binary: binary}
	b.start = b.startPipe
	return b
}

func (b *apeCLIBackend) Name() string { return "apecli" }

func (b *apeCLIBackend) Open(path string, seekSamples int) (*StreamInfo, error) {
	return b.open(path, seekSamples)
}

func (b *apeCLIBackend) Read(p []float64) (int, error) { return b.read(p) }

func (b *apeCLIBackend) Seek(samples int) error { return b.seek(samples) }

func (b *apeCLIBackend) Close() error { return b.close() }

// startPipe launches apecli for path, validates its APEP header and hands the
// still unread PCM stream to the reader goroutine.
func (b *apeCLIBackend) startPipe(path string, seekSamples int) (*pipeStream, *StreamInfo, error) {
	args := make([]string, 0, 3)
	if seekSamples >= 0 {
		args = append(args, "--seek", strconv.Itoa(seekSamples))
	}
	args = append(args, sanitizeFileArg(path))

	// #nosec G204 -- binary path is resolved from a trusted location (env var,
	// executable dir, or PATH) and args are a sanitized file path + seek offset.
	cmd := exec.Command(b.binary, args...)
	stdout, err := startPipeCmd("apecli", cmd)
	if err != nil {
		return nil, nil, err
	}

	// Read the 28-byte header before the PCM reader starts, so the header is
	// never mistaken for samples.
	var hdr apeCLIHeader
	if err := readWithTimeout(stdout, &hdr, pipeHeaderTimeout); err != nil {
		stopProcess(cmd, stdout)
		return nil, nil, fmt.Errorf("apecli read header: %w", err)
	}

	if err := hdr.validate(); err != nil {
		stopProcess(cmd, stdout)
		return nil, nil, err
	}

	info := &StreamInfo{
		SampleRate:    int(hdr.SampleRate),
		Channels:      int(hdr.Channels),
		BitsPerSample: int(hdr.BitsPerSample),
		// #nosec G115 -- sample count fits in int for any real-world track.
		TotalSamples: int(hdr.TotalSamples),
	}
	s := newPipeStream(cmd, stdout, info, int(hdr.BitsPerSample)/8, int(hdr.Channels))

	logger.Debug("ape: apecli stream ready", "sampleRate", info.SampleRate, "channels", info.Channels)
	return s, info, nil
}

// validate rejects headers that would produce a nonsense stream. The values come
// from a subprocess, so they are treated as untrusted input.
func (h apeCLIHeader) validate() error {
	if string(h.Magic[:]) != "APEP" {
		return fmt.Errorf("apecli: bad header magic %q", string(h.Magic[:]))
	}
	if h.SampleRate < 8000 || h.SampleRate > 384000 {
		return fmt.Errorf("apecli: invalid sample rate %d", h.SampleRate)
	}
	if h.Channels < 1 || h.Channels > 2 {
		return fmt.Errorf("apecli: unsupported channels %d", h.Channels)
	}
	switch h.BitsPerSample {
	case 8, 16, 24, 32:
	default:
		return fmt.Errorf("apecli: unsupported bits-per-sample %d", h.BitsPerSample)
	}
	return nil
}
