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
		ftype := string(buf[8:12])
		knownTypes := map[string]bool{
			"M4A ": true, "mp42": true, "isom": true, "M4B": true,
		}
		if knownTypes[ftype] {
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