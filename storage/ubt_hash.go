package storage

import "github.com/zeebo/blake3"

// Hasher provides UBT hashing operations.
type Hasher interface {
	// Hash32 hashes a 32-byte value (leaf nodes, profile-defined).
	Hash32(value *[32]byte) [32]byte
	// Hash64 hashes a 64-byte value (internal nodes, profile-defined).
	// MUST return ZERO32 if left==ZERO32 && right==ZERO32 (all profiles).
	Hash64(left, right *[32]byte) [32]byte
	// HashStemNode hashes stem nodes (profile-defined).
	HashStemNode(stem *Stem, subtreeRoot *[32]byte) [32]byte
	// HashRaw hashes arbitrary input.
	HashRaw(input []byte) [32]byte
}

// Profile selects the hash/profile semantics.
type Profile int

const (
	EIPProfile Profile = iota
	JAMProfile
)

// Blake3Hasher implements Hasher with Blake3.
type Blake3Hasher struct {
	profile Profile
}

func NewBlake3Hasher(profile Profile) *Blake3Hasher {
	return &Blake3Hasher{profile: profile}
}

func (h *Blake3Hasher) Hash32(value *[32]byte) [32]byte {
	if h.profile == JAMProfile {
		var buf [33]byte
		buf[0] = 0x00
		copy(buf[1:], value[:])
		return blake3.Sum256(buf[:])
	}
	return blake3.Sum256(value[:])
}

func (h *Blake3Hasher) Hash64(left, right *[32]byte) [32]byte {
	var zero [32]byte
	if *left == zero && *right == zero {
		return zero
	}
	if h.profile == JAMProfile {
		var buf [65]byte
		buf[0] = 0x01
		copy(buf[1:33], left[:])
		copy(buf[33:], right[:])
		return blake3.Sum256(buf[:])
	}
	var buf [64]byte
	copy(buf[:32], left[:])
	copy(buf[32:], right[:])
	return blake3.Sum256(buf[:])
}

func (h *Blake3Hasher) HashStemNode(stem *Stem, subtreeRoot *[32]byte) [32]byte {
	if h.profile == JAMProfile {
		var buf [65]byte
		buf[0] = 0x02
		copy(buf[1:32], stem[:])
		buf[32] = 0x00
		copy(buf[33:], subtreeRoot[:])
		return blake3.Sum256(buf[:])
	}
	var buf [64]byte
	copy(buf[:31], stem[:])
	buf[31] = 0x00
	copy(buf[32:], subtreeRoot[:])
	return blake3.Sum256(buf[:])
}

func (h *Blake3Hasher) HashRaw(input []byte) [32]byte {
	return blake3.Sum256(input)
}
