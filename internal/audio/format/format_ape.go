package format

import (
	"fmt"
	"io"
	"os"

	"github.com/gopxl/beep/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/apestream"
)

func init() {
	// Register magic probe for bare APE files (starts with "MAC ").
	registerMagicProbe(func(buf []byte, n int) (string, bool) {
		if n >= 4 && string(buf[0:4]) == "MAC " {
			return ".ape", true
		}
		return "", false
	})

	// Register ID3 probe for APE files preceded by an ID3v2 tag.
	registerID3Probe(func(buf []byte, n int) (string, bool) {
		if n < 10 {
			return "", false
		}
		tagSize := int(buf[6])<<21 | int(buf[7])<<14 | int(buf[8])<<7 | int(buf[9])
		pastID3 := 10 + tagSize
		if pastID3+4 <= n && string(buf[pastID3:pastID3+4]) == "MAC " {
			return ".ape", true
		}
		return "", false
	})

	registerFormat(formatHandler{
		extensions: []string{".ape"},
		decodeSeeker: func(r io.ReadSeeker) (beep.StreamSeekCloser, beep.Format, error) {
			file, ok := r.(*os.File)
			if !ok {
				return nil, beep.Format{}, fmt.Errorf("ape: decode requires *os.File")
			}
			return apestream.Decode(file, file.Name())
		},
	})
}