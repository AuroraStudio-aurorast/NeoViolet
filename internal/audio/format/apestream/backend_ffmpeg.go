package apestream

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

// ffmpeg pipe backend

// ffmpegBackend decodes APE through ffmpeg, asking it for 16-bit PCM on stdout.
// Process lifetime is handled by pipeOwner.
type ffmpegBackend struct {
	pipeOwner
}

func newFFmpegBackend() *ffmpegBackend {
	b := &ffmpegBackend{}
	b.start = startFFmpegPipe
	return b
}

func (b *ffmpegBackend) Name() string { return "ffmpeg" }

func (b *ffmpegBackend) Open(path string, seekSamples int) (*StreamInfo, error) {
	return b.open(path, seekSamples)
}

func (b *ffmpegBackend) Read(p []float64) (int, error) { return b.read(p) }

func (b *ffmpegBackend) Seek(samples int) error { return b.seek(samples) }

func (b *ffmpegBackend) Close() error { return b.close() }

// startFFmpegPipe probes the file, launches ffmpeg and returns the running
// generation. ffmpeg writes s16le PCM, so the decoded samples are always
// 16-bit regardless of the file's native depth.
func startFFmpegPipe(path string, seekSamples int) (*pipeStream, *StreamInfo, error) {
	info, err := probeFFmpegMetadata(path)
	if err != nil {
		return nil, nil, fmt.Errorf("ffmpeg probe: %w", err)
	}

	// We request s16le PCM output (ffmpeg's most compatible format) at the
	// file's native sample rate and channel count.
	args := []string{
		"-i", sanitizeFileArg(path),
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ac", strconv.Itoa(info.Channels),
		"-ar", strconv.Itoa(info.SampleRate),
	}
	if seekSamples >= 0 {
		seconds := float64(seekSamples) / float64(info.SampleRate)
		args = append(args, "-ss", fmt.Sprintf("%.6f", seconds))
	}
	args = append(args, "pipe:1")

	// #nosec G204 -- ffmpeg is a fixed binary name; args are a fixed flag set
	// plus a sanitized file path.
	cmd := exec.Command("ffmpeg", args...)
	stdout, err := startPipeCmd("ffmpeg", cmd)
	if err != nil {
		return nil, nil, err
	}

	channels := info.Channels
	if channels == 0 {
		channels = 2
	}

	const bytesPerSample = 2 // pcm_s16le
	return newPipeStream(cmd, stdout, info, bytesPerSample, channels), info, nil
}

// probeFFmpegMetadata runs ffprobe to extract audio properties of the APE file.
func probeFFmpegMetadata(path string) (*StreamInfo, error) {
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"--",
		path,
	}
	// #nosec G204 -- ffprobe is a fixed binary name; args are a fixed flag set
	// plus the user's own audio path (with a "--" separator).
	cmd := exec.Command("ffprobe", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}

	var result struct {
		Streams []struct {
			CodecType     string `json:"codec_type"`
			SampleRate    string `json:"sample_rate"`
			Channels      int    `json:"channels"`
			BitsPerSample int    `json:"bits_per_sample"`
			Duration      string `json:"duration"`
			NbSamples     int64  `json:"nb_samples"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("ffprobe parse: %w", err)
	}

	for _, s := range result.Streams {
		if s.CodecType == "audio" {
			var sampleRate int
			_, _ = fmt.Sscanf(s.SampleRate, "%d", &sampleRate)

			totalSamples := int(s.NbSamples)
			if totalSamples == 0 && s.Duration != "" {
				var duration float64
				_, _ = fmt.Sscanf(s.Duration, "%f", &duration)
				totalSamples = int(duration*float64(sampleRate) + 0.5)
			}

			bps := s.BitsPerSample
			if bps == 0 {
				bps = 16 // ffmpeg always outputs 16-bit PCM
			}

			return &StreamInfo{
				SampleRate:    sampleRate,
				Channels:      s.Channels,
				BitsPerSample: bps,
				TotalSamples:  totalSamples,
			}, nil
		}
	}

	return nil, fmt.Errorf("ffprobe: no audio stream found")
}
