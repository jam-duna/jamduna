# Beatree Incremental Hash Propagation Design (Phase 6)

## Problem Statement

**Current bottleneck**: After Phases 1-4 eliminated O(N × log N) entry copying, the remaining bottleneck is `beatreeWrapper.Root()` which enumerates the entire tree on every block import.

### Verified Bottleneck

**File**: `bmt/beatree_wrapper.go:184-225`

```go
func (bw *beatreeWrapper) Root() [32]byte {
    if !bw.rootDirty {
        return bw.cachedRoot  // Cache helps when root not dirty, but...
    }

    // BOTTLENECK #1: Enumerates EVERY key/value in the tree
    allEntries, err := bw.store.CollectAllEntries()

    // BOTTLENECK #2: Sorts ALL entries (O(N log N) even with sort.Slice)
    sort.Slice(allEntries, func(i, j int) bool {
        return allEntries[i].Key.Compare(allEntries[j].Key) < 0
    })

    // BOTTLENECK #3: Rebuilds entire GP tree from scratch
    gpTree := grayPatchey.NewTree()
    for _, entry := range allEntries {
        gpTree.Insert(entry.Key, entry.Value)
    }

    bw.cachedRoot = gpTree.Root()
    bw.rootDirty = false
    return bw.cachedRoot
}
```

### Performance Impact

- **Current**: Every block import with changes = O(N log N) full enumeration
- **CoW Phases 1-4**: Reduced node copying to O(log N), but full enumeration still dominates
- **Test results**: TestTracesRecompiler = 22.1s (no improvement from CoW on small workloads)
- **Root cause**: Small test workloads (hundreds of keys) spend most time in Root() enumeration, not node copying

### Why CoW Alone Isn't Enough

On small trees (N < 1000):
- O(log N) path updates: ~8-10 nodes touched
- O(N) full enumeration: ~100-1000 entries processed
- Full enumeration still dominates runtime

**Target**: Eliminate full enumeration entirely with incremental hash propagation.

## Solution Architecture

### Core Idea: Store GP Hash Per Node

Instead of rebuilding the GP tree from scratch, compute and cache the GP hash for each B-tree node. When a node changes, recompute only its hash and propagate up the tree.

### Data Structure Changes

#### 1. Leaf Node Format (bmt/beatree/leaf/node.go)

**Add hash field**:
```go
type Node struct {
    Entries []Entry

    // NEW: Cached GP hash for this leaf's entries
    // Zero value ([32]byte{}) means hash needs recomputation
    cachedHash [32]byte
}
```

**Serialization format change**:
```
Current: [NumEntries:4 bytes][PageType:1 byte][Reserved:3 bytes][Entries...]
Phase 6:  [NumEntries:4 bytes][PageType:1 byte][Reserved:3 bytes][CachedHash:32 bytes][Entries...]
```

**Header size impact**:
- **Current header**: 8 bytes (4 numEntries + 1 pageType + 3 reserved)
- **Phase 6 header**: 40 bytes (4 numEntries + 1 pageType + 3 reserved + 32 cachedHash)
- **Header size increase**: +32 bytes per leaf node
- **Usable payload shrinks**: From (16384 - 8) = 16,376 bytes to (16384 - 40) = 16,344 bytes
- **Exact capacity impact** (assuming 137 bytes per entry for 100-byte inline values: 32-byte key + 1-byte type + 4-byte length + 100-byte value):
  - Current format: 16,376 ÷ 137 ≈ **119.5 entries** → ~119 entries per leaf
  - Phase 6 format: 16,344 ÷ 137 ≈ **119.3 entries** → ~119 entries per leaf
  - **Reduction**: 0.2 entries per leaf (rounds to same capacity in practice)

#### 2. Branch Node Format (bmt/beatree/branch/node.go)

