package ui

import (
	"fmt"
	"math"
	"strings"
)

var pageBands = [...]string{"80 Hz", "250 Hz", "1 kHz", "4 kHz", "12 kHz"}

func (p *pageState) renderEffects(c *pageCanvas, focused bool) {
	w, h := c.width, len(c.rows)
	bypass := "Off"
	if p.bypass {
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
		c.dim(1, 1, "Channel controls / sample values")
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
		c.dim(h-2, 1, "PCM → Preamp → EQ → Pan / Mono / Swap → Output")
	}
	c.dim(h-1, 1, "Preview only · ←/→ Select · +/- Adjust · r Reset")
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
		knob := top + int(math.Round((12-p.gains[i])/24*float64(height)))
		c.label(knob, x-1, "━━━")
		c.text(bottom+1, x-3, band)
		c.dim(bottom+2, x-2, fmt.Sprintf("%.1f", p.gains[i]))
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
	if h <= 14 {
		y = 2
		offsets = []int{0, 1, 2, 3, 4}
	} else if h < 20 {
		y = 3
		offsets = []int{0, 3, 5, 8, 10}
	}
	states := []string{fmt.Sprintf("%.1f", p.pan), "Off", "Off", fmt.Sprintf("%.1f dB", p.preamp), "Off"}
	if p.pan == 0 {
		states[0] = "Center"
	}
	if p.mono {
		states[1] = "On"
	}
	if p.swap {
		states[2] = "On"
	}
	if p.bypass {
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
	if h > 14 {
		c.dim(y+1, x, "L"+strings.Repeat("─", max(1, width-2))+"R")
		c.label(y+1, x+1+int(math.Round((p.pan+1)/2*float64(max(0, width-3)))), "●")
		gy := y + offsets[3] + 1
		c.dim(gy, x, strings.Repeat("─", max(0, width)))
		c.label(gy, x+int(math.Round((p.preamp+12)/12*float64(max(0, width-1)))), "●")
	}
}

func (p *pageState) adjustEffect(delta float64) {
	switch p.effect {
	case 0, 1, 2, 3, 4:
		p.gains[p.effect] = max(-12, min(12, p.gains[p.effect]+delta*.5))
	case 5:
		p.pan = math.Round(max(-1, min(1, p.pan+delta*.1))*10) / 10
	case 6:
		p.mono = !p.mono
	case 7:
		p.swap = !p.swap
	case 8:
		p.preamp = max(-12, min(0, p.preamp+delta*.5))
	case 9:
		p.bypass = !p.bypass
	}
}
func (p *pageState) resetEffect() {
	switch p.effect {
	case 0, 1, 2, 3, 4:
		p.gains[p.effect] = 0
	case 5:
		p.pan = 0
	case 6:
		p.mono = false
	case 7:
		p.swap = false
	case 8:
		p.preamp = -3
	case 9:
		p.bypass = false
	}
}
