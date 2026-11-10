package format

import "encoding/binary"

type probeMP4Walker struct {
	r     *probeReader
	out   *ProbeInfo
	boxes int
	valid bool
	movie bool
}

func probeMP4(r *probeReader, out *ProbeInfo) bool {
	w := probeMP4Walker{r: r, out: out, valid: true}
	w.walk(0, r.size, 0, "")
	return w.valid && w.movie
}
func (w *probeMP4Walker) walk(start, end int64, depth int, textKey string) {
	if depth > 8 {
		w.valid = false
		return
	}
	for offset := start; offset+8 <= end && w.boxes < probeRecordLimit; w.boxes++ {
		var h [16]byte
		if !w.r.at(h[:8], offset) {
			w.valid = false
			return
		}
		n := uint64(binary.BigEndian.Uint32(h[:4]))
		header := int64(8)
		kind := string(h[4:8])
		if n == 1 {
			if !w.r.at(h[8:], offset+8) {
				w.valid = false
				return
			}
			n = binary.BigEndian.Uint64(h[8:])
			header = 16
		}
		if n == 0 {
			// #nosec G115 -- the loop requires offset+8 <= end.
			n = uint64(end - offset)
		}
		// #nosec G115 -- header is 8 or 16; end-offset is positive.
		if n < uint64(header) || n > uint64(end-offset) {
			w.valid = false
			return
		}
		// #nosec G115 -- n is bounded by the positive int64 end-offset above.
		body, limit := offset+header, offset+int64(n)
		switch kind {
		case "moov", "udta", "ilst":
			if kind == "moov" {
				w.movie = true
			}
			w.walk(body, limit, depth+1, "")
		case "meta":
			if limit-body >= 4 {
				w.walk(body+4, limit, depth+1, "")
			}
		case "\xa9nam", "\xa9ART", "\xa9art", "\xa9alb":
			key := map[string]string{"\xa9nam": "TITLE", "\xa9ART": "ARTIST", "\xa9art": "ARTIST", "\xa9alb": "ALBUM"}[kind]
			w.walk(body, limit, depth+1, key)
		case "data":
			if textKey != "" && limit-body >= 8 && limit-body-8 <= probeTextLimit && w.r.reserveText(limit-body-8) {
				var flags [4]byte
				if w.r.at(flags[:], body) {
					value := make([]byte, limit-body-8)
					if w.r.at(value, body+8) {
						if flags[3] == 2 {
							value = append([]byte{2}, value...)
							probeSetText(&w.out.Metadata, textKey, probeID3Text(value))
						} else {
							probeSetText(&w.out.Metadata, textKey, string(value))
						}
					}
				}
			}
		case "mvhd":
			var b [32]byte
			length := min(int64(len(b)), limit-body)
			if length >= 20 && w.r.at(b[:length], body) {
				if b[0] == 0 {
					ticks := binary.BigEndian.Uint32(b[16:20])
					if ticks != ^uint32(0) {
						w.out.Duration = probeDuration(uint64(ticks), uint64(binary.BigEndian.Uint32(b[12:16])))
					}
				}
				if b[0] == 1 && length >= 32 {
					ticks := binary.BigEndian.Uint64(b[24:32])
					if ticks != ^uint64(0) {
						w.out.Duration = probeDuration(ticks, uint64(binary.BigEndian.Uint32(b[20:24])))
					}
				}
			}
		}
		offset = limit
	}
}
