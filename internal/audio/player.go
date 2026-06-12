package audio

import (
	"bytes"
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

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/cover"
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

func NewPlayer() *Player {
	return NewPlayerWithDeps(format.NewFormatDecoder(), format.NewMetadataReader())
}

func NewPlayerWithDeps(decoder *format.FormatDecoder, tagReader *format.MetadataReader) *Player {
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
		file.Close()
		logger.Info("Detected synthetic format", "path", path, "ext", synthExt)
		return p.openSynthetic(path, synthExt)
	}

	if p.isPlaying {
		speaker.Clear()
		p.isPlaying = false
	}
	if p.streamer != nil && p.file != nil {
		p.file.Close()
	}

	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		file.Close()
		return fmt.Errorf("file seek: %w", seekErr)
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
	p.synthActive = false

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

func (p *Player) readTags(path string) {
	metadata := p.tagReader.Read(path)
	p.title = metadata.Title
	p.artist = metadata.Artist

	img, err := cover.ExtractFromFile(path)
	if err == nil {
		p.coverImage = img
	}
}

// readSeekCloser wraps *bytes.Reader to implement both io.ReadSeeker and io.ReadCloser.
// This is needed by decoders that require Seek for Len() computation (e.g. MP3)
// while also needing Close().
type readSeekCloser struct {
	*bytes.Reader
}

func (r *readSeekCloser) Close() error { return nil }

// OpenReader opens audio from an in-memory byte buffer (e.g. from stdin).
// name is a display label (e.g. "stdin") shown in the UI.
// For formats that require a file path (APE, MIDI, tracker), the data is
// transparently written to a temporary file and opened via the normal Open path.
func (p *Player) OpenReader(name string, data []byte) error {
	logger.Debug("Player.OpenReader", "name", name, "size", len(data))

	if len(data) == 0 {
		return fmt.Errorf("stdin is empty")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Detect format from magic bytes
	ext, detectErr := p.decoder.DetectFormatFromBytes(data)
	synthExt := ext
	if detectErr != nil {
		// Can't detect — attempt to pick a reasonable default or error out
		return fmt.Errorf("stdin: %w", detectErr)
	}

	// Synthetic formats (MIDI, tracker) and APE require a file path.
	// Write to a temp file and delegate to normal Open.
	if isSyntheticFormat(synthExt) || ext == ".ape" {
		tmpFile, err := os.CreateTemp("", "neoviolet-stdin-*"+ext)
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
		tmpPath := tmpFile.Name()
		if _, err := tmpFile.Write(data); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("write temp file: %w", err)
		}
		if err := tmpFile.Close(); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("close temp file: %w", err)
		}
		// Track for cleanup
		p.tempFiles = append(p.tempFiles, tmpPath)
		p.mu.Unlock()
		err = p.Open(tmpPath)
		p.mu.Lock()
		// Override the path to "stdin" for display purposes
		p.path = name
		return err
	}

	// Standard audio formats: decode from memory
	if p.isPlaying {
		speaker.Clear()
		p.isPlaying = false
	}
	if p.streamer != nil && p.file != nil {
		p.file.Close()
	}

	// Read metadata from the buffer FIRST, before decoding.
	// Use a separate reader so we never touch the decoder's internal reader.
	metaReader := bytes.NewReader(data)
	metadata := p.tagReader.ReadFromSeeker(metaReader)
	p.title = metadata.Title
	p.artist = metadata.Artist

	// Extract cover art from the buffer (using another reader).
	coverReader := bytes.NewReader(data)
	img, err := cover.ExtractFromReader(coverReader)
	if err == nil {
		p.coverImage = img
	}

	// Now decode — use its own fresh *bytes.Reader so the decoder has
	// exclusive ownership and its internal buffers are never corrupted.
	// The wrapper implements both io.ReadSeeker (for Len()) and io.ReadCloser.
	decReader := &readSeekCloser{Reader: bytes.NewReader(data)}
	streamer, format, err := p.decoder.DecodeFromReader(decReader, ext)
	if err != nil {
		return fmt.Errorf("decode stdin: %w", err)
	}

	if err := ensureSpeakerInit(format.SampleRate); err != nil {
		return fmt.Errorf("speaker init failed: %w", err)
	}

	logger.Info("Audio loaded from stdin", "name", name, "format", format.SampleRate)

	p.streamer = streamer
	p.format = format
	p.file = decReader
	p.isPaused = true
	p.isPlaying = false
	p.path = name
	p.synthActive = false

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

	// If no title detected from tags, use the display name
	if p.title == "" {
		p.title = name
	}

	return nil
}

// closeStreamer stops playback and closes the file/streamer resources. Caller must hold p.mu.
func (p *Player) closeStreamer() {
	if p.isPlaying {
		speaker.Clear()
		p.isPlaying = false
	}
	if p.streamer != nil && p.file != nil {
		p.file.Close()
		p.file = nil
	}
	p.streamer = nil
	p.ctrl = nil
	p.volume = nil
}

func (p *Player) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.synthActive && p.synthCtrl != nil {
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

	if p.synthActive && p.synthCtrl != nil {
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

	if p.synthActive && p.synthCtrl != nil {
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

	if p.synthActive && p.synthCtrl != nil {
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
	if p.synthActive && p.synthCtrl != nil {
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

	if p.synthActive && p.synthCtrl != nil {
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

	if p.synthActive && p.synthCtrl != nil {
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
	if p.synthActive && p.synthCtrl != nil {
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
	if p.synthActive && p.synthCtrl != nil {
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

	if p.synthActive && p.synthCtrl != nil {
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

	// Clean up any temp files created by OpenReader
	for _, tmpPath := range p.tempFiles {
		logger.Debug("Removing temp file", "path", tmpPath)
		os.Remove(tmpPath)
	}
	p.tempFiles = nil

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
	if p.synthActive && p.synthCtrl != nil {
		return p.synthCtrl.Title()
	}
	return p.title
}

func (p *Player) Artist() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.synthActive && p.synthCtrl != nil {
		return p.synthCtrl.Artist()
	}
	return p.artist
}

func (p *Player) CoverImage() image.Image {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.synthActive && p.synthCtrl != nil {
		return p.synthCtrl.CoverImage()
	}
	return p.coverImage
}

func (p *Player) Format() beep.Format {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.synthActive && p.synthCtrl != nil {
		return p.synthCtrl.Streamer().Format()
	}
	return p.format
}

var syntheticFormats = map[string]bool{
	".mid":  true,
	".midi": true,
	".mod":  true,
	".xm":   true,
	".s3m":  true,
	".it":   true,
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

func (p *Player) openSynthetic(path, ext string) error {
	logger.Info("Opening synthetic", "path", path, "ext", ext)

	p.closeStreamer()
	if p.synthCtrl != nil {
		p.synthCtrl.Close()
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
		ctrl, err = synth.NewTrackerPlayer(path, ext, sr)
	case "openmpt":
		ctrl, err = synth.NewOpenmptPlayer(path, sr)
		if err != nil {
			logger.Info("openmpt unavailable, falling back to gotracker", "err", err)
			ctrl, err = synth.NewTrackerPlayer(path, ext, sr)
		}
	default:
		ctrl, err = synth.NewOpenmptPlayer(path, sr)
		if err != nil {
			logger.Info("openmpt unavailable, falling back to gotracker", "err", err)
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
	p.synthCtrl.Play()
	speaker.Unlock()

	p.isPlaying = true
	p.isPaused = false
	return nil
}