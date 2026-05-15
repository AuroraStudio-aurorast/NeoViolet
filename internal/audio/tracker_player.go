package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"os"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gotracker/playback/format"
	"github.com/gotracker/playback/mixing"
	"github.com/gotracker/playback/mixing/sampling"
	"github.com/gotracker/playback/output"
	"github.com/gotracker/playback/player/feature"
	"github.com/gotracker/playback/player/machine"
	"github.com/gotracker/playback/player/machine/settings"
	"github.com/gotracker/playback/player/sampler"
	"github.com/gotracker/playback/song"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

type TrackerPlayer struct {
	mu sync.Mutex

	machine      machine.MachineTicker
	songData     song.Data
	userSettings settings.UserSettings
	sampleRate   beep.SampleRate

	mixer          mixing.Mixer
	sampler        *sampler.Sampler
	receivedPremix chan *output.PremixData

	renderSamples [][2]float64
	renderPos     int

	seekTarget int
	seeking    bool

	duration    time.Duration
	elapsed     time.Duration
	isPlaying   bool
	isPaused    bool
	closed      bool
	finished    bool
	volumeScale float64

	title      string
	artist     string
	songFormat string
}

func NewTrackerPlayer(path, ext string, sampleRate beep.SampleRate) (*TrackerPlayer, error) {
	logger.Info("Creating tracker player", "path", path, "ext", ext, "sampleRate", sampleRate)

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open tracker file: %w", err)
	}
	defer file.Close()

	songFmtKey := formatKeyFromExt(ext)
	feats := []feature.Feature{
		feature.UseNativeSampleFormat(true),
		feature.IgnoreUnknownEffect{Enabled: true},
		feature.SongLoop{Count: 0},
	}
	songData, songFmt, err := format.LoadFromReader(songFmtKey, file, feats)
	if err != nil {
		return nil, fmt.Errorf("load tracker module: %w", err)
	}

	var us settings.UserSettings
	songFmt.ConvertFeaturesToSettings(&us, feats)

	mach, err := machine.NewMachine(songData, us)
	if err != nil {
		return nil, fmt.Errorf("create tracker machine: %w", err)
	}

	tp := &TrackerPlayer{
		songData:       songData,
		userSettings:   us,
		sampleRate:     sampleRate,
		mixer:          mixing.Mixer{Channels: 2},
		receivedPremix: make(chan *output.PremixData, 1),
		isPaused:       true,
		volumeScale:    1.0,
	}

	name := songData.GetName()
	if name != "" {
		tp.title = name
	}
	tp.artist = formatDisplayName(songFmtKey)
	tp.songFormat = songFmtKey

	out := sampler.NewSampler(int(sampleRate), 2, 1.0, func(premix *output.PremixData) {
		tp.receivedPremix <- premix
	})
	tp.sampler = out

	dryMachine, _ := machine.NewMachine(songData, us)
	tp.duration = computeDuration(dryMachine, sampleRate)

	tp.machine = mach
	return tp, nil
}

func (p *TrackerPlayer) Stream(samples [][2]float64) (n int, ok bool) {
	p.mu.Lock()

	if p.closed || p.finished {
		p.mu.Unlock()
		return 0, false
	}
	if p.isPaused && !p.seeking {
		p.mu.Unlock()
		for i := range samples {
			samples[i] = [2]float64{}
		}
		return len(samples), true
	}

	vs := p.volumeScale
	elapsed := p.elapsed
	duration := p.duration

	if p.seeking {
		p.handleSeek(samples, vs, &elapsed, duration)
		return len(samples), true
	}

	p.mu.Unlock()

	p.fillSamples(samples, vs, &elapsed, duration)

	p.mu.Lock()
	p.elapsed = elapsed
	p.mu.Unlock()

	return len(samples), true
}

func (p *TrackerPlayer) handleSeek(samples [][2]float64, vs float64, elapsed *time.Duration, duration time.Duration) {
	target := p.seekTarget
	p.seeking = false
	p.mu.Unlock()

	newMach, err := machine.NewMachine(p.songData, p.userSettings)
	if err != nil {
		p.mu.Lock()
		p.finished = true
		p.mu.Unlock()
		return
	}

	newOut := sampler.NewSampler(int(p.sampleRate), 2, 1.0, func(premix *output.PremixData) {
		p.receivedPremix <- premix
	})

	rendered := 0
	for rendered < target {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return
		}
		if p.seeking {
			target = p.seekTarget
			p.seeking = false
			p.mu.Unlock()
			var err error
			newMach, err = machine.NewMachine(p.songData, p.userSettings)
			if err != nil {
				p.mu.Lock()
				p.finished = true
				p.mu.Unlock()
				return
			}
			rendered = 0
			continue
		}
		p.mu.Unlock()

		err := newMach.Tick(newOut)
		if errors.Is(err, song.ErrStopSong) {
			break
		}
		premix := <-p.receivedPremix
		rendered += premix.SamplesLen
	}

	remainingBeforeTarget := rendered - target
	if remainingBeforeTarget > 0 {
		*elapsed = time.Duration(target) * time.Second / time.Duration(p.sampleRate)
	} else {
		*elapsed = time.Duration(rendered) * time.Second / time.Duration(p.sampleRate)
	}

	p.mu.Lock()
	p.machine = newMach
	p.sampler = newOut
	p.renderSamples = nil
	p.renderPos = 0
	if p.seeking || p.isPaused {
		p.mu.Unlock()
		for i := range samples {
			samples[i] = [2]float64{}
		}
		return
	}
	p.mu.Unlock()

	p.fillSamples(samples, vs, elapsed, duration)

	p.mu.Lock()
	p.elapsed = *elapsed
	p.mu.Unlock()
}

