# Proofs of Inclusion in JAM-EVM

## 1. Design Intent

We want light clients (and bridges) to convince themselves that a transaction, receipt, or storage item really ran inside the JAM EVM service. JAM consensus only publishes a single `stateRoot`, so every proof must reduce to "here is an `ObjectRef` stored in JAM state, and here is the DA payload it points to (if the payload is still within the DA retention window)".

**Guiding principles**

1. **One root only.** The authoritative commitment is JAM's `stateRoot`. Binary Merkle Trie (BMT) proofs against that root are sufficient for inclusion.
2. **DA is temporary.** Transaction and receipt payloads are exported to Data Availability (DA) as part of refine. They are guaranteed to remain retrievable for the DA-window (currently 28 days). After that they must be pinned by archival helpers.
3. **Ethereum semantics matter.** RPC consumers still expect `blockHash`, `blockNumber`, `transactionIndex`, `gasUsed`, `logsBloom`, etc. We keep enough metadata in JAM state to answer those fields without bespoke storage.

## 2. Where the Commitments Live

| Commitment            | Location                               | What it covers                                              |
|-----------------------|----------------------------------------|-------------------------------------------------------------|
| `stateRoot`           | JAM block header                       | Meta-shard ObjectRefs + block number/work package mappings  |
| `transactionRoot`     | Block payload in DA                    | BMT root of transaction hashes                              |
| `receiptRoot`         | Block payload in DA                    | BMT root of canonical receipt hashes                        |
| Transaction payloads  | DA exported by refine                  | RLP/typed transaction bodies                                |
| Receipt payloads      | DA exported by refine                  | Receipt fields: status, cumulative gas, logs, bloom inputs  |
| Block number index    | JAM State (accumulate)                 | blocknumber → work_package_hash + timeslot                  |
| Block payload         | DA exported by accumulate              | EvmBlockPayload: header + tx/receipt hashes                 |
| Block metadata        | DA exported by accumulate              | MMR commitments and bidirectional mappings reconstructed via DA |

Only `stateRoot` is consensus-anchored. Everything else either expires with the DA window (unless pinned) or is recovered from the DA artifacts that accumulate writes; JAM State now holds only the routing metadata listed above.【F:services/evm/src/accumulator.rs†L81-L149】【F:services/evm/src/block.rs†L31-L78】

### Meta-shard inclusion (ObjectID → ObjectRef)

Meta-sharding adds a two-step proof chain that matches both the Rust backend and the Go `StateWitness` shape:

1. **Route to meta-shard.** The `meta_shard_object_id(ld, prefix56)` function builds the ObjectID directly as `[ld][prefix_bytes][zero padding]` without hashing. Routing still masks the first `ld` bits of the 56-bit prefix derived from the object hash.
2. **State proof for meta-shard ObjectRef.** The JAM state proof targets that `[ld][prefix]` ObjectID, whose stored value is the 37-byte `ObjectRef` pointing to the meta-shard payload in DA. Rust writes this ObjectID verbatim, and Go uses the same layout when resolving witnesses.
3. **Header binding check.** The meta-shard payload repeats `[ld][prefix56]` in its header. Both Rust and Go recompute the ObjectID from that header and require it to match the state proof key before trusting the payload.
4. **DA fetch + BMT proof.** Fetch the meta-shard payload from DA using that `ObjectRef`, then verify the binary Merkle tree proof that the target object entry lives in the payload. Both Rust and Go treat each entry as the 69-byte tuple `(object_id || object_ref)` when hashing the BMT.
5. **Object payload proof (optional).** If the DA payload for the object is still available, fetch it via the `ObjectRef` stored inside the meta-shard entry. Otherwise only the `ObjectRef` is proven.

## 3. ObjectRef Anatomy (Updated November 2025)

**ObjectRef - Serialized Format (37 bytes)**:
```rust
pub struct ObjectRef {
    pub work_package_hash: [u8; 32],  // 32 bytes
    pub index_start: u16,             // 12 packed bits
    pub payload_length: u32,          // Derived from segments on deserialize
    pub object_kind: u8,              // 4 packed bits
}
// Serialization: work_package_hash (32B) + packed 5 bytes
// Packing: index_start (12 bits) | num_segments (12 bits) | last_segment_bytes (12 bits) | object_kind (4 bits)
```

