package ui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/anim"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

// warnDetailRows is the box height at which the photosensitivity gate can show
// what it measured rather than only what it concluded. Below this it says the
// worst number and offers the same choice. TestWarnPanel_FitsItsBox checks the
// tall layout against this figure, so the two cannot drift apart.

// syncAnim keeps the animation aligned with the audio and sized for its box.
//
// It runs on the tick rather than from a resize or panel hook because the box
// depends on the terminal, the panel mode and the lyric row all at once, and the
// audio position moves underneath it. One idempotent call covers every one of
// those without any of them having to know an animation exists.
func (m *Model) syncAnim() tea.Cmd {
	if m.Anim == nil {
		return nil
	}
	plan := m.layoutPlan()
	return m.Anim.Sync(m.Audio.Elapsed, m.Audio.IsPlaying, plan.ContentInnerW, plan.ContentInnerH)
}

// runAnim implements ":anim": it turns the animation for the current track on or
// off.
func runAnim(m *Model, inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.Parts) > 1 {
		m.Error.Set("The anim command takes no arguments", m.Config.Error.Duration)
		return m, nil
	}
	if m.Anim == nil || m.Anim.Loading {
		return m, nil
	}
	if m.animVisible() {
		m.Anim.Close()
		return m, nil
	}
	if m.Audio.Player == nil {
		m.Error.Set("No audio loaded", m.Config.Error.Duration)
		return m, nil
	}

	plan := m.layoutPlan()
	load := anim.Load{
		Path:     m.Audio.Player.Path(),
		Columns:  plan.ContentInnerW,
		Lines:    plan.ContentInnerH,
		GateMode: m.gateMode(),
	}

	// Asking for the animation is what makes the battery worth asking about.
	// The reading is taken before the sidecar is, so that a warning about a flat
	// battery costs nothing to find out about: nothing is read, decoded or
	// analysed until somebody says to go ahead.
	m.animRequested = true
	if reading, low := m.Anim.LowBattery(m.warnBelow()); low {
		m.Anim.Hold(reading, load)
		return m, nil
	}
	return m, m.Anim.LoadFor(load)
}

// handleAnimLoaded installs a sidecar that finished loading, or reports why
// there was none.
func handleAnimLoaded(m *Model, msg anim.LoadedMsg) (tea.Model, tea.Cmd) {
	cmd, applied := m.Anim.Apply(msg)
	if !applied {
		// An older result is not news about the animation on the surface now, and
		// it has no answer for a request that is still waiting.
		return m, nil
	}

	requested := m.animRequested
	m.animRequested = false

	switch {
	case errors.Is(m.Anim.Err, anim.ErrNoSidecar):
		// Most tracks have no animation, which is an answer rather than a
		// failure -- unless somebody asked for one by name.
		if requested {
			m.Error.Set("No animation for this track", m.Config.Error.Duration)
		} else {
			m.Info.Set("No animation for this track", m.Config.Error.Duration)
		}
	case m.Anim.Err != nil:
		m.Error.Set(fmt.Sprintf("Animation failed to load: %v", m.Anim.Err), m.Config.Error.Duration)
	}

	return m, cmd
}

// animVisible reports whether the animation is showing. A Model built without
// NewModel carries no animation state and reads as hidden.
func (m *Model) animVisible() bool {
	return m.Anim != nil && m.Anim.Visible
}

// animAuto reports whether animations should show without being asked for. A
// Model built without NewModel carries no config and reads as off.
func (m *Model) animAuto() bool {
	return m.Config != nil && m.Config.Anim.Auto
}

// gateMode is the photosensitivity gate the config asks for. An unrecognised
// value behaves as the union inside the anim package, so the only thing to
// guard here is a Model that carries no config at all.
func (m *Model) gateMode() string {
	if m.Config == nil {
		return config.GateModeEither
	}
	return m.Config.Anim.Photosensitivity.Mode
}

// warnBelow is the charge at or below which an animation waits for an answer
// before it is read. A Model built without NewModel carries no config and reads
// as the default the config itself starts from; zero is a config's own way of
// turning the warning off, and is passed through as one.
func (m *Model) warnBelow() int {
	if m.Config == nil {
		return config.DefaultWarnBelow
	}
	return m.Config.Anim.Power.WarnBelow
}

