package synth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// libopenmpt is loaded at run time rather than linked, so it is an optional
// dependency: the player works without it and callers fall back to gotracker.
// Everything OS specific — how a library is opened and closed, and how a symbol
// is looked up — lives in openmpt_paths_<os>.go; this file is shared.

// openmptEnvOverride names the environment variable that replaces the library
// search with a single path. Pointing it at a file that does not exist is how a
// user (or a test) disables libopenmpt for one process.
const openmptEnvOverride = "NEOVIOLET_LIBOPENMPT"

// Values from libopenmpt.h: the default probe asks about both module and
// container formats, and RESULT_SUCCESS is "the file will most likely be
// supported".
const (
	openmptProbeFlagsDefault  = 0x3 // OPENMPT_PROBE_FILE_HEADER_FLAGS_MODULES|CONTAINERS
	openmptProbeResultSuccess = 1
)

// openmptAPI holds the entry points of a loaded libopenmpt. Argument widths
// mirror the C declarations in libopenmpt.h exactly: uintptr for size_t and for
// function pointers, int32 for int and int32_t, uint64 for uint64_t.
type openmptAPI struct {
	probeRecommendedSize func() uintptr
	probeFileHeader      func(flags uint64, data unsafe.Pointer, size uintptr, filesize uint64,
		logFunc uintptr, logUser unsafe.Pointer, errFunc uintptr, errUser unsafe.Pointer,
		outErr *int32, outMsg *unsafe.Pointer) int32
	moduleCreateFromMemory2 func(data unsafe.Pointer, size uintptr,
		logFunc uintptr, logUser unsafe.Pointer, errFunc uintptr, errUser unsafe.Pointer,
		outErr *int32, outMsg *unsafe.Pointer, ctls unsafe.Pointer) unsafe.Pointer
	moduleDestroy               func(mod unsafe.Pointer)
	moduleSetRepeatCount        func(mod unsafe.Pointer, repeatCount int32) int32
	moduleGetDurationSeconds    func(mod unsafe.Pointer) float64
	moduleSetPositionSeconds    func(mod unsafe.Pointer, seconds float64) float64
	moduleGetMetadata           func(mod unsafe.Pointer, key unsafe.Pointer) unsafe.Pointer
	moduleReadInterleavedStereo func(mod unsafe.Pointer, rate int32, count uintptr, buf unsafe.Pointer) uintptr
	getSupportedExtensions      func() unsafe.Pointer
	freeString                  func(s unsafe.Pointer)
	getLibraryVersion           func() uint32

	// The silent logger and the ignoring error handler are exported functions
	// rather than data symbols, so the library takes their addresses as
	// arguments instead of them being registered as entry points.
	logFuncSilent   uintptr
	errorFuncIgnore uintptr
}

// openmptLibState is the once-only load result. Failures are cached too, so a
// missing library is looked for exactly once per process.
type openmptLibState struct {
	mu      sync.Mutex
	done    bool
	api     *openmptAPI
	err     error
	path    string
	version uint32
}

var openmptLib openmptLibState

// OpenmptAvailable reports whether libopenmpt could be loaded. It is a pure
// query: the first call resolves the library and every later one reuses that
// result, so NEOVIOLET_LIBOPENMPT is read once per process.
func OpenmptAvailable() bool {
	return ensureOpenmptLib() != nil
}

// ensureOpenmptLib returns the loaded library, resolving it on first use.
func ensureOpenmptLib() *openmptAPI {
	openmptLib.mu.Lock()
	defer openmptLib.mu.Unlock()

	if openmptLib.done {
		return openmptLib.api
	}
	openmptLib.done = true

	api, path, err := loadOpenmpt()
	openmptLib.api, openmptLib.path, openmptLib.err = api, path, err
	if api == nil {
		logger.Debug("libopenmpt unavailable", "error", openmptLib.err)
		return nil
	}

	openmptLib.version = api.getLibraryVersion()
	logger.Info("libopenmpt loaded",
		"path", path,
		"version", openmptVersionString(openmptLib.version),
	)
	return api
}

// OpenmptLibraryPath reports the file libopenmpt was loaded from, or "" when no
// library is available.
func OpenmptLibraryPath() string {
	ensureOpenmptLib()

	openmptLib.mu.Lock()
	defer openmptLib.mu.Unlock()
	return openmptLib.path
}

// OpenmptVersion reports the version of the resolved libopenmpt, for example
// "0.8.9", or "" when no library is available.
func OpenmptVersion() string {
	if ensureOpenmptLib() == nil {
		return ""
	}

	openmptLib.mu.Lock()
	defer openmptLib.mu.Unlock()
	return openmptVersionString(openmptLib.version)
}

// resetOpenmptState drops the cached load result so the next query resolves the
// library again. The package's own tests need to observe both the available and
// the unavailable outcome in a single process.
func resetOpenmptState() {
	openmptLib.mu.Lock()
	defer openmptLib.mu.Unlock()

	openmptLib.done = false
	openmptLib.api = nil
	openmptLib.err = nil
	openmptLib.path = ""
	openmptLib.version = 0
}

