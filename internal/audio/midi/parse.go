package midi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
)

type parseLimits struct{ input, events, sysex int }

var defaultLimits = parseLimits{64 << 20, 2000000, 1 << 20}

// Parse accepts SMF formats 0/1 and preserves port and stable track ordering.
func Parse(r io.Reader) (*Song, []Diagnostic, error) { return parse(r, defaultLimits) }
func parse(r io.Reader, limits parseLimits) (*Song, []Diagnostic, error) {
	data, err := io.ReadAll(io.LimitReader(r, int64(limits.input)+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > limits.input {
		return nil, nil, errors.New("SMF input resource limit exceeded")
	}
	if len(data) < 14 || string(data[:4]) != "MThd" {
		return nil, nil, errors.New("SMF: missing header")
	}
	h := int64(binary.BigEndian.Uint32(data[4:8]))
	if h < 6 || h > int64(len(data)-8) {
		return nil, nil, errors.New("SMF: invalid header length")
	}
	format, count, division := binary.BigEndian.Uint16(data[8:]), binary.BigEndian.Uint16(data[10:]), binary.BigEndian.Uint16(data[12:])
	if format > 1 {
		return nil, nil, errors.New("SMF: unsupported independent sequence format")
	}
	if count == 0 || (format == 0 && count != 1) {
		return nil, nil, errors.New("SMF: invalid track count")
	}
	if _, _, err = timingRatio(division, 44100); err != nil {
		return nil, nil, err
	}
	s := &Song{division: division}
	var diags []Diagnostic
	pos := int(h) + 8
	for track := uint32(0); track < uint32(count); track++ {
		if len(data)-pos < 8 || string(data[pos:pos+4]) != "MTrk" {
			return nil, nil, fmt.Errorf("track%d: missing MTrk", track)
		}
		n := int64(binary.BigEndian.Uint32(data[pos+4:]))
		pos += 8
		if n > int64(len(data)-pos) {
			return nil, nil, fmt.Errorf("track%d: declared length exceeds file", track)
		}
		end := pos + int(n)
		events, tempos, tick, d, err := parseTrack(data[pos:end], track, limits)
		if err != nil && track == uint32(count)-1 && len(data)-end > 0 && len(data)-end <= 8 {
			// Only append an exact zero-delta EOT, optionally completing the boundary event.
			if bytes.HasSuffix(data[end:], []byte{0, 255, 47, 0}) {
				ev, tm, tk, dd, e := parseTrack(data[pos:], track, limits)
				if e == nil && onlyFinalEOT(end-pos, data[pos:]) {
					events, tempos, tick, d, err = ev, tm, tk, dd, nil
					end = len(data)
					addDiagnostic(&d, "track_length_repaired", "last track length repaired", 0, track)
				}
			}
		}
		if err != nil {
			return nil, nil, fmt.Errorf("track%d: %w", track, err)
		}
		s.Events = append(s.Events, events...)
		s.tempos = append(s.tempos, tempos...)
		s.end = max(s.end, tick)
		if len(s.Events)+len(s.tempos) > limits.events {
			return nil, nil, errors.New("SMF event resource limit exceeded")
		}
		for _, v := range d {
			addDiagnostic(&diags, v.Code, v.Message, v.Port, v.Track)
		}
		pos = end
	}
	if pos != len(data) {
		return nil, nil, errors.New("SMF: trailing bytes")
	}
	sort.SliceStable(s.Events, func(i, j int) bool {
		a, b := s.Events[i], s.Events[j]
		if a.Tick != b.Tick {
			return a.Tick < b.Tick
		}
		if a.Track != b.Track {
			return a.Track < b.Track
		}
		return a.Order < b.Order
	})
	sort.SliceStable(s.tempos, func(i, j int) bool {
		a, b := s.tempos[i], s.tempos[j]
		if a.tick != b.tick {
			return a.tick < b.tick
		}
		if a.track != b.track {
			return a.track < b.track
		}
		return a.order < b.order
	})
	return s, diags, nil
}

// onlyFinalEOT permits just a boundary event completion and zero-delta EOT.
func onlyFinalEOT(boundary int, data []byte) bool {
	// Reparse the original boundary with a virtual suffix only if at most one event
	// extends beyond it; a successful complete parse must end with exactly EOT.
	p := trackParser{data: data}
	cross := 0
	for p.pos < len(data) {
		start := p.pos
		delta, e := p.vlq()
		if e != nil {
			return false
		}
		status, e := p.readStatus()
		if e != nil {
			return false
		}
		switch status {
		case 255:
			typ, e := p.byte()
			if e != nil {
				return false
			}
			n, e := p.vlq()
			if e != nil || int64(len(data)-p.pos) < int64(n) {
				return false
			}
			p.pos += int(n)
			if p.pos > boundary {
				cross++
				if typ == 47 && n == 0 && delta == 0 && p.pos == len(data) && start >= boundary {
					return cross <= 2
				}
			}
		case 240, 247:
			n, e := p.vlq()
			if e != nil || int64(len(data)-p.pos) < int64(n) {
				return false
			}
			p.pos += int(n)
			if p.pos > boundary {
				cross++
			}
		default:
			n := 2
			if status&240 == 192 || status&240 == 208 {
				n = 1
			}
			if len(data)-p.pos < n {
				return false
			}
			p.pos += n
			if p.pos > boundary {
				cross++
			}
		}
		if cross > 1 || start >= boundary {
			return false
		}
	}
	return false
}
