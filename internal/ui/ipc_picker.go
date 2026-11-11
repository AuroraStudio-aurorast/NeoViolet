package ui

import (
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

type nativePicker struct {
	id, purpose string
	session     uint64
	field       int
	validating  bool
}
type nativeSelectionMsg struct {
	picker *nativePicker
	path   string
	err    error
}

func settingFilePurpose(key string) string {
	if key == "gui.neoviolet_path" {
		return "player_binary"
	}
	return "soundfont"
}

// The terminal browser stays available when no capable GUI is attached.
func (m *Model) openFilePicker(purpose string, field int, start string) tea.Cmd {
	wire := purpose
	if purpose == "directory" {
		wire = "library_directory"
	}
	native := m.ipcSettingsPicker
	if wire == "audio" {
		native = m.ipcAudioPicker
	}
	if !native || m.ipcServer == nil || m.ipcSession == 0 {
		return m.openFileBrowser(purpose, field, start)
	}
	if m.ipcPickerID != "" {
		return nil
	}
	if wire != "audio" && m.configSave != nil {
		m.Info.Set("Saving configuration…", 90)
		return nil
	}
	m.ipcPickerSequence++
	id := fmt.Sprintf("%s-%d", wire, m.ipcPickerSequence)
	if err := m.ipcServer.SendTo(m.ipcSession, ipc.Message{Type: "pick_file", Purpose: wire, RequestID: id}); err != nil {
		return m.openFileBrowser(purpose, field, start)
	}
	if m.Anim != nil {
		m.Anim.Close()
	}
	m.ipcPicker = &nativePicker{id: id, purpose: wire, field: field, session: m.ipcSession}
	m.ipcPickerID = id
	m.pages.browser = nil
	return nil
}

func (m *Model) filePicked(message ipc.Message) tea.Cmd {
	p := m.ipcPicker
	// PTY Escape or Quit can invalidate Go's request before the GUI has
	// received pick_file. Its later completion still needs a terminal receipt
	// so the physical chooser can return input ownership to the terminal.
	if p == nil || p.id != message.RequestID {
		m.ipcResult(m.ipcSession, message.RequestID, "cancelled", "")
		return nil
	}
	if (p.validating && message.Status != "cancelled") || p.session != m.ipcSession || p.id != message.RequestID || p.purpose != message.Purpose || m.ipcPickerID != p.id {
		return nil
	}
	switch message.Status {
	case "cancelled":
		m.clearNativePicker()
		m.ipcResult(p.session, p.id, "cancelled", "")
	case "failed":
		reason := message.Error
		if reason == "" {
			reason = "Native file chooser unavailable"
		}
		m.ipcResult(p.session, p.id, "failed", reason)
		m.Error.Set(reason, m.Config.Error.Duration)
		m.clearNativePicker()
		purpose := p.purpose
		if purpose == "library_directory" {
			purpose = "directory"
		}
		m.Info.Set("Native chooser unavailable; using terminal browser", m.Config.Error.Duration)
		return m.openFileBrowser(purpose, p.field, "")
	case "selected":
		if p.purpose == "audio" {
			m.clearNativePicker()
			if isValidAudioPath(message.Path) {
				_, cmd := handleLoadTrack(m, LoadTrackMsg{Path: message.Path})
				m.ipcResult(p.session, p.id, "succeeded", "")
				return cmd
			}
			m.ipcResult(p.session, p.id, "failed", "Invalid or unsupported audio file: "+message.Path)
			m.Error.Set("Invalid or unsupported audio file: "+message.Path, m.Config.Error.Duration)
			return nil
		}
		p.validating = true
		m.Info.Set("Checking selected path…", m.Config.Error.Duration)
		return func() tea.Msg {
			path, err := validateNativePath(p.purpose, message.Path)
			return nativeSelectionMsg{picker: p, path: path, err: err}
		}
	default:
		m.clearNativePicker()
		m.ipcResult(p.session, p.id, "failed", "Invalid native file picker result")
		m.Error.Set("Invalid native file picker result", m.Config.Error.Duration)
	}
	return nil
}

func (m *Model) clearNativePicker() {
	if m.ipcPicker != nil && m.ipcPicker.validating {
		m.Info.Visible = false
	}
	m.ipcPicker = nil
	m.ipcPickerID = ""
}

func (m *Model) applyNativeSelection(msg nativeSelectionMsg) tea.Cmd {
	p := msg.picker
	if p == nil || m.exiting || p != m.ipcPicker || p.session != m.ipcSession || p.id != m.ipcPickerID {
		return nil
	}
	m.clearNativePicker()
	if msg.err != nil {
		m.ipcResult(p.session, p.id, "failed", browserErrorText(msg.err))
		m.Error.Set(browserErrorText(msg.err), m.Config.Error.Duration)
		return nil
	}
	if p.field < 0 || p.field >= len(m.pages.settings) {
		return nil
	}
	var err error
	if p.purpose == "library_directory" && m.pages.settings[p.field].key == "library.roots" {
		err = m.pages.addLibraryRoot(p.field, msg.path)
	} else if m.pages.settings[p.field].kind == "file" && settingFilePurpose(m.pages.settings[p.field].key) == p.purpose {
		err = m.pages.applyDraftValue(p.field, msg.path)
	}
	if err != nil {
		m.Error.Set(err.Error(), m.Config.Error.Duration)
		m.ipcResult(p.session, p.id, "failed", err.Error())
	} else {
		m.Info.Set("Selection added to draft; Save to apply", m.Config.Error.Duration)
		m.ipcResult(p.session, p.id, "succeeded", "")
	}
	return nil
}

func validateNativePath(purpose, path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("choose an absolute local path")
	}
	path = filepath.Clean(path)
	switch purpose {
	case "soundfont":
		_, err := parseSetting(pageSetting{key: "soundfont_path"}, path)
		return path, err
	case "library_directory":
		return path, validateLibraryRoots([]string{path})
	case "player_binary":
		return path, validatePlayerBinary(path)
	default:
		return "", fmt.Errorf("unsupported file chooser purpose")
	}
}
