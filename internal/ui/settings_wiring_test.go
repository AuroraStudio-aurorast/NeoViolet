package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics/fetch"
)

func wiredSettingsModel(t *testing.T) *Model {
	t.Helper()
	config.SetXDGConfig(true)
	t.Cleanup(func() { config.SetXDGConfig(false) })
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.IconTheme = "fallback"
	cfg.SeekStep = 13
	cfg.DefaultVolume = .4
	m := NewModel("", &cfg)
	m.UI.ActiveTab = 3
	m.UI.Focus = FocusContent
	return m
}
func settingIndex(m *Model, label string) int {
	for i, f := range m.pages.settings {
		if f.label == label {
			return i
		}
	}
	return -1
}
func editRealSetting(t *testing.T, m *Model, label, value string) {
	t.Helper()
	i := settingIndex(m, label)
	if i < 0 {
		t.Fatalf("missing field %s", label)
	}
	m.pages.cursors[3] = i
	pageKey(m, tea.KeyEnter, "")
	if m.pages.editor == nil {
		t.Fatalf("no editor for %s", label)
	}
	m.pages.editor.input.SetValue(value)
	pageKey(m, tea.KeyEnter, "")
}
func completeSettingsCmd(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("save did not produce work")
	}
	msg := cmd()
	_, _ = m.Update(msg)
}
func TestSettingsReadRealConfigAndKeepDraftIsolated(t *testing.T) {
	m := wiredSettingsModel(t)
	if got := m.pages.settings[settingIndex(m, "Seek step")].value; !strings.Contains(got, "13") {
		t.Fatalf("still using sample setting: %q", got)
	}
	editRealSetting(t, m, "Seek step", "21")
	if m.Config.SeekStep != 13 {
		t.Fatal("uncommitted edit mutated config")
	}
	cmd := pageKey(m, 's', "s")
	completeSettingsCmd(t, m, cmd)
	if m.Config.SeekStep != 21 {
		t.Fatal("saved draft did not become active")
	}
	loaded, err := config.Load()
	if err != nil || loaded.SeekStep != 21 {
		t.Fatalf("saved setting missing after reload: %+v %v", loaded, err)
	}
}
func TestSettingsRejectInvalidInputAndRetainEditor(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "UI tick rate", "0")
	if m.pages.editor == nil || !m.Error.Visible || m.Config.TickRate != 30 {
		t.Fatal("invalid tick rate accepted")
	}
	pageKey(m, tea.KeyEscape, "")
	editRealSetting(t, m, "Volume step", "NaN")
	if m.pages.editor == nil || m.Config.VolumeStep != .1 {
		t.Fatal("nonfinite step accepted")
	}
}
func TestSettingsSaveFailureRetainsDraftAndAppliedConfig(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(dir, "config.json"), 0700); err != nil {
		t.Fatal(err)
	}
	completeSettingsCmd(t, m, pageKey(m, 's', "s"))
	if m.Config.SeekStep != 13 || !m.pages.dirty || !m.Error.Visible {
		t.Fatal("failed save discarded draft or applied it")
	}
}

func TestSettingsSaveMergesVolumeChangesDuringWrite(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	cmd := pageKey(m, 's', "s")
	m.setVolume(.7)
	completeSettingsCmd(t, m, cmd)
	m.Close() // Shutdown flushes the volume merged after the settings snapshot.
	loaded, err := config.Load()
	if err != nil || loaded.SeekStep != 21 || loaded.DefaultVolume != .7 || m.Config.DefaultVolume != .7 || m.Audio.Volume != .7 {
		t.Fatalf("volume update lost after save: %+v %v", loaded, err)
	}
}
func TestSettingsFormatDraftDoesNotAliasAppliedConfig(t *testing.T) {
	m := wiredSettingsModel(t)
	m.pages.cursors[3] = settingIndex(m, "Format priority")
	pageKey(m, tea.KeyEnter, "")
	pageKey(m, 'J', "J")
	pageKey(m, tea.KeySpace, "")
	pageKey(m, tea.KeyEnter, "")
	if len(m.Config.Lyrics.FormatPriority) != 9 || m.Config.Lyrics.FormatPriority[0] != "embedded" {
		t.Fatal("format draft modified active config")
	}
	completeSettingsCmd(t, m, pageKey(m, 's', "s"))
	if len(m.Config.Lyrics.FormatPriority) != 8 || m.Config.Lyrics.FormatPriority[0] != "lrc" {
		t.Fatalf("format ordering not saved: %v", m.Config.Lyrics.FormatPriority)
	}
}
func TestSettingsSaveAppliesRuntimeAndDefaults(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	m.pages.cursors[3] = settingIndex(m, "Seek step")
	pageKey(m, 'r', "r")
	m.pages.cursors[3] = settingIndex(m, "Icon theme")
	pageKey(m, tea.KeyEnter, "")
	m.pages.cursors[3] = settingIndex(m, "Panel mode")
	pageKey(m, tea.KeyEnter, "")
	editRealSetting(t, m, "Remembered volume", "25%")
	completeSettingsCmd(t, m, pageKey(m, 's', "s"))
	if m.Config.SeekStep != 5 || m.Config.IconTheme != "nerd" || m.Icons.Home != NerdIcons.Home || m.panelMode != "on" || m.Audio.Volume != .25 {
		t.Fatalf("saved preferences not applied: %+v", m.Config)
	}
}
func TestSettingsSaveFailurePreventsSaveAndQuit(t *testing.T) {
	m := wiredSettingsModel(t)
	dir, _ := config.Dir()
	if err := os.MkdirAll(filepath.Join(dir, "config.json"), 0700); err != nil {
		t.Fatal(err)
	}
	_, cmd := runSaveQuit(m, invocation{})
	msg := cmd()
	_, quit := m.Update(msg)
	if quit != nil || m.exiting || !m.Error.Visible {
		t.Fatal("failed save exited the player")
	}
}

