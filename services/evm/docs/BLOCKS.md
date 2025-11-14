# Block and Receipt Architecture - EVM Service

This document describes the complete block construction, transaction execution, and receipt handling in the JAM EVM service, including payload formats, execution flow, and code organization.

---

## 📋 Code Organization

The EVM service is organized into modules that handle different phases of block construction and transaction processing:

### main.rs - Service Entry Points

**Purpose**: JAM service interface with two entry points

**Entry Points**:
- `refine()` - Phase 1: Transaction execution and ExecutionEffects generation
- `accumulate()` - Phase 2B: Block finalization and storage

**Refine Flow**:
1. Parse `RefineArgs` from input buffer
2. Extract work item and extrinsics from refine context
3. Call `BlockRefiner::from_work_item()` to process work package
4. Call `serialize_execution_effects()` to convert results to binary format
5. Return buffer pointer and length via `leak_output()`

**Accumulate Flow**:
1. Parse `AccumulateArgs` from input buffer
2. Call `BlockAccumulator::accumulate()` to process execution effects
3. Return empty output (no refine output in accumulate phase)

### refiner.rs - Transaction Execution and Block Construction

**Purpose**: Handles all transaction execution logic and block metadata preparation during Phase 1 (refine)

**Core Components**:

- **`PayloadType` enum** - Type-safe payload discriminator:
  - `Builder` (0x00) - Builder-submitted transactions
  - `Transactions` (0x01) - Normal transactions
  - `Blocks` (0x02) - Bootstrap execution with RAW witnesses
  - `Call` (0x03) - Read-only call

- **`PayloadMetadata` struct** - Parsed payload information:
  - `payload_type: PayloadType` - Discriminated payload type
  - `payload_size: u32` - Transaction/extrinsic count
  - `witness_count: u16` - Number of state witnesses included

- **`BlockRefiner` struct** - Manages imported objects during refine:
  - `objects_map: BTreeMap<ObjectId, (ObjectRef, Vec<u8>)>` - Cached DA objects

**Key Methods**:

- `parse_payload_metadata()` - Extracts type and metadata from work item payload
  - Format: type_byte (1) + count (4 LE) + [witness_count (2 LE)]
  - Parses first byte to determine payload type (0x00-0x03)
  - Extracts count (transaction/extrinsic count) and optional witness count
  - Returns `None` for malformed payloads

- `BlockRefiner::from_work_item()` - Main execution orchestrator
  1. Parses payload metadata via `parse_payload_metadata()`
  2. Loads and verifies state witnesses from extrinsics
  3. Populates `object_refs_map` with verified ObjectRefs
  4. Creates EVM backend with environment data
  5. Routes to appropriate handler based on `PayloadType`:
     - `Blocks` → `refine_blocks_payload()` (bootstrap with RAW witnesses)
     - `Call` → `refine_call_payload()`
     - `Transactions/Builder` → `refine_payload_transactions()`
  6. Exports payloads to DA segments
  7. Returns `Some(ExecutionEffects)` or `None` on failure

- `BlockRefiner::refine_payload_transactions()` - Transaction execution loop
  - Decodes and executes RLP-encoded Ethereum transactions
  - Creates `TransactionReceiptRecord` for each transaction
  - Calls `overlay.deconstruct()` which emits write intents for:
    - Code changes (contract deployments/deletions)
    - Storage changes (state updates)
    - Receipts (transaction results)
  - Returns `ExecutionEffects` containing all write intents
  - Uses JAM gas model (zero-cost opcodes + host measurement)

**Transaction Ordering**:
- Uses submission order (extrinsic index order)
- NOT lexicographic hash order
- Critical for Ethereum JSON-RPC compliance

### receipt.rs - Receipt Structures

**Purpose**: Defines receipt data structures and serialization

**Core Structure**:

```rust
pub struct TransactionReceiptRecord {
    pub tx_index: usize,           // Transaction index in block
    pub hash: [u8; 32],           // Transaction hash (keccak256 of RLP tx)
    pub success: bool,            // true = success, false = revert
    pub used_gas: U256,           // JAM gas consumed
    pub logs: Vec<Log>,           // EVM event logs
    pub payload: Vec<u8>,         // Full RLP-encoded signed transaction
}
```

**Key Functions**:

- `encode_canonical_receipt_rlp()` - Ethereum-compatible RLP encoding
  - Format: `tx_type || RLP([status, cumulativeGas, logsBloom, logs])`
  - EIP-2718 typed transaction support
  - Used for computing receipts_root

- `compute_per_receipt_logs_bloom()` - Bloom filter computation
  - EIP-2159 standard (2048 bits, 3-hash algorithm)
  - Includes log addresses and topics

- `TransactionReceiptRecord::from_candidate()` - Receipt deserialization from ObjectCandidateWrite
  - Extracts tx_hash from `candidate.object_id`
  - Extracts tx_index from `candidate.object_ref.tx_slot`
  - Extracts success flag from `candidate.object_ref.tx_slot_2`
  - Extracts gas_used from `candidate.object_ref.gas_used`
  - Deserializes logs from `candidate.payload`
  - Used by accumulate phase to reconstruct receipt data

### block.rs - Block Payload Structure

**Purpose**: Defines block payload format and serialization for DA storage

**Core Structure**:

```rust
pub struct EvmBlockPayload {
    // 476-byte fixed header (hashed for block ID)
    pub number: u64,                    // Block number
    pub parent_hash: [u8; 32],         // Previous block hash
    pub logs_bloom: [u8; 256],         // Aggregated bloom filter
    pub transactions_root: [u8; 32],   // BMT root of tx hashes
    pub state_root: [u8; 32],          // JAM state root
    pub receipts_root: [u8; 32],       // BMT root of canonical receipts
    pub miner: [u8; 20],               // Builder address
    pub extra_data: [u8; 32],          // JAM entropy
    pub size: u64,                      // Block size
    pub gas_limit: u64,                 // Gas limit
    pub gas_used: u64,                  // Gas used
    pub timestamp: u64,                 // JAM timeslot

    // Variable data (not hashed)
    pub tx_hashes: Vec<ObjectId>,       // Transaction hashes (32 bytes each)
    pub receipt_hashes: Vec<ObjectId>,  // Receipt hashes (32 bytes each)
}
```

**Key Methods**:

- `serialize_header()` - Serializes 476-byte fixed header
  - Used for computing block hash: `Blake2b-256(header)`
  - All fields in little-endian format
  - Returns `[u8; 476]` array

- `serialize()` - Full block payload serialization
  - Serializes 476-byte header
  - Appends transaction hash count (u32 LE) + hashes
  - Appends receipt hash count (u32 LE) + hashes
  - Returns `Vec<u8>` for DA export

- `deserialize()` - Deserializes block payload from bytes
  - Parses fixed header fields
  - Parses variable-length hash arrays
  - Returns `Option<(EvmBlockPayload, usize)>`

**Block Number Encoding**:
- Format: `0xFF` repeated 28 times + block_number (4 bytes LE)
- Used as ObjectId key for block number lookups
- Function: `block_number_to_object_id()`

### accumulator.rs - Block and Receipt Storage

**Purpose**: Handles block metadata and receipt storage during Phase 2B (accumulate)

**Core Component**:

