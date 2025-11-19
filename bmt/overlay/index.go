package overlay

import (
	"sync"

	"github.com/colorfulnotion/jam/bmt/beatree"
)

// Index maps keys to the ancestor index that contains the latest value.
// This enables O(1) lookup after building the index, avoiding linear scans.
type Index struct {
	mu sync.RWMutex

	// Maps key to ancestor index (0 = this overlay, 1 = parent, 2 = grandparent, etc.)
	keyToAncestor map[beatree.Key]int
}

// NewIndex creates a new empty index.
func NewIndex() *Index {
	return &Index{
		keyToAncestor: make(map[beatree.Key]int),
	}
}

// Set records that a key's latest value is in the given ancestor.
func (idx *Index) Set(key beatree.Key, ancestorIndex int) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.keyToAncestor[key] = ancestorIndex
}

// Get returns the ancestor index for a key.
// Returns (ancestorIndex, true) if found, (0, false) otherwise.
// NOTE: Must check the bool return value - a zero ancestorIndex with false
// means the key is not present, while (0, true) means it's in this overlay.
func (idx *Index) Get(key beatree.Key) (int, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	ancestorIndex, ok := idx.keyToAncestor[key]
	if !ok {
		return 0, false
	}
	return ancestorIndex, true
}

// Has returns true if the key is in the index.
func (idx *Index) Has(key beatree.Key) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	_, ok := idx.keyToAncestor[key]
	return ok
}

// Remove removes a key from the index.
func (idx *Index) Remove(key beatree.Key) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	delete(idx.keyToAncestor, key)
}

// Size returns the number of keys in the index.
func (idx *Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return len(idx.keyToAncestor)
}

// Clone creates a deep copy of the index.
func (idx *Index) Clone() *Index {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	newIdx := NewIndex()
	for k, v := range idx.keyToAncestor {
		newIdx.keyToAncestor[k] = v
	}
	return newIdx
}

// Merge merges another index into this one.
// For conflicting keys, the other index's value takes precedence.
func (idx *Index) Merge(other *Index) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	other.mu.RLock()
	defer other.mu.RUnlock()

	for k, v := range other.keyToAncestor {
		idx.keyToAncestor[k] = v
	}
}

// GetAll returns a copy of all key-to-ancestor mappings.
func (idx *Index) GetAll() map[beatree.Key]int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	result := make(map[beatree.Key]int, len(idx.keyToAncestor))
	for k, v := range idx.keyToAncestor {
		result[k] = v
	}
	return result
}
