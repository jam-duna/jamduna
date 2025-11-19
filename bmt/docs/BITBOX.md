# BitBox: Hash Table Page Storage

## What is BitBox?

BitBox is a **persistent hash table** designed for fast, random-access storage of 16KB pages indexed by `PageId`. It serves as the **primary page storage layer** in the NOMT (No Merkle Tree) architecture.

> **Implementation Notes**:
> - WAL recovery now clears dirty pages and reinitializes WAL builder (prevents duplicate replay)
> - Probe limit removed (was capped at 10,000, now probes up to full table size)
> - PageId verification enforced on retrieval (each bucket stores 32-byte PageId + 16KB data)

### Role in NOMT

In the NOMT system, BitBox sits between the **Beatree** (B-epsilon tree) and the **disk**:

```
Beatree (in-memory tree)
    ↓
BitBox (hash table on disk)
    ↓
Operating System (file I/O)
```

**When BitBox is used:**
- **Page Writes**: When Beatree needs to persist a page to disk, it calls `InsertPage(pageId, pageData)`
- **Page Reads**: When Beatree needs to load a page from disk, it calls `RetrievePage(pageId)`
- **Sync Operations**: When Beatree commits a transaction, it calls `Sync()` to ensure durability

**Key Characteristics:**
- **Fixed-size pages**: All pages are exactly 16,384 bytes (16KB)
- **Content-addressed**: Pages are indexed by their `PageId` (a 32-byte identifier derived from the tree path)
- **Write-Ahead Logging (WAL)**: Provides crash recovery without sacrificing write performance
- **Triangular probing**: Efficiently handles hash collisions with predictable performance

### Performance Profile

**Strengths:**
- **O(1) average-case lookups**: Hash table provides constant-time access to pages
- **Fast writes**: WAL allows writes to complete without waiting for fsync to the main hash table
- **Predictable collision handling**: Triangular probing ensures bounded probe sequences
- **Crash recovery**: WAL replay restores uncommitted changes after crashes

**Trade-offs:**
- **Fixed capacity**: Hash table size is set at creation time (typically 1 million+ buckets)
- **Space overhead**: Requires metadata (1 byte per bucket) plus hash table file space
- **Probe cost on collisions**: Full tables degrade to O(n) in worst case (mitigated by load factor limits)

**Typical Performance (estimated, hardware-dependent):**
- **InsertPage**: In-memory only (appends to WAL builder, no I/O)
- **RetrievePage** (cache miss): One disk read (16,416 bytes) + PageId verification
- **Sync**: Writes WAL + N dirty pages + metadata, then fsyncs (depends on dirty page count and disk speed)
- **Recovery**: Reads entire WAL + replays entries (linear in WAL size)

---

## Hash Table Design

### Metadata: 2-bit Per-Bucket State

BitBox uses a compact metadata map where each bucket stores **1 byte**:

- **Empty (0x00)**: Bucket never used
- **Tombstone (0x7F)**: Bucket was used but page deleted
- **Full (0x80 | hash[57:64])**: Bucket occupied, with top 7 bits of hash for filtering

This metadata enables **fast probe filtering** without reading page data from disk.

### Triangular Probing

When a hash collision occurs, BitBox uses **triangular probing**:

```
probe[0] = hash % num_buckets
probe[1] = (hash + 1) % num_buckets
probe[2] = (hash + 3) % num_buckets
probe[3] = (hash + 6) % num_buckets
probe[4] = (hash + 10) % num_buckets
...
probe[n] = (hash + n*(n+1)/2) % num_buckets
```

**Why triangular probing?**
- **Good distribution**: Visits buckets in a well-distributed pattern
- **Cache-friendly**: Probes tend to cluster initially, then spread out
- **Deterministic**: Same hash always probes same sequence

### Write-Ahead Log (WAL)

The WAL enables crash recovery by logging operations before applying them to the hash table file.

