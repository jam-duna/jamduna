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

func Uint64ToBytes(val uint64) []byte {
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, val)
	return bytes
}

func Uint32ToBytes(val uint32) []byte {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, val)
	return bytes
}

func Uint16ToBytes(value uint16) []byte {
	bytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(bytes, value)
	return bytes
}

func BytesToUint32(data []byte) uint32 {
	if len(data) < 4 {
		// Handle the error according to your application's needs
		panic("BytesToUint32: byte slice too short")
	}
	return binary.LittleEndian.Uint32(data)
}

func BytesToUint16(data []byte) uint16 {
	if len(data) < 2 {
		// Handle the error according to your application's needs
		panic("BytesToUint16: byte slice too short")
	}
	return binary.LittleEndian.Uint16(data)
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
	if n <= 0 {
		return input
	}
	paddingSize := (n - (len(input) % n)) % n
	if paddingSize == 0 {
		return input
	}
	padded := make([]byte, len(input)+paddingSize)
	copy(padded, input)
	return padded
}

// used for justification.. with $leaf as salt
func ComputeLeafHash_WBT_Blake2B(data []byte) Hash {
	h, _ := blake2b.New256(nil)
	h.Write([]byte("leaf"))
	h.Write(data)
	return Hash(h.Sum(nil))
}

func IsNilHash(h Hash) bool {
	return h == Hash{}
}
