package ui

import (
	"testing"
	"time"
)

func TestFormatDuration_UnderOneHour(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00"},
		{30 * time.Second, "0:30"},
		{90 * time.Second, "1:30"},
		{59*time.Minute + 59*time.Second, "59:59"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatDuration_OverOneHour(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{1 * time.Hour, "1:00:00"},
		{1*time.Hour + 30*time.Minute, "1:30:00"},
		{2*time.Hour + 15*time.Minute + 30*time.Second, "2:15:30"},
		{24*time.Hour + 59*time.Minute + 59*time.Second, "24:59:59"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestErrorState_Set(t *testing.T) {
	e := &ErrorState{}
	e.Set("test error", 90)

	if e.Message != "test error" {
		t.Errorf("Message = %q, want %q", e.Message, "test error")
	}
	if e.Timer != 90 {
		t.Errorf("Timer = %d, want %d", e.Timer, 90)
	}
	if !e.Visible {
		t.Error("Visible = false, want true")
	}
}

func TestErrorState_Tick(t *testing.T) {
	e := &ErrorState{}
	e.Set("test", 3)

	e.Tick()
	if e.Timer != 2 || !e.Visible {
		t.Errorf("after tick 1: Timer=%d Visible=%v", e.Timer, e.Visible)
	}
	e.Tick()
	if e.Timer != 1 || !e.Visible {
		t.Errorf("after tick 2: Timer=%d Visible=%v", e.Timer, e.Visible)
	}
	e.Tick()
	if e.Visible || e.Message != "" {
		t.Errorf("after tick 3: Visible=%v Message=%q, want Visible=false Message=\"\"", e.Visible, e.Message)
	}
}

func TestErrorState_Tick_NotVisible(t *testing.T) {
	e := &ErrorState{}
	e.Tick()
	if e.Visible {
		t.Error("Visible should remain false")
	}
}

func TestAudioState_AdjustVolume_Clamping(t *testing.T) {
	a := &AudioState{Volume: 0.5}
	a.AdjustVolume(0.7)
	if a.Volume != 1.0 {
		t.Errorf("overshoot: Volume=%f, want 1.0", a.Volume)
	}

	a.AdjustVolume(-1.5)
	if a.Volume != 0.0 {
		t.Errorf("undershoot: Volume=%f, want 0.0", a.Volume)
	}
}

func TestAudioState_SetVolume_Clamping(t *testing.T) {
	a := &AudioState{Volume: 0.5}
	a.SetVolume(1.5)
	if a.Volume != 1.0 {
		t.Errorf("overshoot: Volume=%f, want 1.0", a.Volume)
	}

	a.SetVolume(-0.5)
	if a.Volume != 0.0 {
		t.Errorf("undershoot: Volume=%f, want 0.0", a.Volume)
	}
}

func TestAudioState_AdjustVolume_Normal(t *testing.T) {
	a := &AudioState{Volume: 0.3}
	a.AdjustVolume(0.2)
	if a.Volume != 0.5 {
		t.Errorf("Volume=%f, want 0.5", a.Volume)
	}

	a.AdjustVolume(-0.1)
	if a.Volume != 0.4 {
		t.Errorf("Volume=%f, want 0.4", a.Volume)
	}
}
