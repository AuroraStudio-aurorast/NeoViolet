package format

import (
	"io"

	"github.com/gopxl/beep/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/opusstream"
)

func init() {
	registerFormat(formatHandler{
		extensions: []string{".opus"},
		decodeSeeker: func(r io.ReadSeeker) (beep.StreamSeekCloser, beep.Format, error) {
			return opusstream.DecodeOGG(r)
		},
	})
	registerOGGProbe(func(buf []byte, n int) (string, bool) {
		if n >= 37 && string(buf[28:36]) == "OpusHead" {
			return ".opus", true
		}
		return "", false
	})
}