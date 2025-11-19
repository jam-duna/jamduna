package overlay

import (
	"testing"

	"github.com/colorfulnotion/jam/bmt/beatree"
)

func TestOverlayChainDepth(t *testing.T) {
	var root [32]byte

	// No ancestors
	data := NewData()
	overlay := NewOverlay(root, root, data, nil)

	if ChainDepth(overlay) != 0 {
		t.Errorf("Expected depth 0, got %d", ChainDepth(overlay))
	}

	// One ancestor
	parentData := NewData()
	_ = NewOverlay(root, root, parentData, nil) // parent
	childData := NewData()
	child := NewOverlay(root, root, childData, []*Data{parentData})

	if ChainDepth(child) != 1 {
		t.Errorf("Expected depth 1, got %d", ChainDepth(child))
	}
}

func TestLiveOverlayChainDepth(t *testing.T) {
	var root [32]byte

	// No parent
	live := NewLiveOverlay(root, nil)

	if LiveChainDepth(live) != 0 {
		t.Errorf("Expected depth 0, got %d", LiveChainDepth(live))
	}

	// With parent
	parentData := NewData()
	parent := NewOverlay(root, root, parentData, nil)
	liveWithParent := NewLiveOverlay(root, parent)

	if LiveChainDepth(liveWithParent) != 1 {
		t.Errorf("Expected depth 1, got %d", LiveChainDepth(liveWithParent))
	}
}

func TestValidateChain(t *testing.T) {
	var root [32]byte

	data := NewData()
	overlay := NewOverlay(root, root, data, nil)

	// Valid chain
	if err := ValidateChain(overlay); err != nil {
		t.Errorf("Expected valid chain, got error: %v", err)
	}

	// Drop overlay
	overlay.MarkDropped()

	// Now invalid
	if err := ValidateChain(overlay); err == nil {
		t.Error("Expected error for dropped overlay")
	}
}

func TestValidateLiveChain(t *testing.T) {
	var root [32]byte

	// No parent - valid
	live := NewLiveOverlay(root, nil)

	if err := ValidateLiveChain(live); err != nil {
		t.Errorf("Expected valid chain, got error: %v", err)
	}

	// With valid parent
	parentData := NewData()
	parent := NewOverlay(root, root, parentData, nil)
	liveWithParent := NewLiveOverlay(root, parent)

	if err := ValidateLiveChain(liveWithParent); err != nil {
		t.Errorf("Expected valid chain, got error: %v", err)
	}

	// Drop parent
	parent.MarkDropped()

	// Now invalid
	if err := ValidateLiveChain(liveWithParent); err == nil {
		t.Error("Expected error for dropped parent")
	}
}

func TestIsOrphaned(t *testing.T) {
	var root [32]byte

	// Create parent and child
	parentData := NewData()
	parent := NewOverlay(root, root, parentData, nil)
	childData := NewData()
	child := NewOverlay(root, root, childData, []*Data{parentData})
	_ = child // suppress warning

	// Not orphaned initially
	if IsOrphaned(child) {
		t.Error("Child should not be orphaned initially")
	}

	// Drop parent
	parent.MarkDropped()

	// Now orphaned
	if !IsOrphaned(child) {
		t.Error("Child should be orphaned after parent dropped")
	}
}

func TestIsLiveOrphaned(t *testing.T) {
	var root [32]byte

	// Create parent and live child
	parentData := NewData()
	parent := NewOverlay(root, root, parentData, nil)
	liveChild := NewLiveOverlay(root, parent)

	// Not orphaned initially
	if IsLiveOrphaned(liveChild) {
		t.Error("Live child should not be orphaned initially")
	}

	// Drop parent
	parent.MarkDropped()

	// Now orphaned
	if !IsLiveOrphaned(liveChild) {
		t.Error("Live child should be orphaned after parent dropped")
	}
}

func TestGetAncestorStatuses(t *testing.T) {
	var root [32]byte

	// Create chain: grandparent -> parent -> child
	grandparentData := NewData()
	grandparent := NewOverlay(root, root, grandparentData, nil)

	parentData := NewData()
	_ = NewOverlay(root, root, parentData, []*Data{grandparentData}) // parent

	childData := NewData()
	child := NewOverlay(root, root, childData, []*Data{parentData, grandparentData})

	// All should be live initially
	statuses := GetAncestorStatuses(child)
	if len(statuses) != 2 {
		t.Fatalf("Expected 2 statuses, got %d", len(statuses))
	}

	for i, status := range statuses {
		if status != StatusLive {
			t.Errorf("Ancestor %d: expected StatusLive, got %v", i, status)
		}
	}

	// Commit grandparent
	grandparent.MarkCommitted()

	statuses = GetAncestorStatuses(child)
	if statuses[1] != StatusCommitted {
		t.Errorf("Expected grandparent committed, got %v", statuses[1])
	}
}

func TestAllAncestorsCommitted(t *testing.T) {
	var root [32]byte

	// Create parent and child
	parentData := NewData()
	parent := NewOverlay(root, root, parentData, nil)
	childData := NewData()
	child := NewOverlay(root, root, childData, []*Data{parentData})
	_ = child // suppress warning

	// Not all committed initially
	if AllAncestorsCommitted(child) {
		t.Error("Expected not all committed initially")
	}

	// Commit parent
	parent.MarkCommitted()

	// Now all committed
	if !AllAncestorsCommitted(child) {
		t.Error("Expected all ancestors committed")
	}
}

func TestAnyAncestorDropped(t *testing.T) {
	var root [32]byte

	// Create parent and child
	parentData := NewData()
	parent := NewOverlay(root, root, parentData, nil)
	childData := NewData()
	child := NewOverlay(root, root, childData, []*Data{parentData})
	_ = child // suppress warning

	// None dropped initially
	if AnyAncestorDropped(child) {
		t.Error("Expected no ancestors dropped initially")
	}

	// Drop parent
	parent.MarkDropped()

	// Now has dropped ancestor
	if !AnyAncestorDropped(child) {
		t.Error("Expected ancestor dropped")
	}
}

func TestOverlayChainWithMultipleValues(t *testing.T) {
	var root [32]byte

	// Grandparent has key1
	grandparentData := NewData()
	grandparentData.SetValue(testKey("key1"), beatree.NewInsertChange([]byte("value1")))
	_ = NewOverlay(root, root, grandparentData, nil) // grandparent

	// Parent has key2
	parentData := NewData()
	parentData.SetValue(testKey("key2"), beatree.NewInsertChange([]byte("value2")))
	_ = NewOverlay(root, root, parentData, []*Data{grandparentData}) // parent

	// Child has key3 and overrides key1
	childData := NewData()
	childData.SetValue(testKey("key3"), beatree.NewInsertChange([]byte("value3")))
	childData.SetValue(testKey("key1"), beatree.NewInsertChange([]byte("overridden")))
	child := NewOverlay(root, root, childData, []*Data{parentData, grandparentData})

	// Child should see key3
	value, _ := child.Lookup(testKey("key3"))
	if string(value) != "value3" {
		t.Errorf("Expected value3, got %s", value)
	}

	// Child should see key2 from parent
	value, _ = child.Lookup(testKey("key2"))
	if string(value) != "value2" {
		t.Errorf("Expected value2, got %s", value)
	}

	// Child should see overridden key1
	value, _ = child.Lookup(testKey("key1"))
	if string(value) != "overridden" {
		t.Errorf("Expected overridden, got %s", value)
	}
}
