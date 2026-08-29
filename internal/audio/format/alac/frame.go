package alac

import (
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

const riceThreshold = 8

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
				logger.Debug("ALAC: unhandled prediction type", "type", prediction_type)
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
			logger.Debug("ALAC: unimplemented sample size", "size", d.CookieSampleSize)
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
				logger.Debug("ALAC: unhandled prediction type", "type", predictionTypeA)
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
				logger.Debug("ALAC: unhandled prediction type", "type", predictionTypeB)
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
			logger.Debug("ALAC: unimplemented sample size", "size", d.CookieSampleSize)
		}
		return outbuffer

	default:
		logger.Debug("ALAC: unimplemented channel count", "channels", channels+1)
	}

	return nil
}