func TestSettingsUnsavedQuitOffersSaveDiscardCancel(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	_, cmd := runQuit(m, invocation{})
	if cmd != nil || m.exiting {
		t.Fatal("dirty settings were silently lost on quit")
	}
	if got := stripANSI(renderContent(m, m.layoutPlan())); !strings.Contains(got, "Unsaved settings") {
		t.Fatal("missing exit decision")
	}
	pageKey(m, tea.KeyEscape, "")
	if !m.pages.dirty {
		t.Fatal("cancel quit discarded draft")
	}
	_, _ = runQuit(m, invocation{})
	cmd = pageKey(m, 's', "s")
	if cmd == nil {
		t.Fatal("save-and-quit did not save")
	}
	msg := cmd()
	_, quit := m.Update(msg)
	if quit == nil || !m.exiting {
		t.Fatal("successful save did not complete requested quit")
	}
}

func TestSettingsVolumeUpdatesOnlyUntouchedDraftFields(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	m.setVolume(.65)
	i := settingIndex(m, "Remembered volume")
	if m.pages.settings[i].value != "65%" || m.pages.draft.SeekStep != 21 || m.Config.SeekStep != 13 {
		t.Fatal("volume update overwrote draft or left unedited field stale")
	}
}

func TestSettingsResetFormatPriorityUsesDefaults(t *testing.T) {
	m := wiredSettingsModel(t)
	m.pages.cursors[3] = settingIndex(m, "Format priority")
	pageKey(m, tea.KeyEnter, "")
	pageKey(m, 'J', "J")
	pageKey(m, tea.KeyEnter, "")
	pageKey(m, 'r', "r")
	if m.pages.draft.Lyrics.FormatPriority[0] != "embedded" || m.pages.dirty {
		t.Fatal("reset did not restore default priority and clear reverted edits")
	}
}
func TestSettingsCannotSaveAnEmptyExplicitPriority(t *testing.T) {
	m := wiredSettingsModel(t)
	m.pages.cursors[3] = settingIndex(m, "Format priority")
	pageKey(m, tea.KeyEnter, "")
	for range 9 {
		pageKey(m, tea.KeySpace, "")
		pageKey(m, tea.KeyDown, "")
	}
	pageKey(m, tea.KeyEnter, "")
	if m.pages.picker == "" {
		t.Fatal("empty selection was accepted even though it would enable parser fallback")
	}
}

func TestSettingsHiddenLyricsStayHiddenWhenFetchFinishes(t *testing.T) {
	m := wiredSettingsModel(t)
	m.Audio.CurrentSong = "Song"
	m.Audio.Artist = "Artist"
	m.LyricsFetching = true
	m.pages.cursors[3] = settingIndex(m, "Show lyrics")
	pageKey(m, tea.KeyEnter, "")
	completeSettingsCmd(t, m, pageKey(m, 's', "s"))
	_, _ = handleFetchLyricsResult(m, FetchLyricsResultMsg{Sig: m.currentSig(), Data: &lyrics.Data{}})
	if m.Audio.ShowLyrics {
		t.Fatal("late lyric fetch undid saved disabled preference")
	}
}

func TestSettingsDisabledAccentIgnoresPreviouslyRequestedColor(t *testing.T) {
	m := wiredSettingsModel(t)
	m.pages.cursors[3] = settingIndex(m, "Auto accent")
	pageKey(m, tea.KeyEnter, "")
	completeSettingsCmd(t, m, pageKey(m, 's', "s"))
	_, _ = handleAccentApply(m, AccentApplyMsg{Generation: m.activeGeneration, Accent: &accent.Accent{}})
	if m.Accent != nil {
		t.Fatal("late extraction undid saved accent preference")
	}
}