- **`BlockAccumulator` struct**:
  - `service_id: u32` - EVM service identifier
  - `envelopes: Vec<ExecutionEffectsEnvelope>` - Collected execution effects from refine
  - `current_block: EvmBlockPayload` - Block being built (loaded from storage)
  - `parent_hash: [u8; 32]` - Parent block hash

**Key Methods**:

- `BlockAccumulator::accumulate()` - Main entry point
  1. Calls `Self::new()` to create accumulator with loaded block
  2. Iterates through envelopes and processes each write intent
  3. For receipts: calls `current_block.add_receipt()` to update block metadata
 4. Writes each object (receipt, code, storage) to storage via `ObjectRef::write()`
 5. Calls `current_block.write()` to persist the in-progress block payload
    - Note: the block is only **finalized** (roots recomputed, sentinel updated) when the next accumulate call arrives with a **larger timestamp**. At that moment `EvmBlockPayload::read()` finalizes the old block and rolls forward to a fresh payload for the new timeslot.
 6. Returns `parent_hash` as accumulate root

- `BlockAccumulator::new()` - Constructor
  - Calls `collect_envelopes()` to deserialize execution effects
  - Calls `EvmBlockPayload::read(service_id, timeslot)` to load or create block
  - Returns initialized accumulator ready to process receipts

- `collect_envelopes()` - Deserializes refine outputs
  - Reads `ExecutionEffectsEnvelope` from each accumulate input
  - Extracts `ok_data` from operation results
  - Calls `deserialize_execution_effects()` to parse binary format
  - Returns vector of envelopes or None on failure

**Block Operations** (in block.rs):

- `EvmBlockPayload::read()` - Loads or creates the *active* block
  - Reads the current block_number from `BLOCK_NUMBER_KEY`
  - Reads the block payload for that number from storage
  - **If the incoming accumulate timestamp matches** the stored block timestamp:
    - Return the existing block so we can keep appending more receipts/tx hashes
    - No finalization happens in this case (we are still “building” the block)
  - **If the incoming timestamp is greater** than the stored block timestamp:
    - Finalize the previous block (`finalize()` computes roots and hash)
    - Update the block-number sentinel to `block_number + 1`
    - Create and return a brand new block payload for the new timestamp
  - **If the incoming timestamp is smaller** than the stored one:
    - Reject accumulation (the node is too late)

- `EvmBlockPayload::add_receipt()` - Adds receipt to block
  - Updates ObjectRef metadata: timeslot, evm_block, tx_slot
  - Adds tx_hash to block's `tx_hashes` vector (**submission order**)
  - Accumulates gas_used
  - Computes per-receipt logs bloom and updates block-wide logs_bloom
  - Computes canonical receipt hash and adds to receipt_hashes

- `EvmBlockPayload::finalize()` - Computes Merkle roots
  - Calls `compute_transactions_root()` - BMT over tx_hashes
  - Calls `compute_receipts_root()` - BMT over canonical receipt RLP hashes
  - Returns block hash computed from 476-byte header

- `EvmBlockPayload::write()` - Persists block to storage
  - Serializes block payload to bytes
  - Writes to storage at key = block_number_to_object_id(block_number)
  - Block becomes available for RPC queries

### writes.rs - ExecutionEffects Serialization

**Purpose**: Serializes execution effects for refine → accumulate communication

**Core Structure**:

```rust
pub struct ObjectCandidateWrite {
    pub object_id: [u8; 32],              // Object ID (e.g., tx_hash for receipts)
    pub object_ref: ObjectRef,             // 64-byte metadata
    pub dependencies: Vec<ObjectDependency>,
    pub payload: Vec<u8>,                  // Only used for receipts
}
```

**Key Functions**:

- `serialize_execution_effects()` - Main serialization function
  - Converts `ExecutionEffects` to binary format
  - Calls `MajikBackend::to_object_candidate_writes()` to convert write intents  
    *(Note: as of this refactor the helper drops the payload bytes when it builds `ObjectCandidateWrite`. Accumulate currently relies on the write intents themselves for receipt reconstruction.)*
  - Logs ObjectCandidateWrite details (object_id, segments, payload_len, etc.)
  - Serializes: export_count (2) + gas_used (8) + write_count (2) + writes

- `ObjectCandidateWrite::serialize()` - Per-object serialization
  - Format: object_id (32) + object_ref (64) + dep_count (2) + dependencies
  - If object_kind == Receipt: appends payload (variable length)

- `ObjectCandidateWrite::deserialize()` - Deserialization
  - Parses object metadata
  - For receipts: reads payload from end of buffer

---

## Transaction Execution and Receipt Creation

### Phase 1: Receipt Creation (Refine)

**Entry Point**: `BlockRefiner::refine_payload_transactions`
- Located in: `refiner.rs`
- Called from: `BlockRefiner::from_work_item` for `PayloadType::Transactions` or `PayloadType::Builder`

**Process**:
1. Parses work item payload to extract `tx_count_expected` (number of transactions)
2. Executes each transaction via EVM with JAM gas model
3. For each transaction (success or failure):
   - Creates `TransactionReceiptRecord` containing:
     - `tx_index` - Transaction index in block
     - `hash` - Transaction hash (keccak256 of RLP-encoded signed tx)
     - `success` - Boolean status (true for success, false for revert)
     - `used_gas` - JAM gas consumed (U256)
     - `logs` - EVM event logs emitted
     - `payload` - Full RLP-encoded signed transaction
   - Stores receipt in local `receipts` vector

4. After all transactions execute, calls `overlay.deconstruct()` which:
   - Calls `backend::emit_receipts()` to create receipt write intents
   - Each receipt becomes a `WriteIntent` with:
     - `object_id` = transaction hash (from `TransactionReceiptRecord.hash`)
     - `object_kind` = `ObjectKind::Receipt` (0x03)
     - `ref_info.version` = 0
     - `ref_info.tx_slot` = transaction index
     - `ref_info.gas_used` = gas consumed
     - `payload` = serialized receipt data (logs + transaction payload)
   - Returns `ExecutionEffects` containing all write intents (code, storage, receipts)

**Key Code Locations**:
- Receipt creation: `refiner.rs:306-313` (success case), `refiner.rs:374-381` (failure case)
- Receipt collection: `refiner.rs:161` (receipts vector)
- Write intent generation: `overlay.deconstruct()` → `backend::to_execution_effects()` → `backend::emit_receipts()`

### Receipt Serialization Format

Receipts are serialized by `serialize_receipt()` (receipt.rs:203-228) with the following format:

```rust
// Full receipt payload structure:
[logs_payload_len: 4 bytes LE]    // Length of serialized logs section
[logs_payload: variable]          // Serialized logs (see below)
[version: 1 byte]                 // 0 = Phase 1B preliminary
[tx_hash: 32 bytes]               // keccak256(RLP-encoded signed tx)
[tx_type: 1 byte]                 // EIP-2718: 0=Legacy, 1=EIP-2930, 2=EIP-1559
[success: 1 byte]                 // 1 = success, 0 = revert
[used_gas: 8 bytes LE]            // JAM gas consumed (as u64)
[tx_payload_len: 4 bytes LE]      // Transaction payload length
[tx_payload: variable]            // Full RLP-encoded signed transaction

// logs_payload structure (from serialize_logs):
[log_count: 2 bytes LE]           // Number of event logs
For each log:
  [address: 20 bytes]             // Contract address
  [topic_count: 1 byte]           // Number of topics (0-4)
  [topics: 32 bytes × N]          // Log topics
  [data_len: 4 bytes LE]          // Data length
  [data: variable]                // Log data
```