**Transaction Flow:**
1. **InsertPage**: Append `UPDATE` entry to in-memory WAL builder + store in dirty page cache
2. **Sync**: Write WAL to disk (with `END` marker) → flush dirty pages to `ht` → fsync → truncate WAL
3. **Close** (without Sync): Write WAL to disk (with `END` marker) → close files (dirty pages NOT flushed)
4. **Recovery**: On next Open, replay WAL entries → write pages to `ht` → truncate WAL

**WAL Format:**
```
[START marker (0x01)]
[CLEAR bucket_index | UPDATE page_id page_data]*
[END marker (0x02)]
```

**Entry Types:**
- `START (0x01)`: **Required** at beginning of WAL blob (validated on read)
- `END (0x02)`: Marks end of transaction
- `CLEAR (0x03)`: Mark bucket as tombstone (8-byte bucket index)
- `UPDATE (0x04)`: Write page (32-byte PageId + 16KB page data)

**Key Properties:**
- WAL is always written and fsynced before updating `ht` file (durability)
- Sync truncates WAL to zero length after successful write to `ht`
- Missing `START` marker returns validation error (detects corrupted/truncated WAL)
- Recovery clears dirty pages and reinitializes WAL builder after replay

---

## Go Code Organization

### Package Structure

```
bmt/bitbox/
├── bucket.go          # Bucket index types and atomic operations
├── hash.go            # xxhash3-based page hashing
├── meta_map.go        # Compact 1-byte-per-bucket metadata
├── probe.go           # Triangular probing implementation
├── allocate.go        # Bucket allocation with collision handling
├── ht_file.go         # Hash table file layout (meta pages + data pages)
├── db.go              # Main DB API (Open, InsertPage, RetrievePage, Sync, Close)
└── wal/
    ├── constants.go   # WAL format tag constants
    ├── entry.go       # WalEntry struct and types
    ├── blob_builder.go # In-memory WAL builder
    └── blob_reader.go  # Sequential WAL parser
```

### Key Entry Points

#### Opening a Database

```go
db, err := bitbox.Open(dirPath, numBuckets)
```

**What it does:**
1. Creates or opens `ht` (hash table file) and `wal` (WAL file) in `dirPath`
2. Loads metadata map from first N pages of `ht`
3. If `wal` is non-empty, performs recovery:
   - Validates WAL starts with `START` marker
   - Replays `CLEAR` and `UPDATE` entries
   - Writes recovered pages to `ht` with PageId prefix
   - Truncates `wal` to zero (recovery complete)
4. Initializes WAL builder with `START` marker for new transaction
5. Returns ready-to-use `DB` instance

**Files created:**
- `{dirPath}/ht`: Hash table (meta pages + buckets with PageId+data)
- `{dirPath}/wal`: Write-ahead log (requires `START` marker)

#### Inserting a Page

```go
err := db.InsertPage(pageId, pageData)
```

**What it does:**
1. Hash `pageId` to get bucket index
2. Use triangular probing to find empty bucket or matching `pageId`
3. Allocate bucket and mark metadata as "full"
4. Write `UPDATE` entry to in-memory WAL builder
5. Store page in dirty page cache (in memory)

**Note**: Does NOT write to disk immediately. Call `Sync()` for durability.

#### Retrieving a Page

```go
pageData, err := db.RetrievePage(pageId)
```

**What it does:**
1. Hash `pageId` to get starting bucket
2. Probe until finding:
   - **Empty bucket**: Page not found (error)
   - **Matching hash**: Possible hit
3. Check dirty pages cache first (uncommitted writes)
4. Read page from `ht` at bucket's data page offset
5. Verify stored `pageId` matches requested `pageId` (rejects hash collisions)
   - Each bucket stores: `[32-byte PageId][16KB page data]`
   - If PageId mismatch, returns error (hash collision detected)

#### Syncing to Disk

```go
err := db.Sync()
```

