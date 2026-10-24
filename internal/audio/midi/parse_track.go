package midi

import (
	"errors"
	"io"
	"math"
)

type trackParser struct {
	data    []byte
	pos     int
	running byte
}

func (p *trackParser) byte() (byte, error) {
	if p.pos >= len(p.data) {
		return 0, io.ErrUnexpectedEOF
	}
	b := p.data[p.pos]
	p.pos++
	return b, nil
}
func (p *trackParser) vlq() (uint32, error) {
	var n uint32
	for range 4 {
		b, e := p.byte()
		if e != nil {
			return 0, e
		}
		n = n<<7 | uint32(b&127)
		if b < 128 {
			return n, nil
		}
	}
	return 0, errors.New("VLQ exceeds four bytes")
}
func (p *trackParser) readStatus() (byte, error) {
	b, e := p.byte()
	if e != nil {
		return 0, e
	}
	if b < 128 {
		if p.running < 128 || p.running >= 240 {
			return 0, errors.New("invalid running status")
		}
		p.pos--
		return p.running, nil
	}
	if b < 240 {
		p.running = b
	} else if b < 248 || b == 255 {
		p.running = 0
	}
	return b, nil
}
func parseTrack(data []byte, track uint32, limits parseLimits) (events []Event, tempos []tempoEvent, tick uint64, diags []Diagnostic, err error) {
	p := trackParser{data: data}
	port := uint8(0)
	var fragment []byte
	var fragmentPort uint8
	fail := func(e error) ([]Event, []tempoEvent, uint64, []Diagnostic, error) {
		return events, tempos, tick, diags, e
	}
	for order := uint32(0); p.pos < len(data); order++ {
		delta, e := p.vlq()
		if e != nil {
			return fail(e)
		}
		if tick > math.MaxUint64-uint64(delta) {
			return fail(errors.New("tick overflow"))
		}
		tick += uint64(delta)
		status, e := p.readStatus()
		if e != nil {
			return fail(e)
		}
		switch {
		case status < 240:
			d1, e := p.byte()
			if e != nil {
				return fail(e)
			}
			d2 := byte(0)
			if status&240 != 192 && status&240 != 208 {
				d2, e = p.byte()
				if e != nil {
					return fail(e)
				}
			}
			if d1 >= 128 || d2 >= 128 {
				return fail(errors.New("status byte in channel data"))
			}
			events = append(events, Event{Tick: tick, Track: track, Order: order, Port: port, Status: status, Data1: d1, Data2: d2})
		case status == 255:
			typ, e := p.byte()
			if e != nil {
				return fail(e)
			}
			n, e := p.vlq()
			if e != nil {
				return fail(e)
			}
			if int64(n) > int64(len(data)-p.pos) {
				return fail(io.ErrUnexpectedEOF)
			}
			v := data[p.pos : p.pos+int(n)]
			p.pos += int(n)
			switch typ {
			case 47:
				if n != 0 || p.pos != len(data) {
					return fail(errors.New("invalid EOT boundary"))
				}
				if len(fragment) > 0 {
					addDiagnostic(&diags, "incomplete_sysex", "incomplete SysEx ignored", fragmentPort, track)
				}
				return events, tempos, tick, diags, nil
			case 33:
				if n != 1 {
					return fail(errors.New("invalid MIDI port meta"))
				}
				port = v[0]
			case 81:
				if n != 3 {
					return fail(errors.New("invalid tempo meta"))
				}
				tempo := uint32(v[0])<<16 | uint32(v[1])<<8 | uint32(v[2])
				if tempo == 0 {
					return fail(errors.New("zero tempo"))
				}
				tempos = append(tempos, tempoEvent{tick, track, order, tempo})
			}
		case status == 240 || status == 247:
			n, e := p.vlq()
			if e != nil {
				return fail(e)
			}
			if int64(n) > int64(limits.sysex) || len(fragment)+int(n) > limits.sysex {
				return fail(errors.New("SysEx resource limit exceeded"))
			}
			if int64(n) > int64(len(data)-p.pos) {
				return fail(io.ErrUnexpectedEOF)
			}
			v := data[p.pos : p.pos+int(n)]
			p.pos += int(n)
			if status == 240 {
				if len(fragment) > 0 {
					addDiagnostic(&diags, "incomplete_sysex", "incomplete SysEx ignored", fragmentPort, track)
				}
				fragment = nil
				fragmentPort = port
			} else if len(fragment) == 0 || fragmentPort != port {
				addDiagnostic(&diags, "sysex_escape", "standalone F7 escape ignored", port, track)
				fragment = nil
				continue
			}
			fragment = append(fragment, v...)
			if len(fragment) > 0 && fragment[len(fragment)-1] == 247 {
				complete := true
				for _, b := range fragment[:len(fragment)-1] {
					if b >= 128 {
						complete = false
					}
				}
				if complete {
					events = append(events, Event{Tick: tick, Track: track, Order: order, Port: port, Kind: SysExEvent, Payload: fragment})
				} else {
					addDiagnostic(&diags, "invalid_sysex", "invalid SysEx payload ignored", port, track)
				}
				fragment = nil
			}
		default:
			return fail(errors.New("unsupported SMF status"))
		}
		if len(events)+len(tempos) > limits.events {
			return fail(errors.New("SMF event resource limit exceeded"))
		}
	}
	return fail(io.ErrUnexpectedEOF)
}