**Accumulate Deserialization** (`from_candidate`, receipt.rs:41-75):
- Reads `logs_payload_len` from offset 0 (4 bytes)
- Extracts logs by slicing `payload[4..4+logs_len]`
- Calls `deserialize_logs()` on the extracted slice
- This correctly skips the 4-byte length prefix to parse `[log_count:2][logs...]`
- The rest of the receipt (version, hash, tx payload) is preserved but not parsed in accumulate
- Only logs are needed for computing block metadata (logs_bloom, receipt_hashes)

### Phase 2B: Receipt Storage (Accumulate)

**Entry Point**: `BlockAccumulator::accumulate`
- Located in: `accumulator.rs`
- Called from: `main.rs::accumulate()` service entry point

**Process**:
1. Deserializes `ExecutionEffectsEnvelope` from refine output
2. For each `WriteIntent` with `object_kind == Receipt`:
   - Sets timeslot metadata
   - Exports payload to DA segments via `effect.export_effect()`
   - DA segment allocation assigns:
     - `index_start` - Starting segment index
     - `index_end` - Ending segment index (exclusive)
   - Stores ObjectRef metadata in service storage

**Storage Keys**:
- Transaction hash → ObjectRef (64 bytes)
  - Enables direct receipt lookup by transaction hash
  - Used by RPC `eth_getTransactionReceipt`

### Receipt Witness Protocol

Block builders provide receipt witnesses in work packages for block construction:

**Witness Format**:
- Each receipt witness contains:
  - `object_id` = transaction hash
  - `ref_info` = ObjectRef with DA segment location
  - `payload` = serialized receipt data
  - Merkle proof for state root verification

**Loading Process** (`BlockRefiner::from_work_item`):
1. Iterates through witness extrinsics (after transaction extrinsics)
2. Deserializes and verifies each `StateWitness` against `refine_state_root`
3. Populates `object_refs_map` with `(object_id, ObjectRef)` pairs
4. Logs: `✅ Verified state witness for object {hash} (version {v}, {n} proof hashes)`

**Block Building Flow**:
1. Builder submits transactions in work package payload
2. Refine executes transactions and creates receipts (version 0)
3. Refine returns `ExecutionEffects` containing:
   - Code write intents (contract deployments)
   - Storage write intents (state changes)
   - Receipt write intents (transaction receipts)
4. Accumulate processes all write intents:
   - Exports payloads to DA segments
   - Stores ObjectRefs in service storage (keyed by tx_hash for receipts)
   - Updates block metadata

### Receipt Data Flow

**Refine → Accumulate Pipeline**:

1. **Refine Phase - DA Export**:
   - `backend::emit_receipts()` creates `WriteIntent` with full receipt payload
   - Payload contains: logs (serialized) + transaction data
   - `from_work_item()` calls `export_effect()` on each write intent
   - **Payloads exported to DA** - sets `index_start`/`index_end` in ObjectRef
   - DA segments allocated and payload written via host function

2. **Serialization Phase** (`serialize_execution_effects`):
   - Calls `MajikBackend::to_object_candidate_writes()` (backend.rs:968-997)
   - Converts `WriteIntent` → `ObjectCandidateWrite`
   - **Preserves payload ONLY for receipts** - needed for block metadata extraction
   - Code/storage/SSR payloads stripped (already exported to DA, not needed in accumulate)
   - Receipt payloads preserved so accumulate can extract logs for logs_bloom
   - Serializes to binary format for refine output

3. **Accumulate Phase - Metadata Storage**:
   - Deserializes `ObjectCandidateWrite` from refine output
   - Calls `TransactionReceiptRecord::from_candidate()` to extract logs from payload
   - Uses logs to update block's `logs_bloom` and `receipt_hashes`
   - Writes only ObjectRef (64 bytes) to JAM state via `ObjectRef::write()`
   - ObjectRef contains DA pointers (`index_start`, `index_end`) for later retrieval

**Data Preserved**:
- ✅ Receipt payloads exported to DA
- ✅ Logs available for `eth_getLogs` filtering
- ✅ Transaction data available for `eth_getTransactionReceipt`
- ✅ ObjectRef metadata stored in service storage
- ✅ Block metadata computed during accumulate (block hash, transactions_root, receipts_root)
- ✅ Block payload (`EvmBlockPayload`) persisted to storage
- ✅ Block finalization via `finalize()` method computes Merkle roots
- ✅ Block-level queries (`eth_getBlockByNumber`) supported via stored block payloads

**Block Construction**:
- Accumulate phase calls `EvmBlockPayload::read()` to load/create current block
- Receipts added via `add_receipt()` which updates block metadata
- Block finalized via `finalize()` which computes `transactions_root` and `receipts_root`
- Block hash computed from 476-byte header via `Blake2b-256(serialize_header())`
- BLOCK_NUMBER_KEY updated with block_number + parent_hash for next accumulate

### Receipt Versioning

**Current Implementation**:

All receipts are created with `version = 0`:
- Set in `ObjectRef.version` field during emission
- Receipts are immutable (no version upgrade mechanism)

**ObjectRef Metadata**:
- `version`: Always 0 for receipts
- `tx_slot`: Transaction index in block (stored in refine, read in accumulate)
- `tx_slot_2`: Success flag (1 = success, 0 = revert) - stored in refine, read by `from_candidate`
- `gas_used`: Gas consumed by transaction
- `object_kind`: 0x03 (Receipt)

**Critical Note**: The `tx_slot_2` field is used to carry the transaction success/failure status through the refine→accumulate pipeline. This field is set in `emit_receipts()` (backend.rs:219) and read in `from_candidate()` (receipt.rs:70). Without this, all receipts would incorrectly appear as failed transactions.

---


---

## Transaction Logs and Bloom Filters

### Log Collection During Execution

**Runtime Collection** (`MajikOverlay`):
- Buffers logs on a per-transaction basis
- Maintains scratch vector for currently executing transaction
- Maintains `Vec<Vec<Log>>` indexed by transaction number
- Logs appended via `log()` runtime callback
- Transferred into per-transaction vector when transaction commits

**Receipt Record Creation** (`TransactionReceiptRecord`):
- Captures metadata for receipt export:
  - Transaction index, hash, payload
  - Execution status (success/failure)
  - Gas used
  - Committed log list
- Refine builds one record per executed extrinsic
- Passed to `MajikOverlay::deconstruct()` for write intent generation

### Receipt Write Intents

**Creation** (`MajikBackend::to_execution_effects`):
- Creates receipt write intents alongside code/storage exports
- Receipt ObjectID = transaction hash (enables direct RPC lookup)
- ObjectKind = 0x03 (Receipt) stored in ObjectRef's `object_kind` field
- Receipt payload format:
  ```
  [tx_hash:32][tx_payload_len:4][tx_payload][status:1][gas:32][log_blob_len:4][log_blob]
  ```

