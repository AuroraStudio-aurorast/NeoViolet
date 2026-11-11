package synth

import (
	"math"
	"runtime"
	"time"
	"unsafe"
)

// OpenmptDuration returns the duration libopenmpt reports for a module held in
// memory. It is a metadata query: the module is loaded and destroyed without
// rendering. The bool is false when libopenmpt is unavailable, the data is not
// a module, or the library does not report a usable duration.
func OpenmptDuration(data []byte) (time.Duration, bool) {
	api := ensureOpenmptLib()
	if api == nil || len(data) == 0 {
		return 0, false
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
	if cMsg != nil {
		api.freeString(cMsg)
	}
	if mod == nil {
		return 0, false
	}
	defer api.moduleDestroy(mod)

	// SetRepeatCount(0) makes the duration finite in the same way as playback.
	api.moduleSetRepeatCount(mod, 0)
	seconds := api.moduleGetDurationSeconds(mod)
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
		return 0, false
	}
	nanos := seconds * float64(time.Second)
	if nanos >= float64(math.MaxInt64) {
		return 0, false
	}
	return time.Duration(nanos), true
}
