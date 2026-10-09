package anim

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSidecar_SitsBesideTheTrack(t *testing.T) {
	dir := t.TempDir()
	sidecar := filepath.Join(dir, "track.nvaa")
	touch(t, sidecar)

	if got := FindSidecar(filepath.Join(dir, "track.flac")); got != sidecar {
		t.Fatalf("FindSidecar = %q, want %q", got, sidecar)
	}
}

func TestFindSidecar_IgnoresTheAudioExtension(t *testing.T) {
	dir := t.TempDir()
	sidecar := filepath.Join(dir, "track.nvaa")
	touch(t, sidecar)

	for _, name := range []string{"track.flac", "track.mp3", "track.ape", "track"} {
		t.Run(name, func(t *testing.T) {
			if got := FindSidecar(filepath.Join(dir, name)); got != sidecar {
				t.Fatalf("FindSidecar(%q) = %q, want %q", name, got, sidecar)
			}
		})
	}
}

func TestFindSidecar_NoAnimationForTheTrack(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "something-else.nvaa"))

	if got := FindSidecar(filepath.Join(dir, "track.flac")); got != "" {
		t.Fatalf("FindSidecar = %q, want no sidecar", got)
	}
}

func TestFindSidecar_EmptyPath(t *testing.T) {
	if got := FindSidecar(""); got != "" {
		t.Fatalf("FindSidecar(%q) = %q, want no sidecar", "", got)
	}
}

// A directory that happens to carry the extension is not an animation, and
// saying so here keeps the failure where it belongs rather than in the decoder.
func TestFindSidecar_DirectoryIsNotASidecar(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "track.nvaa"), 0o750); err != nil {
		t.Fatal(err)
	}

	if got := FindSidecar(filepath.Join(dir, "track.flac")); got != "" {
		t.Fatalf("FindSidecar = %q, want no sidecar for a directory", got)
	}
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}
