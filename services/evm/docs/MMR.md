
# Merkle Mountain Range (MMR) in JAM EVM Service

## Overview

The EVM service uses a Merkle Mountain Range (MMR) to create a cryptographically verifiable chain of receipt hashes. This allows efficient proof generation and verification for transaction receipts across blocks.

## Accumulate Inputs and Witness Formats

`BlockAccumulator::new` (services/evm/src/accumulator.rs) always pulls accumulate inputs through `collect_envelopes`, which in turn feeds the payload to `deserialize_execution_effects` (services/evm/src/writes.rs). The parser expects the modern ExecutionEffects layout (`export_count | gas_used | write_count | ObjectCandidateWrite…`). If the bytes still follow the legacy `StateWitness` encoding—either the historical `RAW` form without object refs or the fully RLP-encoded witness—the parse fails and the input is discarded. Because `collect_envelopes` returns `None` when every input fails to parse, the accumulator effectively rejects legacy witnesses; refine must be upgraded to emit ExecutionEffects before accumulation succeeds.

## What is Written to JAM State

### MMR Storage Key

The MMR is stored in JAM State using a dedicated storage key:

```rust
pub const MMR_KEY: [u8; 32] = [0xEE; 32];
```

The key name and definition live in `utils/src/constants.rs`. All read/write operations must go through the helper functions in `evm/src/mmr.rs` to ensure consistent error handling.

### MMR Data Structure

The MMR consists of:
- **Peaks**: A vector of optional 32-byte hashes representing the peaks of the MMR tree
- **Serialization Format**:
  - 4 bytes: peak count (little-endian u32)
  - N × 33 bytes: each peak (1 byte flag + 32 bytes hash)
    - Flag = 1: Some(hash)
    - Flag = 0: None

Example serialization:
```
[03 00 00 00] [01] [32 bytes hash] [01] [32 bytes hash] [00] [32 bytes padding]
 ^peak_count   ^Some              ^Some              ^None
```

### MMR Peaks are Sufficient for All Operations

**Key Principle**: The MMR peaks stored in JAM State are the **only persistence required**. No separate storage or caching of historical leaves is needed.

When another node fetches the serialized MMR from JAM State and deserializes it:
- The peaks alone are sufficient to continue appending new leaves
- The peaks alone are sufficient to generate proofs for any historical position
- No replay of historical receipts is required
- The MMR compacts a large number of leaves into peaks by design

This is because the MMR structure allows reconstructing the tree topology from the peaks themselves, enabling both:
1. Appending new receipt hashes (which may combine with existing peaks)
2. Generating inclusion proofs for historical receipts (by traversing the implicit tree structure)

## When Data is Written

### 1. During Accumulation (`accumulate` function)

The MMR is updated during the accumulation phase when processing transaction receipts:
**Location**: `services/evm/src/accumulator.rs`

#### Step-by-Step Process:

1. **Receipt Processing** (for each receipt):
   ```rust
   // In accumulate_receipt()
   let (receipt_hash, _) = self.current_block.add_receipt(candidate, record, ...);
   self.mmr.append(receipt_hash);  // Append receipt hash to MMR
   ```

2. **MMR Write to Storage and Block Finalization** (after all receipts processed):
   ```rust
   // At the end of accumulate() in accumulator.rs
   // Write MMR to storage and return MMR root
   accumulator.current_block.mmr_root = accumulator.mmr.write_mmr()?;

   // Update EvmBlockPayload and write to storage
   accumulator.current_block.write()?;

   Some(accumulator.current_block.mmr_root)
   ```

   The MMR root (super_peak) is stored in the block's `mmr_root` field and becomes part of the block header that is hashed to create the block's Object ID.

### 2. Receipt Hash Computation

Each receipt hash is computed using the canonical Ethereum receipt format:

```rust
// In block.rs::add_receipt()
let receipt_rlp = encode_canonical_receipt_rlp(&record, cumulative_gas, &per_receipt_bloom);
let receipt_hash = keccak256(&receipt_rlp);
```

The receipt hash includes:
- Transaction status (success/failure)
- Cumulative gas used
- Logs bloom filter
- Transaction logs

### 3. MMR Root Calculation

The MMR root (super peak) is computed using the formula from the Graypaper (Equation E.10):

```rust
pub fn super_peak(&self) -> [u8; 32] {
    // Hash all non-None peaks together
    // Format: keccak256(b"peak" || left_peak || right_peak)
}
```

