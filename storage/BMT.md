# BMT Page-Based Storage Specification

## Overview

This document specifies a page-based storage system for Binary Merkle Trees (BMT) that optimizes for:
- **Locality**: 6 tree levels in a single 4K page reduces disk I/O
- **Embedded values**: Values ≤32 bytes stored directly in pages (zero extra lookups)
- **Cache efficiency**: 4K pages align with OS page cache
- **Storage efficiency**: ~33% reduction vs individual node storage

## 1. Page Structure

### 1.1 In-Memory Page Structure

```go
type Page struct {
    // Dirty tracking
    Dirty     bool

    // Content (derived from serialized data)
    IsLeaf    Bitmap   // 126 bits: which nodes are leaves
    IsEmbedded Bitmap  // 126 bits: which leaves have embedded values
    Nodes     [126][32]byte  // Node data (hashes or embedded values)

    // SubRoot is hash of all nodes content (all 6 layers)
    SubRoot   common.Hash

    // Subpages (lazy loaded, nil unless opened)
    SubPages  [126]*Page
}

// Bitmap represents 126 bits (16 bytes)
type Bitmap struct {
    data [16]byte
}

func (b *Bitmap) Get(idx int) bool {
    byteIdx := idx / 8
    bitIdx := idx % 8
    return (b.data[byteIdx] >> bitIdx) & 1 == 1
}

func (b *Bitmap) Set(idx int, val bool) {
    byteIdx := idx / 8
    bitIdx := idx % 8
    if val {
        b.data[byteIdx] |= 1 << bitIdx
    } else {
        b.data[byteIdx] &^= 1 << bitIdx
    }
}
```

### 1.2 Serialized Page Format (exactly 4096 bytes)

```
Offset    Size    Description
------    ----    -----------
0-15      16      IsLeaf Bitmap: 126 bits indicating which nodes are leaves
16-31     16      IsEmbedded Bitmap: 126 bits indicating if leaf has embedded value (vs value hash)
32-4063   4032    Node Hashes: 126 × 32-byte hashes (fixed positions)
4064-4095 32      Reserved
```

**Notes**:
- For branch nodes: `IsLeaf[i]=false`, hash stored in `Nodes[i]`
- For embedded leaves: `IsLeaf[i]=true && IsEmbedded[i]=true`, **value directly stored** (zero-padded to 32 bytes) in `Nodes[i]`
- For large value leaves: `IsLeaf[i]=true && IsEmbedded[i]=false`, hash of 64-byte encoding (with value hash) in `Nodes[i]`
- 64-byte leaf encodings stored in LevelDB with prefix `0x01` only for large leaves
- SubRoot is NOT stored in serialization; computed from Nodes content when needed

### 1.3 Node Layout Within Page

126 nodes arranged in 6 layers (dense binary tree):
- **Layer 0**: Indices 0-1 (2 nodes)
- **Layer 1**: Indices 2-5 (4 nodes)
- **Layer 2**: Indices 6-13 (8 nodes)
- **Layer 3**: Indices 14-29 (16 nodes)
- **Layer 4**: Indices 30-61 (32 nodes)
- **Layer 5**: Indices 62-125 (64 nodes)

**NodeIndex Formula**:
```
NodeIndex(layer, position) = (2^(layer+1) - 2) + position
```

## 2. Page Hierarchy

Pages are stored and retrieved by their **SubRoot hash** (hash of the 4096-byte serialized page).

**Navigation**:
- Each page covers 6 layers of the BMT
- Nodes in layer 5 (indices 62-125) may contain SubRoot hashes of child pages
- Child pages are lazy-loaded from LevelDB using: `[0x06 | SubRoot] → page data`
- No PageID structure needed - pages form a hash-linked tree

## 3. Leaf Storage Strategy

### 3.1 Embedded Leaves (Value ≤ 32 bytes)

**Storage**:
1. Store value directly in `Nodes[nodeIdx]` (zero-padded to 32 bytes)
2. Set `IsLeaf[nodeIdx] = true`
3. Set `IsEmbedded[nodeIdx] = true`

