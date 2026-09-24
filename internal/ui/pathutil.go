package ui

import (
	"os"
	"path/filepath"
	"strings"
)

// pathSeparators is the set of bytes that separate path components on this
// platform. Windows reads both a backslash and a slash; elsewhere a backslash is
// an ordinary character in a file name, so "/" is the only separator. The first
// byte is the one a completed directory gets when the user has not typed a
// separator yet.
//
// It is a variable rather than a constant so tests can pin the Windows reading
// on a host whose separator is "/": the CI test job runs on Linux and macOS
// only, and the Windows job builds without running any test.
var pathSeparators = hostPathSeparators()

// hostPathSeparators asks the platform whether a backslash separates path
// components: it does on Windows, nowhere else.
func hostPathSeparators() string {
	if os.IsPathSeparator('\\') {
		return `\/`
	}
	return "/"
}

// isPathSep reports whether c separates path components on this platform.
func isPathSep(c byte) bool {
	return strings.IndexByte(pathSeparators, c) >= 0
}

// endsWithPathSep reports whether s stops at a directory boundary, which is what
// tells a typed prefix it already names a directory instead of a fragment.
func endsWithPathSep(s string) bool {
	return s != "" && isPathSep(s[len(s)-1])
}

// pathSep returns the separator to write after a completed directory: the last
// one the prefix already carries, so a Windows path keeps the style it went in
// with instead of coming back half-converted, or this platform's default when
// the prefix names no directory yet.
func pathSep(prefix string) string {
	for i := len(prefix) - 1; i >= 0; i-- {
		if isPathSep(prefix[i]) {
			return prefix[i : i+1]
		}
	}
	return pathSeparators[:1]
}

// expandTilde replaces a leading "~" or "~<separator>" with the user's home
// directory. Both of Windows' separators start an expanded path there, so "~\x"
// works as well as "~/x"; everywhere else a backslash is an ordinary character
// and only "~/" is expanded. Everything else is returned unchanged: "~user" and
// an embedded "~" are not expanded, matching what the shell does for the leading
// form only.
func expandTilde(path string) string {
	if path != "~" && (len(path) <= 1 || path[0] != '~' || !isPathSep(path[1])) {
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
