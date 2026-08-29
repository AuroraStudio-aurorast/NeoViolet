package fetch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

// setupCacheDir points the XDG cache dir at a temp directory and restores
// the global XDG flag afterwards.
func setupCacheDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	config.SetXDGConfig(true)
	t.Cleanup(func() { config.SetXDGConfig(false) })
	return filepath.Join(dir, "neoviolet", "lyrics")
}

func sampleCacheFile(sig string) CacheFile {
	return CacheFile{
		Version:   cacheFileVersion,
		Sig:       sig,
		Provider:  "https://lrclib.net",
		State:     "found",
		TrackID:   42,
		TrackName: "Shelter",
		Duration:  219,
		FetchedAt: time.Now(),
	}
}

func TestCacheFileNameDeterministic(t *testing.T) {
	a, b := cacheFileName("x"), cacheFileName("x")
	if a != b {
		t.Errorf("cacheFileName() not deterministic: %q vs %q", a, b)
	}
	if len(a) != 37 || !strings.HasSuffix(a, ".json") {
		t.Errorf("cacheFileName() = %q, want 32 hex + .json", a)
	}
	if cacheFileName("x") == cacheFileName("y") {
		t.Error("cacheFileName() collision for different sigs")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	setupCacheDir(t)
	cf := sampleCacheFile("sig-rt")
	cf.SyncedLyrics = "[00:01.00]line"
	if err := Save(cf); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, ok := Load("sig-rt", "https://lrclib.net")
	if !ok {
		t.Fatal("Load() miss, want hit")
	}
	if got.SyncedLyrics != cf.SyncedLyrics || got.TrackID != 42 {
		t.Errorf("Load() = %+v, want matching record", got)
	}

	// no .tmp residue
	matches, err := filepath.Glob(cacheFilePathOrDie(t, "sig-rt") + ".tmp")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf(".tmp residue: %v", matches)
	}
}

func TestLoadExpired(t *testing.T) {
	setupCacheDir(t)
	cf := sampleCacheFile("sig-exp")
	cf.FetchedAt = time.Now().Add(-ttlFound - time.Hour)
	if err := Save(cf); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	if _, ok := Load("sig-exp", "https://lrclib.net"); ok {
		t.Error("Load() expired = hit, want miss")
	}
	if _, err := os.Stat(cacheFilePathOrDie(t, "sig-exp")); !os.IsNotExist(err) {
		t.Error("expired cache file should be removed")
	}
}

func TestLoadProviderMismatch(t *testing.T) {
	setupCacheDir(t)
	cf := sampleCacheFile("sig-prov")
	if err := Save(cf); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if _, ok := Load("sig-prov", "https://other.example"); ok {
		t.Error("Load() with different provider = hit, want miss")
	}
}

func TestLoadCorrupt(t *testing.T) {
	setupCacheDir(t)
	path := cacheFilePathOrDie(t, "sig-corrupt")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	if _, ok := Load("sig-corrupt", "https://lrclib.net"); ok {
		t.Error("Load() corrupt = hit, want miss")
	}
}

func TestLoadVersionSigMismatch(t *testing.T) {
	setupCacheDir(t)
	cf := sampleCacheFile("sig-v")
	cf.Version = 99
	if err := Save(cf); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if _, ok := Load("sig-v", "https://lrclib.net"); ok {
		t.Error("Load() wrong version = hit, want miss")
	}

	// sig mismatch: file name keys on one sig, content carries another
	mismatchPath := cacheFilePathOrDie(t, "sig-v")
	if err := os.MkdirAll(filepath.Dir(mismatchPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cf2 := sampleCacheFile("other")
	data, _ := json.Marshal(cf2)
	if err := os.WriteFile(mismatchPath, data, 0o600); err != nil {
		t.Fatalf("write mismatched: %v", err)
	}
	if _, ok := Load("sig-v", "https://lrclib.net"); ok {
		t.Error("Load() sig mismatch = hit, want miss")
	}
}

func TestCleanupExpired(t *testing.T) {
	dir := setupCacheDir(t)

	fresh := sampleCacheFile("clean-fresh")
	if err := Save(fresh); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	stale := sampleCacheFile("clean-stale")
	stale.State = "notfound"
	stale.FetchedAt = time.Now().Add(-ttlNotfound - time.Hour)
	if err := Save(stale); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	// corrupt cache-named file
	corruptPath := filepath.Join(dir, strings.Repeat("ab", 16)+".json")
	if err := os.WriteFile(corruptPath, []byte("{bad"), 0o600); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	// unrelated file that must survive
	other := filepath.Join(dir, "ratelimit.json")
	if err := os.WriteFile(other, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write ratelimit: %v", err)
	}
	notHex := filepath.Join(dir, "not-a-cache-file.json")
	if err := os.WriteFile(notHex, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write unrelated: %v", err)
	}

	removed, err := CleanupExpired(dir)
	if err != nil {
		t.Fatalf("CleanupExpired() error: %v", err)
	}
	if removed != 2 { // stale + corrupt
		t.Errorf("CleanupExpired() removed = %d, want 2", removed)
	}
	for _, keep := range []string{cacheFileName("clean-fresh"), "ratelimit.json", "not-a-cache-file.json"} {
		if _, err := os.Stat(filepath.Join(dir, keep)); err != nil {
			t.Errorf("CleanupExpired() removed %q, want kept", keep)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, corruptPath)); !os.IsNotExist(err) {
		t.Error("corrupt file should be removed")
	}
}

func TestCleanupExpiredMissingDir(t *testing.T) {
	removed, err := CleanupExpired(t.TempDir() + "/nope")
	if err != nil {
		t.Fatalf("CleanupExpired() error: %v", err)
	}
	if removed != 0 {
		t.Errorf("CleanupExpired() removed = %d, want 0", removed)
	}
}

func TestSaveUnwritableDir(t *testing.T) {
	// Point XDG cache at a path whose parent is a file, forcing MkdirAll to fail.
	parent := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	t.Setenv("XDG_CACHE_HOME", parent)
	config.SetXDGConfig(true)
	t.Cleanup(func() { config.SetXDGConfig(false) })

	cf := sampleCacheFile("sig-nowrite")
	if err := Save(cf); err == nil {
		t.Error("Save() error = nil, want error for unwritable dir")
	}
}

func TestCacheFileJSONShape(t *testing.T) {
	cf := sampleCacheFile("sig-json")
	data, err := json.Marshal(cf)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var back CacheFile
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if back.State != "found" || back.TrackName != "Shelter" {
		t.Errorf("round trip = %+v", back)
	}
}

func cacheFilePathOrDie(t *testing.T, sig string) string {
	t.Helper()
	dir, err := config.CacheDir()
	if err != nil {
		t.Fatalf("CacheDir() error: %v", err)
	}
	return filepath.Join(dir, cacheFileName(sig))
}
