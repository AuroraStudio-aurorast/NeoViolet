// Package mp4 provides a minimal MP4/M4A demuxer for extracting ALAC audio.
package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
)

// SampleTable holds the parsed sample table for a single audio track.
type SampleTable struct {
	SampleOffsets      []uint64 // byte offset of each sample in the file
	SampleSizes        []uint32 // size of each sample
	SampleCount        uint32
	DurationPerSample  uint32 // in time-scale units (from stts)
	TimeScale          uint32 // timescale from mdhd box
	SampleRate         uint32
}

// ALACTrack contains all info needed to decode an ALAC stream.
type ALACTrack struct {
	SampleTable
	MagicCookie   []byte // the ALAC magic cookie (from stsd)
	Channels      uint16
	SampleSize    uint16 // bits
}

// Demuxer parses an MP4 file to extract ALAC audio tracks.
type Demuxer struct {
	r io.ReadSeeker
}

// NewDemuxer creates a new MP4 demuxer for the given file.
func NewDemuxer(r io.ReadSeeker) *Demuxer {
	return &Demuxer{r: r}
}

// FindALACTrack parses the MP4 file and returns ALAC track info.
func (d *Demuxer) FindALACTrack() (*ALACTrack, error) {
	// Find moov box
	moovOffset, err := findBoxAny(d.r, 0, "moov")
	if err != nil {
		return nil, fmt.Errorf("moov not found: %w", err)
	}

	// Find the stsd box inside moov to get the ALAC magic cookie
	cookie, channels, sampleSize, err := d.findALACInStsd(moovOffset)
	if err != nil {
		return nil, fmt.Errorf("ALAC stsd not found: %w", err)
	}

	// Parse sample table
	stbl, err := d.parseSampleTable(moovOffset)
	if err != nil {
		return nil, fmt.Errorf("sample table: %w", err)
	}

	return &ALACTrack{
		SampleTable: *stbl,
		MagicCookie: cookie,
		Channels:    channels,
		SampleSize:  sampleSize,
	}, nil
}

// ---------------------------------------------------------------------------
// Box iteration
// ---------------------------------------------------------------------------

var containerBoxes = map[string]bool{
	"moov": true, "trak": true, "mdia": true, "minf": true,
	"stbl": true, "udta": true, "moof": true, "traf": true,
}

func isContainer(boxType string) bool {
	return containerBoxes[boxType]
}

// readBoxHeader reads an MP4 box header at the current read position.
func readBoxHeader(r io.Reader) (size uint64, boxType string, err error) {
	var hdr [8]byte
	_, err = io.ReadFull(r, hdr[:])
	if err != nil {
		return 0, "", err
	}
	size32 := binary.BigEndian.Uint32(hdr[0:4])
	boxType = string(hdr[4:8])
	if size32 == 1 {
		var ext [8]byte
		_, err = io.ReadFull(r, ext[:])
		if err != nil {
			return 0, "", err
		}
		size = binary.BigEndian.Uint64(ext[:])
	} else {
		size = uint64(size32)
	}
	return size, boxType, nil
}

// readBoxHeaderAt reads a box header at a specific file offset.
func readBoxHeaderAt(r io.ReadSeeker, offset int64) (uint64, string, error) {
	if _, err := r.Seek(offset, io.SeekStart); err != nil {
		return 0, "", err
	}
	return readBoxHeader(r)
}

// readBoxPayload reads the data of a box (skipping the 8-byte header).
func readBoxPayload(r io.ReadSeeker, boxOffset int64) ([]byte, error) {
	size, _, err := readBoxHeaderAt(r, boxOffset)
	if err != nil {
		return nil, err
	}
	payloadLen := size - 8
	if payloadLen == 0 {
		return nil, nil
	}
	data := make([]byte, payloadLen)
	_, err = io.ReadFull(r, data)
	return data, err
}

