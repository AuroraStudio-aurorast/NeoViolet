package midi

import "testing"

func TestBankModes(t *testing.T) {
	for _, tc := range []struct {
		mode     synthMode
		msb, lsb uint8
		bank     int32
		drum     bool
	}{
		{modeSF2, 8, 16, 8, false}, {modeGS, 8, 16, 8, false}, {modeGM, 8, 16, 0, false},
		{modeXG, 0, 16, 16, false}, {modeXG, 64, 0, 64, false}, {modeXG, 127, 0, 128, true}, {modeGM2, 121, 8, 8, false}, {modeGM2, 120, 0, 128, true},
	} {
		p := &port{mode: tc.mode}
		p.msb[0] = tc.msb
		p.lsb[0] = tc.lsb
		bank, drum := p.bank(0)
		if bank != tc.bank || drum != tc.drum {
			t.Fatalf("%+v => %d,%v", tc, bank, drum)
		}
	}
}
func TestSysExResetAndPartModes(t *testing.T) {
	r, e := NewRenderer(&Timeline{SampleRate: 44100}, testFont(t))
	if e != nil {
		t.Fatal(e)
	}
	_ = r
	p := newPortForTest(t)
	var d []Diagnostic
	p.process(Event{Kind: SysExEvent, Payload: []byte{0x43, 0x10, 0x4C, 0, 0, 0x7E, 0, 0xF7}}, &d)
	if p.mode != modeXG {
		t.Fatal("XG reset ignored")
	}
	p.process(Event{Kind: SysExEvent, Payload: []byte{0x43, 0x10, 0x4C, 8, 2, 7, 1, 0xF7}}, &d)
	if !p.percussion[2] {
		t.Fatal("part mode ignored")
	}
	p.process(Event{Kind: SysExEvent, Payload: []byte{0x7E, 0x7F, 9, 1, 0xF7}}, &d)
	if p.mode != modeGM || p.percussion[2] {
		t.Fatal("GM reset did not reset part state")
	}
	p.process(Event{Kind: SysExEvent, Payload: []byte{0x43, 0x11, 0x4C, 0, 0, 0x7E, 0, 0xF7}}, &d)
	if p.mode != modeGM {
		t.Fatal("wrong device accepted")
	}
}
func newPortForTest(t *testing.T) *port {
	t.Helper()
	r, e := NewRenderer(&Timeline{SampleRate: 44100, Events: []TimedEvent{{Event: Event{Status: 0x90}}}}, testFont(t))
	if e != nil {
		t.Fatal(e)
	}
	return r.ports[0]
}
