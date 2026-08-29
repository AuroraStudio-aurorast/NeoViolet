package ui

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"
)

// mockMediaCtl implements mediactl.Controller for testing the media-control
// message handlers without touching an OS media backend.
type mockMediaCtl struct {
	updates []mediactl.PlayState
	closed  bool
}

func (c *mockMediaCtl) Start() (<-chan mediactl.Command, error) { return nil, nil }
func (c *mockMediaCtl) Update(s mediactl.PlayState)             { c.updates = append(c.updates, s) }
func (c *mockMediaCtl) Close() error                            { c.closed = true; return nil }

func boolPtr(b bool) *bool { return &b }

// dispatcherModel dispatches msg through updateDispatcher and asserts that the
// result is the same *Model pointer we passed in.
func dispatcherModel(t *testing.T, m *Model, msg tea.Msg) (*Model, tea.Cmd) {
	t.Helper()
	got, cmd := updateDispatcher(m, msg)
	nm, ok := got.(*Model)
	if !ok {
		t.Fatalf("updateDispatcher returned %T, want *Model", got)
	}
	if nm != m {
		t.Fatal("updateDispatcher returned a different *Model instance")
	}
	return nm, cmd
}

func TestDispatcherErrorMsg(t *testing.T) {
	m := setupModel()
	m.Loading = true
	m.switchingTrack = true

	nm, cmd := dispatcherModel(t, m, ErrorMsg{Message: "boom", Timer: 120})
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if nm.Error == nil || nm.Error.Message != "boom" {
		t.Errorf("Error = %+v, want message 'boom'", nm.Error)
	}
	if nm.Error.Timer != 120 || !nm.Error.Visible {
		t.Errorf("Error timer/visible = %d/%v, want 120/true", nm.Error.Timer, nm.Error.Visible)
	}
	if nm.Loading || nm.switchingTrack {
		t.Errorf("Loading/switchingTrack = %v/%v, want false/false", nm.Loading, nm.switchingTrack)
	}
}

func TestDispatcherErrorMsg_StaleGeneration(t *testing.T) {
	m := setupModel()
	m.loadGeneration = 7
	m.Loading = true
	m.Error.Set("old", 60)

	nm, cmd := dispatcherModel(t, m, ErrorMsg{Message: "stale", Timer: 120, Generation: 3})
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if nm.Error.Message != "old" {
		t.Errorf("Error.Message = %q, want %q (stale error must be ignored)", nm.Error.Message, "old")
	}
	if !nm.Loading {
		t.Error("Loading should remain true for a stale error")
	}
}

func TestDispatcherVolumeMsg_Delta(t *testing.T) {
	m := setupModel()
	m.Audio.Volume = 0.5

	nm, cmd := dispatcherModel(t, m, VolumeMsg{Delta: 0.3})
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if nm.Audio.Volume != 0.8 {
		t.Errorf("Volume = %v, want 0.8", nm.Audio.Volume)
	}
}

func TestDispatcherVolumeMsg_Level(t *testing.T) {
	m := setupModel()
	m.Audio.Volume = 0.5

	nm, _ := dispatcherModel(t, m, VolumeMsg{Level: 0.2})
	if nm.Audio.Volume != 0.2 {
		t.Errorf("Volume = %v, want 0.2", nm.Audio.Volume)
	}
}

func TestDispatcherVolumeMsg_OutOfRange(t *testing.T) {
	m := setupModel()
	m.Audio.Volume = 0.5

	nm, _ := dispatcherModel(t, m, VolumeMsg{Level: 1.5})
	if nm.Audio.Volume != 0.5 {
		t.Errorf("Volume = %v, want unchanged 0.5", nm.Audio.Volume)
	}
}

func TestDispatcherSeekMsg_NilPlayer(t *testing.T) {
	m := setupModel()

	nm, cmd := dispatcherModel(t, m, SeekMsg{Position: 30 * time.Second})
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if nm.Audio.Player != nil {
		t.Error("Player should remain nil")
	}
}

