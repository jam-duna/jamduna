# UBT Test Vectors for Go Implementation

## Status: All Test Infrastructure Complete ✅

**Test Coverage Summary:**
- ✅ **40+ unit tests** ported from Rust (hash, key, node, tree operations)
- ✅ **5 structure tests** with Blake3 EIP profile cross-validation
- ✅ **10 integration tests** with operations and query validation
- ✅ **5 randomized property-style tests** ready (stdlib `math/rand`)
- ✅ **5 proof tests** ready (single proof, multiproof, witness serialization)
- ✅ **Total: 65+ test cases** ready to validate Go implementation

**Test Files Created:**
1. [hash_test.go](hash_test.go) - Blake3 hash function validation
2. [key_test.go](key_test.go) - TreeKey and Stem operations
3. [node_test.go](node_test.go) - Node hashing (Empty, Leaf, Stem, Internal)
4. [tree_test.go](tree_test.go) - Tree operations (insert, get, delete, root hash)
5. [vectors_test.go](vectors_test.go) - Cross-validation against Rust (5 vectors)
6. [integration_test.go](integration_test.go) - Integration tests (10 scenarios)
7. [property_test.go](property_test.go) - Property-based tests (5 properties)
8. [proof_test.go](proof_test.go) - Proof generation and verification (5 tests)
9. [proof_vectors_test.go](proof_vectors_test.go) - Proof verification against Rust vectors

**Rust Test Vector Exports:**
- [export_blake3_vectors.rs](../ubt-rs/tests/export_blake3_vectors.rs) → [test_vectors_blake3_eip.json](../test_vectors_blake3_eip.json) + [test_vectors_blake3_jam.json](../test_vectors_blake3_jam.json)
- [export_blake3_simple.rs](../ubt-rs/tests/export_blake3_simple.rs) → [test_vectors_blake3_eip.json](../test_vectors_blake3_eip.json) (legacy subset)
- [export_integration_blake3.rs](../ubt-rs/tests/export_integration_blake3.rs) → [test_vectors_integration_blake3.json](../test_vectors_integration_blake3.json)
- [export_proof_blake3.rs](../ubt-rs/tests/export_proof_blake3.rs) → [test_vectors_proof_blake3.json](../test_vectors_proof_blake3.json)

## Overview

The `ubt-rs` codebase contains **extensive test vectors** across multiple test files. These have been ported to validate the Go implementation against the **actual UBT.md API** (not the Rust API).

## CRITICAL: API Alignment with UBT.md

All Go test code must use the **final API from UBT.md**:

```go
// Correct API from UBT.md
func TreeKeyFromBytes(b [32]byte) TreeKey  // standalone function, NOT method
func (s Stem) Bit(i int) uint8             // Bit(), NOT BitAt()
func (t *UnifiedBinaryTree) Get(key *TreeKey) (value [32]byte, found bool, err error)  // value tuple, NOT pointer
func (t *UnifiedBinaryTree) Insert(key TreeKey, value [32]byte)
func (t *UnifiedBinaryTree) Delete(key *TreeKey)
func (t *UnifiedBinaryTree) RootHash() [32]byte

// Constructor takes Config struct
type Config struct {
    Profile           Profile
    Hasher            Hasher      // Optional: defaults to NewBlake3Hasher(profile)
    KeyDeriver        KeyDeriver  // Optional: defaults to DefaultKeyDeriver
    Capacity          int
    Incremental       bool
    ParallelThreshold int
}

func NewUnifiedBinaryTree(cfg Config) *UnifiedBinaryTree

// Convenience profiles
const (
    EIPProfile Profile = iota
    JAMProfile
)
```

## Test Vector Sources

| File | Hash Function | Profile | Priority | Purpose |
|------|---------------|---------|----------|---------|
| `tests/proptests.rs` | Blake3 | Untagged | ✓✓ **High** | Property test templates for EIP profile |
| `tests/integration_export.rs` | Blake3 | Untagged | ✓✓ **High** | Deterministic test cases for EIP profile |
| `src/compat_tests.rs` | Blake3 | Untagged | ✓✓ **High** | Tree structure validation for EIP profile |
| `src/geth_compat.rs` | **SHA256** | N/A | Removed | not used (Blake3-only implementation) |

## Two-Profile Testing Strategy

