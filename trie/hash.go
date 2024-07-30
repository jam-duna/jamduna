package trie

import (
	//"errors"
	//"math"
	"hash"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/crypto/blake2b"
)

func BytesToHash(data []byte) common.Hash {
	return common.BytesToHash(data[:])
}

func createHash() hash.Hash {
	h, _ := blake2b.New256(nil)
	return h
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
func computeHash(data []byte) []byte {
	h, _ := blake2b.New256(nil)
	h.Write(data)
	return h.Sum(nil)
}

// computeNode hashes the data with $node on WBT, CDT using Blake2b-256
func computeNode(data []byte) []byte {
	h, _ := blake2b.New256(nil)
	h.Write([]byte("node"))
	h.Write(data)
	return h.Sum(nil)
}

// computeLeaf hashes the data with $leaf on CDT using Blake2b-256
func computeLeaf(data []byte) []byte {
	h, _ := blake2b.New256(nil)
	h.Write([]byte("leaf"))
	h.Write(data)
	return h.Sum(nil)
}

func hashNodes(left, right *Node) []byte {
	h, _ := blake2b.New256(nil)
	h.Write([]byte("node"))
	h.Write(left.Hash)
	h.Write(right.Hash)
	return h.Sum(nil)
}
