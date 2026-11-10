package midi

// Accepted reset vectors follow the GM Universal Non-Real-Time System On,
// Yamaha XG Parameter Change (model 4C) and Roland GS DT1 (model 42) formats.
// Only GS DT1 uses the Roland address/data checksum. Device 0 is emulated.
func (p *port) sysex(e Event, d *[]Diagnostic) {
	b := e.Payload
	supported := false
	if len(b) == 5 && b[0] == 0x7E && (b[1] == 0 || b[1] == 0x7F) && b[2] == 9 && b[4] == 0xF7 {
		if b[3] == 1 {
			p.reset(modeGM)
			supported = true
		}
		if b[3] == 3 {
			p.reset(modeGM2)
			supported = true
		}
	}
	if len(b) == 8 && b[0] == 0x43 && b[1] == 0x10 && b[2] == 0x4C && b[7] == 0xF7 {
		if b[3] == 0 && b[4] == 0 && b[5] == 0x7E && b[6] == 0 {
			p.reset(modeXG)
			supported = true
		}
		if b[3] == 8 && b[4] < 16 && b[5] == 7 && b[6] <= 3 {
			ch := b[4]
			p.percussion[ch] = b[6] != 0
			bank, drum := p.bank(ch)
			p.synth.SelectPreset(int32(ch), bank, int32(p.program[ch]), drum)
			supported = true
		}
	}
	if len(b) == 10 && b[0] == 0x41 && b[1] == 0x10 && b[2] == 0x42 && b[3] == 0x12 && b[9] == 0xF7 {
		sum := int(b[4]) + int(b[5]) + int(b[6]) + int(b[7]) + int(b[8])
		if sum&127 == 0 {
			if b[4] == 0x40 && b[5] == 0 && b[6] == 0x7F && b[7] == 0 {
				p.reset(modeGS)
				supported = true
			}
			if b[4] == 0x40 && b[5]&0xF0 == 0x10 && b[6] == 0x15 && b[7] <= 2 {
				part := b[5] & 15
				ch := part
				if part == 0 {
					ch = 9
				} else if part <= 9 {
					ch = part - 1
				}
				p.percussion[ch] = b[7] != 0
				bank, drum := p.bank(ch)
				p.synth.SelectPreset(int32(ch), bank, int32(p.program[ch]), drum)
				supported = true
			}
		}
	}
	if !supported {
		addDiagnostic(d, "unsupported_sysex", "unrecognized, invalid or differently addressed SysEx", e.Port, e.Track)
	}
}
