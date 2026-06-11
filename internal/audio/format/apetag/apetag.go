// Package apetag reads APEv2 metadata tags from audio files.
//
// APEv2 binary format (see https://wiki.hydrogenaudio.org/index.php?title=APEv2_specification):
//
// Footer (32 bytes) at end of tag data:
//
//	[0:8]   "APETAGEX" magic
//	[8:12]  Version (int32 LE) — must be 2000
//	[12:16] Tag Size (int32 LE) — includes footer + items, excludes header
//	[16:20] Item Count (int32 LE)
//	[20:24] Tag Flags (int32 LE) — bit 31 = header present
//	[24:32] Reserved (8 bytes, zeros)
//
// Header (32 bytes, optional) — same format, placed before items if present.
//
// Tag Item (variable length):
//
//	[0:4]   Value Size (int32 LE)
//	[4:8]   Item Flags (int32 LE) — bit 1=binary, bit 0=cover art
//	[8:]    Key (ASCII, null-terminated) + Value (value_size bytes)
package apetag

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Tags holds extracted APEv2 metadata fields.
type Tags struct {
	Title  string
	Artist string
	Album  string

	// CoverData holds raw image bytes (JPEG or PNG) if a cover art
	// item was found. May be nil.
	CoverData []byte
}

var (
	errNoTags    = errors.New("apetag: no APEv2 tags found")
	errBadMagic  = errors.New("apetag: bad magic (not APETAGEX)")
	errBadFooter = errors.New("apetag: corrupt or truncated footer")
)

// ParseFile opens the given file and parses any embedded APEv2 tags.
func ParseFile(path string) (*Tags, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("apetag open: %w", err)
	}
	defer f.Close()
	return Parse(f)
}

// Parse reads APEv2 tags from an io.ReadSeeker positioned at the start.
func Parse(r io.ReadSeeker) (*Tags, error) {
	// Determine file/stream size.
	end, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, fmt.Errorf("apetag seek end: %w", err)
	}
	if end < 32 {
		return nil, errNoTags
	}

	// APEv2 footer can be at end-32, or if an ID3v1 tag follows, at end-160.
	footerPos := findFooterPosition(r, end)
	if footerPos < 0 {
		return nil, errNoTags
	}

	// Read footer.
	footer := make([]byte, 32)
	if _, err := r.Seek(footerPos, io.SeekStart); err != nil {
		return nil, fmt.Errorf("apetag seek footer: %w", err)
	}
	if _, err := io.ReadFull(r, footer); err != nil {
		return nil, fmt.Errorf("apetag read footer: %w", err)
	}

	tagSize, itemCount, _, err := parseFooter(footer)
	if err != nil {
		return nil, err
	}

	// Calculate where the item data begins.
	// tagSize includes footer + items but NOT the header.
	// Everything from itemsEnd - tagSize to itemsEnd is the tag data.
	itemsEnd := footerPos + 32 // past-the-end of the footer
	itemsStart := itemsEnd - int64(tagSize)
	// tagSize excludes the header (by spec), so itemsStart already points
	// past the header if one is present. No adjustment needed.
	if itemsStart < 0 {
		return nil, fmt.Errorf("apetag: invalid tag size %d", tagSize)
	}

	// Read all tag items.
	return readItems(r, itemsStart, itemsEnd, int(itemCount))
}

