package format

import (
	"bytes"
	"encoding/binary"
	"io"
)

// oggProbePacket traverses lacing tables, skipping packet payloads by offset.
// The page limit also bounds work for a corrupt, unterminated header packet.
type oggProbePacket struct {
	r                            *probeReader
	next, payload                int64
	serial                       uint32
	laces                        [255]byte
	segments, index, left, pages int
	ended                        bool
}

func (p *oggProbePacket) reserveText(n int64) bool { return p.r.reserveText(n) }
func (p *oggProbePacket) page() bool {
	if p.pages >= probeRecordLimit {
		return false
	}
	p.pages++
	var h [27]byte
	if !p.r.at(h[:], p.next) || string(h[:4]) != "OggS" || h[4] != 0 {
		return false
	}
	serial := binary.LittleEndian.Uint32(h[14:18])
	if p.pages == 1 {
		p.serial = serial
	} else if serial != p.serial {
		return false
	}
	p.segments = int(h[26])
	if !p.r.at(p.laces[:p.segments], p.next+27) {
		return false
	}
	p.payload = p.next + 27 + int64(p.segments)
	p.next = p.payload
	for _, n := range p.laces[:p.segments] {
		p.next += int64(n)
	}
	if p.next > p.r.size {
		return false
	}
	p.index = 0
	return true
}
func (p *oggProbePacket) segment() bool {
	if p.ended {
		return false
	}
	for p.left == 0 {
		if p.index >= p.segments && !p.page() {
			return false
		}
		if p.index >= p.segments {
			return false
		}
		p.left = int(p.laces[p.index])
		p.index++
		if p.left == 0 {
			p.ended = true
			return false
		}
	}
	return true
}
func (p *oggProbePacket) consume(n int) {
	p.left -= n
	p.payload += int64(n)
	if p.left == 0 && p.laces[p.index-1] < 255 {
		p.ended = true
	}
}
func (p *oggProbePacket) Read(b []byte) (int, error) {
	n := 0
	for len(b) > 0 && p.segment() {
		take := min(len(b), p.left)
		if !p.r.at(b[:take], p.payload) {
			return n, io.EOF
		}
		p.consume(take)
		b = b[take:]
		n += take
	}
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}
func (p *oggProbePacket) skip(n int64) bool {
	for n > 0 {
		if !p.segment() {
			return false
		}
		take := min(n, int64(p.left))
		p.consume(int(take))
		n -= take
	}
	return p.r.ctx.Err() == nil
}
func (p *oggProbePacket) finish() bool {
	for p.segment() {
		p.consume(p.left)
	}
	if !p.ended {
		return false
	}
	p.ended = false
	return true
}
func probeOgg(r *probeReader, out *ProbeInfo) bool {
	p := &oggProbePacket{r: r}
	var h [19]byte
	if _, err := io.ReadFull(p, h[:]); err != nil {
		return false
	}
	var rate, preSkip uint64
	var prefix string
	switch {
	case string(h[:8]) == "OpusHead":
		rate = 48000
		preSkip = uint64(binary.LittleEndian.Uint16(h[10:12]))
		prefix = "OpusTags"
		out.Format = "OPUS"
	case string(h[:7]) == "\x01vorbis":
		rate = uint64(binary.LittleEndian.Uint32(h[12:16]))
		prefix = "\x03vorbis"
	default:
		return false
	}
	if rate == 0 || !p.finish() {
		return false
	}
	var marker [8]byte
	if _, err := io.ReadFull(p, marker[:len(prefix)]); err == nil && string(marker[:len(prefix)]) == prefix {
		probeComments(p, &out.Metadata)
	}
	// An Ogg page is at most 65307 bytes. Include two pages so a terminal
	// incomplete capture cannot hide a preceding complete page.
	const tailLimit = 2 * (27 + 255 + 255*255)
	start := max(int64(0), r.size-tailLimit)
	tail := make([]byte, r.size-start)
	if !r.at(tail, start) {
		return true
	}
	var last uint64
	found := false
	for i := 0; i+27 <= len(tail); {
		at := bytes.Index(tail[i:], []byte("OggS"))
		if at < 0 {
			break
		}
		i += at
		if i+27 > len(tail) {
			break
		}
		h := tail[i:]
		segments := int(h[26])
		if h[4] != 0 || 27+segments > len(h) {
			i++
			continue
		}
		length := 27 + segments
		for _, n := range h[27 : 27+segments] {
			length += int(n)
		}
		if length > len(h) {
			i++
			continue
		}
		// A different logical stream may be a chained program. Reporting only
		// the first stream would be an incomplete duration; leave it unknown.
		if binary.LittleEndian.Uint32(h[14:18]) != p.serial {
			return true
		}
		granule := binary.LittleEndian.Uint64(h[6:14])
		if granule != ^uint64(0) {
			last = granule
			found = true
		}
		i += length
	}
	if found && last >= preSkip {
		out.Duration = probeDuration(last-preSkip, rate)
	}
	return true
}