**Retrieval**:
1. Check `IsLeaf[nodeIdx] == true && IsEmbedded[nodeIdx] == true`
2. Read value directly from `Nodes[nodeIdx]` (strip trailing zeros)

**Total: ZERO LevelDB lookups** - value embedded directly in page!

### 3.2 Large Leaves (Value > 32 bytes)

**64-byte Leaf Encoding**:
```
Byte 0:      0b11000000 (top 2 bits = 0b11, lower 6 bits = 0)
Bytes 1-31:  Key (31 bytes)
Bytes 32-63: Hash(value)
```

**Storage**:
1. Compute `leafHash = Hash(64-byte encoding)`
2. Store `leafHash` in `Nodes[nodeIdx]`
3. Set `IsLeaf[nodeIdx] = true`
4. Set `IsEmbedded[nodeIdx] = false`
5. Store 64-byte encoding in LevelDB: `[0x01 | leafHash] → 64-byte encoding`
6. Store actual value: `[0x07 | Hash(value)] → value`

**Retrieval**:
1. Check `IsLeaf[nodeIdx] == true && IsEmbedded[nodeIdx] == false`
2. Lookup encoding: `[0x01 | Nodes[nodeIdx]]`
3. Check byte 0: `0b11` prefix confirms large value
4. Extract `Hash(value)` from bytes 32-63
5. Lookup value: `[0x07 | Hash(value)]`

**Total: 2 LevelDB lookups** (encoding + large value)

### 3.3 Branch Nodes

**Storage**:
- Only hash stored in `Nodes[nodeIdx]`
- Hash computed as per GP 0.6.2 Equation D.3:
  ```
  Branch(left, right) = Hash([left[0] & 0x7F, left[1..31], right[0..31]])
  ```

## 4. Core Algorithms

### 4.1 Page Methods

