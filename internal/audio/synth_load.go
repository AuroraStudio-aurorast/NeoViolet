package audio

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/speaker"
	"github.com/jpodeszfa/go-meltysynth/meltysynth"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// UnloadSoundfont releases the cached SoundFont to free memory.
// It is safe to call at any time; the SoundFont will be reloaded
// on the next MIDI playback if needed.
func (p *Player) UnloadSoundfont() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cachedSF != nil {
		logger.Debug("Releasing cached SoundFont")
		p.cachedSF = nil
		p.cachedSFPath = ""
	}
}

var syntheticFormats = map[string]bool{
	".mid":  true,
	".midi": true,
	".mod":  true,
	".xm":   true,
	".s3m":  true,
	".it":   true,
	".mptm": true,
	".stm":  true,
	".nst":  true,
	".wow":  true,
	".ult":  true,
	".669":  true,
	".mtm":  true,
	".mdl":  true,
	".far":  true,
	".ptm":  true,
	".okt":  true,
	".dmf":  true,
	".dbm":  true,
	".digi": true,
	".imf":  true,
	".j2b":  true,
	".mo3":  true,
	".umx":  true,
	".gdm":  true,
}

func isSyntheticFormat(ext string) bool {
	if syntheticFormats[ext] {
		return true
	}
	for _, se := range synth.OpenmptSupportedFormats() {
		if "."+se == ext {
			return true
		}
	}
	return false
}

// IsSyntheticFormat reports whether ext is a synthesized format (MIDI/tracker)
// that typically carries no lyrics, so online lyric fetch is skipped for it.
func IsSyntheticFormat(ext string) bool { return isSyntheticFormat(ext) }

// gotrackerFormats are the extensions the built-in tracker engine decodes.
var gotrackerFormats = map[string]bool{
	".mod": true,
	".xm":  true,
	".s3m": true,
	".it":  true,
}

// gotrackerSupports reports whether the built-in tracker engine may be handed
// this file; a nil error means it may. The engine decodes MOD/XM/IT/S3M only,
// and the format detector reports other IT-family containers as ".it" because
// they carry the IT magic — an MPTM module reached the engine's IT loader that
// way and made it panic inside its instrument loader. The file's own extension
// is therefore what decides here. Only tracker formats outside those four are
// refused; a nameless or unrelated extension is left to the engine, which
// reports it as unsupported on its own.
func gotrackerSupports(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if gotrackerFormats[ext] || !syntheticFormats[ext] {
		return nil
	}
	switch ext {
	case ".mid", ".midi":
		// MIDI comes from a SoundFont on its own path, not from this engine.
		return nil
	}
	return fmt.Errorf("%s needs libopenmpt: the built-in tracker engine decodes MOD, XM, IT and S3M only", strings.ToUpper(ext))
}

func (p *Player) openSynthetic(path, ext string) error {
	logger.Info("Opening synthetic", "path", path, "ext", ext)

	p.closeStreamer()
	if p.synthCtrl != nil {
		_ = p.synthCtrl.Close()
		p.synthCtrl = nil
	}
	p.synthActive = false

	sr := speakerSampleRate
	if sr == 0 {
		sr = 44100
	}
	if err := ensureSpeakerInit(sr); err != nil {
		return fmt.Errorf("speaker init: %w", err)
	}

	switch ext {
	case ".mid", ".midi":
		return p.openMIDISynth(path, sr)
	default:
		// All tracker formats (MOD, XM, S3M, IT, and OpenMPT-only
		// formats like MPTM) route through openTrackerSynth — it tries
		// OpenMPT first, then falls back to gotracker.
		// Release the SoundFont cache — it is only needed for MIDI playback.
		p.cachedSF = nil
		p.cachedSFPath = ""
		return p.openTrackerSynth(path, ext, sr)
	}
}

func (p *Player) openMIDISynth(path string, sr beep.SampleRate) error {
	if p.sfPath == "" {
		return fmt.Errorf("soundfont_path not configured for MIDI playback")
	}

	var cachedSF *meltysynth.SoundFont
	if p.cachedSF != nil && p.cachedSFPath == p.sfPath {
		cachedSF = p.cachedSF
	}

	mp, sf, err := synth.NewMidiPlayer(path, p.sfPath, cachedSF, sr)
	if err != nil {
		return err
	}
	p.cachedSF = sf
	p.cachedSFPath = p.sfPath

	mp.SetTitle(filepath.Base(path))
	mp.SetArtist("MIDI")
	mp.SetVolume(p.linearVolume)

	p.synthCtrl = mp
	p.synthActive = true
	p.path = path
	p.isPaused = true
	p.isPlaying = false

	return nil
}

func (p *Player) openTrackerSynth(path, ext string, sr beep.SampleRate) error {
	var ctrl synth.Controller
	var err error

	backend := p.trackerBackend
	if backend == "" {
		backend = "auto"
	}

	switch backend {
	case "gotracker":
		if err := gotrackerSupports(path); err != nil {
			return err
		}
		ctrl, err = synth.NewTrackerPlayer(path, ext, sr)
	default:
		// Try OpenMPT first, then fall back to gotracker.
		ctrl, err = synth.NewOpenmptPlayer(path, sr)
		if err != nil {
			logger.Info("openmpt unavailable, falling back to gotracker", "err", err)
			if gateErr := gotrackerSupports(path); gateErr != nil {
				return gateErr
			}
			ctrl, err = synth.NewTrackerPlayer(path, ext, sr)
		}
	}

	if err != nil {
		return err
	}

	ctrl.SetVolume(p.linearVolume)

	p.synthCtrl = ctrl
	p.synthActive = true
	p.path = path
	p.isPaused = true
	p.isPlaying = false

	return nil
}

func (p *Player) playSynthetic() error {
	if p.synthCtrl == nil {
		return fmt.Errorf("no synth controller")
	}

	if !p.isPlaying {
		logger.Info("Synth playback start")
		speaker.Play(p.synthCtrl.Streamer())
	}

	speaker.Lock()
	_ = p.synthCtrl.Play()
	speaker.Unlock()

	p.isPlaying = true
	p.isPaused = false
	return nil
}