func TestDispatcherSeekMsg_Relative(t *testing.T) {
	m := setupModel()
	mp := &mockPlayer{position: 60 * time.Second}
	m.Audio.Player = mp
	m.Audio.Duration = 120 * time.Second

	nm, _ := dispatcherModel(t, m, SeekMsg{Position: 30 * time.Second, Relative: true})
	if !mp.seekCalled || mp.lastSeekPos != 90*time.Second {
		t.Errorf("Seek called with %v, want 90s", mp.lastSeekPos)
	}
	if nm.Error.Visible {
		t.Errorf("unexpected error: %s", nm.Error.Message)
	}
}

func TestDispatcherSeekMsg_AbsoluteClamp(t *testing.T) {
	m := setupModel()
	mp := &mockPlayer{}
	m.Audio.Player = mp
	m.Audio.Duration = 120 * time.Second

	nm, _ := dispatcherModel(t, m, SeekMsg{Position: 200 * time.Second})
	if mp.lastSeekPos != 120*time.Second {
		t.Errorf("Seek called with %v, want clamped 120s", mp.lastSeekPos)
	}
	if nm.Error.Visible {
		t.Errorf("unexpected error: %s", nm.Error.Message)
	}
}

func TestDispatcherAccentApplyMsg(t *testing.T) {
	m := setupModel()
	ac := &accent.Accent{}

	nm, cmd := dispatcherModel(t, m, AccentApplyMsg{Accent: ac})
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if nm.Accent != ac {
		t.Errorf("Accent = %v, want %v", nm.Accent, ac)
	}
}

func TestDispatcherMediaCtlReadyMsg(t *testing.T) {
	m := setupModel()
	ctrl := &mockMediaCtl{}

	nm, cmd := dispatcherModel(t, m, MediaCtlReadyMsg{Controller: ctrl})
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if nm.MediaCtl != ctrl {
		t.Errorf("MediaCtl = %v, want %v", nm.MediaCtl, ctrl)
	}
}

