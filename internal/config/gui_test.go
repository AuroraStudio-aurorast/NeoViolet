package config

import (
	"encoding/json"
	"math"
	"testing"
)

func TestGUIConfigDefaultsAndRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	if err := json.Unmarshal([]byte(`{"gui":{"font_size":18},"desktop_lyrics":{"enabled":false}}`), &cfg); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]map[string]any
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	doc = make(map[string]map[string]any)
	for _, key := range []string{"gui", "desktop_lyrics"} {
		var section map[string]any
		if err := json.Unmarshal(raw[key], &section); err != nil {
			t.Fatal(err)
		}
		doc[key] = section
	}
	if doc["gui"]["font_size"] != float64(18) || doc["gui"]["minimize_button"] != true || doc["desktop_lyrics"]["font_size"] != float64(32) || doc["desktop_lyrics"]["enabled"] != false {
		t.Fatalf("preferences: %s", data)
	}
	if _, ok := doc["gui"]["window_width"]; ok {
		t.Fatal("geometry belongs to GUI state")
	}
	if _, ok := doc["desktop_lyrics"]["position"]; ok {
		t.Fatal("position belongs to GUI state")
	}
}

func TestGUINormalization(t *testing.T) {
	cfg := DefaultConfig()
	cfg.GUI.FontSize, cfg.GUI.Opacity, cfg.GUI.MonospaceFont = 999, math.NaN(), "  "
	cfg.DesktopLyrics.FontSize, cfg.DesktopLyrics.Opacity, cfg.DesktopLyrics.Highlight = 0, -1, "red"
	if !cfg.Normalize() {
		t.Fatal("repairs must be reported")
	}
	if cfg.GUI.FontSize != 48 || cfg.GUI.Opacity != 1 || cfg.GUI.MonospaceFont == "" || cfg.DesktopLyrics.FontSize != 22 || cfg.DesktopLyrics.Opacity != .4 || cfg.DesktopLyrics.Highlight != "violet" {
		t.Fatalf("invalid normalization: %+v %+v", cfg.GUI, cfg.DesktopLyrics)
	}
	if cfg.Normalize() {
		t.Fatal("normalization must be stable")
	}
}
