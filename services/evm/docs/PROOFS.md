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
| `stateRoot`           | JAM block header                       | `ObjectRef`s, contract shards, SSR metadata                 |
| `transactionRoot`     | Block metadata in service storage      | BMT root of transaction hashes                              |
| `receiptRoot`         | Block metadata in service storage      | BMT root of canonical receipt hashes                        |
| Transaction payloads  | DA exported by refine                  | RLP/typed transaction bodies                                |
| Receipt payloads      | DA exported by refine                  | Receipt fields: status, cumulative gas, logs, bloom inputs  |
| Block number index    | JAM service storage (accumulate)       | `block_number (4 LE) || parent_hash (32)`                   |
| Block payload         | JAM service storage (accumulate)       | EvmBlockPayload: header + tx/receipt hashes                 |
| Block metadata        | JAM service storage (accumulate)       | EvmBlockMetadata: transactions_root + receipts_root + mmr_root (96 bytes) |

Only `stateRoot` is consensus-anchored. Everything else either expires with the DA window (unless pinned) or lives in service storage that accumulate writes.

## 3. ObjectRef Anatomy

```rust
pub struct ObjectRef {
    // Deterministic at refine time
    pub service_id: u32,
    pub work_package_hash: [u8; 32],
    pub index_start: u16,
    pub index_end: u16,
    pub version: u32,
    pub payload_length: u32,

    // Accumulate-time (finalized ordering)
    pub object_kind: u8,
    pub log_index: u8,     // Reserved/padding
    pub tx_slot: u16,
    pub timeslot: u32,     // JAM block slot; maps 1:1 to canonical block hash
    pub gas_used: u32,     // Non-zero for receipt objects
    pub evm_block: u32,    // Sequential Ethereum block number
}
```

```rust
#[repr(u8)]
pub enum ObjectKind {
    Code           = 0x00, // Contract bytecode
    StorageShard   = 0x01, // Storage shard object
    SsrMetadata    = 0x02, // SSR metadata
    Receipt        = 0x03, // Receipt payload (embeds full transaction)
    Block          = 0x05, // Block payload
}
```

Receipts write their payload to DA during refine (Phase 1) and populate only the deterministic fields. Accumulate stamps the final ordering metadata: `timeslot`, `gas_used`, `evm_block`, and `tx_slot`. This record becomes the single source of truth for RPC responses.

> **Object IDs.** Receipts use the canonical Keccak hash of the transaction as the `ObjectId`. The `object_kind` field is the authoritative discriminator. Code, shard, and SSR objects keep their existing structured IDs.

### 3.1 Payload discriminators (`work_item.payload`)

The service routes different payload types to different handlers using numeric type tags:

| Payload Type | Tag | Format | Purpose                | Handler |
|--------------|-----|--------|------------------------|---------|
| `Builder`    | `0x00` | `type_byte (1) + count (4 LE) + [witness_count (2 LE)]` | Builder-submitted txs  | `refine_payload_transactions()` |
| `Transactions` | `0x01` | `type_byte (1) + count (4 LE) + [witness_count (2 LE)]` | Transaction execution + block metadata | `refine_payload_transactions()` |
| `Blocks`     | `0x02` | `type_byte (1) + extrinsics_count (4 LE) + [witness_count (2 LE)]` | Genesis bootstrap (K commands only) | `refine_blocks_payload()` |
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
   object_ref (64 bytes) +
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
                   // Value: ObjectRef (64 bytes) + receipt_payload (variable)
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

### 5.4 Contract Storage Proofs

Contract storage follows the same pattern: storage shards and SSR metadata are entries in JAM State. Use `GetServiceStorageWithProof` with the shard key `[address || shard_prefix]` to obtain the BMT proof, and decode the shard locally.

### 5.5 Bridging Beyond 28 Days

After DA garbage collection, bridges need archival helpers:

- Validators maintain cold storage of receipt payloads or regenerate them from re-execution.
- The `stateRoot` proof remains valid forever; only the DA payload fetch requires the short-lived window.
- Bridging protocols should checkpoint proofs within 28 days or rely on archival peers who pin the payloads.

## 6. Mapping Transactions to Blocks

**Current Implementation**:

1. **Read the ObjectRef** via `GetServiceStorageWithProof` using `tx_hash` as key.
   - Storage value format: `ObjectRef (64 bytes) || receipt_payload (variable)`
2. **Use the accumulate metadata** from ObjectRef:
   - `object_kind` = `ObjectKind::Receipt` (0x03)
   - `evm_block` = canonical Ethereum block number
   - `tx_slot` = transaction index within block
   - `timeslot` = JAM timeslot
   - `gas_used` = gas consumed
3. **Read block number index** from service storage:
   - Key: `0xFF...FF` (28 × 0xFF + 4 bytes padding)
   - Value: `block_number (4 LE) || parent_hash (32)`
4. **Pending transactions** simply lack a committed `ObjectRef`; clients treat them as mempool entries.

**Future**: Once block payload construction is implemented (`PayloadType::Block`), the system will also store a pointer to the block object in DA.

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
- `objects.rs` - ObjectRef structure (64 bytes)

**Go (implementation reference)**:
- `statedb/hostfunctions.go:1931-2017` - HostFetchWitness (builder path)
- `statedb/hostfunctions.go:2019-2068` - GetBuilderWitnesses
- `node/node_da.go:379-405` - Witness serialization
- `types/statewitness.go:317-341` - SerializeWitness

## 11. External References

- [JAM Graypaper](https://graypaper.com/) - JAM specification
- [EIP-2159](https://eips.ethereum.org/EIPS/eip-2159) - Ethereum logs bloom filter
- [EIP-2718](https://eips.ethereum.org/EIPS/eip-2718) - Typed transaction envelopes