func TestDispatcherMediaCtlCmd(t *testing.T) {
	t.Run("playpause", func(t *testing.T) {
		m := setupModel()
		mp := &mockPlayer{playing: false}
		m.Audio.Player = mp
		m.MediaCtl = &mockMediaCtl{}

		nm, cmd := dispatcherModel(t, m, MediaCtlMsg{Command: mediactl.Command{Type: mediactl.CmdPlayPause}})
		if cmd != nil {
			t.Errorf("cmd = %v, want nil", cmd)
		}
		if !nm.Audio.IsPlaying || !mp.playing {
			t.Error("expected playing after CmdPlayPause")
		}
	})

	t.Run("play", func(t *testing.T) {
		m := setupModel()
		mp := &mockPlayer{playing: false}
		m.Audio.Player = mp

		nm, _ := dispatcherModel(t, m, MediaCtlMsg{Command: mediactl.Command{Type: mediactl.CmdPlay}})
		if !nm.Audio.IsPlaying || !mp.playing {
			t.Error("expected playing after CmdPlay")
		}
	})

	t.Run("pause", func(t *testing.T) {
		m := setupModel()
		mp := &mockPlayer{playing: true}
		m.Audio.Player = mp
		m.Audio.IsPlaying = true

		nm, _ := dispatcherModel(t, m, MediaCtlMsg{Command: mediactl.Command{Type: mediactl.CmdPause}})
		if nm.Audio.IsPlaying || mp.playing {
			t.Error("expected paused after CmdPause")
		}
	})

	t.Run("stop", func(t *testing.T) {
		m := setupModel()
		mp := &mockPlayer{playing: true, position: 40 * time.Second}
		m.Audio.Player = mp
		m.Audio.IsPlaying = true

		nm, _ := dispatcherModel(t, m, MediaCtlMsg{Command: mediactl.Command{Type: mediactl.CmdStop}})
		if nm.Audio.IsPlaying {
			t.Error("expected not playing after CmdStop")
		}
		if !mp.seekCalled || mp.lastSeekPos != 0 {
			t.Errorf("expected seek to 0 after CmdStop, got %v", mp.lastSeekPos)
		}
	})

	t.Run("next", func(t *testing.T) {
		m := setupModel()
		mp := &mockPlayer{position: 60 * time.Second}
		m.Audio.Player = mp
		m.Audio.Duration = 200 * time.Second

		dispatcherModel(t, m, MediaCtlMsg{Command: mediactl.Command{Type: mediactl.CmdNext}})
		if mp.lastSeekPos != 70*time.Second {
			t.Errorf("Seek called with %v, want 70s", mp.lastSeekPos)
		}
	})

	t.Run("prev", func(t *testing.T) {
		m := setupModel()
		mp := &mockPlayer{position: 60 * time.Second}
		m.Audio.Player = mp
		m.Audio.Duration = 200 * time.Second

		dispatcherModel(t, m, MediaCtlMsg{Command: mediactl.Command{Type: mediactl.CmdPrev}})
		if mp.lastSeekPos != 50*time.Second {
			t.Errorf("Seek called with %v, want 50s", mp.lastSeekPos)
		}
	})

	t.Run("seek", func(t *testing.T) {
		m := setupModel()
		mp := &mockPlayer{position: 60 * time.Second}
		m.Audio.Player = mp
		m.Audio.Duration = 200 * time.Second
		ctrl := &mockMediaCtl{}
		m.MediaCtl = ctrl

		dispatcherModel(t, m, MediaCtlMsg{Command: mediactl.Command{Type: mediactl.CmdSeek, Value: 5_000_000}})
		if mp.lastSeekPos != 65*time.Second {
			t.Errorf("Seek called with %v, want 65s", mp.lastSeekPos)
		}
		if len(ctrl.updates) != 1 {
			t.Errorf("MediaCtl.Update calls = %d, want 1", len(ctrl.updates))
		}
	})

	t.Run("setposition", func(t *testing.T) {
		m := setupModel()
		mp := &mockPlayer{}
		m.Audio.Player = mp
		ctrl := &mockMediaCtl{}
		m.MediaCtl = ctrl

		dispatcherModel(t, m, MediaCtlMsg{Command: mediactl.Command{Type: mediactl.CmdSetPosition, Value: 30_000_000}})
		if mp.lastSeekPos != 30*time.Second {
			t.Errorf("Seek called with %v, want 30s", mp.lastSeekPos)
		}
		if len(ctrl.updates) != 1 {
			t.Errorf("MediaCtl.Update calls = %d, want 1", len(ctrl.updates))
		}
	})

	t.Run("setvolume", func(t *testing.T) {
		m := setupModel()
		mp := &mockPlayer{}
		m.Audio.Player = mp

		nm, _ := dispatcherModel(t, m, MediaCtlMsg{Command: mediactl.Command{Type: mediactl.CmdSetVolume, Volume: 0.7}})
		if nm.Audio.Volume != 0.7 {
			t.Errorf("Volume = %v, want 0.7", nm.Audio.Volume)
		}
		if mp.volume != 0.7 {
			t.Errorf("Player volume = %v, want 0.7", mp.volume)
		}
	})
}

