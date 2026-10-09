package anim

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/power"
)

// An animation is read whole, so the size of the file is the size of the
// allocation. A track that has something enormous beside it -- a file that is
// not an animation at all, or one far larger than anything real -- is refused
// rather than read.
func TestLoadFor_RefusesASidecarTooLargeToRead(t *testing.T) {
	dir := t.TempDir()
	track := filepath.Join(dir, "track.flac")
	if err := os.WriteFile(track, nil, 0o600); err != nil { // #nosec G703 -- a file this test just made in its own temp dir
		t.Fatal(err)
	}

	// Sparse, so the size is real without the disk being asked for it.
	big, err := os.Create(filepath.Join(dir, "track.nvaa")) // #nosec G304 -- a file this test just made in its own temp dir
	if err != nil {
		t.Fatal(err)
	}
	if err := big.Truncate(maxSidecarSize + 1); err != nil {
		t.Fatal(err)
	}
	if err := big.Close(); err != nil {
		t.Fatal(err)
	}

	state := New()
	state.ReadPower = func() (power.Status, error) { return power.Status{}, nil }
	cmd := state.LoadFor(loadAt(track, 40, 12, config.GateModeEither))
	if cmd == nil {
		t.Fatal("no load command")
	}
	state.Apply(cmd().(LoadedMsg))

	if !errors.Is(state.Err, ErrSidecarTooLarge) {
		t.Fatalf("err = %v, want %v", state.Err, ErrSidecarTooLarge)
	}
	if state.Visible {
		t.Error("a sidecar that was refused is showing")
	}
}
