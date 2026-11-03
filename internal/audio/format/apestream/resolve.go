package apestream

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// findApeCLI locates the apecli helper binary and reports candidates that exist
// but cannot be used. Search order:
//  1. NEOVIOLET_APECLI env var
//  2. Next to the running executable
//  3. The dev build dir (tools/apecli/target/release)
//  4. System PATH
//
// Candidates are passed to exec.LookPath without a platform extension, so
// LookPath appends PATHEXT on Windows (.exe) and enforces the executable bit on
// Unix.
func findApeCLI() (string, []string) {
	binary, problems := findApeCLIIn(os.Getenv("NEOVIOLET_APECLI"), apecliCandidates())
	for _, problem := range problems {
		logger.Warn("ape: unusable apecli candidate", "reason", problem)
	}
	return binary, problems
}

// apecliCandidates lists the automatic search locations, most specific first.
func apecliCandidates() []string {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "apecli"),
			filepath.Join(dir, "tools", "apecli", "target", "release", "apecli"),
		)
	}
	return append(candidates, "apecli")
}

// findApeCLIIn resolves the helper from an explicit override (the
// NEOVIOLET_APECLI value, empty when unset) and a list of candidate paths. An
// override that cannot be used does not disable the search: it is reported and
// the automatic candidates are still tried, so a stale env var degrades to a
// warning instead of losing the helper bundled next to the binary.
func findApeCLIIn(override string, candidates []string) (string, []string) {
	var problems []string

	if override != "" {
		path, err := exec.LookPath(override)
		if err == nil {
			return path, nil
		}
		problems = append(problems, fmt.Sprintf("NEOVIOLET_APECLI=%s was ignored: %v", override, err))
	}

	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err == nil {
			return path, problems
		}
		if unusable(err) {
			problems = append(problems, fmt.Sprintf("ignoring apecli at %s: %v", candidate, err))
		}
	}

	return "", problems
}

// unusable reports whether a failed lookup means "this file is here but cannot
// be run" (missing executable bit, a directory, …) rather than "there is no
// such file". Only the former is worth reporting: most of the search order is
// expected to be absent. exec.ErrDot is filtered out as well — refusing a
// helper found through a relative PATH entry is Go's default, not a broken
// install.
func unusable(err error) bool {
	switch {
	case err == nil,
		errors.Is(err, exec.ErrNotFound),
		errors.Is(err, fs.ErrNotExist),
		errors.Is(err, exec.ErrDot):
		return false
	}
	return true
}
