package soundfont

import (
	"encoding/binary"
	"errors"
	"io"
)

type riffChunk struct {
	id              string
	size, end, next int64
}

func readChunk(r io.ReadSeeker, end int64) (riffChunk, error) {
	pos, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return riffChunk{}, err
	}
	if end-pos < 8 {
		return riffChunk{}, io.ErrUnexpectedEOF
	}
	var h [8]byte
	if _, err = io.ReadFull(r, h[:]); err != nil {
		return riffChunk{}, err
	}
	n := int64(binary.LittleEndian.Uint32(h[4:]))
	if n > end-pos-8 || n+n%2 > end-pos-8 {
		return riffChunk{}, errors.New("chunk exceeds enclosing boundary")
	}
	return riffChunk{string(h[:4]), n, pos + 8 + n, pos + 8 + n + n%2}, nil
}
func chunkBytes(r io.Reader, n int64) ([]byte, error) {
	b := make([]byte, int(n))
	_, err := io.ReadFull(r, b)
	return b, err
}
