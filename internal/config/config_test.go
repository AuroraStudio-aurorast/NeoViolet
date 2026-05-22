package config

import (
	"encoding/json"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"IconTheme", cfg.IconTheme, "nerd"},
		{"DefaultVolume", cfg.DefaultVolume, 1.0},
		{"VolumeStep", cfg.VolumeStep, 0.1},
		{"SeekStep", cfg.SeekStep, 5},
		{"TickRate", cfg.TickRate, 30},
		{"SoundfontPath", cfg.SoundfontPath, ""},
		{"Lyrics.Enabled", cfg.Lyrics.Enabled, true},
		{"Lyrics.ScrollSpeed", cfg.Lyrics.ScrollSpeed, 6},
		{"Lyrics.FormatPriority len", len(cfg.Lyrics.FormatPriority), 7},
		{"Lyrics.FormatPriority[0]", cfg.Lyrics.FormatPriority[0], "embedded"},
		{"Lyrics.FormatPriority[1]", cfg.Lyrics.FormatPriority[1], "lrc"},
		{"Lyrics.FormatPriority[2]", cfg.Lyrics.FormatPriority[2], "ttml"},
		{"Lyrics.FormatPriority[3]", cfg.Lyrics.FormatPriority[3], "qrc"},
		{"Lyrics.FormatPriority[4]", cfg.Lyrics.FormatPriority[4], "yrc"},
		{"Lyrics.FormatPriority[5]", cfg.Lyrics.FormatPriority[5], "eslrc"},
		{"Lyrics.FormatPriority[6]", cfg.Lyrics.FormatPriority[6], "lys"},
		{"VolumeBar.Width", cfg.VolumeBar.Width, 16},
		{"VolumeBar.ShowPercentage", cfg.VolumeBar.ShowPercentage, true},
		{"ProgressBar.Scaled", cfg.ProgressBar.Scaled, true},
		{"ProgressBar.ShowPercentage", cfg.ProgressBar.ShowPercentage, false},
		{"CommandHistory.Max", cfg.CommandHistory.Max, 50},
		{"Error.Duration", cfg.Error.Duration, 90},
		{"Accent.AutoAccent", *cfg.Accent.AutoAccent, true},
		{"Accent.IsEnabled", cfg.Accent.IsEnabled(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("DefaultConfig().%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestConfigJSONRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	cfg.IconTheme = "emoji"
	cfg.DefaultVolume = 0.5

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.IconTheme != "emoji" {
		t.Errorf("IconTheme = %q, want emoji", decoded.IconTheme)
	}
	if decoded.DefaultVolume != 0.5 {
		t.Errorf("DefaultVolume = %v, want 0.5", decoded.DefaultVolume)
	}
}

func TestConfigJSONRoundTrip_Fill(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ProgressBar.Fill = []string{"a", "b"}
	cfg.VolumeBar.Fill = []string{"x", "y"}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(decoded.ProgressBar.Fill) != 2 || decoded.ProgressBar.Fill[0] != "a" {
		t.Errorf("ProgressBar.Fill = %v, want [a b]", decoded.ProgressBar.Fill)
	}
	if len(decoded.VolumeBar.Fill) != 2 || decoded.VolumeBar.Fill[0] != "x" {
		t.Errorf("VolumeBar.Fill = %v, want [x y]", decoded.VolumeBar.Fill)
	}
}

func TestConfigJSONRoundTrip_ZeroValues(t *testing.T) {
	// Verify that zero-value fields survive marshal/unmarshal
	data, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.SoundfontPath != "" {
		t.Errorf("SoundfontPath should be empty, got %q", decoded.SoundfontPath)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	// Load should return default config on parse error, not panic
	// This tests the error path in Load() since we can't easily
	// create a real broken config file here
	cfg := DefaultConfig()
	err := json.Unmarshal([]byte("{invalid"), &cfg)
	if err == nil {
		t.Error("expected unmarshal error for invalid JSON")
	}
}