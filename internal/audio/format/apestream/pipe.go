package apestream

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// Pipe backend plumbing shared by the apecli and ffmpeg backends.
//
// Both decode APE through a subprocess that writes raw PCM to stdout: a reader
// goroutine turns that into chunks, and Read drains them. Keeping that machinery
// in one place keeps the two backends from drifting apart in exactly the code
// where lifetime bugs live.
//
// Lifetime rules:
//   - A pipeStream owns one subprocess generation. Only its readLoop closes
//     chunkChan, so Read can never wait for a sender that has already left, and
//     nobody closes a channel another goroutine may still send on.
//   - abort() kills the process and waits for the reader to reap it.
//   - Read gives the decoder a bounded amount of time to produce each chunk: a
//     decoder that stalls must not block its caller (and, through beep's
//     speaker lock, the whole player) forever.
//   - A backend keeps the current generation behind a mutex and hands Read a
//     snapshot, so Read, Seek and Close never share mutable state.

const (
	// pipeBufFrames is the number of frames read from the decoder per syscall.
	pipeBufFrames = 4096
	// pipeChunkBuffer is how many decoded chunks may wait for Read.
	pipeChunkBuffer = 4
	// pipeHeaderTimeout bounds the metadata handshake at startup.
	pipeHeaderTimeout = 30 * time.Second
	// pipeAbortTimeout bounds how long abort waits for the reader goroutine.
	pipeAbortTimeout = 2 * time.Second
	// pipeReadTimeout bounds how long one Read waits for the decoder to produce
	// its next chunk. A healthy decoder delivers a chunk well under a
	// millisecond after the pipe has data, so a gap this long means the process
	// is wedged (deadlock, stuck filesystem, killed without closing the pipe).
	// Waiting forever instead would freeze playback and every later
	// output detach, so the generation is aborted and playback reports
	// a decoder error.
	pipeReadTimeout = 15 * time.Second
)

// pcmChunk is a block of decoded interleaved stereo float64 samples.
type pcmChunk struct {
	data []float64
}

// pipeStream is one decoder subprocess generation: the process, its PCM pipe and
// the reader goroutine draining it.
type pipeStream struct {
	cmd            *exec.Cmd
	stdout         io.ReadCloser
	info           *StreamInfo
	bytesPerSample int
	numChannels    int

	chunkChan   chan pcmChunk // closed by readLoop when it stops (sole closer)
	cancel      chan struct{} // closed by abort to stop the reader early
	done        chan struct{} // closed after the decoder is reaped
	terminalErr error         // published by closing chunkChan

	// readBuf holds decoded samples not yet handed to the caller. Only the
	// single Read caller touches it.
	readBuf []float64

	// readTimeout bounds how long Read waits for the next chunk; tests shorten
	// it. See pipeReadTimeout.
	readTimeout time.Duration

	abortOnce sync.Once
}

// newPipeStream starts the reader goroutine for an already running subprocess.
func newPipeStream(cmd *exec.Cmd, stdout io.ReadCloser, info *StreamInfo, bytesPerSample, numChannels int) *pipeStream {
	s := &pipeStream{
		cmd:            cmd,
		stdout:         stdout,
		info:           info,
		bytesPerSample: bytesPerSample,
		numChannels:    numChannels,
		chunkChan:      make(chan pcmChunk, pipeChunkBuffer),
		cancel:         make(chan struct{}),
		done:           make(chan struct{}),
		readTimeout:    pipeReadTimeout,
	}
	go s.readLoop()
	return s
}

// readLoop drains stdout before reaping the process. Readers see the terminal
// error only after all queued samples have been handed off.
func (s *pipeStream) readLoop() {
	defer func() {
		if s.cmd != nil {
			if err := s.cmd.Wait(); s.terminalErr == nil && err != nil {
				s.terminalErr = fmt.Errorf("APE decoder exited: %w", err)
			}
		}
		close(s.chunkChan)
		close(s.done)
	}()

	frameBytes := s.bytesPerSample * s.numChannels
	if frameBytes == 0 {
		return
	}
	rawBuf := make([]byte, pipeBufFrames*frameBytes)

	for {
		n, err := io.ReadFull(s.stdout, rawBuf)
		if n > 0 {
			validFrames := n / frameBytes
			pcmBuf := make([]float64, pipeBufFrames*2)
			pcmBuf = convertPCMToFloat64(rawBuf[:n], s.numChannels, s.bytesPerSample, pcmBuf)
			select {
			case s.chunkChan <- pcmChunk{data: pcmBuf[:validFrames*2]}:
			case <-s.cancel:
				return
			}
		}
		if err != nil {
			if n%frameBytes != 0 || (!errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF)) {
				s.terminalErr = fmt.Errorf("APE decoder PCM: %w", err)
			}
			return
		}
	}
}

