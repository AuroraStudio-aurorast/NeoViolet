package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func TestSettingsFirstScreenPrioritizesLibraryPlaybackAndLyrics(t *testing.T) {
	m := wiredSettingsModel(t)
	m.UI.Width = 80
	m.UI.Height = 24
	if m.pages.settings[0].key != "library.roots" {
		t.Fatal("music folder setup is not the first action")
	}
	body := stripANSI(renderContent(m, m.layoutPlan()))
	for _, label := range []string{"Music folders", "Remembered volume", "Seek step", "Show lyrics", "Online lyrics"} {
		if !strings.Contains(body, label) {
			t.Fatalf("core setting %s is not on first screen", label)
		}
	}
	for _, label := range []string{"UI tick rate", "Service URL", "Allow insecure TLS", "Progress characters", "History limit"} {
		if strings.Contains(body, label) {
			t.Fatalf("advanced setting %s displaced core controls", label)
		}
	}
}
func TestSettingsPriorityKeepsAllFieldsAndGroupShortcutsReachable(t *testing.T) {
	m := wiredSettingsModel(t)
	expected := []string{"library.roots", "default_volume", "volume_step", "seek_step", "track_switch_mode", "lyrics.enabled", "lyrics.fetch.enabled", "lyrics.panel.mode", "lyrics.format_priority", "icon_theme", "accent.auto_accent", "progress_bar.show_percentage", "volume_bar.show_percentage", "soundfont_path", "tracker_backend", "anim.auto", "anim.photosensitivity.mode", "anim.power.warn_below", "lyrics.panel.width", "lyrics.panel.context_lines", "lyrics.scroll_speed", "lyrics.fetch.base_url", "lyrics.fetch.timeout", "lyrics.fetch.security", "lyrics.fetch.insecure_tls", "progress_bar.fill", "progress_bar.scaled", "volume_bar.fill", "volume_bar.width", "tick_rate", "command_history.max", "error.duration"}
	keys := []string{}
	previous := -1
	for _, field := range m.pages.settings {
		keys = append(keys, field.key)
		if field.group < previous {
			t.Fatal("group headers repeat out of order")
		}
		previous = field.group
	}
	if !slices.Equal(keys, expected) {
		t.Fatalf("priority order lost/duplicated setting keys: %v", keys)
	}
	for group, want := range []string{"library.roots", "lyrics.enabled", "icon_theme", "soundfont_path", "anim.auto", "lyrics.panel.width"} {
		digit := rune('1' + group)
		pageKey(m, digit, string(digit))
		if m.pages.settings[m.pages.cursors[3]].key != want {
			t.Fatalf("shortcut %c reached wrong group", digit)
		}
	}
	// The old positional volume label override must not rename Volume step.
	if m.pages.settings[settingIndex(m, "Remembered volume")].key != "default_volume" || m.pages.settings[settingIndex(m, "Volume step")].key != "volume_step" {
		t.Fatal("labels no longer match configuration keys")
	}
}
func TestReorderedSettingsEditorsStillSaveCorrectConfig(t *testing.T) {
	m := wiredSettingsModel(t)
	pageKey(m, '4', "4")
	pageKey(m, tea.KeyEnter, "")
	if m.pages.browser == nil || m.pages.browser.purpose != "soundfont" {
		t.Fatal("engine shortcut no longer opens SoundFont")
	}
	pageKey(m, tea.KeyEscape, "")
	editRealSetting(t, m, "Seek step", "21")
	editRealSetting(t, m, "Remembered volume", "60%")
	editRealSetting(t, m, "UI tick rate", "24")
	completeSettingsCmd(t, m, pageKey(m, 's', "s"))
	cfg, err := config.Load()
	if err != nil || cfg.SeekStep != 21 || cfg.DefaultVolume != .6 || cfg.TickRate != 24 || cfg.VolumeStep != .1 {
		t.Fatal("reordered fields saved to wrong config keys")
	}
	if m.pages.settings[m.pages.cursors[3]].key != "tick_rate" {
		t.Fatal("save moved current setting")
	}
}

func TestSettingsFirstGroupHeaderReturnsAfterScrolling(t *testing.T) {
	for _, height := range []int{17, 24, 30} {
		for _, back := range []string{"arrows", "home", "shortcut"} {
			t.Run(fmt.Sprintf("height%d/%s", height, back), func(t *testing.T) {
				m := wiredSettingsModel(t)
				m.UI.Width = 80
				m.UI.Height = height
				pageKey(m, 'G', "G")
				switch back {
				case "arrows":
					for range len(m.pages.settings) {
						pageKey(m, tea.KeyUp, "")
					}
				case "home":
					pageKey(m, tea.KeyHome, "")
				case "shortcut":
					pageKey(m, '1', "1")
				}
				c := selectedCanvas(m)
				rows := strings.Split(stripANSI(c.String()), "\n")
				if !strings.HasPrefix(strings.TrimSpace(rows[3]), "Library & Playback") || !strings.Contains(rows[4], "Music folders") {
					t.Fatalf("heading did not return above first field: %q", rows[3:5])
				}
				if len(c.selections) != 1 || c.selections[0].row != 4 {
					t.Fatal("heading became selectable or current field moved")
				}
				if m.pages.viewports.settings.top != 0 {
					t.Fatal("Update did not restore heading in retained viewport")
				}
			})
		}
	}
}
func TestSettingsOtherGroupHeaderAppearsWhenFirstFieldReachesTop(t *testing.T) {
	m := wiredSettingsModel(t)
	m.UI.Width = 80
	m.UI.Height = 17
	pageKey(m, 'G', "G")
	target := settingIndex(m, "Icon theme")
	for m.pages.cursors[3] > target {
		pageKey(m, tea.KeyUp, "")
	}
	c := selectedCanvas(m)
	rows := strings.Split(stripANSI(c.String()), "\n")
	if !strings.HasPrefix(strings.TrimSpace(rows[3]), "Appearance") || !strings.Contains(rows[4], "Icon theme") {
		t.Fatalf("group heading stayed hidden: %q", rows[3:5])
	}
	before := m.pages.viewports.settings.top
	pageKey(m, tea.KeyDown, "")
	if m.pages.viewports.settings.top != before || selectedCanvas(m).selections[0].row != 5 {
		t.Fatal("heading context broke normal in-window movement")
	}
}
