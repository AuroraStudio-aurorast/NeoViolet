package synth

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/gopxl/beep/v2"
)

// The player calls libopenmpt through the entry points openmpt_lib.go resolves
// at run time, so the library stays an optional dependency: without it the
// probe answers false, the extension list is empty and NewOpenmptPlayer reports
// an error, leaving the caller to fall back to the gotracker backend.

const openmptBlockSize = 2048

// OpenmptProbe checks whether libopenmpt can handle a file by probing its header
// bytes. It returns false when libopenmpt is unavailable.
func OpenmptProbe(data []byte) bool {
	api := ensureOpenmptLib()
	if api == nil || len(data) == 0 {
		return false
	}

	bufSize := len(data)
	if recSize := int(api.probeRecommendedSize()); bufSize > recSize {
		bufSize = recSize
	}
	result := api.probeFileHeader(
		openmptProbeFlagsDefault,
		unsafe.Pointer(&data[0]), uintptr(bufSize), uint64(len(data)), //nolint:gosec // the library reads the caller's bytes for the duration of the call.
		api.logFuncSilent, nil,
		api.errorFuncIgnore, nil,
		nil, nil,
	)
	runtime.KeepAlive(data)
	return result == openmptProbeResultSuccess
}

// openmptFormats holds the extension list, which is constant for a loaded
// library but is asked for on every file open by isSyntheticFormat.
var openmptFormats struct {
	mu   sync.Mutex
	list []string
}

// OpenmptSupportedFormats returns the file extensions libopenmpt supports,
// without the leading dot. It returns nil when libopenmpt is unavailable.
func OpenmptSupportedFormats() []string {
	api := ensureOpenmptLib()
	if api == nil {
		return nil
	}

	openmptFormats.mu.Lock()
	defer openmptFormats.mu.Unlock()

	if openmptFormats.list == nil {
		openmptFormats.list = queryOpenmptFormats(api)
	}
	// Callers keep the slice, so hand out a copy of the cached one.
	return append([]string(nil), openmptFormats.list...)
}

// queryOpenmptFormats asks the library for its extension list.
func queryOpenmptFormats(api *openmptAPI) []string {
	exts := api.getSupportedExtensions()
	if exts == nil {
		return nil
	}
	s := openmptCString(exts)
	api.freeString(exts)
	return strings.Split(s, ";")
}

// OpenmptPlayer plays a tracker module through libopenmpt. mod is the library's
// module handle and nil once the player is closed, which is what makes seeking
// after a close report an error instead of handing a freed handle back.
type OpenmptPlayer struct {
	baseSynth
	mod unsafe.Pointer
	api *openmptAPI

	renderSamples [][2]float64
	renderPos     int
}

// NewOpenmptPlayer loads a tracker module from disk and creates a player. It
// reports an error when libopenmpt is unavailable or when the library cannot
// read the file.
func NewOpenmptPlayer(path string, sampleRate beep.SampleRate) (*OpenmptPlayer, error) {
	api := ensureOpenmptLib()
	if api == nil {
		return nil, errors.New("openmpt: libopenmpt is not available")
	}

	// #nosec G304 -- path is the user's own tracker module being loaded.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tracker file: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("openmpt load module: %s is empty", path)
	}

	var cErr int32
	var cMsg unsafe.Pointer
	mod := api.moduleCreateFromMemory2(
		unsafe.Pointer(&data[0]), uintptr(len(data)), //nolint:gosec // the library reads the caller's bytes for the duration of the call.
		api.logFuncSilent, nil,
		api.errorFuncIgnore, nil,
		&cErr, &cMsg, nil,
	)
	runtime.KeepAlive(data)
	if mod == nil {
		msg := ""
		if cMsg != nil {
			msg = openmptCString(cMsg)
			api.freeString(cMsg)
		}
		return nil, fmt.Errorf("openmpt load module: error %d: %s", cErr, msg)
	}

	api.moduleSetRepeatCount(mod, 0)

	p := &OpenmptPlayer{mod: mod, api: api}
	p.sampleRate = sampleRate
	p.duration = time.Duration(api.moduleGetDurationSeconds(mod) * float64(time.Second))
	p.isPaused = true
	p.volumeScale = 1.0
	p.title = openmptMetadata(api, mod, "title")
	p.artist = openmptMetadata(api, mod, "artist")

	return p, nil
}

