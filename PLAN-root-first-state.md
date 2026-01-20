# Implementation Plan: Root-First State Model for evm3

Status: completed in evm3. Legacy notes below are retained for historical context.

## Executive Summary

This plan describes how to eliminate `CurrentUBT` and implement root-first state access in the evm3 branch. The goal is to support standalone pre/post state per bundle, enabling safe Phase 2 resubmission without affecting other bundles.

## Problem Analysis

### Current Issues (UBT2 Branch)

1. **Invalidate + Delete Doesn't Actually Clean Up Root Indexes**
   - `DiscardSnapshot(blockNumber)` deletes `pendingSnapshots[blockNumber]`
   - But `blockRoots[blockNumber]` and `ubtRoots[rootHash]` still contain the polluted tree
   - `SetActiveTreeByRoot(root)` and `GetStateByRoot(root)` can still resolve the old root
   - Result: Resubmission can access polluted state

2. **Block-Number → Root is Single-Valued**
   - `blockRoots[blockNumber] = postRoot` is overwritten on rebuilds
   - Multiple versions in flight can't coexist in the lookup
   - Can't reliably pick the intended parent root for resubmission

3. **CurrentUBT is a Hard Fallback**
   - `CreateSnapshotForBlock` falls back to `CurrentUBT` when parent snapshot missing
   - `GetActiveTree` returns `CurrentUBT` when nothing else is set
   - Removing `CurrentUBT` requires explicit root-first access everywhere

4. **Snapshot Pollution on Rebuild**
   - When a bundle is rebuilt, the old snapshot may have been mutated (writes applied)
   - `DiscardSnapshot` removes it, but `CreateSnapshotForBlock` may clone from wrong source
   - Need to ensure rebuild always clones from the correct parent root

## Design Goals

1. **Standalone Pre/Post Per Bundle**: Each bundle execution produces `(preRoot, postRoot)` that are self-contained
2. **Root-First Access**: All state access is by root hash, never by block number alone
3. **Safe Resubmission**: Rebuilding a bundle doesn't affect other bundles or corrupt shared state
4. **Eliminate CurrentUBT**: Replace with explicit `canonicalRoot` and root→tree store

## Architecture

### New State Model

```
                   ┌─────────────────────────────────┐
                   │     Canonical State             │
                   │   canonicalRoot: Hash           │
                   │   treeStore: map[Hash]*UBT      │
                   └─────────────────────────────────┘
                              │
           ┌──────────────────┴──────────────────┐
           │                                      │
    ┌──────▼──────┐                      ┌───────▼──────┐
    │  Bundle A   │                      │   Bundle B   │
    │  preRoot: R0│                      │  preRoot: R1 │
    │  postRoot:R1│                      │  postRoot:R2 │
    │  version: 1 │                      │  version: 1  │
    └─────────────┘                      └──────────────┘
```

### Key Data Structures

```go
// StateDBStorage changes
type StateDBStorage struct {
    // REMOVE: CurrentUBT *UnifiedBinaryTree

    // NEW: Canonical state is just "the tree at canonicalRoot"
    canonicalRoot common.Hash

    // NEW: Unified tree store - all trees by root
    treeStore      map[common.Hash]*UnifiedBinaryTree
    treeStoreMutex sync.RWMutex

    // NEW: Refcount for GC (optional - can defer to Phase 2)
    treeRefCount   map[common.Hash]int

    // KEEP but repurpose: Active tree for current execution
    activeRoot common.Hash  // Instead of activeTree pointer

    // REMOVE: blockRoots map[uint64]common.Hash (replaced by root in QueueItem)
    // REMOVE: activeSnapshotBlock (replaced by activeRoot)
    // REMOVE: lastCommittedSnapshotBlock (replaced by canonicalRoot tracking)
}

// QueueItem changes (builder/queue/queue.go)
type QueueItem struct {
    // EXISTING fields...

    // NEW: Explicit roots for this bundle version
    PreRoot  common.Hash  // State root before execution (parent's postRoot or canonical)
    PostRoot common.Hash  // State root after execution (computed during build)

    // KEEP: Version for resubmission tracking
    Version int
}
```

## Implementation Phases

### Phase 1: Add Root-Based Infrastructure (Non-Breaking)

**Files**: `storage/storage.go`, `storage/witness.go`

1. Add `canonicalRoot` field to `StateDBStorage`
2. Add unified `treeStore` map replacing separate `ubtRoots`
3. Add helper methods:
   ```go
   func (s *StateDBStorage) GetCanonicalRoot() common.Hash
   func (s *StateDBStorage) SetCanonicalRoot(root common.Hash) error
   func (s *StateDBStorage) GetTreeByRoot(root common.Hash) (*UnifiedBinaryTree, bool)
   func (s *StateDBStorage) StoreTree(root common.Hash, tree *UnifiedBinaryTree)
   func (s *StateDBStorage) CloneTreeFromRoot(root common.Hash) (*UnifiedBinaryTree, error)
   ```
