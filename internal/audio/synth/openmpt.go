//go:build openmpt

package synth

/*
#cgo pkg-config: libopenmpt
#include <libopenmpt/libopenmpt.h>
#include <stdlib.h>

static openmpt_module* create_module(const void* data, size_t size, int* out_err, const char** out_msg) {
	return openmpt_module_create_from_memory2(
		data, size,
		openmpt_log_func_silent, NULL,
		openmpt_error_func_ignore, NULL,
		out_err, out_msg, NULL
	);
}

static int probe_file_header(const void* data, size_t size, uint64_t filesize) {
	return openmpt_probe_file_header(
		OPENMPT_PROBE_FILE_HEADER_FLAGS_DEFAULT,
		data, size, filesize,
		openmpt_log_func_silent, NULL,
		openmpt_error_func_ignore, NULL,
		NULL, NULL
	);
}

static size_t probe_recommended_size(void) {
	return openmpt_probe_file_header_get_recommended_size();
}
*/
import "C"
import (
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/gopxl/beep/v2"
)

const openmptBlockSize = 2048

// OpenmptProbe checks whether libopenmpt can handle a file by probing its header bytes.
// Returns true if the file is likely supported by libopenmpt.
func OpenmptProbe(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	recSize := int(C.probe_recommended_size())
	bufSize := len(data)
	if bufSize > recSize {
		bufSize = recSize
	}
	result := C.probe_file_header(unsafe.Pointer(&data[0]), C.size_t(bufSize), C.uint64_t(len(data)))
	return result == C.OPENMPT_PROBE_FILE_HEADER_RESULT_SUCCESS
}

// OpenmptSupportedFormats returns the list of file extensions supported by this build of libopenmpt.
func OpenmptSupportedFormats() []string {
	exts := C.openmpt_get_supported_extensions()
	if exts == nil {
		return nil
	}
	s := C.GoString(exts)
	C.openmpt_free_string(exts)
	return strings.Split(s, ";")
}

type OpenmptPlayer struct {
	baseSynth
	mod *C.openmpt_module

	renderSamples [][2]float64
	renderPos     int
}

func NewOpenmptPlayer(path string, sampleRate beep.SampleRate) (*OpenmptPlayer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tracker file: %w", err)
	}

	var cErr C.int
	var cMsg *C.char
	mod := C.create_module(unsafe.Pointer(&data[0]), C.size_t(len(data)), &cErr, &cMsg)
	if mod == nil {
		msg := C.GoString(cMsg)
		C.openmpt_free_string(cMsg)
		return nil, fmt.Errorf("openmpt load module: error %d: %s", int(cErr), msg)
	}

	C.openmpt_module_set_repeat_count(mod, 0)

	title := metadata(mod, "title")
	artist := metadata(mod, "artist")

	p := &OpenmptPlayer{
		mod: mod,
	}
	p.sampleRate = sampleRate
	p.duration = time.Duration(float64(C.openmpt_module_get_duration_seconds(mod)) * float64(time.Second))
	p.isPaused = true
	p.volumeScale = 1.0
	p.title = title
	p.artist = artist

	return p, nil
}

func metadata(mod *C.openmpt_module, key string) string {
	ck := C.CString(key)
	defer C.free(unsafe.Pointer(ck))
	val := C.openmpt_module_get_metadata(mod, ck)
	if val == nil {
		return ""
	}
	s := C.GoString(val)
	C.openmpt_free_string(val)
	return s
}

func (p *OpenmptPlayer) Stream(samples [][2]float64) (n int, ok bool) {
	p.mu.Lock()

	if p.closed || p.finished {
		p.mu.Unlock()
		return 0, false
	}
	if p.isPaused {
		p.mu.Unlock()
		for i := range samples {
			samples[i] = [2]float64{}
		}
		return len(samples), true
	}

	vs := p.volumeScale
	elapsed := p.elapsed
	duration := p.duration
	p.mu.Unlock()

	p.fillSamples(samples, vs, &elapsed, duration)

	p.mu.Lock()
	p.elapsed = elapsed
	p.mu.Unlock()

	return len(samples), true
}

func (p *OpenmptPlayer) fillSamples(samples [][2]float64, vs float64, elapsed *time.Duration, duration time.Duration) {
	buf := make([]C.int16_t, openmptBlockSize*2)
	sampleDur := time.Second / time.Duration(p.sampleRate)

	for i := range samples {
		if *elapsed >= duration {
			p.mu.Lock()
			p.finished = true
			p.isPlaying = false
			p.mu.Unlock()
			for j := i; j < len(samples); j++ {
				samples[j] = [2]float64{}
			}
			return
		}

		if p.renderPos >= len(p.renderSamples) {
			count := C.openmpt_module_read_interleaved_stereo(
				p.mod, C.int32_t(p.sampleRate), openmptBlockSize, &buf[0],
			)
			if count == 0 {
				p.mu.Lock()
				p.finished = true
				p.isPlaying = false
				p.mu.Unlock()
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

func (p *OpenmptPlayer) Err() error { return nil }

func (p *OpenmptPlayer) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.baseStop()
	p.renderSamples = nil
	p.renderPos = 0
}

func (p *OpenmptPlayer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.baseClose()
	if p.mod != nil {
		C.openmpt_module_destroy(p.mod)
		p.mod = nil
	}
	return nil
}

func (p *OpenmptPlayer) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.elapsed
}

func (p *OpenmptPlayer) Duration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.duration
}

func (p *OpenmptPlayer) Seek(pos time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	secs := pos.Seconds()
	C.openmpt_module_set_position_seconds(p.mod, C.double(secs))
	p.elapsed = pos
	p.finished = false
	p.renderSamples = nil
	p.renderPos = 0
	return nil
}

func (p *OpenmptPlayer) Streamer() Streamer { return p }

func (p *OpenmptPlayer) Open(path string) error {
	return fmt.Errorf("openmpt player does not support Open, use NewOpenmptPlayer")
}