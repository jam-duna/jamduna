package beatree

import (
	"bytes"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
)

// Index is an in-memory index tracking bottom-level branch nodes with Copy-on-Write semantics.
//
// This implements proper CoW semantics for O(1) cloning and memory efficiency.
// Multiple Index instances can share the same underlying data until one of them
// is modified, at which point the modified copy gets its own data.
type Index struct {
	// Shared data structure with reference counting
	data *indexData
}

// indexData holds the actual index data with reference counting for CoW semantics.
type indexData struct {
	// Reference count for CoW semantics
	refCount int64

	// Mutex to protect modifications during CoW operations
	mu sync.RWMutex

	// Map from separator key to branch node data
	branches map[Key][]byte

	// Version number to detect concurrent modifications
	version int64
}

// NewIndex creates a new empty index with CoW semantics.
func NewIndex() *Index {
	data := &indexData{
		refCount: 1,
		branches: make(map[Key][]byte),
		version:  1,
	}
	idx := &Index{
		data: data,
	}

	// Set finalizer to handle cleanup if index is garbage collected
	runtime.SetFinalizer(idx, (*Index).finalize)
	return idx
}

// newIndexFromData creates an index with shared data (for CoW cloning).
func newIndexFromData(data *indexData) *Index {
	atomic.AddInt64(&data.refCount, 1)
	idx := &Index{
		data: data,
	}

	// Set finalizer to handle cleanup if index is garbage collected
	runtime.SetFinalizer(idx, (*Index).finalize)
	return idx
}

// ensureWritable ensures this index has its own copy of the data for writing.
// Implements the "copy" part of Copy-on-Write when modifications are needed.
func (idx *Index) ensureWritable() {
	if atomic.LoadInt64(&idx.data.refCount) == 1 {
		// Only one reference - safe to modify in place
		return
	}

	// Multiple references exist - need to copy
	idx.data.mu.RLock()

	// Create new data with copied branches
	newData := &indexData{
		refCount: 1,
		branches: make(map[Key][]byte, len(idx.data.branches)),
		version:  atomic.AddInt64(&idx.data.version, 1),
	}

	// Deep copy the branch data
	for k, v := range idx.data.branches {
		// Copy the branch data to ensure complete independence
		branchCopy := make([]byte, len(v))
		copy(branchCopy, v)
		newData.branches[k] = branchCopy
	}

	oldData := idx.data
	idx.data.mu.RUnlock()

	// Decrement reference count on old data
	atomic.AddInt64(&oldData.refCount, -1)

	// Switch to new data
	idx.data = newData
}

// Lookup finds the branch that would store the given key.
// This is a read-only operation that doesn't require CoW.
//
// Returns the branch with the highest separator <= key.
// If no such branch exists, returns (zero key, nil, false).
func (idx *Index) Lookup(key Key) (Key, []byte, bool) {
	idx.data.mu.RLock()
	defer idx.data.mu.RUnlock()

	// Find the branch with the highest separator <= key
	var bestSep Key
	var bestBranch []byte
	found := false

	for sep, branch := range idx.data.branches {
		// If separator > key, skip it
		if bytes.Compare(sep[:], key[:]) > 0 {
			continue
		}

		// If this is the first match or better than current best
		if !found || bytes.Compare(sep[:], bestSep[:]) > 0 {
			bestSep = sep
			// Return a copy of the branch data to prevent modification
			bestBranch = make([]byte, len(branch))
			copy(bestBranch, branch)
			found = true
		}
	}

	return bestSep, bestBranch, found
}

// NextKey returns the first separator greater than the given key.
// This is a read-only operation that doesn't require CoW.
// Returns (zero key, false) if no such separator exists.
func (idx *Index) NextKey(key Key) (Key, bool) {
	idx.data.mu.RLock()
	defer idx.data.mu.RUnlock()

	// Collect all separators > key
	var candidates []Key
	for sep := range idx.data.branches {
		if bytes.Compare(sep[:], key[:]) > 0 {
			candidates = append(candidates, sep)
		}
	}

	if len(candidates) == 0 {
		return Key{}, false
	}

	// Find the minimum among candidates
	sort.Slice(candidates, func(i, j int) bool {
		return bytes.Compare(candidates[i][:], candidates[j][:]) < 0
	})

	return candidates[0], true
}

// Insert adds or updates a branch with the given separator key.
// This is a write operation that triggers CoW if needed.
// Returns the previous branch if one existed.
func (idx *Index) Insert(separator Key, branch []byte) ([]byte, bool) {
	// Ensure we have our own copy for modification
	idx.ensureWritable()

	// Now we can safely modify our private copy
	idx.data.mu.Lock()
	defer idx.data.mu.Unlock()

	prev, existed := idx.data.branches[separator]

	// Make a copy of the input branch data to ensure complete independence
	branchCopy := make([]byte, len(branch))
	copy(branchCopy, branch)
	idx.data.branches[separator] = branchCopy

	// Return a copy of the previous data if it existed
	if existed {
		prevCopy := make([]byte, len(prev))
		copy(prevCopy, prev)
		return prevCopy, true
	}

	return nil, false
}

// Remove deletes the branch with the given separator key.
// This is a write operation that triggers CoW if needed.
// Returns the removed branch if it existed.
func (idx *Index) Remove(separator Key) ([]byte, bool) {
	// Ensure we have our own copy for modification
	idx.ensureWritable()

	// Now we can safely modify our private copy
	idx.data.mu.Lock()
	defer idx.data.mu.Unlock()

	branch, existed := idx.data.branches[separator]
	if existed {
		delete(idx.data.branches, separator)

		// Return a copy of the removed data
		branchCopy := make([]byte, len(branch))
		copy(branchCopy, branch)
		return branchCopy, true
	}

	return nil, false
}

// Len returns the number of branches in the index.
// This is a read-only operation that doesn't require CoW.
func (idx *Index) Len() int {
	idx.data.mu.RLock()
	defer idx.data.mu.RUnlock()
	return len(idx.data.branches)
}

// Keys returns all separator keys in sorted order.
// This is a read-only operation that doesn't require CoW.
func (idx *Index) Keys() []Key {
	idx.data.mu.RLock()
	defer idx.data.mu.RUnlock()

	keys := make([]Key, 0, len(idx.data.branches))
	for k := range idx.data.branches {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) < 0
	})

	return keys
}

// Clone creates an O(1) copy of the index using Copy-on-Write semantics.
// The cloned index shares data with the original until one of them is modified.
func (idx *Index) Clone() *Index {
	// O(1) clone by sharing the underlying data structure
	// Reference count is incremented atomically in newIndexFromData
	return newIndexFromData(idx.data)
}

// RefCount returns the current reference count for debugging purposes.
// This should not be used in production code.
func (idx *Index) RefCount() int64 {
	return atomic.LoadInt64(&idx.data.refCount)
}

// Version returns the current version number for debugging purposes.
// This should not be used in production code.
func (idx *Index) Version() int64 {
	return atomic.LoadInt64(&idx.data.version)
}

// Close explicitly releases the reference to the shared data.
// This should be called when the index is no longer needed to ensure
// prompt cleanup of shared resources.
func (idx *Index) Close() {
	if idx.data != nil {
		atomic.AddInt64(&idx.data.refCount, -1)
		idx.data = nil
		runtime.SetFinalizer(idx, nil) // Clear finalizer since we're cleaning up manually
	}
}

// finalize is called by the garbage collector to ensure proper cleanup
// of shared data when an index is garbage collected.
func (idx *Index) finalize() {
	if idx.data != nil {
		atomic.AddInt64(&idx.data.refCount, -1)
	}
}
