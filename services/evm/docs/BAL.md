# Block Access Lists (BAL) for JAM EVM Service

## Implementation Status

### ✅ Completed (Phase 1 & 2)

**Data Structures & Core BAL Construction** - Fully implemented in Go:

1. **Witness Format Augmentation** ([types/storage.go](../../types/storage.go), [storage/witness.go](../../storage/witness.go))
   - ✅ Added `TxIndex` field to `UBTRead` struct (types/storage.go:150)
   - ✅ Updated witness serialization from 157 to 161 bytes per entry (storage/witness.go:199-266)
   - ✅ Modified `KeyMetadata` to include `TxIndex` (storage/witness.go:380)
   - ✅ Updated `extractKeyMetadataFromReads` and `extractKeyMetadata` to propagate TxIndex

2. **BAL Data Structures** ([storage/block_access_list.go:18-78](../../storage/block_access_list.go#L18-L78))
   - ✅ `BlockAccessList`, `AccountChanges`, `StorageRead`, `SlotChanges`, `SlotWrite`
   - ✅ `BalanceChange`, `NonceChange`, `CodeChange`
   - ✅ All using `common.Hash` for 32-byte values, `[8]byte` for nonces

3. **Witness Parsing** ([storage/block_access_list.go:86-250](../../storage/block_access_list.go#L86-L250))
   - ✅ `parseAugmentedPreStateReads()` - Parses 161-byte entries with txIndex
   - ✅ `parseAugmentedPostStateWrites()` - Parses 161-byte entries with txIndex
   - ✅ `ReadEntry` and `WriteEntry` structs

4. **BAL Construction Algorithm** ([storage/block_access_list.go:233-469](../../storage/block_access_list.go#L233-L469))
   - ✅ `buildSlotChanges()` - Groups storage writes by address/key
   - ✅ `buildBalanceChanges()` - Extracts balance changes from BasicData
   - ✅ `buildNonceChanges()` - Extracts nonce changes from BasicData
   - ✅ `buildCodeChanges()` - Extracts code deployments with CodeSize from BasicData cross-reference ([storage/block_access_list.go:308-345](../../storage/block_access_list.go#L308-L345))
   - ✅ `classifyStorageAccesses()` - Separates read-only from modified slots
   - ✅ `assembleAccountChanges()` - Deterministic assembly with sorting
   - ✅ `BuildBlockAccessList()` - Main construction function

5. **Hash & Utilities** ([storage/block_access_list.go:77-84, 472-504](../../storage/block_access_list.go#L77-L84))
   - ✅ `Hash()` - RLP encoding + Blake2b hash
   - ✅ `SplitWitnessSections()` - Parse dual-proof witness into pre/post sections

### ✅ Phase 3 Complete: Transaction Index Tracking

**TX Index Propagation** - Fully implemented for both reads and writes:

1. **Read Attribution** - UBT state fetches track which transaction triggered each read
   - ✅ Rust→Go polkavm FFI passes `tx_index` parameter ([services/evm/src/ubt_host.rs](../../services/evm/src/ubt_host.rs), [statedb/hostfunctions.go](../../statedb/hostfunctions.go))
   - ✅ All `UBTRead` entries populated with correct `TxIndex` ([storage/storage.go](../../storage/storage.go))

2. **Write Attribution** - Contract witness blob embeds per-write tx_index (BREAKING CHANGE)
   - ✅ Blob format: `[20B address][1B kind][4B payload_length][4B tx_index][payload]` (29-byte header)
   - ✅ Go side correctly reads and uses tx_index from blob ([storage/witness.go:440-547](../../storage/witness.go#L440-L547))
   - ✅ Rust side tracks tx_index during transaction execution ([services/evm/src/state.rs:433-443](../state.rs#L433-L443))
   - ✅ All write methods updated to record tx_index ([services/evm/src/state.rs:633-730](../state.rs#L633-L730))
   - ✅ Writes correctly attributed to transaction that caused them ([services/evm/src/backend.rs:410-551](../backend.rs#L410-L551))
   - ⚠️ **Note**: Storage shard writes use tx_index=0 (aggregate multiple slots, individual slots have correct attribution in contract witness)

### ✅ Phase 4 Complete: Builder/Guarantor Integration

**Builder/Guarantor Integration** - Fully implemented:

1. **Builder Integration** ([statedb/refine.go:543-560](../../statedb/refine.go#L543-L560))
   - ✅ Calls `ComputeBlockAccessListHash()` after ubt witness generation
   - ✅ Updates block payload with computed BAL hash before export ([statedb/refine.go:1052-1080](../../statedb/refine.go#L1052-L1080))
   - ✅ Logs BAL statistics (account count, total changes)
   - ✅ **Note**: Single refine processes entire block with tx_index loop in Rust, NOT per-transaction refines

2. **Guarantor Integration** ([statedb/refine.go:360-400](../../statedb/refine.go#L360-L400))
   - ✅ Extracts pre/post ubt witnesses from the first two extrinsics
   - ✅ Computes BAL hash from witness (identical process to builder)
   - ✅ Extracts claimed BAL hash from received block payload
   - ✅ Verifies hash matches builder's claimed `block_access_list_hash`
   - ✅ Logs verification success (✅ PASSED) or fraud detection (🚨 FAILED)
   - ✅ Returns error on hash mismatch to trigger fraud proof mechanism
   - ✅ Compares builder vs guarantor post-state write key sets during refinement (logs value mismatches)

3. **Testing Status**
   - ✅ Unit tests for BAL construction and determinism ([storage/block_access_list_test.go](../../storage/block_access_list_test.go))
     - 13 tests covering RLP encoding, hash determinism, witness parsing, code size extraction
   - ⚠️ Integration tests for builder/guarantor hash matching → **Phase 5**
   - ⚠️ Fuzz tests for determinism verification → **Phase 5**

### 📋 Implementation Files

**Completed**:
- `types/storage.go` - UBTRead with TxIndex field; JAMStorage interface updated
- `storage/witness.go` - 161-byte witness serialization; 29-byte blob header parsing; extractKeyMetadata uses blob tx_index
- `storage/storage.go` - All Fetch* methods accept txIndex parameter
- `statedb/hostfunctions.go` - hostFetchUBT reads tx_index from register 12
- `services/evm/src/ubt_host.rs` - host_fetch_ubt wrappers accept tx_index
- `services/evm/src/ubt.rs` - witness section parsing + multiproof verification (tx_index included in entries)
- `services/evm/src/state.rs` - MajikBackend tracks and passes current_tx_index to ubt fetches
- `services/utils/src/effects.rs` - WriteEffectEntry includes tx_index field
- `services/evm/src/backend.rs` - All WriteEffectEntry creations include tx_index
- `services/evm/src/refiner.rs` - Blob serialization writes 4-byte tx_index; guarantor verifies BAL hash natively in Rust from UBT entries
- `statedb/refine.go` - Builder computes BAL hash and embeds it in payloads
- `storage/block_access_list.go` - Complete BAL construction (504 lines)
- `storage/block_access_list_test.go` - Complete test suite (691 lines, 13 tests passing)

**Pending**:
- Accumulate-phase BAL hash propagation for downstream consumers
- End-to-end builder/guarantor integration tests and fuzzing for determinism

---

## Executive Summary

Block Access Lists (EIP-7928) provide deterministic tracking of all state accesses during block execution. For JAM EVM, BAL construction requires **transaction boundary metadata** embedded in the ubt witness to enable stateless verification.

**Goal**: Let the builder deterministically derive a `BlockAccessList` and `block_access_list_hash` from ubt witness data augmented with transaction indices, so guarantors can recompute and verify it without re-execution.

**Two-Phase Approach**:

1. **Builder Mode (Runtime Tracking)**:
   - During execution: Track writes with `blockAccessIndex` (transaction index)
   - After execution: Build ubt witness WITH transaction indices embedded in metadata
   - Construct `BlockAccessList` from augmented witness → compute `block_access_list_hash`

2. **Guarantor Mode (Witness-Only Reconstruction)**:
   - Parse augmented witness (includes transaction indices in metadata)
   - Rebuild `BlockAccessList` using same deterministic algorithm
   - Compare hash → detect fraud (no re-execution needed)

3. **Accumulate Phase**: Use BAL for audit trails and state application ordering

**Key Benefits**:
- Complete audit trail of state accesses (reads + writes) with execution order preserved
- Fraud detection: Guarantors verify access patterns match builder's claim via hash comparison
- Deterministic serialization enables hash-based verification
- Stateless verification: Guarantor derives BAL from witness metadata alone (no execution state required)
- Complements ubt witnesses (BAL = logical view, UBT = cryptographic proof)

**Critical Requirement**: UBT witness metadata must include `txIndex` (transaction index) for each read/write entry to enable stateless BAL reconstruction.

---

## Design Targets

Building on EIP-7928, our BAL implementation for JAM must achieve:

1. **Deterministic**: No map iteration; stable ordering (address bytes → key bytes → blockAccessIndex)
2. **Verifiable**: Guarantor rebuilds BAL from dual-proof witness + contract witness blob; compare `block_access_list_hash`
3. **Complete**: Includes reads (for cache validity) and writes (for state transition), across balance, nonce, code, and storage
4. **Stateless-friendly**: Guarantor derives BAL from witness metadata without re-execution
5. **Compact**: Fixed-size arrays for deterministic RLP; minimal overhead
6. **Compatible**: Follows EIP-7928 structure while adapting to JAM's work package model

---

## Architecture Overview

### Relationship to UBT Witnesses

**UBT Witnesses** (cryptographic layer):
- Pre-state proof: Keys exist and have claimed values
- Post-state proof: Keys transitioned to new values
- Purpose: Stateless verification via cryptographic proofs

**Block Access Lists** (logical layer):
- Complete access log: Who touched what, when, how
- Purpose: Audit trail + fraud detection + ordering guarantees

**Integration**:
```
Pre-state witness section (reads) + Post-state witness section (writes)
    ↓
Parse metadata: (address, storage_key, pre_value, post_value)
    ↓
Track writeEvents with monotonic blockAccessIndex
    ↓
BuildBlockAccessList() - deterministic grouping/sorting
    ↓
BlockAccessList struct (sorted accounts, sorted keys, ordered writes)
    ↓
RLP encode + Blake2b
    ↓
block_access_list_hash (stored in work package header)
```

**Data Sources** (Builder constructs, Guarantor parses):
- **Augmented Pre-state witness**: `(key, metadata, pre_value, txIndex)` for all reads
  - **NEW**: `txIndex` field added (4 bytes) → 161 bytes total per entry
- **Augmented Post-state witness**: `(key, metadata, pre_value, post_value, txIndex)` for all writes
  - **NEW**: `txIndex` field embedded in metadata
- **Contract witness blob**: Write intents in execution order (provides pre/post values)
- **Runtime tracking** (Builder only): `WriteEventTracker` assigns `blockAccessIndex` during `apply*Writes()`
  - Guarantor does NOT have access to this - must use txIndex from witness

---

## Data Structures

### BlockAccessList

**✅ IMPLEMENTED** in [storage/block_access_list.go:18-78](../../storage/block_access_list.go#L18-L78)

Core structs:
- `BlockAccessList` - Top-level container with sorted accounts
- `AccountChanges` - All changes for a single address
- `StorageRead` - Read-only storage slot access
- `SlotChanges` - All writes to a single storage slot (supports multiple transactions)
- `SlotWrite` - Single write event with pre/post values and blockAccessIndex
- `BalanceChange` - Balance modification with pre/post balance
- `NonceChange` - Nonce update with pre/post nonce
- `CodeChange` - Code deployment (stores CodeHash + CodeSize, not full bytecode)

**Key Design Decisions**:

1. **SlotChanges with multiple writes**: A single slot can be written multiple times in a block (different transactions). Each write is tracked separately with its `blockAccessIndex`.

2. **Pre/Post values for all changes**: Unlike EIP-7928 which only stores post-values, we include both pre and post for complete state transition tracking.

3. **Deterministic ordering**:
   - Accounts: Sorted by address bytes (lexicographic)
   - Storage keys: Sorted by key bytes within each account
   - Writes: Ordered by `blockAccessIndex` (monotonically increasing)

4. **Fixed-size types**: Use `[32]byte`, `[8]byte` instead of `common.Hash`, `uint64` where possible for deterministic RLP encoding.

### Block Access Index Mapping

**Formula**: `blockAccessIndex = extrinsicIndex` for transaction extrinsics (where extrinsic[0] is ubt witness)

**Mapping**:
- `blockAccessIndex = 0`: Pre-execution system calls (genesis, validator set updates)
- `blockAccessIndex = i` (i ∈ [1..n]): Transaction at `extrinsic[i]`
- `blockAccessIndex = n+1`: Post-execution system calls (block finalization, rewards)

**Example Work Package**:
```
Extrinsic[0]: UBT witness          → Not represented in BAL (metadata only)
Extrinsic[1]: Transaction A            → blockAccessIndex = 1
Extrinsic[2]: Transaction B            → blockAccessIndex = 2
Extrinsic[3]: Transaction C            → blockAccessIndex = 3
Post-finalization: Validator rewards   → blockAccessIndex = 4 (if present)
```

**CRITICAL**:
- `blockAccessIndex` is stored in witness metadata (txIndex field) and BAL change entries
- Guarantor uses txIndex from witness to reconstruct blockAccessIndex
- Builder must ensure txIndex in witness matches blockAccessIndex used in BAL construction

---

## Construction Algorithm (Deterministic)

This section describes the **deterministic** algorithm for constructing BAL from witness data, ensuring both builder and guarantor produce identical results.

### Step 1: Collect Ordering Anchors (writeEvents)

**Problem**: Contract witness blob contains writes but no explicit ordering/indexing.

**Solution**: As writes are applied in `apply*Writes()`, track each write event with a monotonic `blockAccessIndex`:

```go
// WriteEvent tracks a single state modification
type WriteEvent struct {
    BlockAccessIndex uint32   // Monotonic counter per block
    UBTKey        [32]byte // Computed ubt key
    Address          common.Address
    KeyType          uint8    // BasicData, Storage, Code, etc.
    StorageKey       [32]byte // For storage writes
    PreValue         [32]byte // Value before write
    PostValue        [32]byte // Value after write
}

// WriteEventTracker accumulates all writes during execution
type WriteEventTracker struct {
    events           []WriteEvent
    nextAccessIndex  uint32
}

// NewWriteEventTracker creates a tracker with initial blockAccessIndex
func NewWriteEventTracker(initialIndex uint32) *WriteEventTracker {
    return &WriteEventTracker{
        events:          make([]WriteEvent, 0),
        nextAccessIndex: initialIndex,
    }
}

func (t *WriteEventTracker) RecordWrite(
    ubtKey [32]byte,
    address common.Address,
    keyType uint8,
    storageKey [32]byte,
    preValue, postValue [32]byte,
) {
    t.events = append(t.events, WriteEvent{
        BlockAccessIndex: t.nextAccessIndex,
        UBTKey:        ubtKey,
        Address:          address,
        KeyType:          keyType,
        StorageKey:       storageKey,
        PreValue:         preValue,
        PostValue:        postValue,
    })
}

func (t *WriteEventTracker) NextTransaction() {
    t.nextAccessIndex++
}
```

**blockAccessIndex Mapping**:
- `0`: Pre-execution system calls (genesis, validator set updates)
- `1..n`: Transaction index (extrinsic[i] where i=1..n, since extrinsic[0] is ubt witness)
- `n+1`: Optional post-finalization (rewards, slashing)

**CRITICAL**: Initialize tracker with correct starting index:
```go
// For regular work package (first tx is index 1):
tracker := NewWriteEventTracker(1)

// For genesis (system calls at index 0):
tracker := NewWriteEventTracker(0)
```

**Integration Points**:
- `statedb/refine.go`: Create `WriteEventTracker` with initial index 1, call `NextTransaction()` AFTER each transaction completes
- `storage/witness.go`: Modify `apply*Writes()` to call `tracker.RecordWrite()` for each write
- Pass tracker to `BuildBlockAccessList()` for ordering information

### Step 2: Parse Augmented Pre-State Witness for Reads

**✅ IMPLEMENTED** in [storage/block_access_list.go:90-183](../../storage/block_access_list.go#L90-L183)

**Functions**:
- `parseAugmentedPreStateReads()` - Parses 161-byte entries from pre-state witness
- `parseAugmentedPostStateWrites()` - Parses 161-byte entries from post-state witness

**Structs**:
- `ReadEntry` - Represents a read from pre-state section (includes TxIndex)
- `WriteEntry` - Represents a write from post-state section (includes TxIndex)

**Input**: Augmented witness section with 161-byte entries (includes 4-byte TxIndex)

**Output**: List of all reads/writes with `txIndex` field populated for stateless BAL reconstruction

### Step 3: Build SlotChanges from Witness Writes

**✅ IMPLEMENTED** in [storage/block_access_list.go:252-276](../../storage/block_access_list.go#L252-L276)

**Function**: `buildSlotChanges(writes []WriteEntry)`

**Input**: Post-state witness entries (with txIndex field)

**Output**: Map of `address → storage_key → []SlotWrite`

**Key Point**:
- Uses `write.TxIndex` from witness metadata as `blockAccessIndex`
- Supports multiple writes per slot (different transactions)
- Both produce identical `blockAccessIndex` values → identical BAL hash

### Step 4: Build Balance/Nonce/Code Changes from Witness

**✅ IMPLEMENTED** in [storage/block_access_list.go:278-345](../../storage/block_access_list.go#L278-L345)

**Functions**:
- `buildBalanceChanges()` - Extracts balance changes from BasicData writes (bytes 16-31)
- `buildNonceChanges()` - Extracts nonce changes from BasicData writes (bytes 8-15)
- `buildCodeChanges()` - Extracts code deployments from CodeHash writes with CodeSize from BasicData (bytes 5-7)

**Input**: Post-state witness entries (with txIndex field)

**Output**: Maps of `address → []ChangeType` with blockAccessIndex from witness

**Note**: All functions use `write.TxIndex` from witness, not runtime `WriteEventTracker`

### Step 5: Classify Reads vs Writes

**✅ IMPLEMENTED** in [storage/block_access_list.go:347-375](../../storage/block_access_list.go#L347-L375)

**Function**: `classifyStorageAccesses(reads []ReadEntry, writes []WriteEntry)`

**Logic**:
- **Read-only**: Key in pre-state reads but NOT in post-state writes
- **Modified**: Key in post-state writes (tracked in SlotChanges)

**Output**: Map of `address → []StorageRead` for read-only accesses

**Note**: Uses witness entries directly, no runtime tracker needed

### Step 6: Assemble AccountChanges (Deterministic Ordering)

**✅ IMPLEMENTED** in [storage/block_access_list.go:366-457](../../storage/block_access_list.go#L366-L457)

**Function**: `assembleAccountChanges(...)`

**Critical**: Uses slices, not maps, for final assembly. Deterministic sorting:
- Addresses sorted lexicographically (bytes.Compare)
- Storage keys sorted lexicographically
- StorageReads sorted by key
- SlotChanges sorted by key, writes within each slot sorted by blockAccessIndex
- Balance/Nonce/Code changes sorted by blockAccessIndex

### Step 7: BuildBlockAccessList (Main Function)

**✅ IMPLEMENTED** in [storage/block_access_list.go:429-469](../../storage/block_access_list.go#L429-L469)

**Function**: `BuildBlockAccessList(preStateWitness []byte, postStateWitness []byte)`

**Process**:
1. Parse augmented witness sections
2. Build change maps from witness entries
3. Classify storage accesses
4. Assemble with deterministic ordering
5. Return BlockAccessList

### Step 8: Hash Computation

**✅ IMPLEMENTED** in [storage/block_access_list.go:77-84](../../storage/block_access_list.go#L77-L84)

**Function**: `(bal *BlockAccessList) Hash() common.Hash`

Uses RLP encoding + Blake2b hash for deterministic block_access_list_hash

### Step 9: Witness Section Splitting

**✅ IMPLEMENTED** in [storage/block_access_list.go:472-504](../../storage/block_access_list.go#L472-L504)

**Function**: `SplitWitnessSections(witness []byte)`

Parses dual-proof witness into pre-state and post-state sections for BAL construction

### Determinism Guardrails

1. **No map iteration**: Always convert maps to sorted slices before encoding
2. **Stable sorting**: Sort by bytes (lexicographic) for all keys
3. **Monotonic indices**: `blockAccessIndex` from witness txIndex field
4. **Identical parsing**: Builder and guarantor use same parsing code
5. **Fixed-length types**: Use `common.Hash`, `[8]byte` for deterministic RLP
6. **Sorted outputs**: All slices sorted before RLP encoding

### Guarantor Verification

Guarantor follows **identical** algorithm:
1. Extract pre/post witness extrinsics (builder's witnesses)
2. Call `BuildBlockAccessList(preWitness, postWitness)` - same function as builder
3. Compute `bal.Hash()` using identical RLP encoding
4. Compare hash with builder's claimed `block_access_list_hash`
5. **No re-execution required** - everything derived from witness metadata (txIndex field)

---

## Implementation Plan

### Phase 1: Core Data Structures

**File**: `statedb/access_list.go` (new)

```go
package statedb

import (
    "bytes"
    "sort"
    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/rlp"
    "github.com/ethereum/go-ethereum/crypto"
)

// BlockAccessList + AccountChanges + change types (as defined above)

// RLP encoding implementations for each type
func (bal *BlockAccessList) EncodeRLP(w io.Writer) error {
    return rlp.Encode(w, bal.Accounts)
}

func (bal *BlockAccessList) DecodeRLP(s *rlp.Stream) error {
    return s.Decode(&bal.Accounts)
}

// Implementation reference: storage/block_access_list.go:77-84
// Hash computes Blake2b(rlp.encode(bal))
// Note: JAM uses Blake2b, not Keccak256 (diverges from EIP-7928)
```

**Key Design Choices**:
- Use `common.Address`, `common.Hash`, `*big.Int` for Ethereum compatibility
- Implement `rlp.Encoder`/`rlp.Decoder` for deterministic serialization
- Provide `Hash()` method for header inclusion
- Handle empty BAL case (no accesses → empty list hash)

---

### Phase 2: Builder - Construct BAL from UBT Logs

**File**: `statedb/access_list_builder.go` (new)

```go
// BuildBlockAccessList constructs BAL from augmented ubt witness
//
// Process:
// 1. Parse augmented pre-state witness for reads (161-byte entries with txIndex)
// 2. Parse augmented post-state witness for writes (161-byte entries with txIndex)
// 3. Build changes using txIndex as blockAccessIndex
// 4. Classify into read-only vs modified
// 5. Sort and deduplicate
// 6. Return BlockAccessList with proper ordering
//
// CRITICAL: Uses witness metadata only - guarantor can call this without execution state
func BuildBlockAccessList(
    preStateWitness []byte,
    postStateWitness []byte,
) (*BlockAccessList, error) {
    // Parse augmented witness entries
    reads, err := parseAugmentedPreStateReads(preStateWitness)
    if err != nil {
        return nil, fmt.Errorf("failed to parse pre-state witness: %w", err)
    }

    writes, err := parseAugmentedPostStateWrites(postStateWitness)
    if err != nil {
        return nil, fmt.Errorf("failed to parse post-state witness: %w", err)
    }

    // Build changes from witness entries
    slotChangesMap := buildSlotChanges(writes)
    balanceChangesMap := buildBalanceChanges(writes)
    nonceChangesMap := buildNonceChanges(writes)
    codeChangesMap := buildCodeChanges(writes)

    // Classify storage reads
    storageReadsMap := classifyStorageAccesses(reads, writes)

    // Assemble AccountChanges with deterministic ordering
    accounts := assembleAccountChanges(
        storageReadsMap,
        slotChangesMap,
        balanceChangesMap,
        nonceChangesMap,
        codeChangesMap,
    )

    return &BlockAccessList{Accounts: accounts}, nil
}

// assembleAccountChanges merges all change maps into sorted AccountChanges list
func assembleAccountChanges(
    storageReadsMap map[common.Address][]StorageRead,
    slotChangesMap map[common.Address]map[[32]byte][]SlotWrite,
    balanceChangesMap map[common.Address][]BalanceChange,
    nonceChangesMap map[common.Address][]NonceChange,
    codeChangesMap map[common.Address][]CodeChange,
) []AccountChanges {
    // Collect all touched addresses
    addressSet := make(map[common.Address]struct{})
    for addr := range storageReadsMap {
        addressSet[addr] = struct{}{}
    }
    for addr := range slotChangesMap {
        addressSet[addr] = struct{}{}
    }
    for addr := range balanceChangesMap {
        addressSet[addr] = struct{}{}
    }
    for addr := range nonceChangesMap {
        addressSet[addr] = struct{}{}
    }
    for addr := range codeChangesMap {
        addressSet[addr] = struct{}{}
    }

    // Sort addresses lexicographically
    addresses := make([]common.Address, 0, len(addressSet))
    for addr := range addressSet {
        addresses = append(addresses, addr)
    }
    sort.Slice(addresses, func(i, j int) bool {
        return bytes.Compare(addresses[i][:], addresses[j][:]) < 0
    })

    // Build AccountChanges per address
    accounts := make([]AccountChanges, 0, len(addresses))
    for _, addr := range addresses {
        // Sort storage reads by key
        storageReads := storageReadsMap[addr]
        sort.Slice(storageReads, func(i, j int) bool {
            return bytes.Compare(storageReads[i].Key[:], storageReads[j].Key[:]) < 0
        })

        // Build sorted SlotChanges
        slotMap := slotChangesMap[addr]
        storageKeys := make([][32]byte, 0, len(slotMap))
        for key := range slotMap {
            storageKeys = append(storageKeys, key)
        }
        sort.Slice(storageKeys, func(i, j int) bool {
            return bytes.Compare(storageKeys[i][:], storageKeys[j][:]) < 0
        })

        slotChanges := make([]SlotChanges, 0, len(storageKeys))
        for _, key := range storageKeys {
            writes := slotMap[key]
            // Sort writes by blockAccessIndex
            sort.Slice(writes, func(i, j int) bool {
                return writes[i].BlockAccessIndex < writes[j].BlockAccessIndex
            })
            slotChanges = append(slotChanges, SlotChanges{
                Key:    key,
                Writes: writes,
            })
        }

        // Sort balance/nonce/code changes by blockAccessIndex
        balanceChanges := balanceChangesMap[addr]
        sort.Slice(balanceChanges, func(i, j int) bool {
            return balanceChanges[i].BlockAccessIndex < balanceChanges[j].BlockAccessIndex
        })

        nonceChanges := nonceChangesMap[addr]
        sort.Slice(nonceChanges, func(i, j int) bool {
            return nonceChanges[i].BlockAccessIndex < nonceChanges[j].BlockAccessIndex
        })

        codeChanges := codeChangesMap[addr]
        sort.Slice(codeChanges, func(i, j int) bool {
            return codeChanges[i].BlockAccessIndex < codeChanges[j].BlockAccessIndex
        })

        accounts = append(accounts, AccountChanges{
            Address:        addr,
            StorageReads:   storageReads,
            StorageChanges: slotChanges,
            BalanceChanges: balanceChanges,
            NonceChanges:   nonceChanges,
            CodeChanges:    codeChanges,
        })
    }

    return accounts
}
```

**Note**: This witness-based approach enables **stateless BAL reconstruction**. Both builder and guarantor call the same `BuildBlockAccessList()` function with the augmented witness data, ensuring deterministic BAL hashes without re-execution

---

### Phase 3: Integrate BAL into Builder Flow

**File**: `statedb/refine.go` (modify existing `BuildBundle`)

**Current Flow** (lines 462-499):
```go
func BuildBundle(...) {
    // ... execute transactions ...

    // Build augmented UBT witnesses (161-byte entries with txIndex)
    contractBlob := extractContractWitness(vm)
    ubtPreWitness, ubtPostWitness, err := evmStorage.BuildUBTWitness(contractBlob)
    if err != nil {
        return nil, fmt.Errorf("failed to build ubt witnesses: %w", err)
    }

    // Build block access list from augmented witnesses
    bal, err := BuildBlockAccessList(ubtPreWitness, ubtPostWitness)
    if err != nil {
        return nil, fmt.Errorf("failed to build block access list: %w", err)
    }

    // Compute BAL hash
    balHash := bal.Hash()
    log.Info(log.SDB, "✅ Built BlockAccessList",
        "accounts", len(bal.Accounts),
        "balHash", balHash.Hex())

    // Store BAL and hash in WorkItem metadata
    workItem.BlockAccessList = bal
    workItem.BlockAccessListHash = balHash

    // Prepend witnesses as first two extrinsics (pre, post)
    extrinsics = append([][]byte{ubtPreWitness, ubtPostWitness}, extrinsics...)
}
```

**WorkItem Extension** (`types/bundle.go`):
```go
type WorkItem struct {
    // ... existing fields ...

    // Block Access List fields
    BlockAccessList     *statedb.BlockAccessList
    BlockAccessListHash common.Hash
}
```

---

### Phase 4: Guarantor - Verify BAL

**File**: `statedb/access_list_verifier.go` (new)

**CRITICAL INSIGHT**: Guarantor uses the same pre/post witness extrinsics as the builder (first two extrinsics), so both call `BuildBlockAccessList()` with identical inputs. This ensures deterministic BAL hashes without re-execution.

```go
// VerifyBlockAccessList checks guarantor's execution matches builder's BAL
//
// Process (STATELESS):
// 1. Read pre/post witness extrinsics (builder's witnesses)
// 2. Call BuildBlockAccessList() with the same witness data
// 3. Compare hash with builder's claimed hash
// 4. If mismatch, perform detailed comparison to identify fraud
//
// NOTE: Guarantor uses builder's witness, NOT its own execution state
func VerifyBlockAccessList(
    builderBAL *BlockAccessList,
    builderHash common.Hash,
    preWitness []byte,  // First extrinsic (augmented pre witness)
    postWitness []byte, // Second extrinsic (augmented post witness)
) error {
    // Step 1: Reconstruct BAL from witness (deterministic)
    guarantorBAL, err := BuildBlockAccessList(preWitness, postWitness)
    if err != nil {
        return fmt.Errorf("failed to build guarantor BAL: %w", err)
    }

    // Step 2: Compute hash
    guarantorHash := guarantorBAL.Hash()

    // Step 3: Quick check - hashes match?
    if guarantorHash == builderHash {
        log.Info(log.SDB, "✅ Block access list verified (hash match)",
            "balHash", builderHash.Hex())
        return nil
    }

    // Step 5: Detailed fraud detection (should never happen if deterministic)
    log.Warn(log.SDB, "❌ Block access list MISMATCH - DETERMINISM BUG",
        "builderHash", builderHash.Hex(),
        "guarantorHash", guarantorHash.Hex())

    // Compare account-by-account to diagnose bug
    builderMap := make(map[common.Address]*AccountChanges)
    for i := range builderBAL.Accounts {
        builderMap[builderBAL.Accounts[i].Address] = &builderBAL.Accounts[i]
    }

    guarantorMap := make(map[common.Address]*AccountChanges)
    for i := range guarantorBAL.Accounts {
        guarantorMap[guarantorBAL.Accounts[i].Address] = &guarantorBAL.Accounts[i]
    }

    // Find missing/extra accounts
    for addr, builderAcc := range builderMap {
        guarantorAcc, exists := guarantorMap[addr]
        if !exists {
            log.Warn(log.SDB, "❌ Builder has address not in guarantor BAL",
                "address", addr.Hex())
            continue
        }

        // Compare access details
        if err := compareAccountAccesses(builderAcc, guarantorAcc); err != nil {
            log.Warn(log.SDB, "❌ Account access mismatch",
                "address", addr.Hex(),
                "error", err.Error())
        }
    }

    for addr := range guarantorMap {
        if _, exists := builderMap[addr]; !exists {
            log.Warn(log.SDB, "❌ Guarantor has address not in builder BAL",
                "address", addr.Hex())
        }
    }

    return fmt.Errorf("block access list verification failed: hash mismatch (indicates determinism bug)")
}

// compareAccountAccesses performs detailed comparison of accesses
func compareAccountAccesses(builder, guarantor *AccountChanges) error {
    // Compare storage reads
    if len(builder.StorageReads) != len(guarantor.StorageReads) {
        return fmt.Errorf("storage read count mismatch: builder=%d, guarantor=%d",
            len(builder.StorageReads), len(guarantor.StorageReads))
    }

    // Compare storage writes
    if len(builder.StorageChanges) != len(guarantor.StorageChanges) {
        return fmt.Errorf("storage change count mismatch: builder=%d, guarantor=%d",
            len(builder.StorageChanges), len(guarantor.StorageChanges))
    }

    // Compare balance/nonce/code changes
    if len(builder.BalanceChanges) != len(guarantor.BalanceChanges) {
        return fmt.Errorf("balance change count mismatch")
    }

    if len(builder.NonceChanges) != len(guarantor.NonceChanges) {
        return fmt.Errorf("nonce change count mismatch")
    }

    if len(builder.CodeChanges) != len(guarantor.CodeChanges) {
        return fmt.Errorf("code change count mismatch")
    }

    // Deep comparison of values
    for i, builderChange := range builder.StorageChanges {
        guarantorChange := guarantor.StorageChanges[i]
        if builderChange.Key != guarantorChange.Key {
            return fmt.Errorf("storage key mismatch at index %d", i)
        }
        if builderChange.NewValue != guarantorChange.NewValue {
            return fmt.Errorf("storage value mismatch for key %s", builderChange.Key.Hex())
        }
    }

    // Similar checks for balance, nonce, code...

    return nil
}
```

**Integration into Guarantor Flow** (modify `refine.go` guarantor execution):
```go
func ExecuteAsGuarantor(...) {
    // ... existing guarantor execution ...

    // After execution, verify BAL
    guarantorWitness := extractContractWitness(vm)
    guarantorReadLog := storage.GetUBTReadLog()

    err := VerifyBlockAccessList(
        workItem.BlockAccessList,
        workItem.BlockAccessListHash,
        guarantorReadLog,
        guarantorWitness,
        txCount,
    )

    if err != nil {
        return nil, fmt.Errorf("BAL verification failed: %w", err)
    }
}
```

---

### Phase 5: Transaction Index Tracking

**Problem**: Current implementation doesn't track which transaction caused each access.

**Solution**: Flow `tx_index` from Rust (refiner.rs) → through host_fetch_ubt → to Go AppendUBTRead.

**Architecture**:
- `refiner.rs:426-491` iterates over transactions in Rust within a single refine call
- Each transaction calls `overlay.begin_transaction(tx_index)` which sets `MajikOverlay.current_tx_index`
- State reads (`balance()`, `nonce()`, `storage()`) flow through `MajikBackend` which has access to `current_tx_index`
- **NOT** per-transaction refines from Go side

---

**File 1**: `services/evm/src/ubt_host.rs` (modify)

Update `host_fetch_ubt` import to accept tx_index:

```rust
#[polkavm_import(index = 255)]
pub fn host_fetch_ubt(
    fetch_type: u64,
    address_ptr: u64,
    key_ptr: u64,
    output_ptr: u64,
    output_max_len: u64,
    tx_index: u64,  // NEW: Transaction index from overlay
) -> u64;
```

Update all wrapper functions to accept and pass tx_index:

```rust
pub fn fetch_balance_ubt(address: H160, tx_index: u32) -> U256 {
    let mut output = [0u8; 32];
    unsafe {
        let written = host_fetch_ubt(
            FETCH_BALANCE as u64,
            address.as_ptr() as u64,
            0,
            output.as_mut_ptr() as u64,
            32,
            tx_index as u64,  // Pass through
        );
        // ...
    }
    U256::from_big_endian(&output)
}

// Similarly update:
// - fetch_nonce_ubt(address, tx_index)
// - fetch_code_ubt(address, tx_index)
// - fetch_code_hash_ubt(address, tx_index)
// - fetch_storage_ubt(address, key, tx_index)
```

---

**File 2**: `services/evm/src/state.rs` (modify)

Update `MajikBackend` methods to pass `current_tx_index` to ubt fetches:

```rust
impl RuntimeBaseBackend for MajikBackend {
    fn balance(&self, address: H160) -> U256 {
        // Check cache first
        if let Some(balance) = self.balances.borrow().get(&address) {
            return *balance;
        }

        match self.execution_mode {
            ExecutionMode::Builder => {
                // Pass current_tx_index from backend
                let tx_index = self.current_tx_index.borrow().unwrap_or(0) as u32;
                let balance = crate::ubt_host::fetch_balance_ubt(address, tx_index);
                self.balances.borrow_mut().insert(address, balance);
                balance
            }
            ExecutionMode::Guarantor => {
                panic!("GUARANTOR BALANCE MISS");
            }
        }
    }

    // Similarly update nonce(), code(), code_hash(), storage()
}
```

---

**File 3**: `statedb/hostfunctions.go` (modify)

Update `hostFetchUBT()` to read tx_index from register 12:

```go
func (vm *VM) hostFetchUBT() {
    fetchType := uint8(vm.ReadRegister(7))
    addressPtr := uint32(vm.ReadRegister(8))
    keyPtr := uint32(vm.ReadRegister(9))
    outputPtr := uint32(vm.ReadRegister(10))
    outputMaxLen := uint32(vm.ReadRegister(11))
    txIndex := uint32(vm.ReadRegister(12))  // NEW: Read tx_index from Rust

    // ... existing address/stateDB extraction ...

    switch fetchType {
    case evmtypes.FETCH_BALANCE:
        vm.fetchBalance(stateDB, address, outputPtr, outputMaxLen, txIndex)
    case evmtypes.FETCH_NONCE:
        vm.fetchNonce(stateDB, address, outputPtr, outputMaxLen, txIndex)
    case evmtypes.FETCH_CODE:
        vm.fetchCode(stateDB, address, outputPtr, outputMaxLen, txIndex)
    case evmtypes.FETCH_CODE_HASH:
        vm.fetchCodeHash(stateDB, address, outputPtr, outputMaxLen, txIndex)
    case evmtypes.FETCH_STORAGE:
        // ... read storageKey ...
        vm.fetchStorage(stateDB, address, storageKey, outputPtr, outputMaxLen, txIndex)
    }
}
```

Update fetch helper functions to accept and pass txIndex:

```go
func (vm *VM) fetchBalance(stateDB *StateDB, address common.Address, outputPtr uint32, outputMaxLen uint32, txIndex uint32) {
    if outputMaxLen == 0 {
        vm.WriteRegister(7, 32)
        return
    }

    balance, err := stateDB.sdb.FetchBalance(address, txIndex)  // Pass txIndex
    // ... rest of implementation
}

// Similarly update fetchNonce, fetchCode, fetchCodeHash, fetchStorage
```

---

**File 4**: `storage/storage.go` (modify)

Update Fetch* methods to accept and use txIndex:

```go
func (store *StateDBStorage) FetchBalance(address common.Address, txIndex uint32) ([32]byte, error) {
    var balance [32]byte

    if store.CurrentUBT == nil {
        return balance, nil
    }

    ubtKey := evmtypes.BasicDataKey(address[:])

    // Track read with transaction index
    store.AppendUBTRead(types.UBTRead{
        UBTKey: common.BytesToHash(ubtKey),
        Address:   address,
        KeyType:   0, // BasicData
        Extra:     0,
        TxIndex:   txIndex,  // NEW: Populate from parameter
    })

    // ... rest of implementation
}

// Similarly update FetchNonce, FetchCode, FetchCodeHash, FetchStorage
```

---

## Witness Format Augmentation (Transaction Boundaries)

### Problem Statement

The original ubt witness format did NOT include transaction index information. Now augmented to 161 bytes:
```
[32B ubt_key][1B key_type][20B address][8B extra]
[32B storage_key][32B pre_value][32B post_value][4B tx_index]
= 161 bytes
```

**Without transaction indices**, guarantors cannot determine which transaction caused each read/write, making BAL reconstruction impossible from witness data alone.

### Solution: Add TxIndex Field

**Augmented Format** (161 bytes per entry):
```
[32B ubt_key][1B key_type][20B address][8B extra]
[32B storage_key][32B pre_value][32B post_value][4B txIndex]
= 161 bytes
```

**Field Details**:
- `txIndex` (uint32, 4 bytes): Transaction index within work package
  - `0`: Pre-execution system calls (genesis, validator updates)
  - `1..n`: Transaction index (maps to extrinsic[i] where i = 1..n)
  - `n+1`: Post-execution system calls (rewards, slashing)

### Implementation Changes

**✅ IMPLEMENTED**

#### Phase 1.1: Add TxIndex field to UBTRead

**File**: [types/storage.go:142-151](../../types/storage.go#L142-L151)

Added `TxIndex uint32` field to `UBTRead` struct

#### Phase 1.2: Update witness serialization to 161 bytes

**File**: [storage/witness.go](../../storage/witness.go)

Key changes:
- Updated `KeyMetadata` struct with `TxIndex` field ([line 380](../../storage/witness.go#L380))
- Changed size calculation from 157 to 161 bytes per entry ([lines 199-203](../../storage/witness.go#L199-L203))
- Added TxIndex serialization (4 bytes, big-endian) in pre-state section ([line 225](../../storage/witness.go#L225))
- Added TxIndex serialization in post-state section ([line 266](../../storage/witness.go#L266))
- Updated `extractKeyMetadataFromReads` to copy TxIndex from UBTRead ([lines 398-404](../../storage/witness.go#L398-L404))
- Updated `extractKeyMetadata` to propagate TxIndex from reads to writes ([lines 421-434](../../storage/witness.go#L421-L434))

**Note**: Transaction index tracking during execution (builder-side) is not yet implemented. Currently TxIndex will be 0 for all entries. Full integration requires:
- `statedb/statedb_evm.go`: Add `currentTxIndex` tracking
- `statedb/refine.go`: Call `SetCurrentTransactionIndex()` before each transaction
- `storage/storage.go`: Update `AppendUBTRead` calls to include current txIndex

### Guarantor Parsing

**✅ IMPLEMENTED** in [storage/block_access_list.go:112-250](../../storage/block_access_list.go#L112-L250)

Functions:
- `parseAugmentedPreStateReads()` - Parses 161-byte entries with txIndex
- `parseAugmentedPostStateWrites()` - Parses 161-byte entries with txIndex

Both functions extract txIndex from offset 157-161 of each witness entry

### Backward Compatibility

**Breaking Change**: This increases witness size by `4 bytes × num_entries`.

**Migration**:
- Witness format is now 161 bytes/entry (includes tx_index)
- New format is `v2` (161 bytes/entry)
- Add version byte to witness header to distinguish

**Impact**:
- Typical transaction: 10-20 entries → +40-80 bytes witness size
- Acceptable overhead for stateless BAL reconstruction

---

## Serialization Format

### RLP Encoding Schema

**CRITICAL**: Field order must match Go struct field order for deterministic RLP encoding.

```
BlockAccessList = [
    AccountChanges_1,
    AccountChanges_2,
    ...
]

AccountChanges = [
    Address,                           // 20 bytes
    StorageReads[],                    // List of StorageRead (sorted by key)
    StorageChanges[],                  // List of SlotChanges (sorted by key)
    BalanceChanges[],                  // List of BalanceChange (ordered by blockAccessIndex)
    NonceChanges[],                    // List of NonceChange (ordered by blockAccessIndex)
    CodeChanges[]                      // List of CodeChange (ordered by blockAccessIndex)
]

StorageRead = [
    Key                   // 32 bytes (fixed-size array)
]

SlotChanges = [
    Key,                  // 32 bytes (fixed-size array)
    Writes[]              // List of SlotWrite (ordered by blockAccessIndex)
]

SlotWrite = [
    BlockAccessIndex,     // uint32
    PreValue,             // 32 bytes (fixed-size array)
    PostValue             // 32 bytes (fixed-size array)
]

BalanceChange = [
    BlockAccessIndex,     // uint32
    PreBalance,           // 32 bytes (fixed-size array, big-endian uint128 in last 16 bytes)
    PostBalance           // 32 bytes (fixed-size array, big-endian uint128 in last 16 bytes)
]

NonceChange = [
    BlockAccessIndex,     // uint32
    PreNonce,             // 8 bytes (fixed-size array, little-endian uint64)
    PostNonce             // 8 bytes (fixed-size array, little-endian uint64)
]

CodeChange = [
    BlockAccessIndex,     // uint32
    CodeHash,             // 32 bytes (fixed-size array, hash of code)
    CodeSize              // uint32
]
```

**Note on Pre/Post Values**:
- All change types include BOTH pre and post values
- This allows guarantors to verify state transitions, not just final state
- Fixed-size arrays ensure deterministic RLP encoding (no variable-length big.Int)

### Example Encoded BAL

**Scenario**: Single transaction (index 1) transferring ETH from A to B

```
Accounts = [
    // Address A (sender)
    AccountChanges{
        Address: 0xAAAA...AAAA,
        StorageReads: [],      // no storage reads
        StorageChanges: [],    // no storage changes
        BalanceChanges: [
            BalanceChange{
                BlockAccessIndex: 1,
                PreBalance:  [32]byte{0x00...0x000D3C21BCECCEDA1000000}, // 1 ETH (last 16 bytes)
                PostBalance: [32]byte{0x00...0x000D3010CADB0FEA000000}  // 0.99 ETH (last 16 bytes)
            }
        ],
        NonceChanges: [
            NonceChange{
                BlockAccessIndex: 1,
                PreNonce:  [8]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // nonce 0
                PostNonce: [8]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}  // nonce 1 (little-endian)
            }
        ],
        CodeChanges: []  // no code changes
    },

    // Address B (recipient)
    AccountChanges{
        Address: 0xBBBB...BBBB,
        StorageReads: [],      // no storage reads
        StorageChanges: [],    // no storage changes
        BalanceChanges: [
            BalanceChange{
                BlockAccessIndex: 1,
                PreBalance:  [32]byte{0x00...0x00}, // 0 ETH
                PostBalance: [32]byte{0x00...0x002386F26FC10000}  // 0.01 ETH (last 16 bytes)
            }
        ],
        NonceChanges: [],  // no nonce changes (received only)
        CodeChanges: []    // no code changes
    }
]

RLP-encoded size: ~200 bytes (includes pre/post values for all changes)
Hash: Blake2b(rlp.encode(Accounts)) = 0x1234...5678
```

**Note**: All values use fixed-size arrays:
- Balance: 32-byte array with uint128 in last 16 bytes (big-endian)
- Nonce: 8-byte array with uint64 (little-endian, matching ubt BasicData format)
- Storage: 32-byte arrays for keys and values

---

## Code Touch Points

Implementation requires changes across multiple files:

### New Files

- **`statedb/block_access_list.go`**: Core BAL structs + builder + serializer (RLP + Blake2b)
  - `BlockAccessList`, `AccountChanges`, and all change type definitions
  - `BuildBlockAccessList()` function
  - `Hash()` method for hash computation

### Modified Files

- **`storage/storage.go`**: Add `TxIndex` to UBTReadEntry
  ```go
  type UBTReadEntry struct {
      // ... existing fields ...
      TxIndex uint32  // NEW: Transaction index within work package
  }
  ```

- **`storage/witness.go`**: Update witness serialization (157 → 161 bytes)
  - Serialize `TxIndex` at end of each entry
  - Update deserialization to read `TxIndex`
  - Expose parsed metadata to BAL builder

- **`statedb/refine.go`**: Capture write intents with incrementing `blockAccessIndex`
  - Create `WriteEventTracker` at start of `BuildBundle()`
  - Call `tracker.NextTransaction()` after each transaction
  - Build BAL from witness + tracker
  - Compute and attach `block_access_list_hash` to work item

- **`statedb/statedb_evm.go`**: Track current transaction index
  - Add `currentTxIndex uint32` field
  - Add `SetCurrentTransactionIndex()` method
  - Call before each transaction execution

- **`statedb/hostfunctions.go`**: Pass `TxIndex` to ubt read tracking
  - Update all `AppendUBTRead()` calls to include `stateDB.GetCurrentTransactionIndex()`
  - Surface `block_access_list_hash` to Rust if needed in payload metadata

- **`statedb/evmtypes/ubt.go`**: Helper to extract address/key from metadata for grouping
  - Utility functions for BAL construction from witness entries

- **`types/bundle.go`**: Add BAL fields to WorkItem
  ```go
  type WorkItem struct {
      // ... existing fields ...
      BlockAccessList     *statedb.BlockAccessList
      BlockAccessListHash common.Hash
  }
  ```

### Rust Side (Optional)

- **`services/evm/src/refiner.rs`**: Parse and validate `block_access_list_hash` from work package metadata

---

## Determinism Guardrails

**CRITICAL for hash consistency between builder and guarantor**:

1. **No Map Iteration**:
   - NEVER iterate Go maps directly for serialization
   - Always convert maps to sorted slices before encoding
   - Example: `for addr := range accountMap` → collect addresses → sort → iterate sorted slice

2. **Stable Sorting**:
   - Sort accounts by address bytes (lexicographic): `bytes.Compare(addr1[:], addr2[:])`
   - Sort keys by bytes (lexicographic): `bytes.Compare(key1[:], key2[:])`
   - Sort writes by `blockAccessIndex` (numeric)

3. **Monotonic Indices**:
   - `blockAccessIndex` assigned sequentially during execution: 0, 1, 2, ...
   - MUST match `txIndex` in witness metadata
   - Initialize `WriteEventTracker` with correct starting index (usually 1)

4. **Identical Parsing**:
   - Builder and guarantor use SAME code to parse witness
   - Builder and guarantor use SAME algorithm to build BAL
   - Any deviation causes hash mismatch

5. **Fixed-Length Types**:
   - Use `[32]byte`, `[8]byte` instead of `common.Hash`, `uint64` where critical
   - Ensures deterministic RLP encoding (no variable-length integers)

6. **Field Order**:
   - RLP encoding MUST match Go struct field order
   - AccountChanges: `[Address, StorageReads[], StorageChanges[], BalanceChanges[], NonceChanges[], CodeChanges[]]`

7. **Testing for Determinism**:
   - Run same input through BAL construction 100+ times
   - Assert all hashes are identical
   - Fuzz test: shuffle inputs, re-sort, verify hash unchanged

---

## Testing Strategy

### Testing Plan

1. **Unit Tests**: Feed synthetic pre/post witness sections and confirm stable hash over multiple runs
2. **Integration Tests**: Extend `rollup_test.go` to assert `block_access_list_hash` matches between builder and guarantor
3. **Regression Tests**: Fuzz ordering (shuffle inputs) to ensure determinism and absence of errors
4. **Compatibility Tests**: Cross-check empty list hash and single-tx example hashes to ensure RLP/Blake2b encoding matches expectations

---

## Testing Strategy

### Unit Tests

**File**: `statedb/access_list_test.go` (new)

```go
func TestBlockAccessListRLPEncoding(t *testing.T) {
    // Create balance with 100 wei (as [32]byte with value in last 16 bytes, big-endian)
    var preBalance, postBalance [32]byte
    // 100 in big-endian in last 16 bytes: 0x00000000000000000000000000000064
    postBalance[31] = 0x64 // 100 in hex

    bal := &BlockAccessList{
        Accounts: []AccountChanges{
            {
                Address: common.HexToAddress("0x1234"),
                BalanceChanges: []BalanceChange{
                    {
                        BlockAccessIndex: 1,
                        PreBalance:       preBalance,  // 0
                        PostBalance:      postBalance, // 100
                    },
                },
            },
        },
    }

    // Encode
    encoded, err := rlp.EncodeToBytes(bal)
    require.NoError(t, err)

    // Decode
    var decoded BlockAccessList
    err = rlp.DecodeBytes(encoded, &decoded)
    require.NoError(t, err)

    // Verify
    require.Equal(t, bal.Accounts[0].Address, decoded.Accounts[0].Address)
    require.Equal(t, bal.Accounts[0].BalanceChanges[0].PostBalance,
                   decoded.Accounts[0].BalanceChanges[0].PostBalance)
}

func TestBlockAccessListHashDeterminism(t *testing.T) {
    // Create identical BALs
    createBAL := func() *BlockAccessList {
        var balance [32]byte
        balance[31] = 0x64 // 100
        return &BlockAccessList{
            Accounts: []AccountChanges{
                {
                    Address: common.HexToAddress("0x1234"),
                    BalanceChanges: []BalanceChange{
                        {BlockAccessIndex: 1, PreBalance: [32]byte{}, PostBalance: balance},
                    },
                },
            },
        }
    }

    bal1 := createBAL()
    bal2 := createBAL()

    // Same content should produce same hash (determinism test)
    hash1 := bal1.Hash()
    hash2 := bal2.Hash()
    require.Equal(t, hash1, hash2, "Identical BALs must produce identical hashes")

    // Run multiple times to ensure no randomness
    for i := 0; i < 100; i++ {
        balN := createBAL()
        require.Equal(t, hash1, balN.Hash(), "Hash must be deterministic")
    }
}

func TestEmptyBlockAccessListHash(t *testing.T) {
    bal := &BlockAccessList{Accounts: []AccountChanges{}}
    hash := bal.Hash()

    // Implementation reference: storage/block_access_list_test.go
    // expectedHash := Blake2b([]byte{0xc0}) // RLP empty list
    // EIP-7928: 0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347
    require.Equal(t, expectedHash, hash)
}
```

### Integration Tests

**File**: `statedb/access_list_integration_test.go` (new)

```go
func TestBuilderBALGeneration(t *testing.T) {
    // Setup genesis with funded account
    builder := setupTestBuilder(t)

    // Execute transaction
    tx := createTestTransfer(from, to, amount)
    bundle, err := builder.BuildBundle([][]byte{tx})
    require.NoError(t, err)

    // Verify BAL was generated
    require.NotNil(t, bundle.WorkItem.BlockAccessList)
    require.NotEqual(t, common.Hash{}, bundle.WorkItem.BlockAccessListHash)

    // Verify BAL contains expected accounts
    bal := bundle.WorkItem.BlockAccessList
    require.Len(t, bal.Accounts, 2) // sender + recipient

    // Verify sender has balance + nonce changes
    sender := findAccount(bal, from)
    require.Len(t, sender.BalanceChanges, 1)
    require.Len(t, sender.NonceChanges, 1)

    // Verify recipient has balance change
    recipient := findAccount(bal, to)
    require.Len(t, recipient.BalanceChanges, 1)
}

func TestGuarantorBALVerification(t *testing.T) {
    // Builder executes and creates BAL
    builder := setupTestBuilder(t)
    tx := createTestTransfer(from, to, amount)
    bundle, err := builder.BuildBundle([][]byte{tx})
    require.NoError(t, err)

    // Guarantor re-executes with witness
    guarantor := setupTestGuarantor(t, bundle.Witness)
    err = guarantor.Execute([][]byte{tx})
    require.NoError(t, err)

    // Verify BAL matches
    err = VerifyBlockAccessList(
        bundle.WorkItem.BlockAccessList,
        bundle.WorkItem.BlockAccessListHash,
        guarantor.ubtReadLog,
        guarantor.contractWitness,
        1,
    )
    require.NoError(t, err)
}

func TestGuarantorDetectsFraudulentBAL(t *testing.T) {
    // Builder creates correct BAL
    builder := setupTestBuilder(t)
    tx := createTestTransfer(from, to, amount)
    bundle, err := builder.BuildBundle([][]byte{tx})
    require.NoError(t, err)

    // Tamper with builder's BAL - change PostBalance to fraudulent value
    var fraudBalance [32]byte
    // Set fraudulent balance: 999999 wei in last 16 bytes (big-endian)
    binary.BigEndian.PutUint64(fraudBalance[24:32], 999999)
    bundle.WorkItem.BlockAccessList.Accounts[0].BalanceChanges[0].PostBalance = fraudBalance
    bundle.WorkItem.BlockAccessListHash = bundle.WorkItem.BlockAccessList.Hash()

    // Guarantor detects fraud
    guarantor := setupTestGuarantor(t, bundle.Witness)
    err = guarantor.Execute([][]byte{tx})
    require.NoError(t, err)

    err = VerifyBlockAccessList(
        bundle.WorkItem.BlockAccessList,
        bundle.WorkItem.BlockAccessListHash,
        guarantor.ubtReadLog,
        guarantor.contractWitness,
        1,
    )
    require.Error(t, err)
    require.Contains(t, err.Error(), "balance value mismatch")
}
```

---

## Security Considerations

### Fraud Detection Scenarios

1. **Missing Storage Read**:
   - Builder omits storage slot from BAL
   - Guarantor reconstructs BAL with additional read
   - Hash mismatch → fraud detected

2. **Incorrect Post-Value**:
   - Builder claims wrong post-balance
   - Guarantor computes correct value from execution
   - Hash mismatch → fraud detected

3. **Extra/Missing Transaction**:
   - Builder includes phantom transaction in BAL
   - Guarantor's BAL has different account count
   - Hash mismatch → fraud detected

4. **Wrong Transaction Order**:
   - Builder reorders transactions (changes BlockAccessIndex)
   - Guarantor follows correct order
   - Hash mismatch → fraud detected

### Attack Vectors

**Builder Attacks**:
- ✅ Mitigated: Builder cannot fake BAL hash (cryptographic binding)
- ✅ Mitigated: Guarantor recomputes entire BAL from scratch
- ✅ Mitigated: RLP encoding is deterministic (same data → same hash)

**Guarantor Attacks**:
- ✅ Mitigated: Guarantor has no incentive to lie (loses stake)
- ✅ Mitigated: Multiple guarantors cross-validate
- ✅ Mitigated: BAL verification is deterministic (all honest guarantors agree)

**Ordering Attacks**:
- ✅ Mitigated: Accounts sorted lexicographically (deterministic)
- ✅ Mitigated: Storage keys sorted lexicographically
- ✅ Mitigated: BlockAccessIndex enforces transaction order

---

## Performance Considerations

### BAL Construction Cost

**Builder**:
- Parse ubtReadLog: O(n) where n = read count
- Parse contractWitnessBlob: O(m) where m = write count
- Sort accounts: O(a log a) where a = account count
- Sort per-account keys: O(k log k) where k = keys per account
- **Total**: O(n + m + a log a + ak log k)
- **Expected**: 5-10ms for typical transactions

**Guarantor**:
- Same construction cost as builder
- Hash comparison: O(1)
- **Total**: 5-10ms + negligible verification

### Memory Usage

**BAL Size**:
- Per account: ~100 bytes (address + list headers)
- Per storage change: ~68 bytes (key + index + value)
- Per storage read: ~32 bytes (key)
- Per balance/nonce change: ~24 bytes (index + value)
- Per code change: ~4 bytes + len(code)

**Example**:
- Simple ETH transfer: 2 accounts, 4 balance/nonce changes → ~300 bytes
- Contract deployment: 1 account, 1 code change + 5 storage writes → ~500 bytes + code size
- Complex DeFi transaction: 5 accounts, 50 storage changes → ~4 KB

**RLP-encoded BAL**: 90-95% efficiency (minimal overhead)

### Optimization Opportunities

1. **Lazy BAL Construction**:
   - Only build BAL if guarantors request it
   - Store raw logs, compute hash on-demand

2. **Incremental Hashing**:
   - Hash accounts as they're added (streaming)
   - Avoid re-encoding entire structure

3. **Parallel Construction**:
   - Build per-account BAL entries in parallel
   - Merge and sort at end

4. **Caching**:
   - Cache RLP-encoded accounts between blocks
   - Only re-encode modified accounts

---

## Implementation Checklist

### Phase 1: Core Data Structures ✅
- [x] Define `BlockAccessList`, `AccountChanges`, and change types → [storage/block_access_list.go:18-78](../../storage/block_access_list.go#L18-L78)
- [x] Implement RLP encoding/decoding → Uses `github.com/ethereum/go-ethereum/rlp` (built-in)
- [x] Implement `Hash()` method (Blake2b) → [storage/block_access_list.go:77-84](../../storage/block_access_list.go#L77-L84)
- [x] Unit tests for serialization → [storage/block_access_list_test.go](../../storage/block_access_list_test.go) (13 tests)

### Phase 2: Builder Construction ✅
- [x] Implement `BuildBlockAccessList()` → [storage/block_access_list.go:429-469](../../storage/block_access_list.go#L429-L469)
- [x] Parse augmented pre-state witness for reads → [storage/block_access_list.go:112-183](../../storage/block_access_list.go#L112-L183)
- [x] Parse augmented post-state witness for writes → [storage/block_access_list.go:185-250](../../storage/block_access_list.go#L185-L250)
- [x] CodeSize extraction from BasicData → [storage/block_access_list.go:308-345](../../storage/block_access_list.go#L308-L345)
- [x] Sort and deduplicate entries → [storage/block_access_list.go:366-457](../../storage/block_access_list.go#L366-L457)
- [x] Unit tests for construction logic → [storage/block_access_list_test.go](../../storage/block_access_list_test.go)

### Phase 3: Transaction Index Tracking ✅ COMPLETE

**Phase 3A: Read Attribution (UBT State Fetches)**
- [x] Extend `UBTRead` with `TxIndex` → [types/storage.go:150](../../types/storage.go#L150)
- [x] Update witness serialization to 161 bytes → [storage/witness.go:199-266](../../storage/witness.go#L199-L266)
- [x] **Rust side**: Add `tx_index` parameter to `host_fetch_ubt()` and wrappers → [services/evm/src/ubt_host.rs](../../services/evm/src/ubt_host.rs)
- [x] **Rust side**: Pass `current_tx_index` from `MajikBackend` to ubt fetches → [services/evm/src/state.rs:133-931](../../services/evm/src/state.rs#L133-L931)
- [x] **Go side**: Read `tx_index` from register 12 in `hostFetchUBT()` → [statedb/hostfunctions.go:2546](../../statedb/hostfunctions.go#L2546)
- [x] **Go side**: Update `Fetch*()` signatures and pass to `AppendUBTRead()` → [storage/storage.go:429-622](../../storage/storage.go#L429-L622)
- [x] **Go side**: Update JAMStorage interface → [types/storage.go:486-490](../../types/storage.go#L486-L490)

**Phase 3B: Write Attribution (Contract Witness Blob) ⚠️ FORMAT READY, DATA NOT YET POPULATED**

**Problem Identified**: Write-only keys (e.g., recipient balance in transfers, new storage slots) were getting `TxIndex=0` because `extractKeyMetadata()` only derived tx_index from the read log. Write-only entries never appeared in reads, causing incorrect BAL construction.

**Partial Solution Implemented**: Blob format now supports per-write tx_index, but Rust side does not yet populate it correctly.

- [x] **Breaking Change**: Contract witness blob format updated from 25-byte to 29-byte header
  - Old format: `[20B address][1B kind][4B payload_length][payload]`
  - New format: `[20B address][1B kind][4B payload_length][4B tx_index][payload]`
- [x] **Rust side**: Add `tx_index: u32` field to `WriteEffectEntry` → [services/utils/src/effects.rs:79-85](../../services/utils/src/effects.rs#L79-L85)
- [x] **Rust side**: Update all `WriteEffectEntry` creations to include `tx_index` → [services/evm/src/backend.rs:197-609](../../services/evm/src/backend.rs#L197-L609)
  - ✅ **FIXED**: Writes now correctly attributed to the transaction that caused them
  - ✅ **Solution**: Track `tx_index` at write-time during transaction execution in `MajikOverlay`
  - ✅ **Implementation**: Added `write_tx_index: BTreeMap<WriteKey, u32>` to track per-write attribution
- [x] **Rust side**: Serialize 4-byte `tx_index` into blob → [services/evm/src/refiner.rs:1099-1104](../../services/evm/src/refiner.rs#L1099-L1104)
- [x] **Go side**: Update all blob parsing to read 29-byte header → [storage/witness.go:440-770](../../storage/witness.go#L440-L770)
- [x] **Go side**: Update `extractKeyMetadata()` to use blob's `txIndex` for writes → [storage/witness.go:460-547](../../storage/witness.go#L460-L547)
  - Changed from `TxIndex: keyToTxIndex[key32]` (read log lookup)
  - To `TxIndex: txIndex` (blob-provided value)
- [x] **Rust tracking**: Balance/nonce/code write methods track `tx_index` when state is modified → [services/evm/src/state.rs:633-730](../state.rs#L633-L730)
  - `set_code()` → tracks code deployments
  - `deposit()` / `withdrawal()` / `transfer()` → track balance changes
  - `inc_nonce()` → tracks nonce increments
  - `set_storage()` records per-slot tx_index, but see limitation below
- [x] **Rust lookup**: `process_*` functions consume tracked indices for balance/nonce/code writes → [services/evm/src/backend.rs:388-560](../backend.rs#L388-L560)
  - `process_code_changes()` → uses `WriteKey::Code(address)`
  - `process_balance_changes()` → uses `WriteKey::Balance(address)`
  - `process_nonce_changes()` → uses `WriteKey::Nonce(address)`
- ⚠️ **Storage limitation**: Contract witness blob still aggregates all storage slots into one `StorageShard` entry with `tx_index=0` (see [services/evm/src/backend.rs:628-634](../backend.rs#L628-L634)). The ubt witness carries per-slot `tx_index` for reads/writes, but when the Go side parses the contract blob, storage slot metadata inherits `tx_index=0`. Impact: BAL hash remains deterministic (builder and guarantor agree) and fraud detection works, but transaction-level attribution for storage writes is lost. Fix would require emitting per-slot storage entries (with per-slot `tx_index` from `write_tx_index` map) instead of an aggregate shard—an invasive format change.

### Phase 4: Builder/Guarantor Integration ✅ COMPLETE

**4A: Block Payload Structure Updates** ✅ COMPLETE
- [x] Add `BlockAccessListHash` field to `EvmBlockPayload` → [services/evm/src/block.rs:36](../block.rs#L36), [statedb/evmtypes/block.go:36](../../../statedb/evmtypes/block.go#L36)
- [x] Update `BLOCK_HEADER_SIZE` from 116→148 bytes → [services/evm/src/block.rs:13](../block.rs#L13), [statedb/evmtypes/block.go:36](../../../statedb/evmtypes/block.go#L36)
- [x] Update serialization/deserialization → [services/evm/src/block.rs:70-71,149-152](../block.rs#L70-L152), [statedb/evmtypes/block.go:165-196](../../../statedb/evmtypes/block.go#L165-L196)

**4B: BAL Computation Infrastructure** ✅ COMPLETE
- [x] Implement `SplitWitnessSections()` → [storage/block_access_list.go:529-563](../../storage/block_access_list.go#L529-L563)
- [x] Implement `ComputeBlockAccessListHash()` → [storage/storage.go:683-715](../../storage/storage.go#L683-L715)
- [x] Add to `JAMStorage` interface → [types/storage.go:509-513](../../types/storage.go#L509-L513)

**4C: Builder BAL Hash Computation** ✅ COMPLETE
- [x] Call `ComputeBlockAccessListHash()` in `BuildBundle()` → [statedb/refine.go:545](../../statedb/refine.go#L545)
- [x] Update block payload in exported segments with computed BAL hash → [statedb/refine.go:1052-1080](../../statedb/refine.go#L1052-L1080)
  - Implemented `updateBlockPayloadWithBALHash()` helper
  - Deserializes block payload, updates hash, re-serializes
  - Called after BAL hash computation → [statedb/refine.go:556](../../statedb/refine.go#L556)
- [x] Log BAL statistics (account count, total changes) → [statedb/refine.go:551](../../statedb/refine.go#L551)

**4D: Guarantor BAL Hash Verification** ✅ COMPLETE
- [x] Add `block_access_list_hash` to work item payload → [statedb/rollup.go:109-116](../../statedb/rollup.go#L109-L116)
  - Updated `BuildPayload()` to include 32-byte BAL hash field
  - Payload format: `type(1) + count(4) + depth(1) + witnesses(2) + bal_hash(32)` = 40 bytes
  - All callers updated to pass BAL hash → [statedb/refine.go:588](../../statedb/refine.go#L588)
  - Genesis/call payloads pass zero hash, transaction payloads pass computed hash
- [x] Parse `block_access_list_hash` from payload metadata in Rust → [services/evm/src/refiner.rs:40,85-86](../refiner.rs#L40)
  - Added `block_access_list_hash: [u8; 32]` field to `PayloadMetadata`
  - Updated `parse_payload_metadata()` to parse 40-byte payload
- [x] **Single point of verification (Rust only)** → [services/evm/src/refiner.rs:1024-1084](../refiner.rs#L1024-L1084)
  - **Payload type enforcement**: Transaction payloads MUST have non-zero BAL hash (lines 1028-1035)
    - `PayloadType::Transactions` and `PayloadType::Builder` require non-zero hash
    - Zero hash only allowed for `PayloadType::Genesis` and `PayloadType::Call`
  - **Witness requirement**: If BAL hash claimed → ubt witness MUST be provided (lines 1071-1078)
    - Missing witness when hash claimed = immediate rejection
    - Prevents witness stripping attacks
  - **Hash verification**: Compute BAL from witness, compare with payload claim (lines 1040-1064)
    - Builds BAL from `UBTWitness` entries and hashes it in Rust
    - Compares computed hash with builder's claimed hash from `PayloadMetadata`
    - Any mismatch = immediate rejection
  - **Fail-closed**: ALL errors reject the work package (no silent failures)
    - Missing witness → reject
    - Hash computation failure → reject
    - Hash mismatch → reject
    - Zero hash on transaction payload → reject

**Security Architecture**: Guarantor verification (Rust - single point of truth)
1. **Payload Type Check**: Transaction payloads MUST have non-zero BAL hash
2. **Witness Requirement Check**: If BAL hash claimed → ubt witness MUST be provided
3. **Hash Verification**: Compute BAL from guarantor's ubt witness, extract builder's claimed hash from payload metadata, compare: computed == claimed → Accept : Reject
4. **Fail-Closed**: Any error → REJECT immediately

**Why Go verification was removed** → [statedb/refine.go:360-364](../../statedb/refine.go#L360-L364)
- Guarantor's `exported_segments` contains their own re-executed block (BAL hash = zeros from Rust)
- Builder's block segment only available from DA during accumulate phase
- Comparing against guarantor's own block would cause false rejections
- Rust verification is sufficient and architecturally correct

### Phase 5: Testing & Validation ✅ COMPLETE (Unit Tests)
- [x] Unit tests for RLP encoding determinism → [storage/block_access_list_test.go:72-114](../../storage/block_access_list_test.go#L72-L114)
  - `TestBlockAccessListHashDeterminism`: Verifies 100+ runs produce identical hash
  - `TestEmptyBlockAccessListHash`: Verifies empty BAL has non-zero hash
  - `TestBlockAccessListRLPEncoding`: Round-trip encoding/decoding verification
- [x] Unit tests for witness parsing → [storage/block_access_list_test.go:211-300,976-1090](../../storage/block_access_list_test.go#L211-L300)
  - `TestParseAugmentedPreStateReads`: Parses pre-state section (161-byte entries with tx_index)
  - `TestParseAugmentedPostStateWrites`: Parses post-state section (writes with tx_index)
  - `TestSplitWitnessSections`: Splits dual-proof witness into pre/post sections
- [x] Unit tests for BAL construction from witness → [storage/block_access_list_test.go:693-880,882-974](../../storage/block_access_list_test.go#L693-L880)
  - `TestBuildBlockAccessListFromWitness`: Complete end-to-end witness → BAL construction
    - Builds dual-proof witness with 3 reads (1 read-only, 2 modified) and 2 writes
    - Verifies account sorting, storage read/write classification, tx_index attribution
    - Tests both balance and storage changes
  - `TestBALHashRoundTrip`: Builder/guarantor hash agreement verification
    - Creates realistic BAL with multiple accounts (balance, storage, nonce, code changes)
    - Verifies builder and guarantor compute identical hash after RLP round-trip
    - Tests with complex multi-transaction scenarios
- [x] Component tests for change builders → [storage/block_access_list_test.go:303-691](../../storage/block_access_list_test.go#L303-L691)
  - `TestBuildSlotChanges`: Storage write grouping by address and key
  - `TestClassifyStorageAccesses`: Read-only vs modified classification
  - `TestAssembleAccountChanges`: Deterministic account sorting
  - `TestBuildBalanceChanges`: Balance extraction from BasicData
  - `TestBuildCodeChanges`: Code size cross-reference with BasicData

### Phase 6: Integration & Security Tests ⚠️ CRITICAL
**Gap Analysis**: Current unit tests only cover RLP encoding, witness parsing, and BAL construction in Go. The Rust verification logic, PVM boundary, and end-to-end builder→guarantor flow are untested.

**6A: PoC Integration Test - Builder→Guarantor Flow** → **HIGHEST PRIORITY**
Test: `TestEVMBlocksTransfers` - End-to-end verification of first work package (builder → guarantor)

**Current Implementation Status** (✅ = implemented, ❌ = blocking):
- ✅ Builder computes BAL hash from witness ([refine.go:550-559](statedb/refine.go#L550))
- ✅ Builder embeds BAL hash in block payload ([refine.go:561-567](statedb/refine.go#L561))
- ✅ Builder passes non-zero BAL hash to `BuildPayload()` ([refine.go:592](statedb/refine.go#L592))
- ✅ Guarantor receives 40-byte payload with BAL hash (Rust parsing implemented)
- ✅ Guarantor rejects transaction payloads with zero BAL hash (verification working correctly)
- ❌ **BLOCKER**: Pre-existing ubt witness bug - `GenerateUBTProofWithTransition: empty key list`

**What's Actually Untested** (Critical Gaps):
1. **Payload round-trip** (Go BuildPayload → Rust parse_payload_metadata → BAL hash extraction) - untested
2. **Rust verification rules** (hash mismatch, missing witness, computation failure) - only tested via integration failure
3. **Builder→Guarantor agreement** (both compute identical hash from same witness) - blocked by ubt bug

**Blocking Issue - UBT Witness Generation**:
- **Error**: `GenerateUBTProofWithTransition: empty key list`
- **Root cause**: Balance-only/nonce-only transactions don't generate storage keys for witness
- **Location**: UBT witness generation in [statedb/witness.go](statedb/witness.go)
- **Impact**: Cannot generate post-state proof → cannot create ubt witness → cannot test BAL flow
- **Scope**: Unrelated to BAL implementation - upstream ubt tree issue
- **Workaround**: Use storage-writing transactions (contract calls) instead of simple transfers

**TODO - Fix Integration Test**:
- [ ] **Option 1**: Fix ubt witness generation to handle balance-only transactions
  - Add account keys to key list even when no storage changes
  - Ensure BasicData (balance, nonce, code) changes generate witness entries
  - Location: [statedb/witness.go](statedb/witness.go)

- [ ] **Option 2**: Change test to use storage-writing transactions
  - Deploy simple contract with storage writes
  - Verify BAL flow with non-empty key list
  - Defer balance-only fix to separate issue

**Expected Flow After Fix**:
1. Builder executes transaction (with storage writes to avoid empty key list)
2. Builder generates ubt witness with state changes
3. Builder computes BAL hash: `blockAccessListHash = ComputeBlockAccessListHash(witness)`
4. Builder embeds hash in block payload (148-byte header, bytes 116-148)
5. Builder creates work item payload: `BuildPayload(..., blockAccessListHash)` (40 bytes, hash at bytes 8-40)
6. Guarantor receives work package with non-zero BAL hash in payload metadata
7. Guarantor re-executes transaction and generates own witness
8. Guarantor computes BAL hash in Rust from UBT witness entries
9. Guarantor compares computed hash with builder's claimed hash → ACCEPT
10. Test passes - first work package verified successfully

**6B: Rust Verification Rule Tests** → **CRITICAL**
- [ ] Test: Transaction payload with zero BAL hash → MUST reject
  - Verify PayloadType::Transactions with block_access_list_hash = [0; 32] is rejected
  - Verify PayloadType::Builder with zero hash is rejected
  - Verify error message: "Transaction payload missing BAL hash"
- [ ] Test: Genesis/call payload with zero BAL hash → MUST accept
  - Verify PayloadType::Genesis with zero hash is accepted (no verification)
  - Verify PayloadType::Call with zero hash is accepted
- [ ] Test: Missing witness when BAL hash claimed → MUST reject
  - Provide non-zero BAL hash in payload metadata
  - Omit ubt witness from extrinsics
  - Verify error message: "Builder claimed BAL hash ... but no UBT witness provided"
- [ ] Test: Hash mismatch → MUST reject
  - Provide ubt witness with state changes
  - Provide incorrect BAL hash in payload metadata
  - Verify guarantor computes correct hash and rejects mismatch
  - Verify error message includes both hashes
- [ ] Test: Hash computation failure → MUST reject
  - Provide malformed ubt witness (e.g., truncated, invalid format)
  - Verify witness parsing fails before BAL hash computation
  - Verify error message: "Failed to compute BAL hash from witness"

**6C: Payload Format Propagation Tests** → **CRITICAL**
- [ ] Test: 40-byte payload round-trip
  - Go: BuildPayload(PayloadTypeTransactions, count, depth, witnesses, bal_hash) → 40 bytes
  - Rust: parse_payload_metadata(payload) → extract all fields
  - Verify block_access_list_hash bytes [8:40] match Go input
  - Test with various BAL hashes (zero, non-zero, all 0xFF, random)
- [ ] Test: Payload length validation
  - Rust: parse_payload_metadata with < 40 bytes → MUST reject
  - Verify error message: "Payload too short ... expected >=40"
- [ ] Test: updateBlockPayloadWithBALHash preserves hash
  - Builder computes BAL hash in Go
  - Updates block payload via updateBlockPayloadWithBALHash
  - Deserialize updated payload
  - Verify BlockAccessListHash field matches input
  - Test with 148-byte header (verify no truncation)

**6D: Rust BAL Computation Tests** → **CRITICAL**
- [ ] Test: malformed ubt witness rejects before BAL hashing
  - Invalid witness: truncated (< 68 bytes)
  - Invalid witness: corrupted count field
  - Invalid witness: wrong entry size
- [ ] Test: BAL computation uses canonical ordering
  - Build a witness with multiple accounts and storage keys
  - Verify BAL hash is stable across repeated runs
  - Test with witness at different memory addresses (alignment)

**6E: Integration Tests - Builder/Guarantor Flows**
- [ ] Test: Builder flow end-to-end (covered by 6A PoC)
  - Execute transactions
  - Generate ubt witness
  - Compute BAL hash from witness
  - Update block payload with BAL hash
  - Update work item payload metadata with BAL hash
  - Verify all BAL hash values consistent across payload metadata, block payload, and computed from witness
- [ ] Test: Guarantor verification - correct builder (covered by 6A PoC)
  - Guarantor receives work package with builder's BAL hash in payload
  - Guarantor receives builder's ubt witness in extrinsics
  - Guarantor re-executes transactions
  - Guarantor computes BAL hash from witness
  - Guarantor compares with builder's claimed hash → ACCEPT
- [ ] Test: Guarantor verification - fraudulent builder scenarios
  - Scenario 1: Builder claims wrong BAL hash (hash mismatch) → REJECT
  - Scenario 2: Builder strips witness (missing witness) → REJECT
  - Scenario 3: Builder provides mismatched witness (hash mismatch) → REJECT
  - Scenario 4: Builder uses zero hash on transaction payload (zero hash not allowed) → REJECT (covered by 6A PoC current failure)

**6F: Negative Tests - Edge Cases**
- [ ] Test: Empty ubt witness (no reads, no writes)
  - Verify non-zero BAL hash produced (RLP of empty structure)
  - Verify hash deterministic
- [ ] Test: Very large ubt witness (1000+ entries)
  - Verify performance acceptable
  - Verify no truncation or buffer overflow
- [ ] Test: Concurrent verification (multiple guarantors)
  - Verify all guarantors compute identical BAL hash
  - Verify deterministic rejection of fraudulent builder

**Priority**:
1. **6A (PoC)** - HIGHEST PRIORITY - Validates entire builder→guarantor flow
2. **6B, 6C, 6D** - CRITICAL - Validate core security properties and PVM boundary
3. **6E, 6F** - HIGH - Validate edge cases and fraud detection


## Open Questions

1. **Transaction Index for System Calls**:
   - How to assign BlockAccessIndex for genesis, validator rewards, etc.?
   - Proposal: Use index 0 for pre-execution, n+1 for post-execution

2. **Multi-Work-Package Blocks**:
   - Should BAL be per work package or per accumulated block?
   - Proposal: Per work package (aligns with JAM architecture)

3. **BAL Storage**:
   - Should BAL be included in extrinsics or in work package header?
   - Proposal: Header only (BAL is metadata, not execution data)

4. **Partial BAL Verification**:
   - Can guarantors verify only accounts they care about?
   - Proposal: No, hash is all-or-nothing (security requirement)

5. **BAL Compression**:
   - Should we compress BAL before hashing?
   - Proposal: No, keep it simple (compression breaks determinism)

---

## Future Enhancements

### Phase 2 Features

1. **Diff-Based BAL**:
   - Only include changes from previous block
   - Reduces size for sequential blocks

2. **Merkle-ized BAL**:
   - Build Merkle tree of accounts
   - Enable partial verification

3. **Access Pattern Analysis**:
   - Detect hot accounts/slots
   - Optimize state access patterns

4. **Cross-Work-Package Dependencies**:
   - Track inter-work-package reads
   - Enable parallel execution analysis

---

## References

- **EIP-7928**: Block-Level Access Lists
  - https://eips.ethereum.org/EIPS/eip-7928

- **EIP-6800**: Ethereum state using a unified ubt tree
  - https://eips.ethereum.org/EIPS/eip-6800

- **UBT Trees**:
  - https://vitalik.eth.limo/general/2021/06/18/ubt.html

- **JAM Gray Paper**:
  - Work packages, refine/accumulate phases

- **go-ethereum RLP**:
  - https://pkg.go.dev/github.com/ethereum/go-ethereum/rlp

---

## File References

**New Files** (to be created):
- `statedb/access_list.go` - Core data structures
- `statedb/access_list_builder.go` - Builder construction logic
- `statedb/access_list_verifier.go` - Guarantor verification logic
- `statedb/access_list_test.go` - Unit tests
- `statedb/access_list_integration_test.go` - Integration tests

**Modified Files**:
- `statedb/refine.go` - Integrate BAL into BuildBundle
- `storage/storage.go` - Add TxIndex to UBTReadEntry
- `statedb/hostfunctions.go` - Pass TxIndex to AppendUBTRead
- `statedb/statedb_evm.go` - Add currentTxIndex tracking
- `types/bundle.go` - Add BAL fields to WorkItem

**Existing Files** (for context):
- `services/evm/docs/UBT-CODEX.md` - UBT witness documentation
- `storage/witness.go` - UBT witness construction
- `statedb/evmtypes/ubt.go` - UBT key generation

---
