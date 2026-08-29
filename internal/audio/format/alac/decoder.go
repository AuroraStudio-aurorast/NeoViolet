// Package alac provides a configurable Apple Lossless (ALAC) decoder.
//
// Based on github.com/alicebob/alac (MIT license), modified to support
// configurable parameters from MP4/ALAC magic cookies.
//
// Copyright (c) 2016 Harmen.
package alac

// Decoder is an Apple Lossless (ALAC) decoder with configurable parameters.
// It decodes ALAC frames into little-endian PCM samples.
type Decoder struct {
	input_buffer                []byte
	input_buffer_index          int
	input_buffer_bitaccumulator int

	sampleSize     int
	numChannels    int
	bytesPerSample int

	predicterror_buffer_a       []int32
	predicterror_buffer_b       []int32
	outputsamples_buffer_a      []int32
	outputsamples_buffer_b      []int32
	uncompressed_bytes_buffer_a []int32
	uncompressed_bytes_buffer_b []int32

	// ALAC magic cookie parameters (from the "alac" box in MP4)
	MaxSamplesPerFrame       uint32
	Cookie_7a                uint8
	CookieSampleSize         uint8
	CookieRiceHistoryMult    uint8
	CookieRiceInitialHistory uint8
	CookieRiceKModifier      uint8
	Cookie_7f                uint8
	Cookie_80                uint16
	Cookie_82                uint32
	Cookie_86                uint32
	CookieSampleRate         uint32
}

// NewDecoder creates a new ALAC decoder configured from the magic cookie
// parameters. The cookie is the content of the "alac" box in an MP4
// container (typically 24-48 bytes).
//
// Standard CD-quality ALAC (44.1kHz, 16-bit, 2ch) uses:
//
//	MaxSamplesPerFrame=4096, CookieSampleSize=16,
//	CookieRiceHistoryMult=40, CookieRiceInitialHistory=10,
//	CookieRiceKModifier=14, Cookie_7f=2, Cookie_80=255,
//	CookieSampleRate=44100
func NewDecoder(cookie []byte) (*Decoder, error) {
	d := &Decoder{}

	if len(cookie) >= 24 {
		d.parseCookie(cookie)
	} else {
		// Sensible defaults for common ALAC files
		d.MaxSamplesPerFrame = 4096
		d.CookieSampleSize = 16
		d.CookieRiceHistoryMult = 40
		d.CookieRiceInitialHistory = 10
		d.CookieRiceKModifier = 14
		d.Cookie_7f = 2
		d.Cookie_80 = 255
		d.CookieSampleRate = 44100
		d.Cookie_82 = 0x000020e7
		d.Cookie_86 = 0x00069fe4
	}

	// Validate ALAC cookie parameters to prevent division by zero and
	// out-of-bounds access. If invalid, fall back to safe defaults.
	validSampleSizes := map[uint8]bool{8: true, 16: true, 20: true, 24: true, 32: true}
	if !validSampleSizes[d.CookieSampleSize] {
		d.CookieSampleSize = 16
	}

	d.numChannels = 2 // stereo is the norm for ALAC; mono is rare
	d.sampleSize = int(d.CookieSampleSize)
	d.bytesPerSample = (d.sampleSize / 8) * d.numChannels

	d.allocateBuffers()
	return d, nil
}

// SetNumChannels overrides the channel count. ALAC files are almost always
// stereo (2 channels). Call this before the first Decode if the file is mono.
func (d *Decoder) SetNumChannels(n int) {
	if n < 1 || n > 8 {
		n = 2
	}
	d.numChannels = n
	d.bytesPerSample = (d.sampleSize / 8) * d.numChannels
}

// SampleRate returns the sample rate configured from the cookie.
func (d *Decoder) SampleRate() int {
	return int(d.CookieSampleRate)
}

// NumChannels returns the configured channel count.
func (d *Decoder) NumChannels() int {
	return d.numChannels
}

// SampleSize returns the configured sample size in bits.
func (d *Decoder) SampleSize() int {
	return d.sampleSize
}

// Decode decodes one ALAC frame into little-endian PCM bytes.
// The input is a raw ALAC frame (without any container headers).
func (d *Decoder) Decode(in []byte) []byte {
	return d.decodeFrame(in)
}

// parseCookie parses the ALAC magic cookie to configure the decoder.
// The cookie from an MP4 "alac" sub-box has a 4-byte version/flags prefix
// followed by the setinfo parameters (all big-endian):
//
//	[0-3]   version/flags (skip)
//	[4-7]   max_samples_per_frame (uint32)
//	[8]     unknown (7a)
//	[9]     sample_size
//	[10]    rice_historymult
//	[11]    rice_initialhistory
//	[12]    rice_kmodifier
//	[13]    unknown (7f)
//	[14-15] unknown (80) (uint16)
//	[16-19] unknown (82) (uint32)
//	[20-23] unknown (86) (uint32)
//	[24-27] sample_rate (uint32)
func (d *Decoder) parseCookie(cookie []byte) {
	// Skip 4-byte version/flags
	offset := 4
	if len(cookie) < offset+24 {
		offset = 0 // try without skipping if too short
	}
	d.MaxSamplesPerFrame = readUint32BE(cookie[offset+0:])
	if d.MaxSamplesPerFrame == 0 || d.MaxSamplesPerFrame > 65536 {
		d.MaxSamplesPerFrame = 4096
	}
	if len(cookie) >= offset+5 {
		d.Cookie_7a = cookie[offset+4]
		d.CookieSampleSize = cookie[offset+5]
		d.CookieRiceHistoryMult = cookie[offset+6]
		d.CookieRiceInitialHistory = cookie[offset+7]
		d.CookieRiceKModifier = cookie[offset+8]
		d.Cookie_7f = cookie[offset+9]
	}
	if len(cookie) >= offset+12 {
		d.Cookie_80 = readUint16BE(cookie[offset+10:])
	}
	if len(cookie) >= offset+16 {
		d.Cookie_82 = readUint32BE(cookie[offset+12:])
	}
	if len(cookie) >= offset+20 {
		d.Cookie_86 = readUint32BE(cookie[offset+16:])
	}
	if len(cookie) >= offset+24 {
		d.CookieSampleRate = readUint32BE(cookie[offset+20:])
	}
}

func (d *Decoder) allocateBuffers() {
	n := d.MaxSamplesPerFrame * 4
	d.predicterror_buffer_a = make([]int32, n)
	d.predicterror_buffer_b = make([]int32, n)
	d.outputsamples_buffer_a = make([]int32, n)
	d.outputsamples_buffer_b = make([]int32, n)
	d.uncompressed_bytes_buffer_a = make([]int32, n)
	d.uncompressed_bytes_buffer_b = make([]int32, n)
}
