package soundfont

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

func loadParameters(r io.ReadSeeker, end, size int64, sf *SoundFont, limits loadLimits) error {
	if size > limits.pdta {
		return errors.New("pdta resource limit exceeded")
	}
	tables := map[string][]byte{}
	for {
		pos, e := r.Seek(0, io.SeekCurrent)
		if e != nil {
			return e
		}
		if pos == end {
			break
		}
		c, e := readChunk(r, end)
		if e != nil {
			return e
		}
		if _, ok := tables[c.id]; ok {
			return fmt.Errorf("duplicate table %s", c.id)
		}
		switch c.id {
		case "phdr", "pbag", "pmod", "pgen", "inst", "ibag", "imod", "igen", "shdr":
			tables[c.id], e = chunkBytes(r, c.size)
			if e != nil {
				return e
			}
		}
		if _, e = r.Seek(c.next, io.SeekStart); e != nil {
			return e
		}
	}
	widths := map[string]int{"phdr": 38, "pbag": 4, "pmod": 10, "pgen": 4, "inst": 22, "ibag": 4, "imod": 10, "igen": 4, "shdr": 46}
	for id, w := range widths {
		b := tables[id]
		if len(b) < w || len(b)%w != 0 {
			return fmt.Errorf("invalid %s table length", id)
		}
	}
	// Validate record references before upstream constructors perform any indexing.
	for _, v := range []struct {
		id            string
		width, offset int
		target        string
		targetWidth   int
	}{
		{"phdr", 38, 24, "pbag", 4}, {"inst", 22, 20, "ibag", 4},
		{"pbag", 4, 0, "pgen", 4}, {"ibag", 4, 0, "igen", 4},
		{"pbag", 4, 2, "pmod", 10}, {"ibag", 4, 2, "imod", 10},
	} {
		prev := 0
		b := tables[v.id]
		last := len(tables[v.target])/v.targetWidth - 1
		for p := 0; p < len(b); p += v.width {
			index := int(binary.LittleEndian.Uint16(b[p+v.offset:]))
			if index < prev || index > last {
				return fmt.Errorf("invalid %s reference to %s", v.id, v.target)
			}
			prev = index
		}
	}
	for _, v := range []struct {
		id    string
		op    uint16
		count int
	}{{"pgen", 41, len(tables["inst"])/22 - 1}, {"igen", 53, len(tables["shdr"])/46 - 1}} {
		b := tables[v.id]
		for p := 0; p < len(b)-4; p += 4 {
			if binary.LittleEndian.Uint16(b[p:]) == v.op && int(binary.LittleEndian.Uint16(b[p+2:])) >= v.count {
				return fmt.Errorf("invalid %s object reference", v.id)
			}
		}
	}
	// #nosec G115 -- each table is bounded by the validated 16 MiB pdta resource limit before allocation.
	pi, e := readPresetsFromChunk(bytes.NewReader(tables["phdr"]), int32(len(tables["phdr"])))
	if e != nil {
		return e
	}
	// #nosec G115 -- each table is bounded by the validated 16 MiB pdta resource limit before allocation.
	ii, e := readInstrumentsFromChunk(bytes.NewReader(tables["inst"]), int32(len(tables["inst"])))
	if e != nil {
		return e
	}
	// #nosec G115 -- each table is bounded by the validated 16 MiB pdta resource limit before allocation.
	pb, e := readZonesFromChunk(bytes.NewReader(tables["pbag"]), int32(len(tables["pbag"])))
	if e != nil {
		return e
	}
	// #nosec G115 -- each table is bounded by the validated 16 MiB pdta resource limit before allocation.
	ib, e := readZonesFromChunk(bytes.NewReader(tables["ibag"]), int32(len(tables["ibag"])))
	if e != nil {
		return e
	}
	// #nosec G115 -- each table is bounded by the validated 16 MiB pdta resource limit before allocation.
	pg, e := readGeneratorsFromChunk(bytes.NewReader(tables["pgen"]), int32(len(tables["pgen"])))
	if e != nil {
		return e
	}
	// #nosec G115 -- each table is bounded by the validated 16 MiB pdta resource limit before allocation.
	ig, e := readGeneratorsFromChunk(bytes.NewReader(tables["igen"]), int32(len(tables["igen"])))
	if e != nil {
		return e
	}
	// #nosec G115 -- each table is bounded by the validated 16 MiB pdta resource limit before allocation.
	sf.SampleHeaders, e = readSampleHeadersFromChunk(bytes.NewReader(tables["shdr"]), int32(len(tables["shdr"])))
	if e != nil {
		return e
	}
	iz, e := createZones(ib, ig, readModulators(tables["imod"]))
	if e != nil {
		return e
	}
	sf.Instruments, e = createInstruments(ii, iz, sf.SampleHeaders)
	if e != nil {
		return e
	}
	pz, e := createZones(pb, pg, readModulators(tables["pmod"]))
	if e != nil {
		return e
	}
	sf.Presets, e = createPresets(pi, pz, sf.Instruments)
	if e != nil {
		return e
	}
	for _, i := range sf.Instruments {
		if len(i.Regions) == 0 {
			sf.diagnose("empty_instrument", i.Name)
		}
	}
	for _, p := range sf.Presets {
		if len(p.Regions) == 0 {
			sf.diagnose("empty_preset", p.Name)
		}
	}
	return nil
}