**What it does:**
1. Add `END` marker to WAL builder
2. Write WAL builder contents to `wal` file and fsync
3. Write dirty pages (with PageId prefix) to `ht` file at their bucket offsets
4. Write updated metadata map to `ht` file (first N pages)
5. Fsync `ht` file
6. Truncate `wal` file to zero length (transaction complete)
7. Clear dirty pages cache and bucket→PageId mapping
8. Reinitialize WAL builder with fresh `START` marker

**Result**: All changes are now durable on disk. WAL is empty and ready for next transaction.

#### Closing the Database

```go
err := db.Close()
```

**What it does:**
1. If dirty pages exist:
   - Add `END` marker to WAL builder
   - Write WAL to disk and fsync (enables crash recovery)
   - Does NOT write to `ht` file or clear dirty pages
2. Close `ht` and `wal` file handles

**Important**: `Close()` does NOT call `Sync()`. Uncommitted changes remain in WAL for recovery on next `Open()`.

---

## Major Functions and Algorithms

### Hash Function: `HashPageId()`

**Purpose**: Convert a `PageId` (32-byte tree path) to a 64-bit hash.

**Implementation**: Uses **xxhash3** with a seed for domain separation.

**Why xxhash3?**
- Extremely fast (5-10 GB/s)
- Excellent distribution (low collision rate)
- Industry-standard, well-tested

### Bucket Allocation: `AllocateBucket()`

**Purpose**: Find an empty bucket for a given `pageId`.

**Algorithm:**
1. Compute `hash = HashPageId(pageId, seed)`
2. Create `ProbeSequence` starting at `hash % num_buckets`
3. For each probe position:
   - If bucket is **empty**: Allocate it, mark metadata as full, return
   - If bucket is **tombstone**: Continue probing
   - If bucket hash **doesn't match**: Continue probing (fast metadata check)
   - If bucket hash **matches**: Possible hit, caller must verify
4. If all buckets probed: Return `BucketExhaustion` error

**Probe Limit**: Maximum `num_buckets` iterations to prevent infinite loops.

### Metadata Map: `MetaMap`

**Purpose**: Store 1 byte per bucket to enable fast probe filtering.

**Key Methods:**
- `SetEmpty(bucket)`: Mark bucket as unused (0x00)
- `SetTombstone(bucket)`: Mark bucket as deleted (0x7F)
- `SetFull(bucket, hash)`: Mark bucket as occupied with hash hint (0x80 | top 7 bits)
- `HintNotMatch(bucket, hash) bool`: Fast check if bucket can't contain this hash

**Storage**: Byte slice rounded up to multiple of 4096 (page size).

### Probe Sequence: `ProbeSequence.Next()`

**Purpose**: Iterate through bucket candidates using triangular probing.

**Algorithm:**
```go
bucket = (bucket + step) % num_buckets
step++
```

**Returns:**
- `ProbeResultEmpty`: Bucket is empty (can allocate here)
- `ProbeResultTombstone`: Bucket is deleted (skip it)
- `ProbeResultPossibleHit`: Bucket hash matches (need to verify page data)
- `ProbeResultExhausted`: All buckets probed, table full

### WAL Recovery: `DB.recover()`

**Purpose**: Replay uncommitted changes from WAL after a crash.

**Algorithm:**
1. Read entire `wal` file into memory
2. If empty, return (nothing to recover)
3. Parse WAL entries using `WalBlobReader` (validates `START` marker)
4. For each entry:
   - **CLEAR**: Mark bucket as tombstone in metadata
   - **UPDATE**: Allocate bucket, write [PageId + page data] to `ht`, track in `bucketToId`
5. Write updated metadata to `ht` file
6. Fsync `ht` file
7. Truncate `wal` file (recovery complete)
8. Clear dirty pages and `bucketToId` mapping
9. Reinitialize WAL builder with fresh `START` marker

**Crash Safety**: WAL is always fsynced before writing to `ht`, so partial writes are safe.

### Hash Table File Layout: `HTOffsets`

**Purpose**: Calculate byte offsets for metadata and buckets in `ht` file.

**Layout:**
```
[Meta Page 0 (16KB)] [Meta Page 1 (16KB)] ... [Meta Page N-1]
[Bucket 0 (16,416 bytes)] [Bucket 1 (16,416 bytes)] ...
```

