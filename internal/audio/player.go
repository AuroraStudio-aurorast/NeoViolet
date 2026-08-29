package audio

import (
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/speaker"
	"github.com/jpodeszfa/go-meltysynth/meltysynth"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

var initSpeakerOnce sync.Once
var initSpeakerErr error
var speakerSampleRate beep.SampleRate

func ensureSpeakerInit(sampleRate beep.SampleRate) error {
	initSpeakerOnce.Do(func() {
		initSpeakerErr = speaker.Init(sampleRate, sampleRate.N(time.Second/10))
		if initSpeakerErr == nil {
			speakerSampleRate = sampleRate
		}
	})
	return initSpeakerErr
}

// Player is the audio playback engine. It wraps a beep stream plus an
// optional synthetic (MIDI/tracker) controller behind a single mutex-guarded
// API used by the UI.
type Player struct {
	mu             sync.Mutex
	streamer       beep.StreamSeekCloser
	ctrl           *beep.Ctrl
	volume         *effects.Volume
	format         beep.Format
	isPaused       bool
	isPlaying      bool
	file           io.Closer
	path           string
	linearVolume   float64
	title          string
	artist         string
	album          string
	coverImage     image.Image
	decoder        *format.FormatDecoder
	tagReader      *format.MetadataReader
	synthCtrl      synth.Controller
	synthActive    bool
	sfPath         string
	cachedSF       *meltysynth.SoundFont
	cachedSFPath   string
	trackerBackend string
	tempFiles      []string // temp files to clean up in Close()
}

// NewPlayer creates a Player with the default format decoder and metadata reader.
func NewPlayer() *Player {
	return NewPlayerWithDeps(format.NewFormatDecoder(), format.NewMetadataReader())
}

// NewPlayerWithDeps creates a Player using the supplied decoder and tag reader.
// It is used by tests to inject fakes without touching the real format backends.
func NewPlayerWithDeps(decoder *format.FormatDecoder, tagReader *format.MetadataReader) *Player {
	return &Player{
		isPaused:     true,
		isPlaying:    false,
		linearVolume: 1.0,
		decoder:      decoder,
		tagReader:    tagReader,
	}
}

// isSynthActive returns true when a synthetic (MIDI/tracker) stream is active.
func (p *Player) isSynthActive() bool {
	return p.synthActive && p.synthCtrl != nil
}

// setupStreamer sets the common streamer, ctrl, volume fields after opening audio.
// Must be called with p.mu held (the caller's Open/OpenReader/openURL holds it).
func (p *Player) setupStreamer(streamer beep.StreamSeekCloser, f beep.Format, file io.Closer, path string, ctrlStreamer beep.Streamer) {
	p.streamer = streamer
	p.format = f
	p.file = file
	p.isPaused = true
	p.isPlaying = false
	p.path = path
	p.synthActive = false

	p.ctrl = &beep.Ctrl{
		Streamer: ctrlStreamer,
		Paused:   true,
	}

	p.volume = &effects.Volume{
		Streamer: p.ctrl,
		Base:     2,
		Silent:   false,
	}

	p.applyLinearVolumeLocked()
}

// SetSoundfontPath configures the SoundFont path used for MIDI playback.
func (p *Player) SetSoundfontPath(sfPath string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sfPath = sfPath
}

// SetTrackerBackend selects the tracker playback backend (auto/gotracker/openmpt).
func (p *Player) SetTrackerBackend(backend string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.trackerBackend = backend
}

// Open loads a local audio file (or remote URL) for playback.
func (p *Player) Open(path string) error {
	logger.Debug("Player.Open", "path", path)

	if isURL(path) {
		return p.openURL(path)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}

	ext, detectErr := p.decoder.DetectFormatByMagic(file)
	synthExt := ext
	if detectErr != nil {
		synthExt = filepath.Ext(path)
	}

	if isSyntheticFormat(synthExt) {
		_ = file.Close()
		logger.Info("Detected synthetic format", "path", path, "ext", synthExt)
		return p.openSynthetic(path, synthExt)
	}

	if p.isPlaying {
		speaker.Clear()
		p.isPlaying = false
	}
	if p.streamer != nil && p.file != nil {
		_ = p.file.Close()
	}

	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		_ = file.Close()
		return fmt.Errorf("file seek: %w", seekErr)
	}

	streamer, format, err := p.decoder.Decode(file, path)
	if err != nil {
		_ = file.Close()
		return err
	}

	if err := ensureSpeakerInit(format.SampleRate); err != nil {
		_ = file.Close()
		return fmt.Errorf("speaker init failed: %w", err)
	}

	// Resample if the streamer's native sample rate differs from the
	// speaker's hardware rate. The beep speaker is initialized once at
	// the first song's rate and cannot be re-initialized; without
	// resampling, a rate mismatch causes pitch/tempo distortion.
	ctrlStreamer := resampleIfNeeded(streamer, format)

	logger.Info("Audio file opened", "path", path, "format", format.SampleRate)

	p.setupStreamer(streamer, format, file, path, ctrlStreamer)

	p.readTags(path)

	return nil
}

// closeStreamer stops playback and closes the file/streamer resources. Caller must hold p.mu.
func (p *Player) closeStreamer() {
	if p.isPlaying {
		speaker.Clear()
		p.isPlaying = false
	}
	if p.streamer != nil && p.file != nil {
		_ = p.file.Close()
		p.file = nil
	}
	p.streamer = nil
	p.ctrl = nil
	p.volume = nil
}

