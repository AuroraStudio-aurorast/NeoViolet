package format

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
)

func TestProbeTrackerReportsDuration(t *testing.T) {
	if !synth.OpenmptAvailable() {
		t.Skip("libopenmpt not installed")
	}
	data, err := os.ReadFile(filepath.Join("..", "synth", "testdata", "minimal.mod"))
	if err != nil {
		t.Fatal(err)
	}
	got := probeTestFile(t, "minimal.mod", data)
	if got.Format != "MOD" || got.Duration <= 0 {
		t.Fatalf("got %+v, want MOD with a positive duration", got)
	}
}