```go
// Serialize converts Page to 4096-byte representation
func (p *Page) Serialize() []byte {
    data := make([]byte, 4096)

    // Copy bitmaps
    copy(data[0:16], p.IsLeaf.data[:])
    copy(data[16:32], p.IsEmbedded.data[:])

    // Copy all 126 nodes
    for i := 0; i < 126; i++ {
        offset := 32 + i*32
        copy(data[offset:offset+32], p.Nodes[i][:])
    }

    // Reserved section (bytes 4064-4095) remains zero

    return data
}

// Deserialize populates Page from 4096-byte data
func (p *Page) Deserialize(data []byte) error {
    if len(data) != 4096 {
        return fmt.Errorf("invalid page size: %d", len(data))
    }

    // Load bitmaps
    copy(p.IsLeaf.data[:], data[0:16])
    copy(p.IsEmbedded.data[:], data[16:32])

    // Load all 126 nodes
    for i := 0; i < 126; i++ {
        offset := 32 + i*32
        copy(p.Nodes[i][:], data[offset:offset+32])
    }

    // Compute SubRoot as hash of entire page data (using Blake2b-256)
    p.SubRoot = common.BytesToHash(computeHash(data))

    // Clear dirty tracking
    p.Dirty = false

    // SubPages remain nil until opened
    p.SubPages = [126]*Page{}

    return nil
}

// Get retrieves value by key from this page (recursive)
func (p *Page) Get(key [31]byte, bitOffset int, db *leveldb.DB) ([]byte, bool, error) {
    nodeIdx := 0

    // Traverse up to 6 layers within this page
    for layer := 0; layer < 6; layer++ {
        if bitOffset >= 248 {
            break
        }

        nodeData := p.Nodes[nodeIdx]

        // Check if this is a leaf node
        if p.IsLeaf.Get(nodeIdx) {
            if p.IsEmbedded.Get(nodeIdx) {
                // Embedded value - return directly
                return stripTrailingZeros(nodeData[:]), true, nil
            } else {
                // Large value - fetch from LevelDB
                encodingKey := append([]byte{0x01}, nodeData[:]...)
                encoding, err := db.Get(encodingKey, nil)
                if err != nil {
                    return nil, false, err
                }

                valueHash := encoding[32:64]
                valueKey := append([]byte{0x07}, valueHash...)
                value, err := db.Get(valueKey, nil)
                if err != nil {
                    return nil, false, err
                }

                return value, true, nil
            }
        }

        // Check if node is empty (not found)
        if isAllZeros(nodeData[:]) {
            return nil, false, nil
        }

        // Get next bit from key
        bit := getBit(key[:], bitOffset)
        bitOffset++

        // Calculate next node index
        nextLayer := layer + 1
        if nextLayer >= 6 {
            // Reached page boundary - descend to child page
            currentPos := nodeIdx - ((1 << (layer + 1)) - 2)
            childIdx := currentPos*2 + bit

            // The child node in layer 5 contains the SubRoot hash of the child page
            childNodeIdx := NodeIndex(5, childIdx)
            childPageHash := p.Nodes[childNodeIdx]

            // Check if child page exists
            if isAllZeros(childPageHash[:]) {
                return nil, false, nil // Not found
            }

            // Lazy load subpage if needed
            if p.SubPages[childIdx] == nil {
                // Load page by its SubRoot hash
                pageKey := append([]byte{0x06}, childPageHash[:]...)
                childPageData, err := db.Get(pageKey, nil)
                if err != nil {
                    return nil, false, nil // Not found
                }

                childPage := &Page{}
                if err := childPage.Deserialize(childPageData); err != nil {
                    return nil, false, err
                }

                p.SubPages[childIdx] = childPage
            }

            // Recursive call to child page
            return p.SubPages[childIdx].Get(key, bitOffset, db)
        }

        // Navigate within page
        currentPos := nodeIdx - ((1 << (layer + 1)) - 2)
        childPos := currentPos*2 + bit
        nodeIdx = NodeIndex(nextLayer, childPos)
    }

    return nil, false, nil
}

// Insert adds or updates a key-value pair in this page (recursive)
func (p *Page) Insert(key [31]byte, value []byte, bitOffset int, db *leveldb.DB) error {
    nodeIdx := 0

    for layer := 0; layer < 6; layer++ {
        if bitOffset >= 248 {
            break
        }

        nodeData := p.Nodes[nodeIdx]

        // Check if current position is empty
        if isAllZeros(nodeData[:]) {
            // Insert leaf here
            return p.insertLeafAt(nodeIdx, key, value, db)
        }

        // Check if this is a leaf (collision case)
        if p.IsLeaf.Get(nodeIdx) {
            // Need to split this leaf
            return p.splitLeaf(nodeIdx, layer, bitOffset, key, value, db)
        }

        // Branch node - navigate to child
        bit := getBit(key[:], bitOffset)
        bitOffset++

        nextLayer := layer + 1
        if nextLayer >= 6 {
            // Descend to child page
            currentPos := nodeIdx - ((1 << (layer + 1)) - 2)
            childIdx := currentPos*2 + bit

            // The child node in layer 5 contains the SubRoot hash of the child page
            childNodeIdx := NodeIndex(5, childIdx)
            childPageHash := p.Nodes[childNodeIdx]

            // Lazy load or create subpage
            if p.SubPages[childIdx] == nil {
                if isAllZeros(childPageHash[:]) {
                    // Child page doesn't exist - create new one
                    p.SubPages[childIdx] = &Page{}
                } else {
                    // Load existing child page by its SubRoot hash
                    pageKey := append([]byte{0x06}, childPageHash[:]...)
                    childPageData, err := db.Get(pageKey, nil)
                    if err != nil {
                        // Page referenced but not found - create new
                        p.SubPages[childIdx] = &Page{}
                    } else {
                        childPage := &Page{}
                        if err := childPage.Deserialize(childPageData); err != nil {
                            return err
                        }
                        p.SubPages[childIdx] = childPage
                    }
                }
            }

            // Recursive insert
            if err := p.SubPages[childIdx].Insert(key, value, bitOffset, db); err != nil {
                return err
            }

            // Mark page as dirty (child's SubRoot will be updated in Flush)
            p.markDirty()
            return nil
        }

        // Navigate within page
        currentPos := nodeIdx - ((1 << (layer + 1)) - 2)
        childPos := currentPos*2 + bit
        nodeIdx = NodeIndex(nextLayer, childPos)
    }

    return nil
}

// insertLeafAt stores a leaf value at the given node index
func (p *Page) insertLeafAt(nodeIdx int, key [31]byte, value []byte, db *leveldb.DB) error {
    p.IsLeaf.Set(nodeIdx, true)

    if len(value) <= 32 {
        // Embedded value - store directly
        p.IsEmbedded.Set(nodeIdx, true)
        copy(p.Nodes[nodeIdx][:], value)
    } else {
        // Large value - store hash of 64-byte encoding
        p.IsEmbedded.Set(nodeIdx, false)

        encoding := make([]byte, 64)
        encoding[0] = 0xC0
        copy(encoding[1:32], key[:])

        // Hash the value (using Blake2b-256)
        valueHash := computeHash(value)
        copy(encoding[32:64], valueHash[:])

        // Compute encoding hash (using Blake2b-256)
        encodingHash := computeHash(encoding)
        copy(p.Nodes[nodeIdx][:], encodingHash[:])

        // Store in LevelDB
        encodingKey := append([]byte{0x01}, encodingHash[:]...)
        if err := db.Put(encodingKey, encoding, nil); err != nil {
            return err
        }

        valueKey := append([]byte{0x07}, valueHash[:]...)
        if err := db.Put(valueKey, value, nil); err != nil {
            return err
        }
    }

    p.markDirty()
    return nil
}

// markDirty marks the page as needing flush
func (p *Page) markDirty() {
    p.Dirty = true
}

// Delete removes a key from this page (recursive)
func (p *Page) Delete(key [31]byte, bitOffset int, db *leveldb.DB) error {
    nodeIdx := 0

    for layer := 0; layer < 6; layer++ {
        if bitOffset >= 248 {
            break
        }

        nodeData := p.Nodes[nodeIdx]

        // Check if node is empty (key not found)
        if isAllZeros(nodeData[:]) {
            return nil // Nothing to delete
        }

        // Check if this is a leaf
        if p.IsLeaf.Get(nodeIdx) {
            // Found the leaf - delete it
            p.IsLeaf.Set(nodeIdx, false)
            p.IsEmbedded.Set(nodeIdx, false)

            // Zero out the node
            for i := range p.Nodes[nodeIdx] {
                p.Nodes[nodeIdx][i] = 0
            }

            p.markDirty()
            return nil
        }

        // Branch node - navigate to child
        bit := getBit(key[:], bitOffset)
        bitOffset++

        nextLayer := layer + 1
        if nextLayer >= 6 {
            // Descend to child page
            currentPos := nodeIdx - ((1 << (layer + 1)) - 2)
            childIdx := currentPos*2 + bit

            // The child node in layer 5 contains the SubRoot hash of the child page
            childNodeIdx := NodeIndex(5, childIdx)
            childPageHash := p.Nodes[childNodeIdx]

            // Check if child page exists
            if isAllZeros(childPageHash[:]) {
                return nil // Key not found
            }

            // Lazy load subpage if needed
            if p.SubPages[childIdx] == nil {
                pageKey := append([]byte{0x06}, childPageHash[:]...)
                childPageData, err := db.Get(pageKey, nil)
                if err != nil {
                    return nil // Key not found
                }

                childPage := &Page{}
                if err := childPage.Deserialize(childPageData); err != nil {
                    return err
                }

                p.SubPages[childIdx] = childPage
            }

            // Recursive delete
            if err := p.SubPages[childIdx].Delete(key, bitOffset, db); err != nil {
                return err
            }

            // Mark page as dirty (child's SubRoot will be updated in Flush)
            p.markDirty()
            return nil
        }

        // Navigate within page
        currentPos := nodeIdx - ((1 << (layer + 1)) - 2)
        childPos := currentPos*2 + bit
        nodeIdx = NodeIndex(nextLayer, childPos)
    }

    return nil
}

// Flush writes dirty page and all dirty subpages to LevelDB
func (p *Page) Flush(db *leveldb.DB) error {
    // Flush subpages first (bottom-up)
    for i, subPage := range p.SubPages {
        if subPage != nil && subPage.Dirty {
            if err := subPage.Flush(db); err != nil {
                return err
            }

            // Update our node with child's SubRoot
            // Find which node in layer 5 corresponds to subpage index i
            nodeIdx := NodeIndex(5, i)
            copy(p.Nodes[nodeIdx][:], subPage.SubRoot[:])
            p.markDirty()
        }
    }

    // Recompute hashes if dirty
    if p.Dirty {
        p.recomputeHashes()

        // Serialize and compute SubRoot
        data := p.Serialize()
        p.SubRoot = common.BytesToHash(computeHash(data))

        // Save page by its SubRoot hash
        pageKey := append([]byte{0x06}, p.SubRoot[:]...)
        if err := db.Put(pageKey, data, nil); err != nil {
            return err
        }

        p.Dirty = false
    }

    return nil
}

// recomputeHashes recalculates branch hashes for modified nodes
func (p *Page) recomputeHashes() {
    // Process layers bottom-up
    for layer := 5; layer >= 0; layer-- {
        layerSize := 1 << (layer + 1)
        layerOffset := (1 << (layer + 1)) - 2

        for pos := 0; pos < layerSize; pos++ {
            nodeIdx := layerOffset + pos

            // Skip leaves
            if p.IsLeaf.Get(nodeIdx) {
                continue
            }

            // Skip if children are in subpages (layer 5)
            if layer == 5 {
                continue
            }

            // Get child hashes
            leftIdx := NodeIndex(layer+1, pos*2)
            rightIdx := NodeIndex(layer+1, pos*2+1)

            // Compute branch hash
            branchData := make([]byte, 64)
            branchData[0] = p.Nodes[leftIdx][0] & 0x7F
            copy(branchData[1:32], p.Nodes[leftIdx][1:])
            copy(branchData[32:64], p.Nodes[rightIdx][:])

            // Compute branch hash (using Blake2b-256)
            branchHash := computeHash(branchData)
            copy(p.Nodes[nodeIdx][:], branchHash[:])
        }
    }
}
```