// findBoxLocations recursively collects offsets of all boxes matching target.
// It does NOT try to skip unknown parent boxes — it descends into all containers.
func findBoxLocations(r io.ReadSeeker, start int64, target string) ([]int64, error) {
	var result []int64
	offset := start
	for {
		size, boxType, err := readBoxHeaderAt(r, offset)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if size < 8 {
			break
		}
		if boxType == target {
			result = append(result, offset)
		}
		if isContainer(boxType) {
			children, err := findBoxLocations(r, offset+8, target)
			if err == nil {
				result = append(result, children...)
			}
		}
		offset += int64(size)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("box %s not found", target)
	}
	return result, nil
}

// findBoxAny returns the first offset of a box matching target, searching
// recursively from start.
func findBoxAny(r io.ReadSeeker, start int64, target string) (int64, error) {
	loc, err := findBoxLocations(r, start, target)
	if err != nil {
		return 0, err
	}
	return loc[0], nil
}

// findFirstBoxIn returns the first child box of parentBoxType whose type
// matches target, searching inside the parent at parentOffset.
// For container children, it recurses inside them.
func (d *Demuxer) findFirstInside(parentOffset int64, target string) (int64, error) {
	parentSize, _, err := readBoxHeaderAt(d.r, parentOffset)
	if err != nil {
		return 0, err
	}
	parentEnd := parentOffset + int64(parentSize)

	off := parentOffset + 8 // skip parent header
	for off < parentEnd {
		size, boxType, err := readBoxHeaderAt(d.r, off)
		if err != nil {
			return 0, err
		}
		if size < 8 {
			break
		}
		if boxType == target {
			return off, nil
		}
		if isContainer(boxType) {
			found, err := d.findFirstInside(off, target)
			if err == nil {
				return found, nil
			}
		}
		off += int64(size)
	}
	return 0, fmt.Errorf("box %s not found inside parent at %d", target, parentOffset)
}

// ---------------------------------------------------------------------------
// findALACInStsd
// ---------------------------------------------------------------------------

func (d *Demuxer) findALACInStsd(moovOffset int64) (cookie []byte, channels uint16, sampleSize uint16, err error) {
	stsdOffset, err := d.findFirstInside(moovOffset, "stsd")
	if err != nil {
		return nil, 0, 0, err
	}
	return d.parseSTSD(stsdOffset)
}

// parseSTSD parses the Sample Description box to find the ALAC entry.
func (d *Demuxer) parseSTSD(stsdOffset int64) (cookie []byte, channels uint16, sampleSize uint16, err error) {
	data, err := readBoxPayload(d.r, stsdOffset)
	if err != nil {
		return nil, 0, 0, err
	}
	if len(data) < 8 {
		return nil, 0, 0, fmt.Errorf("stsd too small")
	}

	pos := 8
	for pos+8 <= len(data) {
		entrySize := binary.BigEndian.Uint32(data[pos : pos+4])
		entryType := string(data[pos+4 : pos+8])
		if entryType == "alac" {
			return parseALACSampleEntry(data[pos:])
		}
		if entrySize == 0 {
			break
		}
		pos += int(entrySize)
	}

	return nil, 0, 0, fmt.Errorf("no alac entry in stsd")
}

// ---------------------------------------------------------------------------
// Sample table parsing
// ---------------------------------------------------------------------------

func (d *Demuxer) parseSampleTable(moovOffset int64) (*SampleTable, error) {
	stbl := &SampleTable{}

	// mdat offset
	mdatOffsets, err := findBoxLocations(d.r, 0, "mdat")
	var mdatOffset int64
	if err == nil && len(mdatOffsets) > 0 {
		mdatOffset = mdatOffsets[0]
	}

	if err := d.parseSTSZ(stbl, moovOffset); err != nil {
		return nil, fmt.Errorf("stsz: %w", err)
	}

	if err := d.parseSTCO(stbl, moovOffset, mdatOffset); err != nil {
		return nil, fmt.Errorf("stco: %w", err)
	}

	if err := d.parseSTSC(stbl, moovOffset, mdatOffset); err != nil {
		return nil, fmt.Errorf("stsc: %w", err)
	}

	if err := d.parseSTTS(stbl, moovOffset); err != nil {
		return nil, fmt.Errorf("stts: %w", err)
	}

	if err := d.parseMDHD(stbl, moovOffset); err != nil {
		stbl.TimeScale = 44100
	}

	return stbl, nil
}