// read fills p with sample floats from this generation, returning io.EOF once
// the decoder has ended. It is called from a single goroutine at a time and may
// block until the decoder produces more samples; abort and the read timeout are
// what release it.
func (s *pipeStream) read(p []float64) (int, error) {
	total := 0
	needed := len(p)

	if len(s.readBuf) > 0 {
		n := copy(p, s.readBuf)
		s.readBuf = s.readBuf[n:]
		total += n
	}

	if total < needed {
		timer := time.NewTimer(s.readTimeout)
		defer timer.Stop()

	drain:
		for total < needed {
			select {
			case chunk, ok := <-s.chunkChan:
				if !ok {
					if s.terminalErr != nil {
						return total / 2, s.terminalErr
					}
					break drain
				}
				n := copy(p[total:], chunk.data)
				total += n
				if n < len(chunk.data) {
					// Keep the tail for the next call, reusing the consumed buffer.
					s.readBuf = append(s.readBuf[:0], chunk.data[n:]...)
				}
			case <-timer.C:
				logger.Warn("ape: decoder stopped producing samples, aborting", "timeout", s.readTimeout)
				s.abort()
				return total / 2, fmt.Errorf("APE decoder timed out waiting for samples after %s", s.readTimeout)
			}
		}
	}

	if total == 0 {
		return 0, io.EOF
	}
	return total / 2, nil
}

// abort stops the subprocess and its reader goroutine. It is safe to call more
// than once, from any goroutine, and it is what releases a Read blocked on this
// generation.
func (s *pipeStream) abort() {
	s.abortOnce.Do(func() {
		close(s.cancel)

		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		if s.stdout != nil {
			_ = s.stdout.Close()
		}

		// The reader reaps the process after it stops using stdout.
		select {
		case <-s.done:
		case <-time.After(pipeAbortTimeout):
			logger.Debug("ape: decoder pipe reader did not stop in time")
		}
	})
}

// startPipeCmd wires up stdout/stderr, starts the subprocess and drains stderr
// into the logger. The returned reader carries the raw PCM stream.
func startPipeCmd(name string, cmd *exec.Cmd) (io.ReadCloser, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("%s stdout pipe: %w", name, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("%s stderr pipe: %w", name, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s start: %w", name, err)
	}

	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, stderr)
		if buf.Len() > 0 {
			logger.Debug("ape: decoder stderr", "backend", name, "msg", buf.String())
		}
	}()

	return stdout, nil
}

// stopProcess cleans up a subprocess whose startup handshake failed.
func stopProcess(cmd *exec.Cmd, stdout io.ReadCloser) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if stdout != nil {
		_ = stdout.Close()
	}
	_ = cmd.Wait()
}

// pipeStarter builds a fresh generation for a file, starting at seekSamples
// decoded frames (negative means from the beginning).
type pipeStarter func(path string, seekSamples int) (*pipeStream, *StreamInfo, error)

// pipeOwner is the shared state machine of a subprocess-backed Backend: it owns
// the current generation and serialises Open/Seek/Close against each other,
// while Read works on the generation it snapshotted.
type pipeOwner struct {
	mu      sync.Mutex
	path    string
	session *pipeStream
	start   pipeStarter
}

func (o *pipeOwner) open(path string, seekSamples int) (*StreamInfo, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.path = path
	s, info, err := o.start(path, seekSamples)
	if err != nil {
		return nil, err
	}
	o.session = s
	return info, nil
}

func (o *pipeOwner) read(p []float64) (int, error) {
	o.mu.Lock()
	s := o.session
	o.mu.Unlock()

	if s == nil {
		return 0, io.EOF
	}
	return s.read(p)
}

func (o *pipeOwner) seek(samples int) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.stopLocked()
	s, _, err := o.start(o.path, samples)
	if err != nil {
		return err
	}
	o.session = s
	return nil
}

func (o *pipeOwner) close() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.stopLocked()
	return nil
}

// stopLocked ends the current generation. The caller must hold o.mu.
func (o *pipeOwner) stopLocked() {
	if o.session != nil {
		o.session.abort()
		o.session = nil
	}
}
