package core

import "testing"

// TestTerminator verifies the terminator node is all zeros.
func TestTerminator(t *testing.T) {
	for i := 0; i < 32; i++ {
		if Terminator[i] != 0 {
			t.Errorf("Terminator[%d] = %d, want 0", i, Terminator[i])
		}
	}
}

// TestIsTerminator verifies the IsTerminator function.
func TestIsTerminator(t *testing.T) {
	if !IsTerminator(Terminator) {
		t.Error("Terminator should return true for IsTerminator()")
	}

	nonTerminator := Node{1, 2, 3}
	if IsTerminator(nonTerminator) {
		t.Error("Non-terminator node should return false for IsTerminator()")
	}
}

// TestLeafData verifies LeafData construction.
func TestLeafData(t *testing.T) {
	var keyPath KeyPath
	copy(keyPath[:], []byte{1, 2, 3, 4, 5})

	var valueHash ValueHash
	copy(valueHash[:], []byte("test value hash"))

	data := NewLeafData(keyPath, valueHash)

	if data.KeyPath != keyPath {
		t.Errorf("KeyPath mismatch: got %v, want %v", data.KeyPath, keyPath)
	}
	if data.ValueHash != valueHash {
		t.Errorf("ValueHash mismatch: got %v, want %v", data.ValueHash, valueHash)
	}
}

// TestLeafDataGP verifies GP leaf data construction with embedded values.
func TestLeafDataGP(t *testing.T) {
	var keyPath KeyPath
	copy(keyPath[:], []byte{1, 2, 3})

	value := []byte("test")
	var valueHash ValueHash
	copy(valueHash[:], []byte("hash"))

	data := NewLeafDataGP(keyPath, value, valueHash)

	if data.KeyPath != keyPath {
		t.Errorf("KeyPath mismatch")
	}
	if data.ValueLen != uint32(len(value)) {
		t.Errorf("ValueLen = %d, want %d", data.ValueLen, len(value))
	}
	if len(data.ValueInline) != len(value) {
		t.Errorf("ValueInline length = %d, want %d", len(data.ValueInline), len(value))
	}
}

// TestLeafDataGPOverflow verifies GP leaf data for large values.
func TestLeafDataGPOverflow(t *testing.T) {
	var keyPath KeyPath
	var valueHash ValueHash

	// Should work for values >32 bytes
	data := NewLeafDataGPOverflow(keyPath, 100, valueHash)
	if data.ValueLen != 100 {
		t.Errorf("ValueLen = %d, want 100", data.ValueLen)
	}
	if data.ValueInline != nil {
		t.Error("ValueInline should be nil for overflow values")
	}

	// Should panic for values ≤32 bytes
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewLeafDataGPOverflow should panic for small values")
		}
	}()
	NewLeafDataGPOverflow(keyPath, 32, valueHash)
}

// TestInternalData verifies InternalData construction.
func TestInternalData(t *testing.T) {
	left := Node{1, 2, 3}
	right := Node{4, 5, 6}

	data := InternalData{
		Left:  left,
		Right: right,
	}

	if data.Left != left {
		t.Errorf("Left mismatch: got %v, want %v", data.Left, left)
	}
	if data.Right != right {
		t.Errorf("Right mismatch: got %v, want %v", data.Right, right)
	}
}

// TestNodeKindString verifies NodeKind string representation.
func TestNodeKindString(t *testing.T) {
	tests := []struct {
		kind NodeKind
		want string
	}{
		{NodeKindTerminator, "Terminator"},
		{NodeKindLeaf, "Leaf"},
		{NodeKindInternal, "Internal"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				t.Errorf("NodeKind.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
