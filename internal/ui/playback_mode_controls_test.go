package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func modeCommand(t *testing.T, m *Model, text string) {
	t.Helper()
	inv, ok := parseInvocation(text)
	if !ok {
		t.Fatal("invalid command fixture")
	}
	spec, ok := commandLookup(inv.Name)
	if !ok {
		t.Fatal("mode command is not registered")
	}
	_, _ = spec.Run(m, inv)
}

func TestPlaybackModeKeyboardCyclesWithoutStartingPlayback(t *testing.T) {
	m := wiredSettingsModel(t)
	m.UI.Focus = FocusFooter
	for _, want := range []string{config.PlaybackRepeatOne, config.PlaybackShuffle, config.PlaybackOrder} {
		pageKey(m, 'm', "m")
		if m.Config.PlaybackMode != want || m.Audio.IsPlaying || config.Exists() {
			t.Fatal("mode key did not cycle in memory without starting audio or writing synchronously")
		}
	}
	found := false
	for _, binding := range shortHelpBindings(m) {
		if binding.Help().Key == "m" && binding.Help().Desc == "mode" {
			found = true
		}
	}
	if !found {
		t.Fatal("footer focus does not advertise the mode key")
	}
}

func TestPlaybackModeKeyRespectsTextInputOwners(t *testing.T) {
	m := wiredSettingsModel(t)
	m.UI.Mode = ModeCommand
	m.Components.CommandInput.Focus()
	handleKeyPress(m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if m.Config.PlaybackMode != config.PlaybackOrder || m.Components.CommandInput.Value() != "m" {
		t.Fatal("mode shortcut stole command input")
	}
	m.UI.Mode = ModeNormal
	m.pages.cursors[3] = settingIndex(m, "Seek step")
	pageKey(m, tea.KeyEnter, "")
	pageKey(m, 'm', "m")
	if m.Config.PlaybackMode != config.PlaybackOrder || !strings.Contains(m.pages.editor.input.Value(), "m") {
		t.Fatal("mode shortcut stole settings editor input")
	}
}

func TestPlaybackModeCommandQueriesValidatesAndCompletes(t *testing.T) {
	m := wiredSettingsModel(t)
	modeCommand(t, m, "mode")
	if !m.Info.Visible || !strings.Contains(m.Info.Message, "Order") {
		t.Fatal("mode query did not report the effective mode")
	}
	for _, mode := range []string{config.PlaybackShuffle, config.PlaybackRepeatOne, config.PlaybackOrder} {
		modeCommand(t, m, "mode "+mode)
		if m.Config.PlaybackMode != mode {
			t.Fatalf("mode command did not select %s", mode)
		}
	}
	for _, text := range []string{"mode unknown", "mode order extra"} {
		m.Error.Visible = false
		modeCommand(t, m, text)
		if !m.Error.Visible || m.Config.PlaybackMode != config.PlaybackOrder {
			t.Fatal("invalid mode command was accepted")
		}
	}
	ctx := completionContextAt("mode ", len("mode "))
	candidates := candidatesFor(ctx)
	if len(candidates) != 3 {
		t.Fatalf("mode completion has %d choices, want 3", len(candidates))
	}
}

func TestPlaybackModeSettingIsDraftUntilSaved(t *testing.T) {
	m := wiredSettingsModel(t)
	i := settingIndex(m, "Playback mode")
	if i < 0 {
		t.Fatal("missing Playback mode setting")
	}
	m.pages.cursors[3] = i
	pageKey(m, tea.KeyEnter, "")
	if m.pages.draft.PlaybackMode != config.PlaybackRepeatOne || m.Config.PlaybackMode != config.PlaybackOrder {
		t.Fatal("settings draft changed the effective playback mode")
	}
	completeSettingsCmd(t, m, m.saveSettings(false))
	disk, err := config.Load()
	if err != nil || disk.PlaybackMode != config.PlaybackRepeatOne || m.Config.PlaybackMode != disk.PlaybackMode {
		t.Fatal("saved mode was not applied and persisted")
	}
	pageKey(m, 'r', "r")
	completeSettingsCmd(t, m, m.saveSettings(false))
	if m.Config.PlaybackMode != config.PlaybackOrder {
		t.Fatal("reset did not restore ordered playback")
	}
}

func TestPlaybackModeRuntimeSavePreservesDraftAndLatestVolume(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	modeCommand(t, m, "mode shuffle")
	m.setVolume(.7)
	first := m.startVolumeSave(time.Now().Add(time.Second))
	if first == nil {
		t.Fatal("mode/volume changes did not schedule persistence")
	}
	modeCommand(t, m, "mode repeat_one")
	m.setVolume(.8)
	_, _ = m.Update(first())
	second := m.startVolumeSave(time.Now().Add(time.Second))
	if second == nil {
		t.Fatal("changes during persistence did not schedule a followup")
	}
	_, _ = m.Update(second())
	disk, err := config.Load()
	if err != nil || disk.PlaybackMode != config.PlaybackRepeatOne || disk.DefaultVolume != .8 || disk.SeekStep != 13 {
		t.Fatalf("runtime save lost latest preferences or persisted an unrelated draft: %+v %v", disk, err)
	}
	if m.pages.draft.SeekStep != 21 || !m.pages.dirty {
		t.Fatal("runtime mode change discarded an unrelated draft")
	}
}

func TestPlaybackModeNewerThanSettingsSnapshotWins(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	settings := m.saveSettings(false)
	modeCommand(t, m, "mode shuffle")
	completeSettingsCmd(t, m, settings)
	if m.Config.PlaybackMode != config.PlaybackShuffle {
		t.Fatal("stale settings snapshot reverted a newer runtime mode")
	}
	cmd := m.startVolumeSave(time.Now().Add(time.Second))
	if cmd == nil {
		t.Fatal("newer mode did not receive a deferred write")
	}
	_, _ = m.Update(cmd())
	disk, err := config.Load()
	if err != nil || disk.PlaybackMode != config.PlaybackShuffle || disk.SeekStep != 21 {
		t.Fatal("mode followup lost the explicitly saved settings")
	}
}

func TestPlaybackModeExplicitCurrentModeSupersedesPendingSettings(t *testing.T) {
	m := wiredSettingsModel(t)
	i := settingIndex(m, "Playback mode")
	if err := m.pages.applyDraftValue(i, config.PlaybackShuffle); err != nil {
		t.Fatal(err)
	}
	settings := m.saveSettings(false)
	modeCommand(t, m, "mode order")
	completeSettingsCmd(t, m, settings)
	if m.Config.PlaybackMode != config.PlaybackOrder {
		t.Fatal("pending settings overrode a newer explicit request for the current mode")
	}
	m.Close()
	disk, err := config.Load()
	if err != nil || disk.PlaybackMode != config.PlaybackOrder {
		t.Fatal("shutdown did not persist the explicitly requested mode")
	}
}

func TestPlaybackModeChangePreservesPlaybackAndResetsShuffle(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackOrder)
	p := m.Audio.Player
	p.Pause()
	id := m.session.current
	modeCommand(t, m, "mode shuffle")
	if m.Audio.Player != p || p.IsPlaying() || m.session.current != id {
		t.Fatal("changing mode replaced or resumed the current track")
	}
	shuffleNext(t, m)
	modeCommand(t, m, "mode order")
	modeCommand(t, m, "mode shuffle")
	if m.stepQueue(-1) != nil {
		t.Fatal("re-entering shuffle retained obsolete listening history")
	}
}