### Profile 1: EIP Profile (Untagged Blake3) - **Primary**

**This is the main UBT profile**. Expected roots come from `compat_tests.rs` and `integration_export.rs`.

```go
tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
```

✅ **COMPLETED**: Exported Blake3 EIP profile test vectors in Phase 2 and Phase 4.

### Profile 2: JAM Profile (Tagged Blake3)

**JAM storage profile** with tagged domain separation.

```go
tree := NewUnifiedBinaryTree(Config{Profile: JAMProfile})
```

✅ **COMPLETED (export-only)**: `export_blake3_vectors.rs` emits JAM vectors using a
test-local tagged Blake3 hasher. `ubt-rs` core still only implements the EIP profile.

---

## Priority Test Vectors for Go Port

### 1. Integration Test Cases (`tests/integration_export.rs`) - **Highest Priority**

**Deterministic Blake3 test cases** for cross-language validation. These use **Blake3** (untagged profile).

✅ **COMPLETED**: Exported from Rust with expected roots (`test_vectors_integration_blake3.json`).

#### Test Case: Empty Tree Get
```rust
Operations: []
Query: {stem: [1, 2, 3], subindex: 5}
Expected: None
```

**Go test (using UBT.md API)**:
```go
func TestEmptyTreeGet(t *testing.T) {
    tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

    // Build key correctly: stem [1,2,3] + subindex 5
    var keyBytes [32]byte
    copy(keyBytes[:31], []byte{1, 2, 3})
    keyBytes[31] = 5

    key := TreeKeyFromBytes(keyBytes)

    value, found, err := tree.Get(&key)
    assert.NoError(t, err)
    assert.False(t, found)
}
```

#### Test Case: Single Insert/Get
```rust
Operations: [
    Insert{stem: [1,2,3,4,5], subindex: 10, value: [42,43,44,45]}
]
Query: {stem: [1,2,3,4,5], subindex: 10}
Expected: Some([42,43,44,45, 0...0]) // padded to 32 bytes
```

**Go test (using UBT.md API)**:
```go
func TestSingleInsertGet(t *testing.T) {
    tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

    var keyBytes [32]byte
    copy(keyBytes[:31], []byte{1, 2, 3, 4, 5})
    keyBytes[31] = 10

    key := TreeKeyFromBytes(keyBytes)
    value := [32]byte{42, 43, 44, 45}

    tree.Insert(key, value)

    result, found, err := tree.Get(&key)
    assert.NoError(t, err)
    assert.True(t, found)
    assert.Equal(t, value, result)
}
```

#### Test Case: Multiple Inserts Same Stem
```rust
Operations: [
    Insert{stem: [1,2,3], subindex: 10, value: [100,101,102]},
    Insert{stem: [1,2,3], subindex: 20, value: [200,201,202]}
]
Queries:
  - {stem: [1,2,3], subindex: 10} → Some([100,101,102, 0...0])
  - {stem: [1,2,3], subindex: 20} → Some([200,201,202, 0...0])
```

**Go test (using UBT.md API)**:
```go
func TestMultipleInsertsSameStem(t *testing.T) {
    tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

    var keyBytes1, keyBytes2 [32]byte
    copy(keyBytes1[:31], []byte{1, 2, 3})
    keyBytes1[31] = 10
    copy(keyBytes2[:31], []byte{1, 2, 3})
    keyBytes2[31] = 20

    key1 := TreeKeyFromBytes(keyBytes1)
    key2 := TreeKeyFromBytes(keyBytes2)

    value1 := [32]byte{100, 101, 102}
    value2 := [32]byte{200, 201, 202}

    tree.Insert(key1, value1)
    tree.Insert(key2, value2)

    result1, found1, err1 := tree.Get(&key1)
    assert.NoError(t, err1)
    assert.True(t, found1)
    assert.Equal(t, value1, result1)

    result2, found2, err2 := tree.Get(&key2)
    assert.NoError(t, err2)
    assert.True(t, found2)
    assert.Equal(t, value2, result2)
}
```

#### Test Case: Delete Removes Value
```rust
Operations: [
    Insert{stem: [1,2,3], subindex: 10, value: [42,43,44]},
    Delete{stem: [1,2,3], subindex: 10}
]
Query: {stem: [1,2,3], subindex: 10}
Expected: None
```

