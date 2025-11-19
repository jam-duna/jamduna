package beatree

import (
	"github.com/colorfulnotion/jam/bmt/core"
)

const (
	// MAX_LEAF_VALUE_SIZE is the maximum size for inline values in leaf nodes.
	// Values larger than this require overflow pages.
	// This duplicates leaf.MAX_LEAF_VALUE_SIZE to avoid import cycles.
	MAX_LEAF_VALUE_SIZE = 1024
)

// ValueChange represents a change to a key-value pair in the tree.
type ValueChange int

const (
	// ValueDelete indicates the key should be deleted.
	ValueDelete ValueChange = iota

	// ValueInsert indicates a new value should be inserted (inline in leaf).
	ValueInsert

	// ValueInsertOverflow indicates a new value requiring overflow page.
	ValueInsertOverflow
)

// Change represents a key-value change with its type and data.
type Change struct {
	Type  ValueChange
	Value []byte // nil for Delete, inline value for Insert, full value for InsertOverflow
	Hash  [32]byte // Value hash for InsertOverflow only
}

// NewDeleteChange creates a delete change.
func NewDeleteChange() *Change {
	return &Change{
		Type: ValueDelete,
	}
}

// NewInsertChange creates an insert change, automatically choosing
// between inline and overflow based on value size.
func NewInsertChange(value []byte) *Change {
	if len(value) <= MAX_LEAF_VALUE_SIZE {
		return &Change{
			Type:  ValueInsert,
			Value: value,
		}
	}

	// For overflow, we need the value hash
	hasher := core.Blake2bBinaryHasher{}
	hash := hasher.Hash(value)

	return &Change{
		Type:  ValueInsertOverflow,
		Value: value,
		Hash:  hash,
	}
}

// NewInsertChangeWithHash creates an overflow insert change with explicit hash.
func NewInsertChangeWithHash(value []byte, hash [32]byte) *Change {
	return &Change{
		Type:  ValueInsertOverflow,
		Value: value,
		Hash:  hash,
	}
}

// IsDelete returns true if this is a delete change.
func (c *Change) IsDelete() bool {
	return c.Type == ValueDelete
}

// IsInsert returns true if this is an inline insert.
func (c *Change) IsInsert() bool {
	return c.Type == ValueInsert
}

// IsOverflow returns true if this is an overflow insert.
func (c *Change) IsOverflow() bool {
	return c.Type == ValueInsertOverflow
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
