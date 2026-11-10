package ui

import (
	"fmt"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

const volumeSaveDelay = 250 * time.Millisecond

type volumeSaveJob struct {
	revision uint64
	run      func() volumeSavedMsg
}
type volumeSavedMsg struct {
	job *volumeSaveJob
	err error
}

// Input updates memory immediately; the existing tick coalesces writes after
// the user stops adjusting, without spawning a timer or writer for every key.
func (m *Model) saveVolumeConfig() {
	if m.Config.DefaultVolume == m.Audio.Volume {
		return
	}
	m.Config.DefaultVolume = m.Audio.Volume
	m.volumeSavePending = true
	m.volumeSaveDue = time.Now().Add(volumeSaveDelay)
}

func (m *Model) startVolumeSave(now time.Time) tea.Cmd {
	if m.exiting || !m.volumeSavePending || m.volumeSave != nil || m.configSave != nil || m.volumeSaveDue.IsZero() || now.Before(m.volumeSaveDue) {
		return nil
	}
	snapshot := m.Config.Clone()
	job := &volumeSaveJob{revision: m.volumeRevision}
	job.run = sync.OnceValue(func() volumeSavedMsg {
		return volumeSavedMsg{job: job, err: snapshot.Save()}
	})
	m.volumeSave = job
	m.volumeSavePending = false
	return func() tea.Msg { return job.run() }
}

func (m *Model) finishVolumeSave(msg volumeSavedMsg) {
	if m.volumeSave != msg.job {
		return
	}
	m.volumeSave = nil
	if msg.err != nil {
		m.volumeSavePending = true
		// Keep the value for the next change, explicit save or shutdown retry.
		// Do not hammer an unavailable disk on every UI tick.
		if m.volumeRevision == msg.job.revision {
			m.volumeSaveDue = time.Time{}
		}
		m.Error.Set(fmt.Sprintf("Volume save failed: %v", msg.err), 90)
	}
}

// Shutdown joins the same immutable writer the command runs. Explicit settings
// saves join it inside their worker; neither path depends on host scheduling.
func (m *Model) flushVolumeSave() {
	if job := m.volumeSave; job != nil {
		m.finishVolumeSave(job.run())
	}
}

func (m *Model) flushDeferredVolume() {
	if !m.volumeSavePending {
		return
	}
	m.Config.DefaultVolume = m.Audio.Volume
	if err := m.Config.Save(); err != nil {
		m.volumeSaveDue = time.Time{}
		m.Error.Set(fmt.Sprintf("Volume save failed: %v", err), 90)
		return
	}
	m.volumeSavePending = false
}
