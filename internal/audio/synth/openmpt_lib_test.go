package synth

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// versionPattern is the shape openmpt_get_library_version decodes to, for
// example "0.8.9".
var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// The tests run in source order in a single process, and the loader caches its
// result, so every test that depends on the cached state resets it first.

func TestCandidatesHaveNoEmptyEntries(t *testing.T) {
	// Platforms without a loader binding (see openmpt_paths_other.go) name no
	// library at all, so there is nothing to assert about their candidates.
	if names := openmptLibraryNames(); len(names) == 0 {
		t.Skip("no libopenmpt library names on this platform")
	}

	candidates := openmptCandidates()
	if len(candidates) == 0 {
		t.Fatal("openmptCandidates() returned no candidates")
	}
	for _, candidate := range candidates {
		if candidate == "" {
			t.Errorf("openmptCandidates() contains an empty entry: %q", candidates)
		}
	}
}

func TestLoadFailsOnBogusOverride(t *testing.T) {
	t.Setenv(openmptEnvOverride, "/nonexistent/libopenmpt.0.dylib")
	resetOpenmptState()
	defer resetOpenmptState()

	if OpenmptAvailable() {
		t.Error("OpenmptAvailable() = true with the override pointing at a missing library")
	}
	if api := ensureOpenmptLib(); api != nil {
		t.Errorf("ensureOpenmptLib() = %v, want nil", api)
	}
	if version := OpenmptVersion(); version != "" {
		t.Errorf("OpenmptVersion() = %q without a library, want empty", version)
	}
	if path := OpenmptLibraryPath(); path != "" {
		t.Errorf("OpenmptLibraryPath() = %q without a library, want empty", path)
	}
}

func TestCheckSymbolsReportsMissing(t *testing.T) {
	resetOpenmptState()
	if !OpenmptAvailable() {
		t.Skip("libopenmpt not installed")
	}

	path := OpenmptLibraryPath()
	handle, err := openmptOpen(path)
	if err != nil {
		t.Fatalf("openmptOpen(%q): %v", path, err)
	}
	defer func() {
		if closeErr := openmptClose(handle); closeErr != nil {
			t.Errorf("openmptClose(%q): %v", path, closeErr)
		}
	}()

	err = checkSymbols(handle, "openmpt_definitely_missing")
	if err == nil {
		t.Fatal("checkSymbols() = nil for a symbol the library does not export")
	}
	if !strings.Contains(err.Error(), "openmpt_definitely_missing") {
		t.Errorf("checkSymbols() error = %q, want it to name the missing symbol", err)
	}
}

func TestLoadRealLibraryIfPresent(t *testing.T) {
	resetOpenmptState()
	if !OpenmptAvailable() {
		t.Skip("libopenmpt not installed")
	}

	if path := OpenmptLibraryPath(); path == "" {
		t.Error("OpenmptLibraryPath() is empty after a successful load")
	}

	api := ensureOpenmptLib()
	if api == nil {
		t.Fatal("ensureOpenmptLib() = nil after OpenmptAvailable() = true")
	}

	version := openmptVersionString(api.getLibraryVersion())
	if !versionPattern.MatchString(version) {
		t.Errorf("openmptVersionString() = %q, want major.minor.patch", version)
	}
	t.Logf("loaded %s, library version %s", OpenmptLibraryPath(), version)

	if got := OpenmptVersion(); got != version {
		t.Errorf("OpenmptVersion() = %q, want %q", got, version)
	}

	bound := map[string]any{
		"openmpt_probe_file_header_get_recommended_size": api.probeRecommendedSize,
		"openmpt_probe_file_header":                      api.probeFileHeader,
		"openmpt_module_create_from_memory2":             api.moduleCreateFromMemory2,
		"openmpt_module_destroy":                         api.moduleDestroy,
		"openmpt_module_set_repeat_count":                api.moduleSetRepeatCount,
		"openmpt_module_get_duration_seconds":            api.moduleGetDurationSeconds,
		"openmpt_module_set_position_seconds":            api.moduleSetPositionSeconds,
		"openmpt_module_get_metadata":                    api.moduleGetMetadata,
		"openmpt_module_read_interleaved_stereo":         api.moduleReadInterleavedStereo,
		"openmpt_get_supported_extensions":               api.getSupportedExtensions,
		"openmpt_free_string":                            api.freeString,
		"openmpt_get_library_version":                    api.getLibraryVersion,
	}
	for name, fn := range bound {
		if reflect.ValueOf(fn).IsNil() {
			t.Errorf("bound symbol %s is nil", name)
		}
	}

	// The two callbacks are exported functions rather than data symbols, so the
	// bind layer keeps their addresses instead of registering them.
	if api.logFuncSilent == 0 {
		t.Error("openmpt_log_func_silent address is zero")
	}
	if api.errorFuncIgnore == 0 {
		t.Error("openmpt_error_func_ignore address is zero")
	}
}

func TestOpenmptVersionString(t *testing.T) {
	// libopenmpt.h documents the packing as (major << 24 | minor << 16 | patch),
	// so 0.8.9 arrives as 0x00080009.
	cases := []struct {
		packed uint32
		want   string
	}{
		{0x00080009, "0.8.9"},
		{0x01070005, "1.7.5"},
		{0x00000000, "0.0.0"},
	}
	for _, tc := range cases {
		if got := openmptVersionString(tc.packed); got != tc.want {
			t.Errorf("openmptVersionString(%#x) = %q, want %q", tc.packed, got, tc.want)
		}
	}
}

func TestEnvOverrideWins(t *testing.T) {
	t.Setenv(openmptEnvOverride, "/tmp/x.dylib")

	got := openmptCandidates()
	if len(got) != 1 || got[0] != "/tmp/x.dylib" {
		t.Errorf("openmptCandidates() = %q, want exactly the override", got)
	}
}

func TestProbeConstantsMatchHeader(t *testing.T) {
	// Named C constants, which happen to be 0x3 and 1; pinning them here keeps a
	// careless edit from silently changing what the probe asks for.
	const flagsDefault = 0x1 | 0x2 // OPENMPT_PROBE_FILE_HEADER_FLAGS_MODULES|CONTAINERS
	if openmptProbeFlagsDefault != flagsDefault {
		t.Errorf("openmptProbeFlagsDefault = %#x, want %#x", openmptProbeFlagsDefault, flagsDefault)
	}
	const resultSuccess = 1 // OPENMPT_PROBE_FILE_HEADER_RESULT_SUCCESS
	if openmptProbeResultSuccess != resultSuccess {
		t.Errorf("openmptProbeResultSuccess = %d, want %d", openmptProbeResultSuccess, resultSuccess)
	}
}
