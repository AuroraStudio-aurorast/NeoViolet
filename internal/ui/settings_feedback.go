package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

func (m *Model) settingsSavedNotice(changed map[string]bool, revision uint64) string {
	guiChanged := false
	for key := range changed {
		if strings.HasPrefix(key, "gui.") || strings.HasPrefix(key, "desktop_lyrics.") {
			guiChanged = true
			break
		}
	}
	if !guiChanged {
		return "Configuration saved"
	}
	if m.ipcSession == 0 || !m.ipcPreferences {
		return "Configuration saved; changes apply on next GUI launch"
	}
	if revision == 0 {
		return "Configuration saved; GUI update unavailable"
	}
	if m.ipcPreferencesFeedback {
		m.ipcPreferencesPending = revision
		return "Configuration saved; updating GUI…"
	}
	return "Configuration saved"
}

func (m *Model) preferencesApplied(message ipc.Message) error {
	if !m.ipcPreferencesFeedback || message.Revision == 0 || message.Revision != m.ipcPreferencesPending || message.Revision != m.ipcPreferencesRevision {
		return nil
	}
	notice := ""
	switch message.Status {
	case "applied":
		notice = "Configuration saved; GUI updated"
	case "restart_required":
		if len(message.RestartRequired) == 0 {
			return fmt.Errorf("restart-required preferences are missing")
		}
		allowed := []string{"gui.system_decorations", "gui.neoviolet_path"}
		for _, key := range message.RestartRequired {
			if !slices.Contains(allowed, key) {
				return fmt.Errorf("unknown restart-required preference")
			}
		}
		notice = "Configuration saved; GUI updated; reopen GUI to finish applying"
	case "failed":
		reason := message.Error
		if reason == "" {
			reason = "GUI could not apply the saved preferences"
		}
		notice = "Configuration saved; GUI update failed"
		m.Error.Set(reason, max(90, m.Config.Error.Duration))
	default:
		return fmt.Errorf("unknown GUI preference application status")
	}
	if m.configSave == nil {
		m.Info.SetSuccess(notice, max(90, m.Config.Error.Duration))
	}
	m.ipcPreferencesPending = 0
	return nil
}
