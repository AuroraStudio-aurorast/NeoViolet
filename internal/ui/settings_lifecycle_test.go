package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func TestSettingsVolumeEditUsesLatestLiveBaseline(t *testing.T) {
	for _, draftFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "untouched", true: "edited"}[draftFirst], func(t *testing.T) {
			m := wiredSettingsModel(t)
			if draftFirst {
				editRealSetting(t, m, "Remembered volume", "25%")
			}
			m.setVolume(.7)
			if draftFirst && m.pages.draft.DefaultVolume != .25 {
				t.Fatal("live volume overwrote the explicit draft")
			}
			editRealSetting(t, m, "Remembered volume", "40%")
			if !m.pages.dirty || !m.pages.changed["default_volume"] {
				t.Fatal("returning to the original volume ignored the new live baseline")
			}
			completeSettingsCmd(t, m, m.saveSettings(false))
			disk, err := config.Load()
			if err != nil || disk.DefaultVolume != .4 || m.Audio.Volume != .4 {
				t.Fatalf("requested volume was not saved/applied: disk=%v live=%v err=%v", disk.DefaultVolume, m.Audio.Volume, err)
			}
		})
	}
}

func TestSettingsLiveVolumeMatchingDraftClearsOnlyVolumeChange(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Remembered volume", "25%")
	editRealSetting(t, m, "Seek step", "21")
	m.setVolume(.25)
	if m.pages.changed["default_volume"] || !m.pages.changed["seek_step"] || !m.pages.dirty {
		t.Fatal("matching live volume left stale dirty state or discarded another draft")
	}
	editRealSetting(t, m, "Seek step", "13")
	if m.pages.dirty {
		t.Fatal("all reverted preferences remained dirty")
	}
}

func TestSettingsCancelQuitRestoresBrowserContext(t *testing.T) {
	for _, location := range []bool{false, true} {
		t.Run(map[bool]string{false: "browser", true: "location"}[location], func(t *testing.T) {
			m := wiredSettingsModel(t)
			editRealSetting(t, m, "Seek step", "21")
			dir := browserFixture(t)
			browserResult(t, m, m.openFileBrowser("audio", -1, dir))
			browser := m.pages.browser
			browser.cursor = 1
			if location {
				_, _ = handleKeyPress(m, tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
				m.pages.editor.input.SetValue(filepath.Join(dir, "z Folder"))
			}
			_, _ = runQuit(m, invocation{})
			if !m.pages.quitSettings {
				t.Fatal("dirty settings did not prompt")
			}
			pageKey(m, tea.KeyEscape, "")
			if m.pages.browser != browser || browser.cursor != 1 || !m.pages.dirty {
				t.Fatal("cancel quit discarded browser context or the settings draft")
			}
			if location {
				cmd := pageKey(m, tea.KeyEnter, "")
				if cmd == nil {
					t.Fatal("restored location editor cannot navigate")
				}
				browserResult(t, m, cmd)
				if browser.directory != filepath.Join(dir, "z Folder") || m.pages.editor != nil {
					t.Fatal("location editor did not complete navigation after cancel")
				}
			}
		})
	}
}

func TestSettingsCleanupFinishesWriterBeforeCommandStarts(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	cmd := m.saveSettings(false)
	done := make(chan struct{})
	go func() { m.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		// Unblock the old implementation so the test does not leak a worker.
		_ = cmd()
		<-done
		t.Fatal("shutdown waited for a save command that never started")
	}
	disk, err := config.Load()
	if err != nil || disk.SeekStep != 21 || m.configSave != nil {
		t.Fatalf("shutdown lost settings: %+v %v", disk, err)
	}
	// A late execution of the original command must not overwrite a newer file.
	disk.SeekStep = 37
	if err := disk.Save(); err != nil {
		t.Fatal(err)
	}
	_, _ = m.Update(cmd())
	disk, err = config.Load()
	if err != nil || disk.SeekStep != 37 || m.configSave != nil {
		t.Fatal("late save command rewrote config or resurrected its job")
	}
}

func TestSettingsCleanupPreservesSaveFailure(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config.json"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := m.saveSettings(false)
	msg := cmd()
	m.Close()
	_, _ = m.Update(msg)
	if !m.Error.Visible || !m.pages.dirty || m.Config.SeekStep != 13 || m.configSave != nil {
		t.Fatal("shutdown lost save error or applied an unsaved draft")
	}
}

func TestSettingsCleanupJoinsConcurrentCommandAndKeepsLatestVolume(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	cmd := m.saveSettings(false)
	m.setVolume(.7)
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	m.Close()
	_, _ = m.Update(<-result)
	disk, err := config.Load()
	if err != nil || disk.SeekStep != 21 || disk.DefaultVolume != .7 || m.configSave != nil {
		t.Fatalf("concurrent shutdown lost saved settings/latest volume: %+v %v", disk, err)
	}
}

func TestSettingsQuitIgnoresPendingBrowserSelection(t *testing.T) {
	for _, purpose := range []string{"audio", "soundfont"} {
		t.Run(purpose, func(t *testing.T) {
			m := wiredSettingsModel(t)
			t.Cleanup(m.Close)
			editRealSetting(t, m, "Seek step", "21")
			dir := browserFixture(t)
			field := settingIndex(m, "MIDI SoundFont")
			browserResult(t, m, m.openFileBrowser(purpose, field, dir))
			browser := m.pages.browser
			name := "song with  spaces, 雨.FLAC"
			if purpose == "soundfont" {
				name = "orchestra.SF2"
			}
			pending := m.fileBrowserLocation(filepath.Join(dir, name))
			_, _ = runQuit(m, invocation{})
			_, _ = m.Update(pending())
			if m.pages.browser != browser || browser.loading || m.Loading || m.pages.draft.SoundfontPath != "" {
				t.Fatal("pending selection changed playback or the draft during quit confirmation")
			}
			pageKey(m, tea.KeyEscape, "")
			if m.pages.browser != browser || m.pages.quitSettings {
				t.Fatal("cancel did not retain the last accepted browser context")
			}
		})
	}
}
