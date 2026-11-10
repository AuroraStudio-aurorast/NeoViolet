//go:build darwin

package mediactl

import (
	"fmt"
	"image"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
)

var nowPlayingTestSeq atomic.Uint64

// Exercise the real Update path with a private ObjC receiver so the test does
// not publish test tracks to the user's system NowPlaying center.
func TestNowPlayingArtworkTransitions(t *testing.T) {
	if err := setup(); err != nil {
		t.Fatalf("setup() error: %v", err)
	}

	var hasInfo, hasArtwork bool
	var title string
	var playbackState int32
	var clears int
	var duration, position float64
	selObjectForKey := objc.RegisterName("objectForKey:")
	selUTF8String := objc.RegisterName("UTF8String")
	selDoubleValue := objc.RegisterName("doubleValue")
	class, err := objc.RegisterClass(
		fmt.Sprintf("NeoVioletNowPlayingTest%d", nowPlayingTestSeq.Add(1)),
		objc.GetClass("NSObject"),
		nil,
		nil,
		[]objc.MethodDef{
			{Cmd: selSetNowPlayingInfo, Fn: func(_ objc.ID, _ objc.SEL, info objc.ID) {
				hasInfo = info != 0
				if !hasInfo {
					clears++
				}
				title = objc.Send[string](info.Send(selObjectForKey, nsString("title")), selUTF8String)
				hasArtwork = info.Send(selObjectForKey, nsString("artwork")) != 0
				duration = objc.Send[float64](info.Send(selObjectForKey, nsString("playbackDuration")), selDoubleValue)
				position = objc.Send[float64](info.Send(selObjectForKey, nsString("MPNowPlayingInfoPropertyElapsedPlaybackTime")), selDoubleValue)
			}},
			{Cmd: selSetPlaybackState, Fn: func(_ objc.ID, _ objc.SEL, state int32) {
				playbackState = state
			}},
		},
	)
	if err != nil {
		t.Fatalf("register NowPlaying receiver: %v", err)
	}

	c := &darwinCtrl{nowPlaying: objc.ID(class).Send(selNew)}
	t.Cleanup(func() {
		_ = c.Close()
		c.nowPlaying.Send(selRelease)
	})

	cover := image.NewRGBA(image.Rect(0, 0, 2, 2))
	otherCover := image.NewRGBA(image.Rect(0, 0, 3, 3))
	invalidCover := image.NewRGBA(image.Rectangle{})
	// These updates form one playback session, including direct track switches
	// with no intermediate unloaded state (the default on_success policy).
	for _, tt := range []struct {
		name       string
		state      PlayState
		wantInfo   bool
		wantTitle  string
		wantArt    bool
		wantStatus int32
		wantClear  bool
	}{
		{
			name: "playing track with artwork",
			state: PlayState{
				Title: "Previous song", HasTrack: true, Playing: true,
				Cover: cover,
			},
			wantInfo: true, wantTitle: "Previous song", wantArt: true, wantStatus: playbackStatePlaying,
		},
		{
			name: "same artwork on subsequent tick",
			state: PlayState{
				Title: "Previous song", HasTrack: true, Playing: true, Cover: cover,
			},
			wantInfo: true, wantTitle: "Previous song", wantArt: true, wantStatus: playbackStatePlaying,
		},
		{
			name: "different artwork replaces previous cover",
			state: PlayState{
				Title: "Another song", HasTrack: true, Playing: true, Cover: otherCover,
			},
			wantInfo: true, wantTitle: "Another song", wantArt: true, wantStatus: playbackStatePlaying,
		},
		{
			name: "cover disappears without changing title",
			state: PlayState{
				Title: "Another song", HasTrack: true, Playing: true,
				Duration: 3 * time.Minute, Position: 15 * time.Second,
			},
			wantInfo: true, wantTitle: "Another song", wantStatus: playbackStatePlaying, wantClear: true,
		},
		{
			name: "coverless tick does not clear again",
			state: PlayState{
				Title: "Another song", HasTrack: true, Playing: true,
				Duration: 3 * time.Minute, Position: 16 * time.Second,
			},
			wantInfo: true, wantTitle: "Another song", wantStatus: playbackStatePlaying,
		},
		{
			name: "cover returns after coverless track",
			state: PlayState{
				Title: "Previous song", HasTrack: true, Playing: true, Cover: cover,
			},
			wantInfo: true, wantTitle: "Previous song", wantArt: true, wantStatus: playbackStatePlaying,
		},
		{
			name: "paused track without cover replaces covered track",
			state: PlayState{
				Title: "Next song", HasTrack: true, Duration: time.Minute, Position: 5 * time.Second,
			},
			wantInfo: true, wantTitle: "Next song", wantStatus: playbackStatePaused, wantClear: true,
		},
		{
			name: "artwork loads again",
			state: PlayState{
				Title: "Previous song", HasTrack: true, Playing: true, Cover: cover,
			},
			wantInfo: true, wantTitle: "Previous song", wantArt: true, wantStatus: playbackStatePlaying,
		},
		{
			name: "unencodable cover removes previous artwork",
			state: PlayState{
				Title: "Broken cover", HasTrack: true, Playing: true, Cover: invalidCover,
			},
			wantInfo: true, wantTitle: "Broken cover", wantStatus: playbackStatePlaying, wantClear: true,
		},
		{
			name: "unencodable cover on subsequent tick does not clear again",
			state: PlayState{
				Title: "Broken cover", HasTrack: true, Playing: true, Cover: invalidCover,
			},
			wantInfo: true, wantTitle: "Broken cover", wantStatus: playbackStatePlaying,
		},
		{
			name: "covered track loads before unload",
			state: PlayState{
				Title: "Previous song", HasTrack: true, Playing: true, Cover: cover,
			},
			wantInfo: true, wantTitle: "Previous song", wantArt: true, wantStatus: playbackStatePlaying,
		},
		{name: "unloaded", wantStatus: playbackStateStopped, wantClear: true},
		{name: "unloaded on subsequent tick", wantStatus: playbackStateStopped, wantClear: true},
		{
			name:       "no track overrides stale metadata",
			state:      PlayState{Title: "Previous song", Playing: true, Cover: cover},
			wantStatus: playbackStateStopped,
			wantClear:  true,
		},
		{
			name:     "next track loads",
			state:    PlayState{Title: "Next song", HasTrack: true, Playing: true},
			wantInfo: true, wantTitle: "Next song", wantStatus: playbackStatePlaying,
		},
		{
			name:     "paused track keeps metadata",
			state:    PlayState{Title: "Next song", HasTrack: true},
			wantInfo: true, wantTitle: "Next song", wantStatus: playbackStatePaused,
		},
		{
			name:       "loaded track without title",
			state:      PlayState{HasTrack: true},
			wantInfo:   true,
			wantStatus: playbackStatePaused,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			clearsBefore := clears
			c.Update(tt.state)
			if hasInfo != tt.wantInfo || title != tt.wantTitle || hasArtwork != tt.wantArt {
				t.Errorf("NowPlaying info = (%v, %q, artwork=%v), want (%v, %q, artwork=%v)",
					hasInfo, title, hasArtwork, tt.wantInfo, tt.wantTitle, tt.wantArt)
			}
			if playbackState != tt.wantStatus {
				t.Errorf("playback state = %d, want %d", playbackState, tt.wantStatus)
			}
			wantClears := 0
			if tt.wantClear {
				wantClears = 1
			}
			if got := clears - clearsBefore; got != wantClears {
				t.Errorf("NowPlaying clear calls = %d, want %d", got, wantClears)
			}
			if !tt.wantArt && (c.coverArtwork != 0 || c.lastCoverImg != nil) {
				t.Error("controller retained artwork for a track without a usable cover")
			}
			if tt.wantInfo && (duration != tt.state.Duration.Seconds() || position != tt.state.Position.Seconds()) {
				t.Errorf("NowPlaying duration/position = (%v, %v), want (%v, %v)",
					duration, position, tt.state.Duration.Seconds(), tt.state.Position.Seconds())
			}
		})
	}
}
