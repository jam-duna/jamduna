package common

import (
	"encoding/binary"
	"golang.org/x/crypto/blake2b"
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

func Blake2AsHex(data []byte) Hash {
	//hash := blake2b.Sum256([]byte(data))
	//return hex.EncodeToString(hash[:])
	return BytesToHash(ComputeHash(data))
}

func Blake2Hash(data []byte) Hash {
	return BytesToHash(ComputeHash(data))
}

// Segment represents a segment of data
type Segment struct {
	Data []byte
}
