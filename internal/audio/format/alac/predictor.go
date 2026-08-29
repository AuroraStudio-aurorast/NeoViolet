package alac

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
			outVal >>= uint(predictorQuantitization)
			outVal = outVal + int(bufferOut[0]) + int(errorVal)
			outVal = int(signExtended32(int32(outVal), readSampleSize)) // #nosec G115

			bufferOut[predictorCoefNum+1] = int32(outVal) // #nosec G115

			if errorVal > 0 {
				for predictorNum := predictorCoefNum - 1; predictorNum >= 0 && errorVal > 0; predictorNum-- {
					val := int(bufferOut[0] - bufferOut[predictorCoefNum-predictorNum])
					sign := signOnly(val)
					predictorCoefTable[predictorNum] -= int16(sign) // #nosec G115 -- sign is -1/0/1, bounded
					val *= sign
					errorVal -= int32((val >> uint(predictorQuantitization)) * (predictorCoefNum - predictorNum)) // #nosec G115
				}
			} else if errorVal < 0 {
				for predictorNum := predictorCoefNum - 1; predictorNum >= 0 && errorVal < 0; predictorNum-- {
					val := int(bufferOut[0] - bufferOut[predictorCoefNum-predictorNum])
					sign := -signOnly(val)
					predictorCoefTable[predictorNum] -= int16(sign) // #nosec G115 -- sign is -1/0/1, bounded
					val *= sign
					errorVal -= int32((val >> uint(predictorQuantitization)) * (predictorCoefNum - predictorNum)) // #nosec G115
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
			// #nosec G115 -- deinterlaced sample narrowed to int16; bounded.
			right := int16(midright - ((difference * int32(interlacingLeftWeight)) >> interlacingShift))
			left := right + int16(difference) // #nosec G115

			bufferOut[2*i*numChannels] = byte(left) // #nosec G115 -- low byte of a 16-bit sample
			bufferOut[2*i*numChannels+1] = byte(left >> 8) // #nosec G115
			bufferOut[2*i*numChannels+2] = byte(right) // #nosec G115
			bufferOut[2*i*numChannels+3] = byte(right >> 8) // #nosec G115
		}
		return
	}

	for i := 0; i < numSamples; i++ {
		left := int16(bufferA[i]) // #nosec G115
		right := int16(bufferB[i]) // #nosec G115

		bufferOut[2*i*numChannels] = byte(left) // #nosec G115
		bufferOut[2*i*numChannels+1] = byte(left >> 8) // #nosec G115
		bufferOut[2*i*numChannels+2] = byte(right) // #nosec G115
		bufferOut[2*i*numChannels+3] = byte(right >> 8) // #nosec G115
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
				left |= uncompressedBytesBufferA[i] & int32(mask) // #nosec G115
				right |= uncompressedBytesBufferB[i] & int32(mask) // #nosec G115
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
			left |= uncompressedBytesBufferA[i] & int32(mask) // #nosec G115
			right |= uncompressedBytesBufferB[i] & int32(mask) // #nosec G115
		}

		bufferOut[i*numChannels*3] = byte(left & 0xFF)
		bufferOut[i*numChannels*3+1] = byte((left >> 8) & 0xFF)
		bufferOut[i*numChannels*3+2] = byte((left >> 16) & 0xFF)
		bufferOut[i*numChannels*3+3] = byte(right & 0xFF)
		bufferOut[i*numChannels*3+4] = byte((right >> 8) & 0xFF)
		bufferOut[i*numChannels*3+5] = byte((right >> 16) & 0xFF)
	}
}