func TestDispatcherLoadTrackMsg(t *testing.T) {
	t.Run("preservesLyricFormat", func(t *testing.T) {
		m := setupModel()
		m.Audio.Volume = 0.5
		m.Audio.Lyrics = &lyrics.LyricsData{Format: "lrc"}

		nm, cmd := dispatcherModel(t, m, LoadTrackMsg{Path: "/music/next.mp3"})
		if cmd == nil {
			t.Fatal("cmd = nil, want a load cmd")
		}
		if nm.loadGeneration != 1 {
			t.Errorf("loadGeneration = %d, want 1", nm.loadGeneration)
		}
		if nm.preferredLyricFormat != "lrc" {
			t.Errorf("preferredLyricFormat = %q, want %q", nm.preferredLyricFormat, "lrc")
		}
		if nm.Audio.Player != nil {
			t.Error("Player should be nil after close")
		}
		if nm.Audio.Volume != 0.5 {
			t.Errorf("Volume = %v, want preserved 0.5", nm.Audio.Volume)
		}
		if !nm.Loading || nm.loadingTick != 0 || !nm.switchingTrack {
			t.Errorf("Loading/loadingTick/switchingTrack = %v/%d/%v, want true/0/true", nm.Loading, nm.loadingTick, nm.switchingTrack)
		}
		if nm.Accent != nil {
			t.Error("Accent should be reset to nil")
		}
	})

	t.Run("noPreviousLyrics", func(t *testing.T) {
		m := setupModel()
		m.Audio.Lyrics = nil

		nm, cmd := dispatcherModel(t, m, LoadTrackMsg{Path: "/music/next.mp3"})
		if cmd == nil {
			t.Fatal("cmd = nil, want a load cmd")
		}
		if nm.preferredLyricFormat != "" {
			t.Errorf("preferredLyricFormat = %q, want empty", nm.preferredLyricFormat)
		}
	})
}

func TestDispatcherWindowSizeMsg(t *testing.T) {
	t.Run("cappedTabWidth", func(t *testing.T) {
		m := setupModel()
		nm, cmd := dispatcherModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
		if cmd != nil {
			t.Errorf("cmd = %v, want nil", cmd)
		}
		if nm.UI.Width != 100 || nm.UI.Height != 30 {
			t.Errorf("Width/Height = %d/%d, want 100/30", nm.UI.Width, nm.UI.Height)
		}
		if nm.UI.tabWidth != 20 {
			t.Errorf("tabWidth = %d, want 20", nm.UI.tabWidth)
		}
	})

	t.Run("uncappedTabWidth", func(t *testing.T) {
		m := setupModel()
		nm, _ := dispatcherModel(t, m, tea.WindowSizeMsg{Width: 40, Height: 24})
		// (40 - 4) / 4 tabs = 9, below the 20 cap.
		if nm.UI.tabWidth != 9 {
			t.Errorf("tabWidth = %d, want 9", nm.UI.tabWidth)
		}
	})
}

func TestDispatcherProgressFrameMsg(t *testing.T) {
	m := setupModel()
	// FrameMsg must be dispatched to handleProgressFrame without panic; the
	// helper already asserts the same *Model is returned.
	dispatcherModel(t, m, progress.FrameMsg{})
}

func TestDispatcherTickMsg(t *testing.T) {
	m := setupModel()
	m.Config.TickRate = 10

	_, cmd := dispatcherModel(t, m, TickMsg{})
	if cmd == nil {
		t.Error("cmd = nil, want a re-scheduled tick cmd")
	}
}

