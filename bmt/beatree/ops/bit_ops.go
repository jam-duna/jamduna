package ops

import (
	"github.com/colorfulnotion/jam/bmt/beatree"
)

// Separate creates a separator key between two keys a and b where b > a.
// The separator is the shortest prefix of b that distinguishes it from a.
func Separate(a, b *beatree.Key) beatree.Key {
	// b > a means at some point b has a 1 where a has a 0, and they are equal up to that point
	bitLen := PrefixLen(a, b) + 1
	var separator beatree.Key

	fullBytes := bitLen / 8
	copy(separator[:fullBytes], b[:fullBytes])

	remaining := bitLen % 8
	if remaining != 0 {
		mask := byte(^((1 << (8 - remaining)) - 1))
		separator[fullBytes] = b[fullBytes] & mask
	}

	return separator
}

// PrefixLen returns the number of bits that are common between key_a and key_b.
func PrefixLen(keyA, keyB *beatree.Key) int {
	bitLen := 0
	for byteIdx := 0; byteIdx < 32; byteIdx++ {
		for bit := 0; bit < 8; bit++ {
			mask := byte(1 << (7 - bit))
			if (keyA[byteIdx] & mask) != (keyB[byteIdx] & mask) {
				return bitLen
			}
			bitLen++
		}
	}
	return bitLen
}

// SeparatorLen returns the bit length of a separator key.
// The separator length is the number of bits from the start up to and including the last 1 bit.
func SeparatorLen(key *beatree.Key) int {
	if *key == (beatree.Key{}) {
		return 1
	}

	trailingZeros := 0
	for byteIdx := 31; byteIdx >= 0; byteIdx-- {
		for bit := 7; bit >= 0; bit-- {
			mask := byte(1 << (7 - bit))
			if (key[byteIdx] & mask) == mask {
				return 256 - trailingZeros
			}
			trailingZeros++
		}
	}

	return 256 - trailingZeros
}

// RawPrefix represents a prefix as (bytes, bit_length).
type RawPrefix struct {
	Bytes  []byte
	BitLen int
}

// RawSeparator represents a separator as (bytes, bit_start, bit_length).
type RawSeparator struct {
	Bytes    []byte
	BitStart int
	BitLen   int
}

// ReconstructKey rebuilds a key from a prefix and a misaligned separator.
// The prefix and separator may have bit-level alignment, not just byte alignment.
func ReconstructKey(maybePrefix *RawPrefix, separator RawSeparator) beatree.Key {
	var key beatree.Key

	var prefixBitLen int
	if maybePrefix != nil {
		prefixBitLen = maybePrefix.BitLen
	}

	prefixByteLen := (prefixBitLen + 7) / 8
	prefixEndBitOffset := prefixBitLen % 8
	separatorBytes := separator.Bytes
	separatorBitStart := separator.BitStart
	separatorBitLen := separator.BitLen

	// Where the separator will start to be stored
	startDestination := 0
	if prefixByteLen > 0 {
		if prefixEndBitOffset == 0 {
			startDestination = prefixByteLen
		} else {
			// Overlap between the end of the prefix and the beginning of the separator
			startDestination = prefixByteLen - 1
		}
	}

	// Shift the separator and store it in the key
	BitwiseMemcpy(
		key[startDestination:],
		prefixEndBitOffset,
		separatorBytes,
		separatorBitStart,
		separatorBitLen,
	)

	if prefixByteLen != 0 && maybePrefix != nil {
		// Copy the prefix into the key up to the penultimate byte
		copy(key[:prefixByteLen-1], maybePrefix.Bytes[:prefixByteLen-1])

		// Copy the last byte of the prefix without interfering with the separator
		maskShift := 8 - uint(prefixBitLen%8)
		var mask byte = 255
		if maskShift < 8 {
			mask = ^((1 << maskShift) - 1)
		}
		key[prefixByteLen-1] |= maybePrefix.Bytes[prefixByteLen-1] & mask
	}

	return key
}

