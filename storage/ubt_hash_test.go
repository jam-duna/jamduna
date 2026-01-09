package storage

import "testing"

func TestHash64ZeroShortCircuit(t *testing.T) {
	zero := [32]byte{}
	profiles := []Profile{EIPProfile, JAMProfile}
	for _, profile := range profiles {
		hasher := NewBlake3Hasher(profile)
		got := hasher.Hash64(&zero, &zero)
		if got != zero {
			t.Fatalf("profile %d: expected ZERO32, got %x", profile, got)
		}
	}
}
