package ui

import (
	"fmt"
	"math"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

type ipcOpenRequest struct {
	id      string
	session uint64
}

func (m *Model) waitIPC() tea.Cmd {
	if m.ipcServer == nil || m.ipcManaged {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-m.ipcServer.Incoming
		if !ok {
			return nil
		}
		return event
	}
}

func handleIPCEvent(m *Model, event ipc.Event) (tea.Model, tea.Cmd) {
	next := m.waitIPC()
	switch event.Kind {
	case "connected":
		if m.ipcServer != nil && event.Session == m.ipcServer.Session() {
			m.ipcSession = event.Session
			m.ipcAudioPicker = false
			m.ipcPickerID = ""
			m.ipcRevision = 0
			m.pushIPCState(true)
			m.Audio.LastSentLyricSig = ""
		}
	case "disconnected":
		if event.Session == m.ipcSession {
			m.ipcSession = 0
			m.ipcAudioPicker = false
			m.ipcPickerID = ""
		}
		if m.ipcPendingOpen != nil && m.ipcPendingOpen.session == event.Session {
			m.ipcPendingOpen = nil
		}
	case "request":
		if m.ipcServer == nil || event.Session != m.ipcSession || event.Session != m.ipcServer.Session() {
			return m, next
		}
		cmd, err := m.ipcRequest(event.Message)
		if err != nil {
			m.ipcResult(event.Session, event.Message.RequestID, "failed", err.Error())
		} else if event.Message.Type != "open" && event.Message.Type != "file_picked" {
			m.ipcResult(event.Session, event.Message.RequestID, "succeeded", "")
		}
		if event.Message.Type == "quit" {
			m.cleanup()
			return m, tea.Quit
		}
		return m, tea.Batch(next, cmd)
	}
	return m, next
}

func (m *Model) ipcRequest(message ipc.Message) (tea.Cmd, error) {
	switch message.Type {
	case "client_hello":
		if message.Client != "neoviolet-gui" || message.Protocol != 1 {
			return nil, fmt.Errorf("unsupported GUI client")
		}
		m.ipcAudioPicker = slices.Contains(message.Capabilities, "audio_file_picker")
	case "file_picked":
		return m.audioPicked(message), nil
	case "request_quit":
		return nil, m.requestGUIQuit()
	case "quit":
		return nil, nil
	case "get_state":
		m.pushIPCState(true)
	case "open":
		if !isValidAudioSource(message.Path) {
			return nil, fmt.Errorf("invalid or unsupported audio source: %s", message.Path)
		}
		_, cmd := handleLoadTrack(m, LoadTrackMsg{Path: message.Path})
		m.ipcPendingOpen = &ipcOpenRequest{id: message.RequestID, session: m.ipcSession}
		m.ipcResult(m.ipcSession, message.RequestID, "accepted", "")
		return cmd, nil
	case "desktop_lyrics":
		if message.Enable == nil {
			return nil, fmt.Errorf("enable is required")
		}
		m.setDesktopLyricsEnabled(*message.Enable)
	case "set_volume":
		if message.Volume == nil || math.IsNaN(*message.Volume) || math.IsInf(*message.Volume, 0) || *message.Volume < 0 || *message.Volume > 1 {
			return nil, fmt.Errorf("volume must be between 0 and 1")
		}
		m.setVolume(*message.Volume)
	case "seek":
		if message.Position == nil || math.IsNaN(*message.Position) || math.IsInf(*message.Position, 0) || *message.Position < 0 || *message.Position >= float64(math.MaxInt64)/float64(time.Second) {
			return nil, fmt.Errorf("position must be a nonnegative number of seconds")
		}
		if m.Audio.Player == nil {
			return nil, fmt.Errorf("no audio loaded")
		}
		pos := time.Duration(*message.Position * float64(time.Second))
		if m.Audio.Duration > 0 && pos > m.Audio.Duration {
			pos = m.Audio.Duration
		}
		if err := m.Audio.SeekPlayer(pos); err != nil {
			return nil, fmt.Errorf("seek failed: %w", err)
		}
		m.Audio.UpdatePosition()
	case "play", "pause", "play_pause":
		if m.Audio.Player == nil {
			return nil, fmt.Errorf("no audio loaded")
		}
		playing := m.Audio.Player.IsPlaying()
		play := message.Type == "play" || (message.Type == "play_pause" && !playing)
		if err := m.Audio.SetPlaying(play); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported command: %s", message.Type)
	}
	return nil, nil
}

func (m *Model) ipcResult(session uint64, id, status, message string) {
	if id == "" || m.ipcServer == nil {
		return
	}
	if err := m.ipcServer.SendTo(session, ipc.Message{Type: "result", RequestID: id, Status: status, Error: message}); err != nil {
		logger.Debug("IPC result unavailable", "err", err)
	}
}

func (m *Model) finishIPCOpen(status, message string) {
	if request := m.ipcPendingOpen; request != nil {
		m.ipcResult(request.session, request.id, status, message)
		m.ipcPendingOpen = nil
	}
}

// requestGUIQuit decides from live Go state; a paused track still needs confirmation.
func (m *Model) requestGUIQuit() error {
	if m.confirmSettingsQuit() {
		return nil
	}
	if m.ipcSession == 0 {
		return fmt.Errorf("GUI session is unavailable")
	}
	confirm := m.Audio.Player != nil || m.Loading
	return m.ipcServer.SendTo(m.ipcSession, ipc.Message{Type: "quit", Dialog: &confirm})
}
