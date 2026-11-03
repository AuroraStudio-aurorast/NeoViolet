package apestream

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// helperFileName is the file name exec.LookPath resolves to on this platform.
func helperFileName() string {
	if runtime.GOOS == "windows" {
		return "apecli.exe"
	}
	return "apecli"
}

// writeHelper creates a file exec.LookPath accepts as apecli: the platform's
// binary name, with the executable bit set on Unix.
func writeHelper(t *testing.T, path string) {
	t.Helper()
	// #nosec G306 -- the executable bit is the property under test: exec.LookPath
	// ignores a helper it cannot run.
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write helper %s: %v", path, err)
	}
}

func TestFindApeCLIInOverride(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, helperFileName())
	writeHelper(t, want)

	// The first candidate does not exist: an override that resolves wins before
	// any candidate is looked at.
	got, problems := findApeCLIIn(want, []string{filepath.Join(t.TempDir(), "apecli")})
	if got != want {
		t.Errorf("findApeCLIIn() = %q, want %q", got, want)
	}
	if len(problems) != 0 {
		t.Errorf("findApeCLIIn() reported %q, want none", problems)
	}
}

// findApeCLI builds its candidates without a platform extension
// (filepath.Join(dir, "apecli")), so resolution has to come from
// exec.LookPath: PATHEXT on Windows, the executable bit on Unix.
func TestFindApeCLIInCandidateNeedsLookPath(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, helperFileName())
	writeHelper(t, want)

	got, problems := findApeCLIIn("", []string{filepath.Join(dir, "apecli")})
	if got != want {
		t.Errorf("findApeCLIIn() = %q, want %q", got, want)
	}
	if len(problems) != 0 {
		t.Errorf("findApeCLIIn() reported %q, want none", problems)
	}
}

// A stale NEOVIOLET_APECLI must not disable the search: the bundled helper is
// still found, and the override is reported.
func TestFindApeCLIInOverrideFallsBack(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, helperFileName())
	writeHelper(t, want)

	override := filepath.Join(t.TempDir(), "gone")
	got, problems := findApeCLIIn(override, []string{filepath.Join(dir, "apecli")})
	if got != want {
		t.Errorf("findApeCLIIn() = %q, want %q", got, want)
	}
	if len(problems) != 1 {
		t.Fatalf("findApeCLIIn() reported %q, want exactly the override", problems)
	}
	if !strings.Contains(problems[0], "NEOVIOLET_APECLI") || !strings.Contains(problems[0], override) {
		t.Errorf("problem %q should name NEOVIOLET_APECLI and %q", problems[0], override)
	}
}

// A candidate that exists but cannot be executed is reported and the search
// continues with the next one.
func TestFindApeCLIInSkipsUnusableCandidate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Chmod cannot clear an executable bit on Windows")
	}

	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked", helperFileName())
	if err := os.MkdirAll(filepath.Dir(blocked), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(blocked, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatalf("write blocked helper: %v", err)
	}

	want := filepath.Join(dir, helperFileName())
	writeHelper(t, want)

	got, problems := findApeCLIIn("", []string{filepath.Join(dir, "blocked", "apecli"), filepath.Join(dir, "apecli")})
	if got != want {
		t.Errorf("findApeCLIIn() = %q, want %q", got, want)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], blocked) {
		t.Errorf("findApeCLIIn() reported %q, want one problem naming %q", problems, blocked)
	}
}

// With nothing but an unusable candidate there is no binary, but the reason is
// available to the caller.
func TestFindApeCLIInUnusableOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Chmod cannot clear an executable bit on Windows")
	}

	dir := t.TempDir()
	blocked := filepath.Join(dir, helperFileName())
	if err := os.WriteFile(blocked, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatalf("write blocked helper: %v", err)
	}

	got, problems := findApeCLIIn("", []string{filepath.Join(dir, "apecli")})
	if got != "" {
		t.Errorf("findApeCLIIn() = %q, want no binary", got)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], blocked) {
		t.Errorf("findApeCLIIn() reported %q, want one problem naming %q", problems, blocked)
	}
}

// A candidate that is simply absent is the normal case and must stay quiet.
func TestFindApeCLIInMissingCandidatesAreQuiet(t *testing.T) {
	got, problems := findApeCLIIn("", []string{filepath.Join(t.TempDir(), "apecli")})
	if got != "" {
		t.Errorf("findApeCLIIn() = %q, want no binary", got)
	}
	if len(problems) != 0 {
		t.Errorf("findApeCLIIn() reported %q, want none", problems)
	}
}

func TestNoBackendError(t *testing.T) {
	const hint = "install ffmpeg or mac, or build apecli"

	err := noBackendError(nil)
	if got, want := err.Error(), "ape: no decoder backend found — "+hint+" (requires cargo)"; got != want {
		t.Errorf("noBackendError(nil) = %q, want %q", got, want)
	}

	problem := "ignoring apecli at /opt/apecli: permission denied"
	err = noBackendError([]string{problem})
	if !strings.Contains(err.Error(), problem) {
		t.Errorf("noBackendError() = %q, want it to mention %q", err, problem)
	}
	if !strings.Contains(err.Error(), hint) {
		t.Errorf("noBackendError() = %q, want it to keep the install hint", err)
	}
}
