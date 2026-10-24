package soundfont

import (
	"errors"
	"math"
)

const (
	synthChannelCount      int32 = 16
	synthPercussionChannel int32 = 9
)

// Synthesizer owns mutable voices, channels and effects for shared SoundFonts. It has one caller at a time.
type Synthesizer struct {
	SoundFont             []*SoundFont
	SampleRate            int32
	BlockSize             int32
	MaximumPolyphony      int32
	EnableReverbAndChorus bool

	minimumVoiceDuration int32

	presetLookup map[int32]*Preset
	presetFonts  map[*Preset]int

	channels []*channel

	voices *voiceCollection

	blockLeft  []float32
	blockRight []float32

	inverseBlockSize float32

	blockRead int32

	MasterVolume float32

	reverb            *reverb
	reverbInput       []float32
	reverbOutputLeft  []float32
	reverbOutputRight []float32

	chorus            *chorus
	chorusInputLeft   []float32
	chorusInputRight  []float32
	chorusOutputLeft  []float32
	chorusOutputRight []float32
}

// NewSynthesizer creates private rendering state without changing its SoundFonts.
func NewSynthesizer(sf []*SoundFont, settings *SynthesizerSettings) (*Synthesizer, error) {
	if settings == nil || len(sf) == 0 {
		return nil, errors.New("missing SoundFont or settings")
	}
	for _, font := range sf {
		if font == nil {
			return nil, errors.New("nil SoundFont")
		}
	}
	err := settings.validate()
	if err != nil {
		return nil, err
	}

	result := new(Synthesizer)

	result.SoundFont = sf
	result.SampleRate = settings.SampleRate
	result.BlockSize = settings.BlockSize
	result.MaximumPolyphony = settings.MaximumPolyphony
	result.EnableReverbAndChorus = settings.EnableReverbAndChorus

	result.minimumVoiceDuration = settings.SampleRate / 500

	result.presetLookup = make(map[int32]*Preset)
	result.presetFonts = make(map[*Preset]int)
	for i := 0; i < len(sf); i++ {
		for j := 0; j < len(sf[i].Presets); j++ {
			preset := sf[i].Presets[j]
			result.presetFonts[preset] = i
			// The preset ID is Int32, where the upper 16 bits represent the bank number
			// and the lower 16 bits represent the patch number.
			// This ID is used to search for presets by the combination of bank number
			// and patch number.
			presetID := (preset.BankNumber << 16) | preset.PatchNumber
			result.presetLookup[presetID] = preset

		}
	}

	result.channels = make([]*channel, synthChannelCount)
	for i := int32(0); int(i) < len(result.channels); i++ {
		result.channels[i] = newChannel(result, i == synthPercussionChannel)
	}

	result.voices = newVoiceCollection(result, result.MaximumPolyphony)

	result.blockLeft = make([]float32, result.BlockSize)
	result.blockRight = make([]float32, result.BlockSize)

	result.inverseBlockSize = 1 / float32(result.BlockSize)

	result.blockRead = result.BlockSize

	result.MasterVolume = 0.5

	if settings.EnableReverbAndChorus {
		result.reverb = newReverb(settings.SampleRate)
		result.reverbInput = make([]float32, result.BlockSize)
		result.reverbOutputLeft = make([]float32, result.BlockSize)
		result.reverbOutputRight = make([]float32, result.BlockSize)

		result.chorus = newChorus(settings.SampleRate, 0.002, 0.0019, 0.4)
		result.chorusInputLeft = make([]float32, result.BlockSize)
		result.chorusInputRight = make([]float32, result.BlockSize)
		result.chorusOutputLeft = make([]float32, result.BlockSize)
		result.chorusOutputRight = make([]float32, result.BlockSize)
	}

	return result, nil
}

