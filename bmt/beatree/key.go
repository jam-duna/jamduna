package beatree

import (
	"bytes"
	"encoding/hex"
)

// Key is a 32-byte key for beatree storage.
// Keys are ordered lexicographically.
type Key [32]byte

// Compare compares two keys lexicographically.
// Returns -1 if k < other, 0 if k == other, 1 if k > other.
func (k Key) Compare(other Key) int {
	return bytes.Compare(k[:], other[:])
}

// Equal returns true if two keys are equal.
func (k Key) Equal(other Key) bool {
	return k == other
}

// String returns a hex representation of the key.
func (k Key) String() string {
	return hex.EncodeToString(k[:])
}

// KeyFromBytes creates a Key from a byte slice.
// Panics if the slice is not exactly 32 bytes.
func KeyFromBytes(b []byte) Key {
	if len(b) != 32 {
		panic("key must be exactly 32 bytes")
	}
	var k Key
	copy(k[:], b)
	return k
}
