package overlay

import (
	"bytes"
	"testing"

	"github.com/jam-duna/jamduna/bmt/beatree"
	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
)

func testKey(s string) beatree.Key {
	var keyBytes [32]byte
	copy(keyBytes[:], s)
	return beatree.KeyFromBytes(keyBytes[:])
}

func TestOverlayCreate(t *testing.T) {
	var prevRoot, root [32]byte
	copy(prevRoot[:], "prev")
	copy(root[:], "root")

	data := NewData()
	overlay := NewOverlay(prevRoot, root, data, nil)

	if overlay.PrevRoot() != prevRoot {
		t.Errorf("PrevRoot mismatch")
	}

	if overlay.Root() != root {
		t.Errorf("Root mismatch")
	}

	if overlay.AncestorCount() != 0 {
		t.Errorf("Expected 0 ancestors, got %d", overlay.AncestorCount())
	}
}

func TestOverlayCommit(t *testing.T) {
	var root [32]byte
	data := NewData()
	overlay := NewOverlay(root, root, data, nil)

	// Initially live
	if overlay.Status() != StatusLive {
		t.Errorf("Expected StatusLive, got %v", overlay.Status())
	}

	// Mark committed
	if !overlay.MarkCommitted() {
		t.Error("MarkCommitted failed")
	}

	if overlay.Status() != StatusCommitted {
		t.Errorf("Expected StatusCommitted, got %v", overlay.Status())
	}

	// Cannot commit again
	if overlay.MarkCommitted() {
		t.Error("Should not be able to commit twice")
	}
}

func TestOverlayDrop(t *testing.T) {
	var root [32]byte
	data := NewData()
	overlay := NewOverlay(root, root, data, nil)

	// Mark dropped
	if !overlay.MarkDropped() {
		t.Error("MarkDropped failed")
	}

	if overlay.Status() != StatusDropped {
		t.Errorf("Expected StatusDropped, got %v", overlay.Status())
	}

	// Cannot drop again
	if overlay.MarkDropped() {
		t.Error("Should not be able to drop twice")
	}
}

func TestOverlayLookup(t *testing.T) {
	var root [32]byte
	data := NewData()

	key := testKey("testkey")
	value := []byte("testvalue")

	data.SetValue(key, beatree.NewInsertChange(value))

	overlay := NewOverlay(root, root, data, nil)

	retrieved, err := overlay.Lookup(key)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}

	if !bytes.Equal(retrieved, value) {
		t.Errorf("Value mismatch: expected %s, got %s", value, retrieved)
	}
}

func TestOverlayLookupDelete(t *testing.T) {
	var root [32]byte
	data := NewData()

	key := testKey("testkey")
	data.SetValue(key, beatree.NewDeleteChange())

	overlay := NewOverlay(root, root, data, nil)

	retrieved, err := overlay.Lookup(key)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("Expected nil for deleted key, got %v", retrieved)
	}
}

func TestOverlayPage(t *testing.T) {
	var root [32]byte
	data := NewData()

	pageNum := allocator.PageNumber(42)
	pageData := []byte("page content")

	data.SetPage(pageNum, pageData)

	overlay := NewOverlay(root, root, data, nil)

	retrieved, ok := overlay.GetPage(pageNum)
	if !ok {
		t.Fatal("Page not found")
	}

	if !bytes.Equal(retrieved, pageData) {
		t.Errorf("Page data mismatch")
	}
}

func TestOverlayAncestry(t *testing.T) {
	var root [32]byte

	// Create parent overlay
	parentData := NewData()
	parentKey := testKey("parent_key")
	parentData.SetValue(parentKey, beatree.NewInsertChange([]byte("parent_value")))
	_ = NewOverlay(root, root, parentData, nil) // parent

	// Create child overlay with parent
	childData := NewData()
	childKey := testKey("child_key")
	childData.SetValue(childKey, beatree.NewInsertChange([]byte("child_value")))
	child := NewOverlay(root, root, childData, []*Data{parentData})

	// Child should have 1 ancestor
	if child.AncestorCount() != 1 {
		t.Errorf("Expected 1 ancestor, got %d", child.AncestorCount())
	}

	// Child can see own value
	value, _ := child.Lookup(childKey)
	if !bytes.Equal(value, []byte("child_value")) {
		t.Error("Child cannot see own value")
	}

	// Child can see parent value
	value, _ = child.Lookup(parentKey)
	if !bytes.Equal(value, []byte("parent_value")) {
		t.Error("Child cannot see parent value")
	}
}

func TestOverlayIsolation(t *testing.T) {
	var root [32]byte

	// Create two independent overlays
	data1 := NewData()
	data2 := NewData()

	key := testKey("shared_key")
	data1.SetValue(key, beatree.NewInsertChange([]byte("value1")))
	data2.SetValue(key, beatree.NewInsertChange([]byte("value2")))

	overlay1 := NewOverlay(root, root, data1, nil)
	overlay2 := NewOverlay(root, root, data2, nil)

	// Each overlay should see its own value
	value1, _ := overlay1.Lookup(key)
	value2, _ := overlay2.Lookup(key)

	if !bytes.Equal(value1, []byte("value1")) {
		t.Error("Overlay1 sees wrong value")
	}

	if !bytes.Equal(value2, []byte("value2")) {
		t.Error("Overlay2 sees wrong value")
	}
}

func TestOverlayValueCount(t *testing.T) {
	var root [32]byte
	data := NewData()

	key1 := testKey("key1")
	key2 := testKey("key2")

	data.SetValue(key1, beatree.NewInsertChange([]byte("v1")))
	data.SetValue(key2, beatree.NewInsertChange([]byte("v2")))

	overlay := NewOverlay(root, root, data, nil)

	if overlay.ValueCount() != 2 {
		t.Errorf("Expected 2 values, got %d", overlay.ValueCount())
	}
}

func TestOverlayPageCount(t *testing.T) {
	var root [32]byte
	data := NewData()

	data.SetPage(allocator.PageNumber(1), []byte("page1"))
	data.SetPage(allocator.PageNumber(2), []byte("page2"))

	overlay := NewOverlay(root, root, data, nil)

	if overlay.PageCount() != 2 {
		t.Errorf("Expected 2 pages, got %d", overlay.PageCount())
	}
}

// TestOverlayImmutability verifies that the user should not mutate
// Data after using it to create an Overlay. This test documents expected
// behavior - mutations are possible but discouraged.
func TestOverlayImmutability(t *testing.T) {
	t.Skip("Skipping - immutability is a usage contract, not enforced by the API")
}

// TestOverlayLookupMissingKey verifies that looking up a key that doesn't
// exist in any overlay returns (nil, nil), not a delete change.
func TestOverlayLookupMissingKey(t *testing.T) {
	var root [32]byte
	key1 := testKey("key1")
	keyMissing := testKey("missing")

	// Create overlay with one key
	data := NewData()
	data.SetValue(key1, beatree.NewInsertChange([]byte("value1")))
	overlay := NewOverlay(root, root, data, nil)

	// Lookup existing key
	val, err := overlay.Lookup(key1)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if string(val) != "value1" {
		t.Errorf("Expected 'value1', got '%s'", string(val))
	}

	// Lookup missing key - should return (nil, nil)
	val, err = overlay.Lookup(keyMissing)
	if err != nil {
		t.Fatalf("Lookup of missing key failed: %v", err)
	}
	if val != nil {
		t.Errorf("Expected nil for missing key, got %v", val)
	}
}