This root is returned from the `accumulate()` function and represents the entire state of all receipts processed.

## Reading from JAM State

### On Service Initialization

When the BlockAccumulator is created, it reads the existing MMR from storage:

```rust
// In accumulator.rs::new()
let mmr = MMR::read_mmr(service_id)?;
```

**Behavior**:
- If `MMR_KEY` not found → returns empty MMR (`MMR::new()`)
- If deserialization fails → returns `None` (accumulation fails)
- Otherwise → returns the deserialized MMR with all peaks

## Error Handling

### Write Failures

The `write_mmr()` function checks for host function error codes:

```rust
if result == WHAT || result == FULL {
    log_error("Failed to write MMR to storage");
    return None;  // Causes accumulation to fail
}
```

**Error Codes**:
- `WHAT` (0xFFFFFFFFFFFFFFFE): Wrong mode (not in Accumulate mode)
- `FULL` (0xFFFFFFFFFFFFFFFB): Insufficient balance threshold

### Read Failures

```rust
if result == WHAT {
    return None;  // Fails accumulation
}
if result == NONE {
    return Some(MMR::new());  // First time, start fresh
}
```

### Receipt Write Failures

Receipt objects are written inside `BlockAccumulator::accumulate_receipt`. The helper builds `[ObjectRef || payload]`, calls the host `write`, and only appends the receipt hash to the MMR after the host confirms success. Any `WHAT`/`FULL` error short-circuits the function, propagating `None` back to `accumulate` and aborting the entire accumulation. No receipt hash is appended when the write fails.

## MMR Properties

### Append-Only Structure

The MMR is append-only, meaning:
- New receipt hashes are always appended
- Existing peaks may be combined when new peaks are added
- The structure never removes data (except during old block deletion)

### Peak Combination

When a new hash is appended at height `n`:
1. If peak at height `n` is empty → place hash there
2. If peak at height `n` is full → combine it with the new hash and recurse to height `n+1`

This maintains the MMR invariant where peaks represent perfect binary trees of different heights.

## Storage Lifecycle

### Creation
- First accumulation for a service creates an empty MMR
- MMR_KEY is written with the first receipt hash

### Updates
- Every accumulation appends new receipt hashes
- MMR_KEY is overwritten with the updated MMR state

### Deletion
- When blocks older than `MAX_BLOCKS_IN_JAM_STATE` are deleted, the MMR persists
- The MMR maintains the full history across all blocks
- **Note**: Currently, there is no mechanism to prune old receipt hashes from the MMR

`EvmBlockPayload::finalize` (services/evm/src/block.rs) enforces `MAX_BLOCKS_IN_JAM_STATE` by deleting block objects older than the configured horizon via `EvmBlockPayload::delete_block`. The pruning only applies to block payload objects; the MMR itself remains append-only and continues to commit to every historical receipt.

## Example Flow

```
Block 1: Accumulate 3 transactions
├─ tx1 → receipt_hash1 → mmr.append(receipt_hash1)
├─ tx2 → receipt_hash2 → mmr.append(receipt_hash2)
└─ tx3 → receipt_hash3 → mmr.append(receipt_hash3)
   └─ mmr.write_mmr() → returns mmr_root1

Block 2: Accumulate 2 transactions
├─ Read existing MMR (3 peaks)
├─ tx4 → receipt_hash4 → mmr.append(receipt_hash4)
└─ tx5 → receipt_hash5 → mmr.append(receipt_hash5)
   └─ mmr.write_mmr() → returns mmr_root2
```

## Proof Workflows

### Receipt Inclusion Proofs

The MMR enables efficient cryptographic proofs that a transaction receipt (and its logs) was processed during accumulation. Everything starts by proving that a specific ObjectRef lives inside JAM State.

#### Transaction Receipt Inclusion via ObjectRef

**Current Model**: Receipts embed the full transaction payload, so transaction proofs = receipt proofs.

**Steps**:
1. Derive `receiptObjectID = keccak256(RLP(signed_tx))`
2. Call `GetServiceStorageWithProof(EVMServiceID, receiptObjectID)`
3. Verify the returned BMT proof against the published `stateRoot`
4. Use the metadata stored in the `ObjectRef`:
   - `timeslot` tells you which JAM header anchors the receipt
   - `object_kind` confirms you fetched a receipt (`ObjectKind::Receipt = 0x03`)
   - `evm_block` gives the Ethereum block number
   - `tx_slot` gives the transaction index within that block
   - `gas_used` gives the gas consumed
