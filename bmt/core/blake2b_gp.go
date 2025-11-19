package core

import (
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/blake2b"
)

// Blake2bBinaryHasher implements BinaryHasher using Blake2b-256.
type Blake2bBinaryHasher struct{}

// Hash implements BinaryHasher.
func (Blake2bBinaryHasher) Hash(input []byte) [32]byte {
	// Use Blake2b with 32-byte (256-bit) output
	hash := blake2b.Sum256(input)
	return hash
}

// Hash2x32Concat implements BinaryHasher for two 32-byte inputs.
func (Blake2bBinaryHasher) Hash2x32Concat(left, right [32]byte) [32]byte {
	// Concatenate and hash
	buf := make([]byte, 64)
	copy(buf[:32], left[:])
	copy(buf[32:], right[:])
	return blake2b.Sum256(buf)
}

// GpNodeHasher is a GP-compliant node hasher that follows Gray Paper Appendix D encoding.
//
// This hasher produces the exact same node hashes as JAM's Go BPT implementation
// by following GP equations D.3 (branch) and D.4 (leaf).
//
// Key differences from standard NOMT:
//   - Uses 31-byte keys (not 32)
//   - Embeds small values (<= 32 bytes) in leaf nodes
//   - Clears MSB of left child *before* hashing (not after)
//   - No MSB manipulation on output hashes
type GpNodeHasher struct{}

// NewGpNodeHasher creates a new GP-compliant node hasher.
func NewGpNodeHasher() *GpNodeHasher {
	return &GpNodeHasher{}
}

// HashLeaf implements NodeHasher for GP encoding.
//
// GP Leaf Encoding (Equation D.4):
//   - Embedded (≤32 bytes): [0b10xxxxxx | 31-byte key | value bytes]
//   - Hashed (>32 bytes):   [0b11000000 | 31-byte key | 32-byte hash]
func (h *GpNodeHasher) HashLeaf(data *LeafData) Node {
	var encoded [64]byte

	// Determine encoding based on value length
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

			// GP DEBUG: hash_leaf for embedded values
			if debugGP {
				fmt.Printf("[GP_HASH_LEAF] EMBEDDED: key_prefix=%s, len=%d, value_prefix=%s, tag=%02x\n",
					hex.EncodeToString(data.KeyPath[:4]), length,
					hex.EncodeToString(data.ValueInline[:min(len(data.ValueInline), 8)]),
					encoded[0])
			}
		} else {
			// GP DEBUG: Metadata issue warning
			if debugGP {
				fmt.Printf("[GP_HASH_LEAF] WARNING: EMBEDDED but value_inline is nil! len=%d\n", length)
			}
		}
	} else {
		// Hashed value: 0b11000000
		encoded[0] = 0b11000000

		// Copy 31-byte key
		copy(encoded[1:32], data.KeyPath[:31])

		// Copy value hash
		copy(encoded[32:64], data.ValueHash[:])
	}

	// Hash the 64-byte encoding (no MSB manipulation)
	return blake2b.Sum256(encoded[:])
}

// HashInternal implements NodeHasher for GP encoding.
//
// GP Branch Encoding (Equation D.3):
// [left' (MSB cleared) || right]
func (h *GpNodeHasher) HashInternal(data *InternalData) Node {
	encoded := EncodeGpInternal(data)
	// Hash the 64-byte encoding (no MSB manipulation)
	return blake2b.Sum256(encoded[:])
}

// NodeKind implements NodeHasher but has limited functionality in GP mode.
//
// IMPORTANT LIMITATION: GP encoding does not encode node type in the hash.
// Node types (leaf vs internal) must be tracked externally by the caller.
// This method can ONLY distinguish terminators from non-terminators.
//
// Callers should NOT use IsLeaf() or IsInternal() helpers with GP hasher,
// as they will give incorrect results. Instead, track node types separately
// during tree construction and traversal.
//
// Returns:
//   - NodeKindTerminator if the node is all zeros
//   - NodeKindInternal for all other nodes (this is INCORRECT for actual leaves!)
func (h *GpNodeHasher) NodeKind(node Node) NodeKind {
	if IsTerminator(node) {
		return NodeKindTerminator
	}
	// BUG: Cannot distinguish leaf from internal in GP mode.
	// Callers must track node kind externally.
	return NodeKindInternal
}

// HashValue implements ValueHasher.
func (h *GpNodeHasher) HashValue(value []byte) ValueHash {
	return blake2b.Sum256(value)
}

// Debug flag for GP encoding (can be toggled for debugging)
var debugGP = false

// SetGpDebug enables or disables GP debug logging.
func SetGpDebug(enabled bool) {
	debugGP = enabled
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