func (p *TrackerPlayer) fillSamples(samples [][2]float64, vs float64, elapsed *time.Duration, duration time.Duration) {
	for i := range samples {
		if *elapsed >= duration {
			p.mu.Lock()
			p.finished = true
			p.isPlaying = false
			p.mu.Unlock()
			for j := i; j < len(samples); j++ {
				samples[j] = [2]float64{}
			}
			return
		}

		for p.renderPos >= len(p.renderSamples) {
			p.renderOneTick()
			if p.finished {
				for j := i; j < len(samples); j++ {
					samples[j] = [2]float64{}
				}
				return
			}
		}

		s := p.renderSamples[p.renderPos]
		p.renderPos++

		samples[i][0] = softClip(s[0] * vs)
		samples[i][1] = softClip(s[1] * vs)
		*elapsed += time.Second / time.Duration(p.sampleRate)
	}
}

func (p *TrackerPlayer) renderOneTick() {
	err := p.machine.Tick(p.sampler)
	if errors.Is(err, song.ErrStopSong) {
		p.finished = true
		p.isPlaying = false
		return
	}

	premix := <-p.receivedPremix
	data := p.mixer.Flatten(premix.SamplesLen, premix.Data, premix.MixerVolume, sampling.Format16BitLESigned)

	sampleCount := len(data) / 4
	p.renderSamples = make([][2]float64, sampleCount)
	for j := 0; j < sampleCount; j++ {
		off := j * 4
		p.renderSamples[j][0] = float64(int16(binary.LittleEndian.Uint16(data[off:off+2]))) / 32768.0
		p.renderSamples[j][1] = float64(int16(binary.LittleEndian.Uint16(data[off+2:off+4]))) / 32768.0
	}
	p.renderPos = 0
}

func (p *TrackerPlayer) Err() error { return nil }

func (p *TrackerPlayer) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	logger.Debug("Tracker player play")
	p.isPaused = false
	p.isPlaying = true
	p.finished = false
	return nil
}

func (p *TrackerPlayer) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	logger.Debug("Tracker player pause")
	p.isPaused = true
}

func (p *TrackerPlayer) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	logger.Debug("Tracker player stop")
	p.isPaused = true
	p.isPlaying = false
	p.finished = false
	p.seeking = false
	p.elapsed = 0
	p.renderSamples = nil
	p.renderPos = 0
}

func (p *TrackerPlayer) Toggle() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isPaused || !p.isPlaying {
		p.isPaused = false
		p.isPlaying = true
		p.finished = false
	} else {
		p.isPaused = true
	}
}

func (p *TrackerPlayer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	logger.Debug("Tracker player close")
	p.closed = true
	p.seeking = false
	return nil
}

func (p *TrackerPlayer) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isPlaying && !p.isPaused
}

func (p *TrackerPlayer) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.elapsed
}

func (p *TrackerPlayer) Duration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.duration
}

func (p *TrackerPlayer) SetVolume(vol float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volumeScale = clamp(vol, 0, 1)
}

func (p *TrackerPlayer) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.volumeScale
}

func (p *TrackerPlayer) Format() beep.Format {
	return beep.Format{SampleRate: p.sampleRate, NumChannels: 2, Precision: 4}
}

func (p *TrackerPlayer) Streamer() SynthStreamer { return p }

func (p *TrackerPlayer) Title() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.title
}

func (p *TrackerPlayer) Artist() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.artist
}

func (p *TrackerPlayer) CoverImage() image.Image { return nil }

func (p *TrackerPlayer) Seek(pos time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	targetSample := int(pos.Seconds() * float64(p.sampleRate))
	if targetSample < 0 {
		targetSample = 0
	}

	logger.Debug("Tracker seek", "position", pos, "targetSample", targetSample)

	p.seekTarget = targetSample
	p.seeking = true
	p.elapsed = pos
	p.finished = false
	return nil
}

func (p *TrackerPlayer) Open(path string) error {
	return fmt.Errorf("tracker player does not support Open, use NewTrackerPlayer")
}

func formatKeyFromExt(ext string) string {
	switch ext {
	case ".mod":
		return "mod"
	case ".xm":
		return "xm"
	case ".s3m":
		return "s3m"
	case ".it", ".mptm":
		return "it"
	default:
		return ext[1:]
	}
}

func formatDisplayName(key string) string {
	switch key {
	case "mod":
		return "MOD"
	case "xm":
		return "XM"
	case "s3m":
		return "S3M"
	case "it":
		return "IT"
	default:
		return key
	}
}

func computeDuration(mach machine.MachineTicker, sampleRate beep.SampleRate) time.Duration {
	ch := make(chan *output.PremixData, 1)
	tempOut := sampler.NewSampler(int(sampleRate), 2, 1.0, func(premix *output.PremixData) {
		ch <- premix
	})

	totalSamples := 0
	for {
		err := mach.Tick(tempOut)
		if errors.Is(err, song.ErrStopSong) {
			break
		}
		premix := <-ch
		totalSamples += premix.SamplesLen
	}
	if totalSamples == 0 {
		return 0
	}
	return time.Duration(totalSamples) * time.Second / time.Duration(sampleRate)
}