**Add hash field**:
```go
type Node struct {
    Separators    []Separator
    LeftmostChild allocator.PageNumber

    // NEW: Cached GP hash combining all child hashes
    // Zero value means hash needs recomputation
    cachedHash [32]byte
}
```

**Serialization format change**:
```
Current: [NumSeparators:4 bytes][PageType:1 byte][Reserved:3 bytes][LeftmostChild:8 bytes][Separators...]
Phase 6:  [NumSeparators:4 bytes][PageType:1 byte][Reserved:3 bytes][LeftmostChild:8 bytes][CachedHash:32 bytes][Separators...]
```

**Header size impact**:
- **Current header**: 16 bytes (4 numSeparators + 1 pageType + 3 reserved + 8 leftmostChild)
- **Phase 6 header**: 48 bytes (4 numSeparators + 1 pageType + 3 reserved + 8 leftmostChild + 32 cachedHash)
- **Header size increase**: +32 bytes per branch node
- **Usable payload shrinks**: From (16384 - 16) = 16,368 bytes to (16384 - 48) = 16,336 bytes
- **Exact capacity impact** (assuming 40 bytes per separator: 32-byte key + 8-byte child pointer):
  - Current format: 16,368 ÷ 40 = **409.2 separators** → ~409 separators per branch
  - Phase 6 format: 16,336 ÷ 40 = **408.4 separators** → ~408 separators per branch
  - **Reduction**: 0.8 separators per branch (rounds to 1 separator reduction in practice)

### Hash Computation Strategy

#### Leaf Node Hash (GP Tree for Leaf Entries)

```go
// ComputeHash computes the GP tree hash for this leaf's entries.
// Entries must be sorted (already guaranteed by Insert/UpdateEntry).
func (n *Node) ComputeHash() [32]byte {
    if len(n.Entries) == 0 {
        return [32]byte{} // Empty leaf = zero hash
    }

    // Build GP tree from sorted entries
    gpTree := grayPatchey.NewTree()
    for _, entry := range n.Entries {
        gpTree.Insert(entry.Key, entry.Value)
    }

    return gpTree.Root()
}

// Hash returns the cached hash, computing if necessary.
func (n *Node) Hash() [32]byte {
    if n.cachedHash == ([32]byte{}) {
        n.cachedHash = n.ComputeHash()
    }
    return n.cachedHash
}
```

#### Branch Node Hash (Merkle Hash of Child Hashes)

**Strategy**: Branch hash is NOT a GP tree. Instead, it's a Merkle hash combining:
1. All child page hashes (in separator order)
2. Separator keys (to ensure uniqueness)

```go
// ComputeHash computes the Merkle hash for this branch's children.
// Algorithm:
//   1. Collect child hashes in left-to-right order (leftmost + separators)
//   2. For each separator: concatenate separatorKey || childHash
//   3. Single Blake2b hash over all concatenated data
func (n *Node) ComputeHash(childHashes map[allocator.PageNumber][32]byte) [32]byte {
    if len(n.Separators) == 0 {
        // Single child branch (edge case during splits)
        if leftHash, ok := childHashes[n.LeftmostChild]; ok {
            return leftHash
        }
        return [32]byte{}
    }

    // Collect hashes in separator order
    var combinedData []byte

    // Leftmost child contribution
    if leftHash, ok := childHashes[n.LeftmostChild]; ok {
        combinedData = append(combinedData, leftHash[:]...)
    }

    // Separator contributions: hash(key || childHash)
    for _, sep := range n.Separators {
        if childHash, ok := childHashes[sep.Child]; ok {
            combinedData = append(combinedData, sep.Key[:]...)
            combinedData = append(combinedData, childHash[:]...)
        }
    }

    // Final hash
    return blake2b.Sum256(combinedData)
}

// Hash returns the cached hash.
// NOTE: Caller must ensure child hashes are already computed.
func (n *Node) Hash() [32]byte {
    return n.cachedHash
}
```

