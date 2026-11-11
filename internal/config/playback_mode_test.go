package config

import (
	"encoding/json"
	"testing"
)

func TestPlaybackModeDefaultsAndNormalization(t *testing.T) {
	for _, mode := range []string{"", "unknown", "order", "repeat_one", "shuffle"} {
		t.Run(mode, func(t *testing.T) {
			cfg := DefaultConfig()
			data, err := json.Marshal(map[string]string{"playback_mode": mode})
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatal(err)
			}
			cfg.Normalize()
			want := mode
			if mode == "" || mode == "unknown" {
				want = "order"
			}
			data, err = json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			var stored map[string]any
			if err := json.Unmarshal(data, &stored); err != nil {
				t.Fatal(err)
			}
			if stored["playback_mode"] != want {
				t.Fatalf("playback_mode = %v, want %s", stored["playback_mode"], want)
			}
			if cfg.Normalize() {
				t.Fatal("normalization was not idempotent")
			}
		})
	}
}

func TestPlaybackModeLoadAndSave(t *testing.T) {
	SetXDGConfig(true)
	t.Cleanup(func() { SetXDGConfig(false) })
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, input := range []string{`{}`, `{"playback_mode":"shuffle"}`, `{"playback_mode":"repeat_one"}`} {
		cfg := DefaultConfig()
		if err := json.Unmarshal([]byte(input), &cfg); err != nil {
			t.Fatal(err)
		}
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(loaded)
		if err != nil {
			t.Fatal(err)
		}
		var stored map[string]any
		if err := json.Unmarshal(data, &stored); err != nil {
			t.Fatal(err)
		}
		want := "order"
		var fields map[string]string
		if err := json.Unmarshal([]byte(input), &fields); err != nil {
			t.Fatal(err)
		}
		if fields["playback_mode"] != "" {
			want = fields["playback_mode"]
		}
		if stored["playback_mode"] != want {
			t.Fatalf("round trip %s: playback_mode = %v, want %s", input, stored["playback_mode"], want)
		}
	}
}