**JAM State Storage Format (45 bytes total)**:
When written to JAM State, ObjectRef is extended with accumulate-time metadata:
```
ObjectRef (37 bytes) + timeslot (4 bytes LE) + blocknumber (4 bytes LE)
```

This separation enables:
- **Refine-time fields** (in ObjectRef): Deterministic DA pointers
- **Accumulate-time metadata** (separate): Block ordering, timeslot, block number

```rust
#[repr(u8)]
pub enum ObjectKind {
    Code             = 0x00, // Contract bytecode
    StorageShard     = 0x01, // Storage shard object
    SsrMetadata      = 0x02, // Contract SSR metadata
    Receipt          = 0x03, // Receipt payload (embeds full transaction)
    MetaShard        = 0x04, // Meta-shard (ObjectID→ObjectRef mappings)
    Block            = 0x05, // Block payload
}
```

**Key Changes (November 2025)**:
- ObjectRef reduced from 64 → 37 bytes (42% smaller)
- Accumulate metadata (timeslot, blocknumber) stored separately in JAM State
- Added ObjectKind::MetaShard for meta-sharding

> **Object IDs.**
> - Receipts: `keccak256(RLP(signed_tx))` (transaction hash)
> - Code: `keccak256(service_id || address || 0xFF || "code")`
> - Storage shard: `keccak256(service_id || address || 0xFF || ld || prefix56 || "shard")`
> - Meta-shard: `[ld][prefix_bytes][zero padding]` (no hashing)
> - Block: `0xFF` repeated 28 times + block_number (4 bytes LE)
>
> The `object_kind` field is the authoritative discriminator.

### 3.1 Payload discriminators (`work_item.payload`)

The service routes different payload types to different handlers using numeric type tags:

| Payload Type | Tag | Format | Purpose                | Handler |
|--------------|-----|--------|------------------------|---------|
| `Builder`    | `0x00` | `type_byte (1) + count (4 LE) + [witness_count (2 LE)]` | Builder-submitted txs  | `refine_payload_transactions()` |
| `Transactions` | `0x01` | `type_byte (1) + count (4 LE) + [witness_count (2 LE)]` | Transaction execution + block metadata | `refine_payload_transactions()` |
| `Genesis`    | `0x02` | `type_byte (1) + extrinsics_count (4 LE) + [witness_count (2 LE)]` | Genesis bootstrap (K commands only) | `refine_genesis_payload()` |
| `Call`       | `0x03` | `type_byte (1) + count (4 LE) + [witness_count (2 LE)]` | eth_call / eth_estimateGas | `refine_call_payload()` |

**Format Notes**:
- All payloads start with a single type byte (0x00-0x03)
- Count field (4 bytes LE) represents: tx_count for txs, extrinsics_count for blocks
- Witness_count field (2 bytes LE) indicates state witnesses included
  - **For PayloadTransactions**: May include block witnesses (object_id pattern: `0xFF...FF[block_number:4 LE]`)
  - **For PayloadBlocks**: Genesis block witness (block 0)
  - Block witnesses enable automatic metadata computation (see section 3.2)

### 3.2 Block Witnesses and Metadata Computation

**Block Witness Pattern**:
- Block payloads stored with ObjectID: `0xFF` repeated 28 times + block_number (4 bytes LE)
- Helper: `is_block_number_state_witness(object_id) -> Option<u32>`
- Returns `Some(block_number)` if pattern matches, `None` otherwise

**Automatic Metadata Computation in PayloadTransactions**:

When `PayloadTransactions` includes block witnesses, the refiner automatically computes metadata for those blocks:

1. **Load witnesses**: Verify all witnesses against `refine_state_root`
2. **Detect block witnesses**: Check each witness object_id with `is_block_number_state_witness()`
3. **Deserialize block**: Extract `EvmBlockPayload` from witness payload
4. **Compute metadata**: Calculate `transactions_root`, `receipts_root`, `mmr_root` from full block data
5. **Create write intent**: Store metadata with ObjectID = `blake2b_hash(block.serialize_header()[0..HEADER_SIZE])`
6. **Return in ExecutionEffects**: Metadata write intents appended alongside receipts

