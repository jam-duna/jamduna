# BMT Package - Implementation TODOs

## Completed

### Enhanced Sessions API
✅ **Actual Root Computation** - Implemented using GP tree building in WriteSession and PreparedSession
✅ **Merkle Proof Generation** - Implemented proof path generation by walking GP tree
✅ **Witness Verification** - Added VerifyWitness() function using core proof package
✅ **Multi-Key Proofs** - Batch witness generation for multiple operations
✅ **ReadSession Proofs** - Generate BMT-format merkle proofs from read-only snapshots ([nomt.go:369-418](bmt/nomt.go#L369-L418))
  - Implemented `GenerateProof()` method for Nomt
  - Works with committed state (via beatreeWrapper.persistedData) and overlay
  - Temporarily rebuilds in-memory MerkleTree for BMT-compatible proof generation
  - Used by `StorageHub.Trace()` for JAM witness generation
  - See [Multi-Key Proof Documentation](docs/API.md#L621-L738) for usage patterns

### Beatree Package
✅ **Branch Deserialization** - Implemented in PartialLookup using branch.DeserializeBranchNode and SearchBranch ([beatree/ops/lookup.go:18-37](bmt/beatree/ops/lookup.go#L18-L37))
✅ **Branch Fullness Target** - Implemented 75% fullness target for left node in SplitLeaf ([beatree/ops/update.go:219-274](bmt/beatree/ops/update.go#L219-L274))
✅ **Branch Serialization** - Implemented in ReconstructFromBranches using branch.Serialize() ([beatree/ops/reconstruction.go:84-88](bmt/beatree/ops/reconstruction.go#L84-L88))

### Coordinator Layer (Partial)
✅ **ops.Update Implementation** - Full tree serialization with CoW semantics via ops.Update
✅ **StoreAdapter Pattern** - Adapter pattern to bridge allocator.Store to ops.PageStore interface
✅ **Test Coverage** - Comprehensive tests demonstrating coordinator pattern ([beatree/ops/coordinator_example_test.go](bmt/beatree/ops/coordinator_example_test.go))

**Note on Integration**: Due to Go's import cycle restrictions (beatree ↔ ops), the coordinator layer must be implemented at the application level rather than within the beatree package itself. The core functionality (ops.Update) is complete and ready for use by higher-level coordinators.

## Future Enhancements

### High Priority - Core Functionality
1. **Multi-level Tree Updates** - Extend Update() to handle branch node splitting and multi-level trees ([beatree/ops/update.go:29-33](bmt/beatree/ops/update.go#L29-L33))
   - Required for scaling beyond single-leaf trees
   - Currently handles single-leaf trees only
2. **PageWalker Disk I/O** - Integrate actual page loading from disk storage ([merkle/page_walker.go:70-72](bmt/merkle/page_walker.go#L70-L72))
   - Essential for persistent state and crash recovery
   - Currently operates in-memory only

### Medium Priority - Scalability & Performance
3. **Async I/O** - Non-blocking disk operations for sync, page loading, and iteration ([beatree/sync_controller.go:16-17](bmt/beatree/sync_controller.go#L16-L17))
   - Significant performance improvement for concurrent workloads
4. **Multi-level Branch Reconstruction** - Extend Reconstruct() to handle multi-level trees for very large datasets
   - Enables scaling to production-size data
5. **Disk-backed Iterator** - Implement disk iteration for leaf pages ([beatree/iterator.go:19-20](bmt/beatree/iterator.go#L19-L20))
   - Required for range queries on large datasets
6. **Overflow Staging Optimization** - Read overflow values from pages instead of storing inline in staging ([beatree/read_transaction.go:77-79](bmt/beatree/read_transaction.go#L77-79))
   - Memory efficiency for large values

### Low Priority - Enhancements
7. **Branch Page Updates** - Implement branch node updates with separator modifications ([merkle/page_walker.go:216-227](bmt/merkle/page_walker.go#L216-L227))
   - Nice-to-have for merkle package completeness
8. **Overflow Page Allocation** - Allocate overflow pages for large values instead of truncating ([merkle/page_walker.go:180-191](bmt/merkle/page_walker.go#L180-L191))
   - Fixes limitation in PageWalker
9. **Async Commit** - Optional non-blocking commit operations
    - Performance optimization for write-heavy workloads
10. **Session Statistics** - Track operation counts per session
    - Observability and debugging aid
11. **Lock-free Leaf Cache** - Upgrade from mutex-based LRU to lock-free structures if performance bottleneck ([beatree/leaf_cache.go:13-16](bmt/beatree/leaf_cache.go#L13-L16))
    - Only if profiling shows cache contention
12. **Beatree Page Traversal** - Implement direct page traversal API (like Rust NOMT's Seeker) for proof generation
    - Would eliminate need to rebuild in-memory tree from persistedData
    - Performance optimization for `GenerateProof()` at scale
