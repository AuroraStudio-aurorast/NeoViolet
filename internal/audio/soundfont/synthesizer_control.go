package soundfont

// Stats reports voice use for this synthesizer, read by its rendering owner.
type Stats struct{ ActiveVoices, PeakVoices, StolenVoices int32 }

// Stats returns current and cumulative voice use for the rendering owner.
func (s *Synthesizer) Stats() Stats {
	return Stats{s.voices.activeVoiceCount, s.voices.peak, s.voices.stolen}
}

// ReleaseAll releases held notes and sustain while retaining effects and envelopes.
func (s *Synthesizer) ReleaseAll() {
	for _, c := range s.channels {
		c.setHoldPedal(0)
	}
	s.NoteOffAll(false)
}

// SetRawController records bank selection as a modulator source without changing
// the active preset before its subsequent program change.
func (s *Synthesizer) SetRawController(ch, cc, value uint8) {
	if ch < 16 && cc < 128 && value < 128 {
		s.channels[ch].cc[cc] = value
	}
}

// HasPreset reports whether a bank/program pair is present, including empty presets.
func (s *Synthesizer) HasPreset(bank, program int32) bool {
	_, ok := s.presetLookup[(bank<<16)|program]
	return ok
}

// SelectPreset explicitly selects an SF2 bank without implicit channel-10 mapping.
func (s *Synthesizer) SelectPreset(channel, bank, program int32, percussion bool) {
	if channel < 0 || int(channel) >= len(s.channels) {
		return
	}
	c := s.channels[channel]
	c.bankNumber = bank
	c.patchNumber = program
	c.isPercussionChannel = percussion
}