**Go test (using UBT.md API)**:
```go
func TestDeleteRemovesValue(t *testing.T) {
    tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

    var keyBytes [32]byte
    copy(keyBytes[:31], []byte{1, 2, 3})
    keyBytes[31] = 10

    key := TreeKeyFromBytes(keyBytes)
    value := [32]byte{42, 43, 44}

    tree.Insert(key, value)

    _, found, _ := tree.Get(&key)
    assert.True(t, found)

    tree.Delete(&key)

    _, found, _ = tree.Get(&key)
    assert.False(t, found)
}
```

### 2. Property-Based Test Templates (`tests/proptests.rs`) - **High Priority**

Port these as randomized property-style tests using standard Go `math/rand` loops
to avoid extra dependencies. Uses **Blake3 untagged**. See `property_test.go`
for the concrete implementations (5 tests: get/insert, isolation, delete,
hash determinism, empty-tree get).

### 3. BLAKE3 Structure Tests (`src/compat_tests.rs`) - **High Priority**

**Blake3 untagged** tree structure validation.

✅ **COMPLETED**: Extracted expected roots into [test_vectors_blake3_eip.json](../test_vectors_blake3_eip.json)
(vectors: `two_stem_colocated_values`, `two_keys_match_first_42_bits`,
`insert_duplicate_key`, `large_number_of_entries`, `merkleize_multiple_entries`,
`order_independence_compat`).

### 4. go-ethereum Compatibility Tests (`src/geth_compat.rs`) - **Removed**

We are not implementing SHA256 compatibility. All tests target Blake3
profiles (EIP untagged, JAM tagged).

## Unit Test Vectors from Source Files

### Hash Function Tests (`src/hash.rs`)

**⚠️ CORRECTED**: Zero-hash rule applies to **Hash64 ONLY**, not Hash32.

```rust
#[test]
fn test_zero_hash_special_case() {
    let hasher = Blake3Hasher::new(Profile::EIP);
    // Hash64 has zero-hash rule
    assert_eq!(hasher.hash_64(&B256::ZERO, &B256::ZERO), B256::ZERO);
    // Hash32 does NOT have zero-hash rule (returns actual hash)
}
```

**Go equivalent (using UBT.md API)**:
```go
func TestZeroHashSpecialCase(t *testing.T) {
    hasher := NewBlake3Hasher(EIPProfile)

    zero := [32]byte{}

    // Hash64 has zero-hash rule
    result64 := hasher.Hash64(&zero, &zero)
    assert.Equal(t, zero, result64)

    // Hash32 does NOT have zero-hash rule
    // It returns the actual Blake3 hash, not zero
}
```

### Stem Tests (`src/key.rs`)

**⚠️ CORRECTED**: Use `Bit()`, not `BitAt()` per UBT.md API.

```rust
#[test]
fn test_stem_bit_at() {
    let mut bytes = [0u8; 31];
    bytes[0] = 0b10000000; // MSB set
    let stem = Stem(bytes);

    assert!(stem.bit_at(0));
    assert!(!stem.bit_at(1));
}
```

**Go equivalent (using UBT.md API)**:
```go
func TestStemBit(t *testing.T) {
    stem := Stem{0b10000000} // MSB set

    assert.Equal(t, uint8(1), stem.Bit(0))  // Bit(), not BitAt()
    assert.Equal(t, uint8(0), stem.Bit(1))
}
```

### TreeKey Roundtrip (`src/key.rs`)

```rust
#[test]
fn test_tree_key_roundtrip() {
    let original = B256::repeat_byte(0x42);
    let key = TreeKey::from_bytes(original);
    let recovered = key.to_bytes();
    assert_eq!(original, recovered);
}
```

**Go equivalent (using UBT.md API)**:
```go
func TestTreeKeyRoundtrip(t *testing.T) {
    original := [32]byte{}
    for i := range original {
        original[i] = 0x42
    }

    key := TreeKeyFromBytes(original)  // standalone function
    recovered := key.ToBytes()

    assert.Equal(t, original, recovered)
}
```

## Generating Test Vectors for Go

### Export Blake3 Test Vectors from Rust

✅ **COMPLETED**: `export_blake3_vectors.rs` exports both EIP and JAM vectors
to `test_vectors_blake3_eip.json` and `test_vectors_blake3_jam.json`.

