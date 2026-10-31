package ui

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

type switchPlayer struct {
	mockPlayer
	playErr error
	onPlay  func()
	name    string
	calls   *[]string
}

func (p *switchPlayer) Play() error {
	if p.calls != nil {
		*p.calls = append(*p.calls, "play "+p.name)
	}
	if p.onPlay != nil {
		p.onPlay()
	}
	if p.playErr != nil {
		return p.playErr
	}
	return p.mockPlayer.Play()
}

func (p *switchPlayer) Pause() {
	if p.calls != nil {
		*p.calls = append(*p.calls, "pause "+p.name)
	}
	p.mockPlayer.Pause()
}

func (p *switchPlayer) Close() error {
	if p.calls != nil {
		*p.calls = append(*p.calls, "close "+p.name)
	}
	return p.mockPlayer.Close()
}

func currentTrackModel() (*Model, *switchPlayer) {
	m := setupModel()
	m.Config.TickRate = 30
	m.Config.Accent.AutoAccent = boolPtr(false)
	current := &switchPlayer{mockPlayer: mockPlayer{
		path: "current.flac", playing: true, position: 30 * time.Second, duration: 120 * time.Second,
	}}
	m.Audio.Player = current
	m.Audio.CurrentSong = "Current song"
	m.Audio.Artist = "Current artist"
	m.Audio.Album = "Current album"
	m.Audio.Duration = 120 * time.Second
	m.Audio.Elapsed = 30 * time.Second
	m.Audio.IsPlaying = true
	m.Audio.Volume = 0.6
	return m, current
}

func TestTrackSwitchKeepsCurrentOnLoadFailure(t *testing.T) {
	for _, playing := range []bool{true, false} {
		t.Run(map[bool]string{true: "playing", false: "paused"}[playing], func(t *testing.T) {
			m, current := currentTrackModel()
			current.playing, m.Audio.IsPlaying = playing, playing
			m.Audio.Lyrics = &lyrics.Data{Format: "lrc"}
			m.Accent = &accent.Accent{}
			oldLyrics, oldAccent := m.Audio.Lyrics, m.Accent
			ctrl := &mockMediaCtl{}
			m.MediaCtl = ctrl

			_, cmd := dispatcherModel(t, m, LoadTrackMsg{Path: filepath.Join(t.TempDir(), "missing.flac")})
			if m.Audio.Player != current || current.closeCalled || current.pauseCalled {
				t.Error("preparing replacement interrupted the current player")
			}
			dispatcherModel(t, m, TickMsg{})
			dispatcherModel(t, m, cmd())
			if m.Audio.Player != current || current.closeCalled || current.IsPlaying() != playing {
				t.Error("failed replacement changed current playback")
			}
			if m.Audio.Elapsed != 30*time.Second || m.Audio.Lyrics != oldLyrics || m.Accent != oldAccent {
				t.Error("failed replacement reset current position, lyrics or accent")
			}
			if m.Loading || m.switchingTrack || !m.Error.Visible {
				t.Error("load failure did not finish loading and show an error")
			}
			for _, state := range ctrl.updates {
				if !state.HasTrack || state.Title != "Current song" || state.Playing != playing {
					t.Errorf("OS metadata stopped describing the current track: %+v", state)
				}
			}
		})
	}
}

func TestTrackSwitchKeepsCurrentOnStartFailure(t *testing.T) {
	for _, playing := range []bool{true, false} {
		t.Run(map[bool]string{true: "playing", false: "paused"}[playing], func(t *testing.T) {
			m, current := currentTrackModel()
			current.playing, m.Audio.IsPlaying = playing, playing
			dispatcherModel(t, m, LoadTrackMsg{Path: "next.flac"})
			candidate := &switchPlayer{playErr: errors.New("cannot start")}
			dispatcherModel(t, m, AudioLoadedMsg{Player: candidate, Path: "next.flac", Generation: m.loadGeneration})
			if m.Audio.Player != current || current.closeCalled || current.IsPlaying() != playing {
				t.Error("start failure did not retain current player and its playback state")
			}
			if !candidate.closeCalled || m.Audio.CurrentSong != "Current song" || current.seekCalled {
				t.Error("start failure leaked candidate, replaced metadata or rewound current track")
			}
		})
	}
}

