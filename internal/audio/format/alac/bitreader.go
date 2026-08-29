package alac

func readUint16BE(b []byte) uint16 {
	return uint16(b[0])<<8 | uint16(b[1])
}

func readUint32BE(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
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
	result <<= uint(d.input_buffer_bitaccumulator)
	result &= 0x00ffffff
	result >>= uint(24 - bits)

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
		// #nosec G115 -- 16-bit read sign-extended to int32; value is bounded.
		result = int32(d.readbits_16(16) << uint(bits))
	}
	// #nosec G115 -- low bits reinterpreted as int32; value is bounded.
	result |= int32(d.readbits_16(bits))
	// #nosec G115 -- bit-pattern reinterpretation of a 32-bit value.
	return uint32(result)
}

func (d *Decoder) readbit() int {
	// Guard: need at least 1 byte available.
	if d.input_buffer_index >= len(d.input_buffer) {
		return 0
	}
	result := int(d.input_buffer[d.input_buffer_index])
	result <<= uint(d.input_buffer_bitaccumulator)
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
