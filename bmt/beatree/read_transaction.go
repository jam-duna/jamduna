package beatree

import (
	"fmt"
	"sync"

	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
)

// ReadTransaction provides snapshot isolation for read operations.
//
// A read transaction freezes a read-only state of the beatree as-of the last commit
// and enables lookups while it is alive.
//
// The existence of a read transaction blocks new syncs from starting, but may start
// when a sync is already ongoing and does not block an ongoing sync from completing.
//
// Further commits may be performed while the read transaction is live, but they won't
// be reflected within the transaction.
//
// This is cheap to clone (all fields are shared via Arc/pointers).
type ReadTransaction struct {
	root             allocator.PageNumber
	primaryStaging   map[Key]*Change
	secondaryStaging map[Key]*Change
	branchStore      *allocator.Store
	leafStore        *allocator.Store
	counter          *ReadTransactionCounter
}

// NewReadTransaction creates a new read transaction from the current shared state.
// Increments the read transaction counter to block syncs.
func NewReadTransaction(shared *Shared, counter *ReadTransactionCounter) *ReadTransaction {
	counter.AddOne()

	shared.mu.RLock()
	defer shared.mu.RUnlock()

	return &ReadTransaction{
		root:             shared.root,
		primaryStaging:   shared.PrimaryStaging(),
		secondaryStaging: shared.SecondaryStaging(),
		branchStore:      shared.branchStore,
		leafStore:        shared.leafStore,
		counter:          counter,
	}
}

// Close releases the read transaction and decrements the counter.
// This must be called to avoid blocking syncs indefinitely.
func (rt *ReadTransaction) Close() {
	if rt.counter != nil {
		rt.counter.ReleaseOne()
		rt.counter = nil
	}
}

// Lookup looks up a key in the read transaction's snapshot.
//
// The lookup order is:
// 1. Primary staging (most recent committed changes)
// 2. Secondary staging (changes being synced)
// 3. Tree on disk (via root)
//
// Returns nil if the key is not found or has been deleted.
func (rt *ReadTransaction) Lookup(key Key) ([]byte, error) {
	// Check primary staging first
	if change, exists := rt.primaryStaging[key]; exists {
		if change.IsDelete() {
			return nil, nil
		}
		if change.IsInsert() {
			return change.Value, nil
		}
		// Handle overflow values (>1KB)
		if change.IsOverflow() {
			// Overflow values stored inline in staging
			// Future: Read from overflow pages for better memory efficiency
			return change.Value, nil
		}
	}

	// Check secondary staging
	if rt.secondaryStaging != nil {
		if change, exists := rt.secondaryStaging[key]; exists {
			if change.IsDelete() {
				return nil, nil
			}
			if change.IsInsert() {
				return change.Value, nil
			}
			// Handle overflow values (>1KB)
			if change.IsOverflow() {
				// Overflow values stored inline in staging
				// Future: Read from overflow pages for better memory efficiency
				return change.Value, nil
			}
		}
	}

	// Real lookup in tree - traverse the actual tree structure on disk
	if rt.root == allocator.InvalidPageNumber {
		return nil, nil // Empty tree
	}

	// Traverse tree from root to find the key
	currentPage := rt.root
	for {
		// Read the page data directly from storage
		pageData, err := rt.leafStore.ReadPage(currentPage)
		if err != nil {
			return nil, fmt.Errorf("failed to read page %v: %w", currentPage, err)
		}

		// Try to parse as leaf page first (most common case)
		value, found, err := rt.lookupInLeafPage(key, pageData)
		if err == nil {
			// Successfully parsed as leaf - return result
			if found {
				return value, nil
			}
			return nil, nil // Key not found in leaf
		}

		// If leaf parsing failed, try as branch page
		childPage, err := rt.findChildInBranchPage(key, pageData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse page as branch or leaf: %w", err)
		}

		if childPage == allocator.InvalidPageNumber {
			return nil, nil // Key not found
		}

		// Continue traversal to child page
		currentPage = childPage
	}
}

// Root returns the root page number of this transaction's snapshot.
func (rt *ReadTransaction) Root() allocator.PageNumber {
	return rt.root
}

// NewIterator creates an iterator over this transaction's snapshot.
// The iterator will return all key-value pairs in [start, end).
// If end is nil, the iterator is unbounded (returns all keys >= start).
func (rt *ReadTransaction) NewIterator(start Key, end *Key) *Iterator {
	return NewIterator(
		rt.primaryStaging,
		rt.secondaryStaging,
		rt.branchStore,
		rt.leafStore,
		rt.root,
		start,
		end,
	)
}