// findFooterPosition locates the APEv2 footer, accounting for an optional
// ID3v1 tag (128 bytes, "TAG" magic) that may follow the APEv2 footer.
// Returns -1 if no footer is found.
func findFooterPosition(r io.ReadSeeker, fileSize int64) int64 {
	readAt := func(pos int64, buf []byte) bool {
		if _, err := r.Seek(pos, io.SeekStart); err != nil {
			return false
		}
		_, err := io.ReadFull(r, buf)
		return err == nil
	}

	// Check at end-32 first.
	if fileSize >= 32 {
		var magic [8]byte
		if readAt(fileSize-32, magic[:]) && string(magic[:]) == "APETAGEX" {
			return fileSize - 32
		}
	}

	// Check at end-160 (accounting for 128-byte ID3v1 tag).
	if fileSize >= 160 {
		var magic [8]byte
		if readAt(fileSize-160, magic[:]) && string(magic[:]) == "APETAGEX" {
			// Verify ID3v1 is actually present at end-128.
			var id3 [3]byte
			if readAt(fileSize-128, id3[:]) && string(id3[:]) == "TAG" {
				return fileSize - 160
			}
		}
	}

	return -1
}

// parseFooter validates and extracts fields from a 32-byte APEv2 footer.
func parseFooter(footer []byte) (tagSize uint32, itemCount uint32, hasHeader bool, err error) {
	if string(footer[0:8]) != "APETAGEX" {
		return 0, 0, false, errBadMagic
	}

	version := binary.LittleEndian.Uint32(footer[8:12])
	if version != 2000 {
		return 0, 0, false, fmt.Errorf("apetag: unsupported version %d", version)
	}

	tagSize = binary.LittleEndian.Uint32(footer[12:16])
	if tagSize < 32 {
		return 0, 0, false, errBadFooter
	}

	itemCount = binary.LittleEndian.Uint32(footer[16:20])
	flags := binary.LittleEndian.Uint32(footer[20:24])
	hasHeader = (flags & 0x80000000) != 0

	// Reserved bytes must be zero.
	for i := 24; i < 32; i++ {
		if footer[i] != 0 {
			return 0, 0, false, errBadFooter
		}
	}

	return
}

// readItems reads itemCount tag items from the given byte range and
// returns the extracted fields as a *Tags.
func readItems(r io.ReadSeeker, start, end int64, count int) (*Tags, error) {
	if count <= 0 {
		return &Tags{}, nil
	}

	// Seek to start and read the entire item data block at once.
	if _, err := r.Seek(start, io.SeekStart); err != nil {
		return nil, fmt.Errorf("apetag seek items: %w", err)
	}
	buf := make([]byte, end-start)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("apetag read items: %w", err)
	}

	tags := &Tags{}
	off := 0
	for i := 0; i < count; i++ {
		// Need at least 8 bytes (size + flags) + 1 (null terminator for key).
		if off+9 > len(buf) {
			break
		}

		valueSize := int(binary.LittleEndian.Uint32(buf[off : off+4]))
		_ = binary.LittleEndian.Uint32(buf[off+4 : off+8]) // itemFlags
		off += 8

		// Find null terminator for the key (max 256 bytes).
		keyEnd := off
		for keyEnd < off+256 && keyEnd < len(buf) && buf[keyEnd] != 0 {
			keyEnd++
		}
		if keyEnd >= len(buf) || buf[keyEnd] != 0 {
			break // corrupt or truncated
		}
		key := string(buf[off:keyEnd])
		off = keyEnd + 1 // skip null terminator

		// Read value.
		if off+valueSize > len(buf) {
			break // truncated
		}
		value := buf[off : off+valueSize]
		off += valueSize

		// Map known keys (case-insensitive).
		switch {
		case strings.EqualFold(key, "TITLE"):
			tags.Title = string(value)

		case strings.EqualFold(key, "ARTIST"):
			tags.Artist = string(value)

		case strings.EqualFold(key, "ALBUM"):
			tags.Album = string(value)

		case strings.EqualFold(key, "COVER ART (FRONT)"):
			tags.CoverData = extractCoverArt(value)
		}
	}

	return tags, nil
}

// extractCoverArt extracts image data from an APEv2 cover art value.
// The value format is: "filename\0" + raw image bytes.
func extractCoverArt(value []byte) []byte {
	nullIdx := bytes.IndexByte(value, 0)
	if nullIdx < 0 || nullIdx+1 >= len(value) {
		return nil
	}
	return value[nullIdx+1:]
}