// ProcessMidiMessage applies a validated MIDI channel message.
func (s *Synthesizer) ProcessMidiMessage(channel int32, command int32, data1 int32, data2 int32) {
	if 0 > channel || int(channel) >= len(s.channels) {
		return
	}

	if data1 < 0 || data1 > 127 || data2 < 0 || data2 > 127 {
		return
	}
	channelInfo := s.channels[channel]

	switch command {
	case 0x80: // Note Off
		s.NoteOff(channel, data1)

	case 0x90: // Note On
		s.NoteOn(channel, data1, data2)

	case 0xA0: // Polyphonic pressure
		channelInfo.polyPressure[data1] = uint8(data2)
	case 0xD0: // Channel pressure
		channelInfo.pressure = uint8(data1)
	case 0xB0: // Controller
		channelInfo.cc[data1] = uint8(data2)
		switch data1 {
		case 0x00: // Bank Selection
			channelInfo.setBank(data2)

		case 0x01: // Modulation Coarse
			channelInfo.setModulationCoarse(data2)

		case 0x21: // Modulation Fine
			channelInfo.setModulationFine(data2)

		case 0x06: // Data Entry Coarse
			channelInfo.dataEntryCoarse(data2)

		case 0x26: // Data Entry Fine
			channelInfo.dataEntryFine(data2)

		case 0x07: // Channel Volume Coarse
			channelInfo.setVolumeCoarse(data2)

		case 0x27: // Channel Volume Fine
			channelInfo.setVolumeFine(data2)

		case 0x0A: // Pan Coarse
			channelInfo.setPanCoarse(data2)

		case 0x2A: // Pan Fine
			channelInfo.setPanFine(data2)

		case 0x0B: // Expression Coarse
			channelInfo.setExpressionCoarse(data2)

		case 0x2B: // Expression Fine
			channelInfo.setExpressionFine(data2)

		case 0x40: // Hold Pedal
			channelInfo.setHoldPedal(data2)

		case 0x5B: // Reverb Send
			channelInfo.setReverbSend(data2)

		case 0x5D: // Chorus Send
			channelInfo.setChorusSend(data2)

		case 0x63: // NRPN Coarse
			channelInfo.setNrpnCoarse(data2)

		case 0x62: // NRPN Fine
			channelInfo.setNrpnFine(data2)

		case 0x65: // RPN Coarse
			channelInfo.setRpnCoarse(data2)

		case 0x64: // RPN Fine
			channelInfo.setRpnFine(data2)

		case 0x78: // All Sound Off
			s.NoteOffAllChannel(channel, true)

		case 0x79: // Reset All Controllers
			s.ResetAllControllersChannel(channel)

		case 0x7B: // All Note Off
			s.NoteOffAllChannel(channel, false)
		}

	case 0xC0: // Program Change
		channelInfo.setPatch(data1)

	case 0xE0: // Pitch Bend
		channelInfo.setPitchBend(data1, data2)
	}
}

// NoteOff releases every voice for this channel and key.
func (s *Synthesizer) NoteOff(channel int32, key int32) {
	if 0 > channel || int(channel) >= len(s.channels) {
		return
	}

	for i := int32(0); i < s.voices.activeVoiceCount; i++ {
		voice := s.voices.voices[i]
		if voice.channel == channel && voice.key == key {
			voice.end()
		}
	}
}

// NoteOn starts all matching regions, or releases the key for zero velocity.
func (s *Synthesizer) NoteOn(channel int32, key int32, velocity int32) {
	if key < 0 || key > 127 || velocity < 0 || velocity > 127 {
		return
	}
	if velocity == 0 {
		s.NoteOff(channel, key)
		return
	}

	if 0 > channel || int(channel) >= len(s.channels) {
		return
	}

	channelInfo := s.channels[channel]
	presetID := (channelInfo.bankNumber << 16) | channelInfo.patchNumber

	preset, found := s.presetLookup[presetID]
	if !found {
		// Only fall back within the melodic or percussion family.
		gmPresetID := channelInfo.patchNumber
		if channelInfo.isPercussionChannel {
			gmPresetID |= 128 << 16
		}

		preset, found = s.presetLookup[gmPresetID]
		if !found {
			if channelInfo.isPercussionChannel {
				preset, found = s.presetLookup[128<<16]
			} else {
				preset, found = s.presetLookup[0]
			}
			if !found {
				return
			}
		}
	}

	presetCount := len(preset.Regions)
	for i := 0; i < presetCount; i++ {
		presetRegion := preset.Regions[i]
		if presetRegion.contains(key, velocity) {
			instrumentCount := len(presetRegion.Instrument.Regions)
			for j := 0; j < instrumentCount; j++ {
				instrumentRegion := presetRegion.Instrument.Regions[j]
				if instrumentRegion.contains(key, velocity) {
					regionPair := newRegionPair(presetRegion, instrumentRegion)

					voice := s.voices.requestNew(instrumentRegion, channel)
					if voice != nil {
						voice.start(regionPair, channel, key, velocity, s.presetFonts[preset])
					}
				}
			}
		}
	}
}

// NoteOffAll releases all notes, or clears voices immediately when requested.
func (s *Synthesizer) NoteOffAll(immediate bool) {
	if immediate {
		s.voices.clear()
	} else {
		for i := 0; i < int(s.voices.activeVoiceCount); i++ {
			s.voices.voices[i].end()
		}
	}
}

// NoteOffAllChannel releases or kills only the selected channel.
func (s *Synthesizer) NoteOffAllChannel(channel int32, immediate bool) {
	if immediate {
		for i := 0; i < int(s.voices.activeVoiceCount); i++ {
			if s.voices.voices[i].channel == channel {
				s.voices.voices[i].kill()
			}
		}
		return
	}
	for i := 0; i < int(s.voices.activeVoiceCount); i++ {
		if s.voices.voices[i].channel == channel {
			s.voices.voices[i].end()
		}
	}
}

// ResetAllControllers resets controller state while preserving bank and program.
func (s *Synthesizer) ResetAllControllers() {
	channelCount := len(s.channels)
	for i := 0; i < channelCount; i++ {
		s.channels[i].resetAllControllers()
	}
}

