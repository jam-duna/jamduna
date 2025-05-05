package trie

import (
	"hash"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/sha3"
)

func BytesToHash(data []byte) common.Hash {
	return common.BytesToHash(data[:])
}

var EMPTYHASH = make([]byte, 32)

// compareHashes compares two byte slices for equality
func compareBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// computeHash hashes the data using Blake2b-256
func computeHash(data []byte, hashType ...string) []byte {
	var h hash.Hash
	// Check if "keccak" is passed in hashType, else use default Blake2b-256.
	if len(hashType) > 0 && hashType[0] == types.Keccak {
		h = sha3.NewLegacyKeccak256()
	} else {
		h, _ = blake2b.New256(nil)
	}
	h.Write(data)
	return h.Sum(nil)
}

// computeNode hashes the data with $node on WBT, CDT using Blake2b-256
func computeNode(data []byte, hashType ...string) []byte {
	var h hash.Hash
	// Check if "keccak" is passed in hashType, else use default Blake2b-256.
	if len(hashType) > 0 && hashType[0] == types.Keccak {
		h = sha3.NewLegacyKeccak256()
	} else {
		h, _ = blake2b.New256(nil)
	}
	h.Write([]byte("node"))
	h.Write(data)
	return h.Sum(nil)
}

// computeLeaf hashes the data with $leaf on CDT using Blake2b-256
// related about Equation(E.6) in GP 0.6.2
func computeLeaf(data []byte, hashType ...string) []byte {
	var h hash.Hash
	// Check if "keccak" is passed in hashType, else use default Blake2b-256.
	if len(hashType) > 0 && hashType[0] == types.Keccak {
		h = sha3.NewLegacyKeccak256()
	} else {
		h, _ = blake2b.New256(nil)
	}
	if len(hashType) > 0 && hashType[0] == "keccak" {
		// this is the WBT case
	} else {
		h.Write([]byte("leaf"))
	}
	h.Write(data)
	return h.Sum(nil)
}

func ComputeLeaf(data []byte, hashType ...string) []byte {
	return computeLeaf(data, hashType...)
}

// eq 187
func PadToMultipleOfN(x []byte, n int) []byte {
	if n <= 0 {
		return x // If n is not positive, return the original slice
	}
	paddingSize := (n - (len(x) % n)) % n // Calculate how many zeros to add
	if paddingSize == 0 {
		return x // Already a multiple of n
	}
	padded := make([]byte, len(x)+paddingSize)
	copy(padded, x) // Copy original slice to the new padded slice
	return padded
}
