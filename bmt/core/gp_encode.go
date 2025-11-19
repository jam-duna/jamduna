package core

import "encoding/hex"

// EncodeGpInternal encodes an internal node according to GP (Gray Paper) Equation D.3.
//
// GP Branch Encoding (Equation D.3):
// The encoding is [left' || right] where left' has its MSB cleared.
//
// This is a critical difference from standard NOMT:
//   - NOMT clears MSB *after* hashing
//   - GP clears MSB *before* hashing
func EncodeGpInternal(data *InternalData) [64]byte {
	var encoded [64]byte

	// Copy left child with MSB cleared
	leftPrime := data.Left
	UnsetMSB(&leftPrime)
	copy(encoded[:32], leftPrime[:])

	// Copy right child as-is
	copy(encoded[32:], data.Right[:])

	if debugGP {
		println("[GP_ENCODE_INTERNAL]",
			"left=", hex.EncodeToString(data.Left[:8])+"...",
			"left'=", hex.EncodeToString(leftPrime[:8])+"...",
			"right=", hex.EncodeToString(data.Right[:8])+"...")
	}

	return encoded
}

// EncodeGpLeaf encodes a leaf node according to GP (Gray Paper) Equation D.4.
//
// GP Leaf Encoding (Equation D.4):
//   - Embedded (≤32 bytes): [0b10xxxxxx | 31-byte key | value bytes]
//     where xxxxxx is the 6-bit value length
//   - Hashed (>32 bytes):   [0b11000000 | 31-byte key | 32-byte hash]
//
// The key difference from standard NOMT:
//   - Uses 31-byte keys (not 32)
//   - Embeds small values directly (not hashed)
//   - Tag byte indicates embedded vs hashed
func EncodeGpLeaf(data *LeafData) [64]byte {
	var encoded [64]byte

	isEmbedded := data.ValueLen <= 32

	if isEmbedded {
		// Embedded value: 0b10 + 6-bit length
		length := uint8(data.ValueLen)
		encoded[0] = 0b10000000 | (length & 0b00111111)

		// Copy 31-byte key
		copy(encoded[1:32], data.KeyPath[:31])

		// Copy embedded value bytes
		if data.ValueInline != nil {
			copy(encoded[32:], data.ValueInline)
		}
	} else {
		// Hashed value: 0b11000000
		encoded[0] = 0b11000000

		// Copy 31-byte key
		copy(encoded[1:32], data.KeyPath[:31])

		// Copy value hash
		copy(encoded[32:64], data.ValueHash[:])
	}

	return encoded
}
