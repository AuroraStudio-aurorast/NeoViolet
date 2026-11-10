package format

import (
	"encoding/binary"
	"io"
	"strings"
	"unicode/utf16"
)

func probeSynchsafe(b []byte) int64 {
	return int64(b[0]&127)<<21 | int64(b[1]&127)<<14 | int64(b[2]&127)<<7 | int64(b[3]&127)
}
func probeID3(r *probeReader, m *Metadata) int64 {
	var h [10]byte
	if !r.at(h[:], 0) || string(h[:3]) != "ID3" {
		return 0
	}
	end := probeSynchsafe(h[6:]) + 10
	audio := end
	if h[3] == 4 && h[5]&16 != 0 {
		audio += 10
	}
	if end > r.size || h[3] < 2 || h[3] > 4 {
		return audio
	}
	var input probeInput = &probeCursor{r: r, pos: 10, end: end}
	// v2.3 frame sizes count decoded bytes. Finding the physical end of an
	// unsynchronized cover requires streaming its bytes, bounded to 3 MiB of
	// input; no cover bytes are retained. Ordinary tags still seek.
	if h[3] <= 3 && h[5]&128 != 0 {
		// Leave the remaining read budget for MPEG and trailing tag headers.
		input = &probeUnsyncReader{input: &probeCursor{r: r, pos: 10, end: min(end, 10+(3<<20))}}
	}
	if h[3] == 2 && h[5]&64 != 0 {
		return audio
	}
	if h[5]&64 != 0 && h[3] >= 3 {
		var ext [4]byte
		if _, err := io.ReadFull(input, ext[:]); err != nil {
			return audio
		}
		n := int64(binary.BigEndian.Uint32(ext[:]))
		if h[3] == 4 {
			n = probeSynchsafe(ext[:]) - 4
		}
		if n < 0 || !input.skip(n) {
			return audio
		}
	}
	for count := 0; count < probeRecordLimit; count++ {
		var frame [10]byte
		hs := 6
		if h[3] >= 3 {
			hs = 10
		}
		if _, err := io.ReadFull(input, frame[:hs]); err != nil || frame[0] == 0 {
			return audio
		}
		key := string(frame[:4])
		n := int64(binary.BigEndian.Uint32(frame[4:8]))
		flags := frame[9]
		switch h[3] {
		case 2:
			key = string(frame[:3])
			n = int64(frame[3])<<16 | int64(frame[4])<<8 | int64(frame[5])
			flags = 0
		case 4:
			n = probeSynchsafe(frame[4:8])
		}
		if n <= 0 || n > end-10 {
			return audio
		}
		unsupported := flags&0xe0 != 0
		if h[3] == 4 {
			unsupported = flags&0x4d != 0
		}
		if probeTextKey(key) && n <= probeTextLimit && !unsupported && input.reserveText(n) {
			data := make([]byte, int(n))
			if _, err := io.ReadFull(input, data); err != nil {
				return audio
			}
			if h[3] == 4 && (flags&2 != 0 || h[5]&128 != 0) {
				data = probeUnstuff(data)
			}
			probeSetText(m, key, probeID3Text(data))
		} else if !input.skip(n) {
			return audio
		}
	}
	return audio
}

// probeUnsyncReader removes inserted zeros with constant memory. It must not
// seek over an unsynchronized frame: declared lengths refer to decoded bytes.
type probeUnsyncReader struct {
	input    probeInput
	buffer   [4096]byte
	discard  [4096]byte
	pos, end int
	ff       bool
}

