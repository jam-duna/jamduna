package storage

// Stem is the 31-byte prefix of a TreeKey.
type Stem [31]byte

func (s Stem) Bit(i int) uint8 {
	// Invariant: 0 <= i < 248 (callers must enforce).
	return (s[i/8] >> (7 - i%8)) & 1
}

func (s Stem) IsZero() bool {
	for _, b := range s {
		if b != 0 {
			return false
		}
	}
	return true
}

func (s Stem) Less(other *Stem) bool {
	if other == nil {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < other[i] {
			return true
		}
		if s[i] > other[i] {
			return false
		}
	}
	return false
}

func (s Stem) FirstDifferingBit(other *Stem) *int {
	if other == nil {
		return nil
	}
	for i := 0; i < 248; i++ {
		if s.Bit(i) != other.Bit(i) {
			idx := i
			return &idx
		}
	}
	return nil
}

// TreeKey is a 32-byte key split into stem + subindex.
type TreeKey struct {
	Stem     Stem
	Subindex uint8
}

func (k TreeKey) ToBytes() [32]byte {
	var out [32]byte
	copy(out[:31], k.Stem[:])
	out[31] = k.Subindex
	return out
}

func TreeKeyFromBytes(b [32]byte) TreeKey {
	var stem Stem
	copy(stem[:], b[:31])
	return TreeKey{Stem: stem, Subindex: b[31]}
}
