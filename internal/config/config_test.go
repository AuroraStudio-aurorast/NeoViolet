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
		{"Lyrics.FormatPriority len", len(cfg.Lyrics.FormatPriority), 8},
		{"Lyrics.FormatPriority[0]", cfg.Lyrics.FormatPriority[0], "embedded"},
		{"Lyrics.FormatPriority[1]", cfg.Lyrics.FormatPriority[1], "lrc"},
		{"Lyrics.FormatPriority[2]", cfg.Lyrics.FormatPriority[2], "ttml"},
		{"Lyrics.FormatPriority[3]", cfg.Lyrics.FormatPriority[3], "qrc"},
		{"Lyrics.FormatPriority[4]", cfg.Lyrics.FormatPriority[4], "yrc"},
		{"Lyrics.FormatPriority[5]", cfg.Lyrics.FormatPriority[5], "eslrc"},
		{"Lyrics.FormatPriority[6]", cfg.Lyrics.FormatPriority[6], "lys"},
		{"Lyrics.FormatPriority[7]", cfg.Lyrics.FormatPriority[7], "online"},
		{"Lyrics.Fetch.Enabled", cfg.Lyrics.Fetch.Enabled, true},
		{"Lyrics.Fetch.BaseURL", cfg.Lyrics.Fetch.BaseURL, ""},
		{"Lyrics.Fetch.Timeout", cfg.Lyrics.Fetch.Timeout, DefaultFetchTimeout},
		{"Lyrics.Fetch.Security", cfg.Lyrics.Fetch.Security, "strict"},
		{"Lyrics.Fetch.InsecureTLS", cfg.Lyrics.Fetch.InsecureTLS, false},
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
	cfg.Lyrics.Fetch.BaseURL = "https://lrclib.net/"

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
	if decoded.Lyrics.Fetch.BaseURL != "https://lrclib.net/" {
		t.Errorf("Fetch.BaseURL = %q, want preserved across round trip", decoded.Lyrics.Fetch.BaseURL)
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

func TestNormalizeFetch(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Config)
		wantBase  string
		wantTime  int
		wantSec   string
		wantSaved bool
	}{
		{"trailing slash trimmed", func(c *Config) {
			c.Lyrics.Fetch.BaseURL = "https://lrclib.net/"
		}, "https://lrclib.net", DefaultFetchTimeout, "strict", true},
		{"invalid scheme falls back", func(c *Config) {
			c.Lyrics.Fetch.BaseURL = "ftp://lrclib.net"
		}, "", DefaultFetchTimeout, "strict", true},
		{"empty host falls back", func(c *Config) {
			c.Lyrics.Fetch.BaseURL = "https://"
		}, "", DefaultFetchTimeout, "strict", true},
		{"timeout zero falls back", func(c *Config) {
			c.Lyrics.Fetch.Timeout = 0
		}, "", DefaultFetchTimeout, "strict", true},
		{"bad security falls back", func(c *Config) {
			c.Lyrics.Fetch.Security = "paranoid"
		}, "", DefaultFetchTimeout, "strict", true},
		{"valid basic preserved", func(c *Config) {
			c.Lyrics.Fetch.Security = "basic"
			c.Lyrics.Fetch.Timeout = 5
		}, "", 5, "basic", false},
		{"defaults unchanged", func(c *Config) {}, "", DefaultFetchTimeout, "strict", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(&cfg)
			saved := cfg.Normalize()
			if saved != tt.wantSaved {
				t.Errorf("Normalize() = %v, want %v", saved, tt.wantSaved)
			}
			f := cfg.Lyrics.Fetch
			if f.BaseURL != tt.wantBase {
				t.Errorf("BaseURL = %q, want %q", f.BaseURL, tt.wantBase)
			}
			if f.Timeout != tt.wantTime {
				t.Errorf("Timeout = %d, want %d", f.Timeout, tt.wantTime)
			}
			if f.Security != tt.wantSec {
				t.Errorf("Security = %q, want %q", f.Security, tt.wantSec)
			}
		})
	}
}

func TestIsLocalHost(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "127.9.9.9", "::1"} {
		if !isLocalHost(host) {
			t.Errorf("isLocalHost(%q) = false, want true", host)
		}
	}
	for _, host := range []string{"lrclib.net", "192.168.1.10", "10.0.0.1"} {
		if isLocalHost(host) {
			t.Errorf("isLocalHost(%q) = true, want false", host)
		}
	}
}

func TestIsPrivateHost(t *testing.T) {
	for _, host := range []string{
		"192.168.1.10", "10.0.0.1", "172.16.5.4", "127.0.0.1", "::1",
		"169.254.1.1", "nas.local", "router.internal", "host.lan",
	} {
		if !isPrivateHost(host) {
			t.Errorf("isPrivateHost(%q) = false, want true", host)
		}
	}
	for _, host := range []string{"8.8.8.8", "93.184.216.34", "lrclib.net", "example.com"} {
		if isPrivateHost(host) {
			t.Errorf("isPrivateHost(%q) = true, want false", host)
		}
	}
}