func TestDispatcherAudioLoadedMsg(t *testing.T) {
	t.Run("stale", func(t *testing.T) {
		m := setupModel()
		m.loadGeneration = 5
		m.Loading = true

		nm, cmd := dispatcherModel(t, m, AudioLoadedMsg{Player: &mockPlayer{}, Path: "/x/y.mp3", Generation: 3})
		if cmd != nil {
			t.Errorf("cmd = %v, want nil", cmd)
		}
		if nm.Audio.Player != nil {
			t.Error("Player should remain nil for a stale load")
		}
		if !nm.Loading {
			t.Error("Loading should remain true for a stale load")
		}
	})

	t.Run("success", func(t *testing.T) {
		m := setupModel()
		m.Config.Accent.AutoAccent = boolPtr(false)
		m.Audio.Volume = 0.4
		mp := &mockPlayer{duration: 120 * time.Second}

		nm, cmd := dispatcherModel(t, m, AudioLoadedMsg{Player: mp, Path: "/music/My Song.flac", Generation: 0})
		if cmd != nil {
			t.Errorf("cmd = %v, want nil", cmd)
		}
		if nm.Audio.Player != mp {
			t.Error("Player not mounted")
		}
		if nm.Audio.Duration != 120*time.Second {
			t.Errorf("Duration = %v, want 120s", nm.Audio.Duration)
		}
		if nm.Audio.CurrentSong != "My Song.flac" {
			t.Errorf("CurrentSong = %q, want %q", nm.Audio.CurrentSong, "My Song.flac")
		}
		if nm.Audio.Artist != "Unknown Artist" {
			t.Errorf("Artist = %q, want %q", nm.Audio.Artist, "Unknown Artist")
		}
		if !nm.Audio.IsPlaying {
			t.Error("IsPlaying should be true after successful play")
		}
		if mp.volume != 0.4 {
			t.Errorf("Player volume = %v, want 0.4", mp.volume)
		}
		if nm.Loading || nm.switchingTrack {
			t.Errorf("Loading/switchingTrack = %v/%v, want false/false", nm.Loading, nm.switchingTrack)
		}
	})

	t.Run("pendingSeek", func(t *testing.T) {
		m := setupModel()
		m.Config.Accent.AutoAccent = boolPtr(false)
		m.pendingSeek = 30 * time.Second
		mp := &mockPlayer{duration: 120 * time.Second}

		nm, _ := dispatcherModel(t, m, AudioLoadedMsg{Player: mp, Path: "/a/b.flac", Generation: 0})
		if !mp.seekCalled || mp.lastSeekPos != 30*time.Second {
			t.Errorf("Seek called with %v, want 30s", mp.lastSeekPos)
		}
		if nm.pendingSeek != 0 {
			t.Errorf("pendingSeek = %v, want 0", nm.pendingSeek)
		}
	})
}

func TestDispatcherUnknownMsg(t *testing.T) {
	m := setupModel()
	nm, cmd := dispatcherModel(t, m, "not-a-real-message")
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if nm != m {
		t.Error("unknown message should return the same model")
	}
}

func TestLyricSig(t *testing.T) {
	if got := lyricSig(nil, 2*time.Second, 3); got != "0|next=3" {
		t.Errorf("lyricSig(nil) = %q, want %q", got, "0|next=3")
	}

	lines := []ipc.LyricLineJSON{{Text: "a"}, {Text: "b"}}
	if got := lyricSig(lines, 1500*time.Millisecond, 3); got != "2|a|b|next=3|elapsed=1.5" {
		t.Errorf("lyricSig(lines) = %q, want %q", got, "2|a|b|next=3|elapsed=1.5")
	}
}

func TestBuildLyricLinesJSON(t *testing.T) {
	if got := buildLyricLinesJSON(nil, time.Second); got != nil {
		t.Errorf("buildLyricLinesJSON(nil) = %v, want nil", got)
	}
	if got := buildLyricLinesJSON(&lyrics.LyricsData{}, time.Second); got != nil {
		t.Errorf("buildLyricLinesJSON(empty) = %v, want nil", got)
	}

	data := &lyrics.LyricsData{Lines: []lyrics.LyricLine{
		{Time: 0, Text: "hello"},
	}}
	got := buildLyricLinesJSON(data, time.Second)
	if len(got) != 1 || got[0].Text != "hello" {
		t.Errorf("buildLyricLinesJSON = %+v, want one line 'hello'", got)
	}
}

func TestEffectiveBaseURL(t *testing.T) {
	if got := effectiveBaseURL(""); got != config.DefaultBaseURL {
		t.Errorf("effectiveBaseURL(\"\") = %q, want %q", got, config.DefaultBaseURL)
	}
	if got := effectiveBaseURL("http://example.com"); got != "http://example.com" {
		t.Errorf("effectiveBaseURL(custom) = %q, want %q", got, "http://example.com")
	}
}
