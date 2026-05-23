package ui

import (
	"image"
	"testing"
	"time"

	"github.com/gopxl/beep/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

// mockPlayer implements audio.AudioPlayer for testing.
type mockPlayer struct {
	path        string
	playing     bool
	volume      float64
	position    time.Duration
	duration    time.Duration
	seekCalled  bool
	lastSeekPos time.Duration
	playCalled  bool
	pauseCalled bool
	closeCalled bool
}

func (m *mockPlayer) Open(path string) error { return nil }
func (m *mockPlayer) Play() error            { m.playing = true; m.playCalled = true; return nil }
func (m *mockPlayer) Pause()                 { m.playing = false; m.pauseCalled = true }
func (m *mockPlayer) Stop()                  { m.playing = false }
func (m *mockPlayer) Toggle()                { m.playing = !m.playing }
func (m *mockPlayer) Seek(pos time.Duration) error {
	m.seekCalled = true
	m.lastSeekPos = pos
	m.position = pos
	return nil
}
func (m *mockPlayer) SetVolume(vol float64)   { m.volume = vol }
func (m *mockPlayer) Volume() float64         { return m.volume }
func (m *mockPlayer) IsPlaying() bool         { return m.playing }
func (m *mockPlayer) Duration() time.Duration { return m.duration }
func (m *mockPlayer) Position() time.Duration { return m.position }
func (m *mockPlayer) Close() error            { m.closeCalled = true; return nil }
func (m *mockPlayer) Format() beep.Format {
	return beep.Format{SampleRate: 44100, NumChannels: 2, Precision: 4}
}
func (m *mockPlayer) Path() string            { return m.path }
func (m *mockPlayer) Title() string           { return "" }
func (m *mockPlayer) Artist() string          { return "" }
func (m *mockPlayer) CoverImage() image.Image { return nil }

func TestAudioState_UpdatePosition(t *testing.T) {
	mp := &mockPlayer{position: 30 * time.Second, duration: 120 * time.Second}
	a := &AudioState{Player: mp}

	a.UpdatePosition()

	if a.Elapsed != 30*time.Second {
		t.Errorf("Elapsed = %v, want 30s", a.Elapsed)
	}
	if a.Progress != 0.25 {
		t.Errorf("Progress = %v, want 0.25", a.Progress)
	}
}

func TestAudioState_UpdatePosition_NilPlayer(t *testing.T) {
	a := &AudioState{}
	a.UpdatePosition() // should not panic
}

func TestAudioState_UpdatePosition_Clamp(t *testing.T) {
	mp := &mockPlayer{position: 200 * time.Second, duration: 120 * time.Second}
	a := &AudioState{Player: mp}

	a.UpdatePosition()

	if a.Progress != 1.0 {
		t.Errorf("Progress = %v, want 1.0", a.Progress)
	}
}

func TestAudioState_UpdatePosition_Negative(t *testing.T) {
	mp := &mockPlayer{position: -5 * time.Second, duration: 120 * time.Second}
	a := &AudioState{Player: mp}

	a.UpdatePosition()

	if a.Progress != 0 {
		t.Errorf("Progress = %v, want 0", a.Progress)
	}
}

func TestAudioState_SeekRelative(t *testing.T) {
	mp := &mockPlayer{position: 60 * time.Second, duration: 120 * time.Second}
	a := &AudioState{Player: mp, Duration: 120 * time.Second}

	got := a.SeekRelative(30 * time.Second)

	if got != 90*time.Second {
		t.Errorf("SeekRelative = %v, want 90s", got)
	}
	if !mp.seekCalled || mp.lastSeekPos != 90*time.Second {
		t.Errorf("Player.Seek not called with 90s")
	}
}

func TestAudioState_SeekRelative_ClampNeg(t *testing.T) {
	mp := &mockPlayer{position: 10 * time.Second, duration: 120 * time.Second}
	a := &AudioState{Player: mp, Duration: 120 * time.Second}

	got := a.SeekRelative(-30 * time.Second)

	if got != 0 {
		t.Errorf("SeekRelative = %v, want 0", got)
	}
}

func TestAudioState_SeekRelative_ClampOver(t *testing.T) {
	mp := &mockPlayer{position: 110 * time.Second, duration: 120 * time.Second}
	a := &AudioState{Player: mp, Duration: 120 * time.Second}

	got := a.SeekRelative(30 * time.Second)

	if got != 120*time.Second {
		t.Errorf("SeekRelative = %v, want 120s", got)
	}
}

func TestAudioState_SeekRelative_NilPlayer(t *testing.T) {
	a := &AudioState{}
	got := a.SeekRelative(10 * time.Second)
	if got != 0 {
		t.Errorf("SeekRelative = %v, want 0", got)
	}
}

func TestAudioState_SeekPlayer(t *testing.T) {
	mp := &mockPlayer{}
	a := &AudioState{Player: mp}

	if err := a.SeekPlayer(30 * time.Second); err != nil {
		t.Fatalf("SeekPlayer error: %v", err)
	}
	if !mp.seekCalled || mp.lastSeekPos != 30*time.Second {
		t.Error("Player.Seek not called")
	}
}

func TestAudioState_SeekPlayer_NilPlayer(t *testing.T) {
	a := &AudioState{}
	if err := a.SeekPlayer(30 * time.Second); err != nil {
		t.Fatalf("SeekPlayer with nil player should not error: %v", err)
	}
}

func TestAudioState_TogglePlayback(t *testing.T) {
	mp := &mockPlayer{playing: true}
	a := &AudioState{Player: mp}

	a.TogglePlayback()
	if mp.playing || a.IsPlaying {
		t.Error("expected playing=false after toggle")
	}

	a.TogglePlayback()
	if !mp.playing || !a.IsPlaying {
		t.Error("expected playing=true after toggle again")
	}
}

func TestAudioState_TogglePlayback_NilPlayer(t *testing.T) {
	a := &AudioState{}
	a.TogglePlayback() // should not panic
}

func TestAudioState_Close(t *testing.T) {
	mp := &mockPlayer{}
	a := &AudioState{Player: mp}

	a.Close()
	if !mp.closeCalled {
		t.Error("Player.Close not called")
	}
	if a.Player != nil {
		t.Error("Player should be nil after Close")
	}
}

func TestAudioState_UpdateLyricIndex(t *testing.T) {
	ld := &lyrics.LyricsData{
		Lines: []lyrics.LyricLine{
			{Time: 1000 * time.Millisecond, Text: "line one"},
			{Time: 3000 * time.Millisecond, Text: "line two"},
		},
	}
	a := &AudioState{Lyrics: ld, Elapsed: 2000 * time.Millisecond}

	a.UpdateLyricIndex()

	if a.LyricIndex != 0 {
		t.Errorf("LyricIndex = %d, want 0", a.LyricIndex)
	}
}

func TestAudioState_UpdateLyricIndex_Nil(t *testing.T) {
	a := &AudioState{}
	a.UpdateLyricIndex() // should not panic

	if a.LyricIndex != -1 {
		t.Errorf("LyricIndex = %d, want -1", a.LyricIndex)
	}
}

func TestAudioState_AdvanceLyricScroll_Nil(t *testing.T) {
	a := &AudioState{}
	a.AdvanceLyricScroll(6, 80) // should not panic
}

func TestAudioState_AdjustVolumeNoPlayer(t *testing.T) {
	a := &AudioState{Volume: 0.5}

	a.AdjustVolume(0.3)
	if a.Volume != 0.8 {
		t.Errorf("Volume = %f, want 0.8", a.Volume)
	}
}

func TestAudioState_SetVolumeNoPlayer(t *testing.T) {
	a := &AudioState{Volume: 0.5}

	a.SetVolume(0.2)
	if a.Volume != 0.2 {
		t.Errorf("Volume = %f, want 0.2", a.Volume)
	}
}

func TestErrorState_Tick_Empty(t *testing.T) {
	e := &ErrorState{Visible: true}
	e.Tick() // Timer = 0, no decrement
	if !e.Visible {
		t.Error("Visible should remain true when Timer is 0")
	}
}
