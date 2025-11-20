# Beatree Copy-on-Write Optimization Design

## Problem Statement

Current implementation copies **all entries** in every touched node during updates, causing O(N × log N) work per block instead of O(log N).

### Current Bottleneck (Verified)

**File**: `bmt/beatree/ops/update.go`

```go
// Lines 186-191: COPIES ALL ENTRIES even for 1-key change
func updateLeafNode(...) {
    newLeaf := leaf.NewNode()
    for _, entry := range oldLeaf.Entries {  // ← COPIES EVERYTHING
        if err := newLeaf.Insert(entry); err != nil {
            return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to copy entry: %w", err)
        }
    }
    // Then apply the actual change...
}

// Lines 257-262: COPIES ALL SEPARATORS even for 1-child change
func updateBranchNode(...) {
    newBranch := branch.NewNode(oldBranch.LeftmostChild)
    for _, sep := range oldBranch.Separators {  // ← COPIES EVERYTHING
        if err := newBranch.Insert(sep); err != nil {
            return allocator.InvalidPageNumber, nil, nil, fmt.Errorf("failed to copy separator: %w", err)
        }
    }
    // Then update the changed child...
}
```

### Performance Impact

- **Current**: Every block import = O(N × log N) entry copies
- **Target**: Every block import = O(log N) node allocations (only modified path)
- **Expected improvement**: 10-100x speedup on block imports

## Solution Architecture

### 1. Leaf Node In-Place Updates

#### New API Methods (add to `bmt/beatree/leaf/node.go`)

```go
// UpdateEntry modifies an existing entry in-place or inserts if not found
// Returns: (found bool, needsSplit bool, error)
func (n *Node) UpdateEntry(key beatree.Key, value []byte) (bool, bool, error)

// DeleteEntry removes an entry in-place
// Returns: (found bool, error)
func (n *Node) DeleteEntry(key beatree.Key) (bool, error)

// Clone creates a shallow copy of the node (reuses entry slices)
func (n *Node) Clone() *Node
```

#### Implementation Strategy

1. **Binary search** to find entry position
2. **In-place modification** if key exists
3. **Insertion** only if key is new
4. **Split detection** based on size estimation after change

#### Updated `updateLeafNode` Flow

```go
func updateLeafNode(pageNum, pageData, changeset, leafStore) {
    oldLeaf := leaf.DeserializeLeafNode(pageData)

    // Shallow clone - reuses entry slices initially
    newLeaf := oldLeaf.Clone()

    // Apply changes with in-place updates (no full copy)
    for key, change := range changeset {
        if change.IsDelete() {
            found, err := newLeaf.DeleteEntry(key)
            // handle err
        } else {
            found, needsSplit, err := newLeaf.UpdateEntry(key, change.Value)
            // handle err
        }
    }

    // Only now check if split is needed
    if newLeaf.NeedsSplit() {
        return handleLeafSplit(newLeaf, leafStore)
    }

    // Single allocation for modified leaf
    return storeLeafNode(newLeaf, leafStore)
}
```

### 2. Branch Node Point Updates

#### New API Methods (add to `bmt/beatree/branch/node.go`)

```go
// UpdateChild modifies a single child pointer in-place
// Returns: (found bool, error)
func (n *Node) UpdateChild(oldChild, newChild allocator.PageNumber) (bool, error)

// InsertSeparator adds a separator (for splits) without copying all separators
func (n *Node) InsertSeparator(sep Separator) error

// Clone creates a shallow copy of the node (reuses separator slices)
func (n *Node) Clone() *Node
```

#### Updated `updateBranchNode` Flow

```go
func updateBranchNode(pageNum, pageData, changeset, leafStore) {
    oldBranch := branch.DeserializeBranchNode(pageData)

    // Shallow clone - reuses separator slices
    newBranch := oldBranch.Clone()

    // Group changes by child (no change here)
    childChangesets := groupChangesetsByChild(changeset, oldBranch)

    // Update ONLY the affected children
    for childPage, childChanges := range childChangesets {
        newChildPage, childSplit, _, err := updateNode(childPage, childChanges, leafStore)

        // Point update - modify only this child pointer
        found, err := newBranch.UpdateChild(childPage, newChildPage)

        if childSplit != nil {
            // Insert separator without copying existing ones
            err := newBranch.InsertSeparator(childSplit.Separator)
        }
    }

    if newBranch.NeedsSplit() {
        return handleBranchSplit(newBranch, leafStore)
    }

    return storeBranchNode(newBranch, leafStore)
}
```

### 3. Copy-on-Write Path Optimization

#### Structural Sharing

- **Unchanged nodes**: Keep existing page numbers (no allocation)
- **Modified path**: Allocate new pages ONLY for nodes that changed
- **Siblings**: Reference existing pages (structural sharing)

#### Example: Update 1 key in 10,000-key tree

