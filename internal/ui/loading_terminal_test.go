package ui

import (
	"io"
	"os"
	"strings"
	"testing"
)

func captureLoadingTerminalWrites(t *testing.T, action func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	stdout := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = stdout }()
	action()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
}

func TestLoadingResultsEraseOnlyTheStartupLine(t *testing.T) {
	for _, startup := range []bool{true, false} {
		for _, failed := range []bool{false, true} {
			m := setupModel()
			m.Config.Accent.AutoAccent = boolPtr(false)
			m.Loading, m.switchingTrack = true, !startup
			output := captureLoadingTerminalWrites(t, func() {
				if failed {
					handleError(m, ErrorMsg{Message: "cannot open audio", Timer: 30})
				} else {
					handleAudioLoaded(m, AudioLoadedMsg{Player: &mockPlayer{}, Path: "/track.flac"})
				}
			})
			erases := strings.Contains(output, "\x1b[2K\r")
			if erases != startup {
				t.Errorf("startup=%v failed=%v: direct line erase=%v, want %v; output %q",
					startup, failed, erases, startup, output)
			}
			if !strings.Contains(output, "\x1b]9;4;0;0\a") {
				t.Error("loading result no longer clears the terminal progress indicator")
			}
		}
	}
}

func TestPlaybackErrorDoesNotEraseAnActiveHelpbar(t *testing.T) {
	m := setupModel()
	output := captureLoadingTerminalWrites(t, func() {
		handleError(m, ErrorMsg{Message: "playback failed", Timer: 30})
	})
	if strings.Contains(output, "\x1b[2K") {
		t.Fatalf("runtime error erases a renderer-owned row: %q", output)
	}
}
