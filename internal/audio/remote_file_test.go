package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

// Remote decode tests exercise the real HTTP/decoder chain without a speaker device.
func remoteTestPlayer(t *testing.T) *Player {
	t.Helper()
	initialized := false
	initSpeakerOnce.Do(func() { initialized = true; speakerSampleRate = 44100; initSpeakerErr = nil })
	if initialized {
		t.Cleanup(func() { initSpeakerOnce = sync.Once{}; speakerSampleRate = 0; initSpeakerErr = nil })
	}
	player := NewPlayer()
	t.Cleanup(func() { _ = player.Close() })
	return player
}

func remoteWAV(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	data.WriteString("RIFF")
	values := []any{uint32(40), []byte("WAVEfmt "), uint32(16), uint16(1), uint16(1), uint32(44100), uint32(88200), uint16(2), uint16(16), []byte("data"), uint32(4), int16(4096), int16(-4096)}
	for _, value := range values {
		if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	return data.Bytes()
}

func TestRemoteWAVWaitsForCompleteDownloadAndPreservesSplitSamples(t *testing.T) {
	player := remoteTestPlayer(t)
	data := remoteWAV(t)
	headerSent, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(data[:44])
		w.(http.Flusher).Flush()
		close(headerSent)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		for _, b := range data[44:] {
			_, _ = w.Write([]byte{b})
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opened := make(chan error, 1)
	go func() { opened <- player.OpenContext(ctx, server.URL+"/track.wav") }()
	<-headerSent
	select {
	case err := <-opened:
		t.Fatalf("Open returned before audio download completed: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	unblock()
	select {
	case err := <-opened:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Open did not complete after EOF")
	}
	samples := make([][2]float64, 2)
	n, _ := player.streamer.Stream(samples)
	if n != 2 || samples[0][0] != 0.125 || samples[1][0] != -0.125 {
		t.Fatalf("lost or corrupted PCM: %d frames, %v", n, samples)
	}
	if err := player.Seek(0); err != nil {
		t.Fatal(err)
	}
	player.Pause()
	player.SetVolume(0.4)
	if err := player.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteAudioDetectsFormatBehindScriptAndWrongExtension(t *testing.T) {
	for _, fixture := range []struct{ name, mime string }{
		{"test_wav_no_metadata.wav", "audio/wav"},
		{"test_mp3_with_metadata.mp3", "audio/mpeg"},
		{"test_flac_with_metadata.flac", "audio/flac"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/" + fixture.name)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirect.php" {
					http.Redirect(w, r, "/actual?token=signed-value", http.StatusFound)
					return
				}
				if r.URL.Query().Get("token") != "signed-value" {
					t.Error("lost signed query")
				}
				w.Header().Set("Content-Type", fixture.mime)
				_, _ = w.Write(data)
			}))
			defer server.Close()
			for _, path := range []string{"/download.php", "/wrong.mp3", "/no-extension", "/redirect.php"} {
				player := remoteTestPlayer(t)
				if err := player.Open(server.URL + path + "?token=signed-value"); err != nil {
					t.Fatalf("%s: %v", path, err)
				}
				samples := make([][2]float64, 8)
				if n, _ := player.streamer.Stream(samples); n == 0 {
					t.Fatal("no decoded samples")
				}
				if player.Duration() <= 0 {
					t.Fatal("no decoded duration")
				}
				if err := player.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestRemoteCachedFileSurvivesRequestCancellation(t *testing.T) {
	data := remoteWAV(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) }))
	defer server.Close()
	player := remoteTestPlayer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := player.OpenContext(ctx, server.URL+"/audio.wav"); err != nil {
		t.Fatal(err)
	}
	cancel()
	samples := make([][2]float64, 2)
	if n, _ := player.streamer.Stream(samples); n != 2 {
		t.Fatalf("complete cached audio cancelled after open: %d frames", n)
	}
	cacheDir := player.file.(*remoteReadSeeker).tempDir
	if err := player.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Fatalf("remote cache survived Close: %v", err)
	}
}

func TestRemoteRequestCancellationRemainsDetectable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	player := NewPlayer()
	err := player.OpenContext(ctx, "https://audio.example.test/a.mp3?token=private")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Open = %v", err)
	}
}
func TestRemoteCacheRejectsOversizedBodies(t *testing.T) {
	for _, known := range []bool{true, false} {
		size := int64(-1)
		if known {
			size = 9
		}
		response := &http.Response{Body: io.NopCloser(bytes.NewReader([]byte("123456789"))), ContentLength: size}
		reader, err := newRemoteReadSeeker(response, 8)
		if err != nil {
			if !known {
				t.Fatal(err)
			}
			continue
		}
		defer func() { _ = reader.Close() }()
		if err := reader.waitDownload(context.Background()); err == nil {
			t.Fatal("oversized response was cached")
		}
		if reader.written > 8 {
			t.Fatalf("cached %d bytes beyond limit", reader.written)
		}
	}
}
