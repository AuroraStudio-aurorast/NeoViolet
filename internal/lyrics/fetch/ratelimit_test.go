package fetch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadRateLimitMissingFile(t *testing.T) {
	r, err := LoadRateLimit(t.TempDir())
	if err != nil {
		t.Fatalf("LoadRateLimit() error: %v", err)
	}
	if r.Blocked("https://lrclib.net") {
		t.Error("Blocked() = true for empty state")
	}
}

func TestLoadRateLimitCorruptFile(t *testing.T) {
	dir := t.TempDir()
	// #nosec G306 -- test fixture written to a private temp dir.
	if err := os.WriteFile(filepath.Join(dir, rateLimitFileName), []byte("{bad"), 0o644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	if _, err := LoadRateLimit(dir); err != nil {
		t.Fatalf("LoadRateLimit() corrupt file: %v, want no error", err)
	}
}

func TestSetRateLimitedPersists(t *testing.T) {
	dir := t.TempDir()
	r, err := LoadRateLimit(dir)
	if err != nil {
		t.Fatalf("LoadRateLimit() error: %v", err)
	}
	if err := r.SetRateLimited("https://lrclib.net", time.Hour); err != nil {
		t.Fatalf("SetRateLimited() error: %v", err)
	}
	if !r.Blocked("https://lrclib.net") {
		t.Fatal("Blocked() = false after SetRateLimited")
	}

	// simulate restart: a fresh instance must still see the cooldown
	r2, err := LoadRateLimit(dir)
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}
	if !r2.Blocked("https://lrclib.net") {
		t.Error("Blocked() = false after reload, cooldown lost")
	}
	if r2.Remaining("https://lrclib.net") <= 0 {
		t.Error("Remaining() <= 0 after reload, want positive")
	}
}

func TestMultiProviderIsolation(t *testing.T) {
	dir := t.TempDir()
	r, err := LoadRateLimit(dir)
	if err != nil {
		t.Fatalf("LoadRateLimit() error: %v", err)
	}
	if err := r.SetRateLimited("https://provider-a.example", time.Hour); err != nil {
		t.Fatalf("SetRateLimited() error: %v", err)
	}
	if r.Blocked("https://provider-b.example") {
		t.Error("Blocked(B) = true, provider A cooldown leaked")
	}
	if err := r.Clear("https://provider-a.example"); err != nil {
		t.Fatalf("Clear() error: %v", err)
	}
	if r.Blocked("https://provider-a.example") {
		t.Error("Blocked(A) = true after Clear")
	}
	if r.Blocked("https://provider-b.example") {
		t.Error("Blocked(B) = true, should never have been set")
	}
}

func TestRemainingExpiry(t *testing.T) {
	dir := t.TempDir()
	r, err := LoadRateLimit(dir)
	if err != nil {
		t.Fatalf("LoadRateLimit() error: %v", err)
	}
	if err := r.SetRateLimited("https://lrclib.net", 30*time.Millisecond); err != nil {
		t.Fatalf("SetRateLimited() error: %v", err)
	}
	if d := r.Remaining("https://lrclib.net"); d <= 0 || d > time.Second {
		t.Errorf("Remaining() = %v, want (0, 1s]", d)
	}
	time.Sleep(50 * time.Millisecond)
	if r.Blocked("https://lrclib.net") {
		t.Error("Blocked() = true after cooldown expiry")
	}
	if d := r.Remaining("https://lrclib.net"); d > 0 {
		t.Errorf("Remaining() = %v after expiry, want <= 0", d)
	}
}

func TestLoadRateLimitExpiredEntriesPruned(t *testing.T) {
	dir := t.TempDir()
	r, err := LoadRateLimit(dir)
	if err != nil {
		t.Fatalf("LoadRateLimit() error: %v", err)
	}
	if err := r.SetRateLimited("https://expired.example", 10*time.Millisecond); err != nil {
		t.Fatalf("SetRateLimited() error: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	// reload: expired entry must not be loaded back
	r2, err := LoadRateLimit(dir)
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}
	if r2.Blocked("https://expired.example") {
		t.Error("Blocked() = true for expired entry after reload")
	}
}

func TestSetRateLimitedUnwritableDir(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	// #nosec G306 -- test fixture written to a private temp dir.
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	r, err := LoadRateLimit(blocker) // file path under a file → MkdirAll fails
	if err != nil {
		t.Fatalf("LoadRateLimit() error: %v", err)
	}
	if err := r.SetRateLimited("https://lrclib.net", time.Hour); err == nil {
		t.Error("SetRateLimited() error = nil, want error for unwritable dir")
	}
	// memory state still effective
	if !r.Blocked("https://lrclib.net") {
		t.Error("Blocked() = false, in-memory cooldown lost after failed write")
	}
}

func TestNoTmpResidue(t *testing.T) {
	dir := t.TempDir()
	r, err := LoadRateLimit(dir)
	if err != nil {
		t.Fatalf("LoadRateLimit() error: %v", err)
	}
	if err := r.SetRateLimited("https://lrclib.net", time.Minute); err != nil {
		t.Fatalf("SetRateLimited() error: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, rateLimitFileName+".tmp"))
	if len(matches) != 0 {
		t.Errorf(".tmp residue: %v", matches)
	}
}