**Design choice rationale**:
- Leaf nodes use GP hash (matches existing semantics for leaf data)
- Branch nodes use Merkle hash (efficient, no need to re-enumerate children)
- Root hash = hash of root node (whether leaf or branch)

### Hash Propagation During Updates

#### updateLeafNode (bmt/beatree/ops/update.go)

**After Phase 2 optimization**, add hash computation:

```go
func updateLeafNode(...) (allocator.PageNumber, *splitInfo, []allocator.PageNumber, error) {
    oldLeaf, err := leaf.DeserializeLeafNode(pageData)
    newLeaf := oldLeaf.Clone()

    // Apply changes using in-place updates
    for key, change := range changeset {
        // ... (existing UpdateEntry/DeleteEntry logic)
    }

    // NEW: Compute hash for modified leaf
    newLeaf.cachedHash = newLeaf.ComputeHash()

    // Check for split
    if newLeaf.EstimateSize() >= leaf.LeafNodeSize {
        rightLeaf, separator, err := newLeaf.Split()

        // NEW: Compute hashes for both split leaves
        newLeaf.cachedHash = newLeaf.ComputeHash()
        rightLeaf.cachedHash = rightLeaf.ComputeHash()

        // Store both leaves with hashes
        leftPageNum, _ := storeLeafNode(newLeaf, leafStore)
        rightPageNum, _ := storeLeafNode(rightLeaf, leafStore)

        return leftPageNum, &splitInfo{...}, ...
    }

    // Store modified leaf with hash
    newPageNum := leafStore.Alloc()
    leafData, _ := newLeaf.Serialize() // Includes cachedHash
    leafStore.WritePage(newPageNum, leafData)

    return newPageNum, nil, []allocator.PageNumber{newPageNum}, nil
}
```

#### updateBranchNode (bmt/beatree/ops/update.go)

**After Phase 4 optimization**, add hash propagation:

