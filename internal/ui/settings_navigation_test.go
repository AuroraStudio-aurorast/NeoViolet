package ui

import (
	"fmt"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

func TestGUISettingsNavigationReturnsAuthenticatedRequestResults(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	m.UI.ActiveTab = 0
	if _, err := fmt.Fprintln(conn, `{"type":"open_settings","request_id":"settings-1"}`); err != nil {
		t.Fatal(err)
	}
	m.Update(<-m.ipcServer.Incoming)
	if readIPC(t, decoder, "result", "settings-1")["status"] != "succeeded" || m.UI.ActiveTab != 3 {
		t.Fatal("settings navigation did not acknowledge the displayed page")
	}
	m.pages.openEditor("Filter", "filter", "typed text", 0)
	if _, err := fmt.Fprintln(conn, `{"type":"open_settings","request_id":"settings-2"}`); err != nil {
		t.Fatal(err)
	}
	m.Update(<-m.ipcServer.Incoming)
	if readIPC(t, decoder, "result", "settings-2")["status"] != "failed" || m.pages.editor.input.Value() != "typed text" {
		t.Fatal("blocked navigation was not rejected without losing input")
	}
}

func TestGUISettingsNavigationPreservesDraftAndPlayback(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	m.UI.ActiveTab = 0
	m.UI.Focus = FocusFooter
	player := &mockPlayer{playing: true}
	m.Audio.Player, m.Audio.IsPlaying = player, true
	if _, err := m.ipcRequest(ipc.Message{Type: "open_settings"}); err != nil {
		t.Fatal(err)
	}
	if m.UI.ActiveTab != 3 || m.UI.Focus != FocusContent || !m.pages.dirty || m.pages.draft.SeekStep != 21 || m.Config.SeekStep != 13 {
		t.Fatal("settings navigation lost its draft or failed to focus the page")
	}
	if m.Audio.Player != player || !m.Audio.IsPlaying || player.pauseCalled || player.closeCalled {
		t.Fatal("opening settings changed playback")
	}
}

func TestGUISettingsNavigationDoesNotDiscardActiveInteraction(t *testing.T) {
	for _, interaction := range []string{"editor", "picker", "browser", "folders", "quit", "delete", "save", "command", "native-picker", "playlist-save"} {
		t.Run(interaction, func(t *testing.T) {
			m := wiredSettingsModel(t)
			m.UI.ActiveTab = 0
			switch interaction {
			case "editor":
				m.pages.openEditor("Filter", "filter", "typed", 0)
			case "picker":
				m.pages.picker = "formats"
			case "browser":
				m.pages.browser = &fileBrowser{purpose: "audio"}
			case "folders":
				m.pages.rootEditor = true
			case "quit":
				m.pages.quitSettings = true
			case "delete":
				m.pages.deletePlaylist = "list"
			case "save":
				m.configSave = &configSaveJob{}
			case "command":
				m.UI.Mode = ModeCommand
			case "native-picker":
				m.ipcPickerID = "audio-1"
			case "playlist-save":
				m.playlists.save = &playlistSaveJob{}
			}
			if _, err := m.ipcRequest(ipc.Message{Type: "open_settings"}); err == nil {
				t.Fatal("settings navigation silently replaced an active interaction")
			}
			if m.UI.ActiveTab != 0 {
				t.Fatal("rejected request changed pages")
			}
		})
	}
}