**Dependencies**:
- Computed from overlay tracker
- All objects written by same transaction become dependencies
- Receipt only applies if every conflicting write is accepted
- Expected versions taken from write intents produced earlier

**Version Tracking**:
- Receipt version = current on-chain version + 1 (or 1 for new hashes)
- Looked up from prior `ObjectRef`
- Refine-time fields: `service_id`, `work_package_hash`, `version`, `payload_length`
- Accumulate-time fields: `timeslot`, `gas_used`, `evm_block`, `object_kind`, `tx_slot` (zeroed during refine)

### JAM DA and State Layout

**ObjectId**: Transaction hash (`record.hash`) with no modification
- Ensures RPC can find receipts via `tx_to_objectID(txHash) = txHash`

**ObjectKind**: Stored in ObjectRef's `object_kind` field = 0x03 (Receipt)

**Payload Format**:
```
tx_hash (32 bytes)
tx_payload_len (4 bytes LE)
tx_payload (variable)
status (1 byte)
gas (32 bytes U256)
log_blob_len (4 bytes LE)
log_blob (variable)
```

**JAM State Storage**:
- Accumulate writes receipt's `ObjectRef` under transaction hash
- No extra state entries created
- Receipt object is entire state representation for that transaction hash

### Accumulate Bookkeeping

**Receipt Identification**:
- Identified by checking `ObjectRef.object_kind == 0x03`

**Metadata Population**:
- `timeslot` - JAM block slot
- `gas_used` - From execution effects
- `evm_block` - Sequential block number
- `object_kind` - 0x03 for Receipt
- `tx_slot` - Transaction index in block

**Timeslot Tracking** (implementation detail):
- Applied receipts grouped by `ObjectRef.timeslot`
- Each timeslot key (4 bytes) stores length-prefixed list of receipt object ids
- New receipt object id appended to set and written back
- Service prunes keys 60 slots in the past
- For each hash in expired slot: `delete_tx_hash` clears receipt entry, then timeslot key removed

**Guarantees**:
1. Refine produces one deterministic receipt per executed transaction
2. Receipt payloads (including transaction data) and logs available in JAM DA keyed by transaction hash
3. RPC can lookup receipts directly using transaction hash without transformations
4. JAM State retains only canonical receipt object and rolling timeslot index
5. No ObjectID collisions (export only receipts, not separate transaction objects)

### Logs Bloom Filter

**Purpose**: Enable efficient log filtering without fetching all receipts

**Data Structures**:
- `TransactionReceiptRecord` contains `logs: Vec<Log>`
- `Log` contains:
  - `address: H160` - Emitting contract address
  - `topics: Vec<H256>` - Indexed topics (0-4 per log)
  - `data: Vec<u8>` - Unindexed log data

**Algorithm**:

1. **Topic Counting**:
   - Iterate through every log in ordered receipts
   - Hash emitting address and each topic using Ethereum three-hash rule
   - Compute `keccak256(item)` and extract three 16-bit indices from bytes `[0..1]`, `[2..3]`, `[4..5]`
   - Each as big-endian `uint16`
   - Accumulate topic count: each log contributes `1 + len(topics)`

2. **Bloom Size Selection**:

   | Class | Topic Count Range | Bloom Bits | Bloom Bytes |
   |-------|-------------------|------------|-------------|
   | 0     | 0 – 127           | 2,048      | 256         |
   | 1     | 128 – 1,023       | 8,192      | 1,024       |
   | 2     | 1,024 – 9,999     | 32,768     | 4,096       |
   | 3     | 10,000+           | 131,072    | 16,384      |

   - Select smallest class whose upper bound covers finalized topic count
   - Bloom bits always power of two for `hash % bloom_bits` reduction
   - Large blocks amortize higher search cost with more bits
   - Small blocks avoid bloating DA payload

3. **Bloom Construction**:
   - Emit bloom as big-endian byte array (same ordering as Ethereum)
   - Persist both class and byte payload in block object
   - Store block object's ObjectRef in state
   - Bloom discoverable via `GetServiceStorageWithProof`

**Implementation** (`receipt.rs`):
```rust
fn build_bloom(receipts: &[TransactionReceiptRecord]) -> (Vec<u8>, u8) {
    // First pass: count topics
    let mut topic_count = 0;
    for receipt in receipts {
        for log in &receipt.logs {
            topic_count += 1 + log.topics.len(); // address + topics
        }
    }

    // Select class and allocate buffer
    let class = select_class(topic_count);
    let bloom_size = 256 << class; // 0→256, 1→1024, 2→4096, 3→16384
    let bloom_bits = bloom_size * 8;
    let mut bits = vec![0u8; bloom_size];

    // Second pass: populate bloom
    for receipt in receipts {
        for log in &receipt.logs {
            add_to_bloom(&mut bits, bloom_bits, log.address.as_bytes());
            for topic in &log.topics {
                add_to_bloom(&mut bits, bloom_bits, topic.as_bytes());
            }
        }
    }

    (bits, class)
}

fn add_to_bloom(bits: &mut [u8], bloom_bits: usize, item: &[u8]) {
    let hash = keccak256(item);
    
    // Extract three 16-bit indices
    let idx1 = u16::from_be_bytes([hash[0], hash[1]]) as usize % bloom_bits;
    let idx2 = u16::from_be_bytes([hash[2], hash[3]]) as usize % bloom_bits;
    let idx3 = u16::from_be_bytes([hash[4], hash[5]]) as usize % bloom_bits;
    
    set_bit(bits, idx1);
    set_bit(bits, idx2);
    set_bit(bits, idx3);
}

fn set_bit(bits: &mut [u8], index: usize) {
    let byte_index = index / 8;
    let bit_index = 7 - (index % 8); // big-endian bit order within byte
    bits[byte_index] |= 1 << bit_index;
}
```

### RPC Integration: Efficient `eth_getLogs`

**Current Implementation** (`node_evm_logsreceipt.go:116-326`):
- Fetches receipts for every transaction in queried block range
- Inefficient for sparse log queries (e.g., single contract event across thousands of blocks)

**Optimized Flow**:

1. **Fetch block object** - Read `ObjectKind::EVMBlock` entry for block number from JAM State
2. **Extract bloom metadata** - Parse `logsBloomClass` and `logsBloom` bytes from block object
3. **Test filter criteria** - For each address and topic in RPC filter:
   ```go
   func bloomContains(bloom []byte, bloomBits int, item []byte) bool {
       hash := crypto.Keccak256(item)
       idx1 := int(binary.BigEndian.Uint16(hash[0:2])) % bloomBits
       idx2 := int(binary.BigEndian.Uint16(hash[2:4])) % bloomBits
       idx3 := int(binary.BigEndian.Uint16(hash[4:6])) % bloomBits
       
       return testBit(bloom, idx1) && testBit(bloom, idx2) && testBit(bloom, idx3)
   }
   
   func testBit(bits []byte, index int) bool {
       byteIndex := index / 8
       bitIndex := 7 - (index % 8) // big-endian bit order
       return (bits[byteIndex] & (1 << bitIndex)) != 0
   }
   ```
4. **Skip on negative match** - If bloom test fails for any required address/topic, skip entire block without fetching receipts
5. **Fetch receipts on positive match** - Only when bloom indicates potential matches, call `ReadObject` for transaction receipts

