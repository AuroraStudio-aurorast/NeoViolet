package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestScanDeduplicatesRootsKeepsFailuresAndHonorsCancellation(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"B.wav", "a.wav", "sub/broken.wav", "ignore.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	include := func(path string) bool { return filepath.Ext(path) == ".wav" }
	calls := 0
	probe := func(ctx context.Context, path string) (Track, error) {
		calls++
		if filepath.Base(path) == "broken.wav" {
			return Track{}, errors.New("bad header")
		}
		return Track{Title: filepath.Base(path), Format: "WAV"}, ctx.Err()
	}
	got, err := Scan(context.Background(), []string{root, sub, filepath.Join(root, "missing")}, include, probe, nil)
	if err != nil || len(got.Tracks) != 3 || calls != 3 || got.Tracks[0].Title != "a.wav" || len(got.Issues) != 2 {
		t.Fatalf("wrong scan: %+v calls=%d %v", got, calls, err)
	}
	if got.Tracks[2].Path == "" || got.Tracks[2].Title != "broken.wav" {
		t.Fatal("probe failure removed track identity")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	_, err = Scan(ctx, []string{root}, include, probe, nil)
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled scan still probed files")
	}
}
func TestScanDoesNotFollowDirectorySymlinkLoops(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Skip(err)
	}
	if err := os.WriteFile(filepath.Join(root, "track.wav"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Scan(context.Background(), []string{root}, func(p string) bool { return filepath.Ext(p) == ".wav" }, func(context.Context, string) (Track, error) { return Track{}, nil }, nil)
	if err != nil || len(got.Tracks) != 1 {
		t.Fatalf("symlink loop followed: %+v %v", got, err)
	}
}