5. Optionally fetch the raw receipt payload from DA using `work_package_hash`, `index_start`, and `index_end` (while the DA window lasts)

**Receipt Payload Format** (`receipt.rs:229-254`):
```
logs_payload_len (4 bytes LE) - total length of logs_payload below
logs_payload:
  log_count (2 bytes LE, u16) - number of logs
  For each log:
    address (20 bytes)
    topic_count (1 byte, u8) - number of topics
    topics (32 bytes each)
    data_len (4 bytes LE, u32)
    data (variable)
version (1 byte) = 0
tx_hash (32 bytes)
tx_type (1 byte) - EIP-2718 type (0=legacy, 1=EIP-2930, 2=EIP-1559)
success (1 byte) - 1=success, 0=revert
used_gas (8 bytes LE, u64)
tx_payload_len (4 bytes LE, u32)
tx_payload (variable) - full RLP-encoded signed transaction
```

### Log/Event Proofs via MMR Receipt Hash Inclusion

**Design**: The MMR accumulates receipt hashes during accumulation phase. Each receipt's hash is appended to the MMR as it's processed. The MMR root (super_peak) is stored in JAM State at key `0xEE...EE`, providing a single commitment to all receipt hashes.

**How It Proves Log Emission**:

A Merkle Mountain Range inclusion proof can demonstrate that:
1. A specific receipt hash was appended to the MMR at a known position
2. The MMR root matches what's stored in JAM State (provable via stateRoot)
3. Therefore, the receipt (containing the log) was processed during accumulation

**Proof Components**:
1. **Receipt hash**: `keccak256(receipt_payload)` where payload contains all logs
2. **MMR position**: Index in the MMR where this receipt hash was appended
3. **MMR proof**: Sibling hashes needed to recompute the MMR root
4. **MMR root**: The super_peak stored in JAM State at key `0xEE...EE`
5. **Log data**: The specific log from the receipt payload (address, topics, data, logIndex)

**Verification Flow**:
```
1. Extract log from receipt payload:
   - address (20 bytes)
   - topics (32 bytes each)
   - data (variable)
   - logIndex (global log index across all receipts)

2. Compute receipt_hash = keccak256(receipt_payload)

3. Verify MMR inclusion:
   - Given: receipt_hash, mmr_position, mmr_proof
   - Recompute MMR root by climbing the mountain
   - Check computed root == stored MMR root

4. Verify MMR root in JAM State:
   - Call GetServiceStorageWithProof(EVMServiceID, 0xEE...EE)
   - Verify BMT proof against stateRoot
   - Confirms MMR root is consensus-anchored

5. Result: Log was emitted in a finalized receipt
```

## Implementation Architecture

The MMR proof infrastructure is organized into clean layers:

### Layer 1: Core MMR Implementation (`trie/mmr.go`)

**MMR Structure**:
```go
type MMR struct {
    Peaks  types.Peaks   // Array of optional 32-byte hashes
    leaves []common.Hash // Internal leaf cache for proof generation
}
```

**Core Functions**:
- `NewMMR()` - Creates empty MMR
- `NewMMRFromBytes(data []byte)` - Deserializes MMR from Rust format
- `Append(data *common.Hash)` - Adds receipt hash to MMR (implements Equation E.8)
- `SuperPeak()` - Computes MMR root (implements Equation E.10)
- `SetLeaves(leaves []common.Hash)` - Sets leaf cache for proof generation

### Layer 2: MMR Proof Structure (`trie/mmr.go:209-257`)

**MMRProof Structure**:
```go
type MMRProof struct {
    Position    uint64        // Position in MMR (leaf index)
    LeafHash    common.Hash   // The receipt_hash being proven
    Siblings    []common.Hash // Sibling hashes for tree climbing
    SiblingLeft []bool        // True if sibling is on left, false if on right
    Peaks       []common.Hash // All MMR peaks in left-to-right order
    PeakIndex   int           // Index of peak containing the leaf
}
```

**Proof Functions**:
```go
func (m *MMR) GenerateProof(position uint64) MMRProof
// Generates inclusion proof for receipt at given position

func (proof MMRProof) Verify(expectedRoot common.Hash) bool
// Verifies proof by climbing tree and checking computed root
```