### 4.2 MerkleTree Methods

```go
// Get retrieves a value by key (checks staged operations first)
func (t *MerkleTree) Get(key [31]byte) ([]byte, bool, error) {
    keyHash := common.BytesToHash(key[:])

    // Check staged deletes first
    t.stagedMutex.Lock()
    if t.stagedDeletes[keyHash] {
        t.stagedMutex.Unlock()
        return nil, false, nil
    }

    // Check staged inserts
    if value, exists := t.stagedInserts[keyHash]; exists {
        t.stagedMutex.Unlock()
        return value, true, nil
    }
    t.stagedMutex.Unlock()

    // Fall back to page storage
    if t.rootPage == nil {
        return nil, false, nil
    }
    return t.rootPage.Get(key, 0, t.db)
}

// Insert stages a key-value pair (doesn't modify pages until Flush)
func (t *MerkleTree) Insert(key [31]byte, value []byte) error {
    keyHash := common.BytesToHash(key[:])

    t.stagedMutex.Lock()
    defer t.stagedMutex.Unlock()

    // Remove from staged deletes if present
    delete(t.stagedDeletes, keyHash)

    // Add to staged inserts
    t.stagedInserts[keyHash] = value

    return nil
}

// Delete stages a key for deletion (doesn't modify pages until Flush)
func (t *MerkleTree) Delete(key [31]byte) error {
    keyHash := common.BytesToHash(key[:])

    t.stagedMutex.Lock()
    defer t.stagedMutex.Unlock()

    // Remove from staged inserts if present
    delete(t.stagedInserts, keyHash)

    // Add to staged deletes
    t.stagedDeletes[keyHash] = true

    return nil
}

// Flush applies all staged operations to pages and returns the root hash
func (t *MerkleTree) Flush() (common.Hash, error) {
    t.stagedMutex.Lock()
    inserts := t.stagedInserts
    deletes := t.stagedDeletes

    // Clear staged operations
    t.stagedInserts = make(map[common.Hash][]byte)
    t.stagedDeletes = make(map[common.Hash]bool)
    t.stagedMutex.Unlock()

    // Apply deletes
    for keyHash := range deletes {
        var key [31]byte
        copy(key[:], keyHash[:31])

        if t.rootPage == nil {
            continue // Nothing to delete from
        }

        if err := t.rootPage.Delete(key, 0, t.db); err != nil {
            return common.Hash{}, err
        }
    }

    // Apply inserts
    for keyHash, value := range inserts {
        var key [31]byte
        copy(key[:], keyHash[:31])

        if t.rootPage == nil {
            t.rootPage = &Page{}
        }

        if err := t.rootPage.Insert(key, value, 0, t.db); err != nil {
            return common.Hash{}, err
        }
    }

    // Flush dirty pages
    if t.rootPage == nil {
        return common.Hash{}, nil
    }

    if err := t.rootPage.Flush(t.db); err != nil {
        return common.Hash{}, err
    }

    return t.rootPage.SubRoot, nil
}

// InitMerkleTreeFromHash creates a MerkleTree from a root hash (page-based storage)
func InitMerkleTreeFromHash(rootHash common.Hash, db types.JAMStorage) (*MerkleTree, error) {
    if db == nil {
        return nil, fmt.Errorf("database is not initialized")
    }

    tree := &MerkleTree{
        writeBatch:    make(map[common.Hash][]byte),
        stagedInserts: make(map[common.Hash][]byte),
        stagedDeletes: make(map[common.Hash]bool),
        usePages:      true,
        db:            db,
    }

    // Empty tree
    if rootHash == (common.Hash{}) {
        return tree, nil
    }

    // Load root page
    pageKey := append([]byte{0x06}, rootHash[:]...)
    data, err := db.Get(pageKey, nil)
    if err != nil {
        return nil, fmt.Errorf("root page not found: %w", err)
    }

    tree.rootPage = &Page{}
    if err := tree.rootPage.Deserialize(data); err != nil {
        return nil, err
    }

    return tree, nil
}

// GetRoot returns the current root hash (flushes if dirty)
func (t *MerkleTree) GetRoot() common.Hash {
    // Flush if there are staged operations or dirty pages
    if len(t.stagedInserts) > 0 || len(t.stagedDeletes) > 0 || (t.rootPage != nil && t.rootPage.Dirty) {
        root, err := t.Flush()
        if err != nil {
            // Log error but return best effort
            return normalizeKey32(make([]byte, 32))
        }
        return normalizeKey32(root[:])
    }

    if t.rootPage == nil {
        return normalizeKey32(make([]byte, 32))
    }
    return normalizeKey32(t.rootPage.SubRoot[:])
}

// Helper: NodeIndex calculates the index in Nodes array
func NodeIndex(layer, position int) int {
    if layer < 0 || layer >= 6 {
        return -1
    }
    return (1 << (layer + 1)) - 2 + position
}

// Helper: Get bit at offset from byte array
func getBit(data []byte, offset int) int {
    byteIdx := offset / 8
    bitIdx := 7 - (offset % 8)
    if (data[byteIdx] >> bitIdx) & 1 == 1 {
        return 1
    }
    return 0
}

// Helper: Check if all bytes are zero
func isAllZeros(data []byte) bool {
    for _, b := range data {
        if b != 0 {
            return false
        }
    }
    return true
}

// Helper: Strip trailing zero bytes
func stripTrailingZeros(data []byte) []byte {
    for i := len(data) - 1; i >= 0; i-- {
        if data[i] != 0 {
            return data[:i+1]
        }
    }
    return []byte{}
}
```