**Why This Works**:
- Block N is finalized during accumulate (contains all tx/receipt hashes)
- Block N+1 transactions (next round) include witness for block N
- Block N metadata computed from FULL block data in witness payload
- Metadata lag (computed in N+1) is acceptable - only needed for queries

**PayloadBlocks Usage**:
- **Genesis only**: Block 0 initialization with 'K' storage commands
- **No 'B' commands**: Removed (old approach was flawed)
- **Includes block 0 witness**: Genesis block witness for metadata computation in first transaction round

**Implementation**:
- Rust: `refiner.rs:770-816` (block witness detection in PayloadTransactions)
- Rust: `block.rs:37-56` (is_block_number_state_witness helper)
- Go: `rollup.go:589-675` (SubmitEVMTransactions with witness collection)
- Go: `rollup.go:756-879` (SubmitEVMPayloadBlocks genesis-only)

## 4. Lifecycle: From Execution to Proved Inclusion

### Phase 1 (Refine) — Transaction Execution

**Entry**: `BlockRefiner::from_work_item()`
- Located in: `refiner.rs:409-540`
- Parses payload via `parse_payload_metadata()`
- Routes to appropriate handler based on `PayloadType`

**For PayloadTransactions/PayloadBuilder:**

1. **Parse Payload Metadata** (`refiner.rs:47-93`):
   ```rust
   // Format: type_byte (1) + count (4 LE) + [witness_count (2 LE)]
   // Type bytes: 0x00=Builder, 0x01=Transactions, 0x02=Blocks, 0x03=Call
   let metadata = parse_payload_metadata(&work_item.payload)?;
   // Returns: PayloadMetadata { payload_type, payload_size, witness_count }
   ```

2. **Load and Verify State Witnesses** (`refiner.rs:420-456`):
   ```rust
   // Witnesses come after transaction extrinsics
   let witness_start_idx = metadata.payload_size as usize;

   for (idx, extrinsic) in extrinsics.iter().enumerate() {
       if idx < witness_start_idx { continue; }  // Skip tx extrinsics

       // Deserialize and verify witness against refine_state_root
       match StateWitness::deserialize_and_verify(extrinsic, refine_state_root) {
           Ok(state_witness) => {
               object_refs_map.insert(state_witness.object_id, state_witness.ref_info);
           }
           Err(e) => return None;  // Fail on invalid witness
       }
   }
   ```

   **Witness Format** (serialized in extrinsic):
   ```
   object_id (32 bytes) +
   object_ref (37 bytes) +
   proof_hashes (32 bytes each, zero or more entries)
   ```

3. **Execute Transactions** (`refiner.rs:142-397`):
   - Calls `BlockRefiner::refine_payload_transactions()`
   - For each transaction:
     - Compute `tx_hash = keccak256(RLP(signed_tx))`
     - Execute via EVM
     - Create `TransactionReceiptRecord`
   - Calls `overlay.deconstruct()` which:
     - Calls `backend::emit_receipts`
     - Creates `WriteIntent` for each receipt with:
       - `object_id` = tx_hash
       - `object_kind` = `ObjectKind::Receipt` (0x03)
       - `version` = 0 (v0 receipts)
       - `payload` = serialized receipt (see format below)

4. **Export to DA** (`from_work_item:516-526`):
   ```rust
   // Export payloads to DA segments
   let mut export_count: u16 = 0;
   for intent in execution_effects.write_intents.iter_mut() {
       match intent.effect.export_effect(export_count as usize) {
           Ok(next_index) => export_count = next_index,
           Err(e) => { /* log error */ }
       }
   }
   ```

**Output**: Serialized `ExecutionEffects` containing receipt write intents

### Phase 2B (Accumulate) — Receipt Storage

**Entry**: `BlockAccumulator::accumulate()`
- Located in: `accumulator.rs:26-50`

**Process**:

1. **Collect Envelopes** (`accumulator.rs:52-95`):
   - Deserialize `ExecutionEffectsEnvelope` from each work item refine output
   - Extract block number if present