func TestTrackSwitchCommitsOnlyAfterStart(t *testing.T) {
	m, current := currentTrackModel()
	calls := []string{}
	current.name, current.calls = "current", &calls
	dispatcherModel(t, m, LoadTrackMsg{Path: "next.flac"})
	m.Audio.SetVolume(0.3)
	candidate := &switchPlayer{name: "next", calls: &calls}
	candidate.onPlay = func() {
		if m.Audio.Player != current || current.closeCalled || current.IsPlaying() {
			t.Error("handoff did not keep the paused current track available for rollback")
		}
	}
	ctrl := &mockMediaCtl{}
	m.MediaCtl = ctrl
	dispatcherModel(t, m, AudioLoadedMsg{Player: candidate, Path: "next.flac", Generation: m.loadGeneration})
	if !slices.Equal(calls, []string{"pause current", "play next", "close current"}) {
		t.Errorf("handoff = %v, want pause current, play next, close current", calls)
	}
	if m.Audio.Player != candidate || !m.Audio.IsPlaying || m.Audio.Album != "" || m.Audio.Elapsed != 0 {
		t.Error("successful switch did not mount candidate and reset old track fields")
	}
	if candidate.volume != 0.3 {
		t.Error("candidate did not use volume selected during loading")
	}
	if len(ctrl.updates) != 1 || ctrl.updates[0].Title != "next.flac" {
		t.Error("successful switch did not immediately publish new metadata")
	}
}

func TestTrackSwitchDiscardsSupersededResults(t *testing.T) {
	m, current := currentTrackModel()
	_, older := dispatcherModel(t, m, LoadTrackMsg{Path: filepath.Join(t.TempDir(), "older.flac")})
	olderGeneration := m.loadGeneration
	dispatcherModel(t, m, LoadTrackMsg{Path: "latest.flac"})
	if msg := older(); msg != nil {
		dispatcherModel(t, m, msg)
	}
	if !m.Loading || m.Error.Visible {
		t.Error("older command was labeled as the latest request and ended its loading state")
	}
	stale := &switchPlayer{}
	dispatcherModel(t, m, AudioLoadedMsg{Player: stale, Path: "older.flac", Generation: olderGeneration})
	if !stale.closeCalled || m.Audio.Player != current || !m.Loading {
		t.Error("stale success leaked resources or changed current playback")
	}
}

func TestTrackSwitchIgnoresStaleStartupFailure(t *testing.T) {
	m, _ := currentTrackModel()
	dispatcherModel(t, m, LoadTrackMsg{Path: "next.flac"})
	msg := loadAudio(filepath.Join(t.TempDir(), "startup.flac"), "", "auto", 0)
	dispatcherModel(t, m, msg)
	if !m.Loading || m.Error.Visible {
		t.Error("generation-zero startup failure interrupted a newer load")
	}
}

func TestTrackSwitchUnrelatedErrorKeepsLoading(t *testing.T) {
	m, current := currentTrackModel()
	dispatcherModel(t, m, LoadTrackMsg{Path: "next.flac"})
	dispatcherModel(t, m, ErrorMsg{Message: "Invalid path", Timer: 90})
	if !m.Loading || !m.switchingTrack || m.Audio.Player != current {
		t.Error("unrelated UI error canceled the pending replacement")
	}
}

func TestTrackSwitchRejectsResultsAfterCleanup(t *testing.T) {
	m, _ := currentTrackModel()
	dispatcherModel(t, m, LoadTrackMsg{Path: "next.flac"})
	m.cleanup()
	candidate := &switchPlayer{}
	dispatcherModel(t, m, AudioLoadedMsg{Player: candidate, Path: "next.flac", Generation: m.loadGeneration})
	if !candidate.closeCalled || m.Audio.Player != nil {
		t.Error("load result arriving after shutdown was mounted or leaked")
	}
}

