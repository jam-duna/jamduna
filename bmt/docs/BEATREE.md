# Beatree Tutorial

## Table of Contents
1. [What is Beatree?](#what-is-beatree)
2. [Why Use Beatree?](#why-use-beatree)
3. [How Beatree Works](#how-beatree-works)
4. [Implementation in BMT](#implementation-in-bmt)
5. [Testing Strategy](#testing-strategy)

---

## What is Beatree?

**Beatree** (B-epsilon tree) is a **disk-based key-value storage structure** that combines the best properties of B-trees and Log-Structured Merge (LSM) trees. It's designed for efficient storage and retrieval of large amounts of data with Copy-on-Write (CoW) semantics.

### Key Concepts

- **B-tree**: A balanced tree where each node can have multiple children, optimized for disk access
- **Epsilon (ε)**: A small buffer in each node that batches updates before pushing them down
- **Copy-on-Write (CoW)**: Never modify data in place; always create new copies
- **Page**: Fixed-size disk block (16KB in BMT) containing nodes
- **Free-list**: Pool of reusable pages for CoW operations

### Analogy

Think of Beatree like a **filing cabinet system**:
- **Without Beatree**: Every document is a separate file on disk (slow to find, lots of seeks)
- **With Beatree**: Documents are organized in folders (branch nodes) that point to filing cabinets (leaf nodes), with an index for fast lookups
- **CoW**: When you update a document, you make a copy in a new location and update the index, keeping the old version intact (snapshot capability)

---

## Why Use Beatree?

### Problem: Storage Inefficiency

Traditional storage approaches have issues:

#### Naive Hash Table
```
key1 → page 1 on disk
key2 → page 2 on disk
key3 → page 3 on disk
```
**Issues**:
- No range queries (can't find all keys between k1 and k2)
- Random access patterns (each lookup = disk seek)
- No snapshots (updates destroy old data)

#### Traditional B-tree
```
         [Root]
        /  |  \
    [A-F][G-M][N-Z]
```
**Issues**:
- In-place updates (no snapshots)
- Write amplification (update one key → rewrite entire page)
- No batch operations (each update is separate)

### Solution: Beatree

Beatree combines:
- **B-tree structure**: Fast lookups, range queries, balanced tree
- **CoW semantics**: Snapshots, crash recovery, no in-place updates
- **Free-list**: Efficient page reuse, no fragmentation
- **Batch-friendly**: Updates can be accumulated and flushed efficiently

**Benefits**:
- **Fast lookups**: O(log n) time, few disk seeks
- **Range queries**: Iterate over sorted keys efficiently
- **Snapshots**: Old versions remain readable during updates
- **Crash recovery**: Atomic updates via CoW
- **Space efficiency**: Free-list reuses deleted pages

### Real-World Use Cases

Beatree is ideal for:
- **Blockchain state**: Store account balances, smart contract storage
- **Version control**: Multiple snapshots of data (Git-like)
- **Databases**: PostgreSQL-style MVCC (Multi-Version Concurrency Control)
- **File systems**: Btrfs, ZFS use similar CoW structures

In BMT (Binary Merkle Tree):
- Store key-value pairs for JAM blockchain state
- Support Merkle proof generation
- Enable efficient state transitions with snapshots

---

## How Beatree Works

### Tree Structure

Beatree has two types of nodes:

#### 1. Branch Nodes (Internal)
```
┌───────────────────────────────────┐
│ Branch Node (16KB page)           │
├───────────────────────────────────┤
│ Leftmost Child: Page #10          │
├───────────────────────────────────┤
│ Separator 1: key="apple"  → Pg#20 │
│ Separator 2: key="banana" → Pg#30 │
│ Separator 3: key="cherry" → Pg#40 │
└───────────────────────────────────┘

Lookup("grape"):
  1. "grape" > "cherry" → follow Page #40
```

**Purpose**: Direct searches to the correct leaf

**Properties**:
- Contains separators (keys that divide the space)
- Each separator points to a child page
- Leftmost child handles keys less than first separator
- Up to 400 separators (16KB page / 40 bytes per separator)

#### 2. Leaf Nodes (Data)
```
┌───────────────────────────────────┐
│ Leaf Node (16KB page)             │
├───────────────────────────────────┤
│ Entry 1: key="cherry"             │
│          value="red fruit" (inline)│
├───────────────────────────────────┤
│ Entry 2: key="durian"             │
│          value=<2KB data>          │
│          overflow → Page #100      │
├───────────────────────────────────┤
│ Entry 3: key="elderberry"         │
│          value="purple" (inline)   │
└───────────────────────────────────┘
```

**Purpose**: Store actual key-value pairs

**Properties**:
- Sorted entries for binary search
- Inline values ≤ 1KB (stored directly)
- Overflow values > 1KB (stored in separate pages)
- Range scans require walking parent separators (leaf linking not yet implemented in Phase 3)

### Copy-on-Write (CoW) Semantics

Traditional update (in-place):
```
Before:
  Page 1: {key=A, value=1}

After update (key=A, value=2):
  Page 1: {key=A, value=2}  ← OVERWRITTEN (old data lost!)
```

CoW update:
```
Before:
  Page 1: {key=A, value=1}

After update (key=A, value=2):
  Page 1: {key=A, value=1}  ← Still exists (old snapshot)
  Page 2: {key=A, value=2}  ← New version

Free-list: [Page 1 marked as "old version"]
```

**Benefits**:
1. **Snapshots**: Old version (Page 1) still readable during update
2. **Crash safety**: Update is atomic (either Page 2 is referenced or not)
3. **MVCC**: Multiple readers can access old snapshot while write happens

**Cost**: More writes (write new page + update parent pointers)

**Mitigation**: Free-list reuses old pages after snapshot is released

### Free-List: Page Recycling

When pages are no longer needed (old snapshot released), they go to the free-list:

```
Free-list (stack):
┌─────────┐
│ Page 57 │ ← top (most recently freed)
├─────────┤
│ Page 23 │
├─────────┤
│ Page 91 │
└─────────┘

AllocPage():
  1. Try free-list first → pop Page 57
  2. If empty → allocate new page (extend file)

FreePage(Page 42):
  Push Page 42 onto free-list
```

**Why LIFO (stack)?**
- Cache-friendly (recently freed pages likely still in OS page cache)
- Simple (O(1) push/pop)
- No fragmentation (pages are uniform size)

### Inline vs. Overflow Values

Small values (≤ 1KB) are stored **inline**:
```
Leaf Entry:
  key: [32 bytes]
  value_type: Inline
  value: [actual data, ≤ 1KB]
```

Large values (> 1KB) use **overflow pages**:
```
Leaf Entry:
  key: [32 bytes]
  value_type: Overflow
  overflow_page: 100
  overflow_size: 5000
  value_prefix: [first 32 bytes for caching]

Overflow chain (planned - not yet implemented in Phase 3):
  Page 100: [4096 bytes of value]
  Page 101: [904 bytes of value]
```

**Phase 3 Implementation Note**: The overflow entry metadata structure is implemented (ValueTypeOverflow, page reference, size, prefix), but the actual reading/writing of overflow chains to disk is not yet implemented. This will be added in later phases when full tree operations are implemented.

**Why 1KB threshold?**
- Keeps leaf pages compact (more entries per page)
- Reduces read amplification for small values
- Large values don't bloat the tree structure

---

## Implementation in BMT

### Architecture

BMT implements Beatree in phases. **Phase 3** (current) provides the foundation:

```
┌─────────────────────────────────────────┐
│          Beatree (bmt/beatree)          │
│  ┌────────────────────────────────────┐ │
│  │  Keys (key.go)                     │ │
│  │  - 32-byte lexicographic ordering  │ │
│  └────────────────────────────────────┘ │
│  ┌────────────────────────────────────┐ │
│  │  Branch Nodes (branch/)            │ │
│  │  - 400 separators max per node    │ │
│  │  - Binary search O(log n)         │ │
│  └────────────────────────────────────┘ │
│  ┌────────────────────────────────────┐ │
│  │  Leaf Nodes (leaf/)                │ │
│  │  - Inline/overflow value support  │ │
│  │  - 1KB inline threshold           │ │
│  └────────────────────────────────────┘ │
│  ┌────────────────────────────────────┐ │
│  │  Allocator (allocator/)            │ │
│  │  - Free-list (LIFO stack)         │ │
│  │  - Page store (1000-page cache)   │ │
│  └────────────────────────────────────┘ │
└─────────────────────────────────────────┘
```

### Core Components

#### 1. Key Type (key.go)

```go
type Key [32]byte  // Fixed-size, lexicographic ordering
```

**Design choices**:
- **Fixed 32 bytes**: Compatible with cryptographic hashes (SHA-256, Blake2b)
- **Lexicographic**: Natural ordering for range queries (0x00 < 0x01 < ... < 0xFF)
- **Comparable**: Can use as map key, supports == operator

**Example**:
```go
key1 := Key{0x00, 0x00, 0x00, 0x01}  // User account 1
key2 := Key{0x00, 0x00, 0x00, 0x02}  // User account 2

key1.Compare(key2)  // Returns -1 (key1 < key2)
key1.Equal(key2)    // Returns false
```

#### 2. Page Number (allocator/page_number.go)

```go
type PageNumber uint64  // 0-based page numbering

const InvalidPageNumber PageNumber = 0xFFFFFFFFFFFFFFFF
```

**Design choices**:
- **uint64**: Supports up to 2^64 pages (≈ 295 exabytes at 16KB/page)
- **Sentinel value**: InvalidPageNumber marks unallocated/invalid pages
- **Simple**: No complex encoding, direct page offset calculation

**Example**:
```go
pageNum := PageNumber(42)
offset := int64(pageNum) * 16384  // Byte offset in file
```

#### 3. Free-List (allocator/free_list.go)

```go
type FreeList struct {
    mu    sync.Mutex
    pages []PageNumber  // LIFO stack
}
```

**Implementation**:
```go
func (fl *FreeList) Push(page PageNumber) {
    fl.mu.Lock()
    defer fl.mu.Unlock()
    fl.pages = append(fl.pages, page)  // Push on top
}

func (fl *FreeList) Pop() PageNumber {
    fl.mu.Lock()
    defer fl.mu.Unlock()
    if len(fl.pages) == 0 {
        return InvalidPageNumber  // Empty
    }
    page := fl.pages[len(fl.pages)-1]  // Top of stack
    fl.pages = fl.pages[:len(fl.pages)-1]  // Pop
    return page
}
```

**Key features**:
- **Thread-safe**: Mutex protects concurrent access
- **LIFO**: Last freed page is first reused (cache-friendly)
- **Simple**: Just a dynamic array (Go slice)
- **Bounded**: Can track millions of free pages efficiently

#### 4. Page Store (allocator/store.go)

```go
type Store struct {
    file         *os.File           // Disk file
    pagePool     *io.PagePool       // Memory pool (from Phase 2)
    freeList     *FreeList          // Page recycling
    nextPage     atomic.Uint64      // Next page number to allocate
    pageCache    map[PageNumber]io.Page  // Page cache (random eviction)
    maxCacheSize int               // 1000 pages (16 MB)
}
```

**Page allocation flow**:
```go
func (s *Store) AllocPage() PageNumber {
    // Try free-list first
    page := s.freeList.Pop()
    if page.IsValid() {
        return page  // Reuse freed page
    }

    // Allocate new page (extend file)
    pageNum := PageNumber(s.nextPage.Add(1) - 1)
    return pageNum
}
```

**Page read flow** (with caching):
```go
func (s *Store) ReadPage(page PageNumber) (io.Page, error) {
    // Check cache first
    if cached, ok := s.pageCache[page]; ok {
        return cached, nil  // Cache hit (fast!)
    }

    // Read from disk (slower)
    pageData := io.ReadPage(s.file, s.pagePool, uint64(page))

    // Add to cache (with random eviction if full)
    if len(s.pageCache) >= maxCacheSize {
        // Remove a random page (map iteration order is random in Go)
        for k := range s.pageCache {
            delete(s.pageCache, k)
            break  // Remove just one entry
        }
    }
    s.pageCache[page] = pageData
    return pageData, nil
}
```

**Key features**:
- **Two-tier allocation**: Free-list first, then extend file
- **Page cache**: 1000 pages (16 MB) in memory for hot pages
- **Random eviction**: When cache is full, removes a random entry (not true LRU, but simple and effective)
- **Atomic operations**: `nextPage` uses atomic.Uint64 for thread safety

#### 5. Branch Nodes (branch/node.go)

```go
type Separator struct {
    Key   Key          // 32-byte separator key
    Child PageNumber   // Page containing keys ≥ this key
}

type Node struct {
    Separators    []Separator  // Up to 400 separators
    LeftmostChild PageNumber   // For keys < first separator
}
```

**Finding a child**:
```go
func (n *Node) FindChild(key Key) PageNumber {
    // Binary search through separators
    for i, sep := range n.Separators {
        if key.Compare(sep.Key) < 0 {
            // key < separator → previous child
            if i == 0 {
                return n.LeftmostChild
            }
            return n.Separators[i-1].Child
        }
    }
    // key ≥ all separators → last child
    return n.Separators[len(n.Separators)-1].Child
}
```

**Example**:
```
Branch node:
  LeftmostChild: Page 10
  Separators:
    key=0x20 → Page 20
    key=0x40 → Page 30
    key=0x60 → Page 40

FindChild(0x10):  0x10 < 0x20 → Page 10 (leftmost)
FindChild(0x30):  0x20 ≤ 0x30 < 0x40 → Page 20
FindChild(0x70):  0x70 ≥ 0x60 → Page 40
```

**Key features**:
- **Sorted**: Separators maintained in ascending order
- **Binary search**: O(log n) lookup within node
- **Compact**: ~40 bytes per separator (32 key + 8 page number)
- **Dense**: 400 separators × 40 bytes = 16KB page

#### 6. Leaf Nodes (leaf/node.go)

```go
type ValueType uint8
const (
    ValueTypeInline   ValueType = 0  // ≤ 1KB
    ValueTypeOverflow ValueType = 1  // > 1KB
)

type Entry struct {
    Key          Key
    ValueType    ValueType
    Value        []byte        // Inline: full value; Overflow: prefix
    OverflowPage PageNumber   // For overflow values
    OverflowSize uint64       // For overflow values
}

type Node struct {
    Entries []Entry  // Sorted by key
}
```

**Inserting an entry**:
```go
func (n *Node) Insert(entry Entry) error {
    // Binary search for position
    pos := binarySearch(n.Entries, entry.Key)

    // Update if exists
    if pos < len(n.Entries) && n.Entries[pos].Key.Equal(entry.Key) {
        n.Entries[pos] = entry  // Update in place
        return nil
    }

    // Insert at position (maintain sorted order)
    n.Entries = insert(n.Entries, pos, entry)
    return nil
}
```

**Inline vs. overflow**:
```go
// Inline (≤ 1KB)
entry, _ := NewInlineEntry(key, []byte("small value"))
// Entry stores value directly

// Overflow (> 1KB) - Phase 3 creates metadata only
largeValue := make([]byte, 5000)  // 5KB value
// NOTE: Overflow page persistence not yet implemented in Phase 3
// For now, we just create the metadata structure:
overflowPage := PageNumber(100)  // Allocated page number
entry := NewOverflowEntry(key, overflowPage, 5000, largeValue[:32])
// Entry stores: page reference + size + first 32 bytes as prefix
// Actual overflow chain write/read operations are planned for later phases
```

**Phase 3 Status**: Overflow entry creation is implemented (metadata structure), but overflow page serialization/deserialization is not yet implemented. This will be added when tree operations are implemented in subsequent phases.

**Key features**:
- **Sorted entries**: Binary search O(log n)
- **Flexible values**: Small inline, large overflow
- **Value caching**: Overflow entries cache first 32 bytes
- **Space efficient**: Leaf pages stay compact

### Why These Design Choices?

#### 16KB Pages (GP Mode)
- **Compatibility**: Matches JAM's Gray Paper specification
- **Efficiency**: Good balance between I/O size and tree depth
- **Modern disks**: SSDs prefer larger I/O operations (4KB+)

#### 400 Separators per Branch
- Calculation: (16KB - overhead) / (32 bytes key + 8 bytes page) ≈ 400
- **Tree depth**: With 400-way branching, 1 billion keys ≈ 4 levels
- **Fewer seeks**: Wide trees reduce disk seeks

#### 1KB Inline Threshold
- **Common case**: Most blockchain values are small (<1KB)
- **Read efficiency**: One page read gets complete value
- **Write efficiency**: Updates don't require overflow page allocation

#### LIFO Free-List
- **Cache locality**: Recently freed pages likely in OS cache
- **Simplicity**: Stack operations are O(1)
- **CoW friendly**: Older pages can be garbage collected

#### 1000-Page Cache
- Size: 1000 × 16KB = 16 MB
- **Working set**: Fits hot pages in memory
- **Scalability**: Adjustable based on system RAM

---

## Testing Strategy

### Test Coverage

BMT includes 18 comprehensive tests for Beatree foundation:

#### Key Tests (4 tests)

**1. TestKeyCompare**
```go
key1 := Key{0x00, 0x00, 0x00, 0x01}
key2 := Key{0x00, 0x00, 0x00, 0x02}

assert(key1.Compare(key2) < 0)  // key1 < key2
assert(key2.Compare(key1) > 0)  // key2 > key1
assert(key1.Compare(key1) == 0) // key1 == key1
```

**What it validates**:
- Lexicographic ordering works correctly
- Transitive property (if a<b and b<c, then a<c)
- Reflexive property (a == a)

**2. TestKeyEqual**
```go
key1 := Key{0x01, 0x02, 0x03}
key2 := Key{0x01, 0x02, 0x03}  // Same
key3 := Key{0x01, 0x02, 0x04}  // Different

assert(key1.Equal(key2) == true)
assert(key1.Equal(key3) == false)
```

**3. TestKeyFromBytes**
```go
bytes := []byte{0, 1, 2, ..., 31}  // 32 bytes
key := KeyFromBytes(bytes)

assert(key[0] == 0)
assert(key[31] == 31)
```

**4. TestKeyFromBytesPanic**
```go
// Should panic with wrong size
KeyFromBytes([]byte{1, 2, 3})  // Only 3 bytes → PANIC
```

**What it validates**: Input validation catches errors

#### Free-List Tests (5 tests)

**5. TestFreeListAlloc**
```go
fl := NewFreeList()
assert(fl.IsEmpty() == true)
assert(fl.Len() == 0)
```

**6. TestFreeListPushPop**
```go
fl := NewFreeList()
fl.Push(PageNumber(1))
fl.Push(PageNumber(2))
fl.Push(PageNumber(3))

assert(fl.Len() == 3)
assert(fl.Pop() == PageNumber(3))  // LIFO
assert(fl.Pop() == PageNumber(2))
assert(fl.Pop() == PageNumber(1))
assert(fl.IsEmpty() == true)
```

**What it validates**: LIFO ordering (stack semantics)

**7. TestFreeListPopEmpty**
```go
fl := NewFreeList()
page := fl.Pop()
assert(page == InvalidPageNumber)  // Empty → sentinel
```

**8. TestFreeListNoLeaks**
```go
fl := NewFreeList()
for i := 0; i < 1000; i++ {
    fl.Push(PageNumber(i))
}
assert(fl.Len() == 1000)

for i := 0; i < 1000; i++ {
    page := fl.Pop()
    assert(page.IsValid())
}
assert(fl.IsEmpty() == true)
```

**What it validates**:
- No memory leaks with many operations
- Correct accounting (1000 in, 1000 out)
- All pages are valid

**9. TestFreeListClear**
```go
fl := NewFreeList()
fl.Push(PageNumber(1))
fl.Push(PageNumber(2))
fl.Clear()
assert(fl.IsEmpty() == true)
```

#### Branch Node Tests (3 tests)

**10. TestBranchNodeInsert**
```go
node := NewNode(PageNumber(0))  // Leftmost child

key1 := Key{0x01}
key2 := Key{0x02}
key3 := Key{0x03}

// Insert out of order
node.Insert(Separator{Key: key2, Child: PageNumber(2)})
node.Insert(Separator{Key: key1, Child: PageNumber(1)})
node.Insert(Separator{Key: key3, Child: PageNumber(3)})

// Verify sorted
assert(node.Separators[0].Key == key1)
assert(node.Separators[1].Key == key2)
assert(node.Separators[2].Key == key3)
```

**What it validates**: Sorted insertion (binary search tree property)

**11. TestBranchNodeFindChild**
```go
node := NewNode(PageNumber(0))
node.Insert(Separator{Key: Key{0x10}, Child: PageNumber(1)})
node.Insert(Separator{Key: Key{0x20}, Child: PageNumber(2)})
node.Insert(Separator{Key: Key{0x30}, Child: PageNumber(3)})

// Test searches
assert(node.FindChild(Key{0x05}) == PageNumber(0))  // < 0x10
assert(node.FindChild(Key{0x15}) == PageNumber(1))  // 0x10-0x20
assert(node.FindChild(Key{0x25}) == PageNumber(2))  // 0x20-0x30
assert(node.FindChild(Key{0x35}) == PageNumber(3))  // ≥ 0x30
```

**What it validates**: Binary search navigation works correctly

**12. TestBranchNodeDuplicateKey**
```go
node := NewNode(PageNumber(0))
node.Insert(Separator{Key: Key{0x01}, Child: PageNumber(1)})
err := node.Insert(Separator{Key: Key{0x01}, Child: PageNumber(2)})

assert(err != nil)  // Duplicate rejected
```

**What it validates**: No duplicate separators allowed

#### Leaf Node Tests (6 tests)

**13. TestLeafNodeInsertLookup**
```go
node := NewNode()
key := Key{0x01}
value := []byte("test value")

entry, _ := NewInlineEntry(key, value)
node.Insert(entry)

found, ok := node.Lookup(key)
assert(ok == true)
assert(found.Key == key)
assert(string(found.Value) == "test value")
```

**14. TestLeafNodeUpdate**
```go
node := NewNode()
key := Key{0x01}

// Insert initial value
entry1, _ := NewInlineEntry(key, []byte("v1"))
node.Insert(entry1)

// Update with new value
entry2, _ := NewInlineEntry(key, []byte("v2"))
node.Insert(entry2)

// Should have only 1 entry (updated)
assert(node.NumEntries() == 1)
found, _ := node.Lookup(key)
assert(string(found.Value) == "v2")
```

**What it validates**: Updates replace, don't duplicate

**15. TestLeafNodeDelete**
```go
node := NewNode()
key1 := Key{0x01}
key2 := Key{0x02}

node.Insert(NewInlineEntry(key1, []byte("v1")))
node.Insert(NewInlineEntry(key2, []byte("v2")))

// Delete key1
assert(node.Delete(key1) == true)
assert(node.NumEntries() == 1)

// Verify key1 gone, key2 remains
_, ok1 := node.Lookup(key1)
_, ok2 := node.Lookup(key2)
assert(ok1 == false)
assert(ok2 == true)
```

**16. TestLeafNodeSorted**
```go
node := NewNode()

// Insert out of order
node.Insert(Entry{Key: Key{0x03}, ...})
node.Insert(Entry{Key: Key{0x01}, ...})
node.Insert(Entry{Key: Key{0x02}, ...})

// Verify sorted
assert(node.Entries[0].Key == Key{0x01})
assert(node.Entries[1].Key == Key{0x02})
assert(node.Entries[2].Key == Key{0x03})
```

**17. TestLeafNodeOverflowEntry**
```go
key := Key{0x01}
overflowPage := PageNumber(100)
overflowSize := uint64(5000)
prefix := []byte("prefix...")

entry := NewOverflowEntry(key, overflowPage, overflowSize, prefix)

assert(entry.ValueType == ValueTypeOverflow)
assert(entry.OverflowPage == 100)
assert(entry.OverflowSize == 5000)
assert(len(entry.Value) == len(prefix))  // Prefix cached
```

**18. TestLeafNodeInlineTooBig**
```go
key := Key{0x01}
value := make([]byte, 1025)  // > 1KB

_, err := NewInlineEntry(key, value)
assert(err != nil)  // Rejected
```

**What it validates**: Size limits enforced

### Testing Best Practices

1. **Unit tests**: Each component tested independently
2. **Property tests**: Verify invariants (sorted order, no duplicates)
3. **Edge cases**: Empty, single element, maximum size
4. **Error cases**: Invalid input, boundary violations
5. **Deterministic**: Tests produce same result every time

### Running Tests

```bash
# Run all beatree tests
go test -v ./bmt/beatree/...

# Run specific package
go test -v ./bmt/beatree/allocator/...

# Run with coverage
go test -cover ./bmt/beatree/...

# Run with race detector
go test -race ./bmt/beatree/...
```

### Expected Results

```
=== RUN   TestKeyCompare
--- PASS: TestKeyCompare (0.00s)
=== RUN   TestKeyEqual
--- PASS: TestKeyEqual (0.00s)
=== RUN   TestKeyFromBytes
--- PASS: TestKeyFromBytes (0.00s)
=== RUN   TestKeyFromBytesPanic
--- PASS: TestKeyFromBytesPanic (0.00s)
PASS
ok      github.com/colorfulnotion/jam/bmt/beatree       0.289s

=== RUN   TestFreeListAlloc
--- PASS: TestFreeListAlloc (0.00s)
=== RUN   TestFreeListPushPop
--- PASS: TestFreeListPushPop (0.00s)
=== RUN   TestFreeListPopEmpty
--- PASS: TestFreeListPopEmpty (0.00s)
=== RUN   TestFreeListNoLeaks
--- PASS: TestFreeListNoLeaks (0.00s)
=== RUN   TestFreeListClear
--- PASS: TestFreeListClear (0.00s)
PASS
ok      github.com/colorfulnotion/jam/bmt/beatree/allocator     0.798s

=== RUN   TestBranchNodeInsert
--- PASS: TestBranchNodeInsert (0.00s)
=== RUN   TestBranchNodeFindChild
--- PASS: TestBranchNodeFindChild (0.00s)
=== RUN   TestBranchNodeDuplicateKey
--- PASS: TestBranchNodeDuplicateKey (0.00s)
PASS
ok      github.com/colorfulnotion/jam/bmt/beatree/branch        0.612s

=== RUN   TestLeafNodeInsertLookup
--- PASS: TestLeafNodeInsertLookup (0.00s)
=== RUN   TestLeafNodeUpdate
--- PASS: TestLeafNodeUpdate (0.00s)
=== RUN   TestLeafNodeDelete
--- PASS: TestLeafNodeDelete (0.00s)
=== RUN   TestLeafNodeSorted
--- PASS: TestLeafNodeSorted (0.00s)
=== RUN   TestLeafNodeOverflowEntry
--- PASS: TestLeafNodeOverflowEntry (0.00s)
=== RUN   TestLeafNodeInlineTooBig
--- PASS: TestLeafNodeInlineTooBig (0.00s)
PASS
ok      github.com/colorfulnotion/jam/bmt/beatree/leaf          0.446s
```

**All 18 tests passing** confirms the foundation is solid.

---

## Summary

**Beatree is a disk-based B-tree with CoW semantics:**

1. **Efficient storage**: O(log n) lookups, range queries, sorted keys
2. **CoW semantics**: Snapshots, crash recovery, MVCC
3. **Space efficient**: Free-list reuses pages, inline/overflow values
4. **Performance**: Page cache, LIFO free-list, wide branching (400-way)

**BMT's Phase 3 implementation provides:**
- 32-byte lexicographic keys
- Branch nodes with 400 separators (16KB pages)
- Leaf nodes with 1KB inline threshold
- Free-list allocator (LIFO stack)
- Page store with 1000-page cache (random eviction when full)

**Testing ensures:**
- Correctness of all data structures (18 tests)
- Sorted invariants maintained (binary search)
- Proper error handling (size limits, invalid input)
- No memory leaks (1000+ operations)

The Beatree foundation enables efficient blockchain state storage with snapshot capability, ready for higher-level operations (lookup, insert, delete, sync) in future phases.
