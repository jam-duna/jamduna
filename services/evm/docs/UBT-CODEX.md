# UBT Proof Integration Plan

This document consolidates the UBT integration plan for the JAM EVM service. It replaces Verkle proofs with Unified Binary Tree (UBT) proofs while preserving the builder/guarantor security model.

Sources reviewed:
- `services/evm/docs/BACKEND.md`
- `services/evm/docs/BAL.md`
- `services/evm/docs/UBT-CLAUDE.md`

---

## Overview

UBT replaces Verkle proofs with hash-based multiproofs that are:
- Post-quantum (hash-based).
- Simpler (no IPA/SRS).
- Efficient for batching (O(k log k) multiproofs).
- Compatible with JAM storage patterns and BAL workflows.

The builder still provides pre-state and post-state witnesses; the guarantor still verifies both and re-executes from cache-only state.

---

## Legacy Verkle Integration (Removed)

Verkle witness plumbing has been removed in favor of UBT multiproofs.

---

## Target UBT Architecture

### Key Principles
- Same builder/guarantor contract: pre-state proof binds reads to `pre_state_root`, post-state proof binds writes to `post_state_root` (full-state roots).
- UBT multiproofs: compact, deduplicated witness (`Nodes` + `NodeRefs`) with canonical ordering.
- Extension proofs: missing-stem non-existence uses `ExtensionProofNode` commitments encoded alongside the multiproof (key, stem, stem_hash, divergence_depth, 248 siblings).
- Profile consistency: JAMProfile only (tagged Blake3) for builder + guarantor.

### Data Flow (UBT)

**Builder**
1. EVM runtime uses `host_fetch_ubt(...)` for cache misses (replacing Verkle host functions).
2. Read log becomes **UBTReadLog** and preserves BAL inputs.
3. Go storage builds two UBT multiproofs from read/write logs:
   - `pre_ubt_proof` for read keys (pre witness root).
   - `post_ubt_proof` for write keys (post witness root).
4. `BuildBundle()` prepends **two extrinsics** (pre, post) instead of a single Verkle witness.

**Guarantor**
1. Detect `UBTWitnessPre` and `UBTWitnessPost` extrinsics.
2. Verify each witness section (UBT multiproof + root).
3. Pre/post values populate caches.
4. Execute block in `ExecutionMode::Guarantor`; cache miss is a hard failure.

---

## Witness Format (UBT)

Two extrinsics in `BuildBundle`:

```
Extrinsic 0: UBTWitnessPre  = { pre_witness_root, entries, multiproof_pre, extensions_pre }
Extrinsic 1: UBTWitnessPost = { post_witness_root, entries, multiproof_post, extensions_post }
```

Each extrinsic is a single witness section (root + entries + multiproof + extension proofs).

Each multiproof uses canonical UBT encoding:
- `Keys` (CompactTreeKey list, sorted by expanded key)
- `Values` (Option<[32]byte>, supports non-existence within existing stems)
- `Nodes` + `NodeRefs` (deduped nodes + traversal order)
- `Stems` (deduped stems, strictly sorted)

**Implementation note**: Canonical ordering is enforced in verification, so non-canonical witnesses are rejected.

### Read/Write Entry Payloads (for BAL)
Witness entries preserve deterministic BAL inputs: tree_key, key_type, address, extra, storage_key, pre_value, post_value, tx_index.
Non-existence is represented by zero values in witness entries; the read log itself does not carry a presence flag.

---

## Host Functions + Read Log

Replace Verkle host functions and logs with UBT equivalents:
- `host_fetch_ubt(FETCH_BALANCE/NONCE/CODE/CODEHASH/STORAGE)`
- Rust verifies UBT multiproofs directly (no host `verify_ubt_proof` shim).

Read log format should preserve:
- Tree key (derived via UBT key derivation)
- Key metadata (key_type, address, extra, storage_key)
- Tx index (first-touch ordering for BAL)

---

## Implementation Plan

### Phase 0: Types + Interfaces (done)
- UBT witness extrinsics are emitted as raw bytes; no new bundle type needed yet.

### Phase 1: Host Functions + Read Log (done)
- Replaced read/write logs with `ubtReadLog` / `ubtWriteLog`.
- `host_fetch_ubt` is the single read path (no proof_type or back-compat).

### Phase 2: Builder Proof Generation (done)
- `BuildUBTWitness()` in Go storage (dual pre/post sections).
- `BuildBundle()` prepends two extrinsics (pre first, post second) and keeps BAL split logic aligned.