**Performance Impact**:

For query `eth_getLogs` with `fromBlock=0`, `toBlock=100000`, single topic filter:
- **Without bloom**: ~100K receipt fetches from DA
- **With bloom**: ~100 receipt fetches (assuming 0.1% false-positive rate on class 2 blooms)

This is why variable bloom size is important: high-throughput blocks pay for larger blooms to keep false positives low across entire chain history.

## Block Construction Flow

### Phase 1 (Refine - Transaction Execution)

**Input**: Work package with payload and extrinsics

**Process**:
1. **Payload Parsing** (`parse_payload_metadata`):
   - Extract payload type and transaction count
   - Validate payload format

2. **Witness Loading** (`from_work_item`):
   - Iterate through witness extrinsics (after transactions)
   - Deserialize and verify each `StateWitness` against state root
   - Build `object_refs_map` with verified ObjectRefs
   - Log: `✅ Verified state witness for object {hash}`

3. **Backend Creation**:
   - Create `EnvironmentData` with block context
   - Initialize `MajikBackend` with verified objects
   - Set coinbase address and gas limits

4. **Transaction Execution** (`refine_payload_transactions`):
   - For each transaction extrinsic:
     - Compute tx_hash = keccak256(RLP-encoded tx)
     - Decode transaction (legacy, EIP-2930, EIP-1559)
     - Execute via EVM with JAM gas model
     - Create `TransactionReceiptRecord`
   - Call `overlay.deconstruct()` to generate write intents
   - Return `ExecutionEffects` with all write intents (code, storage, receipts)

5. **DA Export** (`from_work_item`):
   - For each write intent, call `export_effect()` to allocate DA segments
   - Update `export_count` with total segments used

**Output**: `ExecutionEffects` serialized via `serialize_execution_effects()`

### Phase 2B (Accumulate - Block Storage)

**Input**: Serialized `ExecutionEffectsEnvelope` from refine

**Process**:
1. **Envelope Collection** (`collect_envelopes`):
   - Deserialize execution effects from each work item
   - Extract current block number from witness if present
   - Build `envelopes` vector

2. **State Accumulation** (`accumulate_states`):
   - For each write intent:
     - Set timeslot metadata
     - Export payload to DA segments
     - Log segment allocation
     - Store ObjectRef (for receipts: key=tx_hash, for blocks: separate flow)

3. **Block Finalization** (`accumulate_block`):
   - Extract block write intent from envelopes
   - Deserialize `EvmBlockPayload`
   - Compute block_hash = Blake2b-256(476-byte header)
   - Store block metadata to service storage
   - Log: `Accumulate: block #{number} hash={hash} #txs:{count}`

**Output**: Empty (no refine output in accumulate phase)

---

## Block Hash Computation

**Formula**:
```
block_hash = Blake2b-256(serialize_header())
```

**Header Format** (476 bytes total):
```
number (8) || parent_hash (32) || logs_bloom (256) ||
transactions_root (32) || state_root (32) || receipts_root (32) ||
miner (20) || extra_data (32) || size (8) || gas_limit (8) ||
gas_used (8) || timestamp (8)
```

- All fields serialized in **little-endian** format
- Variable data (tx_hashes, receipt_hashes) NOT included in hash
- Implementation: `block.rs::serialize_header()`

---

## Merkle Roots

### Binary Merkle Tree (BMT) vs Ethereum MPT

JAM uses Binary Merkle Tree (BMT), NOT Ethereum's Modified Patricia Trie:

| Aspect | JAM BMT | Ethereum MPT |
|--------|---------|--------------|
| Hash function | Blake2b-256 | Keccak-256 |
| Key encoding | 32-byte big-endian index | RLP-encoded |
| Leaf format | `[header\|31B key\|32B value]` | RLP `[path, value]` |
| Branch format | `[255b left\|256b right]` | RLP `[16 children, value]` |
| Bit addressing | MSB-first | Nibble-based |

**Implementation**: `bmt.rs`

### Transaction Root

- **Keys**: Transaction indices (0, 1, 2...) as 32-byte big-endian
- **Values**: Transaction hashes (keccak256 of RLP-encoded tx)
- **Computation**: During block finalization via BMT

### Receipt Root

- **Keys**: Transaction indices (0, 1, 2...) as 32-byte big-endian
- **Values**: Keccak-256 of Ethereum canonical receipt:
  ```
  tx_type || RLP([status, cumulativeGas, logsBloom, logs])
  ```
- **EIP-2718 Support**: Typed transaction envelopes included
- **Implementation**: `receipt.rs::encode_canonical_receipt_rlp()`

---

## Logs Bloom Filter

**Ethereum Standard (EIP-2159)**:
- 2048 bits (256 bytes)
- 3-hash algorithm with Keccak-256
- 11-bit mask (0x7FF) for bit positions
- Includes log addresses and topics

**Implementation**: `receipt.rs::compute_per_receipt_logs_bloom()`

**Aggregation**: Block-wide bloom computed from all receipt logs during block finalization

---

## Payload Type Routing

**Unified Payload Format**:

All payload types use the same binary format:
```
type_byte (1) + count (4 LE) + [witness_count (2 LE)]
```

Where:
- `type_byte`: Discriminator (0x00-0x03)
- `count`: Transaction/extrinsic count (u32 LE)
- `witness_count`: Optional witness count (u16 LE), present when witnesses are included

**Supported Payload Types**:

1. **Builder (0x00)**:
   - Handler: `refine_payload_transactions()`
   - Purpose: Builder-submitted transaction bundles
   - Count: Number of transactions
   - Witness count: Number of builder witnesses (receipts + state)

2. **Transactions (0x01)**:
   - Handler: `refine_payload_transactions()`
   - Purpose: Normal transaction execution
   - Count: Number of transactions
   - Witness count: Number of transaction witnesses
   - Creates receipts for each transaction

3. **Blocks (0x02)**:
   - Handler: `refine_blocks_payload()`
   - Purpose: Bootstrap execution with RAW witnesses
   - Count: Number of bootstrap extrinsics ('A' add code, 'K' set storage)
   - Witness count: Number of RAW witnesses (BLOCK_NUMBER_KEY + blocks)

4. **Call (0x03)**:
   - Handler: `refine_call_payload()`
   - Purpose: Read-only eth_call / eth_estimateGas
   - Count: Always 1 (single call extrinsic)
   - Witness count: Always 0 (no witnesses for simulation)
   - No receipts generated

**Routing Logic** (`from_work_item`):
```rust
match metadata.payload_type {
    PayloadType::Builder | PayloadType::Transactions => {
        refine_payload_transactions(...)
    }
    PayloadType::Blocks => refine_blocks_payload(...),
    PayloadType::Call => refine_call_payload(...),
}
```


## Block Metadata Computation via Witnesses (New Architecture)

### Problem Statement

EVM blocks need metadata (transactions_root, receipts_root, mmr_root) computed from the complete block data. However, transactions are executed incrementally during refine, and the full block is only available after accumulate completes. The previous approach using a 'B' command in PayloadBlocks was flawed because blocks from previous accumulate don't contain current transactions.

### Solution: Automatic Block Witness Detection in PayloadTransactions