func (d *Demuxer) parseSTSZ(stbl *SampleTable, moovOffset int64) error {
	stszOffset, err := d.findFirstInside(moovOffset, "stsz")
	if err != nil {
		return err
	}
	data, err := readBoxPayload(d.r, stszOffset)
	if err != nil {
		return err
	}
	if len(data) < 12 {
		return fmt.Errorf("stsz too small")
	}

	sampleSize := binary.BigEndian.Uint32(data[4:8])
	count := binary.BigEndian.Uint32(data[8:12])
	stbl.SampleCount = count

	if sampleSize == 0 {
		if len(data) < int(12+count*4) {
			return fmt.Errorf("stsz data too small for %d entries", count)
		}
		stbl.SampleSizes = make([]uint32, count)
		for i := uint32(0); i < count; i++ {
			stbl.SampleSizes[i] = binary.BigEndian.Uint32(data[12+i*4:])
		}
	} else {
		stbl.SampleSizes = make([]uint32, count)
		for i := uint32(0); i < count; i++ {
			stbl.SampleSizes[i] = sampleSize
		}
	}
	return nil
}

func (d *Demuxer) parseSTCO(stbl *SampleTable, moovOffset int64, mdatOffset int64) error {
	stcoOffset, err := d.findFirstInside(moovOffset, "stco")
	if err != nil {
		stcoOffset, err = d.findFirstInside(moovOffset, "co64")
		if err != nil {
			return fmt.Errorf("no stco/co64 found")
		}
		return d.parseCO64(stbl, stcoOffset)
	}

	data, err := readBoxPayload(d.r, stcoOffset)
	if err != nil {
		return err
	}
	if len(data) < 8 {
		return fmt.Errorf("stco too small")
	}
	count := binary.BigEndian.Uint32(data[4:8])
	if len(data) < int(8+count*4) {
		return fmt.Errorf("stco data too small")
	}

	stbl.SampleOffsets = make([]uint64, count)
	for i := uint32(0); i < count; i++ {
		stbl.SampleOffsets[i] = uint64(binary.BigEndian.Uint32(data[8+i*4:]))
	}
	return nil
}

func (d *Demuxer) parseCO64(stbl *SampleTable, co64Offset int64) error {
	data, err := readBoxPayload(d.r, co64Offset)
	if err != nil {
		return err
	}
	if len(data) < 8 {
		return fmt.Errorf("co64 too small")
	}
	count := binary.BigEndian.Uint32(data[4:8])
	if len(data) < int(8+count*8) {
		return fmt.Errorf("co64 data too small")
	}

	stbl.SampleOffsets = make([]uint64, count)
	for i := uint32(0); i < count; i++ {
		stbl.SampleOffsets[i] = binary.BigEndian.Uint64(data[8+i*8:])
	}
	return nil
}