func TestTrackSwitchCancelsPreparedCandidates(t *testing.T) {
	for _, queued := range []bool{true, false} {
		t.Run(map[bool]string{true: "queued result", false: "still opening"}[queued], func(t *testing.T) {
			m, current := currentTrackModel()
			dispatcherModel(t, m, LoadTrackMsg{Path: "next.flac"})
			load := m.pendingLoad
			candidate := &switchPlayer{}
			msg := AudioLoadedMsg{Player: candidate, Path: "next.flac", Generation: m.loadGeneration}
			if queued {
				load.deliver(msg)
			}
			dispatcherModel(t, m, LoadTrackMsg{Path: "latest.flac"})
			if !queued && load.deliver(msg) != nil {
				t.Error("canceled load still delivered a result")
			}
			if !candidate.closeCalled || current.closeCalled || m.Audio.Player != current {
				t.Error("superseded load leaked its candidate or interrupted current playback")
			}
		})
	}
}

func TestTrackSwitchCleanupReleasesQueuedCandidate(t *testing.T) {
	m, _ := currentTrackModel()
	dispatcherModel(t, m, LoadTrackMsg{Path: "next.flac"})
	candidate := &switchPlayer{}
	m.pendingLoad.deliver(AudioLoadedMsg{Player: candidate, Generation: m.loadGeneration})
	m.cleanup()
	if !candidate.closeCalled {
		t.Error("shutdown leaked a candidate whose result was never dispatched")
	}
}

func TestTrackSwitchKeepsAnimationOnLoadFailure(t *testing.T) {
	m := animModel(t, animPlainFixture, 100, 30)
	m.Audio.Player = &mockPlayer{playing: true}
	_, cmd := handleLoadTrack(m, LoadTrackMsg{Path: filepath.Join(t.TempDir(), "missing.flac")})
	if !m.animVisible() || m.Anim.Loading {
		t.Error("preparing a candidate changed the current animation")
	}
	dispatcherModel(t, m, cmd())
	if !m.animVisible() || m.Anim.Loading {
		t.Error("failed candidate changed the current animation")
	}
}

func TestTrackSwitchAcceptsOnlyCurrentTrackAccent(t *testing.T) {
	m, _ := currentTrackModel()
	oldAccent := &accent.Accent{}
	dispatcherModel(t, m, LoadTrackMsg{Path: "next.flac"})
	dispatcherModel(t, m, AccentApplyMsg{Accent: oldAccent, Generation: 0})
	if m.Accent != oldAccent {
		t.Error("preparing a replacement discarded the current track's accent")
	}
	dispatcherModel(t, m, AudioLoadedMsg{Player: &switchPlayer{}, Path: "next.flac", Generation: m.loadGeneration})
	dispatcherModel(t, m, AccentApplyMsg{Accent: oldAccent, Generation: 0})
	if m.Accent != nil {
		t.Error("an old accent result recolored the accepted replacement")
	}
}

func TestTrackSwitchPublishesMetadataBeforeAccentFinishes(t *testing.T) {
	m, _ := currentTrackModel()
	m.Config.Accent.AutoAccent = boolPtr(true)
	ctrl := &mockMediaCtl{}
	m.MediaCtl = ctrl
	dispatcherModel(t, m, LoadTrackMsg{Path: "next.flac"})
	dispatcherModel(t, m, AudioLoadedMsg{Player: &switchPlayer{}, Path: "next.flac", Generation: m.loadGeneration})
	if len(ctrl.updates) != 1 || ctrl.updates[0].Title != "next.flac" {
		t.Error("system metadata waited for async accent extraction")
	}
}

