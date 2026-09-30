package power

import "testing"

func TestStatus_Low(t *testing.T) {
	battery := func(percent int) Status {
		return Status{Percent: percent, OnBattery: true, Known: true}
	}

	tests := []struct {
		name      string
		status    Status
		warnBelow int
		want      bool
	}{
		{"at the line", battery(20), 20, true},
		{"under the line", battery(18), 20, true},
		{"flat", battery(0), 20, true},
		{"over the line", battery(21), 20, false},
		{"plugged in under the line", Status{Percent: 18, Charging: true, Known: true}, 20, false},
		{"no battery", Status{}, 20, false},
		{"warning off", battery(18), 0, false},
		{"warning off by a negative", battery(18), -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.Low(tt.warnBelow); got != tt.want {
				t.Errorf("Low(%d) = %v, want %v (status %+v)", tt.warnBelow, got, tt.want, tt.status)
			}
		})
	}
}
