package storage

import "testing"

func TestStemNodeHashMatchesReference(t *testing.T) {
	hasher := NewBlake3Hasher(EIPProfile)
	var stem Stem
	stem[0] = 0x42

	node := NewStemNode(stem)
	var v1 [32]byte
	var v2 [32]byte
	v1[0] = 0x01
	v2[0] = 0x02
	node.SetValue(0, v1)
	node.SetValue(5, v2)

	got := node.Hash(hasher)
	want := referenceStemHash(stem, node.Values, hasher)
	if got != want {
		t.Fatalf("stem hash mismatch: got %x want %x", got, want)
	}
}

func referenceStemHash(stem Stem, values map[uint8][32]byte, hasher Hasher) [32]byte {
	var data [256][32]byte
	for i := 0; i < 256; i++ {
		if val, ok := values[uint8(i)]; ok {
			h := hasher.Hash32(&val)
			data[i] = h
		}
	}
	for level := 0; level < 8; level++ {
		pairs := 256 >> (level + 1)
		for i := 0; i < pairs; i++ {
			left := data[i*2]
			right := data[i*2+1]
			data[i] = hasher.Hash64(&left, &right)
		}
	}
	root := data[0]
	return hasher.HashStemNode(&stem, &root)
}