// BitwiseMemcpy copies source_bit_len bits from source (starting at source_bit_start)
// to destination (starting at destination_bit_start).
//
// This handles bit-level copying with arbitrary alignment.
// All bits in destination not involved in the copy are left unchanged.
func BitwiseMemcpy(
	destination []byte,
	destinationBitStart int,
	source []byte,
	sourceBitStart int,
	sourceBitLen int,
) {
	if sourceBitLen == 0 {
		return
	}

	type shift struct {
		isLeft        bool
		amount        int
		prevRemainder byte
		currRemainder byte
	}

	var shiftOp *shift
	bitDiff := destinationBitStart - sourceBitStart
	if bitDiff != 0 {
		if bitDiff < 0 {
			shiftOp = &shift{isLeft: true, amount: -bitDiff}
		} else {
			shiftOp = &shift{isLeft: false, amount: bitDiff}
		}
	}

	nChunks := len(source) / 8
	bytesToWrite := (destinationBitStart + sourceBitLen + 7) / 8

	var chunkData uint64
	hasChunkData := false

	destinationOffset := 0
	for chunkIndex := 0; chunkIndex < nChunks; chunkIndex++ {
		chunkStart := chunkIndex * 8

		// Handle right shift remainder
		if shiftOp != nil && !shiftOp.isLeft {
			mask := byte((1 << shiftOp.amount) - 1)
			if chunkIndex == nChunks-1 {
				// Remove garbage of the last chunk from the remainder
				lastMask := lastChunkMask(sourceBitStart, sourceBitLen, nChunks)
				mask &= byte(lastMask >> 56)
			}
			bits := source[chunkStart+7] & mask
			shiftOp.currRemainder = bits << (8 - shiftOp.amount)
		}

		// Read 8-byte chunk as big-endian uint64
		chunk := uint64(source[chunkStart])<<56 |
			uint64(source[chunkStart+1])<<48 |
			uint64(source[chunkStart+2])<<40 |
			uint64(source[chunkStart+3])<<32 |
			uint64(source[chunkStart+4])<<24 |
			uint64(source[chunkStart+5])<<16 |
			uint64(source[chunkStart+6])<<8 |
			uint64(source[chunkStart+7])

		// Apply masks for first and last chunks
		var maybeMasks *struct{ maskFrom, maskTo uint64 }
		if chunkIndex == 0 {
			maybeMasks = &struct{ maskFrom, maskTo uint64 }{
				maskFrom: firstChunkMask(sourceBitStart),
				maskTo:   firstChunkMask(destinationBitStart),
			}
		}
		if chunkIndex == nChunks-1 {
			maskFrom := uint64(0xFFFFFFFFFFFFFFFF)
			maskTo := uint64(0xFFFFFFFFFFFFFFFF)
			if maybeMasks != nil {
				maskFrom = maybeMasks.maskFrom
				maskTo = maybeMasks.maskTo
			}
			maybeMasks = &struct{ maskFrom, maskTo uint64 }{
				maskFrom: maskFrom & lastChunkMask(sourceBitStart, sourceBitLen, nChunks),
				maskTo:   maskTo & lastChunkMask(destinationBitStart, sourceBitLen, nChunks),
			}
		}

		if maybeMasks != nil {
			// Read destination chunk
			nByte := min(8, len(destination)-destinationOffset)
			var destBuf [8]byte
			copy(destBuf[:nByte], destination[destinationOffset:destinationOffset+nByte])

			destChunk := uint64(destBuf[0])<<56 |
				uint64(destBuf[1])<<48 |
				uint64(destBuf[2])<<40 |
				uint64(destBuf[3])<<32 |
				uint64(destBuf[4])<<24 |
				uint64(destBuf[5])<<16 |
				uint64(destBuf[6])<<8 |
				uint64(destBuf[7])

			chunkData = destChunk & ^maybeMasks.maskTo
			hasChunkData = true
			chunk &= maybeMasks.maskFrom
		}

		// Apply shift
		if shiftOp != nil {
			if shiftOp.isLeft {
				chunk <<= shiftOp.amount
			} else {
				chunk >>= shiftOp.amount
			}
		}

		// Merge with preserved data
		if hasChunkData {
			chunk |= chunkData
			hasChunkData = false
		}

		// Convert back to bytes (big-endian)
		var chunkShifted [8]byte
		chunkShifted[0] = byte(chunk >> 56)
		chunkShifted[1] = byte(chunk >> 48)
		chunkShifted[2] = byte(chunk >> 40)
		chunkShifted[3] = byte(chunk >> 32)
		chunkShifted[4] = byte(chunk >> 24)
		chunkShifted[5] = byte(chunk >> 16)
		chunkShifted[6] = byte(chunk >> 8)
		chunkShifted[7] = byte(chunk)

		// Move bits remainder between chunk boundaries
		if shiftOp != nil {
			if shiftOp.isLeft && chunkIndex < nChunks-1 {
				// Handle left shift remainder
				mask := byte(255)
				unchangedBits := byte(0)
				if destinationOffset+8 == bytesToWrite {
					mask = byte(lastChunkMask(sourceBitStart, sourceBitLen, nChunks) >> 56)
					bitsToKeep := (bytesToWrite * 8) - (destinationBitStart + sourceBitLen)
					maskTo := byte((1 << bitsToKeep) - 1)
					unchangedBits = destination[destinationOffset+7] & maskTo
				}

				remainderBits := (source[(chunkIndex+1)*8] & mask) >> (8 - shiftOp.amount)
				chunkShifted[7] |= remainderBits | unchangedBits
			} else if !shiftOp.isLeft {
				// Handle right shift remainder
				chunkShifted[0] |= shiftOp.prevRemainder
				shiftOp.prevRemainder = shiftOp.currRemainder
			}
		}

		// Write to destination
		nByte := min(8, len(destination)-destinationOffset)
		copy(destination[destinationOffset:destinationOffset+nByte], chunkShifted[:nByte])
		destinationOffset += nByte

		// Break if destination is fully written
		if destinationOffset >= bytesToWrite {
			break
		}
	}

	// Handle possible right remainder left unapplied
	if destinationOffset < bytesToWrite {
		if shiftOp != nil && !shiftOp.isLeft {
			garbageBits := (nChunks * 64) - (sourceBitStart + sourceBitLen)
			mask := byte((1 << (8 - (shiftOp.amount - garbageBits))) - 1)

			destination[destinationOffset] &= mask
			destination[destinationOffset] |= shiftOp.prevRemainder & ^mask
		}
	}
}

// firstChunkMask returns a mask for the first chunk based on bit_start.
func firstChunkMask(bitStart int) uint64 {
	maskShift := uint(7-bitStart) + 1 + (8 * 7)
	if maskShift >= 64 {
		return 0xFFFFFFFFFFFFFFFF
	}
	return (1 << maskShift) - 1
}

// lastChunkMask returns a mask for the last chunk.
func lastChunkMask(bitStart, bitLen, nChunks int) uint64 {
	usedLastChunkBits := bitStart + bitLen - (nChunks-1)*64
	if usedLastChunkBits < 0 {
		usedLastChunkBits = 0
	}

	shift := 64 - usedLastChunkBits
	if shift >= 64 {
		return 0
	}
	return ^((1 << shift) - 1)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
