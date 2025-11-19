# BMT Documentation

This directory contains comprehensive documentation for the BMT (Binary Merkle Tree) implementation.

## Available Documentation

### Core Components
- ✅ **[BEATREE.md](BEATREE.md)** - Phase 3: B-epsilon tree storage with CoW semantics
  - What is Beatree and why use it
  - Copy-on-Write mechanics
  - Implementation details
  - Testing strategy

- ✅ **[BITBOX.md](BITBOX.md)** - Phase 4: Hash table page storage with triangular probing
  - Triangular probing algorithm
  - WAL (Write-Ahead Log) format
  - Crash recovery mechanics
  - Performance characteristics

- ✅ **[PAGEPOOLS.md](PAGEPOOLS.md)** - Phase 2: Page pool with sync.Pool recycling
  - Page allocation and reuse
  - Memory management
  - sync.Pool integration
  - Performance optimization

- ✅ **[STORAGE-COORDINATION.md](STORAGE-COORDINATION.md)** - Phase 5: Two-phase atomic sync
  - Two-phase sync protocol
  - Crash recovery scenarios
  - File locking
  - PageId allocation

### Missing Documentation (TODO)

These documents need to be created as part of Phase 9:

- 🔴 **CORE.md** - Phase 1: Foundation layer
  - GP (Gray Paper) tree builder
  - Blake2b-GP hasher
  - JAM compliance
  - Path proofs

- 🔴 **ROLLBACK.md** - Phase 7A/7B: Delta-based rollback
  - Segmented log architecture
  - Reverse delta encoding
  - Persistent metadata
  - Rollback mechanics

- 🔴 **OVERLAY.md** - Phase 7C: In-memory uncommitted changes
  - Frozen vs live overlays
  - Ancestry chain validation
  - Status lifecycle (LIVE → COMMITTED/DROPPED)
  - Index-based O(1) lookups

- ✅ **[API.md](API.md)** - Phase 8: Top-level API
  - Database handle operations (simple & enhanced sessions)
  - Read sessions and snapshot isolation
  - Enhanced transaction semantics (begin/prepare/commit)
  - Metrics collection
  - Rollback integration
  - Root generation at every stage

- 🔴 **MERKLE.md** - Phase 6: Parallel merkle updates
  - Sharded page cache
  - Worker pool architecture
  - Page elision
  - Witness generation

## Documentation Standards

Each documentation file should include:

1. **What is it?** - High-level overview with analogies
2. **Why use it?** - Problem it solves, design decisions
3. **How it works** - Architecture, algorithms, data structures
4. **Implementation** - Key files, types, functions
5. **Testing strategy** - Test coverage, edge cases
6. **Common pitfalls** - Gotchas, debugging tips

## Quick Links

- **Main Planning Document**: [NOMT.md](../../nomt/NOMT.md) - Overall porting plan and Phase 9 roadmap
- **Enhanced Sessions Guide**: [SESSIONS.md](../SESSIONS.md) - Comprehensive guide to enhanced session API
- **Implementation Summary**: [IMPLEMENTATION-SUMMARY.md](../IMPLEMENTATION-SUMMARY.md) - Enhanced sessions implementation details
- **Rust vs Go Comparison**: [RUST-VS-GO.md](../RUST-VS-GO.md) - Go BMT vs Rust NOMT detailed comparison
- **Source Code**: [bmt/](../) - Go implementation
- **Rust Reference**: [nomt/](../../nomt/) - Original Rust implementation