// ResetAllControllersChannel resets only the selected channel controllers.
func (s *Synthesizer) ResetAllControllersChannel(channel int32) {
	if 0 > channel || int(channel) >= len(s.channels) {
		return
	}

	s.channels[channel].resetAllControllers()
}

// Reset clears voices, effects, controller state and pending DSP output.
func (s *Synthesizer) Reset() {
	s.voices.clear()

	channelCount := len(s.channels)
	for i := 0; i < channelCount; i++ {
		s.channels[i].reset()
	}

	if s.EnableReverbAndChorus {
		s.reverb.mute()
		s.chorus.mute()
	}

	s.blockRead = s.BlockSize
}

// Render writes stereo PCM; left and right must have equal length.
func (s *Synthesizer) Render(left []float32, right []float32) {
	wrote := 0
	length := min(len(left), len(right))
	for wrote < length {
		if s.blockRead == s.BlockSize {
			s.renderBlock()
			s.blockRead = 0
		}

		rem := min(int(s.BlockSize-s.blockRead), length-wrote)
		copy(left[wrote:wrote+rem], s.blockLeft[int(s.blockRead):int(s.blockRead)+rem])
		copy(right[wrote:wrote+rem], s.blockRight[int(s.blockRead):int(s.blockRead)+rem])
		// #nosec G115 -- rem is at most the validated DSP block size (1024).
		s.blockRead += int32(rem)
		wrote += rem
	}
}

func (s *Synthesizer) renderBlock() {
	blockSize := int(s.BlockSize)
	s.voices.process()
	activeVoiceCount := int(s.voices.activeVoiceCount)

	for i := 0; i < blockSize; i++ {
		s.blockLeft[i] = 0
		s.blockRight[i] = 0
	}

	for i := 0; i < activeVoiceCount; i++ {
		voice := s.voices.voices[i]
		previousGainLeft := s.MasterVolume * voice.previousMixGainLeft
		currentGainLeft := s.MasterVolume * voice.currentMixGainLeft
		s.writeBlock(previousGainLeft, currentGainLeft, voice.block, s.blockLeft)
		var previousGainRight = s.MasterVolume * voice.previousMixGainRight
		var currentGainRight = s.MasterVolume * voice.currentMixGainRight
		s.writeBlock(previousGainRight, currentGainRight, voice.block, s.blockRight)
	}

	if s.EnableReverbAndChorus {
		for i := 0; i < blockSize; i++ {
			s.chorusInputLeft[i] = 0
		}
		for i := 0; i < blockSize; i++ {
			s.chorusInputRight[i] = 0
		}
		for i := 0; i < activeVoiceCount; i++ {
			voice := s.voices.voices[i]
			previousGainLeft := voice.previousChorusSend * voice.previousMixGainLeft
			currentGainLeft := voice.currentChorusSend * voice.currentMixGainLeft
			s.writeBlock(previousGainLeft, currentGainLeft, voice.block, s.chorusInputLeft)
			previousGainRight := voice.previousChorusSend * voice.previousMixGainRight
			currentGainRight := voice.currentChorusSend * voice.currentMixGainRight
			s.writeBlock(previousGainRight, currentGainRight, voice.block, s.chorusInputRight)
		}
		s.chorus.process(s.chorusInputLeft, s.chorusInputRight, s.chorusOutputLeft, s.chorusOutputRight)
		arrayMultiplyAdd(s.MasterVolume, s.chorusOutputLeft, s.blockLeft)
		arrayMultiplyAdd(s.MasterVolume, s.chorusOutputRight, s.blockRight)

		for i := 0; i < blockSize; i++ {
			s.reverbInput[i] = 0
		}
		for i := 0; i < activeVoiceCount; i++ {
			voice := s.voices.voices[i]
			previousGain := s.reverb.getInputGain() * voice.previousReverbSend * (voice.previousMixGainLeft + voice.previousMixGainRight)
			currentGain := s.reverb.getInputGain() * voice.currentReverbSend * (voice.currentMixGainLeft + voice.currentMixGainRight)
			s.writeBlock(previousGain, currentGain, voice.block, s.reverbInput)
		}
		s.reverb.process(s.reverbInput, s.reverbOutputLeft, s.reverbOutputRight)
		arrayMultiplyAdd(s.MasterVolume, s.reverbOutputLeft, s.blockLeft)
		arrayMultiplyAdd(s.MasterVolume, s.reverbOutputRight, s.blockRight)
	}
}

func (s *Synthesizer) writeBlock(previousGain float32, currentGain float32, source []float32, destination []float32) {
	if math.Max(float64(previousGain), float64(currentGain)) < float64(nonAudible) {
		return
	}

	if math.Abs(float64(currentGain-previousGain)) < 1.0e-3 {
		arrayMultiplyAdd(currentGain, source, destination)
	} else {
		step := s.inverseBlockSize * (currentGain - previousGain)
		arrayMultiplyAddSlope(previousGain, step, source, destination)
	}
}
