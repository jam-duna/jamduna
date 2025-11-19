package core

import "testing"

func TestPageConstants(t *testing.T) {
	// Test Depth constant
	if Depth != 6 {
		t.Errorf("Depth = %d, want 6", Depth)
	}

	// Test NodesPerPage constant
	// For depth 6: 2 + 4 + 8 + 16 + 32 + 64 = 126 nodes
	expected := 126
	if NodesPerPage != expected {
		t.Errorf("NodesPerPage = %d, want %d", NodesPerPage, expected)
	}
}

func TestNodeIndices(t *testing.T) {
	// Test that node indices fit within NodesPerPage
	// Root is at index 0
	// Children of root are at indices 1 and 2
	// Maximum index should be NodesPerPage - 1

	tests := []struct {
		name     string
		index    int
		valid    bool
		expected string
	}{
		{"root", 0, true, "root node should be valid"},
		{"first child", 1, true, "first child should be valid"},
		{"second child", 2, true, "second child should be valid"},
		{"last valid", NodesPerPage - 1, true, "last valid index"},
		{"out of bounds", NodesPerPage, false, "index >= NodesPerPage should be invalid"},
		{"negative", -1, false, "negative index should be invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.index >= 0 && tt.index < NodesPerPage
			if isValid != tt.valid {
				t.Errorf("%s: index %d validity = %v, want %v",
					tt.expected, tt.index, isValid, tt.valid)
			}
		})
	}
}

func TestPageStructure(t *testing.T) {
	// Test that a full page can hold exactly NodesPerPage nodes
	// This is a structural test to ensure our constants are consistent

	// Calculate number of nodes in a rootless sub-tree of depth 6
	// For depth 6: levels 1,2,3,4,5,6 = 2+4+8+16+32+64 = 126 nodes
	// This is a rootless tree, so we don't include level 0

	maxNodes := 0
	for level := 1; level <= Depth; level++ {
		maxNodes += 1 << level
	}

	// Should equal 126 for Depth=6
	expected := 126
	if maxNodes != expected {
		t.Errorf("Calculation error: maxNodes = %d, expected %d", maxNodes, expected)
	}

	// NodesPerPage should match
	if NodesPerPage != maxNodes {
		t.Errorf("NodesPerPage = %d, want %d", NodesPerPage, maxNodes)
	}
}