4. Keep `CurrentUBT` temporarily as a compatibility shim that reads from `treeStore[canonicalRoot]`

### Phase 2: Update Snapshot Lifecycle to Use Roots

**Files**: `storage/witness.go`

1. Modify `CreateSnapshotForBlock` to require explicit `parentRoot` parameter:
   ```go
   func (s *StateDBStorage) CreateSnapshotFromRoot(parentRoot common.Hash, blockNumber uint64) (common.Hash, error)
   ```

2. Update `ApplyWritesToActiveSnapshot` to return new root:
   ```go
   func (s *StateDBStorage) ApplyWritesToTree(root common.Hash, blob []byte) (newRoot common.Hash, err error)
   ```

3. Modify `CommitSnapshot` to update `canonicalRoot`:
   ```go
   func (s *StateDBStorage) CommitAsCanonical(root common.Hash) error
   ```

4. Fix `DiscardSnapshot` to also remove from `treeStore`:
   ```go
   func (s *StateDBStorage) DiscardTreeVersion(root common.Hash) bool
   ```

### Phase 3: Update QueueItem to Carry Roots

**Files**: `builder/queue/queue.go`, `builder/queue/runner.go`

1. Add `PreRoot` and `PostRoot` fields to `QueueItem`

2. Update `EnqueueBundleWithOriginalExtrinsics` to accept and store roots:
   ```go
   func (qs *QueueState) EnqueueWithRoots(
       bundle *types.WorkPackageBundle,
       preRoot common.Hash,
       postRoot common.Hash,
       // ... other params
   ) (uint64, error)
   ```

3. Update `BundleBuilder` callback signature to return roots:
   ```go
   type BundleBuilder func(item *QueueItem, stats QueueStats) (*types.WorkPackageBundle, common.Hash, common.Hash, error)
   // Returns: bundle, preRoot, postRoot, error
   ```

4. Update runner's rebuild logic to:
   - Get parent root from queue item or canonical
   - Clone from that root
   - Execute and produce new postRoot
   - Store in QueueItem

### Phase 4: Update Builder/Refine Paths

**Files**: `statedb/refine.go`, builder EVM integration

1. Update `BuildBundle` to:
   - Accept explicit `preRoot` parameter (or use canonical if none)
   - Clone tree from preRoot before execution
   - Return `postRoot` after writes applied
   - Store post-tree in `treeStore`

2. Update snapshot callbacks in runner:
   ```go
   func (r *Runner) SetOnAccumulatedWithRoots(storage RootBasedStorage, userCallback func(...)) {
       r.queue.SetOnAccumulated(func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash) {
           item := r.queue.GetByWPHash(wpHash)
           if item != nil {
               storage.CommitAsCanonical(item.PostRoot)
           }
           // ...
       })
   }
   ```

3. Update failed bundle handling:
   - Don't need to discard snapshots by block number
   - The unused postRoot tree can be GC'd later (or immediately if refcount=0)

### Phase 5: Remove CurrentUBT and Legacy Paths

**Files**: `storage/storage.go`, `storage/witness.go`, `types/storage.go`

1. Remove `CurrentUBT` field from `StateDBStorage`

2. Update all callers to use root-based access:
   - `GetActiveTree()` → `GetTreeByRoot(s.activeRoot)` (fallback to canonical)
   - `FetchBalance/Nonce/Code/Storage` → use `GetTreeByRoot`

3. Remove deprecated methods:
   - `SetActiveSnapshot(blockNumber)`
   - `GetSnapshotForBlock(blockNumber)`
   - `blockRoots` map

4. Update `EVMJAMStorage` interface to remove block-number-based methods

### Phase 6: Testing and Validation

1. Add focused tests for:
   - Resubmission isolation: v1 writes don't affect v2 rebuild
   - Out-of-order accumulation: block N+1 commits before N
   - Concurrent builds: multiple bundles building in parallel
   - Root consistency: canonical advances correctly

2. Verify existing tests pass with new implementation

## Migration Strategy

### Backward Compatibility

During migration, maintain compatibility with:
1. Existing `EVMJAMStorage` interface methods
2. Builder code that uses block-number-based snapshots
3. RPC handlers that query by block number

### Deprecation Path

1. **Phase 1-2**: Add new root-based methods alongside existing
2. **Phase 3-4**: Update callers to use new methods
3. **Phase 5**: Remove deprecated methods
4. **Phase 6**: Update interface, breaking change for external callers

## Detailed Implementation Checklist

### storage/storage.go

