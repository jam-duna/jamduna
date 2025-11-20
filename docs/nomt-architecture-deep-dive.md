# NOMT Architecture Deep Dive: How 50,000 Key-Value Pairs Get Stored

> **Note on Page Sizes**: The current implementation uses **16KB pages** (see [bmt/io/page_pool.go:11](../bmt/io/page_pool.go#L11), [bmt/beatree/leaf/constants.go:10](../bmt/beatree/leaf/constants.go#L10), [bmt/beatree/branch/constants.go:8](../bmt/beatree/branch/constants.go#L8)). Many examples in this document reference 4KB pages for illustration purposes. When calculating actual storage capacity, node fanout, or tree depth, multiply the capacities shown in examples by **4×** (e.g., a leaf that holds ~30 entries with 4KB pages would hold ~120 entries with 16KB pages).

## Overview: The Storage Stack

JAM's state storage uses a **layered architecture** where each component has a specific responsibility:

```
┌─────────────────────────────────────────────────────────────┐
│ StateDB Layer (statedb/statedb.go, statedb/genesis.go)      │
│ - User-facing API: Insert(), Flush(), Root(), Lookup()      │
│ - Entry point: NewStateDBFromStateKeyVals()                 │
│ - Manages block numbers, metadata, state transitions        │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│ Storage Layer (storage/bmt_storage.go)                      │
│ - Storage implementation wrapping BMT database              │
│ - Direct BMT session API: Insert() → bmtDB.Insert()         │
│ - Flush() → bmtDB.Commit() → session.Root()                 │
│ - Tracks root history for rollback support                  │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│ BMT Database Layer (bmt/bmt.go, bmt/beatree_wrapper.go)     │
│ - Wraps Beatree with session management                     │
│ - Insert/Delete operations on active session                │
│ - Commit() creates new immutable session                    │
│ - Computes GP tree root hash for each session               │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│ Beatree Layer (bmt/beatree/)                                │
│ - B+ tree implementation (sorted key-value storage)         │
│ - Leaf nodes: Store actual key-value pairs                  │
│ - Branch nodes: Store separators (keys) + child pointers    │
│ - Page-based storage (16KB pages)                           │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│ BitBox Layer (bmt/bitbox/)                                  │
│ - Raw page storage backend (mmap'd file or in-memory)       │
│ - Allocates/deallocates pages                               │
│ - Manages free lists, compaction, persistence               │
└─────────────────────────────────────────────────────────────┘
```

## Example Walkthrough: Importing 50,000 Key-Value Pairs

Let's trace how 50,000 new key-value pairs flow through the entire system using the actual APIs. This number is chosen to demonstrate a proper 2-level tree with 16KB pages (~417 leaves requiring ~2 branch pages + 1 root).

### Step 1: User Calls NewStateDBFromStateKeyVals() or Insert()

**File**: `statedb/genesis.go:NewStateDBFromStateKeyVals()` or `statedb/statedb.go:UpdateAllTrieKeyVals()`

```go
// Actual code path (from fuzzer and tests):
stateKeyVals := &StateKeyVals{
    KeyVals: []StateKeyVal{
        {Key: [32]byte{0x12, 0x34, ...}, Value: []byte{...}},
        {Key: [32]byte{0x56, 0x78, ...}, Value: []byte{...}},
        // ... 49,998 more entries
    }
}

// Create StateDB from key-value pairs
stateDB, err := statedb.NewStateDBFromStateKeyVals(store, stateKeyVals)
```

**What happens** (inside `UpdateAllTrieKeyVals()`):
```go
func (s *StateDB) UpdateAllTrieKeyVals(skv StateKeyVals) (common.Hash, error) {
    // Insert all key-value pairs directly into BMT session
    for _, kv := range skv.KeyVals {
        s.sdb.Insert(kv.Key[:], kv.Value)  // Mutates in-memory BMT session
    }

    // Commit BMT session and compute root hash
    root, err := s.sdb.Flush()
    return root, nil
}
```

**Key points**:
- Each `Insert()` call mutates the in-memory BMT session (no disk I/O yet)
- `Flush()` calls `bmtDB.Commit()` which writes the updated B+ tree pages to disk
- Each key is 32 bytes (Blake2b hash of storage key)
- Values are variable-length (1 byte to many KB)

### Step 2: Storage.Insert() Applies Directly to BMT Session

**File**: [storage/bmt_storage.go:95-106](../storage/bmt_storage.go#L95-L106)

**What happens**:
```go
func (t *StateDBStorage) Insert(keyBytes []byte, value []byte) {
    key := normalizeKey32(keyBytes)
    var key32 [32]byte
    copy(key32[:], key[:])

    // Apply directly to BMT session (no pendingChanges map!)
    if err := t.bmtDB.Insert(key32, value); err != nil {
        return
    }

    t.keys[key] = true  // Track key for enumeration
}
```

**How it works**:
1. **Normalize key** to 32 bytes (Blake2b hash of storage key)
2. **Insert directly** into BMT database session via `t.bmtDB.Insert(key32, value)`
3. **Track key** for later enumeration (used by `Keys()` method)

**Important differences from batched approach**:
- **No intermediate pendingChanges map** - inserts go directly to BMT session
- **BMT session manages changes** internally until `Commit()` is called
- **Each Insert() updates the active session** but doesn't write to disk yet

### Step 3: Storage.Flush() Commits BMT Session and Gets Root

**File**: [storage/bmt_storage.go:59-78](../storage/bmt_storage.go#L59-L78)

When `Flush()` is called, the BMT session is committed and the root hash is retrieved.

**What happens**:
```go
func (t *StateDBStorage) Flush() (common.Hash, error) {
    // Commit current BMT state to get the actual root hash
    session, err := t.bmtDB.Commit()
    if err != nil {
        return common.Hash{}, fmt.Errorf("BMT commit failed: %v", err)
    }

    // Get the committed root hash
    newRoot := session.Root()
    t.Root = common.BytesToHash(newRoot[:])

    // Update root history for rollback support
    t.currentSeqNum++
    t.rootHistory = append(t.rootHistory, t.Root)
    t.rootToSeqNum[t.Root] = t.currentSeqNum

    return t.Root, nil
}
```

**How it works**:
1. **Commit BMT session** via `t.bmtDB.Commit()` - returns a new immutable session
2. **Get root hash** from committed session via `session.Root()` (returns GP tree root)
3. **Update root history** to track state progression (for rollback/revert)
4. **Return root hash** to caller (e.g., StateDB)

### Step 4: Beatree Inserts 50,000 Key-Value Pairs

**File**: `bmt/beatree/ops/update.go:Update()`

This is where the magic happens. Let's break down how 50,000 KV pairs get stored in a B+ tree.

#### 4.1: B+ Tree Structure Basics

A B+ tree stores data in **pages** (16KB each in current implementation):
- **Leaf pages**: Store actual key-value pairs
- **Branch pages**: Store separators (keys) + pointers to child pages

**Example tree structure** (simplified):
```
                    Root (Branch Page)
                    ┌─────────────────┐
                    │ Key: 0x5000...  │
                    │ LeftChild: P1   │
                    │ Separators:     │
                    │   [0x5000, P2]  │
                    │   [0xA000, P3]  │
                    └─────────────────┘
                    /        |        \
                   /         |         \
              Page P1    Page P2    Page P3
            (Leaf)      (Leaf)      (Leaf)
         ┌─────────┐ ┌─────────┐ ┌─────────┐
         │ 0x0000  │ │ 0x5000  │ │ 0xA000  │
         │ 0x1000  │ │ 0x6000  │ │ 0xB000  │
         │ 0x2000  │ │ 0x7000  │ │ 0xC000  │
         │ ...     │ │ ...     │ │ ...     │
         └─────────┘ └─────────┘ └─────────┘
```

#### 4.2: Inserting 50,000 Keys (Phase-by-Phase)

**Assumption**: Starting with an **empty tree**, inserting 50,000 keys sorted by hash.

##### Phase A: First Key Inserted

**Changeset entry**: `Key{0x0001, ...} -> Value{100 bytes}`

1. **Root doesn't exist yet** → Create first leaf page (Page 0)
2. **Serialize leaf**:
   ```
   Page 0:
   [Version: 0x01]
   [NumEntries: 1]
   [Entry 0]:
     - Key: 0x0001... (32 bytes)
     - ValueType: Inline (fits in page)
     - Value: [100 bytes]
   ```
3. **Write to BitBox**: `bitbox.WritePage(0, leafData)`
4. **Root page number**: 0

**Tree state**: 1 page, 1 key

##### Phase B: Keys 2-50 Inserted (Still Fit in One Leaf)

**Assumption**: Each key has ~100 byte value → ~132 bytes per entry

**Leaf capacity** (with 16KB pages): 16KB / 132 bytes ≈ **~120 entries per leaf**
**Leaf capacity** (with 4KB pages, for reference): 4KB / 132 bytes ≈ **30 entries per leaf**

For this example, we'll use the 4KB reference numbers for simplicity (see disclaimer at top of document).

After inserting **30 keys**, leaf page 0 would be **full** (in a 4KB page system).

**Page 0 contents**:
```
[Version: 0x01]
[NumEntries: 30]
[Entry 0]: Key 0x0001..., Value [100 bytes]
[Entry 1]: Key 0x0002..., Value [100 bytes]
...
[Entry 29]: Key 0x001E..., Value [100 bytes]
```

##### Phase C: Key 31 Causes First Split

**Changeset entry**: `Key{0x001F, ...} -> Value{100 bytes}`

1. **Leaf page 0 is full** → Split required!
2. **Split algorithm**:
   - Keep first 15 entries in Page 0 (left leaf)
   - Move last 15 entries to new Page 1 (right leaf)
   - Insert new key into appropriate leaf
3. **Create branch page** (Page 2) as new root:
   ```
   Page 2 (Root Branch):
   [Version: 0x01]
   [NumSeparators: 1]
   [LeftmostChild: Page 0]
   [Separator 0]:
     - Key: 0x000F... (first key in Page 1)
     - Child: Page 1
   ```

**Tree state after split**:
```
        Root (Page 2, Branch)
        LeftChild: P0
        Separator: [0x000F, P1]
               /         \
          Page 0        Page 1
          (Leaf)        (Leaf)
       15 entries     16 entries
```

**Total pages**: 3 (2 leaves + 1 branch)

##### Phase D: Keys 32-50,000 Inserted (Tree Grows)

As more keys are inserted:
- **More leaf splits** occur (every ~120 keys with 16KB pages)
- **Branch pages split** when they become full
- **Tree height increases**

**Calculation with 16KB pages**:
- 50,000 keys ÷ 120 keys/leaf ≈ **~417 leaf pages**
- Branch fanout: ~409 separators per 16KB branch page (16,368 bytes ÷ 40 bytes per separator)
- **Level 1 branches**: 417 leaves ÷ 409 ≈ **~2 branch pages**
- **Level 2 (root)**: 2 branches → **1 root branch page**

**Calculation with 4KB pages (for reference)**:
- 50,000 keys ÷ 30 keys/leaf ≈ **~1,667 leaf pages**
- Branch fanout: ~102 separators per 4KB branch page (4,085 bytes ÷ 40 bytes per separator)
- **Level 1 branches**: 1,667 leaves ÷ 102 ≈ **~17 branch pages**
- **Level 2 branches**: 17 branches ÷ 102 ≈ **~1 branch page**
- **Level 3 (root)**: **1 root branch page**

**Final tree structure** (16KB pages):
```
                Root (Page X, Branch, Height=2)
                  ~409 separators max
                /                    \
          Branch 0                Branch 1  (2 pages, Height=1)
          ~209 sep                ~208 sep
         /  |  \                  /  |  \
     Leaf Leaf ... (209 leaves)  Leaf Leaf ... (208 leaves, Height=0)
     120kv 120kv                  120kv 120kv
```

**Total pages** (16KB): ~420 pages (417 leaves + 2 branches + 1 root)
**Total storage** (16KB): 420 pages × 16KB = **~6.7MB**

**Total pages** (4KB reference): ~1,685 pages (1,667 leaves + 17 branches + 1 root)
**Total storage** (4KB reference): 1,685 pages × 4KB = **~6.7MB**

#### 4.3: Handling Large Values (Value Overflow Pages)

**Problem**: What if a value is **> ~15KB** (won't fit in a 16KB leaf page with other entries)?

**Answer**: Use **overflow pages** to store the value separately.

Note: In a 4KB page system, the threshold would be ~3.8KB. With 16KB pages, the threshold is roughly 4× larger.

##### Value Overflow Example: 10KB Value

**Scenario**: Insert `Key{0xAAAA, ...} -> Value{10,240 bytes}`

**Step-by-step process**:

1. **Detect overflow** in `insertIntoLeaf()` (bmt/beatree/ops/update.go):
   ```go
   if len(change.Value) > leaf.MaxLeafValueSize { // MaxLeafValueSize = 3800 bytes
       // Value too large for inline storage → Use overflow
   }
   ```

2. **Allocate overflow pages** from BitBox:
   ```go
   // With 16KB pages:
   numPages := (10240 + 16383) / 16384 = 1 page needed

   // With 4KB pages (reference):
   // numPages := (10240 + 4095) / 4096 = 3 pages needed

   overflowPages := []PageNumber{
       leafStore.Alloc(), // Page 100
   }
   ```

3. **Write value to overflow page(s)**:
   ```go
   // With 16KB pages (10KB value fits in one page):
   leafStore.WritePage(100, value) // 10KB used, 6KB unused

   // With 4KB pages (reference - would need 3 pages):
   // Chunk 0: bytes 0-4095 → Page 100
   // Chunk 1: bytes 4096-8191 → Page 101
   // Chunk 2: bytes 8192-10239 → Page 102
   ```

4. **Create leaf entry with overflow metadata**:
   ```go
   entry := Entry{
       Key: Key{0xAAAA...},
       ValueType: ValueTypeOverflow,        // Flag: value is in overflow pages
       Value: value[0:128],                 // First 128 bytes (prefix for quick checks)
       OverflowPages: [100],                // Page number(s) containing full value (1 page with 16KB)
       OverflowSize: 10240,                 // Total value size in bytes
   }
   ```

5. **Serialize leaf entry** (~200 bytes in leaf page):
   ```
   Leaf Page 50:
   [Entry metadata]:
     - Key: 32 bytes
     - ValueType: 1 byte (0x02 = Overflow)
     - Value: 128 bytes (prefix)
     - OverflowSize: 8 bytes (uint64)
     - NumOverflowPages: 2 bytes (uint16)
     - OverflowPages: 3 × 8 bytes = 24 bytes (page numbers)
   Total: ~195 bytes per overflow entry
   ```

**Physical storage layout** (with 16KB pages):
```
┌─────────────────────────────────────────────────┐
│ Leaf Page 50 (16KB)                             │
│ ┌─────────────────────────────────────────┐     │
│ │ Entry 0 (overflow):                     │     │
│ │   Key: 0xAAAA...                        │     │
│ │   ValueType: Overflow                   │     │
│ │   Value: [first 128 bytes]              │     │
│ │   OverflowPages: [100]                  │ (195 bytes)
│ │   OverflowSize: 10240                   │     │
│ └─────────────────────────────────────────┘     │
│ Entry 1, Entry 2, ... (other entries)           │
└─────────────────────────────────────────────────┘
                  ↓ points to
┌─────────────────────────────────────────────────┐
│ Overflow Page 100 (16KB)                        │
│ [bytes 0-10239 of value]                        │
│ [6144 bytes unused padding]                     │
└─────────────────────────────────────────────────┘
```

**Total storage** (with 16KB pages):
- Leaf entry: ~195 bytes (stored in Leaf Page 50 alongside other entries)
- Overflow pages: 1 page × 16KB = **16KB** (10,240 bytes used + 6,144 bytes padding)
- **Total**: 1 leaf entry slot + 1 overflow page

(With 4KB pages, this would require 3 overflow pages totaling 12KB)

**Reading overflow value**:
```go
// To retrieve the full value:
entry, _ := leaf.Lookup(Key{0xAAAA...})

if entry.ValueType == ValueTypeOverflow {
    // Allocate buffer
    fullValue := make([]byte, entry.OverflowSize)

    // Read chunks from overflow pages
    offset := 0
    for _, pageNum := range entry.OverflowPages {
        pageData, _ := leafStore.ReadPage(pageNum)

        remaining := entry.OverflowSize - offset
        chunkSize := min(4096, remaining)

        copy(fullValue[offset:], pageData[0:chunkSize])
        offset += chunkSize
    }

    // fullValue now contains all 10,240 bytes
}
```

**Storage efficiency**:
- **Without overflow**: Would need to split into multiple leaf entries OR waste space
- **With overflow**: Clean separation of metadata (leaf) and data (overflow pages)
- **Trade-off**: Extra I/O (4 page reads instead of 1), but better space utilization

##### Multiple Overflow Values in Same Leaf

**Scenario**: Leaf has 5 overflow entries

**Leaf page structure**:
```
Leaf Page 50 (4KB):
┌─────────────────────────────────────────┐
│ Entry 0: Key 0xAAAA, Overflow [100-102] │ 195 bytes
│ Entry 1: Key 0xBBBB, Overflow [103-105] │ 195 bytes
│ Entry 2: Key 0xCCCC, Overflow [106-108] │ 195 bytes
│ Entry 3: Key 0xDDDD, Overflow [109-111] │ 195 bytes
│ Entry 4: Key 0xEEEE, Overflow [112-114] │ 195 bytes
└─────────────────────────────────────────┘
Total: 5 × 195 = 975 bytes (fits in 4KB leaf page)

Overflow pages: 5 entries × 3 pages each = 15 overflow pages
```

**Important**: Leaf can hold **~20 overflow entries** (195 bytes each) even though values are huge!

##### Large Value Overflow Chaining (>240KB Values)

**Problem**: The overflow cell in a leaf entry can only store **15 page numbers** ([bmt/beatree/overflow.go:25](../bmt/beatree/overflow.go#L25)). With 16KB pages, 15 pages can hold 15 × 16,380 bytes = ~245KB. What if a value is **larger than 245KB**?

**Solution**: **Overflow page chaining** — The first overflow page stores additional page numbers in its `Pointers` field to reference continuation pages.

##### Example: 100KB Value (Requires 7 Overflow Pages)

With 16KB pages:
- Each overflow page has a 4-byte header (2 bytes for `n_pointers`, 2 bytes for `n_bytes`)
- Each overflow page can store 16,380 bytes of value data ([bmt/beatree/overflow.go:19](../bmt/beatree/overflow.go#L19))
- 100KB ÷ 16,380 bytes/page ≈ **7 pages needed**

**Cell structure** (stored in leaf entry):
```
┌─────────────────────────────────────────────────┐
│ OverflowCell (in Leaf Entry)                    │
│ ┌───────────────────────────────────────────┐   │
│ │ ValueSize: 102400 (100KB)                 │   │
│ │ ValueHash: [32-byte hash]                 │   │
│ │ Pages: [200, 201, 202, 203, 204, 205, 206]│   │ ← All 7 page numbers fit in cell
│ └───────────────────────────────────────────┘   │
└─────────────────────────────────────────────────┘
Total cell size: 8 (size) + 32 (hash) + 7×4 (pages) = 68 bytes
```

**Overflow page layout**:
```
Page 200: [bytes 0-16379]      (16,380 bytes)
Page 201: [bytes 16380-32759]  (16,380 bytes)
Page 202: [bytes 32760-49139]  (16,380 bytes)
Page 203: [bytes 49140-65519]  (16,380 bytes)
Page 204: [bytes 65520-81899]  (16,380 bytes)
Page 205: [bytes 81900-98279]  (16,380 bytes)
Page 206: [bytes 98280-102399] (4,120 bytes used, 12,260 bytes padding)
```

**Reading algorithm** ([bmt/beatree/iterator.go:823-870](../bmt/beatree/iterator.go#L823-L870)):
```go
func readOverflowValue(pages []PageNumber, valueSize uint64) ([]byte, error) {
    value := make([]byte, 0, valueSize)

    // Read all pages listed in the cell
    for _, pageNum := range pages {
        pageData, _ := leafStore.ReadPage(pageNum)
        overflowPage, _ := DecodeOverflowPage(pageData)
        value = append(value, overflowPage.Bytes...)
    }

    // ← For 100KB value, this loop reads all 7 pages and we're done!
    return value, nil
}
```

##### Example: 500KB Value (Requires 31 Overflow Pages + Chaining)

With 16KB pages:
- 500KB ÷ 16,380 bytes/page ≈ **31 pages needed**
- But the cell can only store **15 page numbers**!

**Solution**: Use **pointer chaining** in the first overflow page.

**Cell structure** (only stores first 15 page numbers):
```
┌─────────────────────────────────────────────────┐
│ OverflowCell (in Leaf Entry)                    │
│ ┌───────────────────────────────────────────┐   │
│ │ ValueSize: 512000 (500KB)                 │   │
│ │ ValueHash: [32-byte hash]                 │   │
│ │ Pages: [300, 301, 302, ..., 314]         │   │ ← Only first 15 pages
│ └───────────────────────────────────────────┘   │
└─────────────────────────────────────────────────┘
Total cell size: 8 + 32 + 15×4 = 100 bytes
```

**First overflow page** (Page 300) stores both value data AND pointers to remaining pages:
```
┌─────────────────────────────────────────────────┐
│ Overflow Page 300 (16KB)                        │
│ ┌───────────────────────────────────────────┐   │
│ │ Header (4 bytes):                         │   │
│ │   n_pointers: 16  (stores 16 page nums)  │   │
│ │   n_bytes: 16316  (value data)           │   │
│ ├───────────────────────────────────────────┤   │
│ │ Pointers (16 × 4 = 64 bytes):            │   │
│ │   [315, 316, 317, ..., 330]              │   │ ← Pages 15-30 (remaining 16 pages)
│ ├───────────────────────────────────────────┤   │
│ │ Bytes (16,316 bytes):                     │   │
│ │   [value bytes 0-16315]                   │   │
│ └───────────────────────────────────────────┘   │
└─────────────────────────────────────────────────┘
```

**Subsequent overflow pages** (Pages 301-330) contain only value data:
```
Page 301: [bytes 16316-32695]   (16,380 bytes, no pointers)
Page 302: [bytes 32696-49075]   (16,380 bytes)
...
Page 314: [bytes 229956-246335] (16,380 bytes) ← Last of first 15 pages
Page 315: [bytes 246336-262715] (16,380 bytes) ← First of continuation pages
...
Page 330: [bytes 491900-511999] (20,100 bytes used, padding)
```

**Reading algorithm with chaining** ([bmt/beatree/iterator.go:845-863](../bmt/beatree/iterator.go#L845-L863)):
```go
func readOverflowValue(pages []PageNumber, valueSize uint64) ([]byte, error) {
    value := make([]byte, 0, valueSize)
    var firstOverflowPage *OverflowPage

    // Step 1: Read first 15 pages from cell
    for i, pageNum := range pages {
        pageData, _ := leafStore.ReadPage(pageNum)
        overflowPage, _ := DecodeOverflowPage(pageData)

        if i == 0 {
            firstOverflowPage = overflowPage  // Save first page for pointer following
        }

        value = append(value, overflowPage.Bytes...)
    }

    // Step 2: Check if we need more pages (value not complete yet)
    if len(value) < valueSize && firstOverflowPage != nil && len(firstOverflowPage.Pointers) > 0 {
        // ← This condition triggers for 500KB value!

        // Follow pointers from first overflow page to get remaining pages
        for _, pageNum := range firstOverflowPage.Pointers {
            pageData, _ := leafStore.ReadPage(pageNum)
            overflowPage, _ := DecodeOverflowPage(pageData)
            value = append(value, overflowPage.Bytes...)
        }
    }

    // Verify we got the right amount of data
    if len(value) != valueSize {
        return nil, fmt.Errorf("size mismatch: got %d, expected %d", len(value), valueSize)
    }

    return value, nil
}
```

**Key implementation details**:
1. **Cell limit**: Maximum 15 page numbers ([bmt/beatree/overflow.go:25](../bmt/beatree/overflow.go#L25))
2. **Pointer storage**: First overflow page can store up to 4,095 additional page numbers ([bmt/beatree/overflow.go:22](../bmt/beatree/overflow.go#L22))
3. **Maximum value size**: 512MB ([bmt/beatree/overflow.go:28](../bmt/beatree/overflow.go#L28))
4. **Trade-off**: First overflow page sacrifices 64 bytes of value data to store 16 continuation page numbers

**Total pages calculation** ([bmt/beatree/overflow.go:103-130](../bmt/beatree/overflow.go#L103-L130)):
```go
func TotalNeededPages(valueSize int) int {
    // Calculate pages needed for raw value
    neededPagesRawValue := (valueSize + 16380 - 1) / 16380

    // If <= 15 pages, fits in cell
    if neededPagesRawValue <= 15 {
        return neededPagesRawValue
    }

    // Calculate unused space that can store page numbers
    bytesLeft := (neededPagesRawValue * 16380) - valueSize
    availablePageNumbers := bytesLeft / 4

    // Check if we have enough space for remaining page numbers
    if neededPagesRawValue <= 15 + availablePageNumbers {
        return neededPagesRawValue
    }

    // Calculate additional pages needed for page number storage
    // (This handles extreme cases where even pointer chaining isn't enough)
    ...
}
```

**Example calculations**:
- **100KB value**: 7 pages (fits in cell, no chaining needed)
- **500KB value**: 31 pages (15 in cell + 16 via pointers, no extra pages needed)
- **10MB value**: 626 pages (15 in cell + 611 via pointers, no extra pages needed)
- **100MB value**: 6,258 pages (15 in cell + 6,243 via pointers, may need 1-2 extra pages for pointer overflow)

This chaining mechanism was implemented to fix a critical bug where only the first 15 pages were being read for large values, causing data truncation for values exceeding ~245KB.

#### 4.4: Handling Too Many Keys (Node Overflow / Splitting)

**Problem**: What if a leaf page has **too many entries** (> 16KB capacity)?

**Answer**: **Split the leaf** into two leaves, insert a separator in parent branch.

Note: With 4KB pages, the capacity would be ~4× smaller.

##### Node Overflow Example: Inserting 31st Key

**Scenario** (using 4KB reference for illustration): Leaf Page 0 has 30 entries (4KB full), insert 31st key

Note: With 16KB pages, a leaf would hold ~120 entries before requiring a split.

**Step-by-step process**:

1. **Detect node overflow** in `updateLeafNode()`:
   ```go
   newLeaf := oldLeaf.Clone()
   newLeaf.UpdateEntry(key31, value31) // Insert 31st key

   if newLeaf.EstimateSize() >= leaf.LeafNodeSize { // 16384 bytes in current implementation
       // Node overflow → Split required!
       rightLeaf, separator, err := newLeaf.Split()
   }
   ```

2. **Split algorithm** (bmt/beatree/leaf/node.go:Split()):
   ```go
   func (n *Node) Split() (*Node, Key, error) {
       midpoint := len(n.Entries) / 2 // 31 / 2 = 15

       // Left leaf: entries 0-14 (15 entries)
       leftEntries := n.Entries[0:midpoint]

       // Right leaf: entries 15-30 (16 entries)
       rightEntries := n.Entries[midpoint:]

       // Separator: first key in right leaf
       separator := rightEntries[0].Key

       // Create new right leaf
       rightLeaf := &Node{Entries: rightEntries}

       // Modify left leaf in-place
       n.Entries = leftEntries

       return rightLeaf, separator, nil
   }
   ```

3. **Allocate pages for split leaves**:
   ```go
   // Write left leaf (original entries 0-14)
   leftPageNum := leafStore.Alloc() // Page 0 (reused or new)
   leftData, _ := newLeaf.Serialize()
   leafStore.WritePage(leftPageNum, leftData)

   // Write right leaf (original entries 15-30)
   rightPageNum := leafStore.Alloc() // Page 1 (new allocation)
   rightData, _ := rightLeaf.Serialize()
   leafStore.WritePage(rightPageNum, rightData)
   ```

4. **Update parent branch** to include separator:
   ```go
   // Before split:
   //   Root (Branch): LeftmostChild = Page 0

   // After split:
   //   Root (Branch):
   //     LeftmostChild = Page 0 (left leaf)
   //     Separator[0] = {Key: separator, Child: Page 1 (right leaf)}

   newSeparator := Separator{
       Key: separator,      // First key in right leaf
       Child: rightPageNum, // Page 1
   }

   parentBranch.InsertSeparator(newSeparator)
   ```

**Visual representation**:

**Before split** (30 keys in Page 0):
```
        Root (Branch)
        LeftmostChild: Page 0
             |
        Page 0 (Leaf)
        ┌─────────────────────┐
        │ Entry 0:  Key 0x01  │
        │ Entry 1:  Key 0x02  │
        │ ...                 │
        │ Entry 29: Key 0x1E  │
        └─────────────────────┘
        (30 entries, ~3.9KB - near capacity)
```

**After inserting 31st key** (split occurs):
```
        Root (Branch)
        LeftmostChild: Page 0
        Separator[0]: {Key: 0x0F, Child: Page 1}
             /                    \
        Page 0 (Leaf)         Page 1 (Leaf)
        ┌─────────────┐       ┌─────────────┐
        │ Entry 0-14  │       │ Entry 15-30 │
        │ (15 keys)   │       │ (16 keys)   │
        │ 0x01..0x0E  │       │ 0x0F..0x1F  │
        └─────────────┘       └─────────────┘
        (~2KB)                (~2.1KB)
```

**Routing logic** after split:
- Query for key < 0x0F → Route to Page 0 (left leaf)
- Query for key >= 0x0F → Route to Page 1 (right leaf)

##### Cascading Splits: Branch Overflow

**Scenario** (using 4KB reference): Branch page has 102 separators (full), adding 103rd separator

Note: With 16KB pages, a branch would hold ~409 separators before requiring a split.

**Problem**: Branch page also has a page size limit (16KB in current implementation)!

**Solution**: **Split the branch** into two branches, propagate separator to grandparent.

**Example**:
```
Before branch split (102 separators):
        Root (Branch, Page 2)
        LeftmostChild: Page 3
        Separators[0..101]: 102 separators
             |
        (102 leaf pages below)

After adding 103rd separator (branch split):
        New Root (Branch, Page 200)
        LeftmostChild: Page 2 (left branch)
        Separator[0]: {Key: midpoint, Child: Page 201 (right branch)}
             /                              \
        Page 2 (Branch)                 Page 201 (Branch)
        51 separators                   51 separators
             |                                |
        (51 leaf pages)                 (51 leaf pages)
```

**Cascading splits**: If grandparent is also full, split again! This propagates **up to root**.

**Tree height increases**: When root splits, new root created at higher level.

```
Tree height 0: 1 leaf (root)
Tree height 1: 1 branch (root) + N leaves
Tree height 2: 1 branch (root) + M branches + N leaves
Tree height 3: 1 branch (root) + M branches + P branches + N leaves
```

**Example with 50,000 keys (4KB reference)**:
- Leaf capacity: ~30 keys → 1,667 leaf pages
- Branch capacity: ~102 separators → 17 branch pages (level 1)
- Root capacity: ~102 separators → 1 root branch (level 2)
- **Total height: 3** (root → branch → branch → leaf)

**Example with 50,000 keys (16KB actual)**:
- Leaf capacity: ~120 keys → 417 leaf pages
- Branch capacity: ~409 separators → 2 branch pages (level 1)
- Root capacity: ~409 separators → 1 root branch (level 2)
- **Total height: 2** (root → branch → leaf)

##### Combined Overflow: Value + Node

**Scenario**: Inserting 10KB value causes both value overflow AND node split

**Leaf before insert**: 20 entries (all inline values, ~2.6KB used)

**Insert**: `Key{0xFFFF, ...} -> Value{10KB}` (overflow value)

**Step 1: Value overflow**:
- Allocate 3 overflow pages (Pages 100-102)
- Create overflow entry (~195 bytes)

**Step 2: Check node overflow**:
- New leaf size: 2.6KB + 195 bytes = **2.8KB** (still under 4KB)
- **No split needed!**

**Leaf after insert**: 21 entries (20 inline + 1 overflow, ~2.8KB)

**Physical layout**:
```
Leaf Page 50:
┌─────────────────────────────────────────┐
│ Entry 0:  Key 0x01, Inline [100B]       │
│ Entry 1:  Key 0x02, Inline [100B]       │
│ ...                                     │
│ Entry 19: Key 0x14, Inline [100B]       │
│ Entry 20: Key 0xFFFF, Overflow [100-102]│ ← New overflow entry
└─────────────────────────────────────────┘
Total: ~2.8KB (no split needed)

Overflow Pages 100-102: [10KB value data]
```

**Key insight**: Overflow entries **save space in leaf pages**, allowing more keys per leaf!

**Extreme case**: Leaf with 100% overflow entries
- Leaf capacity: 4096 / 195 ≈ **21 overflow entries**
- Each entry points to arbitrarily large value (MBs if needed)
- **Leaf page doesn't grow**, only overflow pages increase

##### Summary: Two Types of Overflow

| Type | Trigger | Solution | Example |
|------|---------|----------|---------|
| **Value Overflow** | Value > 3.8KB | Allocate overflow pages, store value chunks | 10KB value → 3 overflow pages |
| **Node Overflow** | Node > 4KB | Split node into 2 nodes, update parent | 31 keys → split into 15+16 keys |

**Both can happen independently**:
- Small values, many keys → Node overflow (split)
- Large values, few keys → Value overflow (no split)
- Large values + many keys → Both (overflow pages + split)

**Storage overhead**:
- Value overflow: +1 page per 4KB of value (rounded up)
- Node overflow: +1 page per split (but reduces entries per page)
- **Total pages for 15K keys with mixed sizes**: Depends on value size distribution!

### Step 5: BitBox Allocates and Writes Pages

**File**: `bmt/beatree/bitbox/bitbox.go`

BitBox is the **raw page storage backend**. It doesn't know about B+ trees, just pages.

#### 5.1: Page Allocation

**When Beatree calls** `bitbox.Alloc()`:
1. BitBox checks **free list** for available page numbers
2. If free list empty, **allocate new page** at end of file
3. Return page number (e.g., Page 507)

**Example allocation sequence**:
```
Alloc() -> Page 0   (first leaf)
Alloc() -> Page 1   (second leaf after split)
Alloc() -> Page 2   (first branch/root)
Alloc() -> Page 3   (third leaf)
...
Alloc() -> Page 506 (last branch)
```

#### 5.2: Page Write

**When Beatree calls** `bitbox.WritePage(pageNum, data)`:
1. **Validate**: data is exactly 16KB (in current implementation)
2. **Write to mmap'd region** or file:
   ```go
   offset := pageNum * 16384  // 16KB pages in current implementation
   copy(bitbox.mmap[offset:offset+16384], data)
   ```
3. **Mark page as dirty** (needs fsync)

**Physical layout on disk** (after 50,000 keys inserted):
```
File: nomt.db (16KB pages)
┌─────────────────────────────────────┐
│ Page 0 (Leaf): 120 entries          │  Offset 0
│ Page 1 (Leaf): 120 entries          │  Offset 16384
│ Page 2 (Leaf): 120 entries          │  Offset 32768
│ ...                                 │
│ Page 416 (Leaf): 120 entries        │  Offset 6,815,744
│ Page 417 (Branch): ~200 separators  │  Offset 6,832,128
│ Page 418 (Branch): ~217 separators  │  Offset 6,848,512
│ Page 419 (Root Branch): 2 seps      │  Offset 6,864,896
└─────────────────────────────────────┘
```

**Total file size** (16KB pages): ~420 pages × 16KB = **~6.7MB**
**Total file size** (4KB reference): ~1,685 pages × 4KB = **~6.7MB** (more pages, same data)

#### 5.3: Free List Management

When a page is **deleted** (e.g., old leaf replaced during CoW):
1. BitBox adds page number to **free list**: `freeList = [10, 23, 45, ...]`
2. Next `Alloc()` reuses freed page instead of growing file
3. **Compaction** (optional): Periodically consolidate free pages

**Example**:
```
Before CoW update:
  Page 10: Old leaf with 30 entries

After CoW update:
  Page 10: Added to free list
  Page 507: New leaf with 31 entries (allocated)

Next Alloc():
  Page 10: Reused for new page (instead of Page 508)
```

### Step 6: Copy-on-Write (CoW) Updates

**Scenario**: Block 124 arrives with **1 key-value update** (not 50,000).

**Changeset**: `{Key{0x0001, ...}: &Change{Type: Update, Value: [new 100 bytes]}}`

#### 6.1: Old Implementation (Pre-Phase 1-4)

**Problem**: Updated **every page** along the path:
1. **Find leaf** containing Key 0x0001 (Page 0)
2. **Copy ALL 30 entries** from Page 0 to new Page 507
3. **Update 1 entry** in Page 507
4. **Copy ALL separators** in parent branch (Page 2) to new Page 508
5. **Update 1 child pointer** in Page 508
6. **Repeat for all ancestors** up to root

**Pages written**: ~10 pages (entire path from leaf to root)
**Entries copied**: ~300 entries + ~500 separators = **~800 copy operations**

**Performance**: O(N × log N) for N keys changed

#### 6.2: New Implementation (Phase 1-4 CoW Optimization)

**Solution**: **Point updates** instead of full copies:
1. **Find leaf** containing Key 0x0001 (Page 0)
2. **Clone leaf** (shallow copy of entries array): Page 507
3. **UpdateEntry(Key 0x0001)** using binary search (O(log 30))
4. **Serialize Page 507** and write
5. **Clone parent branch** (Page 2 → Page 508)
6. **UpdateChild(Page 0, Page 507)** in Page 508 (O(log 100))
7. **Repeat up to root**

**Pages written**: ~10 pages (same path)
**Entries copied**: **0 entries** (just clone + point update)
**Separators copied**: **0 separators** (just clone + point update)

**Performance**: O(log N) for N keys in tree

**Key difference**: No more "copy all entries, then modify one" — just "clone + modify one directly"

### Step 7: Computing GP Tree Root Hash

**File**: `bmt/beatree_wrapper.go:Root()`

After inserting 50,000 keys, StateDB needs to compute the **Merkle root** for the block.

#### 7.1: Current Implementation (Pre-Phase 6)

**Problem**: Enumerates **entire tree** to build GP tree:

1. **CollectAllEntries()**: Traverse all 417 leaf pages (16KB) or 1,667 leaf pages (4KB reference), read all 50,000 entries
2. **Sort entries**: O(50,000 × log 50,000) comparison
3. **Build GP tree**:
   ```go
   gpTree := grayPatchey.NewTree()
   for _, entry := range allEntries { // 50,000 iterations
       gpTree.Insert(entry.Key, entry.Value)
   }
   rootHash := gpTree.Root()
   ```
4. **Return hash**: [32]byte

**Performance**: O(N log N) where N = 50,000 keys
**Time**: ~100ms on 10K keys (extrapolating, ~150ms for 15K keys)

#### 7.2: Future Implementation (Phase 6 - Not Yet Implemented)

**Solution**: Store GP hash **per node**, propagate incrementally:

**Leaf nodes**:
```go
type Node struct {
    Entries []Entry
    cachedHash [32]byte  // NEW: GP tree hash for this leaf's entries
}
```

**Branch nodes**:
```go
type Node struct {
    Separators []Separator
    LeftmostChild PageNumber
    cachedHash [32]byte  // NEW: Merkle hash of child hashes
}
```

**How it works**:
1. **Leaf insert**: After updating leaf, compute `cachedHash = GPTree(entries)`
2. **Branch update**: Collect child hashes, compute `cachedHash = Merkle(childHashes)`
3. **Root hash**: Just read `rootPage.cachedHash` (O(1)!)

**Performance**: O(1) for Root() call (vs O(N log N))
**Time**: <1ms (just read root page hash)

**Example**:
```
        Root (Branch)
        cachedHash: 0xABCD...  ← Just return this!
               |
        Computed from:
          hash(child[0].cachedHash || child[1].cachedHash || ...)
```

## Complete Data Flow Diagram

```
┌──────────────────────────────────────────────────────────────┐
│ User: ImportBlock(block with 50,000 KV pairs)                │
└────────────────────────┬─────────────────────────────────────┘
                         ↓
┌──────────────────────────────────────────────────────────────┐
│ StateDB: Extract changeset from block                        │
│   changeset = {                                              │
│     Key{0x0001...}: Change{Type: Insert, Value: [100B]},     │
│     Key{0x0002...}: Change{Type: Insert, Value: [100B]},     │
│     ... (49,998 more)                                        │
│   }                                                          │
└────────────────────────┬─────────────────────────────────────┘
                         ↓
┌──────────────────────────────────────────────────────────────┐
│ NOMT: Store changeset in memory                              │
│   uncommittedChangesets[blockNum] = changeset                │
│   (Not yet written to Beatree)                               │
└────────────────────────┬─────────────────────────────────────┘
                         ↓
┌──────────────────────────────────────────────────────────────┐
│ User: Flush() or Commit()                                    │
└────────────────────────┬─────────────────────────────────────┘
                         ↓
┌──────────────────────────────────────────────────────────────┐
│ BeatreeWrapper: ApplyChangeset()                             │
│   For each KV in changeset:                                  │
│     - Find target leaf using B+ tree traversal               │
│     - Clone leaf (Phase 2 CoW optimization)                  │
│     - UpdateEntry() or DeleteEntry() (binary search)         │
│     - If leaf full → Split into 2 leaves                     │
│     - Write new leaf to BitBox                               │
│     - Clone parent branch (Phase 4 CoW optimization)         │
│     - UpdateChild() parent's child pointer                   │
│     - Propagate up to root                                   │
└────────────────────────┬─────────────────────────────────────┘
                         ↓
┌──────────────────────────────────────────────────────────────┐
│ Beatree: Build B+ tree structure                             │
│   - Allocate ~417 leaf pages (120 entries each, 16KB pages)  │
│   - Allocate ~2 branch pages (409 separators each)           │
│   - Total: 420 pages × 16KB = ~6.7MB                         │
│                                                              │
│   Tree structure (16KB pages):                               │
│          Root (1 branch page)                                │
│         /                    \                               │
│     Branch 0              Branch 1 (2 pages)                 │
│     /  |  \               /  |  \                            │
│   Leaf Leaf ... (200)    Leaf Leaf ... (217 leaves)          │
└────────────────────────┬─────────────────────────────────────┘
                         ↓
┌──────────────────────────────────────────────────────────────┐
│ BitBox: Allocate and write pages                             │
│   - Alloc() returns page numbers: 0, 1, 2, ..., 419          │
│   - WritePage(pageNum, data) writes 16KB to mmap/file        │
│   - Maintains free list for deleted pages                    │
│                                                              │
│   Physical layout on disk (16KB pages):                      │
│   ┌──────────────────────────────┐                           │
│   │ Page 0: Leaf (120 KV pairs)  │ @ offset 0                │
│   │ Page 1: Leaf (120 KV pairs)  │ @ offset 16384            │
│   │ Page 2: Leaf (120 KV pairs)  │ @ offset 32768            │
│   │ ...                           │                          │
│   │ Page 419: Root Branch         │ @ offset 6,864,896       │
│   └──────────────────────────────┘                           │
│   Total file size: ~6.7MB                                    │
└────────────────────────┬─────────────────────────────────────┘
                         ↓
┌──────────────────────────────────────────────────────────────┐
│ User: GetRoot() to compute Merkle root                       │
└────────────────────────┬─────────────────────────────────────┘
                         ↓
┌──────────────────────────────────────────────────────────────┐
│ BeatreeWrapper: Root() (Current - Pre-Phase 6)               │
│   - CollectAllEntries(): Read all 417 leaves                 │
│   - Sort 50,000 entries: O(N log N)                          │
│   - Build GP tree: Insert 50,000 times                       │
│   - Return gpTree.Root() [32]byte                            │
│   Time: ~500ms                                               │
│                                                              │
│ BeatreeWrapper: Root() (Future - Phase 6)                    │
│   - Read root page                                           │
│   - Return rootPage.cachedHash                               │
│   Time: <1ms (500x faster!)                                  │
└──────────────────────────────────────────────────────────────┘
```

## Key Concepts Summary

### 1. **Pages** (16KB chunks in current implementation)
- **Fundamental storage unit** in Beatree/BitBox
- Every node (leaf or branch) fits in **exactly 1 page**
- Overflow values split across **multiple pages**
- Total pages for 50K keys (16KB pages): ~420 pages = **~6.7MB**
- Total pages for 50K keys (4KB reference): ~1,685 pages = **~6.7MB**

### 2. **Leaf Nodes** (Store KV Pairs)
- Contains array of `Entry` structs:
  ```go
  type Entry struct {
      Key [32]byte           // 32 bytes
      ValueType uint8        // 1 byte (Inline or Overflow)
      Value []byte           // Variable length (max ~15KB inline with 16KB pages)
      OverflowPages []uint64 // If value exceeds inline capacity
  }
  ```
- **Capacity** (16KB pages): ~120 entries per page (with 100-byte values)
- **Capacity** (4KB reference): ~30 entries per page (with 100-byte values)
- **Sorted by key** for binary search
- **Phase 6 note**: After incremental hashing is implemented, leaf header grows from 8 bytes to 40 bytes (+32-byte cachedHash). Usable payload shrinks by 32 bytes (16,384 - 40 = 16,344 bytes), reducing capacity by <1 entry (negligible impact on ~119 entry capacity)

### 3. **Branch Nodes** (Store Separators)
- Contains array of `Separator` structs:
  ```go
  type Separator struct {
      Key [32]byte      // 32 bytes
      Child PageNumber  // 8 bytes (uint64)
  }
  ```
- **Capacity** (16KB pages): ~409 separators per page (16,368 bytes ÷ 40 bytes per separator)
- **Capacity** (4KB reference): ~102 separators per page (4,080 bytes ÷ 40 bytes per separator)
- **Routing logic**: Key < Sep[0] → LeftChild, Key >= Sep[i] → Child[i]
- **Phase 6 note**: After incremental hashing is implemented, branch header grows from 16 bytes to 48 bytes (+32-byte cachedHash). Usable payload shrinks by 32 bytes (16,384 - 48 = 16,336 bytes), reducing capacity by ~1 separator (409 → 408, negligible impact)

### 4. **B+ Tree Properties**
- **Height** (16KB pages): log_fanout(N) ≈ log_409(50,000) = **~2 levels** (shallower tree)
- **Height** (4KB reference): log_fanout(N) ≈ log_102(50,000) = **~3 levels**
- **Fan-out** (16KB): ~409 children per branch (very high fan-out = very shallow tree)
- **Fan-out** (4KB reference): ~102 children per branch
- **Leaf linking**: Not currently implemented (could add for range scans)

### 5. **Copy-on-Write (CoW)**
- **Old version**: Copying all entries on every update (O(N × log N))
- **New version** (Phases 1-4): Clone + point update (O(log N))
- **Structural sharing**: Unchanged nodes keep same page number

### 6. **Overflow Pages**
- **Purpose**: Store values that won't fit in leaf page alongside other entries
- **Threshold** (16KB pages): Values > ~15KB typically use overflow
- **Threshold** (4KB reference): Values > ~3.8KB
- **Chunking**: Split value into page-sized chunks (16KB with current implementation)
- **Leaf entry**: Stores prefix + page numbers + size
- **Example** (16KB pages): 10KB value = 1 leaf entry + 1 overflow page
- **Example** (4KB reference): 10KB value = 1 leaf entry + 3 overflow pages

### 7. **GP Tree (Gray-Patchey Tree)**
- **Current**: Rebuilt from scratch on every Root() call (O(N log N))
- **Future** (Phase 6): Incremental hashing with cached hashes (O(1))
- **Purpose**: Compute Merkle root for blockchain state commitment

## Performance Characteristics

### Current Performance (Pre-Phase 6)

| Operation | Complexity | Time (50K keys) |
|-----------|-----------|-----------------|
| **Insert 1 key** | O(log N) tree traversal + O(M) entry copy | ~10ms |
| **Insert 50K keys** | O(K × log N × M) | ~500s (dominated by entry copying) |
| **Lookup 1 key** | O(log N) | <1ms |
| **Root() hash** | O(N log N) full enumeration | ~500ms |
| **CoW update** (Phases 1-4) | O(log N) | ~1ms |

### Target Performance (After Phase 6)

| Operation | Complexity | Time (50K keys) |
|-----------|-----------|-----------------|
| **Insert 1 key** | O(log N) | ~1ms |
| **Insert 50K keys** | O(K × log N) | ~50s (**10x faster**) |
| **Lookup 1 key** | O(log N) | <1ms |
| **Root() hash** | O(1) | <1ms (**500x faster**) |
| **CoW update** | O(log N) | ~1ms |

## Common Gotchas and Clarifications

### Q: Why 16KB pages in current implementation?
**A**: The current implementation uses 16KB pages (see [bmt/io/page_pool.go:11](../bmt/io/page_pool.go#L11)). This is larger than the traditional 4KB OS page size, chosen for:
- **Better fan-out**: More entries/separators per node → shallower trees
- **GP mode optimization**: Blake2b-GP hashing works efficiently with 16KB chunks
- **Fewer I/O operations**: Larger pages mean fewer page reads for same data

**Trade-offs**:
- Larger pages mean more wasted space for small nodes
- Standard OS page size is 4KB, so one Beatree page = 4 OS pages
- Still aligns well with modern SSD erase blocks (often 16KB or larger)

**Benefit**: Higher fan-out, shallower trees, fewer page operations

### Q: Why not use a hash table instead of B+ tree?
**A**: B+ tree provides:
- **Sorted order**: Range scans, efficient GP tree building
- **Merkle proofs**: Can prove inclusion with O(log N) hashes
- **Predictable performance**: O(log N) worst case (hash table can degrade to O(N))

### Q: What's the maximum tree size?
**A**: With 64-bit page numbers:
- Max pages: 2^64 ≈ **18 quintillion pages**
- Max storage (16KB pages): 2^64 × 16KB = **~295 exabytes**
- Max storage (4KB reference): 2^64 × 4KB = **~73 exabytes**
- Practical limit: Disk space + memory for page table

### Q: How does NOMT handle rollbacks?
**A**: Versioned storage with changesets:
1. Each block version has its own changeset
2. To rollback: Discard uncommitted changesets
3. CoW ensures old versions remain intact (no in-place overwrites)

### Q: What happens on crash/power loss?
**A**: BitBox uses:
- **mmap** with `MAP_SHARED`: OS handles flushing dirty pages
- **Explicit fsync**: On Flush() or Commit()
- **Recovery**: Replay WAL (write-ahead log) if implemented

**Current gap**: No WAL yet → uncommitted changes lost on crash

## References

- **B+ Tree spec**: [Wikipedia](https://en.wikipedia.org/wiki/B%2B_tree)
- **GP Tree spec**: [JAM test vectors](https://github.com/w3f/jamtestvectors/blob/main/gray-patchey-tree.ipynb)
- **CoW optimization**: [beatree-cow-optimization.md](beatree-cow-optimization.md)
- **Phase 6 design**: [beatree-incremental-hashing.md](beatree-incremental-hashing.md)