func TestTrackSwitchImmediatePolicyUnloadsBeforePreparing(t *testing.T) {
	m, current := currentTrackModel()
	m.Config.TrackSwitchMode = "immediate"
	ctrl := &mockMediaCtl{}
	m.MediaCtl = ctrl
	dispatcherModel(t, m, LoadTrackMsg{Path: "next.flac"})
	if m.Audio.Player != nil || !current.closeCalled || m.Audio.CurrentSong != "" || !m.Loading {
		t.Error("immediate policy did not unload the current player before preparation")
	}
	dispatcherModel(t, m, AudioLoadFailedMsg{Message: "bad file", Timer: 90, Generation: m.loadGeneration})
	if m.Audio.Player != nil || m.Audio.CurrentSong != "" || m.Loading {
		t.Error("immediate policy restored the current player after load failure")
	}
	if len(ctrl.updates) == 0 || ctrl.updates[0].HasTrack {
		t.Error("immediate policy did not clear OS media metadata")
	}
}

func TestTrackSwitchDefaultPolicyRetainsCurrentDuringPreparation(t *testing.T) {
	m, current := currentTrackModel()
	if config.DefaultConfig().TrackSwitchMode != "on_success" {
		t.Fatal("default config must use on_success")
	}
	dispatcherModel(t, m, LoadTrackMsg{Path: "next.flac"})
	if m.Audio.Player != current || current.closeCalled {
		t.Error("default on_success mode interrupted current playback during preparation")
	}
}

func TestTrackSwitchModesShareSuccessfulHandoff(t *testing.T) {
	for _, mode := range []string{"on_success", "immediate", "", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			m, current := currentTrackModel()
			m.Config.TrackSwitchMode = mode
			dispatcherModel(t, m, LoadTrackMsg{Path: "next.flac"})
			candidate := &switchPlayer{}
			dispatcherModel(t, m, AudioLoadedMsg{Player: candidate, Path: "next.flac", Generation: m.loadGeneration})
			if m.Audio.Player != candidate || !current.closeCalled || m.Loading || !m.Audio.IsPlaying {
				t.Error("mode did not accept successful replacement and release current player")
			}
		})
	}
}

func TestTrackSwitchImmediatePolicyStartFailureLeavesNoTrack(t *testing.T) {
	m, current := currentTrackModel()
	m.Config.TrackSwitchMode = "immediate"
	dispatcherModel(t, m, LoadTrackMsg{Path: "next.flac"})
	candidate := &switchPlayer{playErr: errors.New("cannot start")}
	dispatcherModel(t, m, AudioLoadedMsg{Player: candidate, Path: "next.flac", Generation: m.loadGeneration})
	if m.Audio.Player != nil || !candidate.closeCalled || !current.closeCalled || m.Loading {
		t.Error("immediate policy did not leave an empty player after start failure")
	}
}

func TestTrackSwitchImmediatePolicyFollowsAnimationAfterAcceptance(t *testing.T) {
	m := animModel(t, animPlainFixture, 100, 30)
	m.Config.TrackSwitchMode = "immediate"
	m.Config.Anim.Auto = false
	m.Audio.Player = &mockPlayer{playing: true}
	path := animFixturePath(t, animPlainFixture)
	dispatcherModel(t, m, LoadTrackMsg{Path: path})
	if m.animVisible() || m.Anim.Loading {
		t.Error("immediate switch kept an animation for an unloaded track")
	}
	dispatcherModel(t, m, AudioLoadedMsg{Player: &mockPlayer{}, Path: path, Generation: m.loadGeneration})
	if !m.Anim.Loading {
		t.Error("immediate switch lost the existing animation's follow intent")
	}
}

func TestTrackSwitchModesTransferPendingAnimationWarning(t *testing.T) {
	for _, mode := range []string{"on_success", "immediate"} {
		t.Run(mode, func(t *testing.T) {
			m := batteryModel(t, animPlainFixture, 18)
			m.Config.TrackSwitchMode = mode
			runAnim(m, invocation{})
			if !m.Anim.Held() {
				t.Fatal("fixture must hold a pending battery warning")
			}
			path := animFixturePath(t, animPlainFixture)
			dispatcherModel(t, m, LoadTrackMsg{Path: path})
			dispatcherModel(t, m, AudioLoadedMsg{Player: &mockPlayer{}, Path: path, Generation: m.loadGeneration})
			if !m.Anim.Held() {
				t.Error("accepted replacement lost the pending battery confirmation")
			}
		})
	}
}
