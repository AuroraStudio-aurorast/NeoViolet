package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio"
	"github.com/gopxl/beep/v2"
)

type effectsPlayer struct {
	mockPlayer
	settings   audio.EffectsSettings
	effectsErr error
	calls      int
}

func (p *effectsPlayer) SetEffects(s audio.EffectsSettings) error {
	p.calls++
	if p.effectsErr != nil {
		return p.effectsErr
	}
	p.settings = s
	return nil
}
func (p *effectsPlayer) Effects() audio.EffectsSettings     { return p.settings }
func (p *effectsPlayer) EffectsSampleRate() beep.SampleRate { return 8000 }
func effectsFixture(t *testing.T) (*Model, *effectsPlayer) {
	t.Helper()
	m := wiredSettingsModel(t)
	m.UI.ActiveTab = 2
	p := &effectsPlayer{mockPlayer: mockPlayer{playing: true, duration: time.Minute, volume: .4}}
	m.Audio.Player = p
	m.Audio.IsPlaying = true
	return m, p
}
func TestEffectsPageAppliesControlsAndResetWithoutTransportChanges(t *testing.T) {
	m, p := effectsFixture(t)
	pageKey(m, '+', "+")
	if p.settings.Gains[0] != .5 || m.pages.effects.Gains[0] != .5 {
		t.Fatal("EQ edit did not reach audio")
	}
	m.pages.effect = 5
	pageKey(m, '+', "+")
	if p.settings.Pan != .1 {
		t.Fatal("pan not applied")
	}
	for _, i := range []int{6, 7, 9} {
		m.pages.effect = i
		pageKey(m, tea.KeyEnter, "")
	}
	if !p.settings.Mono || !p.settings.Swap || !p.settings.Bypass {
		t.Fatal("switch not applied")
	}
	m.pages.effect = 8
	pageKey(m, '-', "-")
	if p.settings.PreampDB != -.5 {
		t.Fatal("preamp not applied")
	}
	pageKey(m, 'r', "r")
	if p.settings.PreampDB != 0 {
		t.Fatal("reset not neutral")
	}
	if p.playCalled || p.pauseCalled || p.seekCalled || p.closeCalled || p.volume != .4 {
		t.Fatal("DSP edit changed transport or master volume")
	}
}
func TestEffectsFailedEditKeepsLastAcceptedValues(t *testing.T) {
	m, p := effectsFixture(t)
	pageKey(m, '+', "+")
	p.effectsErr = errors.New("rejected")
	pageKey(m, '+', "+")
	if m.pages.effects.Gains[0] != .5 || p.settings.Gains[0] != .5 || !m.Error.Visible {
		t.Fatal("failed edit replaced accepted values")
	}
}
func TestEffectsSessionPreparedWithoutPlayerAndAppliedAtAcceptance(t *testing.T) {
	m := wiredSettingsModel(t)
	m.UI.ActiveTab = 2
	pageKey(m, '+', "+")
	if m.pages.effects.Gains[0] != .5 || m.Audio.Player != nil {
		t.Fatal("idle edit failed")
	}
	_, _ = handleLoadTrack(m, LoadTrackMsg{Path: "next.wav"})
	pageKey(m, '+', "+")
	candidate := &effectsPlayer{mockPlayer: mockPlayer{duration: time.Second}}
	_, _ = m.Update(AudioLoadedMsg{Generation: m.loadGeneration, Path: "next.wav", Player: candidate})
	if candidate.settings.Gains[0] != 1 || !candidate.playCalled {
		t.Fatal("new track used stale preset")
	}
	stale := &effectsPlayer{}
	_, _ = m.Update(AudioLoadedMsg{Generation: m.loadGeneration - 1, Player: stale, Path: "old.wav"})
	if stale.calls != 0 || !stale.closeCalled || candidate.settings.Gains[0] != 1 {
		t.Fatal("stale candidate received session preset")
	}
}
func TestEffectsUnsupportedPlayerAndFailedCandidateAreHonest(t *testing.T) {
	m, p := effectsFixture(t)
	pageKey(m, '+', "+")
	candidate := &effectsPlayer{effectsErr: errors.New("cannot configure")}
	m.loadGeneration = 2
	_, _ = m.Update(AudioLoadedMsg{Generation: 2, Player: candidate, Path: "bad.wav"})
	if m.Audio.Player != p || !candidate.closeCalled || p.pauseCalled || !m.Error.Visible {
		t.Fatal("failed DSP candidate replaced playing track")
	}
	m.Audio.Player = &mockPlayer{}
	pageKey(m, '+', "+")
	if m.pages.effects.Gains[0] != .5 || !strings.Contains(m.Error.Message, "unavailable") {
		t.Fatal("unsupported player pretended to apply edit")
	}
}
func TestEffectsPageDisplaysSessionAndNyquistLimit(t *testing.T) {
	m, _ := effectsFixture(t)
	m.UI.Width = 120
	m.UI.Height = 30
	got := stripANSI(renderContent(m, m.layoutPlan()))
	if strings.Contains(got, "Preview") || strings.Contains(got, "sample") || !strings.Contains(got, "Session") || !strings.Contains(got, "4 kHz") {
		t.Fatalf("incorrect DSP status: %s", got)
	}
}

func TestEffectsHintsNeverEraseGainValuesOrSelectedBypass(t *testing.T) {
	m, _ := effectsFixture(t)
	m.pages.effects.Gains = [5]float64{-12, -6, 0, 6, 12}
	c := newPageCanvas(76, 14, m)
	m.pages.renderEffects(c, true, m.effectsStatus())
	body := stripANSI(c.String())
	for _, gain := range []string{"-12.0", "-6.0", "0.0", "6.0", "12.0"} {
		if !strings.Contains(body, gain) {
			t.Fatalf("short layout erased gain %s", gain)
		}
	}
	m.pages.effect = 9
	c = newPageCanvas(64, 16, m)
	m.pages.renderEffects(c, true, m.effectsStatus())
	if !strings.Contains(stripANSI(c.rows[13]), "Bypass") {
		t.Fatal("hint erased selected bypass label")
	}
}

func TestEffectsSelectedBypassSurvivesAllSupportedCompactSizes(t *testing.T) {
	m, _ := effectsFixture(t)
	m.pages.effect = 9
	for _, width := range []int{42, 64, 80} {
		for height := 10; height <= 25; height++ {
			c := newPageCanvas(width, height, m)
			m.pages.renderEffects(c, true, m.effectsStatus())
			if len(c.selections) != 1 {
				t.Fatalf("%dx%d bypass selection missing", width, height)
			}
			row := c.selections[0].row
			if !strings.Contains(stripANSI(c.rows[row]), "Bypass") {
				t.Fatalf("%dx%d hint overwrote bypass at row%d: %s", width, height, row, stripANSI(c.rows[row]))
			}
		}
	}
}
