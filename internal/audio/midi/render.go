package midi

import (
	"errors"
	"io"
	"sort"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/soundfont"
)

// Renderer has a single owner; its timeline and SoundFont remain read-only.
type Renderer struct {
	timeline    *Timeline
	ports       map[uint8]*port
	order       []uint8
	position    int64
	next        int
	released    bool
	left, right [64]float32
	diagnostics []Diagnostic
}

// RenderStats contains per-port voice counts and bounded compatibility diagnostics.
type RenderStats struct {
	Ports       map[uint8]soundfont.Stats
	Diagnostics []Diagnostic
}

// NewRenderer creates independent port state sharing only immutable song and sample data.
func NewRenderer(t *Timeline, sf *soundfont.SoundFont) (*Renderer, error) {
	if t == nil || sf == nil || t.SampleRate < 16000 || t.TotalFrames < 0 {
		return nil, errors.New("invalid MIDI renderer inputs")
	}
	r := &Renderer{timeline: t, ports: map[uint8]*port{}}
	for _, e := range t.Events {
		r.ports[e.Event.Port] = nil
	}
	if len(r.ports) > 32 {
		return nil, errors.New("MIDI port resource limit exceeded (32)")
	}
	for id := range r.ports {
		r.order = append(r.order, id)
	}
	sort.Slice(r.order, func(i, j int) bool { return r.order[i] < r.order[j] })
	for _, id := range r.order {
		settings := soundfont.NewSynthesizerSettings(t.SampleRate)
		settings.MaximumPolyphony = 256
		settings.BlockSize = 64
		s, err := soundfont.NewSynthesizer([]*soundfont.SoundFont{sf}, settings)
		if err != nil {
			return nil, err
		}
		r.ports[id] = newPort(s)
	}
	// Compatibility analysis runs once before publication, never inside the
	// audio callback. It uses private throwaway controllers, not DSP voices.
	analysis := map[uint8]*port{}
	for _, id := range r.order {
		settings := soundfont.NewSynthesizerSettings(t.SampleRate)
		settings.MaximumPolyphony = 8
		settings.EnableReverbAndChorus = false
		s, err := soundfont.NewSynthesizer([]*soundfont.SoundFont{sf}, settings)
		if err != nil {
			return nil, err
		}
		analysis[id] = newPort(s)
	}
	for _, e := range t.Events {
		ev := e.Event
		if ev.Kind == SysExEvent || ev.Status&240 == 0xB0 || ev.Status&240 == 0xC0 {
			analysis[ev.Port].process(ev, &r.diagnostics)
		}
	}
	return r, nil
}

// PositionFrames returns the number of output frames consumed.
func (r *Renderer) PositionFrames() int64 { return r.position }

// Stats snapshots counters; call it from the rendering owner.
func (r *Renderer) Stats() RenderStats {
	s := RenderStats{Ports: map[uint8]soundfont.Stats{}, Diagnostics: append([]Diagnostic(nil), r.diagnostics...)}
	for _, id := range r.order {
		s.Ports[id] = r.ports[id].synth.Stats()
	}
	return s
}

// Render splits at DSP boundaries even for irregular consumer buffers. No event
// can change a controller while a previously generated DSP block remains unread.
func (r *Renderer) Render(left, right []float32) (int, error) {
	length := min(len(left), len(right))
	written := 0
	for written < length && r.position < r.timeline.TotalFrames {
		if r.position%64 == 0 {
			for r.next < len(r.timeline.Events) && r.timeline.Events[r.next].DispatchFrame <= r.position {
				ev := r.timeline.Events[r.next].Event
				r.ports[ev.Port].process(ev, nil)
				r.next++
			}
			if !r.released && r.position >= (r.timeline.ContentFrames+63)/64*64 {
				for _, id := range r.order {
					r.ports[id].synth.ReleaseAll()
				}
				r.released = true
			}
		}
		n := min(length-written, int(64-r.position%64), int(r.timeline.TotalFrames-r.position))
		clear(left[written : written+n])
		clear(right[written : written+n])
		for _, id := range r.order {
			p := r.ports[id]
			p.synth.Render(r.left[:n], r.right[:n])
			for i := range n {
				left[written+i] += r.left[i]
				right[written+i] += r.right[i]
			}
		}
		written += n
		r.position += int64(n)
	}
	if r.position >= r.timeline.TotalFrames {
		return written, io.EOF
	}
	return written, nil
}
