package format

import (
	"io"

	"github.com/gopxl/beep/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/alacstream"
)

func init() {
	registerFormat(formatHandler{
		extensions: []string{".m4a"},
		decodeSeeker: func(r io.ReadSeeker) (beep.StreamSeekCloser, beep.Format, error) {
			return alacstream.DecodeM4A(r)
		},
	})
	registerFTYPProbe(func(buf []byte, n int) (string, bool) {
		if n < 8 {
			return "", false
		}
		knownTypes := map[string]bool{
			//nolint:gocritic // "M4A " is the literal 4-byte MP4 brand with a trailing space.
			"M4A ": true, "mp42": true, "isom": true, "M4B": true,
		}
		if knownTypes[string(buf[8:12])] {
			return ".m4a", true
		}
		if n >= 16 {
			ftype2 := string(buf[12:16])
			if ftype2 == "M4A " || ftype2 == "M4B" {
				return ".m4a", true
			}
		}
		return "", false
	})
}
