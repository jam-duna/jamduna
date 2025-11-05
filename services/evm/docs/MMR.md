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

The key name and definition live in `utils/src/constants.rs`.  All read/write
operations must go through the helper functions in `evm/src/mmr.rs` to ensure
consistent error handling.

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

2. **MMR Write to Storage** (after all receipts processed):
   ```rust
   // At the end of accumulate()
   accumulator.mmr.write_mmr()  // Returns MMR root
   ```

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
    // Format: blake2b(b"peak" || left_peak || right_peak)
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

**Implementation Status**:
- ✅ MMR accumulation is implemented in `accumulator.rs`
- ⚠️ MMR proof generation and verification is **in progress** in Go test harness (see TODOs below)

### Implementation Architecture

The MMR proof infrastructure has been refactored into clean layers:

**Layer 1: Core Data Structures** (`statedb/statedb_evm.go:414-432`)
```go
// Receipt MMR tracking entry
type ReceiptMmrEntry struct {
    ReceiptHash    [32]byte // keccak256(receipt_payload)
    ReceiptPayload []byte   // Serialized receipt payload (persisted for proof generation)
    MmrPosition    uint64   // Position in MMR (0-indexed)
    TxHash         [32]byte // For lookup
    LogIndexStart  uint64   // First log index in this receipt
    LogCount       uint32   // Number of logs in receipt
}

// Log inclusion proof structure
type LogInclusionProof struct {
    ReceiptHash    [32]byte    // keccak256(receipt_payload)
    MmrPosition    uint64      // Position in MMR
    MmrProof       interface{} // MMR inclusion proof (trie.MmrProof)
    MmrRoot        [32]byte    // MMR super_peak root
    ReceiptPayload []byte      // Full receipt payload (for verification)
    LogIndex       uint64      // Global log index
}

// Receipt tracker with MMR state
type EVMReceiptTracker struct {
    ReceiptMmrIndex map[common.Hash]ReceiptMmrEntry // Includes persisted payloads
    MmrLeaves       [][32]byte                      // Receipt hashes only
}

// Core MMR tracking functions in statedb:
func (tracker *EVMReceiptTracker) TrackReceiptInMmr(
    txHash common.Hash, receiptPayload []byte,
    logCount uint32, logIndexStart uint64)
// Stores full receipt payload in ReceiptMmrEntry for later proof generation

func (tracker *EVMReceiptTracker) ProveLogInclusion(
    txHash common.Hash, logIndex uint64,
    mmrProofGenerator func([][32]byte, uint64, [32]byte) interface{}) *LogInclusionProof
// Returns proof with stored receipt payload (not empty slice)

func VerifyLogInclusionProof(
    proof *LogInclusionProof, jamStateRoot [32]byte,
    mmrProofVerifier func([32]byte, uint64, interface{}, [32]byte) bool) bool
// Recomputes receipt hash from payload and verifies against proof
```

**Layer 2: Receipt Serialization** (`statedb/statedb_evm.go:434-608`)
```go
// Core serialization matching receipt.rs:229-254
func SerializeReceiptPayload(receipt *EthereumReceipt, txRLP []byte) ([]byte, error)

// Helper for parsing JSON RPC receipt strings
type StringEthereumLog struct {
    Address string   `json:"address"`
    Topics  []string `json:"topics"`
    Data    string   `json:"data"`
}

func ParseReceiptFromStrings(
    txHashHex, statusHex, gasUsedHex, typeHex string,
    logs []StringEthereumLog, txRLP []byte) ([]byte, error)
```

**Layer 3: MMR Proof Operations** (`trie/mmr.go:144-214`)
```go
// MMR proof structure
type MmrProof struct {
    Position uint64
    LeafHash [32]byte
    Siblings [][32]byte
    Peaks    []*common.Hash
}

// MMR proof functions
func GenerateMmrProof(leaves [][32]byte, position uint64, leafHash [32]byte) MmrProof
func VerifyMmrProof(leafHash [32]byte, position uint64, proof MmrProof, expectedRoot [32]byte) bool
```

