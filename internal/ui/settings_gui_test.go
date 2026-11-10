package ui

import (
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func TestSettingsGUIAndDesktopLyricsSave(t *testing.T) {
	m := wiredSettingsModel(t)
	for _, tt := range []struct{ label, value string }{
		{"Terminal font", "Test Mono"}, {"Terminal font size", "20"}, {"Terminal opacity", "80%"},
		{"Desktop font size", "28"}, {"Desktop opacity", "60%"},
	} {
		editRealSetting(t, m, tt.label, tt.value)
	}
	pageKey(m, '7', "7")
	if m.pages.settings[m.pages.cursors[3]].key != "desktop_lyrics.enabled" {
		t.Fatal("desktop group unreachable")
	}
	completeSettingsCmd(t, m, pageKey(m, 's', "s"))
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GUI.MonospaceFont != "Test Mono" || cfg.GUI.FontSize != 20 || cfg.GUI.Opacity != .8 || cfg.DesktopLyrics.FontSize != 28 || cfg.DesktopLyrics.Opacity != .6 {
		t.Fatalf("saved preferences: %+v %+v", cfg.GUI, cfg.DesktopLyrics)
	}
}

func TestSettingsGUIValidation(t *testing.T) {
	for _, tt := range []struct{ key, value string }{
		{"gui.font_size", "7"}, {"gui.font_size", "49"}, {"gui.opacity", "9%"},
		{"desktop_lyrics.font_size", "21"}, {"desktop_lyrics.opacity", "39%"},
		{"gui.monospace_font", ""}, {"gui.monospace_font", "bad\nfont"},
	} {
		if _, err := parseSetting(pageSetting{key: tt.key}, tt.value); err == nil {
			t.Fatalf("accepted %s=%q", tt.key, tt.value)
		}
	}
}