Instead of explicit 'B' commands, block metadata is computed automatically when PayloadTransactions contains block witnesses from the previous accumulate cycle.

**Key Insight**: A block witness (payload from previous accumulate) contains the FULL block data including all transaction hashes and receipt hashes. This allows computing metadata during the NEXT round's PayloadTransactions processing.

### Architecture

**Block Witness Identification**:
- Block witnesses use ObjectID pattern: `0xFF` repeated 28 times + block_number (4 bytes LE)
- Helper function: `is_block_number_state_witness(object_id) -> Option<u32>`
- Returns `Some(block_number)` if object_id matches pattern, `None` otherwise

**PayloadTransactions Processing with Block Witnesses**:
1. Parse payload metadata: type=0x01 (Transactions), tx_count, witness_count
2. Load and verify all witnesses (including block witnesses)
3. Execute transaction extrinsics → create receipts
4. **NEW**: Iterate through verified witnesses:
   - For each witness, check if `is_block_number_state_witness(object_id)`
   - If block witness detected:
     - Deserialize `EvmBlockPayload` from witness payload
     - Compute `EvmBlockMetadata::new(&block)` (transactions_root, receipts_root, mmr_root)
     - Create `WriteIntent` for metadata with `effect = metadata.to_write_effect_entry(&block)`
     - Append to execution_effects
5. Return ExecutionEffects containing: receipts + code + storage + **block metadata**

**PayloadBlocks (Genesis Only)**:
- Used ONLY for genesis/bootstrap (block 0)
- No 'B' commands (removed)
- Only 'K' commands for storage initialization
- Includes genesis block witness (block 0)

**Block Metadata ObjectID**:
- Computed as: `blake2b_hash(block.serialize_header()[0..HEADER_SIZE])`
- HEADER_SIZE = 432 bytes (fixed header portion)
- Stored as WriteIntent during refine, persisted during accumulate

### Implementation Details

**Rust (services/evm/src/refiner.rs)**:

```rust
// In refine_payload_transactions() after transaction execution:
let mut block_write_intents: Vec<WriteIntent> = Vec::new();

for (object_id, (ref_info, payload)) in verified_witnesses {
    if let Some(block_number) = crate::block::is_block_number_state_witness(&object_id) {
        log_info(&format!("  📦 Block witness detected: block_number={}", block_number));

        match EvmBlockPayload::deserialize(&payload) {
            Ok(block) => {
                let metadata = EvmBlockMetadata::new(&block);
                let write_intent = WriteIntent {
                    effect: metadata.to_write_effect_entry(&block),
                    dependencies: Vec::new(),
                };
                block_write_intents.push(write_intent);
            }
            Err(e) => {
                log_error(&format!("Failed to deserialize block {}: {}", format_object_id(object_id), e));
            }
        }
    }
}

// Append block metadata write intents to execution_effects
execution_effects.append(&mut block_write_intents);
```

**Rust (services/evm/src/block.rs)**:

```rust
pub fn is_block_number_state_witness(object_id: &ObjectId) -> Option<u32> {
    // Check if first 28 bytes are 0xFF
    for i in 0..28 {
        if object_id[i] != 0xFF {
            return None;
        }
    }

    // Extract block number from last 4 bytes (little-endian)
    let block_number = u32::from_le_bytes([
        object_id[28], object_id[29], object_id[30], object_id[31],
    ]);
    Some(block_number)
}
```

**Go (statedb/rollup.go)**:

```go
// SubmitEVMTransactions now accepts block witness range
func (b *Rollup) SubmitEVMTransactions(evmTxsMulticore [][][]byte, startBlock uint32, endBlock uint32) (*evmtypes.EvmBlockPayload, error) {
    // For each core:
    // 1. Collect transaction extrinsics
    // 2. If endBlock > startBlock:
    //    - For blockNum in [startBlock, endBlock):
    //      - Read witness for BlockNumberToObjectID(blockNum)
    //      - Append witness as extrinsic
    // 3. Build payload: type=0x01, tx_count, witness_count
    // 4. Process work package
}

// SubmitEVMPayloadBlocks simplified to genesis-only
func (b *Rollup) SubmitEVMPayloadBlocks(startBlock uint32, endBlock uint32) (*evmtypes.EvmBlockPayload, *evmtypes.EvmBlockMetadata, error) {
    if startBlock != 0 || endBlock != 1 {
        return nil, nil, fmt.Errorf("SubmitEVMPayloadBlocks is genesis-only")
    }

    // Genesis flow:
    // 1. Add 'K' extrinsics for USDM storage initialization
    // 2. Add witness for block 0
    // 3. Build payload: type=0x02, extrinsic_count, witness_count=1
    // 4. Process work package
}
```

### Execution Flow Example

**Genesis (Timeslot 0)**:
```
PayloadType::Blocks (0x02)
- Extrinsics: ['K' storage init, 'K' storage init, ...]
- Witnesses: [block_0_witness]
- Refine: Initialize storage, create block 0 payload
- Accumulate: Write block 0 to DA (object_id = 0xFF...FF00000000)
```

**Round 1 (Timeslot 2)**:
```
PayloadType::Transactions (0x01)
- Extrinsics: [tx1_bytes]
- Witnesses: [block_0_witness]
- Refine:
  1. Execute tx1 → create receipt
  2. Detect block 0 witness
  3. Deserialize block 0 payload
  4. Compute metadata (transactions_root, receipts_root, mmr_root)
  5. Create metadata write intent
  6. Return: ExecutionEffects = [receipt, metadata]
- Accumulate:
  1. Write receipt to DA
  2. Write metadata to DA (object_id = blake2b(block_0_header))
  3. Create block 1 payload
```

**Round 2 (Timeslot 4)**:
```
PayloadType::Transactions (0x01)
- Extrinsics: [tx2_bytes]
- Witnesses: [block_1_witness]
- Refine:
  1. Execute tx2 → create receipt
  2. Detect block 1 witness
  3. Compute metadata for block 1
  4. Create metadata write intent
  5. Return: ExecutionEffects = [receipt, metadata]
- Accumulate:
  1. Write receipt to DA
  2. Write metadata to DA
  3. Create block 2 payload
```

### Critical Design Constraints

1. **Witnesses require accumulate**: Block witnesses are only available AFTER accumulate writes them to DA. Cannot witness a block that was just created in the current timeslot.

2. **Metadata lag**: Block N's metadata is computed during timeslot N+1 (when block N+1 is being created). This is acceptable because metadata is only needed for queries, not for block creation.

3. **Genesis special case**: Block 0 witness is included in genesis PayloadBlocks, but metadata is computed in Round 1 when first transaction is processed.

4. **Test implications**: First transaction round (round 0) cannot witness block 0 until after accumulate completes. Tests must either:
   - Not witness block 0 in first round (use startBlock=0, endBlock=0)
   - Manually trigger accumulate between genesis and round 0
   - Accept that block 0 metadata is computed in round 1

### Current Status

**Implemented**:
- ✅ `is_block_number_state_witness()` helper in block.rs
- ✅ Block witness detection in PayloadTransactions (refiner.rs)
- ✅ Metadata computation and WriteIntent creation
- ✅ Go-side witness collection in SubmitEVMTransactions
- ✅ PayloadBlocks simplified to genesis-only

