//go:build darwin

package mediactl

import (
	"fmt"
	"image"
	"sync/atomic"
	"testing"

	"github.com/ebitengine/purego/objc"
)

var nowPlayingTestSeq atomic.Uint64

// Exercise the real Update path with a private ObjC receiver so the test does
// not publish test tracks to the user's system NowPlaying center.
func TestNowPlayingTrackUnload(t *testing.T) {
	if err := setup(); err != nil {
		t.Fatalf("setup() error: %v", err)
	}

	var hasInfo, hasArtwork bool
	var title string
	var playbackState int32
	selObjectForKey := objc.RegisterName("objectForKey:")
	selUTF8String := objc.RegisterName("UTF8String")
	class, err := objc.RegisterClass(
		fmt.Sprintf("NeoVioletNowPlayingTest%d", nowPlayingTestSeq.Add(1)),
		objc.GetClass("NSObject"),
		nil,
		nil,
		[]objc.MethodDef{
			{Cmd: selSetNowPlayingInfo, Fn: func(_ objc.ID, _ objc.SEL, info objc.ID) {
				hasInfo = info != 0
				title = objc.Send[string](info.Send(selObjectForKey, nsString("title")), selUTF8String)
				hasArtwork = info.Send(selObjectForKey, nsString("artwork")) != 0
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

	for _, tt := range []struct {
		name       string
		state      PlayState
		wantInfo   bool
		wantTitle  string
		wantArt    bool
		wantStatus int32
	}{
		{
			name: "playing track with artwork",
			state: PlayState{
				Title: "Previous song", HasTrack: true, Playing: true,
				Cover: image.NewRGBA(image.Rect(0, 0, 2, 2)),
			},
			wantInfo: true, wantTitle: "Previous song", wantArt: true, wantStatus: playbackStatePlaying,
		},
		{name: "unloaded", wantStatus: playbackStateStopped},
		{name: "unloaded on subsequent tick", wantStatus: playbackStateStopped},
		{
			name:       "no track overrides stale metadata",
			state:      PlayState{Title: "Previous song", Playing: true},
			wantStatus: playbackStateStopped,
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
			c.Update(tt.state)
			if hasInfo != tt.wantInfo || title != tt.wantTitle || hasArtwork != tt.wantArt {
				t.Errorf("NowPlaying info = (%v, %q, artwork=%v), want (%v, %q, artwork=%v)",
					hasInfo, title, hasArtwork, tt.wantInfo, tt.wantTitle, tt.wantArt)
			}
			if playbackState != tt.wantStatus {
				t.Errorf("playback state = %d, want %d", playbackState, tt.wantStatus)
			}
		})
	}
}
