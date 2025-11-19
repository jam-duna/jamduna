package core

import (
	"testing"
)

// TestPageIdEncodeDecodeRoundTrip tests that encoding and decoding are inverses.
func TestPageIdEncodeDecodeRoundTrip(t *testing.T) {
	testCases := []struct {
		name string
		path []uint8
	}{
		{"root", []uint8{}},
		{"single child 0", []uint8{0}},
		{"single child 3", []uint8{3}},
		{"single child 63", []uint8{63}},
		{"two children", []uint8{5, 10}},
		{"three children", []uint8{1, 2, 3}},
		{"depth 9", []uint8{0, 1, 2, 3, 4, 5, 6, 7, 8}},
		{"depth 10", []uint8{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}},
		{"max depth", func() []uint8 {
			path := make([]uint8, MaxPageDepth)
			for i := range path {
				path[i] = uint8(i % 64)
			}
			return path
		}()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create PageId
			pageId, err := NewPageId(tc.path)
			if err != nil {
				t.Fatalf("NewPageId failed: %v", err)
			}

			// Encode
			encoded := pageId.Encode()

			// Decode
			decoded, err := DecodePageId(encoded)
			if err != nil {
				t.Fatalf("DecodePageId failed: %v", err)
			}

			// Compare
			if !decoded.Equals(pageId) {
				t.Errorf("Round-trip failed:\n  Original: %v\n  Decoded:  %v\n  Encoded:  %x",
					pageId.path, decoded.path, encoded)
			}
		})
	}
}

// TestPageIdEncodeSingleChild verifies the specific bug case mentioned.
// Encoding path [3] should decode back to [3], not [2, 63].
func TestPageIdEncodeSingleChild(t *testing.T) {
	pageId, err := NewPageId([]uint8{3})
	if err != nil {
		t.Fatalf("NewPageId failed: %v", err)
	}

	encoded := pageId.Encode()
	decoded, err := DecodePageId(encoded)
	if err != nil {
		t.Fatalf("DecodePageId failed: %v", err)
	}

	// Should decode to [3], not [2, 63]
	if len(decoded.path) != 1 {
		t.Errorf("Expected path length 1, got %d (path: %v)", len(decoded.path), decoded.path)
	}
	if len(decoded.path) > 0 && decoded.path[0] != 3 {
		t.Errorf("Expected path[0] = 3, got %d", decoded.path[0])
	}
}