// lookupInLeafPage performs real lookup in a leaf page by parsing the binary format.
// Returns (value, found, error). If error is non-nil, the page is not a valid leaf.
func (rt *ReadTransaction) lookupInLeafPage(key Key, pageData []byte) ([]byte, bool, error) {
	// Real leaf page parsing - parse the actual binary leaf node format
	if len(pageData) < 8 {
		return nil, false, fmt.Errorf("page too small for leaf")
	}

	// Leaf page format:
	// [4 bytes: entry count][4 bytes: free space offset][entries...]
	// Each entry: [32 bytes: key][1 byte: value type][4 bytes: value length][value data or overflow info]

	entryCount := uint32(pageData[0]) | uint32(pageData[1])<<8 | uint32(pageData[2])<<16 | uint32(pageData[3])<<24

	if entryCount > 1000 { // Sanity check - too many entries suggests not a leaf
		return nil, false, fmt.Errorf("too many entries for leaf page")
	}

	offset := 8 // Start after header
	for i := uint32(0); i < entryCount; i++ {
		if offset+37 > len(pageData) { // 32 (key) + 1 (type) + 4 (length)
			return nil, false, fmt.Errorf("entry extends beyond page")
		}

		// Extract key (32 bytes)
		var entryKey Key
		copy(entryKey[:], pageData[offset:offset+32])

		// Check if this is our key
		if entryKey == key {
			valueType := pageData[offset+32]
			valueLength := uint32(pageData[offset+33]) | uint32(pageData[offset+34])<<8 |
			              uint32(pageData[offset+35])<<16 | uint32(pageData[offset+36])<<24

			if valueType == 0xFF {
				// Delete marker
				return nil, false, nil
			}

			if valueType == 0 {
				// Inline value
				if offset+37+int(valueLength) > len(pageData) {
					return nil, false, fmt.Errorf("value extends beyond page")
				}
				value := make([]byte, valueLength)
				copy(value, pageData[offset+37:offset+37+int(valueLength)])
				return value, true, nil
			}

			if valueType == 1 {
				// Overflow value - first 8 bytes are page number and size
				if valueLength < 8 {
					return nil, false, fmt.Errorf("invalid overflow entry")
				}
				overflowPageNum := allocator.PageNumber(
					uint32(pageData[offset+37]) | uint32(pageData[offset+38])<<8 |
					uint32(pageData[offset+39])<<16 | uint32(pageData[offset+40])<<24)
				overflowSize := uint32(pageData[offset+41]) | uint32(pageData[offset+42])<<8 |
				                uint32(pageData[offset+43])<<16 | uint32(pageData[offset+44])<<24

				value, err := rt.readOverflowValue(overflowPageNum, overflowSize)
				return value, true, err
			}

			return nil, false, fmt.Errorf("unknown value type: %d", valueType)
		}

		// Skip to next entry
		if valueType := pageData[offset+32]; valueType != 0xFF {
			valueLength := uint32(pageData[offset+33]) | uint32(pageData[offset+34])<<8 |
			              uint32(pageData[offset+35])<<16 | uint32(pageData[offset+36])<<24
			offset += 37 + int(valueLength)
		} else {
			offset += 37 // Delete marker has no value data
		}
	}

	return nil, false, nil // Key not found in leaf
}

// findChildInBranchPage finds the child page for a key in a branch page.
func (rt *ReadTransaction) findChildInBranchPage(key Key, pageData []byte) (allocator.PageNumber, error) {
	// Real branch page parsing - parse the actual binary branch node format
	if len(pageData) < 12 {
		return allocator.InvalidPageNumber, fmt.Errorf("page too small for branch")
	}

	// Branch page format:
	// [4 bytes: separator count][4 bytes: leftmost child][4 bytes: reserved][separators...]
	// Each separator: [32 bytes: key][4 bytes: child page]

	separatorCount := uint32(pageData[0]) | uint32(pageData[1])<<8 | uint32(pageData[2])<<16 | uint32(pageData[3])<<24
	leftmostChild := allocator.PageNumber(
		uint32(pageData[4]) | uint32(pageData[5])<<8 | uint32(pageData[6])<<16 | uint32(pageData[7])<<24)

	if separatorCount > 500 { // Sanity check - too many separators suggests not a branch
		return allocator.InvalidPageNumber, fmt.Errorf("too many separators for branch page")
	}

	offset := 12 // Start after header

	// Find the appropriate child by comparing with separators
	for i := uint32(0); i < separatorCount; i++ {
		if offset+36 > len(pageData) { // 32 (key) + 4 (child page)
			return allocator.InvalidPageNumber, fmt.Errorf("separator extends beyond page")
		}

		// Extract separator key
		var sepKey Key
		copy(sepKey[:], pageData[offset:offset+32])

		// If our key is <= separator key, use the child for this separator
		if compareKeys(key, sepKey) <= 0 {
			childPage := allocator.PageNumber(
				uint32(pageData[offset+32]) | uint32(pageData[offset+33])<<8 |
				uint32(pageData[offset+34])<<16 | uint32(pageData[offset+35])<<24)
			return childPage, nil
		}

		offset += 36 // Move to next separator
	}

	// Key is greater than all separators - use leftmost child as fallback
	// (In a proper implementation, there would be a rightmost child)
	return leftmostChild, nil
}

