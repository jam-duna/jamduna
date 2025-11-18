# JAM Epoch Verkle Index for Service-Aware Light Clients

This document specifies how a JAM service maintains an epoch-wide Verkle index over logs, how it is updated incrementally per block, and how light clients use it to know which blocks and DA chunks to download.  This will replace logs bloom which is so attackable/flooded that it might as well be [zeroed out](https://ethereum-magicians.org/t/eip-7668-remove-bloom-filters/19447).

**Prototype:** See [`services/evm/src/verkle.rs`](../src/verkle.rs)

The design is:

- **SNARK-free** (no prover infra).
- **Consensus-enforced** (no false negatives allowed).
- **Verkle-based** (small proofs, efficient updates).
- **Tailored to an EVM-style log service**, but extensible.

---

## 1. High-Level Goals

For a given JAM service:

**A user's light client should know which blocks in an epoch contain events relevant to them**, using only:
- A small number of Verkle proofs per epoch,
- **Selective DA downloads** (down to DA chunk / offset, not whole blocks).

**The service must maintain an authenticated index of its events:**
- Built incrementally as blocks are processed,
- Committed as a single Verkle root per epoch,
- Enforced by JAM consensus (no false negatives).

---

## 2. Epoch-Level Verkle Index

JAM has epochs:

> Let epoch `E` span blocks `B₀, B₁, …, Bₖ`.

For each epoch `E`, the service maintains a **single Verkle tree**:

```
T_E : index_key → value
```

- `T_E` is initially empty at the beginning of `E`.
- It is mutated once per block as logs are processed.
- At the end of the epoch, we have a final Verkle root:

```
root_E_final = root(T_E after block Bₖ)
```

`root_E_final` is committed into JAM state/CoreEVM as the **epoch index root**.

### Epoch Boundary Semantics

- **Epoch `E`'s index covers exactly** the logs from blocks whose `epoch_id == E`.
- Logs in the last block of epoch `E` are fully included in `T_E`.
- Logs in the first block of epoch `E+1` belong to `T_{E+1}`.
- Cross-epoch queries (e.g., "spans two epochs") are handled at the application level by querying both epochs.
- No lookahead / lookbehind across epochs is required at the indexing layer.

## 3. Core Types

See [`verkle.rs`](../src/verkle.rs) for full implementation.

### 3.1 LogId - Compact 128-bit Pointer

**Type:** [`LogId`](../src/verkle.rs#L77-L168)

```rust
pub struct LogId(u128);
```

**Bit layout:**
```
[ 16 bits epoch_local_block ]   // up to 65,535 blocks per epoch (600)
[ 16 bits tx_index ]            // up to 65,535 txs per block (need more)
[ 16 bits log_index ]           // up to 65,535 logs per tx
[ 16 bits da_chunk_id ]         // up to 65,535 DA chunks in epoch
[ 32 bits offset_in_chunk ]     // byte offset within DA chunk
```

**Key methods:**
- `LogId::new(block_in_epoch, tx_index, log_index, da_chunk_id, offset_in_chunk) -> Result<Self, LogIdError>`
- `log_id.block_in_epoch() -> u16`
- `log_id.da_chunk_id() -> u16`
- `log_id.offset_in_chunk() -> u32`

**Purpose:** Enables light clients to download **only specific DA chunks**, not full blocks.

### 3.2 IndexKey - 32-byte Hash

**Type:** [`IndexKey`](../src/verkle.rs#L28)

```rust
pub type IndexKey = [u8; 32];
```

Derived via `blake2b(key_type || data)`.

### 3.3 Index Key Types

**Constants:** [`key_types`](../src/verkle.rs#L35-L41)

```rust
pub const CONTRACT_KEY: u8 = 0x01;      // Find logs from specific contract
pub const EVENT_TOPIC0_KEY: u8 = 0x02;  // Find logs of specific event type
pub const INDEXED_ADDR_KEY: u8 = 0x03;  // Find logs with address in indexed params
pub const INDEXED_UINT_KEY: u8 = 0x04;  // Find logs with uint256 in indexed params
pub const INDEXED_BYTES32_KEY: u8 = 0x05; // Find logs with bytes32 in indexed params
```

**Key derivation:** [`derive_index_keys_from_log()`](../src/verkle.rs#L438-L463)

For each log, generates:
1. Contract address key: `blake2b(0x01 || address)`
2. Event topic0 key: `blake2b(0x02 || topic0)` (if present)
3. Indexed param keys: `blake2b(0x03 || position || topic)` for topic1/2/3

**User key derivation:** [`derive_user_index_keys()`](../src/verkle.rs#L465-L500)

Light clients use this to build their query key set.

### 3.4 IndexValue - Fixed-Size Leaf Value

**Type:** [`IndexValue`](../src/verkle.rs#L176-L183)

```rust
pub struct IndexValue {
    pub total_count: u64,           // total logs for this key in epoch
    pub pointer_list_root: [u8; 32], // Merkle root of LogId list (in DA/side-tree TBD)
}
```

**Why fixed-size?** Popular keys (e.g., USDC Transfer) can have millions of logs per epoch. Verkle leaves store only this small summary; actual pointer lists are in DA.

---

## 4. VerkleTree Trait

**Trait:** [`VerkleTree`](../src/verkle.rs#L201-L229)

```rust
pub trait VerkleTree {
    fn new() -> Self;
    fn upsert(&mut self, key: IndexKey, value: IndexValue);
    fn get(&self, key: &IndexKey) -> Option<IndexValue>;
    fn commit(&self) -> VerkleRoot;
    fn prove_batch(&self, keys: &[IndexKey]) -> VerkleBatchProof;
    fn verify_batch(
        root: VerkleRoot,
        keys: &[IndexKey],
        values: &BTreeMap<IndexKey, IndexValue>,
        proof: &VerkleBatchProof,
    ) -> bool;
}
```

### 4.1 SimpleVerkleTree (Prototype)

**Implementation:** [`SimpleVerkleTree`](../src/verkle.rs#L236-L331)

- Uses `BTreeMap<IndexKey, Vec<LogId>>` for in-memory storage
- Deterministic iteration order
- Hash-based commitment (for testing)
- **Use for:** Prototyping, testing, initial deployment
- **Replace with:** Real Verkle (256-way, KZG/BLS12-381) for production

### 4.2 Production Requirements

See [§12](#12-production-verkle-requirements) for full specs.

Must have:
- **256-way branching** (like Ethereum Verkle)
- **KZG commitments** over BLS12-381
- **Batch proofs** (128+ keys, 0.5-2 KB proof size)
- **Update batching** API
- **Deterministic** commitment

---

## 5. Per-Block Processing

**Function:** [`process_block_for_epoch_index()`](../src/verkle.rs#L504-L527)

```rust
pub fn process_block_for_epoch_index<T: VerkleTree>(
    tree: &mut SimpleVerkleTree,
    epoch_start_block: u64,
    block_number: u64,
    logs_by_tx: &[Vec<Log>],
    da_chunk_id: u16,
    offset_fn: impl Fn(usize, usize) -> u32,
) -> VerkleRoot
```

**For each block in epoch:**
1. Extract all logs from transactions
2. For each log:
   - Derive index keys (contract, topic0, indexed params)
   - Construct `LogId` with DA chunk granularity
   - Insert into epoch index: `tree.insert_log_ids(key, &[log_id])`
3. Return updated epoch root: `tree.commit()`

**Complexity:** `O(#logs × #keys_per_log × log_depth)` - very manageable.

---

## 6. Light Client Protocol

### 6.1 User Index Keys

```rust
let my_keys: Vec<IndexKey> = derive_user_index_keys(
    &[usdc_contract, dai_contract],           // contracts to track
    &["Transfer(address,address,uint256)"],   // event signatures
    my_wallet_address,                        // my address
);
```

### 6.2 Streaming Mode (During Epoch)

**Incremental updates every ~100 blocks:**

```rust
pub struct IncrementalUpdate {
    pub epoch_id: u32,
    pub from_block: u16,
    pub to_block: u16,
    pub partial_root: VerkleRoot,  // epoch index root after to_block
    pub new_log_ids: BTreeMap<IndexKey, Vec<LogId>>,
    pub proof: VerkleBatchProof,
}
```

Light client:
1. Receives incremental update from relayer
2. Verifies Verkle proof against `partial_root`
3. Accumulates `LogId`s for their keys
4. Downloads DA chunks as needed (using `da_chunk_id` from `LogId`)

**Latency:** Bounded to ~10 minutes worst-case (100 blocks × 6s)

### 6.3 Batch Mode (After Epoch Finalization)

```rust
QueryEpochKeys(E, my_keys[]) → {
    key → IndexValue,          // from Verkle (total_count, pointer_list_root)
    key → Vec<LogId>,          // from DA
    aggregated_verkle_proof    // proves all IndexValues
}
```

Light client:
1. Requests all keys at once after epoch ends
2. Verifies single aggregated Verkle proof
3. Verifies each pointer list against its `pointer_list_root`
4. Downloads only DA chunks containing relevant logs
5. Reconstructs full log data

**Efficiency:** One proof per epoch for all keys.

---

## 7. Security & Guarantees

### Consensus-Enforced Correctness

- Full nodes deterministically construct epoch index from logs
- Incorrect index (missing `LogId`) ⇒ invalid block
- Under JAM consensus: **no false negatives**

### Light Client Trust

- **During epoch:** Streaming proofs provide "soft" guarantees
- **At epoch end:** Single aggregated proof provides **hard guarantee** of completeness

### DoS / Spam Protection

**Index Fee Model:**

```rust
const BASE_LOG_FEE: u64 = 1000;
const INDEX_KEY_FEE: u64 = 500;
const POINTER_STORAGE_FEE: u64 = 10;
const POPULAR_KEY_MULTIPLIER: u64 = 10;  // surcharge for >1M logs/epoch

fn compute_log_indexing_fee(log: &Log, epoch_key_counts: &BTreeMap<IndexKey, u64>) -> u64;
```

**Fee principles:**
1. Base cost prevents spam
2. Per-key cost scales with index overhead
3. Storage cost accounts for pointer list growth
4. Popular key surcharge discourages global indexing (use contract-specific keys)
5. Fees are consensus-critical

---

## 8. Implementation Phases

### Phase 1: Prototype (Current)

✅ **Status:** Implemented in [`verkle.rs`](../src/verkle.rs)

- `SimpleVerkleTree` with `BTreeMap` storage
- `LogId` with DA granularity
- Index key derivation functions
- Hash-based commitment
- 5 passing tests

**Use for:**
- Development and testing
- Initial deployment
- Proof of concept

### Phase 2: Production

**TODO:** Integrate real Verkle tree library

Requirements:
- 256-way branching
- KZG commitments over BLS12-381
- Batch proof generation/verification
- External pointer list storage (DA or side-tree)
- `IndexValue` in leaves (not `Vec<LogId>`)

**Candidate Libraries (as of 2024-2025):**

1. **[crate-crypto/rust-verkle](https://github.com/crate-crypto/rust-verkle)** ⚠️ Research
   - **Status:** Proof-of-concept, not production-ready
   - **Pros:** More verbose, Rust-idiomatic, uses BTreeMap
   - **Cons:** No benchmarks, no parallelism, explicitly marked unsafe for production
   - **Warning:** "This code has not been reviewed and is not safe to use in non-research capacities"
   - **License:** MIT/Apache-2.0

2. **[CleanPegasus/verkle-tree](https://github.com/CleanPegasus/verkle-tree)** ⚠️ Experimental
   - **Status:** v0.1.0 (July 2024), unstable
   - **Pros:** Uses arkworks BLS12-381, KZG commitments, simple API
   - **Cons:** No multiproof support, no storage, no benchmarks, early stage
   - **API:**
     ```rust
     let tree = VerkleTree::new(&data, width);
     let proof = tree.generate_proof(index, &data);
     VerkleTree::verify_proof(root, &proof, width);
     ```
   - **TODO:** Multiproof, storage, benchmarks, Solidity verifier
   - **License:** MIT

3. **[InternetMaximalism/verkle-tree](https://github.com/InternetMaximalism/verkle-tree)** 🛑 Archived
   - **Status:** Archived March 2023
   - **Pros:** PlonK verification support, inclusion/exclusion proofs
   - **Cons:** Uses alt-BabyJubjub (not BLS12-381), archived, nightly Rust only
   - **Note:** Designed for PlonK circuits, not standard KZG/BLS12-381

**Reference Implementation:**
- **[gballet/go-verkle](https://github.com/gballet/go-verkle)** (Golang)
  - Active development through 2024 (Ethereum statelessness)
  - Integrated into go-ethereum
  - Production-quality but not Rust

**Recommendation:**

For Phase 2, consider:
1. **Short-term:** Fork and extend `CleanPegasus/verkle-tree` to add:
   - Multiproof support (critical for batch queries)
   - 256-way branching (currently configurable width)
   - Production-grade testing and benchmarks

2. **Long-term:** Port critical parts of `gballet/go-verkle` to Rust or
   collaborate with Ethereum Verkle teams on a canonical Rust implementation

**Migration Path:**
1. Replace `SimpleVerkleTree` with chosen library
2. Implement `VerkleTree` trait adapter for new backend
3. Move pointer lists to DA storage
4. Update leaf values to `IndexValue`
5. Add comprehensive tests and benchmarks
6. Maintain same external API via trait abstraction

---

## 9. Key Design Decisions

### Problem 1: Pointer List Growth ✅

**Solution:** `IndexValue` (40 bytes) in Verkle leaves; pointer lists in DA

- Popular keys (USDC) can have 1M+ logs/epoch
- `Vec<LogId>` would be 16+ MB
- Verkle leaves remain small regardless of log count

### Problem 2: Pointer Redundancy ✅

**Solution:** Compact `LogId` (16 bytes) + selective indexing

- One reference per (log, key) pair is inherent to inverted indices
- Make `EVENT_TOPIC0_KEY` opt-in (not default)
- Separate high-volume keys into dedicated services

### Problem 3: DA Granularity ✅

**Solution:** `LogId` contains `da_chunk_id` + `offset_in_chunk`

- Light clients download only relevant DA chunks
- No need to fetch full blocks

### Problem 4: Epoch Boundaries ✅

**Solution:** Explicit epoch scoping

- Index `T_E` covers only logs with `epoch_id == E`
- Cross-epoch queries handled at application level

### Problem 5: Verkle Implementation Gap ✅

**Solution:** Trait abstraction + phased rollout

- `VerkleTree` trait allows swapping backends
- `SimpleVerkleTree` for prototyping
- Production impl for final deployment

---

## 10. Determinism Requirements

**Critical for consensus:**

1. **Index key derivation:** Deterministic hashing (blake2b, keccak256), no randomness
2. **LogId ordering:** Insertion order (block → tx → log), no sorting
3. **DA chunk assignment:** Deterministic from transaction data, not runtime state
4. **No floating-point:** All arithmetic is integer-based
5. **Verkle insertion order independence:** Production Verkle trees are commutative
6. **Pointer list serialization:** Canonical format `[count:u32][id0:u128][id1:u128]...`

**Validation:**

```rust
fn validate_epoch_index_root(
    epoch_id: u32,
    computed_root: VerkleRoot,
    expected_root: VerkleRoot,
) -> Result<(), ConsensusError> {
    if computed_root != expected_root {
        return Err(ConsensusError::EpochIndexRootMismatch {
            epoch_id,
            computed,
            expected,
        });
    }
    Ok(())
}
```

---

## 11. Example Usage

### Indexer (Full Node)

```rust
use evm::verkle::{SimpleVerkleTree, VerkleTree, process_block_for_epoch_index};

// Epoch start
let mut epoch_tree = SimpleVerkleTree::new();
let epoch_id = 42;
let epoch_start_block = 1000;

// Per block
for block_num in epoch_start_block..epoch_end_block {
    let logs_by_tx = get_block_logs(block_num);
    let da_chunk_id = compute_da_chunk_id(block_num);

    let epoch_root = process_block_for_epoch_index(
        &mut epoch_tree,
        epoch_start_block,
        block_num,
        &logs_by_tx,
        da_chunk_id,
        |tx_idx, log_idx| compute_offset(tx_idx, log_idx),
    );

    // Include epoch_root in block's JAM state
    commit_state_root(block_num, epoch_root);
}

// Epoch end
let root_final = epoch_tree.commit();
publish_epoch_index_root(epoch_id, root_final);
```

### Light Client

```rust
use evm::verkle::{IndexKey, derive_user_index_keys};

// Setup
let my_keys = derive_user_index_keys(
    &[usdc_addr, dai_addr],
    &["Transfer(address,address,uint256)"],
    my_wallet,
);

// Query after epoch finalization
let response = query_epoch_keys(epoch_id, &my_keys);

// Verify proof
assert!(VerkleTree::verify_batch(
    response.root_final,
    &my_keys,
    &response.index_values,
    &response.proof,
));

// Download DA chunks
for (key, log_ids) in response.log_ids {
    for log_id in log_ids {
        let chunk = fetch_da_chunk(log_id.da_chunk_id());
        let log = parse_log_at_offset(chunk, log_id.offset_in_chunk());
        process_log(log);
    }
}
```

---

## 12. Production Verkle Requirements

For consensus-critical deployment, the Verkle backend **MUST** satisfy:

### Branching Factor
- **256-way branching** (matching Ethereum's Verkle design)
- Minimizes depth (~2-5 levels) and proof size

### Commitment Scheme
- **KZG commitments** over pairing-friendly curve
- **Recommended:** BLS12-381 (matches Ethereum, broad library support)
- **Alternative:** IPA-based Verkle (less standard)

### Proof Aggregation
- **Batch proofs:** Support 128+ keys per proof
- **Proof size:** 0.5-2 KB for typical batches
- **API:**
  ```rust
  fn prove_batch(keys: &[IndexKey]) -> VerkleBatchProof;
  fn verify_batch(root, keys, values, proof) -> bool;
  ```

### Update Batching
- Support applying multiple leaf updates in one batch:
  ```rust
  fn apply_updates(updates: &[(IndexKey, IndexValue)]) -> VerkleRoot;
  ```
- Efficiently recomputes only affected internal nodes
- Aggregates polynomial commitment updates

### Libraries
- **Rust:** `verkle-trie` (experimental), `arkworks` (KZG primitives)
- **Reference:** Ethereum's `go-verkle`, `rust-verkle`

---

## 13. References

- **Implementation:** [`services/evm/src/verkle.rs`](../src/verkle.rs)
- **Log format:** [`services/evm/src/receipt.rs`](../src/receipt.rs)
- **Tests:** [`verkle.rs:506-602`](../src/verkle.rs#L506-L602)
- **Ethereum Verkle:** [EIP-6800](https://eips.ethereum.org/EIPS/eip-6800)
- **KZG Commitments:** [Dankrad Feist's blog](https://dankradfeist.de/ethereum/2020/06/16/kate-polynomial-commitments.html)