```go
func updateBranchNode(...) (allocator.PageNumber, *splitInfo, []allocator.PageNumber, error) {
    oldBranch, err := branch.DeserializeBranchNode(pageData)
    newBranch := oldBranch.Clone()

    // Collect child hashes during update
    // OPTIMIZATION: Build hash map incrementally to avoid redundant reads
    childHashes := make(map[allocator.PageNumber][32]byte)

    // Update ONLY the affected children
    for childPage, childChanges := range childChangesets {
        newChildPage, childSplit, childAllocated, err := updateNode(childPage, childChanges, leafStore)

        // NEW: Read child's hash after update
        // NOTE: This is a fresh write, so we MUST read to get the new hash
        childData, _ := leafStore.ReadPage(newChildPage)
        if isLeafPage(childData) {
            childNode, _ := leaf.DeserializeLeafNode(childData)
            childHashes[newChildPage] = childNode.cachedHash // Use cached hash from serialized node
        } else {
            childNode, _ := branch.DeserializeBranchNode(childData)
            childHashes[newChildPage] = childNode.cachedHash
        }

        // Point update
        found, _ := newBranch.UpdateChild(childPage, newChildPage)

        if childSplit != nil {
            newSep := branch.Separator{Key: childSplit.SeparatorKey, Child: childSplit.RightPage}
            newBranch.InsertSeparator(newSep)

            // NEW: Also get split child's hash (already in cache from updateNode return)
            splitChildData, _ := leafStore.ReadPage(childSplit.RightPage)
            if isLeafPage(splitChildData) {
                splitNode, _ := leaf.DeserializeLeafNode(splitChildData)
                childHashes[childSplit.RightPage] = splitNode.cachedHash
            }
        }
    }

    // NEW: Collect hashes for UNCHANGED children by reading from old serialized nodes
    // OPTIMIZATION: Unchanged children already have valid cachedHash in their serialized form
    // We read the page ONCE to get the hash, not to recompute it
    //
    // FUTURE OPTIMIZATION (Phase 6.7): The updateNode calls above already return freshly
    // allocated page numbers for modified children. We could extend updateNode to also
    // return the computed hash, avoiding the ReadPage calls at lines 264 and 281.
    // This would eliminate all reads for MODIFIED children, only reading UNCHANGED siblings.
    // For now, we accept the O(K) extra reads where K = number of modified children.
    for _, sep := range oldBranch.Separators {
        if _, ok := childHashes[sep.Child]; !ok {
            // Child not updated - read its cached hash from disk
            childData, _ := leafStore.ReadPage(sep.Child)
            if isLeafPage(childData) {
                childNode, _ := leaf.DeserializeLeafNode(childData)
                childHashes[sep.Child] = childNode.cachedHash // Reuse cached hash
            } else {
                childNode, _ := branch.DeserializeBranchNode(childData)
                childHashes[sep.Child] = childNode.cachedHash // Reuse cached hash
            }
        }
    }
    // Also handle leftmost child
    if _, ok := childHashes[oldBranch.LeftmostChild]; !ok {
        childData, _ := leafStore.ReadPage(oldBranch.LeftmostChild)
        if isLeafPage(childData) {
            childNode, _ := leaf.DeserializeLeafNode(childData)
            childHashes[oldBranch.LeftmostChild] = childNode.cachedHash
        } else {
            childNode, _ := branch.DeserializeBranchNode(childData)
            childHashes[oldBranch.LeftmostChild] = childNode.cachedHash
        }
    }

    // NEW: Compute hash for modified branch using collected child hashes
    newBranch.cachedHash = newBranch.ComputeHash(childHashes)

    // Handle split if needed
    if newBranch.EstimateSize() >= branch.BranchNodeSize {
        rightBranch, separator, err := splitBranch(newBranch)

        // NEW: Compute hashes for both split branches
        // childHashes already contains all necessary child hashes
        newBranch.cachedHash = newBranch.ComputeHash(childHashes)
        rightBranch.cachedHash = rightBranch.ComputeHash(childHashes)

        // Store both branches with hashes
        // ...
    }

    // Store modified branch with hash
    newPageNum := leafStore.Alloc()
    branchData, _ := newBranch.Serialize() // Includes cachedHash
    leafStore.WritePage(newPageNum, branchData)

    return newPageNum, nil, []allocator.PageNumber{newPageNum}, nil
}
```

**Key optimization**: Unchanged children have their hashes read from disk (one ReadPage per child), but **we don't recompute** their hashes. We just extract `cachedHash` from the deserialized node. This is still O(S) reads for S separators, but each read is cheap (just deserialization, no hash recomputation).

**Future optimization** (Phase 6.7, optional): In-memory cache of child hashes to avoid even the ReadPage calls for unchanged children. This would reduce branch hash computation to O(K) where K = number of changed children, instead of O(S) where S = total separators.

#### beatreeWrapper.Root() Optimization

**After Phase 6**, Root() becomes O(1):

```go
func (bw *beatreeWrapper) Root() [32]byte {
    if !bw.rootDirty {
        return bw.cachedRoot
    }

    // NEW: Just read root page's hash (no enumeration!)
    rootPageData, err := bw.store.ReadPage(bw.store.RootPage())
    if err != nil {
        return [32]byte{} // Empty tree
    }

    // Determine if root is leaf or branch
    if isLeafPage(rootPageData) {
        rootLeaf, _ := leaf.DeserializeLeafNode(rootPageData)
        bw.cachedRoot = rootLeaf.Hash()
    } else {
        rootBranch, _ := branch.DeserializeBranchNode(rootPageData)
        bw.cachedRoot = rootBranch.Hash()
    }

    bw.rootDirty = false
    return bw.cachedRoot
}
```

**Performance**: O(1) read + deserialize, vs O(N log N) full enumeration.

