package format

import "encoding/binary"

const (
	apeDescriptorSize = 52
	apeHeaderSize     = 24
)

// probeAPE reads the fixed Monkey's Audio descriptor and header. Unlike the
// optional decoder backends, this needs neither apecli nor ffmpeg and is cheap
// enough for a library scan. Malformed headers leave Duration zero.
func probeAPE(r *probeReader, result *ProbeInfo) {
	offset := int64(0)
	var magic [4]byte
	if !r.at(magic[:], offset) || string(magic[:]) != "MAC " {
		var id3 [10]byte
		if !r.at(id3[:], 0) || string(id3[:3]) != "ID3" {
			return
		}
		tagSize := int64(id3[6])<<21 | int64(id3[7])<<14 | int64(id3[8])<<7 | int64(id3[9])
		offset = 10 + tagSize
		if !r.at(magic[:], offset) || string(magic[:]) != "MAC " {
			return
		}
	}

	var descriptor [apeDescriptorSize]byte
	if !r.at(descriptor[:], offset) {
		return
	}
	descriptorSize := int64(binary.LittleEndian.Uint32(descriptor[8:12]))
	if descriptorSize < apeDescriptorSize || descriptorSize > r.size-offset {
		return
	}

	var header [apeHeaderSize]byte
	if !r.at(header[:], offset+descriptorSize) {
		return
	}
	blocksPerFrame := uint64(binary.LittleEndian.Uint32(header[4:8]))
	finalFrameBlocks := uint64(binary.LittleEndian.Uint32(header[8:12]))
	totalFrames := uint64(binary.LittleEndian.Uint32(header[12:16]))
	sampleRate := uint64(binary.LittleEndian.Uint32(header[20:24]))
	if totalFrames == 0 || sampleRate == 0 {
		return
	}

	samples := finalFrameBlocks
	if totalFrames > 1 {
		if blocksPerFrame == 0 {
			return
		}
		remaining := totalFrames - 1
		if blocksPerFrame > (^uint64(0)-samples)/remaining {
			return
		}
		samples += remaining * blocksPerFrame
	}
	result.Duration = probeDuration(samples, sampleRate)
}
