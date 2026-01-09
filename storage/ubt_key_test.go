package storage

import "testing"

func TestTreeKeyRoundTrip(t *testing.T) {
	var input [32]byte
	for i := range input {
		input[i] = byte(i)
	}
	key := TreeKeyFromBytes(input)
	got := key.ToBytes()
	if got != input {
		t.Fatalf("round-trip mismatch: got %x want %x", got, input)
	}
}