### 5.2 Proof Generation

```go
// GetProof generates a Merkle proof for a key
func (t *MerkleTree) GetProof(key [31]byte) ([]common.Hash, error) {
    if t.rootPage == nil {
        return nil, fmt.Errorf("empty tree")
    }

    proof := []common.Hash{}
    return t.rootPage.GetProof(key, 0, &proof)
}

// GetProof on Page collects sibling hashes along the path
func (p *Page) GetProof(key [31]byte, bitOffset int, proof *[]common.Hash) ([]common.Hash, error) {
    // Same traversal as Get(), but collect sibling hashes at each level
    // Implementation details similar to Get() but appending siblings to proof
    // ... (implementation left as exercise)
    return *proof, nil
}
```

## 6. LevelDB Key Prefixes

```
0x06  Page data (SubRoot → serialized 4096-byte page)
0x07  Large leaf values (Hash(value) → value)
0x01  Large leaf encodings (Hash(encoding) → 64-byte encoding)
```

Historical prefixes (from node-based storage, may coexist):
```
0x00  Regular data
0x02  Values
0x03  Branches
```

## 7. Caching Strategy

### 7.1 Page Cache

**LRU cache** for recently accessed pages:
- Default size: 1000 pages (~4 MB)
- Key: `SubRoot` hash
- Value: Deserialized `Page`

