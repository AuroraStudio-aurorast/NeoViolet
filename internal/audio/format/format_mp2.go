package format

import (
	"io"

	"github.com/gopxl/beep/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/mp2stream"
)

func init() {
	registerFormat(formatHandler{
		extensions: []string{".mp2"},
		decodeSeeker: func(r io.ReadSeeker) (beep.StreamSeekCloser, beep.Format, error) {
			return mp2stream.DecodeMP2(r)
		},
	})
	registerMPEGProbe(func(buf []byte, n int) (string, bool) {
		if n >= 2 {
			layer := (buf[1] >> 1) & 0x03
			if layer == 2 {
				return ".mp2", true
			}
		}
		return "", false
	})
	registerID3Probe(func(buf []byte, n int) (string, bool) {
		if n < 10 {
			return "", false
		}
		pastID3 := id3AudioOffset(buf)
		if pastID3+2 > n {
			return "", false
		}
		if buf[pastID3] == 0xFF && (buf[pastID3+1]&0xE0) == 0xE0 {
			layer := (buf[pastID3+1] >> 1) & 0x03
			if layer == 2 {
				return ".mp2", true
			}
		}
		return "", false
	})
}