**Before** (current):
```
Root (copy all 100 separators)
├─ Branch A (copy all 100 separators)
│  ├─ Branch A1 (copy all 100 separators)
│  │  └─ Leaf (copy all 50 entries) ← Target leaf with 1 changed key
│  └─ Branch A2 (UNCHANGED - but still copied!)
└─ Branch B (UNCHANGED - but still copied!)
```
**Allocations**: 5 nodes × ~100 entries/separators = ~500 copy operations

**After** (optimized):
```
Root' (new, modified 1 child pointer)
├─ Branch A' (new, modified 1 child pointer)
│  ├─ Branch A1' (new, modified 1 child pointer)
│  │  └─ Leaf' (new, modified 1 entry)
│  └─ Branch A2 (REUSED - same page number)
└─ Branch B (REUSED - same page number)
```
**Allocations**: 4 nodes with point modifications = O(log N) operations

### 4. Hash Propagation (Future Work)

**Current issue**: `Root()` rebuilds entire GP tree from scratch

**Future optimization** (not in initial implementation):
- Store GP hash with each node
- Update hashes incrementally along modified path
- Requires node metadata expansion

**Deferred because**:
- Current tests show Root() is called rarely
- Sync() optimization already eliminated it from critical path
- Can add later once CoW is working

## Implementation Plan

### Phase 1: Leaf Node API

**Files to modify**:
- `bmt/beatree/leaf/node.go` - Add UpdateEntry, DeleteEntry, Clone methods
- `bmt/beatree/leaf/node_test.go` - Unit tests for new methods

**Acceptance criteria**:
- UpdateEntry works with binary search (O(log M) where M = entries)
- DeleteEntry maintains sorted order
- Clone is shallow and cheap
- All existing tests pass

### Phase 2: Leaf CoW Implementation

**Files to modify**:
- `bmt/beatree/ops/update.go` - Rewrite updateLeafNode to use new API

**Acceptance criteria**:
- updateLeafNode only allocates 1 new leaf (not copying old entries)
- Benchmark shows <10 entry operations per update (not ~100)
- All existing tests pass

### Phase 3: Branch Node API

**Files to modify**:
- `bmt/beatree/branch/node.go` - Add UpdateChild, Clone methods
- `bmt/beatree/branch/node_test.go` - Unit tests

**Acceptance criteria**:
- UpdateChild works with binary search
- Clone is shallow
- All existing tests pass

### Phase 4: Branch CoW Implementation

**Files to modify**:
- `bmt/beatree/ops/update.go` - Rewrite updateBranchNode

**Acceptance criteria**:
- updateBranchNode only modifies affected child pointers
- Structural sharing verified (sibling pages unchanged)
- All existing tests pass

### Phase 5: Integration & Performance Validation

**Testing**:
- Add benchmark: Update 1 key in 10K-key tree, verify O(log N) page writes
- Run `TestTracesRecompiler` and verify 5-10x speedup
- Compare against JavaJAM performance

**Acceptance criteria**:
- Block import time competitive with JavaJAM (~12ms)
- No regression in correctness tests
- Performance improvement documented

## Testing Strategy

### Unit Tests

```go
func TestLeafUpdateEntry(t *testing.T) {
    leaf := leaf.NewNode()
    // Insert 100 entries
    // Update 1 entry
    // Verify only 1 entry changed, rest unchanged
}

func TestBranchUpdateChild(t *testing.T) {
    branch := branch.NewNode(...)
    // Add 100 separators
    // Update 1 child pointer
    // Verify only 1 separator changed
}
```

### Integration Benchmark

```go
func BenchmarkSingleKeyUpdate(b *testing.B) {
    // Build tree with 10K keys
    tree := buildLargeTree(10000)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // Update single key
        tree.Update(key, newValue)

        // Verify only O(log N) pages written
        assert(pagesWritten <= 20) // log2(10000) ≈ 13
    }
}
```

### Regression Tests

- All existing tests must pass
- Fuzz testing with traces
- Correctness verification against JavaJAM

## Risk Mitigation

### Risks

1. **Serialization format changes**: Clone() may break if we change node structure
2. **Slice aliasing bugs**: Shallow copies could cause unintended mutations
3. **Performance regression**: If clone is too expensive

### Mitigation

1. Extensive unit tests for Clone() behavior
2. Document ownership semantics clearly
3. Benchmark Clone() operations separately
4. Keep old implementation as fallback (feature flag)

## Success Metrics

- **Performance**: Block import time < 15ms (vs current ~36s for fuzzy trace)
- **Correctness**: All existing tests pass
- **Code quality**: No increase in cyclomatic complexity
- **Maintainability**: Clear ownership semantics, well-documented

## Non-Goals (Deferred)

- Incremental GP tree hashing (can add later)
- Page-level deduplication (not needed if CoW works)
- Compression (separate optimization)

## References

- [Bottleneck analysis](bmt/beatree/ops/update.go#L186-L191)
- [Current test performance](statedb/importblock_test.go#L260-L311)
- JavaJAM performance baseline: 12.6ms mean import time
