package storage

import "testing"

func TestGetSubtreeHashRejectsInvalidPrefixBits(t *testing.T) {
	store := NewMemoryUBTStore(NewBlake3Hasher(EIPProfile))
	var prefix Stem

	if _, ok := store.GetSubtreeHash(prefix, -1); ok {
		t.Fatalf("expected invalid prefixBits to return ok=false")
	}
	if _, ok := store.GetSubtreeHash(prefix, 249); ok {
		t.Fatalf("expected invalid prefixBits to return ok=false")
	}
}