**Proof Generation Algorithm** (`trie/mmr.go:267-353`):
1. Locate which peak contains the target position
2. Traverse down the peak's binary tree
3. Collect sibling hashes at each level
4. Record sibling positions (left/right)
5. Return proof with all peaks and sibling path

**Proof Verification Algorithm** (`trie/mmr.go:228-257`):
1. Start with leaf hash
2. Combine with siblings in correct order (using `SiblingLeft` flags)
3. Climb to peak using keccak256 hashing
4. Substitute computed peak into peaks array
5. Compute super_peak from all peaks
6. Compare with expected root

### Layer 3: Receipt Serialization (`statedb/statedb_evm.go`)

**Receipt Structures**:
```go
type EthereumReceipt struct {
    TxHash  common.Hash
    Success bool
    GasUsed uint64
    TxType  uint8
    Logs    []EthereumLog
}

type EthereumLog struct {
    Address common.Address
    Topics  []common.Hash
    Data    []byte
}
```

**Serialization Function** (`statedb/statedb_evm.go:526-604`):
```go
func SerializeReceiptPayload(receipt *EthereumReceipt, txRLP []byte) ([]byte, error)
// Matches Rust receipt.rs:229-254 format exactly
```

**Helper for JSON RPC** (`statedb/statedb_evm.go:606-680`):
```go
func ParseReceiptFromStrings(
    txHashHex, statusHex, gasUsedHex, typeHex string,
    logs []StringEthereumLog, txRLP []byte) ([]byte, error)
// Converts JSON RPC string fields to EthereumReceipt
```

### Layer 4: ServiceProof - Two-Layer Verification (`trie/serviceproof.go`)

**ServiceProof Structure**:
```go
type ServiceProof struct {
    ServiceID  uint32      // EVM service ID
    StorageKey []byte      // Storage key (0xEE...EE for MMR)

    // Layer 1: MMR proof (receipt → MMR root)
    MmrProof  MMRProof     // Merkle Mountain Range inclusion proof
    SuperPeak common.Hash  // MMR super_peak root

    // Layer 2: BMT proof (MMR root → JAM State)
    BmtProof  []common.Hash  // Binary Merkle Trie proof path
    StateRoot common.Hash     // JAM consensus state root
}
```

**Two-Layer Verification**:
```
Layer 1 (MMR Proof):
  Input: receipt_hash, mmr_position, mmr_proof
  Verify: receipt_hash is in MMR at given position
  Output: MMR super_peak (root commitment)

Layer 2 (BMT Proof):
  Input: MMR super_peak, storage_key (0xEE...EE), bmt_proof
  Verify: MMR super_peak is stored in JAM State
  Output: JAM state_root (consensus anchor)

Result: Receipt hash is cryptographically anchored to JAM consensus
```

**Proof Generation** (`statedb/statedb_evm.go:682-698`):
```go
func (s *StateDB) GenerateServiceProof(
    serviceID uint32, storageKey []byte, position uint64) (*trie.ServiceProof, error) {
    // 1. Get MMR root from JAM State with BMT proof
    value, bmtProof, stateRoot, ok, err := s.trie.GetServiceStorageWithProof(serviceID, storageKey)

    // 2. Deserialize MMR from peaks (no leaf cache seeding required)
    mmr, err := trie.NewMMRFromBytes(value)

    // 3. Generate combined proof and verify it
    // Note: GenerateServiceProof calls VerifyServiceProof internally (serviceproof.go:72)
    return trie.GenerateServiceProof(mmr, serviceID, storageKey, position, bmtProof, stateRoot)
}
```

**Important**: `GenerateServiceProof` (trie/serviceproof.go:44-77) performs **both verification layers** by calling `VerifyServiceProof` at line 72 before returning the proof. This ensures:
1. The MMR proof correctly verifies the receipt inclusion against the super_peak
2. The BMT proof correctly verifies the MMR root is in JAM State at the given state root

The function returns an error if either verification fails, guaranteeing that only valid proofs are generated.

**Proof Verification** (`trie/serviceproof.go:79-90`):
```go
func VerifyServiceProof(proof *ServiceProof) bool {
    // Layer 1: Verify MMR proof (receipt → MMR root)
    if !proof.MmrProof.Verify(proof.SuperPeak) {
        return false
    }

    // Layer 2: Verify BMT proof (MMR root → JAM State)
    return Verify(proof.ServiceID, proof.StorageKey, proof.SuperPeak.Bytes(),
                  proof.StateRoot.Bytes(), proof.BmtProof)
}
```