func (r *probeUnsyncReader) reserveText(n int64) bool { return r.input.reserveText(n) }
func (r *probeUnsyncReader) Read(out []byte) (int, error) {
	n := 0
	for n < len(out) {
		if r.pos == r.end {
			count, err := r.input.Read(r.buffer[:])
			r.pos = 0
			r.end = count
			if count == 0 {
				return n, err
			}
		}
		b := r.buffer[r.pos]
		r.pos++
		if r.ff && b == 0 {
			r.ff = false
			continue
		}
		r.ff = b == 255
		out[n] = b
		n++
	}
	return n, nil
}
func (r *probeUnsyncReader) skip(n int64) bool {
	for n > 0 {
		take := min(n, int64(len(r.discard)))
		if _, err := io.ReadFull(r, r.discard[:take]); err != nil {
			return false
		}
		n -= take
	}
	return true
}
func probeUnstuff(data []byte) []byte {
	out := data[:0]
	ff := false
	for _, b := range data {
		if ff && b == 0 {
			ff = false
			continue
		}
		ff = b == 255
		out = append(out, b)
	}
	return out
}
func probeID3Text(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	encoding := data[0]
	data = data[1:]
	switch encoding {
	case 0:
		runes := make([]rune, len(data))
		for i, b := range data {
			runes[i] = rune(b)
		}
		return string(runes)
	case 1, 2:
		little := false
		if len(data) >= 2 {
			if data[0] == 255 && data[1] == 254 {
				little = true
				data = data[2:]
			} else if data[0] == 254 && data[1] == 255 {
				data = data[2:]
			}
		}
		words := make([]uint16, len(data)/2)
		for i := range words {
			if little {
				words[i] = binary.LittleEndian.Uint16(data[i*2:])
			} else {
				words[i] = binary.BigEndian.Uint16(data[i*2:])
			}
		}
		return string(utf16.Decode(words))
	case 3:
		return string(data)
	}
	return ""
}
func probeMP3(r *probeReader, offset int64, out *ProbeInfo) bool {
	// Inspect only the first frame. Without a frame-count index, VBR duration is unknown.
	var b [2048]byte
	n, _ := r.readAt(b[:], offset)
	if n < 4 {
		return false
	}
	if b[0] != 255 || b[1]&0xe0 != 0xe0 || b[1]&6 != 2 {
		return false
	}
	version := (b[1] >> 3) & 3
	rateIndex := (b[2] >> 2) & 3
	if version == 1 || rateIndex == 3 || b[2]>>4 == 0 || b[2]>>4 == 15 {
		return false
	}
	rate := [3]uint64{44100, 48000, 32000}[rateIndex]
	samples := uint64(1152)
	side := 32
	if b[3]>>6 == 3 {
		side = 17
	}
	if version != 3 {
		rate /= 2
		samples = 576
		side = 17
		if b[3]>>6 == 3 {
			side = 9
		}
	}
	if version == 0 {
		rate /= 2
	}
	start := 4 + side
	if b[1]&1 == 0 {
		start += 2
	}
	var frames uint32
	if start+12 <= n && (string(b[start:start+4]) == "Xing" || string(b[start:start+4]) == "Info") && binary.BigEndian.Uint32(b[start+4:])&1 != 0 {
		frames = binary.BigEndian.Uint32(b[start+8:])
	}
	if frames == 0 && n >= 54 && string(b[36:40]) == "VBRI" {
		frames = binary.BigEndian.Uint32(b[50:54])
	}
	out.Duration = probeDuration(uint64(frames)*samples, rate)
	return true
}
func probeTrailingTags(r *probeReader, m *Metadata) {
	end := r.size
	var id3 [128]byte
	if r.at(id3[:], end-128) && string(id3[:3]) == "TAG" {
		if m.Title == "" {
			m.Title = strings.TrimRight(string(id3[3:33]), "\x00 ")
		}
		if m.Artist == "" {
			m.Artist = strings.TrimRight(string(id3[33:63]), "\x00 ")
		}
		if m.Album == "" {
			m.Album = strings.TrimRight(string(id3[63:93]), "\x00 ")
		}
		end -= 128
	}
	var footer [32]byte
	if !r.at(footer[:], end-32) || string(footer[:8]) != "APETAGEX" || binary.LittleEndian.Uint32(footer[8:]) != 2000 {
		return
	}
	size := int64(binary.LittleEndian.Uint32(footer[12:]))
	count := binary.LittleEndian.Uint32(footer[16:])
	offset := end - size
	if size < 32 || offset < 0 {
		return
	}
	for range min(count, probeRecordLimit) {
		var h [8]byte
		if offset+8 > end-32 || !r.at(h[:], offset) {
			return
		}
		n := int64(binary.LittleEndian.Uint32(h[:]))
		flags := binary.LittleEndian.Uint32(h[4:])
		offset += 8
		var key [256]byte
		k := 0
		for ; k < len(key); k++ {
			if offset >= end-32 || !r.at(key[k:k+1], offset) {
				return
			}
			offset++
			if key[k] == 0 {
				break
			}
		}
		if k == len(key) || n > end-32-offset {
			return
		}
		if flags&6 == 0 && probeTextKey(string(key[:k])) && n <= probeTextLimit && r.reserveText(n) {
			value := make([]byte, int(n))
			if r.at(value, offset) {
				var fallback Metadata
				probeSetText(&fallback, string(key[:k]), string(value))
				// Match playback's ID3 precedence; APE tags only fill gaps.
				if m.Title == "" {
					m.Title = fallback.Title
				}
				if m.Artist == "" {
					m.Artist = fallback.Artist
				}
				if m.Album == "" {
					m.Album = fallback.Album
				}
			}
		}
		offset += n
	}
}
