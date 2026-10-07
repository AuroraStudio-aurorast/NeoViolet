package anim

import (
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

// deltaChainFixture is the one vector whose keyframes are not every frame: two
// frames, and only the first of them is a keyframe. Landing on a keyframe and
// landing on the exact frame are different places in it, which is what makes the
// choice between the two observable.
const deltaChainFixture = "delta-chain.nvaa"

// testTick is the interval the tests below report, near enough to thirty reports
// a second.
const testTick = 33 * time.Millisecond

func TestIsJump(t *testing.T) {
	cases := []struct {
		name             string
		previous, target uint64
		tick             time.Duration
		want             bool
	}{
		{"one tick of playback", 1000, 1033, testTick, false},
		{"three ticks of playback", 1000, 1099, testTick, false},
		{"a scrub forward", 1000, 2000, testTick, true},
		{"a rewind", 5000, 1000, testTick, true},
		{"a loop that wrapped", 9900, 100, testTick, true},
		{"a host that ticks fast does not turn playback into a jump", 1000, 1033, time.Millisecond, false},
		{"a host that reports no tick still has a floor", 0, 80, 0, false},
		{"a host that reports no tick still shows a scrub", 0, 150, 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isJump(tc.previous, tc.target, tc.tick); got != tc.want {
				t.Errorf("isJump(%d, %d, %v) = %v, want %v", tc.previous, tc.target, tc.tick, got, tc.want)
			}
		})
	}
}

func TestKeyframeAtOrBefore(t *testing.T) {
	keyframes := []int{0, 12, 24}
	cases := []struct {
		name      string
		keyframes []int
		frame     int
		want      int
	}{
		{"the frame is a keyframe", keyframes, 12, 12},
		{"between two of them", keyframes, 20, 12},
		{"past the last one", keyframes, 400, 24},
		{"before the first one", []int{6, 12}, 2, 0},
		{"no keyframes at all", nil, 7, -1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := keyframeAtOrBefore(tc.keyframes, tc.frame); got != tc.want {
				t.Errorf("keyframeAtOrBefore(%v, %d) = %d, want %d", tc.keyframes, tc.frame, got, tc.want)
			}
		})
	}
}

// A target that moves further in one tick than playback could have moved it is a
// seek or a scrub rather than drift. Reaching an exact frame means decoding every
// frame between it and the nearest keyframe before it, so while the position is
// flying the animation lands on that keyframe instead, and goes back for the
// exact frame once the position settles.
func TestSync_LandsOnAKeyframeWhileThePositionFlies(t *testing.T) {
	state := New()
	msg := run(t, state.LoadFor(loadAt(audioWith(t, deltaChainFixture), 8, 1, config.GateModeEither)))
	if cmd := state.Apply(msg); cmd != nil {
		cmd()
	}

	// Frame 1 is a delta frame, so the keyframe and the exact frame differ.
	state.Sync(170*time.Millisecond, true, testTick, 8, 1)
	if got := state.player.Index(); got != 0 {
		t.Fatalf("a position that flew landed on frame %d, want the keyframe at 0", got)
	}

	state.Sync(170*time.Millisecond, true, testTick, 8, 1)
	if got := state.player.Index(); got != 1 {
		t.Fatalf("a position that settled landed on frame %d, want the exact frame 1", got)
	}
	if got := state.player.ElapsedMS(); got != 100 {
		t.Fatalf("the exact landing sits %dms into the animation, want 100ms", got)
	}
}

// A frame that is already a keyframe costs nothing to reach, so flying to one
// does not have to wait for the position to settle.
func TestSync_AJumpToAKeyframeLandsExactly(t *testing.T) {
	state := New()
	msg := run(t, state.LoadFor(loadAt(audioWith(t, plainFixture), 40, 12, config.GateModeEither)))
	if cmd := state.Apply(msg); cmd != nil {
		cmd()
	}

	state.Sync(170*time.Millisecond, true, testTick, 40, 12)
	if got := state.player.Index(); got != 1 {
		t.Fatalf("a jump onto a keyframe landed on frame %d, want 1", got)
	}
}