// openmptMetadata reads one metadata field of a module, for example "title".
// The key crosses the ABI as a NUL terminated string, and the value is copied
// out of the library's memory before that memory is freed.
func openmptMetadata(api *openmptAPI, mod unsafe.Pointer, key string) string {
	ck := append([]byte(key), 0)
	val := api.moduleGetMetadata(mod, unsafe.Pointer(&ck[0])) //nolint:gosec // the key is NUL terminated here and outlives the call.
	runtime.KeepAlive(ck)
	if val == nil {
		return ""
	}
	s := openmptCString(val)
	api.freeString(val)
	return s
}

// openmptCString copies a NUL terminated string the library owns. The copy has
// to be taken before the matching openmpt_free_string.
func openmptCString(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	var out []byte
	for i := 0; ; i++ {
		c := *(*byte)(unsafe.Add(p, i))
		if c == 0 {
			break
		}
		out = append(out, c)
	}
	return string(out)
}

// Stream implements beep.Streamer.
func (p *OpenmptPlayer) Stream(samples [][2]float64) (n int, ok bool) {
	// The whole call stays under the lock: Close frees the module handle that
	// fillSamples renders from and Stop rewinds the state it writes, so the
	// three have to be mutually exclusive. One call renders its own sample
	// block and nothing else, so a concurrent Close or Position waits for that
	// long.
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed || p.finished {
		return 0, false
	}
	if p.isPaused {
		for i := range samples {
			samples[i] = [2]float64{}
		}
		return len(samples), true
	}

	p.fillSamples(samples, p.volumeScale, &p.elapsed, p.duration)

	return len(samples), true
}

// fillSamples renders module audio into samples and advances elapsed. The
// caller must hold p.mu: this reads p.mod, which Close frees, and writes
// p.renderSamples, p.renderPos and p.finished, which Stop also writes under the
// same lock.
func (p *OpenmptPlayer) fillSamples(samples [][2]float64, vs float64, elapsed *time.Duration, duration time.Duration) {
	buf := make([]int16, openmptBlockSize*2)
	sampleDur := time.Second / time.Duration(p.sampleRate)

	for i := range samples {
		if *elapsed >= duration {
			p.finished = true
			p.isPlaying = false
			for j := i; j < len(samples); j++ {
				samples[j] = [2]float64{}
			}
			return
		}

		if p.renderPos >= len(p.renderSamples) {
			count := p.api.moduleReadInterleavedStereo(
				p.mod, int32(p.sampleRate), openmptBlockSize, //nolint:gosec // the library takes the rate as int32; a rate beyond its range is not a real rate.
				unsafe.Pointer(&buf[0]), //nolint:gosec // the library fills the buffer for the duration of the call.
			)
			runtime.KeepAlive(buf)
			if count == 0 {
				p.finished = true
				p.isPlaying = false
				for j := i; j < len(samples); j++ {
					samples[j] = [2]float64{}
				}
				return
			}
			p.renderSamples = make([][2]float64, int(count))
			for j := 0; j < int(count); j++ {
				p.renderSamples[j][0] = float64(buf[j*2]) / 32768.0
				p.renderSamples[j][1] = float64(buf[j*2+1]) / 32768.0
			}
			p.renderPos = 0
		}

		s := p.renderSamples[p.renderPos]
		p.renderPos++

		samples[i][0] = softClip(s[0] * vs)
		samples[i][1] = softClip(s[1] * vs)
		*elapsed += sampleDur
	}
}

// Err always returns nil: a module is either loaded by the constructor or not
// loaded at all.
func (p *OpenmptPlayer) Err() error { return nil }

// Stop rewinds the player and drops the rendered buffer.
func (p *OpenmptPlayer) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.baseStop()
	p.renderSamples = nil
	p.renderPos = 0
}

// Close releases the module handle. It is safe to call more than once.
func (p *OpenmptPlayer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.baseClose()
	if p.mod != nil {
		p.api.moduleDestroy(p.mod)
		p.mod = nil
	}
	return nil
}

// Position reports how far playback has advanced.
func (p *OpenmptPlayer) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.elapsed
}

// Duration reports the length of the module.
func (p *OpenmptPlayer) Duration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.duration
}

// Seek moves playback to pos. It fails once the player has been closed.
func (p *OpenmptPlayer) Seek(pos time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.mod == nil {
		return errors.New("openmpt player is closed")
	}

	p.api.moduleSetPositionSeconds(p.mod, pos.Seconds())
	p.elapsed = pos
	p.finished = false
	p.renderSamples = nil
	p.renderPos = 0
	return nil
}

// Streamer returns the player itself.
func (p *OpenmptPlayer) Streamer() Streamer { return p }

// Open is unsupported: an openmpt player is bound to the module it was created
// from.
func (p *OpenmptPlayer) Open(_ string) error {
	return fmt.Errorf("openmpt player does not support Open, use NewOpenmptPlayer")
}
