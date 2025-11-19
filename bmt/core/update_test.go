package core

import (
	"bytes"
	"testing"
)

// TestSortOperations tests operation sorting by key.
func TestSortOperations(t *testing.T) {
	ops := []Operation{
		{Key: KeyPath{0x30}, Value: []byte("c")},
		{Key: KeyPath{0x10}, Value: []byte("a")},
		{Key: KeyPath{0x20}, Value: []byte("b")},
	}

	SortOperations(ops)

	// Should be sorted: 0x10, 0x20, 0x30
	if ops[0].Key[0] != 0x10 {
		t.Errorf("ops[0] should be 0x10, got 0x%02x", ops[0].Key[0])
	}
	if ops[1].Key[0] != 0x20 {
		t.Errorf("ops[1] should be 0x20, got 0x%02x", ops[1].Key[0])
	}
	if ops[2].Key[0] != 0x30 {
		t.Errorf("ops[2] should be 0x30, got 0x%02x", ops[2].Key[0])
	}
}

// TestSharedBitsIdentical tests shared bits for identical keys.
func TestSharedBitsIdentical(t *testing.T) {
	key := KeyPath{0xFF, 0xAA, 0x55}
	shared := SharedBits(key, key)

	expected := len(key) * 8
	if shared != expected {
		t.Errorf("Identical keys should share all %d bits, got %d", expected, shared)
	}
}

// TestSharedBitsDifferentFirstBit tests keys differing at bit 0.
func TestSharedBitsDifferentFirstBit(t *testing.T) {
	// 0x80 = 10000000
	// 0x00 = 00000000
	// First bit differs
	a := KeyPath{0x80}
	b := KeyPath{0x00}

	shared := SharedBits(a, b)
	if shared != 0 {
		t.Errorf("Keys differing at bit 0 should share 0 bits, got %d", shared)
	}
}

// TestSharedBitsDifferentSecondBit tests keys differing at bit 1.
func TestSharedBitsDifferentSecondBit(t *testing.T) {
	// 0xC0 = 11000000
	// 0x80 = 10000000
	// Second bit (bit 1) differs
	a := KeyPath{0xC0}
	b := KeyPath{0x80}

	shared := SharedBits(a, b)
	if shared != 1 {
		t.Errorf("Keys differing at bit 1 should share 1 bit, got %d", shared)
	}
}

// TestSharedBitsMultiBytePrefix tests shared bits across byte boundaries.
func TestSharedBitsMultiBytePrefix(t *testing.T) {
	// Both start with 0xFF (8 bits shared)
	// Then 0xF0 = 11110000 vs 0xE0 = 11100000
	// Share first 3 bits of second byte
	// Total: 8 + 3 = 11 bits
	a := KeyPath{0xFF, 0xF0}
	b := KeyPath{0xFF, 0xE0}

	shared := SharedBits(a, b)
	if shared != 11 {
		t.Errorf("Expected 11 shared bits, got %d", shared)
	}
}

// TestSharedBitsFullByte tests keys sharing complete bytes.
func TestSharedBitsFullByte(t *testing.T) {
	// Share first byte exactly, differ in second
	a := KeyPath{0xAA, 0xFF}
	b := KeyPath{0xAA, 0x00}

	shared := SharedBits(a, b)
	if shared != 8 {
		t.Errorf("Expected 8 shared bits (1 full byte), got %d", shared)
	}
}

// TestOperationIsDelete tests delete detection.
func TestOperationIsDelete(t *testing.T) {
	deleteOp := Operation{
		Key:   KeyPath{1, 2, 3},
		Value: nil,
	}

	if !deleteOp.IsDelete() {
		t.Error("Operation with nil value should be delete")
	}

	if deleteOp.IsInsert() {
		t.Error("Delete operation should not be insert")
	}
}

// TestOperationIsInsert tests insert detection.
func TestOperationIsInsert(t *testing.T) {
	insertOp := Operation{
		Key:   KeyPath{1, 2, 3},
		Value: []byte("data"),
	}

	if !insertOp.IsInsert() {
		t.Error("Operation with value should be insert")
	}

	if insertOp.IsDelete() {
		t.Error("Insert operation should not be delete")
	}
}

// TestSortOperationsPreserveValues tests that sorting preserves values.
func TestSortOperationsPreserveValues(t *testing.T) {
	ops := []Operation{
		{Key: KeyPath{0x30}, Value: []byte("third")},
		{Key: KeyPath{0x10}, Value: []byte("first")},
		{Key: KeyPath{0x20}, Value: []byte("second")},
	}

	SortOperations(ops)

	// Verify key-value pairs are preserved
	if !bytes.Equal(ops[0].Value, []byte("first")) {
		t.Errorf("First op value mismatch: got %s", ops[0].Value)
	}
	if !bytes.Equal(ops[1].Value, []byte("second")) {
		t.Errorf("Second op value mismatch: got %s", ops[1].Value)
	}
	if !bytes.Equal(ops[2].Value, []byte("third")) {
		t.Errorf("Third op value mismatch: got %s", ops[2].Value)
	}
}

// TestSortOperationsMixedOps tests sorting with mix of inserts and deletes.
func TestSortOperationsMixedOps(t *testing.T) {
	ops := []Operation{
		{Key: KeyPath{0x30}, Value: []byte("c")},
		{Key: KeyPath{0x10}, Value: nil}, // delete
		{Key: KeyPath{0x20}, Value: []byte("b")},
	}

	SortOperations(ops)

	// Verify sorted by key regardless of delete/insert
	if ops[0].Key[0] != 0x10 || !ops[0].IsDelete() {
		t.Error("First op should be delete at 0x10")
	}
	if ops[1].Key[0] != 0x20 || !ops[1].IsInsert() {
		t.Error("Second op should be insert at 0x20")
	}
	if ops[2].Key[0] != 0x30 || !ops[2].IsInsert() {
		t.Error("Third op should be insert at 0x30")
	}
}

// TestSharedBitsGpTreeExample tests shared bits using GP tree test vectors.
func TestSharedBitsGpTreeExample(t *testing.T) {
	// From GP tree tests: keys that split at bit 0
	// 0x00... vs 0x80... should share 0 bits
	keyA := KeyPath{0x00, 0x00, 0x00, 0x00}
	keyB := KeyPath{0x80, 0x00, 0x00, 0x00}

	shared := SharedBits(keyA, keyB)
	if shared != 0 {
		t.Errorf("Keys 0x00... and 0x80... should share 0 bits, got %d", shared)
	}

	// Keys that share first bit but differ at bit 1
	// 0x80... (10...) vs 0xC0... (11...) should share 1 bit
	keyC := KeyPath{0x80, 0x00, 0x00, 0x00}
	keyD := KeyPath{0xC0, 0x00, 0x00, 0x00}

	shared = SharedBits(keyC, keyD)
	if shared != 1 {
		t.Errorf("Keys 0x80... and 0xC0... should share 1 bit, got %d", shared)
	}
}
