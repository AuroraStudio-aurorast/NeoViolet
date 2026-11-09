package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func browserFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"z Folder", "a, 音楽"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"song with  spaces, 雨.FLAC", "voice.ogg", "module.xm", "orchestra.SF2", "readme.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
func browserResult(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("browser did not schedule filesystem work")
	}
	_, _ = m.Update(cmd())
}
func TestFileBrowserListsDirectoriesAndSupportedAudio(t *testing.T) {
	m := wiredSettingsModel(t)
	m.UI.ActiveTab = 0
	dir := browserFixture(t)
	browserResult(t, m, m.openFileBrowser("audio", -1, dir))
	got := stripANSI(renderContent(m, m.layoutPlan()))
	for _, name := range []string{"a, 音楽", "z Folder", "song with  spaces, 雨.FLAC", "voice.ogg", "module.xm"} {
		if !strings.Contains(got, name) {
			t.Errorf("missing %s in %q", name, got)
		}
	}
	if strings.Contains(got, "readme.txt") || strings.Contains(got, "orchestra.SF2") {
		t.Fatal("browser offered unsupported audio files")
	}
	p := m.pages.browser
	if len(p.entries) < 3 || !p.entries[0].directory || !p.entries[1].directory {
		t.Fatal("directories not listed first")
	}
}
func TestFileBrowserCancelIgnoresLateResults(t *testing.T) {
	m := wiredSettingsModel(t)
	m.UI.ActiveTab = 0
	cmd := m.openFileBrowser("audio", -1, browserFixture(t))
	pageKey(m, tea.KeyEscape, "")
	_, _ = m.Update(cmd())
	if m.pages.browser != nil || m.Audio.Player != nil || m.Loading {
		t.Fatal("late read reopened canceled browser or loaded audio")
	}
}
func TestFileBrowserSoundFontUsesRealDraftOnly(t *testing.T) {
	m := wiredSettingsModel(t)
	dir := browserFixture(t)
	field := settingIndex(m, "MIDI SoundFont")
	browserResult(t, m, m.openFileBrowser("soundfont", field, dir))
	p := m.pages.browser
	for i, e := range p.entries {
		if e.name == "orchestra.SF2" {
			p.cursor = i
		}
	}
	browserResult(t, m, pageKey(m, tea.KeyEnter, ""))
	if m.pages.browser != nil || m.Config.SoundfontPath != "" || m.pages.draft.SoundfontPath != filepath.Join(dir, "orchestra.SF2") {
		t.Fatal("font selection did not stay in real draft")
	}
	completeSettingsCmd(t, m, pageKey(m, 's', "s"))
	if m.Config.SoundfontPath != filepath.Join(dir, "orchestra.SF2") {
		t.Fatal("selected font was not saved")
	}
}
func TestFileBrowserFailedLocationKeepsPreviousDirectory(t *testing.T) {
	m := wiredSettingsModel(t)
	dir := browserFixture(t)
	browserResult(t, m, m.openFileBrowser("audio", -1, dir))
	p := m.pages.browser
	browserResult(t, m, m.fileBrowserLocation(filepath.Join(dir, "missing")))
	if p.directory != dir || len(p.entries) == 0 || p.err == "" {
		t.Fatal("failed navigation lost current directory")
	}
}

