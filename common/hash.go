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

func Blake2Hash(data []byte) Hash {
	return BytesToHash(ComputeHash(data))
}

func Keccak256(data []byte) Hash {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	h := hash.Sum(nil)
	return BytesToHash(h)
}
