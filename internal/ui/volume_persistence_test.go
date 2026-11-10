package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func TestVolumeSaveDebouncesAndKeepsChangesDuringWrite(t *testing.T) {
	m := wiredSettingsModel(t)
	m.setVolume(.7)
	due := m.volumeSaveDue
	if m.startVolumeSave(due.Add(-time.Nanosecond)) != nil {
		t.Fatal("volume was saved before the quiet period")
	}
	first := m.startVolumeSave(due)
	if first == nil || config.Exists() {
		t.Fatal("scheduling did not defer disk I/O to the command")
	}
	m.setVolume(.8)
	if m.startVolumeSave(m.volumeSaveDue) != nil {
		t.Fatal("started another writer while the first was pending")
	}
	_, _ = m.Update(first())
	disk, err := config.Load()
	if err != nil || disk.DefaultVolume != .7 || m.Audio.Volume != .8 || !m.volumeSavePending {
		t.Fatal("old snapshot overwrote current playback or lost its followup")
	}
	second := m.startVolumeSave(m.volumeSaveDue)
	if second == nil {
		t.Fatal("latest volume had no followup write")
	}
	_, _ = m.Update(second())
	disk, err = config.Load()
	if err != nil || disk.DefaultVolume != .8 || m.volumeSavePending {
		t.Fatal("latest volume did not reach disk")
	}
}

func TestVolumeSaveFailureWaitsForChangeAndPreservesValue(t *testing.T) {
	m := wiredSettingsModel(t)
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	obstacle := filepath.Join(dir, "config.json")
	if err := os.MkdirAll(obstacle, 0700); err != nil {
		t.Fatal(err)
	}
	m.setVolume(.7)
	cmd := m.startVolumeSave(m.volumeSaveDue)
	_, _ = m.Update(cmd())
	if !m.Error.Visible || !m.volumeSavePending || m.Audio.Volume != .7 || m.startVolumeSave(time.Now().Add(time.Hour)) != nil {
		t.Fatal("failed writer lost the volume or retried on every tick")
	}
	if err := os.Remove(obstacle); err != nil {
		t.Fatal(err)
	}
	m.setVolume(.8)
	cmd = m.startVolumeSave(m.volumeSaveDue)
	if cmd == nil {
		t.Fatal("new volume did not rearm persistence")
	}
	_, _ = m.Update(cmd())
	disk, err := config.Load()
	if err != nil || disk.DefaultVolume != .8 || m.volumeSavePending {
		t.Fatal("rearmed writer did not save the latest value")
	}
}

func TestVolumeTickSchedulesWriterAndShutdownFinishesItOnce(t *testing.T) {
	m := wiredSettingsModel(t)
	m.setVolume(.7)
	m.volumeSaveDue = time.Now().Add(-time.Second)
	_, _ = m.Update(TickMsg{})
	job := m.volumeSave
	if job == nil || config.Exists() {
		t.Fatal("tick did not schedule an asynchronous volume write")
	}
	m.Close()
	disk, err := config.Load()
	if err != nil || disk.DefaultVolume != .7 || m.volumeSave != nil {
		t.Fatal("shutdown did not finish the unstarted volume command")
	}
	disk.DefaultVolume = .9
	if err := disk.Save(); err != nil {
		t.Fatal(err)
	}
	_, _ = m.Update(job.run())
	disk, err = config.Load()
	if err != nil || disk.DefaultVolume != .9 || m.volumeSave != nil {
		t.Fatal("late volume command rewrote newer settings")
	}
}

func TestVolumeNoOpDoesNotScheduleOrDelaySave(t *testing.T) {
	m := wiredSettingsModel(t)
	m.setVolume(.4)
	if m.volumeSavePending || m.startVolumeSave(time.Now().Add(time.Hour)) != nil {
		t.Fatal("no-op volume scheduled a save")
	}
	m.setVolume(.7)
	due := m.volumeSaveDue
	m.setVolume(.7)
	if !m.volumeSaveDue.Equal(due) {
		t.Fatal("no-op volume postponed pending persistence")
	}
	m.Close()
}

func TestSettingsSaveDoesNotRunPendingVolumeWriterInUpdate(t *testing.T) {
	m := wiredSettingsModel(t)
	m.setVolume(.7)
	volume := m.startVolumeSave(time.Now().Add(time.Second))
	if volume == nil {
		t.Fatal("volume writer was not scheduled")
	}
	editRealSetting(t, m, "Seek step", "21")
	settings := m.saveSettings(false)
	if config.Exists() {
		t.Error("settings input executed the pending volume writer")
	}
	completeSettingsCmd(t, m, settings)
	_, _ = m.Update(volume())
	disk, err := config.Load()
	if err != nil || disk.SeekStep != 21 || disk.DefaultVolume != .7 || m.volumeSave != nil {
		t.Fatalf("late volume writer reverted explicit settings: %+v %v", disk, err)
	}
}