### Phase 3: Guarantor Verification (done)
- Parse the two witness extrinsics (pre/post).
- Verify UBT multiproofs using JAMProfile hashing (tagged Blake3).
- Load caches from pre-witness entries and compute BAL from UBT entries.

### Phase 4: Cleanup + Config Caching (Phase 7 fused) (done)
- Removed remaining Verkle plumbing (`verkleReadLog`, `BuildVerkleWitness`, `verify_verkle_proof`) and Rust Verkle modules.
- Updated docs: `BACKEND.md`, `BAL.md`, `BLOCKS.md` to be UBT-only.
- Checkpoint tree UBT equivalent: `CheckpointTreeManager` now caches UBT trees for historical lookups and replay.

### Phase 5: Tests + Fixtures (done)
- Unit tests:
  - Multiproof encoding/decoding for UBT witnesses (`ubt::tests::test_parse_multiproof_round_trip`).
  - Rust verification of pre/post multiproofs (`ubt::tests::test_verify_witness_section_pre_post_single_key`).
  - Cache-only guarantor mode with UBT witnesses (`ubt::tests::test_entries_to_caches_with_basic_data_and_storage`).
- Integration coverage:
  - UBT witness serialization/verification exercised in-crate (replaces Verkle fixtures for now).

### Phase 6: Optional Transition Witness (Post-Parity Optimization)
- Consider a single “transition” UBT proof that binds pre/post values in one proof.
- This is not required for parity; treat as an optimization phase after dual-proof stability.
- Optional: cache UBT config/preimages at genesis with `historical_lookup`.
- Optional: typed bundle wrappers for UBT witness extrinsics (from Phases 0-2).

**Goal**: Replace the two witness extrinsics (pre + post) with one transition witness that
verifies both `pre_state_root` and `post_state_root` while deduplicating shared structure.

**Core idea**: Build two UBT multiproofs (pre, post) over the same ordered key set, then
merge their shared tables (stems + nodes) and keep two node-ref streams. Verification
reuses the same tables but applies different node-ref streams and value vectors.

**Suggested TransitionWitness layout (conceptual):**
```
TransitionWitness {
  pre_root:  [32]byte,
  post_root: [32]byte,
  keys:      [CompactTreeKey],       // sorted by expanded key
  pre_vals:  [Option<[32]byte>],
  post_vals: [Option<[32]byte>],
  stems:     [31]byte,               // deduped stems table
  nodes:     [32]byte,               // deduped sibling hashes
  node_refs_pre:  [u32],             // traversal order for pre-root
  node_refs_post: [u32],             // traversal order for post-root
}
```

**Builder flow**:
1. Collect the union key set from read/write logs; sort by expanded key.
2. Build standard UBT multiproof for `pre_root` using `pre_vals`.
3. Build standard UBT multiproof for `post_root` using `post_vals`.
4. Dedup `stems` and `nodes` across the two proofs; remap `node_refs_pre` and
   `node_refs_post` to the shared tables.
5. Emit a single TransitionWitness extrinsic carrying both roots and vectors.

**Verifier flow**:
1. Validate canonical ordering for `keys`, `stems`, and table lengths.
2. Verify `pre_root` using the shared tables with `node_refs_pre` + `pre_vals`.
3. Verify `post_root` using the shared tables with `node_refs_post` + `post_vals`.
4. Populate caches from `pre_vals` and use `post_vals` to check writes.

**Non-existence handling**:
- Keep `Option<[32]byte>` for missing subindices.
- Missing stems are covered by extension proofs appended to each witness section.
- If a single transition witness is introduced later, extension proofs must be
  carried per-root (or explicitly deduped with a canonical mapping).

**Compression knobs**:
- Omit `post_vals` entries when unchanged (`post == pre`) and include a bitmask
  to indicate which entries are present.
- Dedup identical nodes via a shared `nodes` table (already required for multiproof).

This keeps the cryptographic model unchanged (two root verifications) while
reducing witness size and extrinsic count.

---

### Review Triage (UBT Proof Findings)