func TestSettingsNoOpVolumeDoesNotDivergeFromSavedSnapshot(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Remembered volume", "25%")
	cmd := pageKey(m, 's', "s")
	m.setVolume(.4) // No actual change to the live volume.
	completeSettingsCmd(t, m, cmd)
	loaded, err := config.Load()
	if err != nil || loaded.DefaultVolume != m.Config.DefaultVolume || loaded.DefaultVolume != m.Audio.Volume {
		t.Fatalf("saved/live volume diverged: disk=%v config=%v audio=%v %v", loaded.DefaultVolume, m.Config.DefaultVolume, m.Audio.Volume, err)
	}
}

func TestSettingsRememberedVolumeUsesPlaybackPrecision(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Remembered volume", "25.6%")
	completeSettingsCmd(t, m, pageKey(m, 's', "s"))
	if m.Config.DefaultVolume != .26 || m.Audio.Volume != .26 || m.pages.settings[settingIndex(m, "Remembered volume")].value != "26%" {
		t.Fatal("saved volume has different precision from playback")
	}
}
func TestSettingsSaveAndQuitUpgradesCurrentSave(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	cmd := pageKey(m, 's', "s")
	_, _ = runSaveQuit(m, invocation{})
	msg := cmd()
	_, quit := m.Update(msg)
	if quit == nil || !m.exiting {
		t.Fatal("wq during save lost quit intent")
	}
}
func TestSettingsEnablingLyricsLoadsCurrentSidecar(t *testing.T) {
	m := wiredSettingsModel(t)
	m.Config.Lyrics.Enabled = false
	m.pages.initSettings(*m.Config)
	m.Audio.ShowLyrics = false
	path := filepath.Join(t.TempDir(), "track.flac")
	if err := os.WriteFile(strings.TrimSuffix(path, ".flac")+".lrc", []byte("[00:00.00]Current track lyric\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m.Audio.Player = &mockPlayer{path: path}
	m.pages.cursors[3] = settingIndex(m, "Show lyrics")
	pageKey(m, tea.KeyEnter, "")
	cmd := pageKey(m, 's', "s")
	_, follow := m.Update(cmd())
	if follow == nil {
		t.Fatal("enabling lyrics did not load current track")
	}
	msg := follow()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			_, _ = m.Update(child())
		}
	} else {
		_, _ = m.Update(msg)
	}
	if m.Audio.Lyrics == nil || !m.Audio.ShowLyrics {
		t.Fatal("current sidecar was not installed")
	}
}

func TestSettingsDisabledDefaultStillAllowsManualOnlineLyrics(t *testing.T) {
	m := wiredSettingsModel(t)
	m.Config.Lyrics.Enabled = false
	m.Audio.ShowLyrics = false
	m.Audio.CurrentSong = "Song"
	m.Audio.Artist = "Artist"
	m.Audio.Player = &mockPlayer{}
	_, cmd := executeLrcCommand(m, []string{"lrc", "switch", "online"})
	if cmd == nil {
		t.Fatal("manual request was blocked by saved default")
	}
	_, _ = handleFetchLyricsResult(m, FetchLyricsResultMsg{Revision: m.settingsLyricsRevision, Sig: m.currentSig(), Data: &lyrics.Data{}})
	if !m.Audio.ShowLyrics {
		t.Fatal("saved default blocked an explicitly requested lyric result")
	}
}
func TestSettingsLateSidecarDoesNotUndoExplicitLyricsOff(t *testing.T) {
	m := wiredSettingsModel(t)
	m.Audio.Player = &mockPlayer{path: filepath.Join(t.TempDir(), "song.flac")}
	_ = m.loadSettingsLyrics()
	revision := m.settingsLyricsRevision
	_, _ = executeLrcCommand(m, []string{"lrc", "off"})
	_, _ = m.Update(settingsLyricsMsg{generation: m.activeGeneration, revision: revision, data: &lyrics.Data{}})
	if m.Audio.ShowLyrics || m.LyricsFetching {
		t.Fatal("late settings sidecar undid explicit off")
	}
}

func TestSettingsInvalidationDoesNotRetainOldPendingCache(t *testing.T) {
	m := wiredSettingsModel(t)
	sig := m.currentSig()
	old := m.fetchCache
	old.Store(sig, fetch.CachePending, nil)
	_, _ = executeLrcCommand(m, []string{"lrc", "off"})
	_, _ = handleFetchLyricsResult(m, FetchLyricsResultMsg{Sig: sig, Err: fetch.ErrOffline})
	if _, _, ok := m.fetchCache.Lookup(sig); ok {
		t.Fatal("old request still blocks later lyric fetches")
	}
	// A stale worker may finish later; it cannot write into the active request cache.
	old.Store(sig, fetch.CacheNotFound, nil)
	if _, _, ok := m.fetchCache.Lookup(sig); ok {
		t.Fatal("stale worker overwrote the new request cache")
	}
}
