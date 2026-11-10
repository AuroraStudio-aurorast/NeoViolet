package midi

import (
	"bytes"
	"math"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/soundfont"
)

func TestPortsIsolateProgramExpressionBend(t *testing.T) {
	sf, e := soundfont.NewSoundFont(bytes.NewReader(miditest.SF2(&miditest.SF2Options{SampleModes: 1, LoopEnd: 1024, Instruments: [][]miditest.Generator{
		{{Operator: 54, Amount: 1}, {Operator: 53, Amount: 0}}, {{Operator: 51, Amount: 12}, {Operator: 54, Amount: 1}, {Operator: 53, Amount: 0}}}, PresetInstruments: []uint16{0, 1}})))
	if e != nil {
		t.Fatal(e)
	}
	a := []byte{0, 0xC0, 0, 0, 0x90, 60, 100, 12, 0xB0, 11, 0, 12, 0xE0, 127, 127, 96, 0x80, 60, 0, 0, 255, 47, 0}
	b := []byte{0, 255, 33, 1, 1, 0, 0xC0, 1, 0, 0x90, 60, 100, 120, 0x80, 60, 0, 0, 255, 47, 0}
	mix := renderPCM(t, timeline(t, a, b), sf, []int{512})
	x := renderPCM(t, timeline(t, a), sf, []int{512})
	y := renderPCM(t, timeline(t, b), sf, []int{512})
	for i := range mix {
		for c := range 2 {
			if math.Abs(float64(mix[i][c]-x[i][c]-y[i][c])) > 1e-6 {
				t.Fatalf("port controls leaked frame%d", i)
			}
		}
	}
}
func TestTracksSharePortState(t *testing.T) {
	a := []byte{0, 0xB0, 11, 0, 0, 255, 47, 0}
	b := []byte{0, 0x90, 60, 100, 120, 0x80, 60, 0, 0, 255, 47, 0}
	pcm := renderPCM(t, timeline(t, a, b), testFont(t), []int{512})
	var energy float64
	for _, x := range pcm {
		energy += math.Abs(float64(x[0]))
	}
	if energy > 0.01 {
		t.Fatalf("same-port CC did not affect other track: %g", energy)
	}
}
func TestSysExPortIsolation(t *testing.T) {
	r, e := NewRenderer(&Timeline{SampleRate: 44100, Events: []TimedEvent{{Event: Event{Port: 0, Status: 0x90}}, {Event: Event{Port: 1, Status: 0x90}}}}, testFont(t))
	if e != nil {
		t.Fatal(e)
	}
	r.ports[0].process(Event{Kind: SysExEvent, Payload: []byte{0x43, 0x10, 0x4C, 0, 0, 0x7E, 0, 247}}, &r.diagnostics)
	if r.ports[0].mode != modeXG || r.ports[1].mode != modeSF2 {
		t.Fatal("SysEx changed another port")
	}
}
