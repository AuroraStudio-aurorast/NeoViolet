//go:build linux

package power

import "testing"

// TestRead_Smoke checks that a reading arrives in the shape everything else
// assumes. A machine with no battery passes: the point is that Read answers at
// all, not that it finds something, and the machines this runs on in CI have no
// battery.
func TestRead_Smoke(t *testing.T) {
	status, err := Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !status.Known {
		t.Log("no battery on this machine")
		return
	}
	if status.Percent < 0 || status.Percent > 100 {
		t.Errorf("Percent = %d, want 0 to 100", status.Percent)
	}
	t.Logf("percent=%d onBattery=%v", status.Percent, status.OnBattery)
}
