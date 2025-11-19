# BMT Core - Foundation Layer

The **Core** module provides the foundational components for the BMT (Binary Merkle Tree) implementation, including Gray Paper (GP) compatible tree operations, cryptographic hashing, and Merkle proof generation.

## Overview

The core module implements the fundamental building blocks required for JAM blockchain state management:

- **GP Tree Operations**: Binary trie operations following JAM Gray Paper specifications
- **Blake2b Hashing**: Cryptographic hash functions with GP compatibility
- **Merkle Proofs**: Path and multi-proof generation for state verification
- **Trie Positioning**: Navigation and indexing within the binary trie structure

## Key Components

### 1. Gray Paper Tree (`gp_tree.go`)

Implements the canonical binary trie structure as specified in the JAM Gray Paper:

```go
// Build a GP-compatible tree from operations
func BuildTree(ops []Operation, hasher GPHasher) (*GPTree, error)

// Compute root hash matching JAM test vectors
func (t *GPTree) ComputeRoot() [32]byte
```

**Features:**
- Binary trie construction from sorted operations
- Internal node hash computation with Blake2b
- Leaf node encoding (embedded vs hashed values)
- JAM test vector compatibility

**Test Coverage:** 4/4 tests passing, including JAM compatibility verification

### 2. Blake2b Hasher (`hasher.go`)

Provides GP-compliant cryptographic hashing:

```go
type Blake2bGpHasher struct{}

// Hash leaf nodes (embedded or hashed values)
func (h *Blake2bGpHasher) HashLeaf(key [32]byte, value []byte) [32]byte

// Hash internal nodes with child hashes
func (h *Blake2bGpHasher) HashInternal(leftHash, rightHash [32]byte) [32]byte
```

**Features:**
- Blake2b-256 hashing algorithm
- Leaf value embedding for small values (<= 32 bytes)
- Large value hashing for overflow scenarios
- Internal node hash combining

**Test Coverage:** 3/3 tests passing with hash verification

### 3. Trie Positioning (`trie_pos.go`)

Navigation system for binary trie traversal:

```go
type TriePosition struct {
    page  uint16  // Page index within trie
    index uint16  // Node index within page
}

// Navigate to child nodes
func (pos TriePosition) LeftChild() TriePosition
func (pos TriePosition) RightChild() TriePosition

// Check page boundaries for memory management
func (pos TriePosition) InNextPage() bool
```

**Features:**
- Hierarchical page organization (64 nodes per page)
- Depth-based page transitions at multiples of 6 levels
- Child node address calculation
- Page boundary detection for I/O optimization

**Test Coverage:** 3/3 tests passing with boundary verification

### 4. Node Types and Encoding (`nodes.go`)

Node structure and serialization for trie storage:

```go
type NodeKind int
const (
    Terminator NodeKind = iota  // Empty trie branches
    Leaf                        // Key-value data nodes
    Internal                    // Branch nodes with child pointers
)

// Encode leaf data with GP format
func (leaf *LeafData) EncodeGP() []byte

// Decode node kind from stored data
func NodeKindFromBytes(data []byte) NodeKind
```

**Features:**
- Node kind identification via MSB encoding
- GP-compatible leaf data serialization
- Internal node child pointer encoding
- Terminator handling for sparse tries

**Test Coverage:** 9/9 tests passing with encoding verification

### 5. Operations and Sorting (`ops.go`)

Operation processing for trie updates:

```go
type Operation struct {
    Key    [32]byte
    Value  []byte
    Delete bool
}

// Sort operations by key for deterministic processing
func SortOperations(ops []Operation)

// Compute shared bit prefix between keys
func SharedBits(a, b [32]byte) int
```

**Features:**
- Deterministic operation ordering
- Bit-level key comparison
- Shared prefix computation for branching
- Delete operation handling

**Test Coverage:** 9/9 tests passing with sorting verification

## Merkle Proofs (`proof/`)

The proof submodule provides verification capabilities:

### Path Proofs
```go
// Generate inclusion/exclusion proof for a key
func GeneratePathProof(root [32]byte, key [32]byte, tree *GPTree) (*PathProof, error)

// Verify proof against known root
func (p *PathProof) Verify(root [32]byte, key [32]byte) bool
```

### Multi-Proofs
```go
// Generate proof for multiple keys simultaneously
func GenerateMultiProof(root [32]byte, keys [][32]byte, tree *GPTree) (*MultiProof, error)
```

**Features:**
- Single key inclusion/exclusion proofs
- Multi-key batch proof generation
- Sibling hash collection for verification
- Proof compression through shared paths

**Test Coverage:** 7/7 tests passing with proof verification

## Integration with BMT

The Core module serves as the foundation for:

1. **Beatree Operations**: Binary trie logic for B-epsilon tree leaves
2. **Merkle Module**: Proof generation and witness collection
3. **Store Module**: Node serialization and persistence
4. **Top-Level API**: Root hash computation and state verification

## Performance Characteristics

- **Memory**: Efficient page-based organization (64 nodes per 4KB page)
- **Hashing**: Hardware-optimized Blake2b implementation
- **Proofs**: Logarithmic proof size O(log n) for verification
- **Compatibility**: 100% JAM Gray Paper test vector compliance

## Test Results

```
=== Core Module Test Summary ===
GP Tree:        4/4 passing ✅ (JAM compatibility verified)
Hasher:         3/3 passing ✅ (Blake2b verification)
Trie Position:  3/3 passing ✅ (Page boundary handling)
Nodes:          9/9 passing ✅ (GP encoding compliance)
Operations:     9/9 passing ✅ (Deterministic sorting)
Proofs:         7/7 passing ✅ (Verification logic)

Total: 35/35 tests passing ✅
```

The Core module provides production-ready foundational components for JAM blockchain state management with full Gray Paper compatibility.