func TestSettingsSaveFailureKeepsPendingRememberedVolume(t *testing.T) {
	m := wiredSettingsModel(t)
	m.setVolume(.7)
	editRealSetting(t, m, "Seek step", "21")
	// Validation fails before writing, so the volume must remain pending.
	m.pages.changed["library.roots"] = true
	m.pages.draft.Library.Roots = []string{filepath.Join(t.TempDir(), "missing")}
	completeSettingsCmd(t, m, m.saveSettings(false))
	if !m.volumeSavePending {
		t.Fatal("failed explicit save lost a pending volume write")
	}
	m.Close()
	disk, err := config.Load()
	if err != nil || disk.DefaultVolume != .7 || disk.SeekStep != 13 {
		t.Fatalf("failed settings save lost volume or saved its draft: %+v %v", disk, err)
	}
}

func TestSettingsCompletionDefersFollowupVolumeWrite(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	settings := m.saveSettings(false)
	m.setVolume(.7)
	completeSettingsCmd(t, m, settings)
	disk, err := config.Load()
	if err != nil || disk.DefaultVolume != .4 || m.Audio.Volume != .7 || m.Config.DefaultVolume != .7 {
		t.Fatalf("completion wrote synchronously or lost current volume: disk=%+v live=%v err=%v", disk, m.Audio.Volume, err)
	}
	cmd := m.startVolumeSave(time.Now().Add(time.Second))
	if cmd == nil {
		t.Fatal("latest volume did not schedule a followup writer")
	}
	_, _ = m.Update(cmd())
	disk, err = config.Load()
	if err != nil || disk.DefaultVolume != .7 || disk.SeekStep != 21 {
		t.Fatal("followup volume snapshot lost explicit settings")
	}
}

func TestVolumeBurstKeepsDiskUnchangedUntilSaveAndFlushesOnExit(t *testing.T) {
	m := wiredSettingsModel(t)
	if err := m.Config.Save(); err != nil {
		t.Fatal(err)
	}
	editRealSetting(t, m, "Seek step", "21")
	for i := range 20 {
		m.setVolume(float64(30+i) / 100)
	}
	disk, err := config.Load()
	if err != nil || disk.DefaultVolume != .4 {
		t.Errorf("volume keys synchronously changed disk: volume=%v err=%v", disk.DefaultVolume, err)
	}
	if m.Audio.Volume != .49 || m.Config.DefaultVolume != .49 || m.pages.draft.SeekStep != 21 {
		t.Fatal("deferring persistence delayed playback or discarded a settings draft")
	}
	m.Close()
	disk, err = config.Load()
	if err != nil || disk.DefaultVolume != .49 || disk.SeekStep != 13 {
		t.Fatalf("exit lost latest volume or saved unrelated draft: %+v %v", disk, err)
	}
}

func TestVolumeInputDoesNotCreateConfigBeforeBackgroundSave(t *testing.T) {
	m := wiredSettingsModel(t)
	m.setVolume(.7)
	if config.Exists() {
		t.Fatal("volume input performed configuration I/O")
	}
	m.Close()
	disk, err := config.Load()
	if err != nil || disk.DefaultVolume != .7 {
		t.Fatal("shutdown did not finish the deferred volume save")
	}
}

func BenchmarkVolumeInputBurst(b *testing.B) {
	config.SetXDGConfig(true)
	b.Cleanup(func() { config.SetXDGConfig(false) })
	b.Setenv("XDG_CONFIG_HOME", b.TempDir())
	b.Setenv("XDG_CACHE_HOME", b.TempDir())
	m := benchmarkPageModel(0)
	b.Cleanup(m.Close)
	b.ReportAllocs()
	for b.Loop() {
		for i := range 20 {
			m.setVolume(float64(30+i%10) / 100)
		}
	}
}

func BenchmarkVolumeBurstWithPersistence(b *testing.B) {
	config.SetXDGConfig(true)
	b.Cleanup(func() { config.SetXDGConfig(false) })
	b.Setenv("XDG_CONFIG_HOME", b.TempDir())
	b.Setenv("XDG_CACHE_HOME", b.TempDir())
	m := benchmarkPageModel(0)
	b.Cleanup(m.Close)
	b.ReportAllocs()
	for b.Loop() {
		for i := range 20 {
			m.setVolume(float64(30+i%10) / 100)
		}
		// Include the final write/sync as well as input processing in this metric.
		m.flushDeferredVolume()
	}
}
