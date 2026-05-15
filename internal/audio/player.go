package audio

import (
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/speaker"
	"github.com/jpodeszfa/go-meltysynth/meltysynth"

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

type Player struct {
	mu           sync.Mutex
	streamer     beep.StreamSeekCloser
	ctrl         *beep.Ctrl
	volume       *effects.Volume
	format       beep.Format
	isPaused     bool
	isPlaying    bool
	file         io.Closer
	path         string
	linearVolume float64
	title        string
	artist       string
	coverImage   image.Image
	decoder      *FormatDecoder
	tagReader    *MetadataReader
	synthCtrl      SynthController
	sfPath         string
	cachedSF       *meltysynth.SoundFont
	cachedSFPath   string
	trackerBackend string
}

func NewPlayer() *Player {
	return NewPlayerWithDeps(NewFormatDecoder(), NewMetadataReader())
}

func NewPlayerWithDeps(decoder *FormatDecoder, tagReader *MetadataReader) *Player {
	return &Player{
		isPaused:     true,
		isPlaying:    false,
		linearVolume: 1.0,
		decoder:      decoder,
		tagReader:    tagReader,
	}
}

func (p *Player) SetSoundfontPath(sfPath string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sfPath = sfPath
}

func (p *Player) SetTrackerBackend(backend string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.trackerBackend = backend
}

func (p *Player) Open(path string) error {
	logger.Debug("Player.Open", "path", path)

	p.mu.Lock()
	defer p.mu.Unlock()

	detectFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	ext, detectErr := p.decoder.DetectFormatByMagic(detectFile)
	detectFile.Close()

	synthExt := ext
	if detectErr != nil {
		synthExt = filepath.Ext(path)
	} else if filepath.Ext(path) == ".mptm" {
		synthExt = ".mptm"
	}

	if isSyntheticFormat(synthExt) {
		logger.Info("Detected synthetic format", "path", path, "ext", synthExt)
		return p.openSynthetic(path, synthExt)
	}

	if p.isPlaying {
		speaker.Clear()
		p.isPlaying = false
	}
	if p.streamer != nil {
		if p.file != nil {
			p.file.Close()
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open audio file: %w", err)
	}

	streamer, format, err := p.decoder.Decode(file, path)
	if err != nil {
		file.Close()
		return err
	}

	if err := ensureSpeakerInit(format.SampleRate); err != nil {
		file.Close()
		return fmt.Errorf("speaker init failed: %w", err)
	}

	logger.Info("Audio file opened", "path", path, "format", format.SampleRate)

	p.streamer = streamer
	p.format = format
	p.file = file
	p.isPaused = true
	p.isPlaying = false
	p.path = path

	p.ctrl = &beep.Ctrl{
		Streamer: streamer,
		Paused:   true,
	}

	p.volume = &effects.Volume{
		Streamer: p.ctrl,
		Base:     2,
		Silent:   false,
	}

	p.applyLinearVolumeLocked()

	p.readTags(path)

	return nil
}

func (p *Player) applyLinearVolumeLocked() {
	if p.volume == nil {
		return
	}

	if p.linearVolume <= 0.0 {
		p.volume.Silent = true
		p.volume.Volume = 0
	} else {
		p.volume.Silent = false
		exponent := math.Log2(p.linearVolume)
		p.volume.Volume = exponent
	}
}

func (p *Player) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.synthCtrl != nil {
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

func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.synthCtrl != nil {
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

	if p.synthCtrl != nil {
		logger.Debug("Synth resume")
		p.synthCtrl.Play()
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

	if p.synthCtrl != nil {
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
	if p.synthCtrl != nil {
		if p.isPaused || !p.isPlaying {
			p.Play()
		} else {
			p.Pause()
		}
		return
	}
	if p.isPaused || !p.isPlaying {
		p.Play()
	} else {
		p.Pause()
	}
}

func (p *Player) Seek(position time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.synthCtrl != nil {
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
		Paused:   true,
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

func (p *Player) SetVolume(vol float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if vol < 0 {
		vol = 0
	}
	if vol > 1 {
		vol = 1
	}
	p.linearVolume = vol

	if p.synthCtrl != nil {
		p.synthCtrl.SetVolume(vol)
	}
	logger.Debug("Volume set", "volume", vol)
	p.applyLinearVolumeLocked()
}

func (p *Player) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.linearVolume
}

func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isPlaying && !p.isPaused
}

func (p *Player) Duration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.synthCtrl != nil {
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
	if p.synthCtrl != nil {
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

	if p.synthCtrl != nil {
		logger.Debug("Player.Close (synth)")
		return p.synthCtrl.Close()
	}

	logger.Debug("Player.Close (audio)")
	speaker.Clear()
	if p.file != nil {
		p.file.Close()
		p.file = nil
	}
	p.streamer = nil
	p.ctrl = nil
	p.volume = nil
	p.isPaused = true
	p.isPlaying = false
	return nil
}

func (p *Player) Path() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.path
}

func (p *Player) Title() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.synthCtrl != nil {
		return p.synthCtrl.Title()
	}
	return p.title
}

func (p *Player) Artist() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.synthCtrl != nil {
		return p.synthCtrl.Artist()
	}
	return p.artist
}

func (p *Player) CoverImage() image.Image {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.synthCtrl != nil {
		return p.synthCtrl.CoverImage()
	}
	return p.coverImage
}

func (p *Player) Format() beep.Format {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.synthCtrl != nil {
		return p.synthCtrl.Streamer().Format()
	}
	return p.format
}

var ErrUnsupportedFormat = &UnsupportedFormatError{}

type UnsupportedFormatError struct{}

func (e *UnsupportedFormatError) Error() string {
	return "unsupported audio format"
}

var syntheticFormats = map[string]bool{
	".mid":  true,
	".mod":  true,
	".xm":   true,
	".s3m":  true,
	".it":   true,
	".mptm": true,
}

func isSyntheticFormat(ext string) bool {
	return syntheticFormats[ext]
}

func (p *Player) openSynthetic(path, ext string) error {
	logger.Info("Opening synthetic", "path", path, "ext", ext)

	if p.isPlaying {
		speaker.Clear()
		p.isPlaying = false
	}
	if p.streamer != nil {
		if p.file != nil {
			p.file.Close()
		}
	}
	if p.synthCtrl != nil {
		p.synthCtrl.Close()
		p.synthCtrl = nil
	}

	sr := speakerSampleRate
	if sr == 0 {
		sr = 44100
	}
	if err := ensureSpeakerInit(sr); err != nil {
		return fmt.Errorf("speaker init: %w", err)
	}

	switch ext {
	case ".mid":
		return p.openMIDISynth(path, sr)
	case ".mod", ".xm", ".s3m", ".it":
		return p.openTrackerSynth(path, ext, sr)
	default:
		return fmt.Errorf("unknown synthetic format: %s", ext)
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

	mp, sf, err := NewMidiPlayer(path, p.sfPath, cachedSF, sr)
	if err != nil {
		return err
	}
	p.cachedSF = sf
	p.cachedSFPath = p.sfPath

	mp.SetTitle(filepath.Base(path))
	mp.SetArtist("MIDI")
	mp.SetVolume(p.linearVolume)

	p.synthCtrl = mp
	p.path = path
	p.isPaused = true
	p.isPlaying = false

	return nil
}

func (p *Player) openTrackerSynth(path, ext string, sr beep.SampleRate) error {
	var ctrl SynthController
	var err error

	if ext == ".mptm" {
		ctrl, err = NewOpenmptPlayer(path, sr)
		if err != nil {
			return fmt.Errorf("mptm format requires libopenmpt: %w", err)
		}
		ctrl.SetVolume(p.linearVolume)
		p.synthCtrl = ctrl
		p.path = path
		p.isPaused = true
		p.isPlaying = false
		return nil
	}

	backend := p.trackerBackend
	if backend == "" {
		backend = "auto"
	}

	switch backend {
	case "gotracker":
		ctrl, err = NewTrackerPlayer(path, ext, sr)
	case "openmpt":
		ctrl, err = NewOpenmptPlayer(path, sr)
		if err != nil {
			logger.Info("openmpt unavailable, falling back to gotracker", "err", err)
			ctrl, err = NewTrackerPlayer(path, ext, sr)
		}
	default:
		ctrl, err = NewOpenmptPlayer(path, sr)
		if err != nil {
			logger.Info("openmpt unavailable, falling back to gotracker", "err", err)
			ctrl, err = NewTrackerPlayer(path, ext, sr)
		}
	}

	if err != nil {
		return err
	}

	ctrl.SetVolume(p.linearVolume)

	p.synthCtrl = ctrl
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
	p.synthCtrl.Play()
	speaker.Unlock()

	p.isPlaying = true
	p.isPaused = false
	return nil
}