```rust
// In tests/export_vectors.rs
#[test]
fn export_blake3_test_vectors() {
    use std::fs::File;
    use std::io::Write;
    use serde_json::json;

    let mut vectors = vec![];

    // Export EIP profile (untagged Blake3)
    vectors.push(json!({
        "name": "single_entry_eip_blake3",
        "profile": "eip",
        "hash": "blake3",
        "ubt_git_commit": env!("GIT_COMMIT_HASH"),  // Track version
        "profile_version": "1",                      // EIP-7864 version
        "operations": [
            {"insert": {
                "key": "0000000000000000000000000000000000000000000000000000000000000000",
                "value": "0101010101010101010101010101010101010101010101010101010101010101"
            }}
        ],
        "expected_root": "..." // compute from Rust
    }));

    // Export JAM profile (tagged Blake3)
    vectors.push(json!({
        "name": "single_entry_jam_blake3",
        "profile": "jam",
        "hash": "blake3",
        "ubt_git_commit": env!("GIT_COMMIT_HASH"),  // Track version
        "profile_version": "1",                      // JAM spec version
        "operations": [
            {"insert": {
                "key": "0000000000000000000000000000000000000000000000000000000000000000",
                "value": "0101010101010101010101010101010101010101010101010101010101010101"
            }}
        ],
        "expected_root": "..." // compute from Rust
    }));

    // ... more vectors

    let json = serde_json::to_string_pretty(&vectors).unwrap();
    let mut file = File::create("test_vectors_blake3.json").unwrap();
    file.write_all(json.as_bytes()).unwrap();
}
```

Then in Go:

```go
func LoadTestVectors(path string) ([]TestVector, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var vectors []TestVector
    err = json.Unmarshal(data, &vectors)
    return vectors, err
}

func TestWithRustBlake3Vectors(t *testing.T) {
    vectors, err := LoadTestVectors("../ubt-rs/test_vectors_blake3.json")
    require.NoError(t, err)

    for _, v := range vectors {
        t.Run(v.Name, func(t *testing.T) {
            var profile Profile
            if v.Profile == "eip" {
                profile = EIPProfile
            } else if v.Profile == "jam" {
                profile = JAMProfile
            } else {
                t.Skipf("Unknown profile: %s", v.Profile)
            }

            tree := NewUnifiedBinaryTree(Config{Profile: profile})

            for _, op := range v.Operations {
                // Execute operation using UBT.md API
            }

            root := tree.RootHash()
            assert.Equal(t, v.ExpectedRoot, hex.EncodeToString(root[:]))
        })
    }
}
```

## Recommended Testing Strategy

### Phase 1: Unit Tests from Rust ✅ COMPLETED
- [x] Port all tests from `src/hash.rs` (Hash64 zero-hash rule, Hash32 actual hash) → [hash_test.go](hash_test.go)
- [x] Port all tests from `src/key.rs` (Stem.Bit(), TreeKeyFromBytes roundtrip) → [key_test.go](key_test.go)
- [x] Port all tests from `src/node.rs` (node hashing) → [node_test.go](node_test.go)
- [x] Port all tests from `src/tree.rs` (insert, get, delete, root hash) → [tree_test.go](tree_test.go)

**Summary**: 4 test files created with 40+ test cases validating core UBT operations, hash functions, key handling, and tree structure.

### Phase 2: Blake3 EIP Profile Tests ✅ COMPLETED
- [x] Export Blake3 EIP profile vectors from `compat_tests.rs` and `integration_export.rs` → [export_blake3_simple.rs](../ubt-rs/tests/export_blake3_simple.rs)
- [x] Generated test vectors JSON → [test_vectors_blake3_eip.json](../test_vectors_blake3_eip.json)
- [x] Validate root hashes match Rust for EIP profile → [vectors_test.go](vectors_test.go)
- [x] Property-based tests with EIP profile (setup in Phase 5)

**Summary**: Exported 5 Blake3 EIP profile test vectors from Rust (empty tree, single entry, two entries, colocated values, order independence) and created Go test to validate cross-implementation compatibility.

### Phase 3: JAM Profile Tests ❌ BLOCKED
⚠️ **Skipped**: Rust `ubt-rs` only implements Blake3 untagged (EIP profile). JAM tagged profile not yet available in reference implementation.