**Root dirty flag contract**:
- `rootDirty` is set to `true` whenever `ops.Update()` completes successfully
- This happens in the wrapper's `Insert()` or `Delete()` methods after calling `updateNode()`
- `rootDirty` is set to `false` when `Root()` reads and caches the hash
- **Critical assumption**: After `ops.Update()` finishes, the root page on disk has a valid `cachedHash`
  - This is guaranteed because `updateBranchNode()` and `updateLeafNode()` compute hashes before serializing
  - The wrapper can trust the on-disk hash immediately after the update path completes
- **Alternative design** (Phase 6.7): Instead of `rootDirty` flag, extend `updateNode()` to return the root hash directly, eliminating even the root page read

### Complexity Analysis

#### Before Phase 6 (Current)
- Block import with K changes:
  - Update path: O(K × log N) with CoW optimizations
  - Root computation: **O(N log N)** enumeration + sort + GP rebuild
  - **Total: O(N log N)** (dominated by Root)
- TestTracesRecompiler (small workload, ~100-1000 keys): **22.1s**
- Root() on 10K-key tree: **~100ms+** (full enumeration)

#### After Phase 6 (Target)
- Block import with K changes:
  - Update path: O(K × log N) with hash recomputation
  - Hash propagation: O(S) reads per branch (S = separators, to collect child hashes)
  - Root computation: **O(1)** read root page hash
  - **Total: O(K × log N × S)** where S = avg separators per branch
- TestTracesRecompiler: **Likely < 10s** (2-3x speedup, not 10x)
  - Small trees have less Root() overhead to eliminate
  - Hash propagation overhead (O(S) reads per branch) partially offsets savings
  - PVM execution time still dominates on small workloads
- Root() on 10K-key tree: **< 1ms** (just read root page hash)
- **Large trees (10K+ keys)**: **10-50x speedup** as Root() enumeration dominates

**Realistic performance expectations**:
- **Small test workloads** (TestTracesRecompiler, < 1K keys): **2-5x speedup**
  - Root() is called infrequently, so eliminating it has modest impact
  - Hash propagation overhead is non-trivial on small trees
- **Medium workloads** (1K-10K keys): **5-10x speedup**
  - Root() enumeration becomes significant bottleneck
  - Hash propagation overhead amortizes better
- **Large workloads** (10K+ keys, production scale): **10-50x speedup**
  - Root() enumeration completely dominates runtime
  - Target: Insert + Flush + Root cycle competitive with JavaJAM (~12ms mean)

## Implementation Plan

### Phase 6.1: Serialization Format

**Files to modify**:
- `bmt/beatree/leaf/node.go` - Add cachedHash field, update Serialize/Deserialize
- `bmt/beatree/branch/node.go` - Add cachedHash field, update Serialize/Deserialize
- `bmt/beatree/leaf/node_test.go` - Test serialization round-trip with hash
- `bmt/beatree/branch/node_test.go` - Test serialization round-trip with hash

**Acceptance criteria**:
- Serialize/Deserialize preserves cachedHash field
- Serialization format matches Phase 6 spec (header + cachedHash + entries/separators)
- All existing serialization tests pass

### Phase 6.2: Hash Computation

**Files to modify**:
- `bmt/beatree/leaf/node.go` - Add ComputeHash, Hash methods
- `bmt/beatree/branch/node.go` - Add ComputeHash, Hash methods
- `bmt/beatree/leaf/node_test.go` - Test hash computation correctness
- `bmt/beatree/branch/node_test.go` - Test hash computation correctness

**Acceptance criteria**:
- Leaf hash matches GP tree root for same entries
- Branch hash is deterministic and unique per structure
- Hash recomputation is idempotent
- All hash computation tests pass

### Phase 6.3: Hash Propagation in updateLeafNode

**Files to modify**:
- `bmt/beatree/ops/update.go` - Add hash computation to updateLeafNode

**Acceptance criteria**:
- Newly created leaves have valid cachedHash
- Split leaves both have valid hashes
- Hashes are correctly serialized to storage
- All leaf update tests pass

