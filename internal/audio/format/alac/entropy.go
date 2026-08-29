package alac

func (d *Decoder) entropyDecodeValue(readSampleSize int, k int, riceKModifierMask int) int32 {
	x := int32(0)
	for x <= riceThreshold && d.readbit() != 0 {
		x++
	}
	if x > riceThreshold {
		// #nosec G115 -- readbits returns a bounded value within readSampleSize bits.
		value := int32(d.readbits(readSampleSize))
		value &= int32((uint32(0xffffffff) >> uint(32-readSampleSize)))
		x = value
	} else if k != 1 {
		extraBits := int(d.readbits(k))
		// #nosec G115 -- rice k modifier is bounded by the mask width.
		x *= int32((((1 << uint(k)) - 1) & riceKModifierMask))
		if extraBits > 1 {
			// #nosec G115 -- extraBits-1 is a small bounded value.
			x += int32(extraBits - 1)
		} else {
			d.unreadbits(1)
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
		// #nosec G115 -- k is a small rice parameter bounded by the cookie values.
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
