package ui

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

type configSaveJob struct {
	receipt        *ipcOpenRequest
	preserveDraft  bool
	saveDraft      bool
	snapshot       config.Config
	changed        map[string]bool
	volumeRevision uint64
	run            func() configSavedMsg
	quit           bool
	pendingVolume  bool
}
type configSavedMsg struct {
	job *configSaveJob
	err error
}

func (p *pageState) initSettings(cfg config.Config) {
	p.draft = cfg.Clone()
	p.baseline = cfg.Clone()
	p.settings = settingsFromConfig(cfg)
	p.changed = map[string]bool{}
	p.dirty = false
	p.baseFormats = slices.Clone(cfg.Lyrics.FormatPriority)
	p.formats = slices.Clone(cfg.Lyrics.FormatPriority)
	for _, name := range append([]string{"embedded"}, append(lyrics.AvailableParsers(), "online")...) {
		if !slices.Contains(p.formats, name) {
			p.formats = append(p.formats, name)
		}
	}
	p.enabled = make([]bool, len(p.formats))
	for i, name := range p.formats {
		p.enabled[i] = slices.Contains(cfg.Lyrics.FormatPriority, name)
	}
}

func (m *Model) saveSettings(quit bool) tea.Cmd {
	if m.configSave != nil {
		if m.configSave.preserveDraft {
			m.configSave.saveDraft = true
		}
		if quit {
			m.configSave.quit = true
		}
		m.Info.Set("Saving configuration…", 90)
		return nil
	}
	candidate := m.Config.Clone()
	changed := map[string]bool{}
	if m.pages != nil && m.pages.dirty {
		values := configValues(candidate)
		draft := configValues(m.pages.draft)
		for key := range m.pages.changed {
			setSettingValue(values, key, settingValue(draft, key))
			changed[key] = true
		}
		data, err := json.Marshal(values)
		if err != nil {
			m.Error.Set(err.Error(), 90)
			return nil
		}
		if err = json.Unmarshal(data, &candidate); err != nil {
			m.Error.Set(err.Error(), 90)
			return nil
		}
	}
	if !changed["default_volume"] {
		candidate.DefaultVolume = m.Audio.Volume
	}
	return m.startConfigSave(candidate, changed, quit)
}

func (m *Model) startConfigSave(candidate config.Config, changed map[string]bool, quit bool) tea.Cmd {
	previousVolume := m.volumeSave
	job := &configSaveJob{snapshot: candidate, changed: changed, volumeRevision: m.volumeRevision, quit: quit, pendingVolume: m.volumeSavePending || previousVolume != nil}
	m.volumeSave = nil
	m.volumeSavePending = false // This snapshot includes the latest remembered volume.
	m.configSave = job
	m.Info.Set("Saving configuration…", max(90, m.Config.Error.Duration))
	job.run = sync.OnceValue(func() configSavedMsg {
		// Serialize snapshots in the worker, never by waiting in Update.
		if previousVolume != nil {
			_ = previousVolume.run()
		}
		var err error
		if job.changed["soundfont_path"] {
			_, err = parseSetting(pageSetting{key: "soundfont_path"}, job.snapshot.SoundfontPath)
		}
		if err == nil && job.changed["library.roots"] {
			err = validateLibraryRoots(job.snapshot.Library.Roots)
		}
		if err == nil {
			err = job.snapshot.Save()
		}
		return configSavedMsg{job: job, err: err}
	})
	return func() tea.Msg { return job.run() }
}

func (m *Model) finishSettingsSave(msg configSavedMsg) tea.Cmd {
	if m.configSave != msg.job {
		return nil
	}
	m.configSave = nil
	if msg.err != nil {
		m.Info.Visible = false
		if r := msg.job.receipt; r != nil {
			m.ipcResult(r.session, r.id, "failed", msg.err.Error())
		}
		m.Error.Set(fmt.Sprintf("Save failed: %v", msg.err), max(90, m.Config.Error.Duration))
		m.volumeSavePending = m.volumeSavePending || msg.job.pendingVolume
		return nil
	}
	cfg := msg.job.snapshot.Clone()
	if m.volumeRevision != msg.job.volumeRevision {
		cfg.DefaultVolume = m.Audio.Volume
	} else if msg.job.changed["default_volume"] {
		m.Audio.SetVolume(cfg.DefaultVolume)
	}
	m.applySavedSettings(cfg, msg.job.changed)
	if msg.job.preserveDraft {
		draft, changed := configValues(m.pages.draft), m.pages.changed
		formats, enabled := m.pages.formats, m.pages.enabled
		m.pages.initSettings(cfg)
		for i, f := range m.pages.settings {
			if changed[f.key] {
				_ = m.pages.applyDraftValue(i, settingValue(draft, f.key))
			}
		}
		if changed["lyrics.format_priority"] {
			m.pages.formats, m.pages.enabled = formats, enabled
		}
	} else {
		m.pages.initSettings(cfg)
	}
	m.pushGUIPreferences()
	if r := msg.job.receipt; r != nil {
		m.ipcResult(r.session, r.id, "succeeded", "")
	}
	m.Info.SetSuccess("Configuration saved", max(90, m.Config.Error.Duration))
	if msg.job.saveDraft {
		return m.saveSettings(msg.job.quit)
	}
	if msg.job.quit {
		if m.playlists.save != nil {
			m.playlists.save.quit = true
			m.playlists.save.settingsQuit = true
			return nil
		}
		return m.quitAfterSavedSettings()
	}
	cmds := []tea.Cmd{}
	if msg.job.changed["library.roots"] {
		cmds = append(cmds, m.rescanLibrary())
	}
	if msg.job.changed["accent.auto_accent"] && cfg.Accent.IsEnabled() && m.Audio.Player != nil {
		cmds = append(cmds, loadAccentCmd(m.Audio.Player, m.activeGeneration))
	}
	if msg.job.changed["lyrics.enabled"] && cfg.Lyrics.Enabled && m.Audio.Player != nil && m.Audio.Lyrics == nil {
		cmds = append(cmds, m.loadSettingsLyrics())
	}
	return tea.Batch(cmds...)
}