- [x] Add `canonicalRoot common.Hash` field
- [x] Add `treeStore map[common.Hash]*UnifiedBinaryTree` field
- [x] Add `treeStoreMutex sync.RWMutex`
- [x] Add `activeRoot common.Hash` field (replaces activeTree pointer)
- [x] Implement `GetCanonicalRoot() common.Hash`
- [x] Implement `SetCanonicalRoot(root common.Hash) error`
- [x] Implement `GetTreeByRoot(root common.Hash) (*UnifiedBinaryTree, bool)`
- [x] Implement `StoreTree(root common.Hash, tree *UnifiedBinaryTree)`
- [x] Implement `CloneTreeFromRoot(root common.Hash) (*UnifiedBinaryTree, error)`
- [x] Update `NewStateDBStorage` to initialize new fields
- [x] Update `FetchBalance/Nonce/Code/Storage` to use `GetTreeByRoot(s.activeRoot)`
- [x] Deprecate `CurrentUBT` getter (return `treeStore[canonicalRoot]`)
- [ ] Remove `CurrentUBT` field (Phase 5)
- [ ] Remove `blockRoots` map (Phase 5)

### storage/witness.go

- [x] Implement `CreateSnapshotFromRoot(parentRoot common.Hash) (snapshotRoot common.Hash, err error)`
- [x] Implement `ApplyWritesToTree(root common.Hash, blob []byte) (newRoot common.Hash, err error)`
- [x] Implement `CommitAsCanonical(root common.Hash) error`
- [x] Implement `DiscardTree(root common.Hash) bool`
- [x] Update `GetActiveTree()` to use `activeRoot`
- [x] Update `SetActiveTreeByRoot()` to set `activeRoot`
- [x] Deprecate `CreateSnapshotForBlock(blockNumber)` → wrapper around `CreateSnapshotFromRoot`
- [x] Deprecate `ApplyWritesToActiveSnapshot(blob)` → wrapper
- [x] Deprecate `CommitSnapshot(blockNumber)` → wrapper
- [ ] Remove block-number-based methods (Phase 5)

### types/storage.go (EVMJAMStorage interface)

- [x] Add `GetCanonicalRoot() common.Hash`
- [x] Add `SetCanonicalRoot(root common.Hash) error`
- [x] Add `GetTreeByRoot(root common.Hash) (interface{}, bool)`
- [x] Add `CloneTreeFromRoot(root common.Hash) (interface{}, error)`
- [x] Add `ApplyWritesToTree(root common.Hash, blob []byte) (common.Hash, error)`
- [x] Add `CommitAsCanonical(root common.Hash) error`
- [x] Add `GetActiveRoot() common.Hash`
- [x] Deprecate `CreateSnapshotForBlock`, `SetActiveSnapshot`, etc.
- [ ] Remove deprecated methods (Phase 5)

### builder/queue/queue.go

- [x] Add `PreRoot common.Hash` to `QueueItem`
- [x] Add `PostRoot common.Hash` to `QueueItem`
- [x] Update `Enqueue*` methods to accept roots
- [ ] Update `GetStats` to include root info (optional)
- [x] Add method to look up item by root (for accumulation matching)

### builder/queue/runner.go

- [ ] Update `BundleBuilder` callback type to return roots
- [ ] Update rebuild logic to use root-based cloning
- [x] Update `SetOnAccumulatedWithSnapshots` to use root-based commit
- [x] Update `SetOnFailedWithSnapshots` to use root-based discard
- [x] Add new `RootBasedStorage` interface for root-based operations

### statedb/refine.go

- [ ] Update `BuildBundle` to accept optional `preRoot` parameter
- [ ] Update `BuildBundle` to return `postRoot`
- [ ] Update witness building to operate on cloned tree
- [x] Update contract write application to return new root

### Tests

- [ ] Add `TestResubmissionIsolation` - v1 writes don't leak to v2
- [ ] Add `TestOutOfOrderAccumulation` - N+1 before N
- [ ] Add `TestConcurrentBuilds` - parallel bundle building
- [ ] Add `TestRootConsistency` - canonical advances correctly
- [ ] Add `TestCloneFromRoot` - cloning preserves state
- [x] Update existing snapshot tests to use root-based API

### builder/evm/rpc/rollup.go

- [x] Use `activeRoot` as `EVMPreStateRoot` in root-first mode
- [x] Apply root-first writes via `ApplyWritesToTree` when `activeRoot` is set
- [x] Update `activeRoot` to the post-root after copy-on-write

## Risk Mitigation

1. **Backward Compatibility**: Keep CurrentUBT as compatibility shim during migration
2. **Testing**: Add extensive tests before removing deprecated code
3. **Gradual Rollout**: Phase implementation allows partial deployment
4. **Rollback**: Keep git branches for each phase

## Success Criteria

1. `CurrentUBT` field completely removed from `StateDBStorage`
2. All state access is by root hash
3. `QueueItem` carries explicit `PreRoot` and `PostRoot`
4. Resubmission creates fresh clone from correct parent root
5. Out-of-order accumulation works correctly
6. Existing test suite passes
7. New isolation tests pass

## Timeline Notes

This plan provides concrete implementation steps without time estimates. The phases are ordered by dependency (later phases depend on earlier ones). Each phase can be implemented and tested independently before moving to the next.