### Phase 6.4: Hash Propagation in updateBranchNode

**Files to modify**:
- `bmt/beatree/ops/update.go` - Add hash computation to updateBranchNode
- Add helper: `collectChildHashes(branch, leafStore)`

**Acceptance criteria**:
- Branch nodes compute hash from child hashes
- Unchanged children reuse cached hashes from disk (read page, extract hash, no recomputation)
- Split branches both have valid hashes
- All branch update tests pass

**Note on "no re-read needed"**: Phase 6.4 still reads unchanged child pages to extract their cached hashes. This is O(S) reads per branch but cheap (deserialization only, no hash recomputation). A future optimization (Phase 6.7, deferred) could add an in-memory hash cache to eliminate even these reads.

### Phase 6.5: Root() Optimization

**Files to modify**:
- `bmt/beatree_wrapper.go` - Rewrite Root() to use cached hashes

**Acceptance criteria**:
- Root() returns hash without enumerating tree
- Root hash matches old implementation (verified in tests)
- Performance: Root() is O(1), not O(N log N)

### Phase 6.6: Integration & Performance Validation

**Testing**:
- Add benchmark: Measure Root() time on 10K-key tree
- Run TestTracesRecompiler, verify 2-5x speedup (realistic for small workload)
- Compare against JavaJAM performance (~12ms mean for Insert+Flush+Root on production workloads)
- Add correctness test: Verify new Root() == old Root() for same data

**Acceptance criteria**:
- Insert+Flush+Root cycle competitive with JavaJAM **on production-scale workloads** (10K+ keys)
- Root() completes in <1ms (vs current ~100ms+ on large trees)
- TestTracesRecompiler shows **2-5x speedup** (small workload, modest gains)
- All correctness tests pass
- No regressions in existing functionality

**Reality check**: TestTracesRecompiler may only show 2-3x speedup due to:
- Small workload (~100-1000 keys) has minimal Root() overhead to eliminate
- Hash propagation overhead (O(S) reads per branch) partially offsets savings
- PVM execution time dominates on small traces
- **Target metric is production workloads**, not small test traces

## Testing Strategy

### Unit Tests

#### Serialization Round-Trip
```go
func TestLeafSerializeWithHash(t *testing.T) {
    leaf := leaf.NewNode()
    leaf.Insert(entry1)
    leaf.Insert(entry2)
    leaf.cachedHash = leaf.ComputeHash()

    // Serialize
    data, err := leaf.Serialize()

    // Deserialize
    leaf2, err := leaf.DeserializeLeafNode(data)

    // Verify hash preserved
    if leaf2.cachedHash != leaf.cachedHash {
        t.Error("cachedHash not preserved after round-trip")
    }
}
```

#### Hash Computation Correctness
```go
func TestLeafHashMatchesGPTree(t *testing.T) {
    leaf := leaf.NewNode()
    leaf.Insert(entry1)
    leaf.Insert(entry2)

    // Compute hash via new method
    leafHash := leaf.ComputeHash()

    // Compute hash via old method (GP tree)
    gpTree := grayPatchey.NewTree()
    for _, entry := range leaf.Entries {
        gpTree.Insert(entry.Key, entry.Value)
    }
    oldHash := gpTree.Root()

    // Must match
    if leafHash != oldHash {
        t.Errorf("leaf hash mismatch: got %x, want %x", leafHash, oldHash)
    }
}
```

#### Hash Propagation
```go
func TestUpdateLeafSetsHash(t *testing.T) {
    // Create tree with 1 leaf
    tree := createTestTree()

    // Update 1 key
    tree.Update(key, value)

    // Read leaf from storage
    leafData, _ := tree.store.ReadPage(leafPage)
    leaf, _ := leaf.DeserializeLeafNode(leafData)

    // Verify hash is non-zero and correct
    if leaf.cachedHash == ([32]byte{}) {
        t.Error("cachedHash not set after update")
    }

    expectedHash := leaf.ComputeHash()
    if leaf.cachedHash != expectedHash {
        t.Error("cachedHash incorrect")
    }
}
```