**Testing**:
- ⚠️ Test failing: Block 1 has 0 transactions when it should have 1
- Issue: Accumulate creates new block before receipts are added
- Need to debug block creation timing in accumulate phase

## Genesis Bootstrap (Block 0)

### Overview

Genesis (block 0) initializes the EVM service state using bootstrap extrinsics instead of regular transactions. The genesis flow uses `PayloadType::Blocks` with RAW witnesses and follows the same refine → accumulate pattern as normal blocks.

### Genesis Execution Flow

**Entry Point**: `BlockRefiner::from_work_item()` with `PayloadType::Blocks`
- Located in: `refiner.rs`
- Routes to: `refine_blocks_payload()`

**Process**:
1. **Parse Payload**: Work item payload = `0x02` (type) + extrinsics_count (u32 LE) + witness_count (u16 LE)
2. **Load RAW Witnesses**: State witnesses in RAW format (no ObjectRef, just raw Merkle proof)
   - BLOCK_NUMBER_KEY witness (always present)
   - Block witnesses for each block being submitted
3. **Verify Witnesses**: Each RAW witness Merkle proof is verified before parsing
4. **Execute Bootstrap**: Process bootstrap extrinsics ('A' for add code, 'K' for set storage)
5. **Return Effects**: ExecutionEffects with code/storage/SSR write intents

### Bootstrap Extrinsics

**Format**: Custom commands for genesis state initialization

**Supported Commands** (`genesis.rs`):

1. **'A' - Deploy Contract**:
   ```
   Format: [0x41][address:20][code_len:4 LE][code:N]
   ```
   - Deploys bytecode to specified address
   - No constructor execution (raw code deployment)
   - Creates code object + empty SSR header

2. **'K' - Set Storage**:
   ```
   Format: [0x4B][address:20][key:32][value:32]
   ```
   - Sets storage slot for contract
   - Updates SSR and creates/updates storage shards
   - Can set balance/nonce via precompile 0x01

**Execution** (`genesis.rs:30-150`):
- Parse all bootstrap extrinsics
- Group by contract address
- Create code objects for each contract
- Build storage shards per contract
- Generate SSR metadata
- Return ExecutionEffects with all write intents

### Genesis Block Object

**Current Implementation**: Genesis does NOT emit block object
- No `EvmBlockPayload` write intent created
- Accumulate phase stores code/storage but no block metadata
- `eth_getBlockByNumber(0)` may not work

**Planned Enhancement**: Emit genesis block object
```rust
EvmBlockPayload {
    number: 0,
    parent_hash: [0u8; 32],
    logs_bloom: [0u8; 256],
    transactions_root: empty_bmt_root,
    state_root: [0u8; 32],  // Placeholder until JAM state root available
    receipts_root: empty_bmt_root,
    miner: [0u8; 20],
    extra_data: [0u8; 32],
    size: 476,
    gas_limit: 30_000_000,
    gas_used: 0,
    timestamp: 0,
    tx_hashes: vec![],
    receipt_hashes: vec![],
}
```

### Accumulate Phase

**Entry Point**: `BlockAccumulator::accumulate()`
- Detects genesis via envelope analysis
- Processes code/storage write intents
- Exports to DA segments
- Updates service storage

**Genesis-Specific Handling** (`accumulator.rs`):
- No block metadata storage (no block object emitted)
- Code objects stored with ObjectKind::Code
- Storage shards indexed by SSR
- Balance/nonce stored in precompile 0x01 shards

**Re-Bootstrap Prevention**:
- Block number tracking prevents multiple genesis
- Once any block is accumulated, genesis rejected
- Validation in `accumulator.rs:79-89`

### State Initialization

**Contract Deployment**:
1. Bootstrap 'A' command specifies address + bytecode
2. Code exported to DA as code object
3. ObjectRef stored in service storage
4. Empty SSR header created for storage

**Storage Setup**:
1. Bootstrap 'K' commands specify key-value pairs
2. Grouped by contract address
3. Shards created (up to 63 entries per 4KB shard)
4. SSR header tracks shard mapping

**Balance/Nonce Setup**:
- Use 'K' commands targeting precompile address 0x01
- Balance key: `keccak256(account_address)`
- Nonce key: `keccak256(account_address || "nonce")`

### Example Bootstrap Sequence

```rust
// Deploy contract at 0x1234...
[0x41][0x12 0x34 ... (20 bytes)][0x64 0x00 0x00 0x00][bytecode (100 bytes)]

// Set storage for contract 0x1234...
[0x4B][0x12 0x34 ... (20 bytes)][storage_key (32)][storage_value (32)]

// Set balance for EOA 0xabcd...
[0x4B][0x01 0x00 ... (20 bytes)][keccak256(0xabcd...)][balance (32 bytes)]

// Set nonce for EOA 0xabcd...
[0x4B][0x01 0x00 ... (20 bytes)][keccak256(0xabcd... || "nonce")][nonce (32 bytes)]
```

### Security & Authorization

**Current Status**: No authorization implemented
- Any work package with `PayloadType::Genesis` triggers bootstrap
- No signature verification on bootstrap extrinsics
- Relies on JAM work package admission control

**Planned Enhancements** (deferred):
1. Issuer signature verification per bootstrap extrinsic
2. Hard-coded genesis hash validation
3. Authorized genesis issuer public key

### Determinism

**Guarantee**: Bootstrap execution is deterministic
- EVM execution deterministic (same input → same output)
- SSR metadata serialization canonical
- Storage shard ObjectIDs derived from content (deterministic)
- All validators produce identical post-bootstrap state

### Future Work

**Pending Features**:
- [ ] Emit genesis block object for RPC compatibility
- [ ] Compute and validate post-bootstrap state root
- [ ] Bootstrap extrinsic signature verification
- [ ] Hard-coded genesis hash checks
- [ ] Constructor execution for deployed contracts
- [ ] Native balance allocation (without storage keys)

---
## Data Flow Summary

### Complete Transaction and Block Flow

