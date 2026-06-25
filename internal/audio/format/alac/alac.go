// Package alac provides a configurable Apple Lossless (ALAC) decoder.
//
// Based on github.com/alicebob/alac (MIT license), modified to support
// configurable parameters from MP4/ALAC magic cookies.
//
// Copyright (c) 2016 Harmen.
package alac

import (
	"fmt"
)

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

func readUint16BE(b []byte) uint16 {
	return uint16(b[0])<<8 | uint16(b[1])
}

func readUint32BE(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
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

// readbits_16 supports reading 1 to 16 bits in big-endian format.
func (d *Decoder) readbits_16(bits int) uint32 {
	// Guard: need at least 3 bytes for any 16-bit read.
	if d.input_buffer_index+3 > len(d.input_buffer) {
		// Not enough data — return 0 and prevent further index advancement.
		return 0
	}
	result := (uint32(d.input_buffer[d.input_buffer_index]) << 16)
	if len(d.input_buffer)-d.input_buffer_index > 1 {
		result |= (uint32(d.input_buffer[d.input_buffer_index+1]) << 8)
	}
	if len(d.input_buffer)-d.input_buffer_index > 2 {
		result |= uint32(d.input_buffer[d.input_buffer_index+2])
	}
	result = result << uint(d.input_buffer_bitaccumulator)
	result = result & 0x00ffffff
	result = result >> uint(24-bits)

	newAccumulator := d.input_buffer_bitaccumulator + bits
	d.input_buffer_index += newAccumulator >> 3
	d.input_buffer_bitaccumulator = newAccumulator & 7
	return result
}

// readbits supports reading 1 to 32 bits in big-endian format.
func (d *Decoder) readbits(bits int) uint32 {
	var result int32 = 0
	if bits > 16 {
		bits -= 16
		result = int32(d.readbits_16(16) << uint(bits))
	}
	result |= int32(d.readbits_16(bits))
	return uint32(result)
}

func (d *Decoder) readbit() int {
	// Guard: need at least 1 byte available.
	if d.input_buffer_index >= len(d.input_buffer) {
		return 0
	}
	result := int(d.input_buffer[d.input_buffer_index])
	result = result << uint(d.input_buffer_bitaccumulator)
	result = result >> 7 & 1
	newAccumulator := d.input_buffer_bitaccumulator + 1
	d.input_buffer_index += newAccumulator / 8
	d.input_buffer_bitaccumulator = newAccumulator % 8
	return result
}

func (d *Decoder) unreadbits(bits int) {
	newAccumulator := d.input_buffer_bitaccumulator - bits
	d.input_buffer_index += newAccumulator >> 3
	d.input_buffer_bitaccumulator = newAccumulator & 7
	if d.input_buffer_bitaccumulator < 0 {
		d.input_buffer_bitaccumulator *= -1
	}
}

func countLeadingZeros(input int) int {
	output := 0
	curbyte := 0

	curbyte = input >> 24
	if curbyte > 0 {
		goto found
	}
	output += 8

	curbyte = input >> 16
	if curbyte&0xff > 0 {
		goto found
	}
	output += 8

	curbyte = input >> 8
	if curbyte&0xff > 0 {
		goto found
	}
	output += 8

	curbyte = input
	if curbyte&0xff > 0 {
		goto found
	}
	output += 8
	return output

found:
	if (curbyte & 0xf0) == 0 {
		output += 4
	} else {
		curbyte >>= 4
	}
	if curbyte&0x8 > 0 {
		return output
	}
	if curbyte&0x4 > 0 {
		return output + 1
	}
	if curbyte&0x2 > 0 {
		return output + 2
	}
	if curbyte&0x1 > 0 {
		return output + 3
	}
	return output + 4
}

const riceThreshold = 8

func (d *Decoder) entropyDecodeValue(readSampleSize int, k int, riceKModifierMask int) int32 {
	x := int32(0)
	for x <= riceThreshold && d.readbit() != 0 {
		x++
	}
	if x > riceThreshold {
		value := int32(d.readbits(readSampleSize))
		value &= int32((uint32(0xffffffff) >> uint(32-readSampleSize)))
		x = value
	} else {
		if k != 1 {
			extraBits := int(d.readbits(k))
			x *= int32((((1 << uint(k)) - 1) & riceKModifierMask))
			if extraBits > 1 {
				x += int32(extraBits - 1)
			} else {
				d.unreadbits(1)
			}
		}
	}
	return x
}

func (d *Decoder) entropyRiceDecode(
	outputBuffer []int32,
	outputSize int,
	readSampleSize int,
	riceInitialHistory int,
	riceKModifier int,
	riceHistoryMult int,
	riceKModifierMask int,
) {
	history := riceInitialHistory
	signModifier := 0

	for outputCount := 0; outputCount < outputSize; outputCount++ {
		k := int32(31 - riceKModifier - countLeadingZeros((history>>9)+3))
		if k < 0 {
			k += int32(riceKModifier)
		} else {
			k = int32(riceKModifier)
		}

		decodedValue := int32(d.entropyDecodeValue(readSampleSize, int(k), 0xFFFFFFFF))
		decodedValue += int32(signModifier)
		finalValue := (decodedValue + 1) / 2
		if decodedValue&1 != 0 {
			finalValue *= -1
		}
		outputBuffer[outputCount] = finalValue

		signModifier = 0
		history += (int(decodedValue) * riceHistoryMult) - ((history * riceHistoryMult) >> 9)
		if decodedValue > 0xFFFF {
			history = 0xFFFF
		}

		if history < 128 && outputCount+1 < outputSize {
			signModifier = 1
			k = int32(countLeadingZeros(history)) + ((int32(history) + 16) / 64) - 24
			blockSize := int32(d.entropyDecodeValue(16, int(k), riceKModifierMask))
			if blockSize > 0 {
				for i := outputCount + 1; i < outputCount+1+int(blockSize); i++ {
					outputBuffer[i] = 0
				}
				outputCount += int(blockSize)
			}
			if blockSize > 0xFFFF {
				signModifier = 0
			}
			history = 0
		}
	}
}

func signExtended32(val int32, bits int) int32 {
	return ((val << uint(32-bits)) >> uint(32-bits))
}

func signOnly(v int) int {
	if v < 0 {
		return -1
	}
	if v > 0 {
		return 1
	}
	return 0
}

func predictorDecompressFirAdapt(
	errorBuffer []int32,
	bufferOut []int32,
	outputSize int,
	readSampleSize int,
	predictorCoefTable [32]int16,
	predictorCoefNum int,
	predictorQuantitization int,
) {
	bufferOut[0] = errorBuffer[0]

	if predictorCoefNum == 0 {
		if outputSize <= 1 {
			return
		}
		copy(bufferOut[1:], errorBuffer[1:outputSize])
		return
	}

	if predictorCoefNum == 0x1f {
		if outputSize <= 1 {
			return
		}
		for i := 0; i < outputSize-1; i++ {
			prevValue := bufferOut[i]
			errorValue := errorBuffer[i+1]
			bufferOut[i+1] = int32(signExtended32((prevValue + errorValue), readSampleSize))
		}
		return
	}

	if predictorCoefNum > 0 {
		for i := 0; i < predictorCoefNum; i++ {
			val := bufferOut[i] + errorBuffer[i+1]
			val = signExtended32(val, readSampleSize)
			bufferOut[i+1] = val
		}
	}

	if predictorCoefNum > 0 {
		for i := predictorCoefNum + 1; i < outputSize; i++ {
			sum := 0
			errorVal := errorBuffer[i]

			for j := 0; j < predictorCoefNum; j++ {
				sum += int((bufferOut[predictorCoefNum-j] - bufferOut[0]) * int32(predictorCoefTable[j]))
			}

			outVal := (1 << uint(predictorQuantitization-1)) + sum
			outVal = outVal >> uint(predictorQuantitization)
			outVal = outVal + int(bufferOut[0]) + int(errorVal)
			outVal = int(signExtended32(int32(outVal), readSampleSize))

			bufferOut[predictorCoefNum+1] = int32(outVal)

			if errorVal > 0 {
				for predictorNum := predictorCoefNum - 1; predictorNum >= 0 && errorVal > 0; predictorNum-- {
					val := int(bufferOut[0] - bufferOut[predictorCoefNum-predictorNum])
					sign := signOnly(val)
					predictorCoefTable[predictorNum] -= int16(sign)
					val *= sign
					errorVal -= int32((val >> uint(predictorQuantitization)) * (predictorCoefNum - predictorNum))
				}
			} else if errorVal < 0 {
				for predictorNum := predictorCoefNum - 1; predictorNum >= 0 && errorVal < 0; predictorNum-- {
					val := int(bufferOut[0] - bufferOut[predictorCoefNum-predictorNum])
					sign := -signOnly(val)
					predictorCoefTable[predictorNum] -= int16(sign)
					val *= sign
					errorVal -= int32((val >> uint(predictorQuantitization)) * (predictorCoefNum - predictorNum))
				}
			}

			bufferOut = bufferOut[1:]
		}
	}
}

func deinterlace16(
	bufferA, bufferB []int32,
	bufferOut []byte,
	numChannels, numSamples int,
	interlacingShift uint8,
	interlacingLeftWeight uint8,
) {
	if numSamples <= 0 {
		return
	}

	if interlacingLeftWeight != 0 {
		for i := 0; i < numSamples; i++ {
			midright := bufferA[i]
			difference := bufferB[i]
			right := int16(midright - ((difference * int32(interlacingLeftWeight)) >> interlacingShift))
			left := right + int16(difference)

			bufferOut[2*i*numChannels] = byte(left)
			bufferOut[2*i*numChannels+1] = byte(left >> 8)
			bufferOut[2*i*numChannels+2] = byte(right)
			bufferOut[2*i*numChannels+3] = byte(right >> 8)
		}
		return
	}

	for i := 0; i < numSamples; i++ {
		left := int16(bufferA[i])
		right := int16(bufferB[i])

		bufferOut[2*i*numChannels] = byte(left)
		bufferOut[2*i*numChannels+1] = byte(left >> 8)
		bufferOut[2*i*numChannels+2] = byte(right)
		bufferOut[2*i*numChannels+3] = byte(right >> 8)
	}
}

func deinterlace24(
	bufferA, bufferB []int32,
	uncompressedBytes int,
	uncompressedBytesBufferA, uncompressedBytesBufferB []int32,
	bufferOut []byte,
	numChannels, numSamples int,
	interlacingShift, interlacingLeftWeight uint8,
) {
	if numSamples <= 0 {
		return
	}

	if interlacingLeftWeight > 0 {
		for i := 0; i < numSamples; i++ {
			midright := bufferA[i]
			difference := bufferB[i]
			right := midright - ((difference * int32(interlacingLeftWeight)) >> interlacingShift)
			left := right + difference

			if uncompressedBytes > 0 {
				mask := uint32(^(0xFFFFFFFF << uint(uncompressedBytes*8)))
				left <<= uint(uncompressedBytes * 8)
				right <<= uint(uncompressedBytes * 8)
				left |= uncompressedBytesBufferA[i] & int32(mask)
				right |= uncompressedBytesBufferB[i] & int32(mask)
			}

			bufferOut[i*numChannels*3] = byte(left & 0xFF)
			bufferOut[i*numChannels*3+1] = byte((left >> 8) & 0xFF)
			bufferOut[i*numChannels*3+2] = byte((left >> 16) & 0xFF)
			bufferOut[i*numChannels*3+3] = byte(right & 0xFF)
			bufferOut[i*numChannels*3+4] = byte((right >> 8) & 0xFF)
			bufferOut[i*numChannels*3+5] = byte((right >> 16) & 0xFF)
		}
		return
	}

	for i := 0; i < numSamples; i++ {
		left := bufferA[i]
		right := bufferB[i]

		if uncompressedBytes > 0 {
			mask := uint32(^(0xFFFFFFFF << uint(uncompressedBytes*8)))
			left <<= uint(uncompressedBytes * 8)
			right <<= uint(uncompressedBytes * 8)
			left |= uncompressedBytesBufferA[i] & int32(mask)
			right |= uncompressedBytesBufferB[i] & int32(mask)
		}

		bufferOut[i*numChannels*3] = byte(left & 0xFF)
		bufferOut[i*numChannels*3+1] = byte((left >> 8) & 0xFF)
		bufferOut[i*numChannels*3+2] = byte((left >> 16) & 0xFF)
		bufferOut[i*numChannels*3+3] = byte(right & 0xFF)
		bufferOut[i*numChannels*3+4] = byte((right >> 8) & 0xFF)
		bufferOut[i*numChannels*3+5] = byte((right >> 16) & 0xFF)
	}
}

func (d *Decoder) decodeFrame(inbuffer []byte) []byte {
	outputsamples := d.MaxSamplesPerFrame

	d.input_buffer = inbuffer
	d.input_buffer_index = 0
	d.input_buffer_bitaccumulator = 0

	channels := d.readbits(3)
	outputsize := int(outputsamples) * d.bytesPerSample

	switch channels {
	case 0: /* 1 channel */
		readsamplesize := 0
		ricemodifier := 0

		d.readbits(4)
		d.readbits(12)

		hassize := int(d.readbits(1))
		uncompressed_bytes := int(d.readbits(2))
		isnotcompressed := int(d.readbits(1))

		if hassize > 0 {
			outputsamples = d.readbits(32)
			outputsize = int(outputsamples) * d.bytesPerSample
		}

		readsamplesize = int(d.CookieSampleSize) - (uncompressed_bytes * 8)

		if isnotcompressed == 0 {
			var predictor_coef_table [32]int16

			d.readbits(8)
			d.readbits(8)

			prediction_type := int(d.readbits(4))
			prediction_quantitization := int(d.readbits(4))
			ricemodifier = int(d.readbits(3))
			predictor_coef_num := int(d.readbits(5))

			for i := 0; i < predictor_coef_num; i++ {
				predictor_coef_table[i] = int16(d.readbits(16))
			}

			if uncompressed_bytes != 0 {
				for i := uint32(0); i < outputsamples; i++ {
					d.uncompressed_bytes_buffer_a[i] = int32(d.readbits(uncompressed_bytes * 8))
				}
			}

			d.entropyRiceDecode(
				d.predicterror_buffer_a,
				int(outputsamples),
				readsamplesize,
				int(d.CookieRiceInitialHistory),
				int(d.CookieRiceKModifier),
				ricemodifier*int(d.CookieRiceHistoryMult)/4,
				(1<<d.CookieRiceKModifier)-1,
			)

			if prediction_type == 0 {
				predictorDecompressFirAdapt(
					d.predicterror_buffer_a,
					d.outputsamples_buffer_a,
					int(outputsamples),
					readsamplesize,
					predictor_coef_table,
					predictor_coef_num,
					prediction_quantitization,
				)
			} else {
				fmt.Printf("FIXME: unhandled prediction type: %d\n", prediction_type)
			}
		} else {
			if d.CookieSampleSize <= 16 {
				for i := uint32(0); i < outputsamples; i++ {
					audiobits := int32(d.readbits(int(d.CookieSampleSize)))
					audiobits = signExtended32(audiobits, int(d.CookieSampleSize))
					d.outputsamples_buffer_a[i] = audiobits
				}
			} else {
				for i := uint32(0); i < outputsamples; i++ {
					audiobits := int32(d.readbits(16))
					audiobits = audiobits << (d.CookieSampleSize - 16)
					audiobits |= int32(d.readbits(int(d.CookieSampleSize - 16)))
					audiobits = signExtended32(audiobits, int(d.CookieSampleSize))
					d.outputsamples_buffer_a[i] = audiobits
				}
			}
			uncompressed_bytes = 0
		}

		outbuffer := make([]byte, outputsize)
		switch d.CookieSampleSize {
		case 16:
			for i := uint32(0); i < outputsamples; i++ {
				sample := int16(d.outputsamples_buffer_a[i])
				outbuffer[2*int(i)*d.numChannels] = byte(sample)
				outbuffer[2*int(i)*d.numChannels+1] = byte(sample >> 8)
			}
		case 24:
			for i := uint32(0); i < outputsamples; i++ {
				sample := int32(d.outputsamples_buffer_a[i])
				if uncompressed_bytes != 0 {
					sample = sample << uint(uncompressed_bytes*8)
					mask := uint32(^(0xFFFFFFFF << uint(uncompressed_bytes*8)))
					sample |= d.uncompressed_bytes_buffer_a[i] & int32(mask)
				}
				outbuffer[int(i)*d.numChannels*3] = byte(sample & 0xFF)
				outbuffer[int(i)*d.numChannels*3+1] = byte((sample >> 8) & 0xFF)
				outbuffer[int(i)*d.numChannels*3+2] = byte((sample >> 16) & 0xFF)
			}
		case 20, 32:
			fmt.Printf("FIXME: unimplemented sample size %d\n", d.CookieSampleSize)
		}
		return outbuffer

	case 1: /* 2 channels */
		hassize := 0
		isnotcompressed := 0
		readsamplesize := 0
		uncompressed_bytes := 0
		var interlacingShift, interlacingLeftWeight uint8

		d.readbits(4)
		d.readbits(12)

		hassize = int(d.readbits(1))
		uncompressed_bytes = int(d.readbits(2))
		isnotcompressed = int(d.readbits(1))

		if hassize != 0 {
			outputsamples = d.readbits(32)
			outputsize = int(outputsamples) * d.bytesPerSample
		}

		readsamplesize = int(d.CookieSampleSize) - (uncompressed_bytes * 8) + 1

		if isnotcompressed == 0 {
			interlacingShift = uint8(d.readbits(8))
			interlacingLeftWeight = uint8(d.readbits(8))

			var predictorCoefTableA, predictorCoefTableB [32]int16

			predictionTypeA := int(d.readbits(4))
			predictionQuantitizationA := int(d.readbits(4))
			riceModifierA := int(d.readbits(3))
			predictorCoefNumA := int(d.readbits(5))

			for i := 0; i < predictorCoefNumA; i++ {
				predictorCoefTableA[i] = int16(d.readbits(16))
			}

			predictionTypeB := int(d.readbits(4))
			predictionQuantitizationB := int(d.readbits(4))
			riceModifierB := int(d.readbits(3))
			predictorCoefNumB := int(d.readbits(5))

			for i := 0; i < predictorCoefNumB; i++ {
				predictorCoefTableB[i] = int16(d.readbits(16))
			}

			if uncompressed_bytes != 0 {
				for i := uint32(0); i < outputsamples; i++ {
					d.uncompressed_bytes_buffer_a[i] = int32(d.readbits(uncompressed_bytes * 8))
					d.uncompressed_bytes_buffer_b[i] = int32(d.readbits(uncompressed_bytes * 8))
				}
			}

			d.entropyRiceDecode(
				d.predicterror_buffer_a,
				int(outputsamples),
				readsamplesize,
				int(d.CookieRiceInitialHistory),
				int(d.CookieRiceKModifier),
				riceModifierA*int(d.CookieRiceHistoryMult)/4,
				(1<<d.CookieRiceKModifier)-1,
			)

			if predictionTypeA == 0 {
				predictorDecompressFirAdapt(
					d.predicterror_buffer_a,
					d.outputsamples_buffer_a,
					int(outputsamples),
					readsamplesize,
					predictorCoefTableA,
					predictorCoefNumA,
					predictionQuantitizationA,
				)
			} else {
				fmt.Printf("FIXME: unhandled prediction type: %d\n", predictionTypeA)
			}

			d.entropyRiceDecode(
				d.predicterror_buffer_b,
				int(outputsamples),
				readsamplesize,
				int(d.CookieRiceInitialHistory),
				int(d.CookieRiceKModifier),
				riceModifierB*int(d.CookieRiceHistoryMult)/4,
				(1<<d.CookieRiceKModifier)-1,
			)

			if predictionTypeB == 0 {
				predictorDecompressFirAdapt(
					d.predicterror_buffer_b,
					d.outputsamples_buffer_b,
					int(outputsamples),
					readsamplesize,
					predictorCoefTableB,
					predictorCoefNumB,
					predictionQuantitizationB,
				)
			} else {
				fmt.Printf("FIXME: unhandled prediction type: %d\n", predictionTypeB)
			}
		} else {
			if d.CookieSampleSize <= 16 {
				for i := uint32(0); i < outputsamples; i++ {
					audiobitsA := d.readbits(int(d.CookieSampleSize))
					audiobitsB := d.readbits(int(d.CookieSampleSize))
					audiobitsA = uint32(signExtended32(int32(audiobitsA), int(d.CookieSampleSize)))
					audiobitsB = uint32(signExtended32(int32(audiobitsB), int(d.CookieSampleSize)))
					d.outputsamples_buffer_a[i] = int32(audiobitsA)
					d.outputsamples_buffer_b[i] = int32(audiobitsB)
				}
			} else {
				for i := uint32(0); i < outputsamples; i++ {
					audiobitsA := int32(d.readbits(16))
					audiobitsA = audiobitsA << (d.CookieSampleSize - 16)
					audiobitsA |= int32(d.readbits(int(d.CookieSampleSize - 16)))
					audiobitsA = signExtended32(audiobitsA, int(d.CookieSampleSize))

					audiobitsB := int32(d.readbits(16))
					audiobitsB = audiobitsB << (d.CookieSampleSize - 16)
					audiobitsB |= int32(d.readbits(int(d.CookieSampleSize - 16)))
					audiobitsB = signExtended32(audiobitsB, int(d.CookieSampleSize))

					d.outputsamples_buffer_a[i] = audiobitsA
					d.outputsamples_buffer_b[i] = audiobitsB
				}
			}
			uncompressed_bytes = 0
			interlacingShift = 0
			interlacingLeftWeight = 0
		}

		outbuffer := make([]byte, outputsize)
		switch d.CookieSampleSize {
		case 16:
			deinterlace16(
				d.outputsamples_buffer_a,
				d.outputsamples_buffer_b,
				outbuffer,
				d.numChannels,
				int(outputsamples),
				interlacingShift,
				interlacingLeftWeight,
			)
		case 24:
			deinterlace24(
				d.outputsamples_buffer_a,
				d.outputsamples_buffer_b,
				uncompressed_bytes,
				d.uncompressed_bytes_buffer_a,
				d.uncompressed_bytes_buffer_b,
				outbuffer,
				d.numChannels,
				int(outputsamples),
				interlacingShift,
				interlacingLeftWeight,
			)
		case 20, 32:
			fmt.Printf("FIXME: unimplemented sample size %d\n", d.CookieSampleSize)
		}
		return outbuffer

	default:
		fmt.Printf("unimplemented channel count %d\n", channels+1)
	}

	return nil
}
