package soundfont

import "errors"

const (
	synthDefaultBlockSize             int32 = 64
	synthDefaultMaximumPolyphony      int32 = 64
	synthDefaultEnableReverbAndChorus bool  = true
)

// SynthesizerSettings bounds sample rate, DSP block size, voice count and effects.
type SynthesizerSettings struct {
	SampleRate            int32
	BlockSize             int32
	MaximumPolyphony      int32
	EnableReverbAndChorus bool
}

// NewSynthesizerSettings returns the standard 64-frame settings.
func NewSynthesizerSettings(sampleRate int32) *SynthesizerSettings {
	result := new(SynthesizerSettings)

	result.SampleRate = sampleRate
	result.BlockSize = synthDefaultBlockSize
	result.MaximumPolyphony = synthDefaultMaximumPolyphony
	result.EnableReverbAndChorus = synthDefaultEnableReverbAndChorus

	return result
}

func (settings *SynthesizerSettings) validate() error {
	if 16000 > settings.SampleRate || settings.SampleRate > 192000 {
		return errors.New("the sample rate must be between 16000 and 192000")
	}

	if 8 > settings.BlockSize || settings.BlockSize > 1024 {
		return errors.New("the block size must be between 8 and 1024")
	}

	if 8 > settings.MaximumPolyphony || settings.MaximumPolyphony > 256 {
		return errors.New("the maximum number of polyphony must be between 8 and 256")
	}

	return nil
}
