# UBT Go Port Plan

Plan for porting [ubt-rs](https://github.com/igor53627/ubt-rs) to Go for integration into the JAM storage layer.

## Overview

The Unified Binary Tree (UBT) implements [EIP-7864](https://eips.ethereum.org/EIPS/eip-7864), a replacement for Ethereum's hexary Patricia trees. Key features:

- **Single tree**: Account and storage tries merged into one tree with 32-byte keys
- **Code chunking**: Contract bytecode chunked and stored in-tree
- **Data co-location**: Related data grouped in 256-value subtrees to reduce branch openings
- **ZK-friendly**: Designed for efficient proving in ZK circuits
- **Incremental updates**: O(D*C) root updates vs O(S log S) full rebuilds (D=248 depth, C=changed stems)
- **Witness compression**: MultiProofs must scale as O(k log k), not O(k log N)
- **Efficient persistence**: On-disk layout must preserve fast proof generation and incremental updates
- **No migration path**: no MPT -> UBT conversion work in scope
- **Compatibility profile**: hash profile + key-derivation strategy selected per deployment (EIP vs JAM)

This Go plan intentionally diverges from `ubt-rs`. Witness compression and efficient persistence are core requirements, not optional follow-ons.

## 0. Central Priorities (Not a Straight Port)

- **Witness generation is minimal-trie-first**: build the smallest tree that covers the k keys, dedup siblings, and emit a canonical proof order so size is O(k log k).
- **Persistence is proof-aware**: store stems, stem hashes, and prefix hashes so sibling hashes are retrievable without scanning all stems.
- **Reuse existing BMT primitives**: lean on `storage/bmt_*.go` for overlay-root semantics, persistence tests, and path-trace patterns.

## 1. Core Architecture & Packages

Create files under `storage/` using `ubt_*.go` naming:

```
storage/UBT.md           # This file
storage/
├── ubt_tree.go          # Main tree implementation
├── ubt_hash.go          # Hash interface + Blake3 implementation
├── ubt_hash_spec.md     # Domain separation and byte layout spec
├── ubt_key.go           # Stem + TreeKey types
├── ubt_node.go          # Node interface + Internal/Stem/Empty nodes
├── ubt_embedding.go     # Ethereum state layout mapping
├── ubt_code.go          # Code chunking utilities
├── ubt_proof.go         # Merkle proof generation/verification
├── ubt_multiproof.go    # O(k log k) multiproof + witness generation
└── ubt_persistence.go   # Storage interfaces + checkpoints + prefix cache
```

No `ubt_streaming.go`: MPT -> UBT conversion is out of scope.

## 2. Type System Translation

| Rust Type | Go Equivalent |
|-----------|---------------|
| `B256` | `[32]byte` or custom `Hash32` type |
| `Address` | `[20]byte` or `common.Address` |
| `U256` | `*uint256.Int` (github.com/holiman/uint256) |
| `Hasher` trait | `Hasher` interface |
| `Stem` (31 bytes) | Custom `[31]byte` wrapper with methods |
| `SubIndex` | `uint8` |
| `HashMap<K,V>` | `map[K]V` |
| `HashSet<K>` | `map[K]bool` |
| `Option<T>` | Pointer `*T` or sentinel values |
| `Result<T, E>` | `(T, error)` tuple |

## 3. Core Data Structures

### Key Types (`ubt_key.go`)

```go
type Stem [31]byte

func (s Stem) Bit(i int) uint8 {
    // Invariant: 0 <= i < 248 (callers must enforce).
    return (s[i/8] >> (7 - i%8)) & 1
}

type TreeKey struct {
    Stem     Stem
    Subindex uint8
}

func (k TreeKey) ToBytes() [32]byte {
    var out [32]byte
    copy(out[:31], k.Stem[:])
    out[31] = k.Subindex
    return out
}

func TreeKeyFromBytes(b [32]byte) TreeKey {
    var stem Stem
    copy(stem[:], b[:31])
    return TreeKey{Stem: stem, Subindex: b[31]}
}
```

### Node Types (`ubt_node.go`)

```go
// Avoid interface dispatch in hot paths by using a discriminated union.
type NodeKind uint8

const (
    NodeEmpty NodeKind = iota
    NodeInternal
    NodeStem
)

// NodeRef points into typed slabs for each node kind.
type NodeRef struct {
    Kind  NodeKind
    Index uint32
}

var EmptyNodeRef = NodeRef{Kind: NodeEmpty, Index: 0}

// Use EmptyNodeRef as the root sentinel for an empty tree.
// Index 0 is reserved for the empty sentinel.

// EmptyNode represents an empty subtree (hash = 0).
type EmptyNode struct{}

// InternalNode has left/right children.
type InternalNode struct {
    Left  NodeRef
    Right NodeRef
}

// StemNode roots a 256-value subtree.
type StemNode struct {
    Stem   Stem
    Values map[uint8][32]byte  // Sparse storage; must be iterated in sorted order.

    // Incremental subtree cache (8 levels = 255 internal nodes).
    subtreeCache [255][32]byte
    cacheValid   bool
    cacheInit    bool // false until full subtree is built at least once
    dirtyLeaves  [4]uint64 // 256-bit bitmap
}

func NewStemNode(stem Stem) *StemNode {
    return &StemNode{
        Stem:   stem,
        Values: make(map[uint8][32]byte),
    }
}

func (s *StemNode) SortedSubindices() []uint8 {
    indices := make([]uint8, 0, len(s.Values))
    for idx := range s.Values {
        indices = append(indices, idx)
    }
    slices.Sort(indices) // Go 1.21+; use sort.Slice for older versions.
    return indices
}

func (s *StemNode) SetValue(idx uint8, value [32]byte) {
    if value == ([32]byte{}) {
        delete(s.Values, idx)
    } else {
        s.Values[idx] = value
    }
    s.dirtyLeaves[idx/64] |= 1 << (idx % 64)
    s.cacheValid = false
}

func (s *StemNode) DeleteValue(idx uint8) {
    delete(s.Values, idx)
    s.dirtyLeaves[idx/64] |= 1 << (idx % 64)
    s.cacheValid = false
}

func (s *StemNode) Hash(h Hasher) [32]byte {
    if !s.cacheValid {
        if !s.cacheInit {
            s.recomputeFullSubtree(h) // O(256) once per stem
            s.cacheInit = true
        } else {
            // Recompute only dirty paths (O(8) per dirty leaf), not full 256.
            // Implementation detail: update leaf hashes, then update ancestors in subtreeCache.
            s.recomputeDirtySubtree(h)
        }
        s.cacheValid = true
    }

    subtreeRoot := s.subtreeCache[0]
    return h.HashStemNode(&s.Stem, &subtreeRoot)
}

// subtreeCache uses heap indexing for internal nodes only (255 nodes total).
// Level 0 (root) at index 0, level 7 at indices 127..254 (parents of leaves).
func (s *StemNode) recomputeDirtySubtree(h Hasher) {
    // Mark dirty internal nodes once, then recompute bottom-up.
    var dirtyNodes [255]bool
    for word := 0; word < len(s.dirtyLeaves); word++ {
        dirty := s.dirtyLeaves[word]
        if dirty == 0 {
            continue
        }
        for bit := 0; bit < 64; bit++ {
            if (dirty>>bit)&1 == 0 {
                continue
            }
            leaf := word*64 + bit
            if leaf >= 256 {
                continue
            }
            leafIdx := uint8(leaf)
            node := 127 + int(leafIdx>>1)
            for {
                dirtyNodes[node] = true
                if node == 0 {
                    break
                }
                node = (node - 1) / 2
            }
        }
        s.dirtyLeaves[word] = 0
    }

    for node := 254; ; node-- {
        if !dirtyNodes[node] {
            if node == 0 {
                break
            }
            continue
        }
        if node >= 127 {
            idx := node - 127
            leftIdx := uint8(idx * 2)
            rightIdx := uint8(idx*2 + 1)
            left := s.leafHashAt(h, leftIdx)
            right := s.leafHashAt(h, rightIdx)
            s.subtreeCache[node] = h.Hash64(&left, &right)
        } else {
            leftChild := s.subtreeCache[node*2+1]
            rightChild := s.subtreeCache[node*2+2]
            s.subtreeCache[node] = h.Hash64(&leftChild, &rightChild)
        }
        if node == 0 {
            break
        }
    }
}

// If you want to avoid recomputing the same level-7 parent twice when both
// sibling leaves are dirty, track processed pairs (leaf>>1) before marking dirtyNodes.

func (s *StemNode) recomputeFullSubtree(h Hasher) {
    for i := 0; i < 128; i++ {
        left := s.leafHashAt(h, uint8(i*2))
        right := s.leafHashAt(h, uint8(i*2+1))
        s.subtreeCache[127+i] = h.Hash64(&left, &right)
    }
    for node := 126; ; node-- {
        leftChild := s.subtreeCache[node*2+1]
        rightChild := s.subtreeCache[node*2+2]
        s.subtreeCache[node] = h.Hash64(&leftChild, &rightChild)
        if node == 0 {
            break
        }
    }
    for i := range s.dirtyLeaves {
        s.dirtyLeaves[i] = 0
    }
}

func (s *StemNode) leafHashAt(h Hasher, idx uint8) [32]byte {
    if val, ok := s.Values[idx]; ok {
        return h.Hash32(&val)
    }
    return [32]byte{}
}

// Leaf values live inside StemNode.Values (no global leaf nodes).
```

Stem subtree hashing rules:

- Leaf hash at position i = Hash32(value_i) if present, else ZERO32.
- `StemNode.Values` MUST be non-nil; use `NewStemNode` or initialize on load.
- Internal nodes combine with Hash64(left, right) (apply zero-hash rule if specified).
- Updating one subindex must touch only O(8) nodes within the 8-level subtree.
  Cache per-stem subtree hashes or dirty paths to avoid O(256) rebuilds.
- A value of ZERO32 is treated as absent; delete it from `Values` to keep sparse
  encoding canonical.
- When loading a stem with existing Values, either set `cacheInit=false` and let
  `recomputeFullSubtree()` build the cache once, or mark all leaves dirty.
- **CRITICAL**: the subtree cache assumes `Hash64(ZERO32, ZERO32) = ZERO32`.
  If a future profile violates this, compute the empty subtree hash explicitly
  instead of relying on zero-initialized cache values.

### Tree (`ubt_tree.go`)

```go
type UnifiedBinaryTree struct {
    root               NodeRef // EmptyNodeRef when tree is empty
    hasher             Hasher
    stems              map[Stem]NodeRef // index into stemNodes
    rootDirty          bool
    stemHashCache      map[Stem][32]byte
    dirtyStemHashes    map[Stem]bool
    rootHashCached     [32]byte
    nodeHashCache      map[NodeCacheKey][32]byte
    incrementalEnabled bool

    // Node slabs (avoid interface dispatch).
    internalNodes []InternalNode
    stemNodes     []StemNode
}

type NodeCacheKey struct {
    DepthBits  uint16
    PathPrefix Stem // left-aligned prefix; unused bits beyond DepthBits are zeroed
    // Invariant: PathPrefix MUST be CanonicalPrefix(prefix, DepthBits).
}
```

Cache roles:
- Per-stem subtree cache => stem hash (HashStemNode).
- stemHashCache + nodeHashCache => upper trie caching for RootHash/proofs.

### Profile Configuration

```go
type Profile int

const (
    EIPProfile Profile = iota
    JAMProfile
)

type Config struct {
    Profile           Profile
    Hasher            Hasher
    KeyDeriver        KeyDeriver
    Capacity          int
    Incremental       bool
    ParallelThreshold int
}

func NewUnifiedBinaryTree(cfg Config) *UnifiedBinaryTree {
    if cfg.Hasher == nil {
        cfg.Hasher = NewBlake3Hasher(cfg.Profile)
    }
    if cfg.KeyDeriver == nil {
        cfg.KeyDeriver = defaultKeyDeriver(cfg.Profile)
    }
    // ...
    return &UnifiedBinaryTree{
        hasher: cfg.Hasher,
    }
}
```

```go
func CanonicalPrefix(prefix Stem, depthBits uint16) Stem {
    if depthBits >= 248 {
        return prefix
    }
    byteIndex := int(depthBits / 8)
    rem := int(depthBits % 8)

    if rem == 0 {
        for i := byteIndex; i < len(prefix); i++ {
            prefix[i] = 0
        }
        return prefix
    }

    if byteIndex < len(prefix) {
        mask := byte(0xFF << (8 - rem))
        prefix[byteIndex] &= mask
        for i := byteIndex + 1; i < len(prefix); i++ {
            prefix[i] = 0
        }
    }
    return prefix
}
```

```go
type KV struct {
    Key   TreeKey
    Value [32]byte
}
```

Node slab management:

- Start with append-only slabs for simplicity.
- Optionally add a free-list for reuse after deletions.
- On checkpoint load, slabs may be rebuilt from authoritative stem storage.
- Avoid dual sources of truth: if slabs store stem nodes, keep an index map
  (stem -> NodeRef) rather than a separate `map[Stem]*StemNode`.

```go
func (t *UnifiedBinaryTree) hashNode(ref NodeRef, depthBits uint16, prefix Stem) [32]byte {
    prefix = CanonicalPrefix(prefix, depthBits)
    if t.incrementalEnabled {
        if cached, ok := t.nodeHashCache[NodeCacheKey{DepthBits: depthBits, PathPrefix: prefix}]; ok {
            return cached
        }
    }
    if depthBits >= 248 && ref.Kind == NodeInternal {
        // Invalid tree shape; consider panic in debug builds.
        return [32]byte{}
    }

    switch ref.Kind {
    case NodeEmpty:
        return [32]byte{}
    case NodeInternal:
        n := t.internalNodes[int(ref.Index)]
        leftPrefix := prefix
        rightPrefix := prefix
        byteIndex := int(depthBits / 8)
        bitIndex := uint8(7 - (depthBits % 8))
        leftPrefix[byteIndex] &^= 1 << bitIndex
        rightPrefix[byteIndex] |= 1 << bitIndex
        left := t.hashNode(n.Left, depthBits+1, leftPrefix)
        right := t.hashNode(n.Right, depthBits+1, rightPrefix)
        hash := t.hasher.Hash64(&left, &right)
        if t.incrementalEnabled {
            t.nodeHashCache[NodeCacheKey{DepthBits: depthBits, PathPrefix: prefix}] = hash
        }
        return hash
    case NodeStem:
        n := t.stemNodes[int(ref.Index)]
        hash := n.Hash(t.hasher)
        if t.incrementalEnabled {
            t.nodeHashCache[NodeCacheKey{DepthBits: depthBits, PathPrefix: prefix}] = hash
        }
        return hash
    default:
        return [32]byte{}
    }
}
```

**Key Methods:**
- `Insert(key TreeKey, value [32]byte)`
- `Get(key *TreeKey) (value [32]byte, found bool, err error)`
- `Delete(key *TreeKey)`
- `RootHash() [32]byte`
- `EnableIncrementalMode()`
- `InsertBatch(entries []KV)`

**Cache invalidation** (required on mutation):

```go
func (t *UnifiedBinaryTree) Insert(key TreeKey, value [32]byte) {
    if value == ([32]byte{}) {
        t.Delete(&key)
        return
    }
    // ... insert logic ...
    t.invalidatePath(key.Stem)
    t.dirtyStemHashes[key.Stem] = true
    t.rootDirty = true
}

func (t *UnifiedBinaryTree) Delete(key *TreeKey) {
    // ... delete logic ...
    stemRef, ok := t.stems[key.Stem]
    if !ok {
        return
    }
    stemNode := &t.stemNodes[int(stemRef.Index)]
    stemNode.DeleteValue(key.Subindex)
    t.invalidatePath(key.Stem)
    t.dirtyStemHashes[key.Stem] = true
    t.rootDirty = true
}

func (t *UnifiedBinaryTree) invalidatePath(stem Stem) {
    for depth := uint16(0); depth < 248; depth++ {
        prefix := CanonicalPrefix(stem, depth)
        delete(t.nodeHashCache, NodeCacheKey{DepthBits: depth, PathPrefix: prefix})
    }
}
```

If per-mutation invalidation becomes too expensive, prefer one of:
- Batch mutations and invalidate once per batch.
- Use a cache epoch: store `epoch` on each cache entry and bump a global counter
  on mutation to lazily discard stale entries.
- Track which depths have cached entries and only delete those.

### Depth Definitions (Avoid Ambiguity)

- Stem tree depth: 248 bits (31 bytes × 8)
- Stem subtree depth: 8 bits (1 byte subindex)
- Total key depth: 256 bits (32 bytes)

### Determinism Rules

- Never iterate Go maps directly when hashing or emitting proofs.
- Always extract keys, sort them, then process in canonical order.

## 4. Hash Function Abstraction

### Interface (`ubt_hash.go`)

```go
// Hasher provides tree hashing operations per EIP-7864
// Special rule: hash([0x00]*64) = [0x00]*32 (scope must be explicit)
type Hasher interface {
    // Hash32 hashes a 32-byte value (leaf nodes, profile-defined)
    Hash32(value *[32]byte) [32]byte

    // Hash64 hashes a 64-byte value (internal nodes, profile-defined).
    // MUST return ZERO32 if left==ZERO32 && right==ZERO32 (all profiles).
    Hash64(left, right *[32]byte) [32]byte

    // HashStemNode hashes stem nodes (profile-defined)
    HashStemNode(stem *Stem, subtreeRoot *[32]byte) [32]byte

    // HashRaw hashes arbitrary input
    HashRaw(input []byte) [32]byte
}
```

### Implementation

**Blake3Hasher** (`ubt_hash.go`):
- Uses `github.com/zeebo/blake3`
- Single hash option for the Go implementation

Implements the zero-hash special case: `hash([0x00]*64) = [0x00]*32`

### Hash Spec (Pinned)

See `storage/ubt_hash_spec.md` for the hash profile(s) and exact byte layouts:

**EIP profile (compat)**:
- Leaf: `hash(value[32])`
- Internal: `hash(left[32] || right[32])`
- Stem: `hash(stem[31] || 0x00 || subtree_root[32])`
- Empty: `ZERO32`
- Zero-hash special case: if the 64-byte input to `Hash64` is all zero bytes,
  return `ZERO32`. This applies only to internal-node hashing and must match the
  EIP draft exactly.
- **CRITICAL**: the stem-subtree cache assumes `Hash64(ZERO32, ZERO32) = ZERO32`.
  This must hold for both EIP and JAM profiles; otherwise the empty subtree root
  must be computed explicitly.
- EIP draft currently references Blake3; update if the draft changes.
If the EIP becomes hash-agnostic or switches functions, update both the hasher
and the test vectors to stay compatible.

**JAM profile (tagged)**:
- Leaf: `hash(0x00 || value[32])`
- Internal: `hash(0x01 || left[32] || right[32])`
- Stem: `hash(0x02 || stem[31] || 0x00 || subtree_root[32])`
- Empty: `ZERO32`

Pick one profile per deployment. Compatibility claims depend on the profile.
Both profiles use Blake3; the difference is the domain separation and key derivation.

## 5. Parallelization Strategy

Replace Rust's `rayon` with Go concurrency patterns.
Assume the tree is not concurrently mutated during hashing unless guarded by a mutex.

### Parallel Stem Hashing

```go
func (t *UnifiedBinaryTree) computeStemHashesParallel() {
    var wg sync.WaitGroup
    results := make(chan struct{
        stem Stem
        hash [32]byte
    }, len(t.dirtyStemHashes))

    // Snapshot dirty stems under exclusive access or a read lock.
    dirty := make([]Stem, 0, len(t.dirtyStemHashes))
    for stem := range t.dirtyStemHashes {
        dirty = append(dirty, stem)
    }

    for _, stem := range dirty {
        wg.Add(1)
        go func(s Stem) {
            defer wg.Done()
            stemRef, ok := t.stems[s]
            if !ok {
                return
            }
            stemNode := &t.stemNodes[int(stemRef.Index)]
            hash := stemNode.Hash(t.hasher)
            results <- struct{stem Stem; hash [32]byte}{s, hash}
        }(stem)
    }

    go func() {
        wg.Wait()
        close(results)
    }()

    for result := range results {
        t.stemHashCache[result.stem] = result.hash
    }
}
```

**Considerations:**
- Use worker pool pattern for large numbers of stems (avoid goroutine-per-stem).
- Make the parallel threshold configurable or adaptive (not a fixed ~100 stems guess).
- If concurrency is allowed, guard `stems` with an RWMutex or run hashing under exclusive access.

## 6. Ethereum State Embedding

Per EIP-7864, each account maps to tree keys as follows:

| Subindex | Content |
|----------|---------|
| 0 | Basic data (version, code size, nonce, balance) |
| 1 | Code hash |
| 2-63 | Reserved |
| 64-127 | First 64 storage slots |
| 128-255 | First 128 code chunks |

### Constants (`ubt_embedding.go`)

```go
const (
    BASIC_DATA_LEAF_KEY       = 0
    CODE_HASH_LEAF_KEY        = 1
    HEADER_STORAGE_OFFSET     = 64
    CODE_OFFSET               = 128
    STEM_SUBTREE_WIDTH        = 256

    BASIC_DATA_CODE_SIZE_OFFSET = 5
    BASIC_DATA_NONCE_OFFSET     = 8
    BASIC_DATA_BALANCE_OFFSET   = 16
)
```

### Key Derivation

Define a key-derivation strategy per profile (EIP-compatible vs JAM):

```go
type KeyDeriver interface {
    DeriveBinaryTreeKey(address [20]byte, inputKey [32]byte) TreeKey
}
```

**JAM profile** (blake3-256):
```
key = blake3-256(zero[:12] || address[:] || inputKey[:31])
key[31] = inputKey[31]  // subindex preserved
```

```go
func DeriveBinaryTreeKeyJAM(address [20]byte, inputKey [32]byte) TreeKey {
    var payload [12 + 20 + 31]byte
    copy(payload[12:], address[:])
    copy(payload[32:], inputKey[:31])

    hash := blake3.Sum256(payload[:])
    var stem Stem
    copy(stem[:], hash[:31])

    return TreeKey{
        Stem:     stem,
        Subindex: inputKey[31],
    }
}
```

**EIP profile**: use the go-ethereum reference `tree_hash` derivation (SHA-256):
```
key = sha256(zero[:12] || address[:] || inputKey[:31])
key[31] = inputKey[31]  // subindex preserved
```
Keep it selectable so roots/proofs can match EIP fixtures.

### Helper Functions

- `GetBasicDataKey(profile Profile, address [20]byte) TreeKey`
- `GetCodeHashKey(profile Profile, address [20]byte) TreeKey`
- `GetStorageSlotKey(profile Profile, address [20]byte, slot [32]byte) TreeKey`
- `GetCodeChunkKey(profile Profile, address [20]byte, chunkIndex int) TreeKey`

For non-header storage slots, `GetStorageSlotKey` adds `2^248` to the 31-byte prefix
(`slot[0:31]`), which is equivalent to incrementing the most-significant byte
`slot[0]` modulo 256 while leaving bytes `slot[1:31]` unchanged.

### Code Chunking (`ubt_code.go`)

```go
// ChunkifyCode splits bytecode into 31-byte chunks
func ChunkifyCode(bytecode []byte) [][31]byte {
    numChunks := (len(bytecode) + 30) / 31
    if numChunks == 0 {
        return [][31]byte{}
    }
    chunks := make([][31]byte, numChunks)

    for i := 0; i < numChunks; i++ {
        start := i * 31
        end := min(start+31, len(bytecode))
        copy(chunks[i][:], bytecode[start:end])
    }

    return chunks
}

// DechunkifyCode reconstructs bytecode from chunks
func DechunkifyCode(chunks [][31]byte, codeSize int) []byte {
    bytecode := make([]byte, 0, codeSize)
    for _, chunk := range chunks {
        bytecode = append(bytecode, chunk[:]...)
    }
    return bytecode[:codeSize]
}
```

**Code size dependency**: `codeSize` must be read from the basic data leaf
at `BASIC_DATA_CODE_SIZE_OFFSET` before dechunking.

## 7. Efficient Persistence (Core Requirement)

Witness compression depends on retrieving sibling hashes and stems without scanning the full tree. Persistence must therefore preserve:

- Stem storage (sparse values, keyed by stem)
- Stem hash cache (stem -> hash)
- Prefix hash cache (depth + prefix -> hash) for internal nodes
- Ordered stem index for prefix/range queries

### Storage Interfaces (`ubt_persistence.go`)

```go
type StemStore interface {
    GetStem(stem Stem) (*StemNode, bool)
    PutStem(stem Stem, node *StemNode) error
    DeleteStem(stem Stem) error
    IterByPrefix(prefix Stem, prefixBits int) StemIterator
    GetSubtreeHash(prefix Stem, prefixBits int) (hash [32]byte, ok bool)
}

type HashCacheStore interface {
    GetStemHash(stem Stem) (hash [32]byte, ok bool)
    PutStemHash(stem Stem, hash [32]byte) error
    GetNodeHash(depthBits uint16, prefix Stem) (hash [32]byte, ok bool)
    PutNodeHash(depthBits uint16, prefix Stem, hash [32]byte) error
}

type StemIterator interface {
    Next() (stem Stem, node *StemNode, ok bool)
    Close() error
}
```

### Checkpointing and Recovery

- Append-only WAL for stem mutations and index updates (cache entries are derived).
- Periodic checkpoints flush stem nodes, stem index, and stem hashes.
- On restart, replay WAL to restore authoritative state (stems + index), then warm caches lazily.
- WAL format should be explicit (e.g., length-prefixed records) for tooling compatibility.

### Proof-Aware Indexing

- Maintain an ordered stem index (B-tree or sorted slice) for canonical stem order.
- Use prefix hash cache to fetch missing sibling hashes during multiproof generation in O(1).
- Authoritative state is stem contents + ordered index; caches are derived and must be rebuildable.
- `GetSubtreeHash` must be consistent with the checkpoint root used for proof generation.
- `GetSubtreeHash` should return `ZERO32` with `ok=true` for empty subtrees so proofs can emit canonical empty siblings.

### Reuse from `storage/bmt_*.go`

- `storage/bmt_storage.go`: overlay-root computation (preview root from staged ops) and flush/commit flow.
- `storage/trie.go`: `Trace` and `GetStateByRange` patterns for path/sibling collection and prefix/range iteration.
- `storage/bmt_interface.go`: proof-returning API shape (`Trace`, `GetServiceStorageWithProof`) to mirror for UBT.
- `storage/bmt_*_test.go`: persistence/overlay regression tests as templates for UBT checkpoints and recovery.

## 8. Proof System

### Types (`ubt_proof.go`)

```go
type Direction int
const (
    Left Direction = iota
    Right
)

// ProofNode represents one level in the proof path
type ProofNode interface {
    isProofNode()
}

type InternalProofNode struct {
    Sibling   [32]byte
    Direction Direction
}

type StemProofNode struct {
    Stem             Stem
    SubtreeSiblings  [][32]byte  // 8 levels
}

type ExtensionProofNode struct {
    Stem     Stem
    StemHash [32]byte // must bind to Stem (hash_stem_node)
    DivergenceDepth uint16 // in [0, 247], measured from stem-trie root
}

// Proof for a key-value pair
type Proof struct {
    Key   TreeKey
    Value *[32]byte  // nil for non-existence proof
    Path  []ProofNode
}

// Proof.Path is ordered from leaf to root and MUST begin with exactly one
// StemProofNode or ExtensionProofNode.
// Exception: an empty tree may use Path == nil with Value == nil, in which case
// verification succeeds only when expectedRoot == ZERO32.
// Direction means "currentHash is the left child" when Direction==Left.
// StemProofNode.SubtreeSiblings are also leaf-to-root (LSB to MSB for Subindex)
// and MUST contain exactly 8 siblings.

// CompactTreeKey references a deduplicated stem.
type CompactTreeKey struct {
    StemIndex uint32
    Subindex  uint8
}

// MultiProof aggregates k keys with shared siblings.
// Nodes are emitted in canonical left-to-right order over the minimal subtree.
// When NodeRefs is provided, Nodes MUST be in first-appearance order derived from
// the traversal stream (canonical dedup order).
type MultiProof struct {
    Keys     []CompactTreeKey
    Values   []*[32]byte
    Nodes    [][32]byte // deduplicated shared hashes
    NodeRefs []uint32   // traversal order indexes into Nodes (optional)
    Stems    []Stem     // deduplicated stems referenced by Keys
    MissingKeys []TreeKey // stems missing in the tree (require extension proofs; not serialized)
}

func (mp *MultiProof) ExpandKey(i int) TreeKey {
    if i < 0 || i >= len(mp.Keys) {
        return TreeKey{}
    }
    stemIdx := int(mp.Keys[i].StemIndex)
    if stemIdx < 0 || stemIdx >= len(mp.Stems) {
        return TreeKey{}
    }
    return TreeKey{Stem: mp.Stems[stemIdx], Subindex: mp.Keys[i].Subindex}
}
```

### Verification

**Note**: `Proof.Path` is ordered from leaf → root. `ExtensionProofNode.DivergenceDepth`
is measured from the stem-trie root (0..247).

Stem-trie combination uses zero short-circuiting (path compression):

```go
func combineTrieHashes(hasher Hasher, left, right [32]byte) [32]byte {
    if left == ([32]byte{}) {
        return right
    }
    if right == ([32]byte{}) {
        return left
    }
    return hasher.Hash64(&left, &right)
}
```

```go
func (p *Proof) Verify(hasher Hasher, expectedRoot [32]byte) error {
    var currentHash [32]byte

    if p.Value != nil {
        currentHash = hasher.Hash32(p.Value)
    }
    if len(p.Path) == 0 {
        if p.Value == nil && expectedRoot == ([32]byte{}) {
            return nil
        }
        return ErrInvalidProof
    }

    for _, node := range p.Path {
        switch n := node.(type) {
        case *InternalProofNode:
            if n.Direction != Left && n.Direction != Right {
                return ErrInvalidProof
            }
            if n.Direction == Left {
                currentHash = combineTrieHashes(hasher, currentHash, n.Sibling)
            } else {
                currentHash = combineTrieHashes(hasher, n.Sibling, currentHash)
            }
        case *StemProofNode:
            for i, sibling := range n.SubtreeSiblings {
                bit := (p.Key.Subindex >> i) & 1
                if bit == 0 {
                    currentHash = hasher.Hash64(&currentHash, &sibling)
                } else {
                    currentHash = hasher.Hash64(&sibling, &currentHash)
                }
            }
            currentHash = hasher.HashStemNode(&n.Stem, &currentHash)
        case *ExtensionProofNode:
            if err := verifyExtensionPrefix(p.Key.Stem, n); err != nil {
                return err
            }
            currentHash = n.StemHash
        }
    }

    if currentHash != expectedRoot {
        return &ProofError{
            Msg: fmt.Sprintf("root mismatch: got %x, want %x",
                currentHash, expectedRoot),
        }
    }
    return nil
}

func (p *Proof) ComputeRoot(hasher Hasher) [32]byte {
    var currentHash [32]byte

    if p.Value != nil {
        currentHash = hasher.Hash32(p.Value)
    } // else zero hash

    // Path is ordered from leaf to root.
    for _, node := range p.Path {
        switch n := node.(type) {
        case *InternalProofNode:
            if n.Direction == Left {
                currentHash = combineTrieHashes(hasher, currentHash, n.Sibling)
            } else {
                currentHash = combineTrieHashes(hasher, n.Sibling, currentHash)
            }
        case *StemProofNode:
            // Rebuild subtree hash
            for i, sibling := range n.SubtreeSiblings {
                bit := (p.Key.Subindex >> i) & 1
                if bit == 0 {
                    currentHash = hasher.Hash64(&currentHash, &sibling)
                } else {
                    currentHash = hasher.Hash64(&sibling, &currentHash)
                }
            }
            currentHash = hasher.HashStemNode(&n.Stem, &currentHash)
        case *ExtensionProofNode:
            // Verifier must check that n.Stem shares the queried stem prefix and
            // diverges at the expected depth before accepting StemHash.
            currentHash = n.StemHash
        }
    }

    return currentHash
}
```

**ExtensionProofNode binding**: this node represents a divergent stem when proving
non-existence (the queried path lands in a different stem). Verification MUST:

1. Check `Stem` shares the exact prefix with the queried stem up to the divergence
   depth, and that it diverges at that depth (explicitly carried as
   `DivergenceDepth`).
2. Treat `StemHash` as an opaque commitment for the divergent stem subtree.
   The current verifier does not recompute or bind `StemHash` to `Stem`.

If you want fully explicit binding, replace `ExtensionProofNode` with a node that
also carries the stem subtree proof (so the verifier can recompute the stem hash),
or drop `Stem` entirely and treat `StemHash` as an opaque subtree commitment tied
only to the queried stem prefix.

Verifier helper (sketch):

```go
func verifyExtensionPrefix(queryStem Stem, n *ExtensionProofNode) error {
    depth := n.DivergenceDepth
    if depth >= 248 {
        return ErrInvalidProof
    }
    for i := uint16(0); i < depth; i++ {
        if queryStem.Bit(int(i)) != n.Stem.Bit(int(i)) {
            return ErrInvalidProof
        }
    }
    if queryStem.Bit(int(depth)) == n.Stem.Bit(int(depth)) {
        return ErrInvalidProof
    }
    return nil
}
```

### MultiProof Verification

```go
func VerifyMultiProof(mp *MultiProof, hasher Hasher, expectedRoot [32]byte) error {
    // 1) Validate canonical ordering
    //    - Stems strictly sorted
    //    - Keys strictly sorted by expanded (stem, subindex), no duplicates
    //    - Validate StemIndex bounds
    // 2) Rebuild minimal trie structure from keys alone
    // 3) Walk trie, consuming mp.Nodes in canonical left-to-right order
    // 4) Compute root and compare to expectedRoot
    return nil
}
```

### Generation

```go
func GenerateStemProof(tree *UnifiedBinaryTree, key TreeKey) (*Proof, error) {
    // Walk tree from root to leaf, collecting siblings
    // Return proof with path from leaf to root
    // - Missing subindex within an existing stem produces Value=nil
    // - Missing stem produces an ExtensionProofNode with divergence data
}

// GenerateMultiProof builds a compressed witness for k keys in O(k log k).
func GenerateMultiProof(tree *UnifiedBinaryTree, keys []TreeKey) (*MultiProof, error) {
    // 1) Normalize: sort by (stem, subindex), dedup, group by stem
    //    - Missing subindices within existing stems are allowed (encode Value=nil)
    //    - Missing stems are not supported by multiproof format (use extension proofs)
    // 2) For each stem, build an 8-level subtree witness (dedup siblings)
    // 3) Build minimal stem trie (emit sibling hashes; ZERO32 only when subtree empty)
    // 4) Emit nodes in canonical order (verifier reconstructs shape from keys)
}
```

### MultiProof Generation (O(k log k))

**Goal**: build the minimal binary trie that covers the `k` stems and subindices. Emit only the
necessary sibling hashes, in a canonical order so the verifier can replay the same traversal.

#### Step 1: Normalize Keys

- Sort keys by `(stem, subindex)` and deduplicate.
- Group by `stem` to share the stem-level path and subtree hashing.
- Missing subindices within existing stems are allowed and encoded as `nil` values.
- Missing stems are not representable in multiproofs (use extension proofs instead).
  `GenerateMultiProof` skips missing stems and reports them in `MissingKeys`.

#### Step 2: Stem-Subtree Witness (8 levels)

For each stem group with `m` subindices:

- Mark the `m` leaf positions in the 8-level subtree.
- Bottom-up, if only one child is needed, emit the sibling hash for the other child.
- If both children are needed, recurse without emitting a hash.

This yields `O(m log m)` nodes per stem (with a constant upper bound from 8 levels).

#### Step 3: Tree-Level Multiproof over Stems

Build the minimal binary trie over the stem set (248-bit keys):

- Recursively split by bit at depth `d`.
- If both sides contain stems in the proof, recurse into both.
- If only one side has stems, emit the sibling hash for the other side
  (ZERO32 only if that subtree is truly empty) and recurse into the populated side.

This yields `O(k log k)` total nodes across all stems.

#### Step 4: Canonical Emission and Dedup

- Emit sibling hashes in a deterministic left-to-right traversal order.
  - If only the left side is populated, emit left subtree nodes first, then the right sibling.
  - If only the right side is populated, emit the left sibling first, then right subtree nodes.
- Deduplicate by interning `Nodes` and recording `NodeRefs` (indexes in traversal order).
  If `NodeRefs` is empty, `Nodes` must already be deduplicated and listed in
  traversal order (otherwise the proof is non-canonical and should be rejected).
- Verifier reconstructs the same minimal trie from `keys` and consumes `NodeRefs` in order.

**Implementation Note**: reuse the traversal shape from `storage/trie.go` (`Trace`/`tracePath`)
to collect sibling hashes, but adapt it to UBT stem/prefix depth and the minimal-trie walk.

## 9. Dependencies

### Required Packages

```go
import (
    // Hash functions
    "github.com/zeebo/blake3"

    // Ethereum types (optional, can use primitives)
    "github.com/ethereum/go-ethereum/common"

    // Big integers
    "github.com/holiman/uint256"

    // Testing
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)
```

### go.mod additions

```
require (
    github.com/zeebo/blake3 v0.2.3
    github.com/holiman/uint256 v1.2.4
    github.com/ethereum/go-ethereum v1.13.10  // optional
)
```

## 10. Performance Optimizations

### Memory Management

```go
// Use sync.Pool for temporary buffers
var hashBufferPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 64)
    },
}

func (h *Blake3Hasher) Hash64(left, right *[32]byte) [32]byte {
    buf := hashBufferPool.Get().([]byte)
    defer hashBufferPool.Put(buf)

    // Buffer doesn't need zeroing: both halves are fully overwritten.
    copy(buf[:32], left[:])
    copy(buf[32:], right[:])

    return blake3.Sum256(buf)
}
```

### Map Preallocation

```go
// When capacity is known
tree := &UnifiedBinaryTree{
    stems:           make(map[Stem]NodeRef, expectedStems),
    stemHashCache:   make(map[Stem][32]byte, expectedStems),
    dirtyStemHashes: make(map[Stem]bool, expectedChanges),
}
```

### Benchmarking

Benchmarks live in `storage/ubt_bench_test.go`:
- `BenchmarkUBTInsert`
- `BenchmarkUBTRootHashFullRebuild`
- `BenchmarkUBTRootHashIncremental`
- `BenchmarkUBTRootHashParallelThreshold`

Run:
```bash
go test ./storage -bench UBT -benchmem -count=1
```

Bench constants: 4096 stems, 1 value per stem (see `storage/ubt_bench_test.go`).

Baseline results (Apple M2 Pro, go1.25.1, `-count=1`):
```
goos: darwin
goarch: arm64
cpu: Apple M2 Pro
BenchmarkUBTInsert-10                       	     320	   3662324 ns/op	35521137 B/op	    8213 allocs/op
BenchmarkUBTRootHashFullRebuild-10          	       6	 175976257 ns/op	83028258 B/op	 2456549 allocs/op
BenchmarkUBTRootHashIncremental-10          	    1534	    757805 ns/op	  675912 B/op	     551 allocs/op
BenchmarkUBTRootHashParallelThreshold/threshold_off-10         	       6	 167859757 ns/op	82466189 B/op	 2452138 allocs/op
BenchmarkUBTRootHashParallelThreshold/threshold_128-10         	       7	 171763161 ns/op	80603741 B/op	 2381573 allocs/op
```

### Profiling

```bash
go test -bench=. -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

## 11. Testing Strategy

### Unit Tests

Port from Rust:
- Empty tree hash is zero
- Get from empty returns found=false
- Get after insert returns value
- Insert doesn't affect other keys
- Delete removes value
- Hash is deterministic
- Keys with same stem share subtree
- Order independence
- CanonicalPrefix edge cases (depthBits: 0, 1, 7, 8, 9, 247, 248)
- Prefix correctness under caching (left/right prefix bits are set correctly)
- Cache reuse: RootHash() twice yields same root and hits cache in incremental mode

Example targeted tests:

```go
func TestCanonicalPrefixEdges(t *testing.T) {
    var p Stem
    for i := range p {
        p[i] = 0xFF
    }
    cases := []uint16{0, 1, 7, 8, 9, 247, 248}
    for _, d := range cases {
        got := CanonicalPrefix(p, d)
        if d == 248 {
            require.Equal(t, p, got)
        }
        // Spot-check: bits above depth are zero.
        for i := d; i < 248; i++ {
            byteIndex := i / 8
            bitIndex := 7 - (i % 8)
            if (got[byteIndex]>>bitIndex)&1 != 0 {
                t.Fatalf("bit %d not cleared for depth %d", i, d)
            }
        }
    }
}

func TestPrefixCacheCorrectness(t *testing.T) {
    tree := NewUnifiedBinaryTree(Config{Profile: JAMProfile})
    tree.EnableIncrementalMode()
    // Insert two keys that diverge at a known depth and ensure cache stability.
    k1 := TreeKey{Stem: Stem{0x00}, Subindex: 0}
    k2 := TreeKey{Stem: Stem{0x80}, Subindex: 0} // diverges at bit 0
    tree.Insert(k1, [32]byte{1})
    tree.Insert(k2, [32]byte{2})
    root1 := tree.RootHash()
    root2 := tree.RootHash()
    require.Equal(t, root1, root2)
}
```

### Consensus Killer Tests

```go
func TestMapOrderIndependence(t *testing.T) {
    keys := generateRandomKeys(1000)
    var expected [32]byte

    for trial := 0; trial < 10; trial++ {
        tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
        shuffled := shuffle(keys)
        for _, kv := range shuffled {
            tree.Insert(kv.Key, kv.Value)
        }
        root := tree.RootHash()
        if trial == 0 {
            expected = root
        } else {
            require.Equal(t, expected, root)
        }
    }
}

func TestCrossProfileMismatch(t *testing.T) {
    state := []KV{{Key: someKey(), Value: someValue()}, {Key: someKey(), Value: someValue()}}

    eipTree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
    jamTree := NewUnifiedBinaryTree(Config{Profile: JAMProfile})

    for _, kv := range state {
        eipTree.Insert(kv.Key, kv.Value)
        jamTree.Insert(kv.Key, kv.Value)
    }

    require.NotEqual(t, eipTree.RootHash(), jamTree.RootHash())
}

func TestCheckpointProofConsistency(t *testing.T) {
    tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
    tree.Insert(key1, value1)
    rootX := tree.RootHash()
    proof, _ := GenerateStemProof(tree, key1)

    tree.Insert(key2, value2)
    rootY := tree.RootHash()

    require.NoError(t, proof.Verify(tree.hasher, rootX))
    require.Error(t, proof.Verify(tree.hasher, rootY))
}
```

### ExtensionProofNode Tests

```go
func TestExtensionProofNodeDivergenceDepth(t *testing.T) {
    tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
    key := TreeKey{Stem: Stem{0x00}, Subindex: 0}
    tree.Insert(key, [32]byte{1})

    proof, _ := GenerateStemProof(tree, key)
    // Force a bad divergence depth on an extension node (simulate tampering).
    for _, node := range proof.Path {
        if ext, ok := node.(*ExtensionProofNode); ok {
            ext.DivergenceDepth++
            break
        }
    }
    require.Error(t, proof.Verify(tree.hasher, tree.RootHash()))
}
```

### Property-Based Testing

Use `github.com/leanovate/gopter`:

```go
func TestPropertyInsertDelete(t *testing.T) {
    properties := gopter.NewProperties(nil)

properties.Property("insert then delete returns not found",
        prop.ForAll(
            func(keyBytes [32]byte, valueBytes [32]byte) bool {
                tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
                key := TreeKeyFromBytes(keyBytes)

                tree.Insert(key, valueBytes)
                tree.Delete(&key)

                _, found, err := tree.Get(&key)
                return err == nil && !found
            },
            gen.ArrayOf(32, gen.UInt8()),
            gen.ArrayOf(32, gen.UInt8()),
        ))

    properties.TestingRun(t)
}
```

### Integration Tests

- Cross-reference with go-ethereum's bintrie implementation
- Test full account state embedding
- Test large tree operations (millions of entries)
- Test concurrent access patterns
- Pull test vectors from EIP-7864 fixtures or `ubt-rs` test data

### Compatibility Tests

If go-ethereum has reference implementation:
- Compare root hashes for same input
- Verify proof compatibility

## 12. Error Handling

### Custom Errors (`error.go`)

```go
package ubt

import (
    "errors"
    "fmt"
)

var (
    ErrInvalidProof = errors.New("invalid proof")
    ErrKeyNotFound  = errors.New("key not found")
    ErrInvalidKey   = errors.New("invalid key")
    ErrProofTooLarge = errors.New("proof too large")
    ErrTruncated     = errors.New("truncated data")
)

type ProofError struct {
    Msg string
}

func (e *ProofError) Error() string {
    return fmt.Sprintf("proof error: %s", e.Msg)
}
```

### Usage

```go
func (t *UnifiedBinaryTree) Get(key *TreeKey) (value [32]byte, found bool, err error) {
    stemRef, exists := t.stems[key.Stem]
    if !exists {
        return [32]byte{}, false, nil
    }

    stemNode := t.stemNodes[int(stemRef.Index)]
    value, exists = stemNode.Values[key.Subindex]
    if !exists {
        return [32]byte{}, false, nil
    }

    return value, true, nil
}
```

Proof.Verify returns error on mismatch or invalid extension binding.


## 13. Witness Compression Scheme

UBT uses a **raw binary encoding** (not RLP) with structural deduplication to achieve O(k log k) witness sizes.

### Compression Strategy Overview

UBT witness compression relies on **three key mechanisms**:

1. **Structural Deduplication**: Shared tree paths stored once
2. **Sparse Encoding**: Only non-zero values transmitted
3. **Fixed-Size Fields**: No varint overhead, canonical encoding

Unlike Verkle Trees (EC point compression) or MPT (RLP encoding), UBT uses simple **raw concatenation** with deduplication.

### Encoding Formats

#### Primitive Types

| Type | Size | Encoding |
|------|------|----------|
| `Stem` | 31 bytes | Raw bytes (no length prefix) |
| `SubIndex` | 1 byte | Raw uint8 |
| `TreeKey` | 32 bytes | stem (31) + subindex (1) |
| `CompactTreeKey` | 5 bytes | stem_index (u32) + subindex (u8) |
| `Hash` | 32 bytes | Raw bytes |
| `Value` | 32 bytes | Raw bytes |
| `Option<Value>` | 1 or 33 bytes | tag (0/1) + value if Some |
| `Direction` | 1 byte | 0=Left, 1=Right |

#### Proof Structures

**Single Proof** (`Proof`):
```
Encoding:
  key: 32 bytes (TreeKey)
  value: 33 bytes (Option<Value>)  // 1 tag + 32 value
  path_len: 4 bytes (u32 little-endian)
  path: variable (sum of ProofNode sizes)

ProofNode variants:
  Internal:
    tag: 1 byte (0x00)
    sibling: 32 bytes
    direction: 1 byte
    Total: 34 bytes

  Stem:
    tag: 1 byte (0x01)
    stem: 31 bytes
    siblings_len: 1 byte (always 8)
    siblings: 8 × 32 = 256 bytes
    Total: 289 bytes

  Extension:
    tag: 1 byte (0x02)
    stem: 31 bytes
    divergence_depth: 2 bytes (u16)
    stem_hash: 32 bytes
    Total: 66 bytes
```

**MultiProof** (compressed):
```
Encoding:
  keys_len: 4 bytes (u32)
  keys: keys_len × (stem_index u32 + subindex u8) = 5 bytes each

  values_len: 4 bytes (u32) // MUST equal keys_len
  values: values_len × 33 bytes (Option<Value>)

  nodes_len: 4 bytes (u32)
  nodes: nodes_len × 32 bytes  // DEDUPLICATED shared hashes

  node_refs_len: 4 bytes (u32)
  node_refs: node_refs_len × 4 bytes (u32) // traversal order indices into nodes

  stems_len: 4 bytes (u32)
  stems: stems_len × 31 bytes  // DEDUPLICATED stems

Total overhead: 20 bytes (5 length fields)
Total size: 20 + k×5 + k×33 + |nodes|×32 + |node_refs|×4 + |stems|×31
```

**Canonical Order**: `stems` MUST be strictly sorted by bytes; `keys` MUST be strictly sorted by
expanded `(stem, subindex)` with no duplicates. `nodes` are emitted by a deterministic left-to-right traversal of the
minimal subtree implied by the `keys`, then deduplicated. The verifier reconstructs the same
shape from `keys` alone and consumes `node_refs` in order (or `nodes` directly if no
`node_refs` are provided). No extra path encoding is needed.

Canonical verification MUST reject:
- unreferenced stems (every stem must appear in at least one key)
- unreferenced nodes when `node_refs` is provided
- duplicate hashes in `nodes` when `node_refs` is provided
- duplicate hashes in `nodes` when `node_refs` is omitted (require deduped traversal)

**Key Insight**: `stems` and `nodes` are deduplicated; `keys` reference stems by index; `node_refs`
provide the traversal order without duplicating hashes.

### Deduplication Mechanisms

#### 1. Stem Co-location Deduplication

Keys sharing the same stem share tree-level proof:

```
Keys with same stem (31-byte prefix):
  k1: CompactTreeKey { stem_index: 0, subindex: 0 }
  k2: CompactTreeKey { stem_index: 0, subindex: 5 }
  k3: CompactTreeKey { stem_index: 0, subindex: 10 }

MultiProof structure:
  stems: [A]                    // 1 stem, not 3
  Tree path to A: ~248 nodes     // Shared
  Stem subtree proofs:
    k1: 8 siblings within A
    k2: 8 siblings within A
    k3: 8 siblings within A

Total: 248 + 3×8 = 272 nodes
vs Individual: 3×256 = 768 nodes
Compression: 64% savings
```

#### 2. Shared Sibling Deduplication

When multiple keys share path segments:

```
Example: 8 keys in 2 stem clusters
  Cluster A (4 keys): share prefix "0..."
  Cluster B (4 keys): share prefix "1..."

Naive encoding:
  8 keys × 256 nodes = 2048 nodes

Deduplicated encoding:
  Root node: 1
  Path to cluster A: ~log(4) ≈ 2-3 nodes
  Path to cluster B: ~log(4) ≈ 2-3 nodes
  Within each stem: 4 × 8 = 32 nodes
  Total: ~1 + 6 + 64 = 71 nodes

Compression ratio: 71/2048 = 3.5% (96.5% savings)
```

#### 3. Sparse Value Encoding

`StemNode` only encodes non-zero values:

```
StemNode with 256 slots but only 3 values:

Without sparsity:
  256 × 32 = 8,192 bytes

With sparsity (HashMap):
  subindex_count: 4 bytes
  (subindex, value) pairs: 3 × (1 + 32) = 99 bytes
  Total: 103 bytes

Compression: 98.7% savings for sparse nodes
```

**Important**: Proof generation must not expand all 256 positions. Use the sparse subtree
algorithm to emit only the necessary siblings so witness size stays O(m log m) per stem.

### Witness Size Formulas

#### Single Key Proof

```
Best case (shallow stem):
  Tree path: d nodes (where d = actual depth to stem)
  Stem proof: 8 nodes
  Total: (d + 8) × 32 + overhead
  ≈ d×32 + 256 + 70 bytes

Worst case (deep stem):
  Tree path: 248 nodes
  Stem proof: 8 nodes
  Total: 256 × 32 + overhead = 8,192 + 70 ≈ 8,262 bytes
```

#### MultiProof (k keys)

```
Same stem (best case):
  Tree path: 248 nodes (shared)
  Stem proofs: k × 8 nodes
  Total nodes: 248 + k×8
  Size: (248 + k×8) × 32 + k×33 + k×5 + 31 + 16
        ≈ 7,967 + k×(256 + 38) = 7,967 + k×294 bytes

Different stems (worst case, no sharing):
  Tree nodes: ~k × log(k) (minimal trie)
  Stem proofs: k × 8
  Total nodes: k×log(k) + k×8
  Size: (k×log(k) + k×8) × 32 + k×33 + k×5 + k×31 + 16

For k=100:
  Same stem: 7,967 + 29,400 = 37,367 bytes (~37 KB)
  Different: 100×7×32 + 800×32 + 6,900 ≈ 35 KB

For k=1000:
  Same stem: 7,967 + 294,000 ≈ 302 KB
  Different: 1000×10×32 + 8000×32 + 69,000 ≈ 389 KB
```

**Comparison with Naive O(k log N)**:
- Naive k=100: 100 × 8,262 = 826,200 bytes (806 KB)
- UBT k=100: ~40 KB (95% smaller)

### Canonical Encoding (Anti-Malleability)

From [serialization.v:44-50](../ubt-rs/formal/simulations/serialization.v#L44-L50):

```coq
Axiom direction_canonical : forall d1 d2,
  serialize_direction d1 = serialize_direction d2 -> d1 = d2.

Axiom treekey_canonical : forall k1 k2,
  serialize_treekey k1 = serialize_treekey k2 -> k1 = k2.
```

**Purpose**: Prevent multiple encodings of same proof (proof malleability attack).

**Rules**:
1. No padding bytes
2. No alternative encodings (e.g., RLP's variable-length nonsense)
3. Option tags must be exactly 0 (None) or 1 (Some)
4. Length fields use little-endian u32 (deterministic)
5. Arrays/lists sorted in canonical order where applicable

### Size Bounds (DoS Prevention)

From [serialization.v:269-270](../ubt-rs/formal/simulations/serialization.v#L269-L270):

```coq
Axiom serialized_size_bounded : forall mp,
  (length (serialize_multiproof mp) <= multiproof_size mp + 16)%nat.
```

**Protection**: Implementations can check size **before** deserializing to prevent memory exhaustion.

```go
const MAX_PROOF_SIZE = 10_000_000 // 10 MB

func (t *Tree) DeserializeMultiProof(data []byte) (*MultiProof, error) {
    if len(data) > MAX_PROOF_SIZE {
        return nil, ErrProofTooLarge
    }

    // Check size before full parse
    estimatedSize := estimateProofSize(data[:20]) // Read length fields
    if estimatedSize > MAX_PROOF_SIZE {
        return nil, ErrProofTooLarge
    }

    // Safe to deserialize
    return parseMultiProof(data)
}
```

### Comparison with Other Schemes

| Scheme | Encoding | Compression | Post-Quantum | ZK-Friendly |
|--------|----------|-------------|--------------|-------------|
| **MPT** | RLP | None | Yes (hash) | No (16-ary) |
| **Verkle** | EC point compression | ~48 bytes/point | **No** | Moderate |
| **UBT** | Raw binary + dedup | O(k log k) | Yes (hash) | Yes (binary) |

**UBT Advantages**:
- Simpler encoding (no RLP complexity)
- Explicit deduplication (MultiProof structure)
- Post-quantum secure (hash-based)
- Better for batches (stem co-location)

**UBT Disadvantages**:
- Larger single-key proofs than Verkle (~8 KB vs ~800 bytes)
- Requires careful deduplication implementation

### Go Implementation Requirements

#### Serialization Interface

```go
package ubt

import (
    "bytes"
    "encoding/binary"
    "io"
)

// Serializer handles witness encoding
type Serializer struct {
    canonicalOrder bool // Sort for determinism
}

const (
    MAX_KEYS  = 100_000
    MAX_NODES = 10_000_000
    MAX_STEMS = 100_000
)

// Tune limits based on expected block witness sizes.

// SerializeMultiProof encodes a MultiProof
func (s *Serializer) SerializeMultiProof(mp *MultiProof, w io.Writer) error {
    // Write length fields (5 × u32 = 20 bytes)
    if err := binary.Write(w, binary.LittleEndian, uint32(len(mp.Keys))); err != nil {
        return err
    }
    if err := binary.Write(w, binary.LittleEndian, uint32(len(mp.Values))); err != nil {
        return err
    }
    if err := binary.Write(w, binary.LittleEndian, uint32(len(mp.Nodes))); err != nil {
        return err
    }
    if err := binary.Write(w, binary.LittleEndian, uint32(len(mp.NodeRefs))); err != nil {
        return err
    }
    if err := binary.Write(w, binary.LittleEndian, uint32(len(mp.Stems))); err != nil {
        return err
    }

    // Write keys (k × 5 bytes) as CompactTreeKey
    for _, key := range mp.Keys {
        if err := binary.Write(w, binary.LittleEndian, key.StemIndex); err != nil {
            return err
        }
        if _, err := w.Write([]byte{key.Subindex}); err != nil {
            return err
        }
    }

    // Write values (k × 33 bytes)
    for _, val := range mp.Values {
        if val == nil {
            if _, err := w.Write([]byte{0}); err != nil {
                return err
            }
        } else {
            if _, err := w.Write([]byte{1}); err != nil {
                return err
            }
            if _, err := w.Write(val[:]); err != nil {
                return err
            }
        }
    }

    // Write deduplicated nodes (|nodes| × 32 bytes)
    for _, node := range mp.Nodes {
        if _, err := w.Write(node[:]); err != nil {
            return err
        }
    }

    // Write node refs (|node_refs| × 4 bytes)
    for _, ref := range mp.NodeRefs {
        if err := binary.Write(w, binary.LittleEndian, ref); err != nil {
            return err
        }
    }

    // Write deduplicated stems (|stems| × 31 bytes)
    for _, stem := range mp.Stems {
        if _, err := w.Write(stem[:]); err != nil {
            return err
        }
    }

    return nil
}

// DeserializeMultiProof decodes a MultiProof
func (s *Serializer) DeserializeMultiProof(r io.Reader) (*MultiProof, error) {
    // Read length fields
    var keysLen, valuesLen, nodesLen, nodeRefsLen, stemsLen uint32
    if err := binary.Read(r, binary.LittleEndian, &keysLen); err != nil {
        return nil, err
    }
    if err := binary.Read(r, binary.LittleEndian, &valuesLen); err != nil {
        return nil, err
    }
    if err := binary.Read(r, binary.LittleEndian, &nodesLen); err != nil {
        return nil, err
    }
    if err := binary.Read(r, binary.LittleEndian, &nodeRefsLen); err != nil {
        return nil, err
    }
    if err := binary.Read(r, binary.LittleEndian, &stemsLen); err != nil {
        return nil, err
    }

    // Validate bounds
    if keysLen > MAX_KEYS || nodesLen > MAX_NODES || stemsLen > MAX_STEMS {
        return nil, ErrProofTooLarge
    }
    if valuesLen != keysLen {
        return nil, ErrInvalidProof
    }

    // Read keys
    keys := make([]CompactTreeKey, keysLen)
    for i := range keys {
        if err := binary.Read(r, binary.LittleEndian, &keys[i].StemIndex); err != nil {
            return nil, err
        }
        var subindex [1]byte
        if _, err := io.ReadFull(r, subindex[:]); err != nil {
            return nil, err
        }
        keys[i].Subindex = subindex[0]
    }

    // Read values
    values := make([]*[32]byte, valuesLen)
    for i := range values {
        var tag [1]byte
        if _, err := io.ReadFull(r, tag[:]); err != nil {
            return nil, err
        }
        if tag[0] != 0 && tag[0] != 1 {
            return nil, ErrInvalidProof
        }
        if tag[0] == 1 {
            val := &[32]byte{}
            if _, err := io.ReadFull(r, val[:]); err != nil {
                return nil, err
            }
            values[i] = val
        }
    }

    // Read nodes
    nodes := make([][32]byte, nodesLen)
    for i := range nodes {
        if _, err := io.ReadFull(r, nodes[i][:]); err != nil {
            return nil, err
        }
    }

    // Read node refs
    nodeRefs := make([]uint32, nodeRefsLen)
    for i := range nodeRefs {
        if err := binary.Read(r, binary.LittleEndian, &nodeRefs[i]); err != nil {
            return nil, err
        }
        if nodeRefs[i] >= nodesLen {
            return nil, ErrInvalidProof
        }
    }

    // Read stems
    stems := make([]Stem, stemsLen)
    for i := range stems {
        if _, err := io.ReadFull(r, stems[i][:]); err != nil {
            return nil, err
        }
    }
    for i := 1; i < len(stems); i++ {
        if bytes.Compare(stems[i-1][:], stems[i][:]) >= 0 {
            return nil, ErrInvalidProof
        }
    }

    for _, key := range keys {
        if key.StemIndex >= stemsLen {
            return nil, ErrInvalidProof
        }
    }

    return &MultiProof{
        Keys:     keys,
        Values:   values,
        Nodes:    nodes,
        NodeRefs: nodeRefs,
        Stems:    stems,
    }, nil
}
```

Consider implementing `encoding.TextMarshaler` / `encoding.TextUnmarshaler`
for stems and hashes so logs and JSON debugging are consistent and readable.

#### Deduplication Helper

```go
// DeduplicateNodes removes duplicate hashes while preserving traversal order.
func DeduplicateNodes(nodes [][32]byte) ([][32]byte, []uint32) {
    seen := make(map[[32]byte]uint32)
    unique := make([][32]byte, 0, len(nodes))
    refs := make([]uint32, 0, len(nodes))

    for _, node := range nodes {
        if idx, exists := seen[node]; exists {
            refs = append(refs, idx)
            continue
        }
        idx := uint32(len(unique))
        seen[node] = idx
        unique = append(unique, node)
        refs = append(refs, idx)
    }

    return unique, refs
}
```

#### JSON Support (for debugging)

```go
func (s *StemNode) MarshalJSON() ([]byte, error) {
    return json.Marshal(&struct {
        Stem   string            `json:"stem"`
        Values map[uint8]string  `json:"values"`
    }{
        Stem:   hex.EncodeToString(s.Stem[:]),
        Values: encodeValuesMap(s.Values),
    })
}

func encodeValuesMap(values map[uint8][32]byte) map[uint8]string {
    out := make(map[uint8]string, len(values))
    for idx, val := range values {
        out[idx] = hex.EncodeToString(val[:])
    }
    return out
}
```

### Testing Requirements

```go
func TestSerializationRoundtrip(t *testing.T) {
    mp := &MultiProof{
        Keys:   []CompactTreeKey{{StemIndex: 0, Subindex: 0}},
        Values: []*[32]byte{{0x01, ...}},
        Nodes:  [][32]byte{{0x02, ...}},
        Stems:  []Stem{{0x42, ...}},
    }

    // Serialize
    var buf bytes.Buffer
    s := &Serializer{}
    s.SerializeMultiProof(mp, &buf)

    // Deserialize
    mp2, err := s.DeserializeMultiProof(&buf)
    require.NoError(t, err)

    // Verify equality
    assert.Equal(t, mp, mp2)
}

func TestCanonicalEncoding(t *testing.T) {
    // Same MultiProof should always produce same bytes
    mp := generateTestMultiProof()

    var buf1, buf2 bytes.Buffer
    s := &Serializer{}
    s.SerializeMultiProof(mp, &buf1)
    s.SerializeMultiProof(mp, &buf2)

    assert.Equal(t, buf1.Bytes(), buf2.Bytes())
}

func TestMalformedInputRejection(t *testing.T) {
    s := &Serializer{}

    // Truncated input
    _, err := s.DeserializeMultiProof(bytes.NewReader([]byte{1, 2, 3}))
    assert.Error(t, err)

    // Invalid option tag
    badData := []byte{0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
    badData = append(badData, make([]byte, 5)...)  // compact key
    badData = append(badData, 0xFF)                 // invalid tag
    _, err = s.DeserializeMultiProof(bytes.NewReader(badData))
    assert.Error(t, err)
}
```

### Size Estimation (Pre-Deserialization Check)

```go
func EstimateMultiProofSize(data []byte) (int, error) {
    if len(data) < 20 {
        return 0, ErrTruncated
    }

    keysLen := binary.LittleEndian.Uint32(data[0:4])
    valuesLen := binary.LittleEndian.Uint32(data[4:8])
    nodesLen := binary.LittleEndian.Uint32(data[8:12])
    nodeRefsLen := binary.LittleEndian.Uint32(data[12:16])
    stemsLen := binary.LittleEndian.Uint32(data[16:20])

    if valuesLen != keysLen {
        return 0, ErrInvalidProof
    }

    size := 20 +
        int(keysLen)*5 +
        int(valuesLen)*33 +
        int(nodesLen)*32 +
        int(nodeRefsLen)*4 +
        int(stemsLen)*31

    return size, nil
}
```

## 14. Documentation

### Package Documentation (`ubt_tree.go`)

```go
// Package ubt implements the Unified Binary Tree (UBT) per EIP-7864.
//
// The UBT is a binary tree structure designed to replace Ethereum's hexary
// Patricia trees. It provides:
//
//   - Single tree: Account and storage tries merged into one tree
//   - Code chunking: Contract bytecode stored in-tree as 31-byte chunks
//   - ZK-friendly: Efficient for zero-knowledge proof generation
//   - Incremental updates: O(D*C) vs O(S log S) for block-by-block updates
//
// # Tree Structure
//
// Keys are 32 bytes split as:
//   - First 31 bytes: stem (defines subtree path)
//   - Last byte: subindex (position within 256-value subtree)
//
// # Basic Usage
//
//   tree := ubt.NewUnifiedBinaryTree(Config{Profile: ubt.EIPProfile})
//   key := TreeKeyFromBytes([32]byte{1, 2, 3, ...})
//   tree.Insert(key, [32]byte{0x42, ...})
//   root := tree.RootHash()
//
// # Performance Modes
//
// For block-by-block updates with few changes:
//
//   tree.EnableIncrementalMode()
//   tree.RootHash()  // Populate cache
//   // Future updates reuse cached intermediate hashes
//
// See https://eips.ethereum.org/EIPS/eip-7864 for specification.
package ubt
```

### README.md

Create `storage/ubt_README.md` with:
- Overview of UBT and EIP-7864
- Installation instructions
- Usage examples
- Performance comparison with MPT
- API reference link

### Performance Guide

Document when to use:
- **Incremental mode**: Block-by-block updates, <1% stems change
- **Full rebuild**: Large batch inserts, initial tree construction
 - **Parallel hashing**: configurable/adaptive threshold for dirty stems

## 15. Formal Verification (Future)

The Rust implementation has formal verification using Rocq proof assistant:
- 641 theorems proven
- 95% verification confidence
- Core operations proven correct

For Go port:
1. **Phase 1**: Functional correctness via extensive testing
2. **Phase 2**: Consider formal verification approaches:
   - Translation to Dafny (verification-aware language)
   - TLA+ specification for concurrent components
   - Randomized property-style tests (stdlib `math/rand` today; gopter optional)

Document the Rust proofs as reference specification.

## Key Translation Challenges

| Challenge | Rust | Go Solution |
|-----------|------|-------------|
| Generics | `UnifiedBinaryTree<H: Hasher>` | Use `Hasher` interface |
| Options | `Option<B256>` | Use `*[32]byte` |
| Results | `Result<T, E>` | Return `(T, error)` |
| Ownership | Move semantics | Manual GC, copy when needed |
| Traits | `Hasher` trait | `Hasher` interface |
| Parallel | `rayon` | Goroutines + sync.WaitGroup |
| No macros | Derive macros | Manual implementation |

## Implementation Order (Updated Status)

**Pinned spec**: `storage/ubt_hash_spec.md` defines the Blake3 profiles and the
Hash64 zero-hash rule. Test vectors stay aligned to Blake3 only.

**Completed**
1. **ubt_hash.go** - Hash interface + Blake3 implementation
2. **ubt_key.go** - Stem and TreeKey types with bit operations
3. **ubt_node.go** - Node interface + 3 node types (Empty/Internal/Stem)
4. **ubt_tree.go** - Insert/Get/Delete/RootHash (in-memory)
5. **ubt_persistence.go** - Overlay/persistence patterns aligned with `storage/bmt_*`
6. **ubt_embedding.go** - Ethereum state layout mapping (profile-aware derivation)
7. Parallel stem hashing in `RootHash`
8. Incremental mode + prefix hash cache persistence
9. **ubt_proof.go** / **ubt_multiproof.go** - Single proofs + O(k log k) multiproof generation
10. Test suite scaffolding (unit, vectors, integration, randomized property-style, proof tests)

**Remaining**
- Integration with JAM storage layer if additional glue is needed

## Success Criteria (Progress)

- [x] Core Go implementation complete (hash/key/node/tree/persistence/embedding/proof/multiproof)
- [x] Parallel root hashing and incremental prefix-cache mode implemented
- [x] Multiproof hash deduplication implemented (Nodes interning + NodeRefs)
- [x] Canonical multiproof verification enforced (keys/stems/nodes)
- [x] Non-existence proofs supported (missing subindex or missing stem)
- [x] Incremental proof cache invalidation for dirty stems
- [x] Test suite scaffolding added (unit, vectors, integration, randomized property-style, proof tests)
- [x] `storage/ubt_hash_spec.md` pinned and kept in sync with Blake3 profiles
- [x] Root hashes verified against Rust vectors for EIP profile (vector + integration tests)
- [x] Proof verification compatibility validated against reference vectors
- [x] Benchmarks run and baseline numbers captured
- [ ] Memory profiling completed
- [ ] Overlay/checkpoint behavior verified against `storage/bmt_*` patterns
- [ ] Integration tests with JAM storage layer (if required)
- [ ] Documentation completeness pass (godoc + README)

## References

- EIP-7864: https://eips.ethereum.org/EIPS/eip-7864
- ubt-rs: https://github.com/igor53627/ubt-rs
- go-ethereum: https://github.com/ethereum/go-ethereum
- Formal verification: `../ubt-rs/formal/docs/VERIFICATION_SUMMARY.md`
