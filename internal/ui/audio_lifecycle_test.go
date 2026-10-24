package ui

import (
	"path/filepath"
	"testing"
)

func TestStaleAudioLoadedClosesPlayer(t *testing.T) {
	p := &mockPlayer{}
	m := &Model{loadGeneration: 2, Audio: &AudioState{}}
	handleAudioLoaded(m, AudioLoadedMsg{Player: p, Generation: 1})
	if !p.closeCalled {
		t.Fatal("stale player's resources retained")
	}
	if m.Audio.Player != nil {
		t.Fatal("stale result replaced current player")
	}
}
func TestLoadGenerationCaptured(t *testing.T) {
	m := setupModel()
	_, cmd := handleLoadTrack(m, LoadTrackMsg{Path: filepath.Join(t.TempDir(), "missing.mid")})
	generation := m.loadGeneration
	m.loadGeneration++
	result, ok := cmd().(ErrorMsg)
	if !ok || result.Generation != generation {
		t.Fatalf("load command read later generation: %+v", result)
	}
}

func TestUIPlaybackStateTracksSynth(t *testing.T) {
	p := &mockPlayer{playing: false}
	a := AudioState{Player: p, IsPlaying: true}
	a.UpdatePosition()
	if a.IsPlaying {
		t.Fatal("UI retained stale playing state")
	}
}