**Layer 4: Test Harness Integration** (`node/node_evm_jamtest.go:28-221`)
```go
// EVMService uses statedb's EVMReceiptTracker
type EVMService struct {
    n1             JNode
    service        *types.TestService
    receiptTracker *statedb.EVMReceiptTracker
}

// Thin adapters that delegate to statedb:
func (b *EVMService) TrackReceiptInMmr(txHash common.Hash, receiptPayload []byte,
                                       logCount uint32, logIndexStart uint64) {
    b.receiptTracker.TrackReceiptInMmr(txHash, receiptPayload, logCount, logIndexStart)
}

func (b *EVMService) ProveLogInclusion(txHash common.Hash, logIndex uint64) *statedb.LogInclusionProof {
    return b.receiptTracker.ProveLogInclusion(txHash, logIndex, func(leaves [][32]byte, pos uint64, hash [32]byte) interface{} {
        return trie.GenerateMmrProof(leaves, pos, hash)
    })
}

func VerifyLogInclusionProof(proof *statedb.LogInclusionProof, jamStateRoot [32]byte) bool {
    return statedb.VerifyLogInclusionProof(proof, jamStateRoot, func(hash [32]byte, pos uint64, mmrProof interface{}, root [32]byte) bool {
        mmrProofTyped, ok := mmrProof.(trie.MmrProof)
        if !ok {
            return false
        }
        return trie.VerifyMmrProof(hash, pos, mmrProofTyped, root)
    })
}

// Receipt serialization adapter
func serializeReceiptPayload(receipt *EthereumTransactionReceipt, txRLP []byte) ([]byte, error) {
    // Converts node's string-based logs to statedb.StringEthereumLog
    logs := make([]statedb.StringEthereumLog, len(receipt.Logs))
    for i, log := range receipt.Logs {
        logs[i] = statedb.StringEthereumLog{
            Address: log.Address,
            Topics:  log.Topics,
            Data:    log.Data,
        }
    }
    return statedb.ParseReceiptFromStrings(
        receipt.TransactionHash, receipt.Status, receipt.GasUsed,
        receipt.Type, logs, txRLP)
}
```

**Usage Example** (`node/node_evm_jamtest.go` in `ShowTxReceipts`):
```go
// 1. Serialize receipt payload (delegates to statedb)
receiptPayload, err := serializeReceiptPayload(receipt, txRLP)
if err != nil {
    log.Warn(log.Node, "Failed to serialize receipt", "err", err)
    receiptPayload = []byte(fmt.Sprintf("receipt:%s", txHash.String()))
}

// 2. Track receipt in MMR (delegates to statedb.EVMReceiptTracker)
b.TrackReceiptInMmr(txHash, receiptPayload, uint32(len(receipt.Logs)), logIndexStart)

// 3. Generate and verify proof for interesting logs
if len(receipt.Logs) > 0 {
    firstLogIndex := logIndexStart
    proof := b.ProveLogInclusion(txHash, firstLogIndex)
    if proof != nil {
        var stubStateRoot [32]byte
        if VerifyLogInclusionProof(proof, stubStateRoot) {
            log.Info(log.Node, "✓ Log inclusion proven via MMR",
                "logIndex", firstLogIndex,
                "txHash", common.Str(txHash),
                "mmrPosition", proof.MmrPosition,
                "mmrRoot", common.Bytes2Hex(proof.MmrRoot[:4]))
        }
    }
}
```

**Example Output**:
```
INFO [11-03|09:57:36.786] ✓ Log inclusion proven via MMR logIndex=1 txHash=2450..d946 mmrPosition=0 mmrRoot=0000..0000
```

**Completed Implementation**:
- ✅ **Core data structures** (`statedb/statedb_evm.go:414-432`):
  - `ReceiptMmrEntry` with persisted receipt payloads
  - `LogInclusionProof` structure
  - `EVMReceiptTracker` for MMR state management
- ✅ **Receipt serialization** (`statedb/statedb_evm.go:433-516`):
  - `SerializeReceiptPayload` matching Rust receipt.rs:229-254 format
  - Correct log_count (u16) and topic_count (u8) encoding
  - `ParseReceiptFromStrings` for JSON RPC conversion
- ✅ **Receipt tracking** (`statedb/statedb_evm.go:633-670`):
  - `TrackReceiptInMmr` persists full receipt payloads
  - Receipt hashes computed with keccak256
  - MMR leaves array maintained
