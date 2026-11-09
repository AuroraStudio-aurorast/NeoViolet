package ui

import (
	"fmt"
	"math"
	"strings"
)

var pageBands = [...]string{"80 Hz", "250 Hz", "1 kHz", "4 kHz", "12 kHz"}

func (p *pageState) renderEffects(c *pageCanvas, focused bool, status string) {
	w, h := c.width, len(c.rows)
	bypass := "Off"
	if p.effects.Bypass {
		bypass = "On"
	}
	c.pair(0, 1, w-2, "Sound desk", "Bypass "+bypass)
	compact := h <= 14
	split := w >= 58
	left := w - 1
	if split {
		left = w * 61 / 100
		c.divider(left+1, 1, h-2)
	}
	if !split && p.effect >= 5 {
		c.dim(1, 1, "Channel controls / session")
		p.renderChannelControls(c, 1, w-2, focused)
	} else {
		heading := "5-band equalizer / dB"
		if compact {
			heading = "EQ / dB"
		}
		c.dim(1, 1, heading)
		p.renderFaders(c, left, focused)
		if split {
			p.renderChannelControls(c, left+3, w-left-4, focused)
		}
	}
	if h >= 20 {
		c.dim(h-2, 1, "PCM → Preamp → EQ → Mono → Pan → Swap → Volume")
	}
	if status == "" {
		status = "Session · ready for next track"
	}
	c.dim(h-1, 1, status)
	if h >= 20 {
		c.dim(h-3, 1, "←/→ Select · +/- Adjust · Enter Toggle · r Reset")
	} else {
		c.dim(h-2, 1, "←/→ Select · +/- Edit · r Reset")
	}
}

func (p *pageState) renderFaders(c *pageCanvas, left int, focused bool) {
	h := len(c.rows)
	top, bottom := 4, min(15, h-8)
	switch {
	case h <= 14:
		top, bottom = 2, h-5
	case h < 20:
		top, bottom = 3, h-6
	}
	height := max(1, bottom-top)
	zero := top + int(math.Round(float64(height)/2))
	c.dim(zero, 0, "0")
	c.dim(zero, 4, strings.Repeat("┄", max(0, left-5)))
	if h > 14 {
		c.dim(top, 0, "+12")
		c.dim(bottom, 0, "-12")
	}
	for i, band := range pageBands {
		x := 6 + i*(left-10)/4
		selected := p.effect == i
		c.divider(x, top, bottom-top+1)
		knob := top + int(math.Round((12-p.effects.Gains[i])/24*float64(height)))
		c.label(knob, x-1, "━━━")
		c.text(bottom+1, x-3, band)
		c.dim(bottom+2, x-2, fmt.Sprintf("%.1f", p.effects.Gains[i]))
		if selected && focused {
			for y := top; y <= bottom+2; y++ {
				c.highlight(y, x-2, 5)
			}
		}
	}
}

func (p *pageState) renderChannelControls(c *pageCanvas, x, width int, focused bool) {
	h := len(c.rows)
	c.dim(1, x, "Channels")
	y := 4
	offsets := []int{0, 4, 6, 10, 14}
	if h <= 15 {
		y = 2
		offsets = []int{0, 1, 2, 3, 4}
	} else if h < 22 {
		y = 3
		offsets = []int{0, 3, 5, 8, 10}
	}
	states := []string{fmt.Sprintf("%.1f", p.effects.Pan), "Off", "Off", fmt.Sprintf("%.1f dB", p.effects.PreampDB), "Off"}
	if p.effects.Pan == 0 {
		states[0] = "Center"
	}
	if p.effects.Mono {
		states[1] = "On"
	}
	if p.effects.Swap {
		states[2] = "On"
	}
	if p.effects.Bypass {
		states[4] = "On"
	}
	labels := []string{"Pan", "Mono", "Swap L/R", "Preamp", "Bypass"}
	for i, label := range labels {
		row := y + offsets[i]
		if row >= h-1 {
			continue
		}
		c.pair(row, x, width, label, states[i])
		if p.effect == i+5 && focused {
			c.highlight(row, x, width)
		}
	}
	if h > 15 {
		c.dim(y+1, x, "L"+strings.Repeat("─", max(1, width-2))+"R")
		c.label(y+1, x+1+int(math.Round((p.effects.Pan+1)/2*float64(max(0, width-3)))), "●")
		gy := y + offsets[3] + 1
		c.dim(gy, x, strings.Repeat("─", max(0, width)))
		c.label(gy, x+int(math.Round((p.effects.PreampDB+12)/12*float64(max(0, width-1)))), "●")
	}
}
