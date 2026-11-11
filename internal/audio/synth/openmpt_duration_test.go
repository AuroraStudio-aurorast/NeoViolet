package synth

import (
	"testing"
)

func TestOpenmptDurationMatchesPlayer(t *testing.T) {
	requireOpenmpt(t)

	data := minimalMOD()
	duration, ok := OpenmptDuration(data)
	if !ok || duration <= 0 {
		t.Fatalf("OpenmptDuration() = %v, %v, want a positive duration", duration, ok)
	}
	if player := newMinimalMODPlayer(t); duration != player.Duration() {
		t.Fatalf("OpenmptDuration() = %v, player duration = %v", duration, player.Duration())
	}
}

func TestOpenmptDurationRejectsInvalidData(t *testing.T) {
	for _, data := range [][]byte{nil, {}, []byte("not a module")} {
		if duration, ok := OpenmptDuration(data); ok || duration != 0 {
			t.Errorf("OpenmptDuration(%q) = %v, %v, want zero, false", data, duration, ok)
		}
	}
}

func TestOpenmptDurationUnavailable(t *testing.T) {
	t.Setenv(openmptEnvOverride, "/nonexistent/libopenmpt.0.dylib")
	resetOpenmptState()
	defer resetOpenmptState()

	if duration, ok := OpenmptDuration(minimalMOD()); ok || duration != 0 {
		t.Fatalf("OpenmptDuration() = %v, %v without libopenmpt", duration, ok)
	}
}
