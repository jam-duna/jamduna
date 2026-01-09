package ubt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStemBit validates Stem.Bit() method for accessing bits.
// Bit(0) is MSB of first byte, Bit(247) is LSB of last byte.
func TestStemBit(t *testing.T) {
	stem := Stem{0b10000000} // MSB set in first byte

	assert.Equal(t, uint8(1), stem.Bit(0), "Bit(0) should be 1 (MSB set)")
	assert.Equal(t, uint8(0), stem.Bit(1), "Bit(1) should be 0")
	assert.Equal(t, uint8(0), stem.Bit(7), "Bit(7) should be 0 (LSB of first byte)")
}

// TestFirstDifferingBit validates finding the first bit where two stems differ.
func TestFirstDifferingBit(t *testing.T) {
	stem1 := Stem{}     // all zeros
	stem2 := Stem{0x80} // first bit set (0b10000000)

	diff := stem1.FirstDifferingBit(&stem2)
	assert.NotNil(t, diff, "FirstDifferingBit should return Some for different stems")
	assert.Equal(t, 0, *diff, "First differing bit should be 0 (MSB of first byte)")

	stem3 := Stem{0x01} // last bit of first byte set
	diff2 := stem1.FirstDifferingBit(&stem3)
	assert.NotNil(t, diff2, "FirstDifferingBit should return Some")
	assert.Equal(t, 7, *diff2, "First differing bit should be 7 (LSB of first byte)")

	// Identical stems should return None
	stem4 := Stem{}
	diff3 := stem1.FirstDifferingBit(&stem4)
	assert.Nil(t, diff3, "FirstDifferingBit should return None for identical stems")
}

// TestTreeKeyRoundtrip validates TreeKeyFromBytes and ToBytes are inverses.
func TestTreeKeyRoundtrip(t *testing.T) {
	original := [32]byte{}
	for i := range original {
		original[i] = 0x42
	}

	key := TreeKeyFromBytes(original)
	recovered := key.ToBytes()

	assert.Equal(t, original, recovered, "TreeKey roundtrip must preserve bytes")
}

// TestTreeKeyStemSubindexSplit validates that TreeKey correctly splits stem and subindex.
func TestTreeKeyStemSubindexSplit(t *testing.T) {
	var keyBytes [32]byte
	// First 31 bytes = stem
	for i := 0; i < 31; i++ {
		keyBytes[i] = byte(i + 1)
	}
	// Last byte = subindex
	keyBytes[31] = 42

	key := TreeKeyFromBytes(keyBytes)

	// Verify stem
	for i := 0; i < 31; i++ {
		assert.Equal(t, byte(i+1), key.Stem[i], "Stem byte %d should match", i)
	}

	// Verify subindex
	assert.Equal(t, uint8(42), key.Subindex, "Subindex should be 42")
}

// TestStemZero validates Stem.IsZero() method.
func TestStemZero(t *testing.T) {
	stem1 := Stem{} // all zeros
	assert.True(t, stem1.IsZero(), "All-zero stem should return true for IsZero()")

	stem2 := Stem{0x01} // one non-zero byte
	assert.False(t, stem2.IsZero(), "Non-zero stem should return false for IsZero()")
}

// TestStemOrdering validates that Stem implements lexicographic ordering.
func TestStemOrdering(t *testing.T) {
	stem1 := Stem{0x00}
	stem2 := Stem{0x01}
	stem3 := Stem{0xFF}

	// stem1 < stem2
	assert.True(t, stem1.Less(&stem2), "Stem{0x00} < Stem{0x01}")
	assert.False(t, stem2.Less(&stem1), "Stem{0x01} not < Stem{0x00}")

	// stem2 < stem3
	assert.True(t, stem2.Less(&stem3), "Stem{0x01} < Stem{0xFF}")

	// stem1 == stem1
	stem1Copy := Stem{0x00}
	assert.False(t, stem1.Less(&stem1Copy), "Equal stems should not be less than each other")
	assert.Equal(t, stem1, stem1Copy, "Equal stems should be equal")
}

// TestTreeKeyConstruction validates TreeKey.New constructor.
func TestTreeKeyConstruction(t *testing.T) {
	stem := Stem{0x42}
	subindex := uint8(10)

	key := TreeKey{Stem: stem, Subindex: subindex}

	assert.Equal(t, stem, key.Stem, "Stem should match")
	assert.Equal(t, subindex, key.Subindex, "Subindex should match")

	// Verify ToBytes
	keyBytes := key.ToBytes()
	assert.Equal(t, byte(0x42), keyBytes[0], "First byte should be 0x42")
	assert.Equal(t, byte(10), keyBytes[31], "Last byte should be 10")
}