// Play starts or resumes playback.
func (p *Player) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isSynthActive() {
		logger.Debug("Synth play")
		return p.playSynthetic()
	}

	if p.streamer == nil || p.volume == nil {
		return fmt.Errorf("player not initialized")
	}

	speaker.Lock()
	p.ctrl.Paused = false
	speaker.Unlock()

	if p.isPlaying {
		speaker.Clear()
		p.isPlaying = false
	}

	logger.Debug("Audio play/resume")
	speaker.Play(p.volume)
	p.isPlaying = true
	p.isPaused = false
	return nil
}

// resampleIfNeeded wraps the streamer with beep.ResampleRatio when the
// source sample rate differs from the speaker's hardware sample rate.
// The beep speaker is initialized once (at the first song's rate) and
// cannot be re-initialized; without resampling, a rate mismatch causes
// incorrect pitch and playback speed.
//
// The original streamer (p.streamer) is kept as-is for seeking; the
// resampled wrapper is used only in the Ctrl → Volume playback chain.
func resampleIfNeeded(s beep.Streamer, f beep.Format) beep.Streamer {
	if speakerSampleRate == 0 || f.SampleRate == speakerSampleRate {
		return s
	}
	ratio := float64(f.SampleRate) / float64(speakerSampleRate)
	logger.Info("Resampling audio",
		"from", f.SampleRate,
		"to", speakerSampleRate,
		"ratio", ratio,
	)
	return beep.ResampleRatio(4, ratio, s)
}

func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isSynthActive() {
		logger.Debug("Synth pause")
		p.synthCtrl.Pause()
		p.isPaused = true
		return
	}

	if p.ctrl == nil {
		return
	}
	logger.Debug("Audio pause")
	speaker.Lock()
	p.ctrl.Paused = true
	speaker.Unlock()
	p.isPaused = true
}

func (p *Player) Resume() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isSynthActive() {
		logger.Debug("Synth resume")
		_ = p.synthCtrl.Play()
		p.isPaused = false
		return
	}

	if p.ctrl == nil {
		return
	}
	logger.Debug("Audio resume")
	speaker.Lock()
	p.ctrl.Paused = false
	speaker.Unlock()
	p.isPaused = false
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isSynthActive() {
		logger.Debug("Synth stop")
		p.synthCtrl.Stop()
		p.isPlaying = false
		p.isPaused = true
		return
	}

	logger.Debug("Audio stop")
	speaker.Clear()
	p.isPlaying = false
	p.isPaused = true
}

func (p *Player) Toggle() {
	if p.isSynthActive() {
		if p.isPaused || !p.isPlaying {
			_ = p.Play()
		} else {
			p.Pause()
		}
		return
	}
	if p.isPaused || !p.isPlaying {
		_ = p.Play()
	} else {
		p.Pause()
	}
}

func (p *Player) Seek(position time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isSynthActive() {
		logger.Debug("Synth seek", "position", position)
		return p.synthCtrl.Seek(position)
	}

	if p.streamer == nil || p.path == "" {
		return nil
	}

	targetSamples := int(position.Seconds() * float64(p.format.SampleRate))
	if targetSamples < 0 {
		targetSamples = 0
	}
	totalSamples := p.streamer.Len()
	if targetSamples > totalSamples {
		targetSamples = totalSamples
	}

	wasPlaying := p.isPlaying && !p.isPaused

	speaker.Clear()

	if err := p.streamer.Seek(targetSamples); err != nil {
		return fmt.Errorf("seek to %v: %w", position, err)
	}

	p.ctrl = &beep.Ctrl{
		Streamer: p.streamer,
		Paused:   !wasPlaying,
	}
	p.volume = &effects.Volume{
		Streamer: p.ctrl,
		Base:     2,
		Silent:   false,
	}
	p.applyLinearVolumeLocked()

	if wasPlaying {
		speaker.Lock()
		p.ctrl.Paused = false
		speaker.Unlock()
		speaker.Play(p.volume)
		p.isPlaying = true
		p.isPaused = false
	} else {
		p.isPlaying = false
		p.isPaused = true
	}

	return nil
}

func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isPlaying && !p.isPaused
}

func (p *Player) Duration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isSynthActive() {
		return p.synthCtrl.Duration()
	}
	if p.streamer == nil {
		return 0
	}
	return p.format.SampleRate.D(p.streamer.Len())
}

func (p *Player) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isSynthActive() {
		return p.synthCtrl.Position()
	}
	if p.streamer == nil {
		return 0
	}
	return p.format.SampleRate.D(p.streamer.Position())
}

func (p *Player) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isSynthActive() {
		logger.Debug("Player.Close (synth)")
		return p.synthCtrl.Close()
	}

	logger.Debug("Player.Close (audio)")
	speaker.Clear()
	if p.file != nil {
		_ = p.file.Close()
		p.file = nil
	}
	p.streamer = nil
	p.ctrl = nil
	p.volume = nil
	p.isPaused = true
	p.isPlaying = false

	// Clean up any temp files created by OpenReader
	for _, tmpPath := range p.tempFiles {
		logger.Debug("Removing temp file", "path", tmpPath)
		_ = os.Remove(tmpPath)
	}
	p.tempFiles = nil

	// Release the cached SoundFont — it is only needed during MIDI playback.
	p.cachedSF = nil
	p.cachedSFPath = ""

	return nil
}

func (p *Player) Format() beep.Format {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isSynthActive() {
		return p.synthCtrl.Streamer().Format()
	}
	return p.format
}