**Already fixed / by design**
- **Empty-tree non-existence proofs**: `Proof.Verify` now accepts `Path == nil` when `Value == nil` and `expectedRoot == ZERO32`; UBT.md documents the empty-tree exception.
- **Invalid internal-node direction values**: `Proof.Verify` rejects any `Direction` that is not `Left` or `Right`.
- **Extension proofs with values**: `Proof.Verify` rejects any `ExtensionProofNode` when `p.Value != nil`.
- **Stem-path binding + internal-node count**: `Proof.Verify` enforces 248 internal nodes and validates `Direction` against the stem bit at each depth.
- **Storage slot prefix overflow**: `GetStorageSlotKey` now increments `slot[0]` modulo 256 (adds `2^248`) and tests cover wrap behavior.
- **Duplicate keys in multiproofs**: by design, multiproof generation canonicalizes (sort + dedup). Witness generation already dedups reads (`collectUBTReadKeys`), so duplicates are intentionally collapsed.
- **Missing stems in multiproofs**: multiproofs still cover existing stems only; missing stems are now covered via extension proofs attached to the witness section.
- **Prefix-bits validation in persistence**: `IterByPrefix` and `GetSubtreeHash` reject prefixBits outside `[0, 248]`.

**Fix plan (remaining hardening candidates)**
- Add explicit tests that cover internal-node truncation and direction mismatch cases for proof verification.

**Missing-stem handling (witness generation)**
- Missing-stem reads are emitted as extension proofs (non-existence) alongside the multiproof.
- Avoid inserting empty stems just to satisfy multiproof generation.

---

## Architectural Notes for Implementers

- Key derivation: use `storage/ubt_embedding.go` so tree keys match JAM storage layout.
- Read/write coverage: every EVM read must log to `UBTReadLog`; every write must log to `UBTWriteLog`.
- Non-existence proofs: UBT supports missing subindices (value = nil) and missing stems (ExtensionProofNode); witness generation emits extension proofs for missing stems.
- Two-proof model: keep pre/post proofs for security parity.
- BAL continuity: keep witness payloads rich enough to rebuild BAL deterministically (tx_index ordering and metadata).

### Witness Verification Model (Builder + Guarantor)

- Each witness extrinsic is self-contained: its first 32 bytes are the root used to verify the proof in that same extrinsic.
- **Pre witness root** = UBT root before execution; **Post witness root** = UBT root after applying the contract writes.
- Guarantor verification does **not** compare the witness root against `refine_context.state_root`; it verifies the multiproof + extension proofs against the root embedded in the witness itself.
- Builder Go path verifies both witnesses immediately after generation (against the local pre/post UBT trees) and logs head/tail bytes so Rust guarantor logs can be compared byte-for-byte.

---

## Data Format Reference

### UBT Witness Entry (section payload)
```
[ 32B tree_key ][ 1B key_type ][ 20B address ][ 8B extra ][ 32B storage_key ]
[ 32B pre_value ][ 32B post_value ][ 4B tx_index ]
```

### UBT Witness Section Binary Format
```
[ 32B root ][ 4B count (BE) ][ entries... ][ 4B proof_len (BE) ][ proof_bytes ]
[ 4B extension_count (BE) ][ extension_proofs... ]
```
Each witness extrinsic carries exactly one section in this format (pre and post are separate extrinsics).

### Extension Proof Binary Format (Updated: Optimized)
```
Per extension proof:
[ 31B missing_stem ]      // Changed from 32B key - deduped by stem
[ 31B stem ]              // Divergent stem found in tree
[ 32B stem_hash ]         // Hash of divergent stem node
[ 2B divergence_depth ]   // Bit position where stems diverge (0-247, BE)
[ 31B sibling_bitmap ]    // 248 bits indicating non-zero siblings
[ siblings... ]           // Only non-zero siblings (n × 32B, where n = popcount(bitmap))
```

**Optimizations Applied:**
1. **Stem Deduplication**: Changed from storing full `key` (32B) to `missing_stem` (31B). When multiple keys share the same missing stem, only one extension proof is generated. Verification matches witness entries by stem prefix instead of full key.

2. **Bitmap Compression**: Sibling array compressed using sparse encoding. In typical sparse UBT trees, most of the 248 siblings are `ZERO32` (empty branches). The bitmap indicates which siblings are non-zero, and only those are serialized.

**Size Impact:**
- **Before optimizations**: ~97 extensions × 8,033 bytes = 779,201 bytes
- **After stem dedup**: ~46 extensions × 8,033 bytes = 369,518 bytes (52.6% reduction)
- **After bitmap compression**: ~46 extensions × ~537 bytes = 24,702 bytes (96.8% reduction)

**Example:** For a sparse tree with 46 missing stems and ~18 non-zero siblings per extension:
```
Header:   31 + 31 + 32 + 2 = 96 bytes
Bitmap:   31 bytes
Siblings: 18 × 32 = 576 bytes
Total:    ~703 bytes (vs 8,033 bytes uncompressed)
```