2. **Store Write Intents** (`accumulator.rs:33-57`):
   ```rust
   for envelope in envelopes {
       for candidate in envelope.writes {
           match candidate.object_ref.object_kind {
               ObjectKind::Receipt => {
                   // Receipt objects: Write ObjectRef + payload to storage
                   // Key: tx_hash (32 bytes)
                   // Value: ObjectRef (37 bytes) + receipt_payload (variable)
                   let record = TransactionReceiptRecord::from_candidate(&candidate)?;
                   accumulate_receipt(&mut candidate, record)?;

                   // Inside accumulate_receipt:
                   // 1. Add receipt to block (updates tx_hashes, receipt_hashes)
                   // 2. Compute receipt_hash = keccak256(payload)
                   // 3. Write ObjectRef + payload to storage
                   // 4. Append receipt_hash to MMR
               }
               ObjectKind::Block => {
                   // Block objects handled separately (no ObjectRef prefix)
                   // Skipped here, written by EvmBlockPayload::write()
               }
               _ => {
                   // Code, SSR, shards: Write only ObjectRef (no payload)
                   candidate.object_ref.write(&candidate.object_id);
               }
           }
       }
   }
   ```

3. **Finalize Block** (`block.rs:489-502`):
   - Extract block write intent (if present)
   - Deserialize `EvmBlockPayload`
   - Compute `block_hash = Blake2b-256(header)`
   - Store block number index:
     - Key: `0xFF...FF` (28 × 0xFF + 4 bytes padding)
     - Value: `block_number (4 LE) || parent_hash (32)`

## 5. Proof Workflows

The EVM service provides cryptographic proofs for transaction receipts and log events through two complementary mechanisms:

### 5.1 Transaction Receipt Inclusion via ObjectRef

Receipts are stored in JAM State with metadata-rich ObjectRefs, enabling direct BMT proofs. This is the primary mechanism for proving receipt inclusion.

**Quick Overview**:
1. Derive `receiptObjectID = keccak256(RLP(signed_tx))`
2. Call `GetServiceStorageWithProof(EVMServiceID, receiptObjectID)`
3. Verify BMT proof against published `stateRoot`
4. Extract metadata from ObjectRef (timeslot, evm_block, tx_slot, gas_used, etc.)

