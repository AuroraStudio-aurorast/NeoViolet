package audio

import (
	"path/filepath"
	"strings"
	"testing"
)

// A file whose detected format is IT-family but whose own extension says it is
// something the built-in engine cannot decode (MPTM shares the IT magic, so the
// detector calls it ".it") must be refused with a message that names the format
// and the way out, instead of being handed to the engine that panics on it.
func TestGotrackerSupports(t *testing.T) {
	cases := []struct {
		path string
		want string // the format name the message must carry; "" means allowed
	}{
		{"song.mod", ""},
		{"song.XM", ""}, // upper-case extension, allowed like its lower-case form
		{"song.s3m", ""},
		{"song.it", ""},
		{"song", ""},     // nothing to judge by, the engine's detection decides
		{"song.mp3", ""}, // not a tracker format at all
		{"song.mid", ""}, // MIDI goes to its own path, never to this engine
		{"song.midi", ""},
		{"song.mptm", ".MPTM"},
		{"song.MO3", ".MO3"}, // refused, which also pins that the extension is lowercased
		{"song.mtm", ".MTM"},
		{"song.669", ".669"},
		{"/some/dir/song.mptm", ".MPTM"},
	}

	for _, tc := range cases {
		err := gotrackerSupports(tc.path)

		if tc.want == "" {
			if err != nil {
				t.Errorf("gotrackerSupports(%q) = %v, want nil", tc.path, err)
			}
			continue
		}

		if err == nil {
			t.Errorf("gotrackerSupports(%q) = nil, want a refusal", tc.path)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("gotrackerSupports(%q) error = %q, want it to name %s", tc.path, err, tc.want)
		}
		if !strings.Contains(err.Error(), "libopenmpt") {
			t.Errorf("gotrackerSupports(%q) error = %q, want it to point at libopenmpt", tc.path, err)
		}
	}
}

// The refusal has to happen before the engine is handed the file and before any
// audio device is opened. Pinning the backend makes the check independent of
// whether libopenmpt happens to be installed, and the file never has to exist.
func TestOpenTrackerSynthRefusesFormatTheEngineCannotDecode(t *testing.T) {
	p := &Player{trackerBackend: "gotracker"}

	err := p.openTrackerSynth(filepath.Join(t.TempDir(), "song.mptm"), ".it", 44100)
	if err == nil {
		t.Fatal("openTrackerSynth() = nil for .mptm, want a refusal")
	}
	if !strings.Contains(err.Error(), "libopenmpt") {
		t.Errorf("openTrackerSynth() error = %q, want it to point at libopenmpt", err)
	}
}
