package midi

import (
	"fmt"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/soundfont"
)

type synthMode uint8

const (
	modeSF2 synthMode = iota
	modeGM
	modeGM2
	modeGS
	modeXG
)

type port struct {
	synth             *soundfont.Synthesizer
	mode              synthMode
	msb, lsb, program [16]uint8
	percussion        [16]bool
}

func newPort(s *soundfont.Synthesizer) *port { p := &port{synth: s}; p.percussion[9] = true; return p }
func (p *port) bank(ch uint8) (int32, bool) {
	msb, lsb := p.msb[ch], p.lsb[ch]
	drum := p.percussion[ch]
	switch p.mode {
	case modeGM:
		if drum {
			return 128, true
		}
		return 0, false
	case modeXG:
		if msb == 126 || msb == 127 {
			return 128, true
		}
		if drum {
			return 128, true
		}
		if msb == 64 {
			return 64, false
		}
		if msb == 0 {
			return int32(lsb), false
		}
	case modeGM2:
		if msb == 120 {
			return 128, true
		}
		if drum {
			return 128, true
		}
		if msb == 121 {
			return int32(lsb), false
		}
	}
	if drum {
		return 128, true
	}
	return int32(msb), false
}
func (p *port) process(e Event, d *[]Diagnostic) {
	if e.Kind == SysExEvent {
		p.sysex(e, d)
		return
	}
	ch, cmd := e.Status&15, e.Status&240
	switch cmd {
	case 0xB0:
		p.synth.SetRawController(ch, e.Data1, e.Data2)
		if e.Data1 == 0 {
			p.msb[ch] = e.Data2
			return
		}
		if e.Data1 == 32 {
			p.lsb[ch] = e.Data2
			return
		}
		if e.Data1 == 98 || e.Data1 == 99 {
			addDiagnostic(d, "unsupported_nrpn", "manufacturer NRPN extension", e.Port, e.Track)
		}
	case 0xC0:
		p.program[ch] = e.Data1
		b, drum := p.bank(ch)
		p.synth.SelectPreset(int32(ch), b, int32(e.Data1), drum)
		if d != nil && !p.synth.HasPreset(b, int32(e.Data1)) {
			addDiagnostic(d, "missing_preset", fmt.Sprintf("bank %d program %d; safe family fallback", b, e.Data1), e.Port, 0)
		}
		return
	}
	p.synth.ProcessMidiMessage(int32(ch), int32(cmd), int32(e.Data1), int32(e.Data2))
}
func (p *port) reset(mode synthMode) {
	p.synth.Reset()
	p.mode = mode
	p.msb = [16]uint8{}
	p.lsb = [16]uint8{}
	p.program = [16]uint8{}
	p.percussion = [16]bool{}
	p.percussion[9] = true
	for ch := range 16 {
		bank, drum := p.bank(uint8(ch))
		p.synth.SelectPreset(int32(ch), bank, 0, drum)
	}
}
