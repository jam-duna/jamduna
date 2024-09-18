package common

import (
	"encoding/binary"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/sha3"
)

// computeHash computes the BLAKE2b hash of the given data
func ComputeHash(data []byte) []byte {
	hash := blake2b.Sum256(data)
	return hash[:]
}

func Uint32ToBytes(val uint32) []byte {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, val)
	return bytes
}

func Uint16ToBytes(value uint16) []byte {
	bytes := make([]byte, 2)
	binary.BigEndian.PutUint16(bytes, value)
	return bytes
}


func Blake2Hash(data []byte) Hash {
	return BytesToHash(ComputeHash(data))
}

func Keccak256(data []byte) Hash {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	h := hash.Sum(nil)
	return BytesToHash(h)
}

// Pad the input to the specified length
func PadToMultipleOfN(input []byte, n int) []byte {
	return padToMultipleOfN(input, n)
}

func padToMultipleOfN(input []byte, n int) []byte {
	length := len(input)
	mod := (length+n-1)%n + 1

	// Calculate the padding
	padding := 0
	if mod != 0 {
		padding = n - mod
	}

	// Fill the padding
	for i := 0; i < padding; i++ {
		input = append(input, 0)
	}
	return input
}