### UBT Multiproof Encoding
```
[ 4B keys_len (LE) ][ 4B values_len (LE) ][ 4B nodes_len (LE) ][ 4B node_refs_len (LE) ][ 4B stems_len (LE) ]
[ keys: stem_index(u32 LE) + subindex(u8) ]
[ values: tag(0/1) + 32B if present ]
[ nodes: 32B each ][ node_refs: u32 LE each ][ stems: 31B each ]
```
If `node_refs_len == 0`, `nodes` must already be deduplicated and emitted in traversal order.

---

## Performance Targets (Measured)

| Aspect | Verkle | UBT (Optimized) | Notes |
|--------|--------|-----------------|------|
| Cryptography | IPA + SRS | Hash-based | No curve/SRS dependency |
| Batch proof size | O(k) | O(k log k) | **97% reduction achieved** via stem dedup + bitmap compression |
| Verification cost | High | Lower | Hash-only, no IPA |
| Post-quantum | No | Yes | Hash-based |

**Actual Measurements (51-transfer test):**
| Metric | Size | Optimizations Applied |
|--------|------|----------------------|
| Pre-state witness | 24,700 bytes | Stem dedup (52.6%) + Bitmap compression (91% per-proof) |
| Post-state witness | 65,218 bytes | Multiproof only (fewer missing stems in post-state) |
| **Total reduction** | **97.1%** | From 845KB → 25KB for pre-state |

**Key Insight:** UBT witness size scales with **unique missing stems**, not total missing keys. Sparse trees (typical in blockchain state) have very few non-zero sibling hashes, enabling aggressive bitmap compression.

---

## Extension Proof Optimization Implementation

### Stem Deduplication (buildExtensionProofs)
**Location:** `storage/ubt_witness.go`

When building extension proofs for missing keys, the builder now:
1. Groups missing keys by their stem (first 31 bytes)
2. Picks one sample key per unique missing stem
3. Generates a single extension proof per stem
4. Stores `missing_stem` (31B) instead of full `key` (32B)

**Code:**
```go
stemSamples := make(map[Stem][32]byte, len(keys))
for _, keyBytes := range keys {
    key := TreeKeyFromBytes(keyBytes)
    if existing, ok := stemSamples[key.Stem]; !ok || bytes.Compare(keyBytes[:], existing[:]) < 0 {
        stemSamples[key.Stem] = keyBytes
    }
}
for stem := range stemSamples {
    stems = append(stems, stem)
}
// Generate one proof per unique stem
for _, stem := range stems {
    sampleKey := TreeKeyFromBytes(stemSamples[stem])
    proof, err := tree.GenerateProof(&sampleKey)
    // ... build extension proof
}
```

### Bitmap Compression (serializeExtensionProofs)
**Location:** `storage/ubt_witness.go`

Extension proofs compress the 248-sibling array using sparse encoding:
1. Create 248-bit bitmap (31 bytes) indicating non-zero siblings
2. Serialize only the non-zero siblings
3. Verifier reconstructs full array from bitmap + compressed siblings

**Code:**
```go
var bitmap [31]byte
nonZeroSiblings := make([][32]byte, 0, 248)

for i, sibling := range ext.siblings {
    if sibling != ([32]byte{}) {
        bitmap[i/8] |= (1 << (7 - i%8))  // Set bit i
        nonZeroSiblings = append(nonZeroSiblings, sibling)
    }
}
// Write bitmap, then only non-zero siblings
```

### Stem-Based Verification (verifyEntriesAgainstProof)
**Locations:** `storage/ubt_witness_verify.go`, `services/evm/src/ubt.rs`

Witness entry verification now matches by stem instead of full key:
1. Extract entry stem from tree key (first 31 bytes)
2. Look up extension proof by `missing_stem` in extension map
3. Mark the entry as covered by the missing-stem proof (prefix validation is enforced in `verifyExtensionProofs`)

**Go Code:**
```go
extensionMap := make(map[Stem]extensionProof, len(section.extensions))
for _, ext := range section.extensions {
    extensionMap[ext.missingStem] = ext
}

for _, entry := range section.entries {
    entryStem := TreeKeyFromBytes(entry.key).Stem
    if _, ok := extensionMap[entryStem]; ok {
        // Entry is covered by the missing-stem extension proof
    }
}
```

**Rust Code:**
```rust
let mut extension_map: BTreeMap<Stem, &ExtensionProof> = BTreeMap::new();
for ext in &section.extensions {
    extension_map.insert(ext.missing_stem, ext);
}

for entry in &section.entries {
    let entry_stem = tree_key_from_bytes(entry.tree_key).stem;
    if extension_map.contains_key(&entry_stem) {
        // Entry is covered by the missing-stem extension proof
    }
}
```