// animFrame returns the frame the animation is showing, when it is showing one
// that already fills the box's interior. That is the case worth splicing into the
// box rather than measuring into it -- see contentBox.
func animFrame(m *Model, plan layoutPlan) (string, bool) {
	if m.Anim == nil || !m.Anim.Visible || m.Anim.Loading || m.Anim.Held() || m.Anim.Gated() {
		return "", false
	}
	if !m.Anim.Fits(plan.ContentInnerW, plan.ContentInnerH) {
		return "", false
	}
	return m.Anim.View(), true
}

// contentBox wraps a frame in the content style without handing the frame to
// lipgloss to measure.
//
// A frame arrives already the size of the box's interior -- it is fitted to it and
// centred in it before it gets here -- so measuring it is work with nothing to
// show for it, and it is the heaviest work a frame does. A dense animation is tens
// of thousands of bytes of escape sequences, and lipgloss walks every one of them
// to find where the lines end, once for this box and again for each box it is
// joined into. An empty box of the same shape has the same border, the same
// padding and the same interior rows, and costs almost nothing to measure, so the
// frame's own lines are spliced into that instead.
func contentBox(style lipgloss.Style, block string, plan layoutPlan) string {
	paint := func(content string) string {
		return style.Width(plan.ContentWidth).Height(plan.ContentHeight).Render(content)
	}
	if plan.ContentInnerW <= 0 || plan.ContentInnerH <= 0 {
		// There is no interior to splice into. The style still knows what to draw.
		return paint(block)
	}

	// What the frame needs from the style is what sits either side of it: the
	// border colour, the border glyphs, and the padding. Every interior row has the
	// same ones, so a box one interior row tall says what the whole box says, and
	// painting it costs the width of the box rather than its area. One space is
	// placeholder enough: the style pads what is short.
	shallow := strings.Split(style.Width(plan.ContentWidth).
		Height(contentBorderH+2*contentPaddingV+1).
		Render(" "), "\n")

	// The left edge is half the border, the two sides together being
	// contentBorderW.
	row := 1 + contentPaddingV
	edge := contentBorderW/2 + contentPaddingH
	if len(shallow) <= row {
		return paint(block)
	}
	left := ansi.Truncate(shallow[row], edge, "")
	right := ansi.TruncateLeft(shallow[row], edge+plan.ContentInnerW, "")

	// That row, repeated between the two border rows, is the box the frame goes
	// into.
	interior := left + strings.Repeat(" ", plan.ContentInnerW) + right
	box := make([]string, 0, plan.ContentHeight)
	box = append(box, shallow[0])
	for range plan.ContentHeight - contentBorderH {
		box = append(box, interior)
	}
	box = append(box, shallow[len(shallow)-1])

	lines := strings.Split(block, "\n")
	for i := 0; i < plan.ContentInnerH && i < len(lines); i++ {
		if lines[i] != "" {
			box[row+i] = left + lines[i] + right
		}
	}
	return strings.Join(box, "\n")
}

// animBody returns what the animation wants to draw in the content box, and
// whether it wants to draw anything at all.
//
// The box belongs to the active tab, and the animation borrows it: an .nvaa file
// is a sidecar of the track, like its lyrics, so it takes the content area
// rather than a page of its own. A frame that is ready to be drawn does not come
// through here -- see animFrame.
func animBody(m *Model, plan layoutPlan) (string, bool) {
	if m.Anim == nil {
		return "", false
	}

	switch {
	case m.Anim.Loading:
		return renderAnimLoading(m, plan), true
	case m.Anim.Held():
		return renderAnimPowerWarning(m, plan), true
	case m.Anim.Gated():
		return renderAnimWarning(m, plan), true
	case m.Anim.Visible:
		// The animation owns the box but has nothing to put in it yet: the box
		// changed and the player has not been resized to it. One blank frame is
		// better than a frame that overflows the border.
		return "", true
	}
	return "", false
}

// renderAnimLoading fills the box while a sidecar is being read. The vectors in
// testdata load instantly; a full-length animation does not, and an empty box
// would read as a failure.
func renderAnimLoading(m *Model, plan layoutPlan) string {
	idx := m.loadingTick / 6 % len(loadingFrames)
	return layoutWarnLines([]warnLine{{
		Text:  loadingFrames[idx] + " Loading animation...",
		Style: infoStyle,
	}}, plan.ContentInnerW, plan.ContentInnerH)
}

// animSkippedMessage says why an animation nobody asked for did not appear. It
// names the way to overrule the answer, because the only other place the battery
// is mentioned is a warning that was never opened.
func animSkippedMessage(percent int) string {
	return fmt.Sprintf("Animation skipped: battery at %d%% (:anim to play)", percent)
}