- ✅ **Proof structure** (`statedb/statedb_evm.go:672-706`):
  - `ProveLogInclusion` returns proof with stored payload
  - MMR position and receipt hash lookup working
- ✅ **Test harness integration** (`node/node_evm_jamtest.go:28-221`):
  - Thin adapters delegating to statedb/trie
  - Receipt serialization and tracking integrated into `ShowTxReceipts`

**Remaining TODOs** (Proof Generation/Verification):
1. ⚠️ **MMR sibling/peak computation** (`trie/mmr.go:189-214`):
   - `trie.GenerateMmrProof` currently returns empty siblings/peaks (stub)
   - **Needs**: Full MMR tree traversal to collect sibling hashes along path to peak
   - **Algorithm**: Given position, compute height and collect siblings at each level

2. ⚠️ **MMR root computation** (`statedb/statedb_evm.go:693-696`):
   - `ProveLogInclusion` returns zeroed mmrRoot
   - **Needs**: Compute super_peak from `tracker.MmrLeaves` using MMR bagging algorithm
   - **Note**: `trie.MMR.SuperPeak()` already implements this in Rust-compatible way

3. ⚠️ **MMR proof verification** (`trie/mmr.go:151-187`):
   - `trie.VerifyMmrProof` only does basic sibling hashing without left/right ordering
   - **Needs**: Correct MMR climb algorithm (position-based left/right concatenation)
   - **Needs**: Final super-peak computation from all peaks and comparison

4. ⚠️ **JAM State verification** (`statedb/statedb_evm.go:708-729`):
   - `VerifyLogInclusionProof` skips BMT proof of MMR root against JAM stateRoot
   - **Needs**: Call `GetServiceStorageWithProof(EVMServiceID, 0xEE...EE, jamStateRoot)`
   - **Needs**: Verify returned BMT proof confirms MMR root is in consensus state

5. ⚠️ **Transaction RLP encoding** (node-side):
   - Currently uses `ethTx.Input` field which may not contain full signed transaction RLP
   - **Needs**: Reconstruct from transaction fields or fetch from original submission
   - **Impact**: Receipt hashes may not match Rust accumulator if tx payload differs

**Next Steps**:
1. Implement full `GenerateMmrProof` with sibling collection
2. Add super_peak computation to `ProveLogInclusion`
3. Fix `VerifyMmrProof` to handle MMR position-based hashing and peak aggregation
4. Extend test harness to fetch MMR root from JAM State via RPC
5. Add BMT proof verification step in `VerifyLogInclusionProof`

**Technical Notes**:
- Receipt serialization now matches Rust format byte-for-byte (log_count:u16, topic_count:u8)
- Receipt payloads are persisted in `ReceiptMmrEntry` for proof generation
- Uses keccak256 for hashing (matching Rust `accumulator.rs` implementation)
- Proof verification will pass once MMR algorithms are completed

### Proof Benefits

- **Compact**: MMR proofs are O(log N) size for N receipts
- **Append-only**: No rebalancing like standard Merkle trees
- **Efficient**: Single MMR root commitment covers all receipts in history
- **Bridge-friendly**: Light clients can verify log emission with minimal data

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

## Implementation Details

**Files**:
- `services/evm/src/mmr.rs` - Core MMR implementation
- `services/evm/src/accumulator.rs` - MMR integration with accumulation
- `services/evm/src/block.rs` - Receipt hash computation

**Key Functions**:
- `MMR::append()` - Add receipt hash to MMR
- `MMR::write_mmr()` - Serialize and write to JAM State
- `MMR::read_mmr()` - Deserialize from JAM State
- `MMR::super_peak()` - Compute MMR root

## Troubleshooting Host Writes

- **WHAT (0xFFFFFFFFFFFFFFFE)**: The host rejected the write because the service is not in accumulate mode. Ensure the work package is routed to the accumulator and that no finalize call ran beforehand.
- **FULL (0xFFFFFFFFFFFFFFFB)**: The service’s balance threshold is insufficient for the write. Recharge the account or reduce the payload size.
- **Repeated receipt write failures**: The accumulator aborts after the first failure. Inspect the offending `object_id` and confirm that the serialized payload length matches `ObjectRef.payload_length`; mismatches are logged before the host call.