**Eviction**: Least recently used

**Hit ratio optimization**:
- Root page pinned in MerkleTree.rootPage
- Frequently accessed pages naturally stay cached
- 6 levels per page = ~6× fewer cache entries vs node-based

### 7.2 Prefetching

For sequential key access:
```go
// PrefetchPath asynchronously loads pages along the path for a key
func (t *MerkleTree) PrefetchPath(key [31]byte) {
    // Could traverse and load child pages in background
    // Implementation would use goroutines to async load from LevelDB
}
```

## 8. Performance Characteristics

### 8.1 Read Performance

| Operation | Page-Based | Node-Based | Improvement |
|-----------|-----------|------------|-------------|
| Embedded leaf (≤32B) | 1 page load | 1-3 node loads | ~2× fewer I/O |
| Large leaf (>32B) | 1 page + 2 lookups | 3-5 node loads | ~2× fewer I/O |
| Proof generation | ⌈h/6⌉ pages | h nodes | ~6× fewer I/O |

Where `h` = tree height

**Note**: Embedded leaves require ZERO additional lookups - value read directly from page

### 8.2 Write Performance

**Overhead**: Modified node requires rewriting entire 4K page

**Mitigation**:
- Batch updates before flushing to pages
- Keep recent writes in node-based storage
- Periodic compaction to page format

