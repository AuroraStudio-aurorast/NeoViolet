package ui

import (
	"net/url"
	"strings"
	"unicode"
)

// isValidAudioSource accepts existing local audio files and direct HTTP(S)
// URLs. Remote format detection and network errors remain the player's job.
func isValidAudioSource(source string) bool {
	if strings.Contains(source, "://") {
		if strings.IndexFunc(source, unicode.IsSpace) >= 0 {
			return false
		}
		u, err := url.Parse(source)
		if err != nil || u.Opaque != "" || u.Hostname() == "" {
			return false
		}
		return strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")
	}
	return isValidAudioPath(source)
}
