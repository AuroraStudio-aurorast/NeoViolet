package miditest

import (
	"bytes"
	"encoding/binary"
)

// SMF wraps raw track bytes in an SMF header. Track bytes include delta times.
func SMF(format, division uint16, tracks ...[]byte) []byte {
	b := new(bytes.Buffer)
	b.WriteString("MThd")
	writeBE(b, uint32(6))
	writeBE(b, format)
	// #nosec G115 -- test fixture encodes explicitly supplied in-memory tracks, not untrusted declared lengths.
	writeBE(b, uint16(len(tracks)))
	writeBE(b, division)
	for _, tr := range tracks {
		b.WriteString("MTrk")
		// #nosec G115 -- test fixture encodes explicitly supplied in-memory tracks, not untrusted declared lengths.
		writeBE(b, uint32(len(tr)))
		b.Write(tr)
	}
	return b.Bytes()
}

// VLQ encodes a non-negative SMF delta or length (up to 28 bits).
func VLQ(n uint32) []byte {
	v := []byte{byte(n & 127)}
	for n >>= 7; n > 0; n >>= 7 {
		v = append([]byte{byte(n&127) | 128}, v...)
	}
	return v
}

// Event prefixes a message with its delta ticks.
func Event(delta uint32, message ...byte) []byte { return append(VLQ(delta), message...) }

func writeBE(b *bytes.Buffer, v any) {
	if err := binary.Write(b, binary.BigEndian, v); err != nil {
		panic(err)
	}
}
