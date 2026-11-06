// Package miditest creates synthetic MIDI and SoundFont fixtures for tests.
// Its waveforms and metadata are generated here; no third-party audio is used.
package miditest

import (
	"bytes"
	"encoding/binary"
	"math"
)

// Generator is an SF2 generator record, including signed values encoded as uint16.
type Generator struct{ Operator, Amount uint16 }

// Modulator is an SF2 modulation record with a signed amount.
type Modulator struct {
	Source, Destination     uint16
	Amount                  int16
	AmountSource, Transform uint16
}

// SF2Options describes a small single-sample SoundFont.
type SF2Options struct {
	Samples            []int16
	Instruments        [][]Generator
	InstrumentZones    [][][]Generator
	InstrumentMods     [][]Modulator
	PresetInstruments  []uint16
	LoopStart, LoopEnd uint32
	SampleModes        uint16
}

// SF2 creates a complete RIFF SoundFont with one preset per instrument reference.
// Nil options produce a looping sine instrument suitable for PCM comparisons.
func SF2(opts *SF2Options) []byte {
	o := SF2Options{}
	if opts != nil {
		o = *opts
	}
	if o.Samples == nil {
		o.Samples = make([]int16, 1024)
		for i := range o.Samples {
			o.Samples[i] = int16(12000 * math.Sin(2*math.Pi*float64(i)/64))
		}
	}
	if opts == nil {
		o.SampleModes = 1
		// #nosec G115 -- synthetic test fixture lengths and indices are bounded by in-memory test inputs, not file declarations.
		o.LoopEnd = uint32(len(o.Samples))
	}
	if o.Instruments == nil {
		o.Instruments = [][]Generator{{{54, o.SampleModes}, {53, 0}}}
	}
	if o.InstrumentZones == nil {
		for _, z := range o.Instruments {
			if len(z) == 0 {
				o.InstrumentZones = append(o.InstrumentZones, nil)
			} else {
				o.InstrumentZones = append(o.InstrumentZones, [][]Generator{z})
			}
		}
	}
	if o.PresetInstruments == nil {
		o.PresetInstruments = []uint16{0}
	}
	info := Chunk("LIST", append([]byte("INFO"), bytes.Join([][]byte{
		Chunk("ifil", words(2, 1)), Chunk("isng", []byte("EMU8000\x00")),
		Chunk("INAM", []byte("Generated test bank\x00")),
	}, nil)...))
	pcm := new(bytes.Buffer)
	for _, v := range o.Samples {
		put(pcm, v)
	}
	for range 46 {
		put(pcm, int16(0))
	}
	sdta := Chunk("LIST", append([]byte("sdta"), Chunk("smpl", pcm.Bytes())...))
	phdr, pbag, pgen := new(bytes.Buffer), new(bytes.Buffer), new(bytes.Buffer)
	for i, inst := range o.PresetInstruments {
		phdr.Write(name("Test preset", 20))
		put(phdr, uint16(i))
		put(phdr, uint16(0))
		put(phdr, uint16(i))
		for range 3 {
			put(phdr, uint32(0))
		}
		put(pbag, uint16(i))
		put(pbag, uint16(0))
		put(pgen, uint16(41))
		put(pgen, inst)
	}
	phdr.Write(name("EOP", 20))
	put(phdr, uint16(0))
	put(phdr, uint16(0))
	// #nosec G115 -- synthetic test fixture lengths and indices are bounded by in-memory test inputs, not file declarations.
	put(phdr, uint16(len(o.PresetInstruments)))
	for range 3 {
		put(phdr, uint32(0))
	}
	// #nosec G115 -- synthetic test fixture lengths and indices are bounded by in-memory test inputs, not file declarations.
	put(pbag, uint16(len(o.PresetInstruments)))
	put(pbag, uint16(0))
	put(pgen, uint32(0))
	inst, ibag, igen, imod := new(bytes.Buffer), new(bytes.Buffer), new(bytes.Buffer), new(bytes.Buffer)
	bag, gen, mod := uint16(0), uint16(0), uint16(0)
	for i, zones := range o.InstrumentZones {
		inst.Write(name("Test instrument", 20))
		put(inst, bag)
		for _, region := range zones {
			put(ibag, gen)
			put(ibag, mod)
			bag++
			for _, g := range region {
				put(igen, g.Operator)
				put(igen, g.Amount)
				gen++
			}
			if i < len(o.InstrumentMods) {
				for _, m := range o.InstrumentMods[i] {
					put(imod, m.Source)
					put(imod, m.Destination)
					put(imod, m.Amount)
					put(imod, m.AmountSource)
					put(imod, m.Transform)
					mod++
				}
			}
		}
	}
	inst.Write(name("EOI", 20))
	put(inst, bag)
	put(ibag, gen)
	put(ibag, mod)
	put(igen, uint32(0))
	imod.Write(make([]byte, 10))
	shdr := new(bytes.Buffer)
	shdr.Write(name("Generated sine", 20))
	// #nosec G115 -- synthetic test fixture lengths and indices are bounded by in-memory test inputs, not file declarations.
	for _, v := range []uint32{0, uint32(len(o.Samples)), o.LoopStart, o.LoopEnd, 44100} {
		put(shdr, v)
	}
	shdr.WriteByte(60)
	shdr.WriteByte(0)
	put(shdr, uint16(0))
	put(shdr, uint16(1))
	shdr.Write(name("EOS", 20))
	shdr.Write(make([]byte, 26))
	pdta := Chunk("LIST", append([]byte("pdta"), bytes.Join([][]byte{
		Chunk("phdr", phdr.Bytes()), Chunk("pbag", pbag.Bytes()), Chunk("pmod", make([]byte, 10)), Chunk("pgen", pgen.Bytes()),
		Chunk("inst", inst.Bytes()), Chunk("ibag", ibag.Bytes()), Chunk("imod", imod.Bytes()), Chunk("igen", igen.Bytes()), Chunk("shdr", shdr.Bytes()),
	}, nil)...))
	return Chunk("RIFF", bytes.Join([][]byte{[]byte("sfbk"), info, sdta, pdta}, nil))
}

// Chunk builds a little-endian RIFF chunk with proper even-byte padding.
func Chunk(id string, data []byte) []byte {
	b := new(bytes.Buffer)
	b.WriteString(id)
	// #nosec G115 -- synthetic test fixture lengths and indices are bounded by in-memory test inputs, not file declarations.
	put(b, uint32(len(data)))
	b.Write(data)
	if len(data)%2 != 0 {
		b.WriteByte(0)
	}
	return b.Bytes()
}

func words(v ...uint16) []byte {
	b := new(bytes.Buffer)
	for _, x := range v {
		put(b, x)
	}
	return b.Bytes()
}
func name(s string, n int) []byte { b := make([]byte, n); copy(b, s); return b }
func put(b *bytes.Buffer, v any) {
	if err := binary.Write(b, binary.LittleEndian, v); err != nil {
		panic(err)
	}
}
