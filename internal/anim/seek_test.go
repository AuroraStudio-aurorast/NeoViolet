package anim

import (
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

// deltaChainFixture is the one vector whose keyframes are not every frame: two
// frames, and only the first of them is a keyframe. Landing on the keyframe and
// landing on the exact frame are therefore different places in it, which is what
// makes the two observable as different.
const deltaChainFixture = "delta-chain.nvaa"

// A target that moves further in one tick than playback could have moved it is a
// seek or a scrub rather than drift, and the animation goes where the audio is.
// Reaching that frame costs the frames between it and the keyframe before it,
// which is a price a seek pays once -- landing on the keyframe instead would
// leave a picture on screen that the audio did not ask for.
func TestSync_LandsWhereTheAudioIsWhileThePositionFlies(t *testing.T) {
	state := New()
	msg := run(t, state.LoadFor(loadAt(audioWith(t, deltaChainFixture), 8, 1, config.GateModeEither)))
	if cmd, _ := state.Apply(msg); cmd != nil {
		cmd()
	}

	// Frame 1 is a delta frame, so the keyframe and the exact frame differ.
	state.Sync(170*time.Millisecond, true, 8, 1)
	if got := state.player.Index(); got != 1 {
		t.Fatalf("a position that flew landed on frame %d, want the frame the audio is in, 1", got)
	}
	if got := state.player.ElapsedMS(); got != 100 {
		t.Fatalf("the landing sits %dms into the animation, want 100ms", got)
	}
}
