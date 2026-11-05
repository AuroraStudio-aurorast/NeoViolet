package audio

import "testing"

func TestIsURLAcceptsHTTPSSchemesCaseInsensitively(t *testing.T) {
	for _, value := range []string{
		"http://audio.example.test/track.mp3",
		"https://audio.example.test/track.mp3",
		"HTTP://audio.example.test/track.mp3",
		"HtTpS://audio.example.test/track.mp3",
	} {
		if !isURL(value) {
			t.Errorf("isURL(%q) = false, want true", value)
		}
	}
	for _, value := range []string{"/music/track.mp3", "ftp://audio.example.test/track.mp3"} {
		if isURL(value) {
			t.Errorf("isURL(%q) = true, want false", value)
		}
	}
}
