package ui

import (
	"os"
	"path/filepath"
	"strings"
)

// expandTilde replaces a leading "~" or "~/" with the user's home directory.
// Everything else is returned unchanged: "~user" and an embedded "~" are not
// expanded, matching what the shell does for the leading form only.
func expandTilde(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}
