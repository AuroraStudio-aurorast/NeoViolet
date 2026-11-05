package audio

import (
	"net/http"
	"testing"
	"time"
)

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

func TestPlayerPathRedactsRemoteCredentialsQueryAndFragment(t *testing.T) {
	p := NewPlayer()
	p.path = "https://user:password@audio.example.test/music/track.mp3?token=signed-value#chapter"
	if got, want := p.Path(), "https://audio.example.test/music/track.mp3"; got != want {
		t.Fatalf("Path() = %q, want redacted URL %q", got, want)
	}
}

func TestPlayerPathPreservesLocalFilePaths(t *testing.T) {
	p := NewPlayer()
	p.path = "/music/track.mp3"
	if got, want := p.Path(), p.path; got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestRemoteHTTPClientLimitsConnectionSetupWithoutBodyDeadline(t *testing.T) {
	client := newRemoteHTTPClient()
	if client.Timeout != 0 {
		t.Fatalf("Client.Timeout = %v, want no total request deadline", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport type = %T, want *http.Transport", client.Transport)
	}
	if got, want := transport.ResponseHeaderTimeout, 30*time.Second; got != want {
		t.Errorf("ResponseHeaderTimeout = %v, want %v", got, want)
	}
	if transport.TLSHandshakeTimeout <= 0 {
		t.Error("TLSHandshakeTimeout must be bounded")
	}
}