### 8.3 Space Efficiency

**Fixed overhead per page**: 64 bytes (two 16-byte bitmaps + 32-byte reserved)

**Savings**:
- 126 nodes in 4096 bytes = 32.5 bytes/node vs 64+ bytes in separate storage
- IsEmbedded bitmap allows distinguishing leaf types without fetching encoding
- All 64-byte encodings stored once in LevelDB (no duplication)

## 9. Edge Cases and Error Handling

### 9.1 Empty Nodes

**Definition**: Nodes with all-zero hash and no key

**Handling**:
- DO NOT mark as leaves in IsLeaf bitmap
- Treat as non-existent during traversal
- Return NotFound when encountered

### 9.2 Malformed Pages

**Detection**:
- Page size != 4096 bytes
- IsLeaf or IsEmbedded bitmap has bits set beyond index 125
- Reserved section contains non-zero data (future format versions)

**Handling**: Return error, do not cache

### 9.3 Missing Pages

**Scenario**: SubRoot hash referenced but not in LevelDB

**Handling**:
- Return error if required for traversal
- Indicates corrupted tree or missing data
- Cannot continue traversal without the page

### 9.4 Hash Mismatches

**Verification**: Parent node hash should match child page's SubRoot

**On mismatch**:
- Log corruption warning
- Invalidate cache entry
- Return error - tree integrity compromised