### Bitmap Decompression (deserializeExtensionProofs)
**Locations:** `storage/ubt_witness_verify.go`, `services/evm/src/ubt.rs`

Verifiers reconstruct the full 248-sibling array from compressed format:

**Go Code:**
```go
var bitmap [31]byte
copy(bitmap[:], data[offset:offset+31])
offset += 31

siblings := make([][32]byte, 248)
for i := 0; i < 248; i++ {
    bitSet := (bitmap[i/8] & (1 << (7 - i%8))) != 0
    if bitSet {
        copy(siblings[i][:], data[offset:offset+32])
        offset += 32
    }
    // else sibling remains zero
}
```

**Rust Code:**
```rust
let bitmap = read_array_31(data, &mut offset)?;

let mut siblings = Vec::with_capacity(248);
for i in 0..248 {
    let bit_set = (bitmap[i / 8] & (1 << (7 - i % 8))) != 0;
    if bit_set {
        siblings.push(read_array_32(data, &mut offset)?);
    } else {
        siblings.push(ZERO32);
    }
}
```

---

## Known Issues and Bugs

### Critical Issues

#### 1. Post-state witness root is not verified against expected post-root ✅ FIXED (basic binding)
**Location**: `services/evm/src/refiner.rs`

**Fix**: The guarantor now validates both pre and post witnesses against the roots declared inside each witness section. This enforces internal consistency of each witness. Full binding to an independently computed state root remains a follow-up once an authoritative root is available in refine.

---

#### 2. Witness roots computed from partial tree, not full state root ✅ FIXED
**Location**: `storage/ubt_witness.go`

**Fix**: Witness roots are now derived from the full pre/post UBT trees, and multiproofs are generated against those full trees. This restores binding to full-state roots while still proving only the touched keys.

---

### High-Severity Issues

#### 3. Missing-stem reads are silently dropped from pre-witness ✅ FIXED
**Location**: `storage/ubt_witness.go`

**Fix**: Missing-stem reads now emit extension proofs (non-existence) alongside the multiproof, so read entries remain complete and verifiable.

---

#### 4. Post-state deletions represented as explicit zero values (potential root divergence) ✅ FIXED
**Location**: `storage/ubt_witness.go`

**Fix**: Post-state deletions now remain absent in the witness tree (no forced `present=true` + zero value), so post roots match deletion semantics and proofs validate correctly.

---

### Documentation Gaps

#### 5. Extension proof format not specified ✅ FIXED + OPTIMIZED
Extension proofs are now specified and implemented as an optimized format appended to each witness section:
- `missing_stem` (31B) - **Changed from `key` (32B)** - enables deduplication by stem
- `stem` (31B) - divergent stem found in tree
- `stem_hash` (32B) - hash of divergent stem node
- `divergence_depth` (u16, BE) - bit position where stems diverge
- `sibling_bitmap` (31B) - **New: 248-bit bitmap** indicating non-zero siblings
- `siblings` (variable) - **Compressed: only non-zero siblings** instead of full 248×32B array

**Optimizations:**
1. **Stem Deduplication (52.6% reduction)**: Multiple keys sharing the same missing stem now use a single extension proof. Verification matches entries by stem prefix.
2. **Bitmap Compression (91% per-proof)**: Exploits sparsity - most siblings are `ZERO32` in typical trees. Only non-zero siblings are serialized.

**Combined Impact:** 845KB → 25KB (97% reduction) for pre-state witness in 51-transfer test.

Guarantor verification checks:
- unique missing stems (not full keys)
- divergence-depth prefix match between `missing_stem` and `stem`
- bitmap-driven sibling reconstruction to 248 entries
- root reconstruction against the witness root using bitmap-decompressed siblings

---

#### 6. Witness root vs state root distinction unclear ✅ FIXED
Witness roots now match full pre/post state roots; no special “execution root” distinction remains.

---


## Deliverables Checklist

- [x] `host_fetch_ubt` in `statedb/hostfunctions.go`
- [x] `UBTReadLog` + `UBTWriteLog` in storage layer
- [x] `BuildUBTWitness()` in Go storage
- [x] BuildBundle updated to emit two UBT witness extrinsics
- [x] Rust verifier: UBT multiproof verification for pre/post
- [x] Guarantor backend loads caches from UBT witness entries
- [x] Update docs + tests (remove Verkle-specific fixtures)
- [ ] Optional typed wrappers for `UBTWitnessPre` / `UBTWitnessPost` extrinsics
- [ ] Optional: single transition witness (post-parity optimization)