// parseSTSC: sample-to-chunk table. Converts chunk offsets to per-sample offsets.
func (d *Demuxer) parseSTSC(stbl *SampleTable, moovOffset int64, mdatOffset int64) error {
	stscOffset, err := d.findFirstInside(moovOffset, "stsc")
	if err != nil {
		// No stsc: assume 1 sample per chunk
		if len(stbl.SampleOffsets) > 0 {
			for i := uint32(0); i < stbl.SampleCount; i++ {
				chunkIdx := int(i)
				if chunkIdx < len(stbl.SampleOffsets) {
					stbl.SampleOffsets[i] = stbl.SampleOffsets[chunkIdx] + uint64(i-uint32(chunkIdx))*uint64(stbl.SampleSizes[i])
				}
			}
		}
		return nil
	}

	data, err := readBoxPayload(d.r, stscOffset)
	if err != nil {
		return err
	}
	if len(data) < 8 {
		return fmt.Errorf("stsc too small")
	}
	entryCount := binary.BigEndian.Uint32(data[4:8])

	type stscEntry struct {
		firstChunk      uint32
		samplesPerChunk uint32
		sampleDescIdx   uint32
	}

	entries := make([]stscEntry, entryCount)
	for i := uint32(0); i < entryCount; i++ {
		base := 8 + i*12
		if len(data) < int(base+12) {
			break
		}
		entries[i] = stscEntry{
			firstChunk:      binary.BigEndian.Uint32(data[base:]),
			samplesPerChunk: binary.BigEndian.Uint32(data[base+4:]),
			sampleDescIdx:   binary.BigEndian.Uint32(data[base+8:]),
		}
	}

	chunkOffsets := stbl.SampleOffsets
	stbl.SampleOffsets = make([]uint64, stbl.SampleCount)

	sampleIdx := uint32(0)
	for ci := uint32(0); ci < uint32(len(chunkOffsets)) && sampleIdx < stbl.SampleCount; ci++ {
		// Find samplesPerChunk for this chunk
		samplesPerChunk := uint32(0)
		for j := len(entries) - 1; j >= 0; j-- {
			if ci+1 >= entries[j].firstChunk {
				samplesPerChunk = entries[j].samplesPerChunk
				break
			}
		}
		if samplesPerChunk == 0 {
			samplesPerChunk = 1
		}

		chunkStart := chunkOffsets[ci]

		// The chunk offset itself is the offset of the first sample in the chunk
		// When samplesPerChunk > 1, we need to compute offsets from sample sizes
		firstSampleInChunk := sampleIdx
		for s := uint32(0); s < samplesPerChunk && sampleIdx < stbl.SampleCount; s++ {
			stbl.SampleOffsets[sampleIdx] = chunkStart
			if s > 0 && firstSampleInChunk+uint32(s)-1 < stbl.SampleCount {
				// Offset of sample[firstSampleInChunk+s] = offset[firstSampleInChunk+s-1] + size[firstSampleInChunk+s-1]
				stbl.SampleOffsets[sampleIdx] = stbl.SampleOffsets[sampleIdx-1] + uint64(stbl.SampleSizes[sampleIdx-1])
			}
			sampleIdx++
		}
	}

	return nil
}

// parseSTTS: time-to-sample table.
func (d *Demuxer) parseSTTS(stbl *SampleTable, moovOffset int64) error {
	sttsOffset, err := d.findFirstInside(moovOffset, "stts")
	if err != nil {
		return err
	}

	data, err := readBoxPayload(d.r, sttsOffset)
	if err != nil {
		return err
	}
	if len(data) < 8 {
		return fmt.Errorf("stts too small")
	}

	entryCount := binary.BigEndian.Uint32(data[4:8])
	if entryCount > 0 && len(data) >= 8+8 {
		// First entry for constant-bitrate audio
		duration := binary.BigEndian.Uint32(data[8+4 : 8+8])
		if duration == 0 && entryCount > 1 && len(data) >= 8+16 {
			duration = binary.BigEndian.Uint32(data[16+4 : 16+8])
		}
		stbl.DurationPerSample = duration
	}
	return nil
}

// parseMDHD: media header for timescale.
func (d *Demuxer) parseMDHD(stbl *SampleTable, moovOffset int64) error {
	mdhdOffset, err := d.findFirstInside(moovOffset, "mdhd")
	if err != nil {
		return err
	}

	data, err := readBoxPayload(d.r, mdhdOffset)
	if err != nil {
		return err
	}
	if len(data) < 20 {
		return fmt.Errorf("mdhd too small")
	}

	version := data[0]
	if version == 0 {
		stbl.TimeScale = binary.BigEndian.Uint32(data[12:16])
	} else {
		stbl.TimeScale = binary.BigEndian.Uint32(data[20:24])
	}
	return nil
}

// ReadSample reads a single ALAC sample (frame) from the file.
func (t *ALACTrack) ReadSample(r io.ReadSeeker, index uint32) ([]byte, error) {
	if index >= t.SampleCount {
		return nil, io.EOF
	}
	offset := int64(t.SampleOffsets[index])
	size := t.SampleSizes[index]
	if size == 0 {
		return nil, fmt.Errorf("zero-size sample at index %d", index)
	}
	buf := make([]byte, size)
	if _, err := r.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	_, err := io.ReadFull(r, buf)
	return buf, err
}