func TestPlaybackModeFailedSaveRetainsModeUntilShutdownRetry(t *testing.T) {
	m := wiredSettingsModel(t)
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	obstacle := filepath.Join(dir, "config.json")
	if err := os.MkdirAll(obstacle, 0700); err != nil {
		t.Fatal(err)
	}
	modeCommand(t, m, "mode shuffle")
	cmd := m.startVolumeSave(time.Now().Add(time.Second))
	if cmd == nil {
		t.Fatal("mode change was not scheduled for persistence")
	}
	_, _ = m.Update(cmd())
	if !m.Error.Visible || m.Config.PlaybackMode != config.PlaybackShuffle || m.startVolumeSave(time.Now().Add(time.Hour)) != nil {
		t.Fatal("failed mode save rolled back playback or retried on every tick")
	}
	if err := os.Remove(obstacle); err != nil {
		t.Fatal(err)
	}
	m.Close()
	disk, err := config.Load()
	if err != nil || disk.PlaybackMode != config.PlaybackShuffle {
		t.Fatal("shutdown did not retry the remembered mode")
	}
}

func TestPlaybackModeSettingsSaveJoinsEarlierRuntimeWriter(t *testing.T) {
	m := wiredSettingsModel(t)
	modeCommand(t, m, "mode shuffle")
	runtime := m.startVolumeSave(time.Now().Add(time.Second))
	editRealSetting(t, m, "Seek step", "21")
	settings := m.saveSettings(false)
	completeSettingsCmd(t, m, settings)
	_, _ = m.Update(runtime())
	disk, err := config.Load()
	if err != nil || disk.PlaybackMode != config.PlaybackShuffle || disk.SeekStep != 21 {
		t.Fatal("late runtime writer reverted newer explicit settings")
	}
}
