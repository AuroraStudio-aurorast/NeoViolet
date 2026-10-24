package soundfont

import (
	"maps"
	"slices"
	"unsafe"
)

// Clone makes an independent DSP continuation, including partially read output
// blocks and effect delay lines. The owner must exclude concurrent rendering.
// SoundFont data and the chorus waveform table are immutable and remain shared.
func (s *Synthesizer) Clone() *Synthesizer {
	c := copyState(s)
	c.SoundFont = slices.Clone(s.SoundFont)
	c.presetLookup = maps.Clone(s.presetLookup)
	c.presetFonts = maps.Clone(s.presetFonts)
	c.channels = make([]*channel, len(s.channels))
	for i, ch := range s.channels {
		c.channels[i] = copyState(ch)
		c.channels[i].synthesizer = c
	}
	c.voices = copyState(s.voices)
	c.voices.synthesizer = c
	c.voices.voices = make([]*voice, len(s.voices.voices))
	for i, v := range s.voices.voices {
		x := copyState(v)
		x.synthesizer = c
		x.volEnv = copyState(v.volEnv)
		x.volEnv.synthesizer = c
		x.modEnv = copyState(v.modEnv)
		x.modEnv.synthesizer = c
		x.vibLfo = copyState(v.vibLfo)
		x.vibLfo.synthesizer = c
		x.modLfo = copyState(v.modLfo)
		x.modLfo.synthesizer = c
		x.oscillator = copyState(v.oscillator)
		x.oscillator.synthesizer = c
		x.filter = copyState(v.filter)
		x.filter.synthesizer = c
		x.block = slices.Clone(v.block)
		if v.region.values != nil {
			x.region.values = &x.modValues
		}
		c.voices.voices[i] = x
	}
	c.blockLeft = slices.Clone(s.blockLeft)
	c.blockRight = slices.Clone(s.blockRight)
	c.reverbInput = slices.Clone(s.reverbInput)
	c.reverbOutputLeft = slices.Clone(s.reverbOutputLeft)
	c.reverbOutputRight = slices.Clone(s.reverbOutputRight)
	c.chorusInputLeft = slices.Clone(s.chorusInputLeft)
	c.chorusInputRight = slices.Clone(s.chorusInputRight)
	c.chorusOutputLeft = slices.Clone(s.chorusOutputLeft)
	c.chorusOutputRight = slices.Clone(s.chorusOutputRight)
	if s.reverb != nil {
		c.reverb = copyState(s.reverb)
		c.reverb.cfsL = cloneCombs(s.reverb.cfsL)
		c.reverb.cfsR = cloneCombs(s.reverb.cfsR)
		c.reverb.apfsL = cloneAllPasses(s.reverb.apfsL)
		c.reverb.apfsR = cloneAllPasses(s.reverb.apfsR)
	}
	if s.chorus != nil {
		c.chorus = copyState(s.chorus)
		c.chorus.bufferL = slices.Clone(s.chorus.bufferL)
		c.chorus.bufferR = slices.Clone(s.chorus.bufferR)
	}
	return c
}
func copyState[T any](p *T) *T { c := *p; return &c }
func cloneCombs(src []*combFilter) []*combFilter {
	dst := make([]*combFilter, len(src))
	for i, v := range src {
		dst[i] = copyState(v)
		dst[i].buffer = slices.Clone(v.buffer)
	}
	return dst
}
func cloneAllPasses(src []*allPassFilter) []*allPassFilter {
	dst := make([]*allPassFilter, len(src))
	for i, v := range src {
		dst[i] = copyState(v)
		dst[i].buffer = slices.Clone(v.buffer)
	}
	return dst
}

// StateBytes conservatively estimates a Clone's retained allocations. Shared
// samples, presets and waveform tables are excluded; allocator/GC metadata and
// working renderers are outside the checkpoint cache budget.
func (s *Synthesizer) StateBytes() int64 {
	n := allocationBytes(unsafe.Sizeof(*s))
	n += allocationBytes(uintptr(len(s.SoundFont)) * unsafe.Sizeof((*SoundFont)(nil)))
	// Maps have implementation-dependent bucket storage; budget generously.
	n += int64(256 + 128*(len(s.presetLookup)+len(s.presetFonts)))
	n += allocationBytes(uintptr(len(s.channels)) * unsafe.Sizeof((*channel)(nil)))
	for _, ch := range s.channels {
		n += allocationBytes(unsafe.Sizeof(*ch))
	}
	n += allocationBytes(unsafe.Sizeof(*s.voices)) + allocationBytes(uintptr(len(s.voices.voices))*unsafe.Sizeof((*voice)(nil)))
	for _, v := range s.voices.voices {
		n += allocationBytes(unsafe.Sizeof(*v)) + allocationBytes(unsafe.Sizeof(*v.volEnv)) + allocationBytes(unsafe.Sizeof(*v.modEnv))
		n += 2*allocationBytes(unsafe.Sizeof(*v.vibLfo)) + allocationBytes(unsafe.Sizeof(*v.oscillator)) + allocationBytes(unsafe.Sizeof(*v.filter)) + floatBufferBytes(v.block)
	}
	for _, b := range [][]float32{s.blockLeft, s.blockRight, s.reverbInput, s.reverbOutputLeft, s.reverbOutputRight, s.chorusInputLeft, s.chorusInputRight, s.chorusOutputLeft, s.chorusOutputRight} {
		n += floatBufferBytes(b)
	}
	if s.reverb != nil {
		n += allocationBytes(unsafe.Sizeof(*s.reverb))
		for _, list := range [][]*combFilter{s.reverb.cfsL, s.reverb.cfsR} {
			n += allocationBytes(uintptr(len(list)) * unsafe.Sizeof((*combFilter)(nil)))
			for _, v := range list {
				n += allocationBytes(unsafe.Sizeof(*v)) + floatBufferBytes(v.buffer)
			}
		}
		for _, list := range [][]*allPassFilter{s.reverb.apfsL, s.reverb.apfsR} {
			n += allocationBytes(uintptr(len(list)) * unsafe.Sizeof((*allPassFilter)(nil)))
			for _, v := range list {
				n += allocationBytes(unsafe.Sizeof(*v)) + floatBufferBytes(v.buffer)
			}
		}
	}
	if s.chorus != nil {
		n += allocationBytes(unsafe.Sizeof(*s.chorus)) + floatBufferBytes(s.chorus.bufferL) + floatBufferBytes(s.chorus.bufferR)
	}
	return n
}
func floatBufferBytes(b []float32) int64 { return allocationBytes(uintptr(len(b)) * 4) }
func allocationBytes(n uintptr) int64 {
	if n == 0 {
		return 0
	}
	// #nosec G115 -- sizes come from bounded DSP buffers/structs, never the PCM sample array.
	return int64((n+31)/32*32 + 32)
}