**Metadata Pages**: Store the `MetaMap` (1 byte per bucket, rounded to 16KB pages).

**Buckets**: Each bucket stores `[32-byte PageId][16KB page data]` = 16,416 bytes total.

**Offset Calculation:**
```go
meta_pages = ceil(num_buckets / 16384)
data_start_offset = meta_pages * 16384
bucket_offset = data_start_offset + (bucket_index * 16416)  // PageId (32) + Page (16384)
```

**File Size**: `(meta_pages * 16384) + (num_buckets * 16416)` bytes

---

## Test Organization

### Phase 4A: Core Structures (19 tests)

**bucket_test.go** - Bucket index validation
- `TestBucketIsValid`: Sentinel value (`^0`) is invalid
- `TestSharedMaybeBucketIndex`: Atomic get/set with `nil` representation

**hash_test.go** - Hash function properties
- `TestHashPageId`: Produces uint64 output
- `TestHashConsistency`: Same input → same hash
- `TestHashDistribution`: Different paths → different hashes

**meta_map_test.go** - Metadata storage
- `TestMetaMapEmpty`: 0x00 detection
- `TestMetaMapTombstone`: 0x7F marking
- `TestMetaMapFull`: 0x80|hash[57:64] storage
- `TestMetaMapHints`: `HintNotMatch` filters non-matching hashes
- `TestMetaMapPageSlice`: Alignment to 16KB boundaries
- `TestMetaMapFromBytes`: Deserialization from disk
- `TestMetaMapLen`: Bucket count tracking

**probe_test.go** - Triangular probing
- `TestTriangularProbing`: Sequence formula (hash, +1, +3, +6, +10, ...)
- `TestProbeNoCollision`: First probe finds empty
- `TestProbeWithCollision`: Skips occupied buckets
- `TestProbeWithTombstone`: Skips tombstones
- `TestProbeWithMultipleCollisions`: Long probe chains
- `TestProbePossibleHit`: Returns matching hash hints
- `TestProbeWraparound`: Modulo wrapping at table end

### Phase 4B: WAL Format (11 tests)

**blob_test.go** - WAL builder and reader
- `TestBlobBuilder`: Append entries to in-memory buffer
- `TestBlobReader`: Parse entries sequentially
- `TestBlobRoundtrip`: Write then read produces identical entries
- `TestBlobReaderMissingStartMarker`: Validation error for corrupt WAL
- `TestBlobReaderMissingStartMarkerUpdate`: Validation for update entries
- `TestBlobReaderEmptyBlob`: Empty WAL is valid (no validation)

**wal_test.go** - Entry encoding/decoding
- `TestWalClear`: CLEAR tag (0x03) + 8-byte bucket index
- `TestWalUpdate`: UPDATE tag (0x04) + 32-byte pageId + 16KB data
- `TestWalStartEnd`: START (0x01) and END (0x02) markers
- `TestWalInvalidPageIdSize`: Panic on wrong pageId size
- `TestWalInvalidPageDataSize`: Panic on wrong page size

### Phase 4C: DB Integration (11 tests)

**db_test.go** - Core DB operations
- `TestDbOpen`: Create/open files, initialize metadata
- `TestDbInsertPage`: Write page to dirty cache + WAL
- `TestDbRetrievePage`: Read from dirty cache or disk
- `TestDbSync`: Flush WAL → hash table → truncate WAL

**allocate_test.go** - Bucket allocation
- `TestBucketAllocation`: Find empty bucket via probing
- `TestBucketExhaustion`: Error when table too full
- `TestFreeBucket`: Mark bucket as tombstone

**recovery_test.go** - WAL recovery
- `TestWalRecovery`: Replay WAL after crash (uncommitted inserts)
- `TestWalRecoveryEmpty`: Handle empty WAL (fresh database)
- `TestWalRecoveryPartial`: Handle mixed synced + unsynced pages