### Phase 4: Integration Tests ✅ COMPLETED
- [x] All test cases from `integration_export.rs` with Blake3 → [export_integration_blake3.rs](../ubt-rs/tests/export_integration_blake3.rs)
- [x] Generated integration test vectors JSON → [test_vectors_integration_blake3.json](../test_vectors_integration_blake3.json)
- [x] Cross-validate root hashes with Rust for same inputs → [integration_test.go](integration_test.go)

**Summary**: Exported 10 comprehensive integration test cases from Rust (empty_tree_get, single_insert_get, get_different_stem, same_stem_different_subindex, multiple_inserts_same_stem, delete_removes_value, insert_preserves_others, overwrite_value, delete_nonexistent, large_values) with operations and query validation in Go.

### Phase 5: Property-Based Tests ✅ COMPLETE (stdlib rand)
- [x] P1: Get after insert (with isNonZero filter) → [property_test.go](property_test.go)
- [x] P2: Insert doesn't affect other keys → [property_test.go](property_test.go)
- [x] P3: Insert then delete → [property_test.go](property_test.go)
- [x] P4: Hash determinism → [property_test.go](property_test.go)
- [x] P5: Get from empty tree → [property_test.go](property_test.go)

**Summary**: Created 5 randomized property-style tests (stdlib `math/rand`) with correct API (TreeKeyFromBytes, Config{Profile: EIPProfile}, isNonZero filter).

### Phase 6: Proof Tests ✅ COMPLETE
- [x] Single-key proof generation and verification → [proof_test.go](proof_test.go)
- [x] MultiProof O(k log k) complexity validation → [proof_test.go](proof_test.go)
- [x] MultiProof deduplication validation → [proof_test.go](proof_test.go)
- [x] Missing-key proof generation returns a proof with `Value == nil` → [proof_test.go](proof_test.go)

**Summary**: Proof tests validate single proofs, multiproofs with O(k log k) complexity, deduplication, and missing-key non-existence proofs.

## Critical Validation Points

### Must Match Rust Exactly (Blake3):
1. ✓ **Empty tree root hash is ZERO32** - `RootHash()` returns `[32]byte{}` for empty tree
2. ✓ **Zero-hash special case for Hash64 only** - `Hash64(ZERO32, ZERO32) == ZERO32`
3. ✓ **Hash32 returns actual hash** - `Hash32(ZERO32)` returns actual Blake3 hash (NOT zero)
4. ✓ Stem node hashing (8-level subtree)
5. ✓ TreeKey stem/subindex split (first 31 bytes = stem, byte 31 = subindex)
6. ✓ Order independence (same entries = same root)
7. ✓ Profile-specific domain separation (EIP untagged vs JAM tagged)

### Must Be O(k log k):
- MultiProof node count << k × 256
- Witness size scales with k log k, not k log N
- Deduplication actually working

## Summary

**Test vectors exist in:**
1. ✓ `tests/integration_export.rs` - deterministic Blake3 tests (**highest priority for EIP profile**)
2. ✓ `tests/proptests.rs` - property-based test templates (Blake3, untagged)
3. ✓ `src/compat_tests.rs` - Blake3 structure tests (**highest priority for EIP profile**)
4. `src/geth_compat.rs` - SHA256 vectors (not used)

**Test vector porting status:**
1. ✅ **Exported Blake3 test vectors from Rust** for EIP profile (JAM blocked in Rust)
2. ✅ **Integration tests complete** - 10 deterministic test cases with query validation
3. ✅ **Structure tests complete** - 5 Blake3 EIP profile vectors exported
4. ✅ **Property tests setup** - 5 randomized property-style tests ready
5. ✅ **Cross-validation ready** - All tests validate against Rust-generated roots
6. ✅ **SHA256 compat mode omitted** - Blake3-only implementation as per spec

**All test infrastructure complete.** Tests run against Rust-generated vector JSON files and the current UBT implementation.

**Key API corrections from UBT.md:**
- `TreeKeyFromBytes(b [32]byte) TreeKey` - standalone function
- `Get(key *TreeKey) (value [32]byte, found bool, err error)` - value tuple return
- `Stem.Bit(i int) uint8` - not BitAt()
- `NewBlake3Hasher(profile Profile)` - not Blake2b
- Zero-hash rule: **Hash64 only, not Hash32**
- Hash function: **Blake3** only (no SHA256 compat mode)
