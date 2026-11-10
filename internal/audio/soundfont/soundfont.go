package soundfont

import (
	"errors"
	"fmt"
	"io"
)

// SoundFont owns immutable sample and region data, shared by independent synthesizers.
type SoundFont struct {
	Info          *Info
	BitsPerSample int32
	WaveData      []int16
	SampleHeaders []*SampleHeader
	Presets       []*Preset
	Instruments   []*Instrument
	Diagnostics   []Diagnostic
}

// NewSoundFont loads an SF2 from a seekable source without copying its PCM twice.
func NewSoundFont(r io.ReadSeeker) (*SoundFont, error) { sf, _, err := Load(r); return sf, err }

// Load checks RIFF and table boundaries before allocating or resolving references.
func Load(r io.ReadSeeker) (*SoundFont, []Diagnostic, error) {
	return load(r, loadLimits{1 << 30, 16 << 20})
}

type loadLimits struct{ pcm, pdta int64 }

func load(r io.ReadSeeker, limits loadLimits) (*SoundFont, []Diagnostic, error) {
	start, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, nil, err
	}
	end, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, nil, err
	}
	if _, err = r.Seek(start, io.SeekStart); err != nil {
		return nil, nil, err
	}
	if end-start < 12 {
		return nil, nil, io.ErrUnexpectedEOF
	}
	root, err := readChunk(r, end)
	if err != nil {
		return nil, nil, err
	}
	if root.id != "RIFF" {
		return nil, nil, errors.New("SF2: missing RIFF")
	}
	form := make([]byte, 4)
	if _, err = io.ReadFull(r, form); err != nil {
		return nil, nil, err
	}
	if string(form) != "sfbk" {
		return nil, nil, errors.New("SF2: unsupported form (expected sfbk)")
	}
	sf := &SoundFont{BitsPerSample: 16}
	seen := map[string]bool{}
	for {
		pos, e := r.Seek(0, io.SeekCurrent)
		if e != nil {
			return nil, nil, e
		}
		if pos == root.end {
			break
		}
		c, e := readChunk(r, root.end)
		if e != nil {
			return nil, nil, e
		}
		if c.id == "LIST" {
			if c.size < 4 {
				return nil, nil, errors.New("SF2: short LIST")
			}
			if _, e = io.ReadFull(r, form); e != nil {
				return nil, nil, e
			}
			typ := string(form)
			if typ == "INFO" || typ == "sdta" || typ == "pdta" {
				if seen[typ] {
					return nil, nil, fmt.Errorf("SF2: duplicate %s", typ)
				}
				seen[typ] = true
				switch typ {
				case "sdta":
					e = loadSamples(r, c.end, sf, limits)
				case "INFO":
					e = loadInfo(r, c.end, sf)
				case "pdta":
					e = loadParameters(r, c.end, c.size, sf, limits)
				}
				if e != nil {
					return nil, nil, fmt.Errorf("SF2 %s: %w", typ, e)
				}
			}
		}
		if _, e = r.Seek(c.next, io.SeekStart); e != nil {
			return nil, nil, e
		}
	}
	if !seen["INFO"] || !seen["sdta"] || !seen["pdta"] {
		return nil, nil, errors.New("SF2: missing required LIST")
	}
	validateRegions(sf)
	validateModulators(sf)
	playable := false
	for _, p := range sf.Presets {
		for _, pr := range p.Regions {
			for _, ir := range pr.Instrument.Regions {
				if ir.valid {
					playable = true
				}
			}
		}
	}
	if !playable {
		return nil, nil, errors.New("SF2: no playable regions")
	}
	return sf, append([]Diagnostic(nil), sf.Diagnostics...), nil
}