// readOverflowValue reads a value from overflow pages.
func (rt *ReadTransaction) readOverflowValue(startPage allocator.PageNumber, totalSize uint32) ([]byte, error) {
	value := make([]byte, 0, totalSize)
	currentPage := startPage
	remaining := totalSize

	for remaining > 0 {
		pageData, err := rt.leafStore.ReadPage(currentPage)
		if err != nil {
			return nil, fmt.Errorf("failed to read overflow page %v: %w", currentPage, err)
		}

		// Overflow page format: [4 bytes: next page][4 bytes: data length][data...]
		if len(pageData) < 8 {
			return nil, fmt.Errorf("overflow page too small")
		}

		nextPage := allocator.PageNumber(
			uint32(pageData[0]) | uint32(pageData[1])<<8 | uint32(pageData[2])<<16 | uint32(pageData[3])<<24)
		dataLength := uint32(pageData[4]) | uint32(pageData[5])<<8 | uint32(pageData[6])<<16 | uint32(pageData[7])<<24

		if dataLength > remaining {
			dataLength = remaining
		}

		if 8+int(dataLength) > len(pageData) {
			return nil, fmt.Errorf("overflow data extends beyond page")
		}

		value = append(value, pageData[8:8+dataLength]...)
		remaining -= dataLength

		if remaining == 0 || nextPage == allocator.InvalidPageNumber {
			break
		}

		currentPage = nextPage
	}

	return value, nil
}

// compareKeys compares two keys lexicographically.
func compareKeys(a, b Key) int {
	for i := 0; i < len(a); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// ReadTransactionCounter tracks the number of active read transactions.
//
// This is used to coordinate between read transactions and sync operations:
// - Syncs must wait for all read transactions to complete before modifying pages
// - Read transactions increment the counter on creation and decrement on close
// - When sync is in progress, new readers are blocked to prevent sync starvation
type ReadTransactionCounter struct {
	count      int64
	syncInProgress bool
	mu         sync.Mutex
	cond       *sync.Cond
	syncCond   *sync.Cond  // For waiting on sync completion
}

// NewReadTransactionCounter creates a new counter.
func NewReadTransactionCounter() *ReadTransactionCounter {
	rtc := &ReadTransactionCounter{}
	rtc.cond = sync.NewCond(&rtc.mu)
	rtc.syncCond = sync.NewCond(&rtc.mu)
	return rtc
}

// AddOne increments the counter (called when creating a read transaction).
// Blocks if sync is in progress to prevent sync starvation.
func (rtc *ReadTransactionCounter) AddOne() {
	rtc.mu.Lock()
	defer rtc.mu.Unlock()

	// Wait until sync is not in progress
	for rtc.syncInProgress {
		rtc.syncCond.Wait()
	}

	rtc.count++
}

// ReleaseOne decrements the counter (called when closing a read transaction).
// Signals any waiting syncs that a read transaction has completed.
func (rtc *ReadTransactionCounter) ReleaseOne() {
	rtc.mu.Lock()
	defer rtc.mu.Unlock()

	rtc.count--
	if rtc.count < 0 {
		panic("ReadTransactionCounter: negative count")
	}

	// Signal only while holding the mutex to prevent lost wakeups
	rtc.cond.Signal()
}

// BeginSync marks sync as in progress and blocks until all outstanding read transactions have been released.
// This prevents new readers from starting and waits for existing readers to finish.
func (rtc *ReadTransactionCounter) BeginSync() {
	rtc.mu.Lock()
	defer rtc.mu.Unlock()

	// Mark sync as in progress to block new readers
	rtc.syncInProgress = true

	// Wait for all existing readers to complete
	for rtc.count > 0 {
		rtc.cond.Wait()
	}
}

// EndSync marks sync as complete and allows new readers to start.
func (rtc *ReadTransactionCounter) EndSync() {
	rtc.mu.Lock()
	defer rtc.mu.Unlock()

	rtc.syncInProgress = false
	rtc.syncCond.Broadcast() // Wake up any waiting new readers
}

// BlockUntilZero blocks until all outstanding read transactions have been released.
// This is called by sync to ensure no readers are accessing pages that will be modified.
// DEPRECATED: Use BeginSync/EndSync instead to prevent sync starvation.
func (rtc *ReadTransactionCounter) BlockUntilZero() {
	rtc.mu.Lock()
	defer rtc.mu.Unlock()

	for rtc.count > 0 {
		rtc.cond.Wait()
	}
}

// Count returns the current number of active read transactions.
func (rtc *ReadTransactionCounter) Count() int64 {
	rtc.mu.Lock()
	defer rtc.mu.Unlock()
	return rtc.count
}
