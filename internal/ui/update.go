package ui

import (
	"encoding/json"
	"fmt"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/anim"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

func updateDispatcher(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TickMsg:
		return handleTick(m)
	case tea.WindowSizeMsg:
		return handleResize(m, msg)
	case tea.KeyPressMsg:
		return handleKeyPress(m, msg)
	case tea.PasteMsg:
		return handlePaste(m, msg.Content)
	case progress.FrameMsg:
		return handleProgressFrame(m, msg)
	case VolumeMsg:
		return handleVolume(m, msg)
	case SeekMsg:
		return handleSeek(m, msg)
	case ErrorMsg:
		return handleError(m, msg)
	case AudioLoadFailedMsg:
		return handleAudioLoadFailed(m, msg)
	case AudioLoadedMsg:
		return handleAudioLoaded(m, msg)
	case AccentApplyMsg:
		return handleAccentApply(m, msg)
	case MediaCtlMsg:
		return handleMediaCtlCmd(m, msg)
	case MediaCtlReadyMsg:
		return handleMediaCtlReady(m, msg)
	case LoadTrackMsg:
		return handleLoadTrack(m, msg)
	case FetchLyricsResultMsg:
		return handleFetchLyricsResult(m, msg)
	case anim.LoadedMsg:
		return handleAnimLoaded(m, msg)
	default:
		// The animation wakes itself with a tick whose type is unexported, so the
		// host cannot name it and cannot tell it apart from anything else it does
		// not recognise. Everything the host does not claim is offered to the
		// animation, which answers for what was its own.
		if m.Anim != nil {
			if cmd, handled := m.Anim.Update(msg); handled {
				return m, cmd
			}
		}
		return m, nil
	}
}

// lyricPushInterval caps how often progress reaches the desktop lyrics overlay.
// A changed line is still pushed the moment it changes; this only bounds the
// word-highlight refresh, so 33ms is 30 Hz at the default tick rate.
const lyricPushInterval = 33 * time.Millisecond

func handleTick(m *Model) (tea.Model, tea.Cmd) {
	cmd := m.updatePlaybackState()
	cmd = tea.Batch(cmd, m.syncAnim())
	m.Error.Tick()
	m.Info.Tick()

	if m.Loading || (m.Anim != nil && m.Anim.Loading) {
		m.loadingTick++
	}

	// Drain IPC messages from GUI wrapper (non-blocking, one per tick)
	if m.ipcServer != nil {
		select {
		case raw := <-m.ipcServer.Incoming:
			var ipcMsg ipc.Message
			if err := json.Unmarshal([]byte(raw), &ipcMsg); err != nil {
				logger.Debug("IPC: invalid JSON", "line", raw, "err", err)
			} else {
				switch ipcMsg.Type {
				case "open":
					if ipcMsg.Path != "" {
						if !isValidAudioPath(ipcMsg.Path) {
							logger.Warn("IPC: invalid audio path from GUI", "path", ipcMsg.Path)
							m.Error.Set("Invalid or unsupported audio file: "+ipcMsg.Path, 180)
							break
						}
						logger.Info("IPC: loading track", "path", ipcMsg.Path)
						_, loadCmd := handleLoadTrack(m, LoadTrackMsg{Path: ipcMsg.Path})
						return m, tea.Batch(loadCmd,
							tea.Tick(time.Second/time.Duration(m.Config.TickRate),
								func(_ time.Time) tea.Msg { return TickMsg{} },
							),
						)
					}
				case "desktop_lyrics":
					if ipcMsg.Enable != nil {
						m.DesktopLyricsEnabled = *ipcMsg.Enable
						logger.Info("IPC: desktop lyrics", "enabled", *ipcMsg.Enable)
					}
				case "play_pause":
					logger.Debug("IPC: play_pause from GUI")
					m.togglePlayback()
				default:
					logger.Debug("IPC: unhandled message type", "type", ipcMsg.Type)
				}
			}
		default:
		}
	}

	// Push current playback state to OS media control layer (MPRIS on Linux)
	if m.MediaCtl != nil {
		m.MediaCtl.Update(m.buildPlayState())
	}

	// Stream lyrics to GUI for desktop lyrics overlay. A changed signature goes
	// out at once; progress goes out on its own cadence, because the overlay
	// highlights each word from elapsed without the line changing. Sends even
	// when lyrics are nil so the GUI can clear stale display.
	if m.DesktopLyricsEnabled && m.ipcServer != nil {
		lines := buildLyricLinesJSON(m.Audio.Lyrics, m.Audio.Elapsed)
		sig := lyricSig(lines, m.Audio.LyricNextIndex)
		now := time.Now()
		changed := sig != m.Audio.LastSentLyricSig
		// Only a frame that actually moved is worth sending, so a paused player
		// stays quiet instead of repeating an identical payload.
		moved := m.Audio.Elapsed != m.Audio.LastSentLyricElapsed
		due := now.Sub(m.Audio.LastLyricPush) >= lyricPushInterval
		if changed || (moved && due) {
			m.Audio.LastSentLyricSig = sig
			m.Audio.LastSentLyricElapsed = m.Audio.Elapsed
			m.Audio.LastLyricPush = now
			lyricMsg := ipc.Message{
				Type:    "lyrics",
				Lines:   lines,
				Elapsed: m.Audio.Elapsed.Seconds(),
				Title:   m.Audio.CurrentSong,
				Artist:  m.Audio.Artist,
			}
			_ = m.ipcServer.SendJSON(lyricMsg)
		}
	}

	return m, tea.Batch(cmd, tea.Tick(time.Second/time.Duration(m.Config.TickRate), func(_ time.Time) tea.Msg {
		return TickMsg{}
	}))
}