See [MMR.md](MMR.md#transaction-receipt-inclusion-via-objectref) for complete steps and receipt payload format.

### 5.2 Log/Event Proofs via MMR

For proving that specific logs were emitted (useful for bridges and light clients), the EVM service maintains a Merkle Mountain Range (MMR) of receipt hashes.

**Quick Overview**:
- MMR accumulates receipt hashes during accumulation
- MMR root stored at JAM State key `0xEE...EE`
- Compact O(log N) proofs for N receipts
- Append-only structure (no rebalancing)

**Benefits**:
- **Compact**: MMR proofs scale logarithmically
- **Efficient**: Single commitment covers all receipts in history
- **Bridge-friendly**: Light clients can verify log emission with minimal data

See [MMR.md](MMR.md#logevent-proofs-via-mmr-receipt-hash-inclusion) for:
- Complete verification flow
- Implementation architecture (4-layer design)
- Code examples and usage
- Current implementation status and remaining TODOs

### 5.3 Legacy Log/Event Proofs (Alternative Method)

1. Obtain the receipt ObjectRef proof.
2. Fetch the receipt payload from DA and extract the log of interest.
3. Provide the DA payload inclusion proof (chunk index + erasure coding witness) alongside the state proof.
4. Consumers recompute the log hash and check it against the `receipts_root` in the block object (when available).

### 5.4 Contract Storage Proofs (Sharded Architecture)

Contract storage uses a **two-level sharding architecture** for scalability:

**Layer 1: Per-Contract SSR (Sparse Split Registry)**
- Each contract has an SSR that routes storage keys to shard IDs
- SSR stored in JAM State with ObjectKind::SsrMetadata
- ObjectID: `keccak256(service_id || address || 0xFF || "ssr")`
- Contains routing metadata for up to 62 entries per shard

**Layer 2: Storage Shards**
- Storage keys grouped into shards (up to 62 key-value pairs per 4104-byte shard)
- Each shard stored in DA, with ObjectRef in JAM State
- ObjectID: `keccak256(service_id || address || 0xFF || ld || prefix56 || "shard")`
- Shard splits automatically when reaching 34 entries (~55% capacity)

**Proof Workflow**:
1. **Fetch SSR**: `GetServiceStorageWithProof(EVMServiceID, ssr_object_id)`
2. **Resolve shard**: Use SSR routing to determine which shard contains the key
3. **Fetch shard ObjectRef**: `GetServiceStorageWithProof(EVMServiceID, shard_object_id)`
4. **Fetch shard payload**: Use ObjectRef DA pointers (index_start, payload_length)
5. **Extract value**: Decode shard payload and lookup key-value pair
6. **Verify**: All steps use BMT proofs against `stateRoot`

**Benefits**:
- Bounded JAM State usage per contract (SSR + ObjectRefs)
- Logarithmic proof size O(log N) for N storage slots
- Automatic rebalancing via adaptive splitting
- Conflict-free parallel execution (different shards = no conflicts)

See [DA-STORAGE.md](DA-STORAGE.md) for complete sharding architecture details.

### 5.5 Meta-Sharding Proofs (ObjectID→ObjectRef Lookups)

For objects that don't use deterministic ObjectIDs, the **meta-sharding layer** provides ObjectID→ObjectRef mappings with a **two-level cryptographic proof chain**:

**Architecture**:
- **Global meta-SSR**: Routes ObjectIDs to meta-shard IDs (stored in JAM State)
- **Meta-shards**: Contain ObjectRefEntry mappings (stored in DA, ObjectRef in JAM State)
- **Capacity**: 58 entries per meta-shard (69-byte entries in 4104-byte segments)
- **Split threshold**: 30 entries (~50% capacity)

**ObjectRefEntry Format (69 bytes)**:
```
ObjectID (32 bytes) + ObjectRef (37 bytes)
```

**Meta-Shard Payload Format**:
```
shard_id (8 bytes LE) +
merkle_root (32 bytes) +        // BMT root over entries for inclusion proofs
entry_count (2 bytes LE) +
[entries (69 bytes each)]
```

**Two-Level Proof Chain**:

The witness system provides cryptographic guarantees through two complementary proofs:

1. **JAM State Proof** (Level 1): Proves meta-shard exists in consensus state
   - Verify `GetServiceStorageWithProof(serviceID, metaShardKey)` against `stateRoot`
   - Authenticates meta-shard's ObjectRef (DA pointer to meta-shard payload)
   - Uses JAM's Binary Merkle Trie (BMT) over state storage

2. **Meta-Shard Membership Proof** (Level 2): Proves object is in meta-shard
   - Deserialize meta-shard payload from DA
   - Verify requested `ObjectID` exists in entries
   - Compute BMT root over entries: `compute_bmt_root([(object_id, object_ref.serialize())])`
   - Verify computed root matches `merkle_root` from payload header
   - Ensures cryptographic binding between returned object and authenticated meta-shard

**StateWitness Structure**:
```go
type StateWitness struct {
    ServiceID   uint32      // EVM service ID
    ObjectID    common.Hash // Requested object ID (NOT meta-shard key)
    Ref         ObjectRef   // Object's DA pointer (from meta-shard entry)

    // JAM State proof (Level 1)
    Path        []common.Hash // BMT proof path
    Value       []byte        // Meta-shard ObjectRef (what proof authenticates)

    // Meta-shard membership proof (Level 2)
    MetaShardMerkleRoot [32]byte    // BMT root from meta-shard payload
    MetaShardPayload    []byte      // Complete meta-shard payload from DA

    Payload     []byte // Actual object payload (fetched via Ref)
}
```

**Verification Workflow** (Client-Side):

1. **Verify JAM State Proof**:
   ```
  // Use meta-shard objectID as the trie key (NOT the child objectID)
  proofKey = witness.ObjectID  // meta-shard's key in JAM State
   VerifyBMTProof(stateRoot, proofKey, Value, Path)
   ```
   - Confirms meta-shard exists in consensus state
   - Authenticates meta-shard's ObjectRef
  - **Critical**: Proof is for the meta-shard objectID, not the child objectID

2. **Verify Meta-Shard Membership (with header validation)**:
   ```
  entries = DeserializeMetaShard(MetaShardPayload, metaShardObjectID, serviceID)
   // DeserializeMetaShard internally validates:
  // assert(ComputeMetaShardObjectID(serviceID, entries.shard_id) == metaShardObjectID)
   foundEntry = entries.find(e => e.object_id == ObjectID)
   computedRoot = ComputeBMTRoot(entries)
   assert(computedRoot == MetaShardMerkleRoot)
   assert(foundEntry != null)
   assert(foundEntry.object_ref == Ref)
   ```
   - Confirms object is actually in the meta-shard **and** that the payload matches the state entry
   - Validates meta-shard header (ld/prefix) matches the authenticated meta-shard key
   - Prevents fabricated payloads (untrusted refiner cannot forge BMT root or swap shard headers)

**Meta-Shard ObjectID Computation**:
```
Rust:  keccak256(service_id || "meta_shard" || ld || prefix56)
Go:    keccak256(service_id || "meta_shard" || ld || prefix56)

Meta-SSR: keccak256(service_id || "meta_ssr")
```

The `"meta_shard"` domain separator ensures meta-shard ObjectIDs don't collide with other object types.

**Proof Workflow** (Server-Side - `ReadObject`):

Located in: `statedb/statedb_hostenv.go:302-533`

1. **Probe-backwards lookup**:
   - Fetch global depth hint from MetaSSR key
   - Probe backwards from global depth to find meta-shard containing object
   - Check cache (`GetWitnesses()`) for existing witness
   - If cache miss, fetch meta-shard with proof and populate cache

2. **Meta-shard processing**:
   - Fetch meta-shard payload from DA using meta-shard ObjectRef
   - Deserialize meta-shard and validate header against expected ObjectID
   - Extract ObjectRef for requested objectID from meta-shard entries
   - Generate BMT inclusion proof for objectID within meta-shard

3. **Construct StateWitness** (returns `*StateWitness`):
   ```go
   witness := &types.StateWitness{
       ServiceID:           serviceID,           // Service ID for verification
       ObjectID:            objectID,            // Requested object
       Ref:                 foundRef,            // Object's DA pointer
       Path:                proofHashes,         // JAM State BMT proof
       Value:               valueBytes,          // Meta-shard ObjectRef (proof authenticates this)
      ObjectID:            metaShardObjectID,   // Meta-shard's JAM State key
       MetaShardMerkleRoot: metaShard.MerkleRoot,// BMT root from meta-shard
       MetaShardPayload:    metaShardPayload,    // Complete meta-shard for verification
       ObjectRefs:          make(map[common.Hash]types.ObjectRef), // Additional refs
       Payloads:            make(map[common.Hash][]byte),          // Cached payloads
      ObjectProofs:        map[common.Hash][]common.Hash{objectID: inclusionProof}, // BMT proof for object in meta-shard
   }
   ```

**Security Properties**:

- **Consensus binding**: JAM State proof anchors meta-shard to `stateRoot`
- **Payload integrity**: BMT merkle_root prevents payload fabrication
- **Object authenticity**: Two-level proof chain ties returned object to consensus state
  - ObjectID derivation is identical in Rust and Go: `keccak256(service_id_le || "meta_shard" || ld || prefix56)`
- **Fail-closed**: Invalid proof at either level → verification fails

**When to Use Meta-Sharding**:
- Objects without deterministic ObjectIDs
- Objects requiring version tracking
- Objects needing conflict detection

**When NOT to Use**:
- Receipts (deterministic ObjectID = tx_hash)
- Blocks (deterministic ObjectID = 0xFF...FF + block_number)
- Code (deterministic ObjectID = keccak256(service_id || address || "code"))

**Benefits**:
- Scales to billions of ObjectRefs across all services
- JAM State bounded at ~72MB for 1M meta-shards
- Logarithmic proof size O(log N)
- Conflict-free parallel execution
- Cryptographically sound proofs (two-level verification)

**Implementation**:
- Rust: `services/evm/src/meta_sharding.rs:125-140` - BMT root computation
- Rust: `services/evm/src/da.rs:196-608` - Meta-shard serialization
- Go: `statedb/metashard_lookup.go` - Meta-shard routing and data structures
- Go: `statedb/statedb_hostenv.go:302-533` - ReadObject with probe-backwards lookup
- Go: `statedb/statedb_hostenv.go:535-539` - GetWitnesses cache access
- Go: `statedb/hostfunctions.go:2333-2397` - BMT inclusion proof generation
- Go: `types/statewitness.go:24-35` - StateWitness structure

See [SHARDING.md](SHARDING.md) for complete meta-sharding architecture.

### 5.6 Bridging Beyond 28 Days

After DA garbage collection, bridges need archival helpers:

- Validators maintain cold storage of receipt payloads or regenerate them from re-execution.
- The `stateRoot` proof remains valid forever; only the DA payload fetch requires the short-lived window.
- Bridging protocols should checkpoint proofs within 28 days or rely on archival peers who pin the payloads.

## 6. Mapping Transactions to Blocks (Updated January 2025)

**Current Implementation - Bidirectional Block Mappings**:

The accumulator maintains **bidirectional mappings** between block numbers and work package hashes (1 block = 1 envelope):

**Query by Block Number**:
1. **Read blocknumber→wph mapping**:
   - Key: `0xFF` repeated 28 times + block_number (4 bytes LE)
   - Value: `work_package_hash (32 bytes) || timeslot (4 bytes LE)` = 36 bytes
2. **Use work_package_hash** to fetch ExecutionEffectsEnvelope from DA
3. Extract receipts, transactions, and metadata from envelope

**Query by Work Package Hash**:
1. **Read wph→blocknumber mapping**:
   - Key: work_package_hash (32 bytes)
   - Value: `blocknumber (4 bytes LE) || timeslot (4 bytes LE)` = 8 bytes
2. **Use blocknumber** to fetch block data
3. Timeslot provides temporal ordering

**Receipt-to-Block Mapping**:
- Receipts stored in **DA only** (not in JAM State)
- Receipt ObjectID = `keccak256(RLP(signed_tx))` (transaction hash)
- To find which block a receipt is in:
  1. Extract work_package_hash from ObjectRef
  2. Query wph→blocknumber mapping
  3. Get block number and timeslot

**Benefits**:
- Minimal JAM State (44 bytes per block)
- O(1) lookups in both directions
- Timeslot included for temporal queries
- Fail-fast accumulator (returns None if writes fail)

## 7. Block Payload Layout (Future)

When block payload construction is implemented, the exported block object (`ObjectKind::Block`) will contain:

**EvmBlockPayload Structure** (`block.rs:29-60`):
```rust
pub struct EvmBlockPayload {
    // 476-byte fixed header (hashed for block ID)
    pub number: u64,
    pub parent_hash: [u8; 32],
    pub logs_bloom: [u8; 256],
    pub transactions_root: [u8; 32],  // BMT root
    pub state_root: [u8; 32],
    pub receipts_root: [u8; 32],      // BMT root
    pub miner: [u8; 20],
    pub extra_data: [u8; 32],
    pub size: u64,
    pub gas_limit: u64,
    pub gas_used: u64,
    pub timestamp: u64,

    // Variable data (not hashed)
    pub tx_hashes: Vec<[u8; 32]>,
    pub receipt_hashes: Vec<[u8; 32]>,
}
```

**Block Hash**: `Blake2b-256(serialize_header())` - only the 476-byte header

**Merkle Roots**:
- `transactions_root`: BMT over transaction hashes (keys: 0, 1, 2..., values: keccak256(RLP(tx)))
- `receipts_root`: BMT over canonical receipt hashes (keys: 0, 1, 2..., values: keccak256(canonical_receipt_rlp))

**Logs Bloom**: 256-byte (2048-bit) Ethereum-compatible bloom filter using 3-hash algorithm (EIP-2159)

## 8. State Witness System

### Overview

State witnesses are Merkle proofs that cryptographically verify an object's metadata (ObjectRef) exists in the service state trie at a specific state root. The witness system enables:

1. **Builder efficiency**: First guarantor fetches DA objects once and generates witnesses
2. **Validator efficiency**: Subsequent guarantors verify witnesses instead of fetching from DA
3. **Security**: Only cryptographically verified witnesses allow access to imported objects

### Witness Flow

**Phase 1: Builder (First Guarantor)**

1. Execute refine with DA object access
2. Generate state witnesses via `HostFetchWitness`:
   - Fetch object from DA
   - Get ObjectRef from state storage
   - Generate Merkle proof against state root
3. Collect witnesses via `GetBuilderWitnesses()`
4. Serialize witnesses as extrinsics:
   ```
   object_id (32) + object_ref (64) + proofs (32 each, zero or more)
   ```
5. Distribute bundle with witnesses + imported segments

**Phase 2: Validators (Subsequent Guarantors)**

Located in: `refiner.rs:428-449`

1. Parse witness extrinsics (after transaction extrinsics)
2. Deserialize each witness
3. Verify Merkle proof against `refine_state_root` from RefineContext:
   ```rust
   match StateWitness::deserialize_and_verify(extrinsic, refine_state_root) {
       Ok(state_witness) => {
           object_refs_map.insert(state_witness.object_id, state_witness.ref_info);
       }
       Err(e) => return None;  // FAIL on invalid witness
   }
   ```
4. Only verified witnesses enter `object_refs_map`
5. Process imports using verified metadata (no DA fetch needed)

### Security Model

**Threat: Malicious Builder**

Defense mechanisms:
1. **Cryptographic Verification**: Every witness includes Merkle proof verified against consensus `state_root`
2. **Metadata Integrity**: Imported objects must reference witnesses that passed verification
3. **Fail-Closed**: Invalid proof OR missing witness → import rejected → execution fails
4. **State Root Binding**: RefineContext.StateRoot provided by consensus (trusted)

**Trust Model**:
- **Trusted**: RefineContext.StateRoot, Merkle trie implementation, Blake2b
- **Untrusted**: Bundle builder, witness data, segment data, ObjectRef metadata

**Principle**: "Trust nothing from the builder except what is cryptographically verified against the consensus state root"

### Performance Impact

**Without Witnesses**:
- Every validator fetches objects from DA independently
- 5 validators × N objects = 5N DA reads

**With Witnesses**:
- Builder: 1× DA fetch per object
- Other validators: 0× DA fetches (verify proof instead)
- **5× reduction in DA reads**

## 9. Implementation Status

| Feature | Status | Location |
|---------|--------|----------|
| PayloadType enum | ✅ Complete | `refiner.rs:21-29` |
| parse_payload_metadata() | ✅ Complete | `refiner.rs:47-93` |
| State witness verification | ✅ Complete | `refiner.rs:428-449` |
| Receipt creation & export | ✅ Complete | `refiner.rs:142-397` |
| Receipt storage in accumulate | ✅ Complete | `accumulator.rs:97-166` |
| Block metadata storage | ✅ Complete | `accumulator.rs:168-233` |
| EvmBlockPayload struct | ✅ Complete | `block.rs:29-60` |
| Block hash computation | ✅ Complete | `block.rs:64-95` |
| BMT implementation | ✅ Complete | `bmt.rs` |
| Canonical receipt RLP | ✅ Complete | `receipt.rs` |
| PayloadType::Block handler | ❌ Not implemented | Returns error |
| Block payload export to DA | ❌ Not implemented | Placeholder only |
| Receipt deserialization | ⚠️ Stub | `receipt.rs:36-49` |

## 10. Key Files

**Rust (services/evm/src/)**:
- `refiner.rs:21-93` - PayloadType enum and parsing
- `refiner.rs:409-540` - from_work_item (main orchestrator)
- `refiner.rs:428-449` - State witness verification
- `refiner.rs:142-397` - Transaction execution and receipt creation
- `block.rs:29-95` - EvmBlockPayload structure and serialization
- `accumulator.rs:26-233` - Receipt and block storage
- `receipt.rs` - Receipt structures and RLP encoding
- `bmt.rs` - Binary Merkle Trie implementation

**Rust (services/utils/src/)**:
- `effects.rs` - StateWitness deserialization and verification
- `objects.rs` - ObjectRef structure (37 bytes)

**Go (implementation reference)**:
- `statedb/hostfunctions.go:1931-2017` - HostFetchWitness (builder path)
- `statedb/hostfunctions.go:2188-2284` - GetBuilderWitnesses with BMT proof generation
- `statedb/statedb_hostenv.go:302-533` - ReadObject (witness generation with cache)
- `statedb/statedb_hostenv.go:535-539` - GetWitnesses (cache access)
- `node/node_da.go:379-405` - Witness serialization
- `types/statewitness.go:317-341` - SerializeWitness

## 11. External References

- [JAM Graypaper](https://graypaper.com/) - JAM specification
- [EIP-2159](https://eips.ethereum.org/EIPS/eip-2159) - Ethereum logs bloom filter
- [EIP-2718](https://eips.ethereum.org/EIPS/eip-2718) - Typed transaction envelopes
