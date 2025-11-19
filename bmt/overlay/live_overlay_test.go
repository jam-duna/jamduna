package overlay

import (
	"bytes"
	"testing"

	"github.com/colorfulnotion/jam/bmt/beatree"
)

func TestLiveOverlayInsert(t *testing.T) {
	var root [32]byte
	live := NewLiveOverlay(root, nil)

	key := testKey("testkey")
	value := []byte("testvalue")

	err := live.SetValue(key, beatree.NewInsertChange(value))
	if err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	retrieved, err := live.Lookup(key)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}

	if !bytes.Equal(retrieved, value) {
		t.Errorf("Value mismatch: expected %s, got %s", value, retrieved)
	}
}

func TestLiveOverlayDelete(t *testing.T) {
	var root [32]byte
	live := NewLiveOverlay(root, nil)

	key := testKey("testkey")

	// First insert
	live.SetValue(key, beatree.NewInsertChange([]byte("value")))

	// Then delete
	err := live.SetValue(key, beatree.NewDeleteChange())
	if err != nil {
		t.Fatalf("SetValue (delete) failed: %v", err)
	}

	retrieved, _ := live.Lookup(key)
	if retrieved != nil {
		t.Errorf("Expected nil after delete, got %v", retrieved)
	}
}

func TestLiveOverlayFreeze(t *testing.T) {
	var root [32]byte
	live := NewLiveOverlay(root, nil)

	key := testKey("testkey")
	live.SetValue(key, beatree.NewInsertChange([]byte("value")))

	// Freeze
	overlay, err := live.Freeze()
	if err != nil {
		t.Fatalf("Freeze failed: %v", err)
	}

	// Should be able to lookup from frozen overlay
	value, _ := overlay.Lookup(key)
	if !bytes.Equal(value, []byte("value")) {
		t.Error("Value not preserved after freeze")
	}

	// Cannot modify after freeze
	err = live.SetValue(key, beatree.NewInsertChange([]byte("newvalue")))
	if err == nil {
		t.Error("Should not be able to modify after freeze")
	}
}

func TestLiveOverlayDrop(t *testing.T) {
	var root [32]byte
	live := NewLiveOverlay(root, nil)

	key := testKey("testkey")
	live.SetValue(key, beatree.NewInsertChange([]byte("value")))

	// Drop
	live.Drop()

	// Cannot modify after drop
	err := live.SetValue(key, beatree.NewInsertChange([]byte("newvalue")))
	if err == nil {
		t.Error("Should not be able to modify after drop")
	}
}

func TestLiveOverlayWithParent(t *testing.T) {
	var root [32]byte

	// Create parent
	parentData := NewData()
	parentKey := testKey("parent_key")
	parentData.SetValue(parentKey, beatree.NewInsertChange([]byte("parent_value")))
	parent := NewOverlay(root, root, parentData, nil)

	// Create live overlay with parent
	live := NewLiveOverlay(root, parent)

	if !live.HasParent() {
		t.Error("Expected live overlay to have parent")
	}

	// Can see parent's value
	value, _ := live.Lookup(parentKey)
	if !bytes.Equal(value, []byte("parent_value")) {
		t.Error("Cannot see parent's value")
	}

	// Can add own value
	childKey := testKey("child_key")
	live.SetValue(childKey, beatree.NewInsertChange([]byte("child_value")))

	value, _ = live.Lookup(childKey)
	if !bytes.Equal(value, []byte("child_value")) {
		t.Error("Cannot see own value")
	}
}

func TestLiveOverlaySetRoot(t *testing.T) {
	var prevRoot, newRoot [32]byte
	copy(prevRoot[:], "prev")
	copy(newRoot[:], "new")

	live := NewLiveOverlay(prevRoot, nil)

	if live.PrevRoot() != prevRoot {
		t.Error("PrevRoot mismatch")
	}

	live.SetRoot(newRoot)

	if live.Root() != newRoot {
		t.Error("Root not updated")
	}

	if live.PrevRoot() != prevRoot {
		t.Error("PrevRoot should not change")
	}
}

func TestLiveOverlayValueCount(t *testing.T) {
	var root [32]byte
	live := NewLiveOverlay(root, nil)

	key1 := testKey("key1")
	key2 := testKey("key2")

	live.SetValue(key1, beatree.NewInsertChange([]byte("v1")))
	live.SetValue(key2, beatree.NewInsertChange([]byte("v2")))

	if live.ValueCount() != 2 {
		t.Errorf("Expected 2 values, got %d", live.ValueCount())
	}
}

func TestLiveOverlayFreezeWithParent(t *testing.T) {
	var root [32]byte

	// Create parent
	parentData := NewData()
	parentKey := testKey("parent_key")
	parentData.SetValue(parentKey, beatree.NewInsertChange([]byte("parent_value")))
	parent := NewOverlay(root, root, parentData, nil)

	// Create live with parent
	live := NewLiveOverlay(root, parent)
	childKey := testKey("child_key")
	live.SetValue(childKey, beatree.NewInsertChange([]byte("child_value")))

	// Freeze
	overlay, err := live.Freeze()
	if err != nil {
		t.Fatalf("Freeze failed: %v", err)
	}

	// Should have 1 ancestor
	if overlay.AncestorCount() != 1 {
		t.Errorf("Expected 1 ancestor after freeze, got %d", overlay.AncestorCount())
	}

	// Can see both parent and child values
	parentValue, _ := overlay.Lookup(parentKey)
	if !bytes.Equal(parentValue, []byte("parent_value")) {
		t.Error("Cannot see parent value after freeze")
	}

	childValue, _ := overlay.Lookup(childKey)
	if !bytes.Equal(childValue, []byte("child_value")) {
		t.Error("Cannot see child value after freeze")
	}
}