func handleResize(m *Model, msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.UI.Width = msg.Width
	m.UI.Height = msg.Height

	m.UI.tabWidth = (msg.Width - 4) / len(m.UI.Tabs)
	if m.UI.tabWidth > 20 {
		m.UI.tabWidth = 20
	}

	syncCommandInputWidth(m)

	return m, nil
}

func handleProgressFrame(m *Model, msg progress.FrameMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.Components.ProgressBar, cmd = m.Components.ProgressBar.Update(msg)
	return m, cmd
}

func handleVolume(m *Model, msg VolumeMsg) (tea.Model, tea.Cmd) {
	if msg.Delta != 0 {
		m.Audio.AdjustVolume(msg.Delta)
	} else if msg.Level >= 0 && msg.Level <= 1.0 {
		m.Audio.SetVolume(msg.Level)
	}
	logger.Debug("Volume", "newVolume", m.Audio.Volume)
	m.Components.VolumeBar.SetPercent(m.Audio.Volume)
	return m, nil
}

func handleSeek(m *Model, msg SeekMsg) (tea.Model, tea.Cmd) {
	if m.Audio.Player == nil {
		return m, nil
	}

	currentPos := m.Audio.Player.Position()
	var newPos time.Duration
	if msg.Relative {
		newPos = currentPos + msg.Position
	} else {
		newPos = msg.Position
	}

	if newPos < 0 {
		newPos = 0
	}
	if newPos > m.Audio.Duration {
		newPos = m.Audio.Duration
	}

	logger.Debug("Seek", "from", currentPos, "to", newPos)
	if err := m.Audio.SeekPlayer(newPos); err != nil {
		m.Error.Set(fmt.Sprintf("Seek failed: %v", err), 120)
	}

	return m, nil
}

func handleError(m *Model, msg ErrorMsg) (tea.Model, tea.Cmd) {
	m.Error.Set(msg.Message, msg.Timer)
	return m, nil
}

func handleAccentApply(m *Model, msg AccentApplyMsg) (tea.Model, tea.Cmd) {
	if m.exiting || msg.Generation != m.activeGeneration {
		return m, nil
	}
	m.Accent = msg.Accent
	m.rebuildProgressBar()
	return m, nil
}