### Layer 5: Test Harness Integration (`node/node_evm_jamtest.go`)

**Usage Example**:
```go
// Generate proof for log at position
proof, err := node.GenerateServiceProof(serviceID, statedb.GetMMRStorageKey(), position)
if err != nil {
    log.Warn("Failed to generate proof", "err", err)
    return
}

// Verify proof
if proof.Verify() {
    log.Info("✓ Log inclusion proven via MMR", "logIndex", position)
} else {
    log.Warn("✗ Log inclusion proof verification failed")
}
```

## Implementation Status

### Completed ✅

1. **MMR Core** (`trie/mmr.go`):
   - Append operation (Equation E.8)
   - SuperPeak computation (Equation E.10)
   - Serialization/deserialization matching Rust format
   - Leaf cache for proof generation

2. **MMR Proof** (`trie/mmr.go:209-385`):
   - Full proof generation with sibling collection
   - Proof verification with position-based hashing
   - Peak-based tree traversal

3. **Receipt Serialization** (`statedb/statedb_evm.go:526-680`):
   - Matches Rust receipt.rs:229-254 format
   - Correct log_count (u16) and topic_count (u8) encoding
   - JSON RPC string conversion

4. **ServiceProof** (`trie/serviceproof.go`):
   - Two-layer proof structure
   - Combined MMR + BMT verification
   - Self-contained proof with all necessary data

5. **Test Integration** (`node/node_evm_jamtest.go:256-269`):
   - Proof generation for every log
   - Proof verification in test harness
   - Example output in logs

### Known Limitations ⚠️

1. **MMR Proof Generation from Peaks Alone**: The MMR implementation can generate proofs directly from peaks without requiring historical leaves to be cached. When another node deserializes the MMR from JAM State, the peaks are sufficient to continue appending new leaves and generating proofs. No replay of historical receipts is required.

2. **In-Memory Leaf Cache**: The Go implementation holds a leaf cache in memory during proof generation for convenience, but this is not a requirement. The `GenerateServiceProof` function never needs to seed this cache because the MMR is loaded from JAM State and `mmr.GenerateProof(position)` can operate with just the peaks.

3. **Receipt RLP Encoding**: Test harness currently uses transaction input field which may not contain the full signed transaction RLP. For production, need to reconstruct from transaction fields or fetch from original submission.

4. **ObjectRef.log_index Field Width Overflow** (🔴 High Priority):
   - **Problem**: `ObjectRef.log_index` remains a single byte (u8) but stores global log indices that can exceed 255
   - **Location**: `utils/src/objects.rs:40` (definition), `block.rs:412` (assignment: `candidate.object_ref.log_index = log_index_start as u8`)
   - **Impact**: Blocks with >255 logs will have wrapped log indices in ObjectRef metadata, breaking RPC log index recovery
   - **Example**: A block with `log_index_start = 1000` stores `1000 as u8 = 232`, causing RPC consumers to see incorrect log indices
   - **Status**: Field remains u8; overflow will occur after 255 total logs across all blocks
   - **Required Fix**:
     1. Widen `ObjectRef.log_index` from `u8` to `u64` (or at minimum `u32`)
     2. Update serialization/deserialization methods
     3. Verify/update `SERIALIZED_SIZE` constant (currently 64 bytes)
     4. Update Go side `ObjectRef` in `types/objectref.go`
     5. Handle storage migration if any ObjectRefs exist
   - **Workaround**: Log data in receipt payloads is unaffected; only ObjectRef metadata has wrong indices
   - **Note**: The global log index is written by `EvmBlockPayload::add_receipt` (block.rs:412), but the downstream consumer cannot reconstruct global ordering once the value wraps

## Host Function: Fetching Parent State Root

### Purpose

During accumulation, the EVM service needs to store the JAM state root at block finalization time in the block's `state_root` field. This enables historical state reconstruction and light client queries.

### Implementation

**Rust Side** (`services/utils/src/functions.rs:2050-2070`):
```rust
/// Fetches the parent state root from JAM host using datatype 255
pub fn fetch_parent_state_root() -> HarnessResult<[u8; 32]> {
    const FETCH_DATATYPE_PARENT_STATE_ROOT: u64 = 255;
    let mut buffer = vec![0u8; 32];
    let data = match fetch_data(&mut buffer, FETCH_DATATYPE_PARENT_STATE_ROOT, 0, 0, 32)? {
        Some(buffer) => buffer,
        None => {
            call_log(1, None, "fetch_parent_state_root: host returned no data");
            return Err(HarnessError::HostFetchFailed);
        }
    };

    if data.len() != 32 {
        return Err(HarnessError::TruncatedInput);
    }

    let mut state_root = [0u8; 32];
    state_root.copy_from_slice(&data[..32]);
    Ok(state_root)
}
```