// loadOpenmpt tries every candidate in order and reports the last failure, so
// the error names the path that was tried last along with what went wrong.
func loadOpenmpt() (*openmptAPI, string, error) {
	var lastErr error
	for _, path := range openmptCandidates() {
		handle, err := openmptOpen(path)
		if err != nil {
			lastErr = fmt.Errorf("open %s: %w", path, err)
			continue
		}
		api, err := bindOpenmpt(handle)
		if err != nil {
			// The module loaded but cannot be used, so release it rather than
			// keeping it mapped for the lifetime of the process.
			if closeErr := openmptClose(handle); closeErr != nil {
				err = fmt.Errorf("%w (close: %v)", err, closeErr)
			}
			lastErr = fmt.Errorf("bind %s: %w", path, err)
			continue
		}
		return api, path, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no libopenmpt candidates")
	}
	return nil, "", lastErr
}

// openmptVersionString renders the packed version openmpt_get_library_version
// returns the way libopenmpt writes it, for example "0.8.9". libopenmpt.h
// documents the packing as (major << 24 | minor << 16 | patch).
func openmptVersionString(v uint32) string {
	return fmt.Sprintf("%d.%d.%d", v>>24&0xff, v>>16&0xff, v&0xff)
}

// openmptCandidates lists every library path to try, in order. A non-empty
// NEOVIOLET_LIBOPENMPT replaces the whole search rather than adding to it.
func openmptCandidates() []string {
	if override := os.Getenv(openmptEnvOverride); override != "" {
		return []string{override}
	}

	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}

	names := openmptLibraryNames()
	var candidates []string
	for _, dir := range openmptSearchDirs(exeDir) {
		for _, name := range names {
			if dir == "." {
				// A bare file name is left to the loader's own search: the
				// ldconfig cache on Linux, the executable's directory on Windows.
				candidates = append(candidates, name)
				continue
			}
			candidates = append(candidates, filepath.Join(dir, name))
		}
	}
	return candidates
}

// checkSymbols reports every required symbol the library at handle does not
// export. Registering a symbol that is not there fails, so the bind step asks
// for all of them first and a library that is merely incomplete is rejected
// instead of half-bound.
func checkSymbols(handle uintptr, names ...string) error {
	var missing []string
	for _, name := range names {
		if _, err := openmptSymbol(handle, name); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("library does not export %s", strings.Join(missing, ", "))
}

// libopenmpt exports the silent logger and the ignoring error handler as
// functions rather than data symbols, so their addresses are passed to the
// library as arguments instead of being registered as entry points.
const (
	openmptLogFuncSilent   = "openmpt_log_func_silent"
	openmptErrorFuncIgnore = "openmpt_error_func_ignore"
)

// bindOpenmpt resolves every entry point the player uses. Binding is
// all-or-nothing: a library missing any of them is rejected rather than
// half-bound, which would only fail later and less clearly.
func bindOpenmpt(handle uintptr) (*openmptAPI, error) {
	api := &openmptAPI{}
	bindings := []struct {
		name   string
		target any
	}{
		{"openmpt_probe_file_header_get_recommended_size", &api.probeRecommendedSize},
		{"openmpt_probe_file_header", &api.probeFileHeader},
		{"openmpt_module_create_from_memory2", &api.moduleCreateFromMemory2},
		{"openmpt_module_destroy", &api.moduleDestroy},
		{"openmpt_module_set_repeat_count", &api.moduleSetRepeatCount},
		{"openmpt_module_get_duration_seconds", &api.moduleGetDurationSeconds},
		{"openmpt_module_set_position_seconds", &api.moduleSetPositionSeconds},
		{"openmpt_module_get_metadata", &api.moduleGetMetadata},
		{"openmpt_module_read_interleaved_stereo", &api.moduleReadInterleavedStereo},
		{"openmpt_get_supported_extensions", &api.getSupportedExtensions},
		{"openmpt_free_string", &api.freeString},
		{"openmpt_get_library_version", &api.getLibraryVersion},
	}

	names := make([]string, 0, len(bindings)+2)
	for _, binding := range bindings {
		names = append(names, binding.name)
	}
	names = append(names, openmptLogFuncSilent, openmptErrorFuncIgnore)
	if err := checkSymbols(handle, names...); err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		if err := openmptRegister(binding.target, handle, binding.name); err != nil {
			return nil, fmt.Errorf("register %s: %w", binding.name, err)
		}
	}

	logSilent, err := openmptSymbol(handle, openmptLogFuncSilent)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", openmptLogFuncSilent, err)
	}
	errIgnore, err := openmptSymbol(handle, openmptErrorFuncIgnore)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", openmptErrorFuncIgnore, err)
	}
	api.logFuncSilent, api.errorFuncIgnore = logSilent, errIgnore
	return api, nil
}
