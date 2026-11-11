package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func TestSettingsSaveMediaCtlPreferenceForNextStart(t *testing.T) {
	m := wiredSettingsModel(t)
	controller := &mockMediaCtl{}
	m.MediaCtl = controller
	index := settingIndex(m, "System integration (restart)")
	if index < 0 {
		t.Fatal("missing system integration setting with restart notice")
	}
	m.pages.cursors[3] = index
	if got := stripANSI(renderContent(m, m.layoutPlan())); !strings.Contains(got, "System integration (restart)") {
		t.Fatal("system integration preference is not visible in settings")
	}
	if m.pages.settings[index].value != "On" {
		t.Fatal("system integration should be enabled by default")
	}
	pageKey(m, tea.KeyEnter, "")
	if settingValue(configValues(*m.Config), "mediactl.enabled") != true {
		t.Fatal("unsaved system integration edit changed the configuration")
	}
	completeSettingsCmd(t, m, pageKey(m, 's', "s"))
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if settingValue(configValues(*loaded), "mediactl.enabled") != false || m.pages.settings[index].value != "Off" {
		t.Fatal("disabled system integration preference did not survive save and reload")
	}
	if m.MediaCtl != controller || controller.closed {
		t.Fatal("saving a restart preference changed the running controller")
	}
	pageKey(m, 'r', "r")
	completeSettingsCmd(t, m, pageKey(m, 's', "s"))
	loaded, err = config.Load()
	if err != nil || settingValue(configValues(*loaded), "mediactl.enabled") != true {
		t.Fatalf("reset did not restore enabled system integration: %v", err)
	}
}
