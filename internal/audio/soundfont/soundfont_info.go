package soundfont

import (
	"encoding/binary"
	"errors"
	"io"
	"strings"
)

// Info contains descriptive metadata from the SF2 INFO list.
type Info struct {
	Version                                                         Version
	TargetSoundEngine, BankName, RomName                            string
	RomVersion                                                      Version
	CreationDate, Author, TargetProduct, Copyright, Comments, Tools string
}

func loadInfo(r io.ReadSeeker, end int64, sf *SoundFont) error {
	sf.Info = &Info{}
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
		// INFO is metadata, bounded separately from sample memory.
		if c.size > 1<<20 {
			return errors.New("INFO resource limit exceeded")
		}
		data, e := chunkBytes(r, c.size)
		if e != nil {
			return e
		}
		text := strings.TrimRight(string(data), "\x00")
		switch c.id {
		case "ifil", "iver":
			if len(data) != 4 {
				return errors.New("invalid version record")
			}
			// #nosec G115 -- SF2 version fields are 16-bit wire values retained as the inherited signed representation.
			v := Version{int16(binary.LittleEndian.Uint16(data)), int16(binary.LittleEndian.Uint16(data[2:]))}
			if c.id == "ifil" {
				sf.Info.Version = v
			} else {
				sf.Info.RomVersion = v
			}
		case "isng":
			sf.Info.TargetSoundEngine = text
		case "INAM":
			sf.Info.BankName = text
		case "irom":
			sf.Info.RomName = text
		case "ICRD":
			sf.Info.CreationDate = text
		case "IENG":
			sf.Info.Author = text
		case "IPRD":
			sf.Info.TargetProduct = text
		case "ICOP":
			sf.Info.Copyright = text
		case "ICMT":
			sf.Info.Comments = text
		case "ISFT":
			sf.Info.Tools = text
		}
		if _, e = r.Seek(c.next, io.SeekStart); e != nil {
			return e
		}
	}
	return nil
}
