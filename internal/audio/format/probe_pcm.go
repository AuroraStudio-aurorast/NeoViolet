package format

import "encoding/binary"

func probeWAV(r *probeReader, out *ProbeInfo) bool {
	var header [12]byte
	if !r.at(header[:], 0) || string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return false
	}
	end := min(r.size, int64(binary.LittleEndian.Uint32(header[4:8]))+8)
	var rate uint32
	var bytes uint64
	dataFound := false
	for offset, count := int64(12), 0; offset+8 <= end && count < probeRecordLimit; count++ {
		var h [8]byte
		if !r.at(h[:], offset) {
			return false
		}
		n := int64(binary.LittleEndian.Uint32(h[4:]))
		offset += 8
		if n > end-offset {
			return false
		}
		switch string(h[:4]) {
		case "fmt ":
			var f [16]byte
			if n >= 16 && r.at(f[:], offset) {
				rate = binary.LittleEndian.Uint32(f[8:12])
			}
		case "data":
			dataFound = true
			bytes += uint64(binary.LittleEndian.Uint32(h[4:]))
		}
		offset += n + (n & 1)
	}
	out.Duration = probeDuration(bytes, uint64(rate))
	return rate > 0 && dataFound
}
func probeFLAC(r *probeReader, out *ProbeInfo) bool {
	var magic [4]byte
	if !r.at(magic[:], 0) || string(magic[:]) != "fLaC" {
		return false
	}
	for offset, count := int64(4), 0; count < probeRecordLimit; count++ {
		var h [4]byte
		if !r.at(h[:], offset) {
			return false
		}
		n := int64(h[1])<<16 | int64(h[2])<<8 | int64(h[3])
		offset += 4
		if n > r.size-offset {
			return false
		}
		switch h[0] & 127 {
		case 0:
			var info [34]byte
			if n == 34 && r.at(info[:], offset) {
				bits := binary.BigEndian.Uint64(info[10:18])
				out.Duration = probeDuration(bits&0xfffffffff, bits>>44)
			}
		case 4:
			probeComments(&probeCursor{r: r, pos: offset, end: offset + n}, &out.Metadata)
		}
		offset += n
		if h[0]&128 != 0 {
			return true
		}
	}
	return false
}
