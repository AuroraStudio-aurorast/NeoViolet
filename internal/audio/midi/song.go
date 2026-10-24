// Package midi parses bounded SMF files and renders independent MIDI ports.
package midi

// EventKind distinguishes channel messages from complete SysEx payloads.
type EventKind uint8

// Message kinds retained in the timeline.
const (
	ChannelEvent EventKind = iota
	SysExEvent
)

// Event is a raw channel message or complete SysEx, with its original track identity.
type Event struct {
	Tick                       uint64
	Track, Order               uint32
	Port, Status, Data1, Data2 uint8
	Kind                       EventKind
	Payload                    []byte
}
type tempoEvent struct {
	tick         uint64
	track, order uint32
	tempo        uint32
}

// Song is immutable after Parse; ports remain distinct even across tracks.
type Song struct {
	Events   []Event
	division uint16
	end      uint64
	tempos   []tempoEvent
}

// TimedEvent retains both the exact and DSP-quantized event frames.
type TimedEvent struct {
	Frame, DispatchFrame int64
	Event                Event
}

// Timeline is an immutable, sample-rate-specific schedule including release time.
type Timeline struct {
	Events                     []TimedEvent
	ContentFrames, TotalFrames int64
	SampleRate                 int32
}

// Diagnostic is an aggregate compatibility notice with source identity.
type Diagnostic struct {
	Code, Message string
	Port          uint8
	Track         uint32
	Bank, Program int32
	Count         uint64
}

func addDiagnostic(d *[]Diagnostic, code, msg string, port uint8, track uint32) {
	if d == nil {
		return
	}
	for i := range *d {
		v := &(*d)[i]
		if v.Code == code && v.Port == port && v.Track == track && v.Message == msg {
			v.Count++
			return
		}
	}
	if len(*d) < 128 {
		*d = append(*d, Diagnostic{Code: code, Message: msg, Port: port, Track: track, Count: 1})
		return
	}
	(*d)[127].Code = "suppressed"
	(*d)[127].Message = "additional diagnostics suppressed"
	(*d)[127].Count++
}
