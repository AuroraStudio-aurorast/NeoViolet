package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestScanCacheReusesOnlyUnchangedSuccessfulProbes(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a.wav"), filepath.Join(root, "b.wav")
	for _, path := range []string{a, b} {
		if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	probe := func(_ context.Context, path string) (Track, error) {
		calls++
		return Track{Title: filepath.Base(path)}, nil
	}
	first, err := Scan(context.Background(), []string{root}, func(string) bool { return true }, probe, nil)
	if err != nil || calls != 2 {
		t.Fatal("initial scan failed")
	}
	calls = 0
	second, err := Scan(context.Background(), []string{root}, func(string) bool { return true }, probe, nil, ScanOptions{Cache: first.Cache})
	if err != nil || calls != 0 || len(second.Tracks) != 2 {
		t.Fatal("unchanged files were probed")
	}
	if err = os.WriteFile(a, []byte("changed size"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(b); err != nil {
		t.Fatal(err)
	}
	third, err := Scan(context.Background(), []string{root}, func(string) bool { return true }, probe, nil, ScanOptions{Cache: second.Cache})
	if err != nil || calls != 1 || len(third.Tracks) != 1 || len(third.Cache.Entries) != 1 {
		t.Fatal("changed/deleted entries not updated")
	}
}
func TestScanPublishesSelectablePathsBeforeProbing(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "雨.wav")
	if err := os.WriteFile(path, []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	published := false
	calls := 0
	opts := ScanOptions{Publish: func(tracks []Track) {
		if len(tracks) != 1 || tracks[0].Path != canonical {
			t.Fatal("missing initial identity")
		}
		published = true
		tracks[0].Title = "consumer edit"
	}}
	probe := func(context.Context, string) (Track, error) {
		calls++
		if !published {
			t.Fatal("no selectable rows before metadata")
		}
		return Track{}, errors.New("bad metadata")
	}
	first, err := Scan(context.Background(), []string{root}, func(string) bool { return true }, probe, nil, opts)
	if err != nil || first.Tracks[0].Title != "雨.wav" || len(first.Cache.Entries) != 0 {
		t.Fatal("failure cached or snapshot aliased")
	}
	_, err = Scan(context.Background(), []string{root}, func(string) bool { return true }, probe, nil, ScanOptions{Cache: first.Cache})
	if err != nil || calls != 2 {
		t.Fatal("failed probe not retried")
	}
}
func TestLibraryCachePersistenceAndCorruptRecovery(t *testing.T) {
	root := t.TempDir()
	track := filepath.Join(root, "A.wav")
	if err := os.WriteFile(track, []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(context.Background(), []string{root}, func(string) bool { return true }, func(context.Context, string) (Track, error) { return Track{Title: "A"}, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cache", "library.json")
	if err = SaveCache(context.Background(), path, result.Cache); err != nil {
		t.Fatal(err)
	}
	cached, err := LoadCache(path)
	if err != nil || !cached.MatchesRoots([]string{root}) || len(cached.Tracks()) != 1 || cached.Tracks()[0].Title != "A" {
		t.Fatalf("cache roundtrip failed: %v", err)
	}
	if cached.MatchesRoots([]string{t.TempDir()}) {
		t.Fatal("different roots matched")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = SaveCache(ctx, path, &Cache{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled cache writer replaced store")
	}
	// #nosec G304 -- path is scoped to this temporary cache fixture.
	before, _ := os.ReadFile(path)
	if len(before) == 0 {
		t.Fatal("cancel removed old cache")
	}
	for _, data := range []string{"{", `{"version":999}`, `null`} {
		if err = os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = LoadCache(path); err == nil {
			t.Fatal("invalid cache accepted")
		}
	}
}
