package rollback

import (
	"bytes"
	"testing"

	"github.com/colorfulnotion/jam/bmt/beatree"
)

func TestDeltaRoundtrip(t *testing.T) {
	delta := NewDelta()

	key1 := beatree.KeyFromBytes(bytes.Repeat([]byte{1}, 32))
	key2 := beatree.KeyFromBytes(bytes.Repeat([]byte{2}, 32))
	key3 := beatree.KeyFromBytes(bytes.Repeat([]byte{3}, 32))

	delta.AddPrior(key1, []byte("value1"))
	delta.AddPrior(key2, nil) // Erase
	delta.AddPrior(key3, []byte("value3"))

	// Encode
	encoded := delta.Encode()

	// Decode
	decoded, err := Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("Failed to decode delta: %v", err)
	}

	// Verify
	if len(decoded.Priors) != 3 {
		t.Errorf("Expected 3 priors, got %d", len(decoded.Priors))
	}

	if !bytes.Equal(decoded.Priors[key1], []byte("value1")) {
		t.Errorf("Key1 value mismatch")
	}

	if decoded.Priors[key2] != nil {
		t.Errorf("Key2 should be nil (erase)")
	}

	if !bytes.Equal(decoded.Priors[key3], []byte("value3")) {
		t.Errorf("Key3 value mismatch")
	}
}

func TestDeltaRoundtripEmpty(t *testing.T) {
	delta := NewDelta()

	// Encode
	encoded := delta.Encode()

	// Decode
	decoded, err := Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("Failed to decode empty delta: %v", err)
	}

	// Verify
	if len(decoded.Priors) != 0 {
		t.Errorf("Expected empty priors, got %d", len(decoded.Priors))
	}
}

func TestDeltaPriorValues(t *testing.T) {
	delta := NewDelta()

	// Test adding various prior values
	key1 := beatree.KeyFromBytes(bytes.Repeat([]byte{0xAA}, 32))
	key2 := beatree.KeyFromBytes(bytes.Repeat([]byte{0xBB}, 32))
	key3 := beatree.KeyFromBytes(bytes.Repeat([]byte{0xCC}, 32))

	delta.AddPrior(key1, []byte("original_value"))
	delta.AddPrior(key2, nil) // Deleted key
	delta.AddPrior(key3, []byte{}) // Empty value

	// Encode and decode
	encoded := delta.Encode()
	decoded, err := Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Verify each prior value
	if !bytes.Equal(decoded.Priors[key1], []byte("original_value")) {
		t.Errorf("key1 value mismatch")
	}

	if decoded.Priors[key2] != nil {
		t.Errorf("key2 should be nil")
	}

	if decoded.Priors[key3] == nil || len(decoded.Priors[key3]) != 0 {
		t.Errorf("key3 should be empty byte slice, not nil")
	}
}
