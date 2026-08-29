package apestream

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// apecli pipe backend

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

	args := []string{sanitizeFileArg(path)}
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

	_, err := b.startProcess([]string{"--seek", fmt.Sprintf("%d", samples), sanitizeFileArg(b.path)})
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
