package audio

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
)

const (
	// fallbackHelperEnv marks the re-executed test process that checks the
	// no-library contract. Without it the helper test is skipped, so an
	// ordinary run of this package — where libopenmpt may well be available —
	// is unaffected.
	fallbackHelperEnv = "NEOVIOLET_FALLBACK_HELPER"
	// fallbackHelperTest is the helper's own name, used both to select it in
	// the child process and to read its result out of the child's output.
	fallbackHelperTest = "TestOpenTrackerFallbackHelper"
	// fallbackMissingLib is the NEOVIOLET_LIBOPENMPT value that makes
	// libopenmpt unavailable: the override replaces the whole library search
	// with this single path, which cannot exist.
	fallbackMissingLib = "/nonexistent/libopenmpt.0.dylib"
	// fallbackFixture is the minimal ProTracker module the synth package keeps
	// on disk and verifies byte for byte, so the tracker fallback can be played
	// without a module file from outside the repository.
	fallbackFixture = "synth/testdata/minimal.mod"
	// fallbackContractChecked is logged by the helper once the contract
	// assertions have held. The parent accepts a skip only when this line shows
	// up, so a skip can never stand in for a contract that was not checked: the
	// helper skipping because its guard env did not arrive would otherwise read
	// as a pass and quietly hollow out this test.
	fallbackContractChecked = "helper reached the contract checks"
	// fallbackFixtureMissing marks the skip that is a defect rather than a
	// tolerated platform difference: the fixture ships with the tree.
	fallbackFixtureMissing = "helper skip: fixture missing"
	// fallbackGotrackerRejects marks the one skip the parent tolerates, because
	// it may be platform specific.
	fallbackGotrackerRejects = "helper skip: gotracker rejects fixture"
)

// TestOpenTrackerFallbackWithoutLibrary pins the user-visible contract of a
// process that has no libopenmpt: the library reports itself unavailable, the
// extension list it would contribute is empty, tracker extensions are still
// synthetic, and opening a module falls back to the gotracker backend.
//
// Availability is resolved once per process and cached either way, so a process
// that has already resolved the library cannot be made to lose it. The contract
// is therefore asserted by the helper below, which runs in a fresh process
// started with NEOVIOLET_LIBOPENMPT pointing at a missing file; this test only
// reports the helper's outcome.
func TestOpenTrackerFallbackWithoutLibrary(t *testing.T) {
	// The inherited environment may already name a library, and getenv returns
	// the first match, so drop those entries before appending our own.
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, fallbackHelperEnv+"=") || strings.HasPrefix(kv, "NEOVIOLET_LIBOPENMPT=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		fallbackHelperEnv+"=1",
		"NEOVIOLET_LIBOPENMPT="+fallbackMissingLib,
	)

	//nolint:gosec // the program re-executes its own test binary; no argument comes from input.
	cmd := exec.Command(os.Args[0], "-test.v", "-test.run="+fallbackHelperTest)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper process failed: %v\n%s", err, out)
	}

	// A skip is only acceptable in the one case the helper reaches its last
	// check and cannot run it: gotracker may not accept the fixture on this
	// platform. Every other skip means the run proved nothing — the helper never
	// got its guard env, the fixture is missing from the tree, or the helper
	// skipped for a reason this test has not been taught — so it is a failure.
	// The sentinel the helper logs right after the contract assertions is what
	// separates the two.
	switch out := string(out); {
	case strings.Contains(out, "--- SKIP: "+fallbackHelperTest):
		switch {
		case !strings.Contains(out, fallbackContractChecked):
			t.Fatalf("helper skipped before checking the contract: its guard env %s did not reach the child process\n%s", fallbackHelperEnv, out)
		case strings.Contains(out, fallbackFixtureMissing):
			t.Fatalf("the tracker fixture %s is missing from the tree\n%s", fallbackFixture, out)
		case strings.Contains(out, fallbackGotrackerRejects):
			t.Logf("helper skipped the tracker fallback check, gotracker rejects the fixture:\n%s", out)
		default:
			t.Fatalf("helper skipped after the contract checks for an unrecognised reason\n%s", out)
		}
	case strings.Contains(out, "--- PASS: "+fallbackHelperTest):
		t.Logf("helper passed:\n%s", out)
	default:
		// Neither outcome means the helper never ran, which would make this test
		// pass without exercising the contract at all.
		t.Fatalf("helper process did not run %s:\n%s", fallbackHelperTest, out)
	}
}

// TestOpenTrackerFallbackHelper asserts the no-library contract. It is inert
// unless TestOpenTrackerFallbackWithoutLibrary started this process.
func TestOpenTrackerFallbackHelper(t *testing.T) {
	if os.Getenv(fallbackHelperEnv) != "1" {
		t.Skipf("helper: run through TestOpenTrackerFallbackWithoutLibrary")
	}

	// The contract that holds whether or not tracker modules can actually be
	// played. It is asserted first, so the skip below cannot hide a violation.
	if synth.OpenmptAvailable() {
		t.Errorf("OpenmptAvailable() = true, want false with NEOVIOLET_LIBOPENMPT=%s", fallbackMissingLib)
	}
	if formats := synth.OpenmptSupportedFormats(); formats != nil {
		t.Errorf("OpenmptSupportedFormats() = %v, want nil without libopenmpt", formats)
	}
	if !IsSyntheticFormat(".mod") {
		t.Error(`IsSyntheticFormat(".mod") = false, want true: the built-in extension table works without libopenmpt`)
	}
	if IsSyntheticFormat(".mp3") {
		t.Error(`IsSyntheticFormat(".mp3") = true, want false`)
	}

	// Proof that the contract was actually checked. The parent fails a skip that
	// lacks this line, so the guard env going missing cannot pass as a skip.
	t.Log(fallbackContractChecked)

	if _, err := os.Stat(fallbackFixture); err != nil {
		t.Skipf("%s: %v", fallbackFixtureMissing, err)
	}
	if _, err := synth.NewTrackerPlayer(fallbackFixture, ".mod", 44100); err != nil {
		t.Skipf("%s: %v", fallbackGotrackerRejects, err)
	}

	p := NewPlayer()
	defer func() {
		if p.synthCtrl != nil {
			_ = p.synthCtrl.Close()
		}
	}()
	if err := p.openTrackerSynth(fallbackFixture, ".mod", 44100); err != nil {
		t.Fatalf("openTrackerSynth(%q) = %v, want nil: the gotracker fallback must take over without libopenmpt",
			fallbackFixture, err)
	}
}
