package audio

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRemoteReaderCloseWakesBlockedRead(t *testing.T) {
	body, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	reader, err := newRemoteReadSeeker(&http.Response{Body: body, ContentLength: -1}, maxRemoteAudioSize)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { _, err := reader.Read(make([]byte, 1)); finished <- err }()
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("Read after close = %v, want closed pipe", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not wake blocked Read")
	}
}

func TestRemoteRequestErrorRedactsSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()
	player := NewPlayer()
	err := player.Open(server.URL + "/audio.mp3?token=private-secret")
	if err == nil {
		t.Fatal("expected failed request")
	}
	if strings.Contains(err.Error(), "private-secret") {
		t.Fatalf("request error exposed signed URL: %v", err)
	}
}

func TestRemoteReaderCloseWakesSeekEnd(t *testing.T) {
	body, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	reader, err := newRemoteReadSeeker(&http.Response{Body: body, ContentLength: -1}, maxRemoteAudioSize)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { _, err := reader.Seek(0, io.SeekEnd); finished <- err }()
	_ = reader.Close()
	select {
	case err := <-finished:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("Seek after close = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not wake SeekEnd")
	}
}

func TestRemoteConnectionTimesOutWhenBodyStops(t *testing.T) {
	conn, peer := net.Pipe()
	defer func() { _ = conn.Close() }()
	defer func() { _ = peer.Close() }()
	reader := &remoteConn{Conn: conn, idle: 20 * time.Millisecond}
	_, err := reader.Read(make([]byte, 1))
	var timeout net.Error
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatalf("stalled read = %v, want timeout", err)
	}
}

func TestRemotePlayerCloseCancelsStalledOpen(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)
	player := NewPlayer()
	opened := make(chan error, 1)
	go func() { opened <- player.Open(server.URL + "/audio.mp3") }()
	<-started
	closed := make(chan error, 1)
	go func() { closed <- player.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close remained blocked on Open")
	}
	select {
	case err := <-opened:
		if err == nil {
			t.Fatal("cancelled Open succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("Open did not stop after Close")
	}
}

func TestRemoteRedirectErrorRedactsNestedURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/bad%zz.mp3?token=redirect-secret")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	player := NewPlayer()
	err := player.Open(server.URL + "/audio.mp3?token=original-secret")
	if err == nil {
		t.Fatal("malformed redirect was accepted")
	}
	for _, secret := range []string{"redirect-secret", "original-secret"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("request error exposes %q: %v", secret, err)
		}
	}
}