**Go Side** (`statedb/hostfunctions.go:964-966`):
```go
case 255: // Parent state root (JAM state root from parent block)
    v_Bytes = vm.ParentStateRoot.Bytes()
    log.Trace(vm.logging, "FETCH ParentStateRoot", "stateRoot", vm.ParentStateRoot.Hex())
```

**Allowed Modes** (`statedb/hostfunctions.go:848`):
- Datatype 255 is allowed in `ModeAccumulate`
- Returns the JAM state root from the parent block
- Stored in `vm.ParentStateRoot` field, initialized during `ExecuteAccumulate`

### Usage in Block Finalization

**Block Creation** (`services/evm/src/block.rs:384-385`):
```rust
// Fetch entropy and parent state root from host
let entropy = fetch_entropy().ok()?;
let parent_state_root = fetch_parent_state_root().ok()?;

// Finalize previous block with parent state root
let parent_block_hash = current_block.finalize(service_id, entropy, parent_state_root, mmr_root)?;
```

**Finalize Method** (`services/evm/src/block.rs:435`):
```rust
pub fn finalize(&mut self, service_id: u32, entropy: [u8; 32], state_root: [u8; 32], mmr_root: [u8; 32]) -> Option<[u8; 32]> {
    self.transactions_root = self.compute_transactions_root();
    self.receipts_root = self.compute_receipts_root();
    self.state_root = state_root;  // JAM state root for historical lookups
    self.mmr_root = mmr_root;      // MMR super_peak for log proofs
    self.entropy = entropy;
    // ... compute block hash
}
```

### Benefits

1. **Historical State Queries**: Light clients can verify state at specific block heights
2. **State Proofs**: Cryptographic proofs anchored to JAM consensus
3. **Separate Concerns**: `state_root` for state lookups, `mmr_root` for log proofs
4. **Backwards Compatibility**: Supports both state reconstruction and receipt verification

## Use Cases

### Receipt Proofs
The MMR allows proving that a specific receipt exists by providing:
1. The receipt hash
2. A Merkle proof path to a peak
3. The peak included in the MMR root

### Chain Verification
The MMR root serves as a compact commitment to all receipts, allowing:
- Efficient verification of historical receipts
- Light client proofs without storing all receipt data
- Cross-chain receipt verification

### Bridge Integration
ServiceProof enables:
- Self-contained proofs for light clients
- Two-layer security (MMR + BMT)
- Consensus-anchored verification against JAM state root
- No need to sync full state

## Implementation Details

**Rust Files**:
- `services/evm/src/mmr.rs` - Core MMR implementation
- `services/evm/src/accumulator.rs` - MMR integration with accumulation
- `services/evm/src/block.rs` - Receipt hash computation
- `services/evm/src/receipt.rs` - Receipt serialization

**Go Files**:
- `trie/mmr.go` - MMR and MMRProof implementation
- `trie/serviceproof.go` - ServiceProof two-layer verification
- `statedb/statedb_evm.go` - Receipt serialization and proof generation
- `node/node_evm_jamtest.go` - Test harness integration

**Key Functions**:
- `MMR::append()` - Add receipt hash to MMR
- `MMR::write_mmr()` - Serialize and write to JAM State
- `MMR::read_mmr()` - Deserialize from JAM State
- `MMR::super_peak()` - Compute MMR root
- `MMR::GenerateProof()` - Generate inclusion proof
- `MMRProof::Verify()` - Verify inclusion proof
- `ServiceProof::Verify()` - Two-layer verification

## Troubleshooting Host Writes

- **WHAT (0xFFFFFFFFFFFFFFFE)**: The host rejected the write because the service is not in accumulate mode. Ensure the work package is routed to the accumulator and that no finalize call ran beforehand.
- **FULL (0xFFFFFFFFFFFFFFFB)**: The service's balance threshold is insufficient for the write. Recharge the account or reduce the payload size.
- **Repeated receipt write failures**: The accumulator aborts after the first failure. Inspect the offending `object_id` and confirm that the serialized payload length matches `ObjectRef.payload_length`; mismatches are logged before the host call.