```
┌─────────────────────────────────────────────────────────────┐
│ Phase 1 (Refine)                                            │
├─────────────────────────────────────────────────────────────┤
│ 1. Parse payload metadata (type + tx_count)                 │
│ 2. Load and verify state witnesses                          │
│ 3. Create EVM backend with environment                      │
│ 4. Execute transactions → Create receipts                   │
│    • For each transaction:                                  │
│      - Compute tx_hash = keccak256(RLP tx)                  │
│      - Execute via EVM                                      │
│      - Create TransactionReceiptRecord                      │
│    • overlay.deconstruct() → emit_receipts                  │
│    • Generate WriteIntent per receipt (object_id=tx_hash)   │
│ 5. Export write intent payloads to DA segments              │
│ 6. Return serialized ExecutionEffects                       │
└─────────────────────────────────────────────────────────────┘
                           ↓
┌─────────────────────────────────────────────────────────────┐
│ Phase 2B (Accumulate)                                       │
├─────────────────────────────────────────────────────────────┤
│ 1. BlockAccumulator::new()                                  │
│    • collect_envelopes() - Deserialize ExecutionEffects     │
│    • EvmBlockPayload::read() - Load or create block         │
│ 2. Process write intents → Export to DA                     │
│    • For each Receipt WriteIntent:                          │
│      - current_block.add_receipt() - Add to block           │
│      - Export payload to DA segments                        │
│      - ObjectRef::write() - Store (key=tx_hash)             │
│    • For other objects (code, storage):                     │
│      - ObjectRef::write() - Store to DA                     │
│ 3. Block finalization                                       │
│    • current_block.finalize() - Compute Merkle roots        │
│    • current_block.write() - Serialize and store block      │
│ 4. Update BLOCK_NUMBER_KEY with parent_hash                 │
│ 5. Return parent_hash as accumulate root                    │
└─────────────────────────────────────────────────────────────┘
                           ↓
┌─────────────────────────────────────────────────────────────┐
│ RPC Query (eth_getBlockByNumber / eth_getTransactionReceipt)│
├─────────────────────────────────────────────────────────────┤
│ Block Query:                                                │
│ 1. Compute block_key = 0xFF...FF{block_number}             │
│ 2. Read block metadata from service storage                │
│ 3. Extract state_root + tx_hashes                          │
│ 4. For each tx_hash, lookup receipt ObjectRef              │
│ 5. Fetch receipt payloads from DA                          │
│ 6. Construct EthereumBlock response (JSON-RPC)             │
│                                                             │
│ Receipt Query:                                              │
│ 1. Lookup ObjectRef from service storage (key=tx_hash)     │
│ 2. Fetch receipt payload from DA segments                  │
│ 3. Deserialize and return receipt                          │
└─────────────────────────────────────────────────────────────┘
```

---

## Key Implementation Files

**Rust (services/evm/src/)**:
- `main.rs` - Service entry points (refine/accumulate dispatch)
- `refiner.rs` - Transaction execution and block construction
  - `PayloadType` enum and `parse_payload_metadata()`
  - `BlockRefiner::from_work_item()` - Main orchestrator
  - `BlockRefiner::refine_payload_transactions()` - TX execution and receipt creation
- `receipt.rs` - Receipt structures and RLP encoding
  - `TransactionReceiptRecord` - Receipt metadata structure
  - `encode_canonical_receipt_rlp()` - Ethereum-compatible RLP encoding
  - `compute_per_receipt_logs_bloom()` - Bloom filter computation
- `block.rs` - EvmBlockPayload structure and serialization
  - `EvmBlockPayload` struct definition
  - `read()` - Load or create block from storage
  - `add_receipt()` - Add receipt to block and update metadata
  - `finalize()` - Compute transactions_root and receipts_root
  - `write()` - Persist block to storage
  - `serialize_header()` - Block hash computation (476 bytes)
  - `serialize()` / `deserialize()` - Full payload encoding
  - `read_blocknumber_key()` / `write_blocknumber_key()` - BLOCK_NUMBER_KEY operations
- `accumulator.rs` - Block and receipt storage
  - `BlockAccumulator::accumulate()` - Main entry point
  - `BlockAccumulator::new()` - Initialize with loaded block
  - `collect_envelopes()` - Deserialize execution effects from inputs
- `backend.rs` - Receipt emission to DA
  - `emit_receipts()` - Serializes receipts as write intents
- `writes.rs` - ExecutionEffects serialization
  - `serialize_execution_effects()` - Refine output format
  - `ObjectCandidateWrite` - Metadata container with receipt payloads
- `bmt.rs` - Binary Merkle Trie implementation

**Rust (services/utils/src/)**:
- `effects.rs` - ExecutionEffects, WriteIntent, ObjectRef
- `functions.rs` - Logging and argument parsing

**Go (node/)**:
- `node_evm_block.go` - Block deserialization and RPC handlers
- `statedb/statedb.go` - Historical state reconstruction

---

## Storage Format Reference

### Service Storage Keys

1. **Block Number Key** (current block):
   - Key: `BLOCK_NUMBER_KEY` = `0xFF` repeated 32 times
   - Value: `block_number (4 bytes LE) || parent_hash (32 bytes)`
   - Updated by: `EvmBlockPayload::write_blocknumber_key()`

2. **Block Payload Key** (block by number):
   - Key: `0xFF` repeated 28 times + `block_number (4 bytes LE)`
   - Value: Complete `EvmBlockPayload` (no ObjectRef prefix)
   - Written during accumulate phase via `EvmBlockPayload::write()`

3. **Block Metadata Key** (computed roots):
   - Key: Block hash (32 bytes) computed via `Blake2b-256(432-byte header)`
   - Value: `EvmBlockMetadata` (96 bytes): `transactions_root || receipts_root || mmr_root`
   - Written during accumulate phase via `EvmBlockMetadata::write()`
   - Read via `ReadServiceStorage()` in Go (NOT `ReadStateWitnessRaw`)

4. **Transaction Receipt Key**:
   - Key: `tx_hash` (32 bytes) - keccak256 of RLP-encoded transaction
   - Value: `ObjectRef` (64 bytes) pointing to receipt in DA

---

## Debugging

### Receipts Not Found

1. **Check transaction execution logs**:
   - `Transaction {idx} completed` - Confirms execution
   - `Transaction {idx} committed` - Confirms state commitment
   - Look for transaction errors or revert messages

2. **Check receipt creation**:
   - Each transaction should produce a `TransactionReceiptRecord`
   - Logged in `overlay.deconstruct()` output
   - Verify `receipts.len()` matches transaction count

3. **Check write intent generation**:
   - `ExecutionEffects tally` should show Receipt objects
   - Use `effects.show_tally_object_kinds()` to see counts

4. **Check accumulate storage**:
   - Look for segment export logs
   - Verify ObjectRef written to service storage

### Receipt Data Corruption

1. **Verify serialization**:
   - Check receipt payload length matches expectation
   - Ensure RLP encoding is valid
   - Validate log data structure

2. **Check DA segment allocation**:
   - `index_start` < `index_end`
   - Segment ranges don't overlap
   - Payload length matches actual data size

3. **Witness verification failures**:
   - `Failed to deserialize or verify witness` - Invalid Merkle proof
   - Check state root matches expected value
   - Verify witness payload integrity

---

## Future Implementation Tasks

| Task | Status | Notes |
|------|--------|-------|
| Block payload serialization | ✅ Complete | `block.rs::EvmBlockPayload` |
| Receipt BMT root computation | ✅ Complete | Uses canonical RLP encoding |
| Logs bloom aggregation | ✅ Complete | EIP-2159 compliant |
| DA segment export | ✅ Complete | Via `export_effect()` |
| Receipt deserialization | ⚠️ Stub | `from_candidate()` returns None |
| Block witness ingestion | ❌ Pending | PayloadType::Block not implemented |
| Historical state queries | ⚠️ Partial | Go side `NewStateDBFromStateRoot()` |

---

## External References

- [JAM Graypaper](https://graypaper.com/) - JAM specification
- [EIP-2159](https://eips.ethereum.org/EIPS/eip-2159) - Ethereum logs bloom filter
- [EIP-2718](https://eips.ethereum.org/EIPS/eip-2718) - Typed transaction envelopes
- [EIP-2930](https://eips.ethereum.org/EIPS/eip-2930) - Access list transactions
- [EIP-1559](https://eips.ethereum.org/EIPS/eip-1559) - Fee market transactions