// Shutdown joins the sole settings writer before the CLI's final volume save.
// The worker owns only an immutable snapshot; all model changes stay here.
func (m *Model) flushSettingsSave() {
	for m.configSave != nil {
		job := m.configSave
		msg := job.run()
		job.quit = false
		_ = m.finishSettingsSave(msg)
	}
}
func (m *Model) applySavedSettings(cfg config.Config, changed map[string]bool) {
	*m.Config = cfg
	if changed["desktop_lyrics.enabled"] {
		m.setDesktopLyricsEnabled(cfg.DesktopLyrics.Enabled)
		if m.ipcServer != nil && !m.ipcPreferences {
			enabled := cfg.DesktopLyrics.Enabled
			_ = m.ipcServer.SendJSON(ipc.Message{Type: "desktop_lyrics", Enable: &enabled})
		}
	}
	switch cfg.IconTheme {
	case "emoji":
		m.Icons = EmojiIcons
	case "fallback":
		m.Icons = FallbackIcons
	default:
		m.Icons = NerdIcons
	}
	if changed["lyrics.enabled"] {
		m.invalidateLyricRequests()
		m.LyricsFetching = false
		m.Audio.ShowLyrics = cfg.Lyrics.Enabled
	}
	if changed["lyrics.panel.mode"] {
		m.panelMode = cfg.Lyrics.Panel.Mode
	}
	if changed["accent.auto_accent"] {
		m.Accent = nil
	}
	m.rebuildProgressBar()
	volume := progress.New(progress.WithFillCharacters([]rune(cfg.VolumeBar.Fill[0])[0], []rune(cfg.VolumeBar.Fill[1])[0]), progress.WithScaled(false))
	volume.ShowPercentage = cfg.VolumeBar.ShowPercentage
	volume.SetWidth(cfg.VolumeBar.Width)
	volume.SetPercent(m.Audio.Volume)
	m.Components.VolumeBar = volume
	if n := cfg.CommandHistory.Max; n > 0 && len(m.CommandHistory) > n {
		m.CommandHistory = m.CommandHistory[len(m.CommandHistory)-n:]
	}
	m.historyIndex = len(m.CommandHistory)
}

func (m *Model) confirmSettingsQuit() bool {
	if m.playlists.save != nil {
		m.playlists.save.quit = true
		return true
	}
	if m.pages == nil || !m.pages.dirty {
		return false
	}
	if m.configSave != nil {
		m.configSave.quit = true
		return true
	}
	// Preserve navigation on cancellation, but reject selections still in flight
	// before the user makes a save/discard decision.
	if browser := m.pages.browser; browser != nil {
		browser.generation++
		browser.loading = false
	}
	m.pages.quitSettings = true
	m.UI.Mode = ModeNormal
	m.UI.Focus = FocusContent
	if m.Anim != nil {
		m.Anim.Close()
	}
	return true
}
func (m *Model) updateSettingsQuit(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "ctrl+g":
		m.pages.quitSettings = false
	case "s":
		if m.configSave != nil {
			return nil
		}
		m.pages.quitSettings = false
		return m.saveSettings(true)
	case "d":
		m.pages.quitSettings = false
		m.pages.initSettings(*m.Config)
		if m.ipcServer != nil {
			f := false
			_ = m.ipcServer.SendJSON(ipc.Message{Type: "quit", Dialog: &f})
		}
		m.cleanup()
		return tea.Quit
	}
	return nil
}

// A successful :wq keeps its direct GUI exit semantics after other writers finish.
func (m *Model) quitAfterSavedSettings() tea.Cmd {
	if m.confirmSettingsQuit() {
		return nil
	}
	if m.ipcServer != nil {
		f := false
		_ = m.ipcServer.SendJSON(ipc.Message{Type: "quit", Dialog: &f})
	}
	m.cleanup()
	return tea.Quit
}
