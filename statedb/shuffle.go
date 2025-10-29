package statedb

import (
	"encoding/binary"

	"github.com/colorfulnotion/jam/common"
)

// FisherYatesShuffle performs the Fisher-Yates shuffle on a slice of integers.
func FisherYatesShuffle(s []uint32, r []uint32) []uint32 {
	if len(s) > 0 {
		l := uint32(len(s))
		index := r[0] % l
		head := s[index]

		// Create a copy of the slice
		sPost := append([]uint32(nil), s...)
		sPost[index] = s[l-1]

		// Recursive call with updated slice and remaining `r`
		return append([]uint32{head}, FisherYatesShuffle(sPost[:l-1], r[1:])...)
	}
	return []uint32{}
}

// NumericSequenceFromHash generates a deterministic numeric sequence from a 32-byte hash.
func NumericSequenceFromHash(h [32]byte, l uint32) []uint32 {
	result := make([]uint32, l)
	bytes := make([]byte, 4)
	for i := uint32(0); i < l; i++ {
		offset := (4 * i) % 32
		binary.LittleEndian.PutUint32(bytes, uint32(i>>3)) // i/8
		hash := common.Blake2Hash(append(h[:], bytes...)).Bytes()
		result[i] = binary.LittleEndian.Uint32(hash[offset : offset+4])
	}

	return result
}

// ShuffleFromHash performs the Fisher-Yates shuffle based on a hash value.
func ShuffleFromHash(sequence []uint32, hash common.Hash) []uint32 {
	randomSequence := NumericSequenceFromHash(hash, uint32(len(sequence)))
	return FisherYatesShuffle(sequence, randomSequence)
}