### Integration Tests

#### Correctness Validation
```go
func TestIncrementalHashMatchesFullEnumeration(t *testing.T) {
    tree := createTestTree()

    // Apply 100 random updates
    for i := 0; i < 100; i++ {
        tree.Update(randomKey(), randomValue())
    }

    // Compute root via new method (cached hash)
    newRoot := tree.Root()

    // Compute root via old method (full enumeration)
    allEntries, _ := tree.store.CollectAllEntries()
    sort.Slice(allEntries, ...)
    gpTree := grayPatchey.NewTree()
    for _, entry := range allEntries {
        gpTree.Insert(entry.Key, entry.Value)
    }
    oldRoot := gpTree.Root()

    // Must match
    if newRoot != oldRoot {
        t.Errorf("root hash mismatch: incremental=%x, enumeration=%x", newRoot, oldRoot)
    }
}
```

#### Performance Benchmark
```go
func BenchmarkRootWithIncrementalHashing(b *testing.B) {
    tree := buildLargeTree(10000)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        root := tree.Root() // Should be O(1)
        _ = root
    }
}

func BenchmarkRootWithFullEnumeration(b *testing.B) {
    tree := buildLargeTree(10000)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // Old method
        allEntries, _ := tree.store.CollectAllEntries()
        sort.Slice(allEntries, ...)
        gpTree := grayPatchey.NewTree()
        for _, entry := range allEntries {
            gpTree.Insert(entry.Key, entry.Value)
        }
        root := gpTree.Root()
        _ = root
    }
}
```

Expected results:
- Incremental: ~0.1ms per Root() call
- Full enumeration: ~100ms per Root() call
- **1000x speedup**

### Regression Tests

- All existing beatree tests must pass
- Fuzz testing with traces (TestTracesRecompiler)
- Correctness verification against JavaJAM

## Risk Mitigation

### Risks

1. **Serialization format bugs**: Hash field corruption, deserialization errors
2. **Hash computation bugs**: Incorrect GP tree computation, Merkle hash bugs
3. **Performance regression**: Hash computation overhead exceeds enumeration savings

### Mitigation

1. **Extensive unit tests**: Test every serialization edge case
2. **Dual-path validation**: Run both old and new methods in parallel, assert equality
3. **Benchmark early**: Measure hash computation cost before full integration
4. **Feature flag**: Allow runtime disable for quick rollback if bugs discovered
5. **Incremental rollout**: Deploy to test environments first, monitor correctness

## Success Metrics

- **Performance**: Insert+Flush+Root cycle < 15ms on production workloads (vs current ~22s for TestTracesRecompiler)
- **Root() latency**: < 1ms (vs current ~100ms+ on 10K-key trees)
- **Correctness**: All tests pass, hashes match old implementation
- **Code quality**: No increase in cyclomatic complexity, clear documentation
- **Maintainability**: Serialization format versioning, migration plan documented

## Non-Goals (Deferred)

- **Parallel hash computation**: Single-threaded is sufficient for now
- **Hash caching across versions**: Each version recomputes hashes
- **Compression of hash fields**: 32 bytes is acceptable overhead
- **Alternative hash algorithms**: BLAKE2b is sufficient

## References

- [CoW Optimization Design](beatree-cow-optimization.md) - Phases 1-5
- [Current Root() bottleneck](../bmt/beatree_wrapper.go#L184-L225)
- [Gray-Patchey Tree spec](https://github.com/w3f/jamtestvectors/blob/main/gray-patchey-tree.ipynb)
- JavaJAM performance baseline: 12.6ms mean for Insert+Flush+Root cycle
- Actual API flow: [NOMT Architecture Deep Dive](nomt-architecture-deep-dive.md)