**concurrent_test.go** - Concurrency
- `TestConcurrentInserts`: Parallel inserts with different `PageId`s

### Test Strategy

**Unit Tests** (4A, 4B): Test individual components in isolation
- No file I/O
- Fast (<1ms per test)
- Focus on algorithm correctness

**Integration Tests** (4C): Test full system with real files
- Create temporary directories
- Write to disk
- Test crash recovery (close without sync, reopen)

**Concurrency Tests**: Ensure thread-safety
- Multiple goroutines inserting simultaneously
- Validates mutex protection around shared state

---

## Key Design Decisions

### Why 1-byte metadata instead of bitmaps?

**Trade-off**: 1 byte per bucket uses more space than 2 bits, but enables **hash hint filtering**.

**Benefit**: The top 7 bits of the hash stored in metadata allow rejecting 127/128 of non-matching buckets without reading page data from disk.

**Space cost**: For 1M buckets, metadata is 1MB (vs 250KB for 2-bit). This is negligible compared to 16GB of page data.

### Why triangular probing instead of linear or quadratic?

**vs Linear Probing**:
- Linear probing causes clustering (consecutive occupied buckets)
- Triangular probing spreads out faster, reducing clustering

**vs Quadratic Probing**:
- Quadratic can miss buckets (not all positions visited)
- Triangular guarantees visiting all positions (for prime table sizes)

**Empirical performance**: Triangular probing performs well in practice with good hash functions.

### Why WAL instead of direct writes?

**Problem**: Syncing the hash table file after every insert would be slow (10-50ms per fsync).

**Solution**: Batch writes in WAL (fast append-only), sync WAL quickly, then batch-update hash table.

**Benefit**: Amortizes fsync cost over many inserts (100+ inserts → 1 sync).

### Why fixed-size buckets?

**Simplification**: Every bucket holds exactly one 16KB page, enabling simple offset calculation.

**Trade-off**: Cannot store variable-size data, but NOMT only needs fixed-size pages.

---

## Usage Example

```go
package main

import (
    "github.com/colorfulnotion/jam/bmt/bitbox"
    "github.com/colorfulnotion/jam/bmt/core"
)

func main() {
    // Open database with 1 million buckets
    db, err := bitbox.Open("/tmp/mydb", 1_000_000)
    if err != nil {
        panic(err)
    }
    defer db.Close()

    // Create a page ID (tree path: root → child 5 → child 10)
    pageId, _ := core.NewPageId([]byte{5, 10})

    // Create page data (16KB)
    pageData := make([]byte, 16384)
    copy(pageData, []byte("Hello, BitBox!"))

    // Insert page (in-memory, not yet durable)
    err = db.InsertPage(*pageId, pageData)
    if err != nil {
        panic(err)
    }

    // Make it durable (write to disk)
    err = db.Sync()
    if err != nil {
        panic(err)
    }

    // Retrieve page
    retrieved, err := db.RetrievePage(*pageId)
    if err != nil {
        panic(err)
    }

    println(string(retrieved[:14])) // "Hello, BitBox!"
}
```

---

## Future Optimizations

**Potential Improvements:**

1. **Async I/O**: Use `io_uring` (Linux) or `kqueue` (BSD) for zero-copy reads
2. **mmap for metadata**: Memory-map the metadata pages for faster access
3. **Batch WAL writes**: Buffer multiple inserts before writing to WAL
4. **Bloom filters**: Add per-bucket bloom filter to reduce false probe hits
5. **Compression**: Compress pages before writing (trade CPU for disk space)
6. **Sharding**: Split hash table into multiple files for parallel access

**Current Status**: BitBox is production-ready for NOMT's use case without these optimizations.

---

## Summary

**BitBox** is a specialized hash table optimized for:
- Fast random-access page storage
- Crash recovery via WAL
- Predictable performance with bounded probe sequences
- Simple, testable design

It provides the **storage foundation** for NOMT's Beatree, handling all persistent page I/O with O(1) average-case lookups and strong durability guarantees.
