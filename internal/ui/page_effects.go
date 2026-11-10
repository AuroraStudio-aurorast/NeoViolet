package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// maxEffectRailRows caps the visible rail; frequency and gain labels use two more rows.
const maxEffectRailRows = 17

var pageBands = [...]string{"80 Hz", "250 Hz", "1 kHz", "4 kHz", "12 kHz"}
var effectLabels = [...]string{"80 Hz", "250 Hz", "1 kHz", "4 kHz", "12 kHz", "Pan", "Mono", "Swap L/R", "Preamp", "Bypass"}

func (p *pageState) renderEffects(c *pageCanvas, focused bool, status string) {
	w, h := c.width, len(c.rows)
	badge := "Session"
	if p.effects.Bypass {
		badge = "Bypassed"
	}
	c.pair(0, 1, w-2, "Sound desk", badge)
	split := w >= 72 && h >= 14
	if split {
		right := min(30, max(24, w*31/100))
		left := w - right - 5
		c.pair(2, 1, left, "Equalizer", p.equalizerShape())
		c.dim(2, left+4, "Channels")
		c.rule(3, 1, left)
		c.rule(3, left+4, right)
		c.divider(left+2, 2, h-5)
		p.renderFaders(c, 1, left, focused)
		p.renderChannelControls(c, left+4, right, focused)
	} else {
		heading, cue := "EQ / dB", "Channels →"
		if p.effect >= 5 {
			heading, cue = "Channels", "← Equalizer"
		}
		c.pair(1, 1, w-2, heading, cue)
		if h >= 14 {
			c.rule(2, 1, w-2)
		}
		if p.effect < 5 {
			p.renderFaders(c, 1, w-2, focused)
		} else {
			p.renderChannelControls(c, 1, w-2, focused)
		}
	}
	if h >= 16 {
		c.dim(h-3, 1, ansi.Truncate(p.effectDescription(), w-2, "…"))
	}
	hint := "←/→ Select · +/- Edit · r Reset"
	if p.effect == 6 || p.effect == 7 || p.effect == 9 {
		hint = "←/→ Select · Enter Toggle · r Reset"
	}
	c.dim(h-2, 1, hint)
	if status == "" {
		status = "Session · ready for next track"
	}
	c.dim(h-1, 1, status)
}
func (p *pageState) equalizerShape() string {
	for _, gain := range p.effects.Gains {
		if gain != 0 {
			return "Edited"
		}
	}
	return "Flat"
}
func effectDB(value float64) string {
	if value > 0 {
		return fmt.Sprintf("+%.1f", value)
	}
	return fmt.Sprintf("%.1f", value)
}
func effectCentered(c *pageCanvas, y, center int, text string, accent bool) {
	x := max(0, center-ansi.StringWidth(text)/2)
	if accent {
		c.label(y, x, text)
	} else {
		c.dim(y, x, text)
	}
}
func (p *pageState) renderFaders(c *pageCanvas, x, width int, focused bool) {
	h := len(c.rows)
	top := 4
	if h < 14 {
		top = 3
	}
	// Reserve the description (when shown), keyboard hint and session status.
	areaBottom := h - 3
	if h >= 16 {
		areaBottom = h - 4
	}
	available := areaBottom - top + 1
	railRows := min(maxEffectRailRows, max(2, available-2))
	// Center the complete control, including its frequency and gain labels.
	top += max(0, (available-railRows-2)/2)
	bottom := top + railRows - 1
	height := bottom - top
	zero := top + int(math.Round(float64(height)/2))
	first, last := x+6, x+width-5
	c.dim(zero, first-1, strings.Repeat("┄", max(0, last-first+3)))
	for _, tick := range []int{12, 6, 0, -6, -12} {
		if height < 8 && (tick == 6 || tick == -6) {
			continue
		}
		y := top + int(math.Round(float64(12-tick)/24*float64(height)))
		text := fmt.Sprintf("%3d", tick)
		if tick > 0 {
			text = fmt.Sprintf("+%d", tick)
		}
		c.dim(y, x, text)
	}
	for i, band := range pageBands {
		center := first + i*(last-first)/4
		gain := p.effects.Gains[i]
		knob := top + int(math.Round((12-gain)/24*float64(height)))
		active := focused && p.effect == i
		c.divider(center, top, height+1)
		c.dim(zero, center, "┼")
		for y := min(zero, knob) + 1; y < max(zero, knob); y++ {
			if active && !p.effects.Bypass {
				c.label(y, center, "┃")
			} else {
				c.dim(y, center, "┃")
			}
		}
		glyph := "─○─"
		if active {
			glyph = "━●━"
		}
		effectCentered(c, knob, center, glyph, active && !p.effects.Bypass)
		effectCentered(c, bottom+1, center, band, active)
		value := effectDB(gain)
		if active {
			value = "[" + value + "]"
		}
		effectCentered(c, bottom+2, center, value, active)
		if active {
			c.highlight(bottom+2, center-ansi.StringWidth(value)/2, ansi.StringWidth(value))
		}
	}
}
func (p *pageState) effectDescription() string {
	if p.effect < 5 {
		regions := [...]string{"low bass", "warmth", "midrange", "presence", "air"}
		return fmt.Sprintf("%s · %s · 0.5 dB steps", pageBands[p.effect], regions[p.effect])
	}
	switch p.effect {
	case 5:
		return "Pan · stereo balance · 10% steps"
	case 6:
		return "Mono · combine left and right channels"
	case 7:
		return "Swap L/R · reverse output channels"
	case 8:
		return "Preamp · leave headroom for EQ boosts"
	default:
		return "Bypass · compare with unprocessed sound"
	}
}