func TestFileBrowserNavigationRejectsOlderDirectoryResults(t *testing.T) {
	m := wiredSettingsModel(t)
	dir := browserFixture(t)
	first := m.openFileBrowser("audio", -1, dir)
	second := m.fileBrowserLocation(filepath.Join(dir, "z Folder"))
	browserResult(t, m, second)
	browserResult(t, m, first)
	if m.pages.browser.directory != filepath.Join(dir, "z Folder") {
		t.Fatal("old directory result replaced newer navigation")
	}
}
func TestFileBrowserSelectionUsesExistingTrackSwitchPolicy(t *testing.T) {
	m := wiredSettingsModel(t)
	dir := browserFixture(t)
	m.UI.ActiveTab = 0
	player := &mockPlayer{playing: true}
	m.Audio.Player = player
	m.Audio.IsPlaying = true
	browserResult(t, m, m.openFileBrowser("audio", -1, dir))
	for i, e := range m.pages.browser.entries {
		if e.name == "song with  spaces, 雨.FLAC" {
			m.pages.browser.cursor = i
		}
	}
	cmd := pageKey(m, tea.KeyEnter, "")
	_, load := m.Update(cmd())
	if load == nil || !m.Loading || m.pendingLoad == nil || m.Audio.Player != player || player.closeCalled {
		t.Fatal("browser bypassed asynchronous on-success replacement")
	}
	m.pendingLoad.cancel()
}
func TestFileBrowserCancelPreservesCurrentTrackAndFont(t *testing.T) {
	m := wiredSettingsModel(t)
	m.Config.SoundfontPath = "unchanged.sf2"
	player := &mockPlayer{playing: true}
	m.Audio.Player = player
	browserResult(t, m, m.openFileBrowser("soundfont", settingIndex(m, "MIDI SoundFont"), browserFixture(t)))
	pageKey(m, tea.KeyEscape, "")
	if m.pages.dirty || m.Config.SoundfontPath != "unchanged.sf2" || m.Audio.Player != player || player.closeCalled || m.pages.browser != nil {
		t.Fatal("cancel modified current audio or preferences")
	}
}
func TestFileBrowserInputKeepsPathAndRejectsStaleCanceledSelection(t *testing.T) {
	m := wiredSettingsModel(t)
	dir := browserFixture(t)
	browserResult(t, m, m.openFileBrowser("audio", -1, dir))
	_, _ = handleKeyPress(m, tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	if m.pages.editor == nil {
		t.Fatal("location editor missing")
	}
	path := filepath.Join(dir, "song with  spaces, 雨.FLAC")
	m.pages.editor.input.SetValue(path)
	selection := pageKey(m, tea.KeyEnter, "")
	pageKey(m, tea.KeyEscape, "")
	browserResult(t, m, selection)
	if m.Loading || m.pages.editor != nil || m.pages.browser == nil {
		t.Fatal("canceled location result opened audio")
	}
	pageKey(m, tea.KeyEscape, "")
}
func TestOpenAcceptsValidHTTPAudioURLsWithoutStartingNetworkInUpdate(t *testing.T) {
	m := wiredSettingsModel(t)
	_, cmd := runOpen(m, invocation{Rest: "https://example.com/music/song.flac?token=demo"})
	if cmd == nil || !m.Loading || m.Error.Visible {
		t.Fatal("URL did not enter existing background loading path")
	}
	m.pendingLoad.cancel()
	for _, bad := range []string{"http:///track.flac", "https://", "ftp://example.com/audio.flac"} {
		if validOpenLocation(bad) {
			t.Errorf("invalid location accepted: %s", bad)
		}
	}
}

func TestFileBrowserFramesFitAtSupportedAndForcedPanelSizes(t *testing.T) {
	m := wiredSettingsModel(t)
	dir := browserFixture(t)
	browserResult(t, m, m.openFileBrowser("audio", -1, dir))
	for _, size := range [][2]int{{120, 30}, {80, 24}, {68, 17}} {
		m.UI.Width, m.UI.Height = size[0], size[1]
		for _, panel := range []string{"auto", "on", "off"} {
			m.panelMode = panel
			plan := m.layoutPlan()
			frame := renderContent(m, plan)
			if lipgloss.Height(frame) != plan.ContentHeight || lipgloss.Width(frame) != plan.ContentWidth {
				t.Fatalf("browser overflow at %v, %s", size, panel)
			}
		}
	}
}
func TestFileBrowserNamesDoNotExecuteTerminalControlSequences(t *testing.T) {
	m := wiredSettingsModel(t)
	dir := t.TempDir()
	name := "bad\x1b]0;injected\a.flac"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	browserResult(t, m, m.openFileBrowser("audio", -1, dir))
	frame := renderContent(m, m.layoutPlan())
	if strings.Contains(frame, "\x1b]0;injected") || strings.Contains(frame, "\a") {
		t.Fatal("filename injected terminal control sequence")
	}
	if m.pages.browser.entries[0].name != name {
		t.Fatal("display sanitization changed actual path identity")
	}
}

func TestFileBrowserSoundFontDisappearingBeforeSaveKeepsDraft(t *testing.T) {
	m := wiredSettingsModel(t)
	dir := browserFixture(t)
	field := settingIndex(m, "MIDI SoundFont")
	browserResult(t, m, m.openFileBrowser("soundfont", field, dir))
	for i, e := range m.pages.browser.entries {
		if e.name == "orchestra.SF2" {
			m.pages.browser.cursor = i
		}
	}
	browserResult(t, m, pageKey(m, tea.KeyEnter, ""))
	if err := os.Remove(filepath.Join(dir, "orchestra.SF2")); err != nil {
		t.Fatal(err)
	}
	completeSettingsCmd(t, m, pageKey(m, 's', "s"))
	if m.Config.SoundfontPath != "" || !m.pages.dirty || !m.Error.Visible {
		t.Fatal("save accepted a disappeared SoundFont")
	}
}
func TestFileBrowserLocationErrorIsVisibleInEditor(t *testing.T) {
	m := wiredSettingsModel(t)
	dir := browserFixture(t)
	browserResult(t, m, m.openFileBrowser("audio", -1, dir))
	_, _ = handleKeyPress(m, tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	m.pages.editor.input.SetValue(filepath.Join(dir, "missing.flac"))
	browserResult(t, m, pageKey(m, tea.KeyEnter, ""))
	frame := stripANSI(renderContent(m, m.layoutPlan()))
	if !strings.Contains(frame, "no such file") {
		t.Fatal("invalid path left an editor without an error")
	}
}

func TestFileBrowserDismissesAnimationBeforeTakingInput(t *testing.T) {
	m := animModel(t, animPlainFixture, 120, 30)
	if !m.animVisible() {
		t.Fatal("fixture did not start animation")
	}
	_, cmd := runOpen(m, invocation{})
	if m.animVisible() || !m.pageOwnsInput() || cmd == nil {
		t.Fatal("animation hid the opened file browser")
	}
}
func TestFileBrowserReadDoesNotDismissNewLocationEditor(t *testing.T) {
	m := wiredSettingsModel(t)
	cmd := m.openFileBrowser("audio", -1, browserFixture(t))
	_, _ = handleKeyPress(m, tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	m.pages.editor.input.SetValue("new path in progress")
	browserResult(t, m, cmd)
	if m.pages.editor == nil || m.pages.editor.input.Value() != "new path in progress" {
		t.Fatal("directory result stole new location input")
	}
}
func TestFileBrowserNewLocationInputCancelsEarlierFileChoice(t *testing.T) {
	m := wiredSettingsModel(t)
	dir := browserFixture(t)
	browserResult(t, m, m.openFileBrowser("audio", -1, dir))
	cmd := m.fileBrowserLocation(filepath.Join(dir, "voice.ogg"))
	_, _ = handleKeyPress(m, tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	m.pages.editor.input.SetValue("another location")
	browserResult(t, m, cmd)
	if m.Loading || m.pages.editor == nil {
		t.Fatal("earlier choice loaded audio over new location input")
	}
}

func TestFileBrowserSoundFontResultAppliesWithoutRereadingFile(t *testing.T) {
	m := wiredSettingsModel(t)
	dir := browserFixture(t)
	path := filepath.Join(dir, "orchestra.SF2")
	browserResult(t, m, m.openFileBrowser("soundfont", settingIndex(m, "MIDI SoundFont"), dir))
	result := m.fileBrowserLocation(path)()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_, _ = m.Update(result)
	if m.pages.browser != nil || m.pages.draft.SoundfontPath != path {
		t.Fatal("Update re-read a path already validated by worker")
	}
	completeSettingsCmd(t, m, pageKey(m, 's', "s"))
	if !m.Error.Visible || !m.pages.dirty {
		t.Fatal("save did not independently revalidate selected path")
	}
}
