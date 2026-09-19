package alac

import (
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

const riceThreshold = 8

func (d *Decoder) decodeFrame(inbuffer []byte) []byte {
	outputsamples := d.MaxSamplesPerFrame

	d.inputBuffer = inbuffer
	d.inputBufferIndex = 0
	d.inputBufferBitaccumulator = 0

	channels := d.readbits(3)
	outputsize := int(outputsamples) * d.bytesPerSample

	switch channels {
	case 0: /* 1 channel */
		readsamplesize := 0
		ricemodifier := 0

		d.readbits(4)
		d.readbits(12)

		hassize := int(d.readbits(1))
		uncompressedBytes := int(d.readbits(2))
		isnotcompressed := int(d.readbits(1))

		if hassize > 0 {
			outputsamples = d.readbits(32)
			outputsize = int(outputsamples) * d.bytesPerSample
		}

		readsamplesize = int(d.CookieSampleSize) - (uncompressedBytes * 8)

		if isnotcompressed == 0 {
			var predictorCoefTable [32]int16

			d.readbits(8)
			d.readbits(8)

			predictionType := int(d.readbits(4))
			predictionQuantitization := int(d.readbits(4))
			ricemodifier = int(d.readbits(3))
			predictorCoefNum := int(d.readbits(5))

			for i := 0; i < predictorCoefNum; i++ {
				// #nosec G115 -- 16-bit coefficient, value is bounded.
				predictorCoefTable[i] = int16(d.readbits(16))
			}

			if uncompressedBytes != 0 {
				for i := uint32(0); i < outputsamples; i++ {
					d.uncompressedBytesBufferA[i] = int32(d.readbits(uncompressedBytes * 8)) // #nosec G115
				}
			}

			d.entropyRiceDecode(
				d.predicterrorBufferA,
				int(outputsamples),
				readsamplesize,
				int(d.CookieRiceInitialHistory),
				int(d.CookieRiceKModifier),
				ricemodifier*int(d.CookieRiceHistoryMult)/4,
				(1<<d.CookieRiceKModifier)-1,
			)

			if predictionType == 0 {
				predictorDecompressFirAdapt(
					d.predicterrorBufferA,
					d.outputsamplesBufferA,
					int(outputsamples),
					readsamplesize,
					predictorCoefTable,
					predictorCoefNum,
					predictionQuantitization,
				)
			} else {
				logger.Debug("ALAC: unhandled prediction type", "type", predictionType)
			}
		} else {
			if d.CookieSampleSize <= 16 {
				for i := uint32(0); i < outputsamples; i++ {
					audiobits := int32(d.readbits(int(d.CookieSampleSize))) // #nosec G115
					audiobits = signExtended32(audiobits, int(d.CookieSampleSize))
					d.outputsamplesBufferA[i] = audiobits
				}
			} else {
				for i := uint32(0); i < outputsamples; i++ {
					audiobits := int32(d.readbits(16)) // #nosec G115
					audiobits <<= (d.CookieSampleSize - 16)
					audiobits |= int32(d.readbits(int(d.CookieSampleSize - 16))) // #nosec G115
					audiobits = signExtended32(audiobits, int(d.CookieSampleSize))
					d.outputsamplesBufferA[i] = audiobits
				}
			}
			uncompressedBytes = 0
		}

		outbuffer := make([]byte, outputsize)
		switch d.CookieSampleSize {
		case 16:
			for i := uint32(0); i < outputsamples; i++ {
				// #nosec G115 -- 16-bit sample narrowed from int32 buffer; bounded.
				sample := int16(d.outputsamplesBufferA[i])
				outbuffer[2*int(i)*d.numChannels] = byte(sample)        // #nosec G115 -- low byte of a 16-bit sample
				outbuffer[2*int(i)*d.numChannels+1] = byte(sample >> 8) // #nosec G115 -- high byte of a 16-bit sample
			}
		case 24:
			for i := uint32(0); i < outputsamples; i++ {
				sample := int32(d.outputsamplesBufferA[i])
				if uncompressedBytes != 0 {
					sample <<= uint(uncompressedBytes * 8)
					mask := uint32(^(0xFFFFFFFF << uint(uncompressedBytes*8)))
					sample |= d.uncompressedBytesBufferA[i] & int32(mask) // #nosec G115
				}
				outbuffer[int(i)*d.numChannels*3] = byte(sample & 0xFF)
				outbuffer[int(i)*d.numChannels*3+1] = byte((sample >> 8) & 0xFF)
				outbuffer[int(i)*d.numChannels*3+2] = byte((sample >> 16) & 0xFF)
			}
		case 20, 32:
			logger.Debug("ALAC: unimplemented sample size", "size", d.CookieSampleSize)
		}
		return outbuffer

	case 1: /* 2 channels */
		hassize := 0
		isnotcompressed := 0
		readsamplesize := 0
		uncompressedBytes := 0
		var interlacingShift, interlacingLeftWeight uint8

		d.readbits(4)
		d.readbits(12)

		hassize = int(d.readbits(1))
		uncompressedBytes = int(d.readbits(2))
		isnotcompressed = int(d.readbits(1))

		if hassize != 0 {
			outputsamples = d.readbits(32)
			outputsize = int(outputsamples) * d.bytesPerSample
		}

		readsamplesize = int(d.CookieSampleSize) - (uncompressedBytes * 8) + 1

		if isnotcompressed == 0 {
			// #nosec G115 -- 8-bit interlacing values, bounded by readbits(8).
			interlacingShift = uint8(d.readbits(8))
			interlacingLeftWeight = uint8(d.readbits(8)) // #nosec G115

			var predictorCoefTableA, predictorCoefTableB [32]int16

			predictionTypeA := int(d.readbits(4))
			predictionQuantitizationA := int(d.readbits(4))
			riceModifierA := int(d.readbits(3))
			predictorCoefNumA := int(d.readbits(5))

			for i := 0; i < predictorCoefNumA; i++ {
				// #nosec G115 -- 16-bit coefficient, value is bounded.
				predictorCoefTableA[i] = int16(d.readbits(16))
			}

			predictionTypeB := int(d.readbits(4))
			predictionQuantitizationB := int(d.readbits(4))
			riceModifierB := int(d.readbits(3))
			predictorCoefNumB := int(d.readbits(5))

			for i := 0; i < predictorCoefNumB; i++ {
				// #nosec G115 -- 16-bit coefficient, value is bounded.
				predictorCoefTableB[i] = int16(d.readbits(16))
			}

			if uncompressedBytes != 0 {
				for i := uint32(0); i < outputsamples; i++ {
					d.uncompressedBytesBufferA[i] = int32(d.readbits(uncompressedBytes * 8)) // #nosec G115
					d.uncompressedBytesBufferB[i] = int32(d.readbits(uncompressedBytes * 8)) // #nosec G115
				}
			}

			d.entropyRiceDecode(
				d.predicterrorBufferA,
				int(outputsamples),
				readsamplesize,
				int(d.CookieRiceInitialHistory),
				int(d.CookieRiceKModifier),
				riceModifierA*int(d.CookieRiceHistoryMult)/4,
				(1<<d.CookieRiceKModifier)-1,
			)

			if predictionTypeA == 0 {
				predictorDecompressFirAdapt(
					d.predicterrorBufferA,
					d.outputsamplesBufferA,
					int(outputsamples),
					readsamplesize,
					predictorCoefTableA,
					predictorCoefNumA,
					predictionQuantitizationA,
				)
			} else {
				logger.Debug("ALAC: unhandled prediction type", "type", predictionTypeA)
			}

			d.entropyRiceDecode(
				d.predicterrorBufferB,
				int(outputsamples),
				readsamplesize,
				int(d.CookieRiceInitialHistory),
				int(d.CookieRiceKModifier),
				riceModifierB*int(d.CookieRiceHistoryMult)/4,
				(1<<d.CookieRiceKModifier)-1,
			)

			if predictionTypeB == 0 {
				predictorDecompressFirAdapt(
					d.predicterrorBufferB,
					d.outputsamplesBufferB,
					int(outputsamples),
					readsamplesize,
					predictorCoefTableB,
					predictorCoefNumB,
					predictionQuantitizationB,
				)
			} else {
				logger.Debug("ALAC: unhandled prediction type", "type", predictionTypeB)
			}
		} else {
			if d.CookieSampleSize <= 16 {
				for i := uint32(0); i < outputsamples; i++ {
					audiobitsA := d.readbits(int(d.CookieSampleSize))
					audiobitsB := d.readbits(int(d.CookieSampleSize))
					audiobitsA = uint32(signExtended32(int32(audiobitsA), int(d.CookieSampleSize))) // #nosec G115
					audiobitsB = uint32(signExtended32(int32(audiobitsB), int(d.CookieSampleSize))) // #nosec G115
					d.outputsamplesBufferA[i] = int32(audiobitsA)                                   // #nosec G115
					d.outputsamplesBufferB[i] = int32(audiobitsB)                                   // #nosec G115
				}
			} else {
				for i := uint32(0); i < outputsamples; i++ {
					audiobitsA := int32(d.readbits(16)) // #nosec G115
					audiobitsA <<= (d.CookieSampleSize - 16)
					audiobitsA |= int32(d.readbits(int(d.CookieSampleSize - 16))) // #nosec G115
					audiobitsA = signExtended32(audiobitsA, int(d.CookieSampleSize))

					audiobitsB := int32(d.readbits(16)) // #nosec G115
					audiobitsB <<= (d.CookieSampleSize - 16)
					audiobitsB |= int32(d.readbits(int(d.CookieSampleSize - 16))) // #nosec G115
					audiobitsB = signExtended32(audiobitsB, int(d.CookieSampleSize))

					d.outputsamplesBufferA[i] = audiobitsA
					d.outputsamplesBufferB[i] = audiobitsB
				}
			}
			uncompressedBytes = 0
			interlacingShift = 0
			interlacingLeftWeight = 0
		}

		outbuffer := make([]byte, outputsize)
		switch d.CookieSampleSize {
		case 16:
			deinterlace16(
				d.outputsamplesBufferA,
				d.outputsamplesBufferB,
				outbuffer,
				d.numChannels,
				int(outputsamples),
				interlacingShift,
				interlacingLeftWeight,
			)
		case 24:
			deinterlace24(
				d.outputsamplesBufferA,
				d.outputsamplesBufferB,
				uncompressedBytes,
				d.uncompressedBytesBufferA,
				d.uncompressedBytesBufferB,
				outbuffer,
				d.numChannels,
				int(outputsamples),
				interlacingShift,
				interlacingLeftWeight,
			)
		case 20, 32:
			logger.Debug("ALAC: unimplemented sample size", "size", d.CookieSampleSize)
		}
		return outbuffer

	default:
		logger.Debug("ALAC: unimplemented channel count", "channels", channels+1)
	}

	return nil
}
