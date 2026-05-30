package mp4

import (
	"encoding/binary"
	"fmt"
)

// parseALACSampleEntry parses an ALAC sample description entry.
//
// Layout (ISO 14496-12 / QuickTime SoundDescription, version 0):
//
//	[0-3]   entry_size (uint32)
//	[4-7]   "alac"
//	[8-13]  reserved (6 bytes)
//	[14-15] data_reference_index (uint16)
//	[16-17] version (uint16)
//	[18-19] revision_level (uint16)
//	[20-23] vendor (uint32)
//	[24-25] channels (uint16)
//	[26-27] sample_size (uint16)
//	[28-29] compression_id (uint16)
//	[30-31] packet_size (uint16)
//	[32-35] sample_rate (uint32)
//
// For version 1, there's an additional extension_size field at [36-39].
// For version 0, trailing data begins at offset 36.
//
// The ALAC magic cookie is stored as trailing data, optionally prefixed
// by a 4-byte "alac" tag and a 4-byte version/flags field.
func parseALACSampleEntry(data []byte) (cookie []byte, channels uint16, sampleSize uint16, err error) {
	if len(data) < 36 {
		return nil, 0, 0, fmt.Errorf("alac sample entry too small: %d", len(data))
	}

	channels = binary.BigEndian.Uint16(data[24:26])
	sampleSize = binary.BigEndian.Uint16(data[26:28])

	entrySize := binary.BigEndian.Uint32(data[0:4])
	version := binary.BigEndian.Uint16(data[16:18])

	trailingStart := 36
	if version > 0 {
		// Version 1: extension_size follows sample_rate
		if len(data) < 40 {
			return nil, channels, sampleSize, fmt.Errorf("alac sample entry v1 too small")
		}
		extSize := binary.BigEndian.Uint32(data[36:40])
		if extSize > 0 {
			// The extension data IS the magic cookie (possibly prefixed by "alac")
			trailingStart = 40
			extensionData := data[trailingStart:]
			if len(extensionData) > int(extSize) {
				extensionData = extensionData[:extSize]
			}

			// Strip optional "alac" prefix and version/flags
			var raw []byte
			if len(extensionData) >= 4 && string(extensionData[:4]) == "alac" {
				raw = extensionData[4:]
			} else {
				raw = extensionData
			}
			// Strip optional 4-byte version/flags
			if len(raw) >= 4 && raw[0] == 0 && raw[1] == 0 && raw[2] == 0 && raw[3] == 0 {
				raw = raw[4:]
			}
			return raw, channels, sampleSize, nil
		}
		// extSize == 0: fall through to sub-box scanning
		trailingStart = 40
	}

	// Scan for sub-boxes starting at trailingStart (v0) or after extension
	// (v1 with extSize == 0).
	for uint32(trailingStart) < entrySize && trailingStart+8 <= len(data) {
		subSize := binary.BigEndian.Uint32(data[trailingStart : trailingStart+4])
		if subSize < 8 || subSize > entrySize {
			trailingStart++
			continue
		}
		subType := string(data[trailingStart+4 : trailingStart+8])
		if subType == "alac" {
			end := trailingStart + int(subSize)
			if end > len(data) {
				end = len(data)
			}
			raw := data[trailingStart+8 : end]
			// Strip optional 4-byte version/flags
			if len(raw) >= 4 && raw[0] == 0 && raw[1] == 0 && raw[2] == 0 && raw[3] == 0 {
				raw = raw[4:]
			}
			return raw, channels, sampleSize, nil
		}
		trailingStart += int(subSize)
	}

	return nil, channels, sampleSize, fmt.Errorf("no alac magic cookie found")
}