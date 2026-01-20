# Builder Network: Decoupled Execution and Witness Generation

This document captures the two-phase flow for decoupling EVM execution from
witness generation. Finality and consensus details are intentionally deferred.

---

## Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│  PHASE 1: Builder Network (Fast, No Witnesses)                          │
│                                                                         │
│  TxPool → ExecuteEVMBatch(N blocks) → Phase1Result                      │
│         • No UBT read logging                                           │
│         • No witness generation                                         │
│         • Output: block_hashes[], pre_root, post_root, receipts         │
│                                                                         │
│  Builder Consensus: Agree on (batch_hash, pre_root, post_root)          │
└─────────────────────────────────────────────────────────────────────────┘
                                    ↓
                          Agreement Reached
                                    ↓
┌─────────────────────────────────────────────────────────────────────────┐
│  PHASE 2: JAM Submission (Re-execute with Witnesses)                    │
│                                                                         │
│  Phase1Result → Re-execute with UBT logging → Generate witnesses        │
│               → Build WorkPackageBundle → Submit to JAM validators      │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Phase 1: Builder Network Execution

### Purpose
Execute EVM transactions quickly without witness overhead. Builders agree on
the canonical block sequence before any JAM interaction.

### Batching Strategy
Following Sourabh's guidance, target **6 seconds of rollup blocks per work package**:
- 6 blocks at 1s block time
- 12 blocks at 0.5s block time
- Up to 16 blocks at 0.375s block time (max work items per work package)

This amortizes JAM submission overhead across multiple rollup blocks.

### Phase1Result Structure

```go
type Phase1Result struct {
    // Batch identification
    BatchID         common.Hash     // Hash of (pre_root, block_hashes, post_root)

    // State roots (critical for determinism)
    EVMPreStateRoot  common.Hash    // EVM root BEFORE batch execution (not JAM)
    EVMPostStateRoot common.Hash    // EVM root AFTER batch execution (not JAM)

    // Per-block outputs (N blocks in batch)
    Blocks          []BlockResult   // One per rollup block

    // Deterministic inputs (needed for Phase 2 replay)
    TxLists         [][]Transaction // Per-block transaction lists
    BlockContexts   []BlockContext  // Per-block context (timestamp, number, etc.)

    // Builder signature (for consensus)
    BuilderPubKey   []byte
    Signature       []byte          // Sign(BatchID)
}

type BlockResult struct {
    BlockNumber     uint64
    BlockHash       common.Hash
    StateRoot       common.Hash     // Post-state root for this block
    ReceiptsRoot    common.Hash
    GasUsed         uint64
}
```

### Execution Mode

```go
func ExecutePhase1Batch(
    txLists [][]Transaction,
    blockContexts []BlockContext,
) (*Phase1Result, error) {

    evmStorage := getEVMStorage()

    // 1. Capture pre-state root
    preRoot := evmStorage.GetCanonicalRoot()
    activeRoot := preRoot

    // 2. DISABLE witness tracking for fast execution
    evmStorage.DisableUBTReadLog()

    // 3. Execute each block in sequence
    blocks := make([]BlockResult, len(txLists))
    for i, txs := range txLists {
        result := executeEVMBlock(txs, blockContexts[i])
        postRoot, _ := evmStorage.ApplyWritesToTree(activeRoot, result.StateChanges)
        activeRoot = postRoot
        blocks[i] = BlockResult{
            BlockNumber:  blockContexts[i].Number,
            BlockHash:    result.BlockHash,
            StateRoot:    postRoot,
            ReceiptsRoot: result.ReceiptsRoot,
            GasUsed:      result.GasUsed,
        }
    }

    // 4. Capture post-state root
    postRoot := activeRoot

    // 5. Compute batch ID
    batchID := computeBatchID(preRoot, blocks, postRoot)

    return &Phase1Result{
        BatchID:       batchID,
        EVMPreStateRoot:  preRoot,
        EVMPostStateRoot: postRoot,
        Blocks:        blocks,
        TxLists:       txLists,
        BlockContexts: blockContexts,
    }, nil
}
```

---

## Builder Network Protocol

### Message Types

```go
// UP1: Builder announces Phase1Result to peers
type BuilderProposal struct {
    Phase1Result  *Phase1Result
    BuilderID     uint32
    Signature     []byte
}

// BuilderVote: Other builders confirm they can reproduce the result
type BuilderVote struct {
    BatchID       common.Hash
    VoterID       uint32
    CanReproduce  bool          // Did re-execution match?
    Signature     []byte
}

// BuilderAgreement: Threshold reached, batch is canonical
type BuilderAgreement struct {
    BatchID       common.Hash
    Phase1Result  *Phase1Result
    Votes         []BuilderVote // 2/3+ votes
    AggSignature  []byte        // Aggregated signature (BLS or multisig)
}
```

### Consensus Flow

```
Builder A executes batch → broadcasts BuilderProposal
                                ↓
Builders B, C, D receive proposal
    → Re-execute same txs against same pre_root
    → Compare post_root
    → If match: broadcast BuilderVote{CanReproduce: true}
                                ↓
Once 2/3+ votes collected → BuilderAgreement formed
                                ↓
Any builder can proceed to Phase 2
```

### State Pinning for Verification

When a builder receives a `BuilderProposal`, they must verify against the
**same pre-state root**. This requires state pinning:

```go
func VerifyProposal(proposal *BuilderProposal) (*BuilderVote, error) {
    evmStorage := getEVMStorage()

    // Pin to the proposal's pre-state root
    if err := evmStorage.PinToStateRoot(proposal.Phase1Result.EVMPreStateRoot); err != nil {
        // We don't have this state - cannot verify
        return nil, ErrStateTooOld
    }
    defer evmStorage.UnpinState()

    // Re-execute without witnesses
    evmStorage.DisableUBTReadLog()

    result, err := ExecutePhase1Batch(
        proposal.Phase1Result.TxLists,
        proposal.Phase1Result.BlockContexts,
    )
    if err != nil {
        return &BuilderVote{CanReproduce: false}, nil
    }

    // Compare results
    canReproduce := result.EVMPostStateRoot == proposal.Phase1Result.EVMPostStateRoot

    return &BuilderVote{
        BatchID:      proposal.Phase1Result.BatchID,
        CanReproduce: canReproduce,
    }, nil
}
```

---

## Phase 2: JAM Submission

### Purpose
Re-execute the agreed batch with UBT read logging enabled, generate witnesses,
and submit to JAM validators.

### Execution Mode

```go
func ExecutePhase2(agreement *BuilderAgreement) (*types.WorkPackageBundle, error) {
    phase1 := agreement.Phase1Result
    evmStorage := getEVMStorage()

    // 1. Pin to the agreed pre-state root
    if err := evmStorage.PinToStateRoot(phase1.EVMPreStateRoot); err != nil {
        return nil, fmt.Errorf("cannot pin to pre-state: %w", err)
    }

    // 2. ENABLE UBT read logging for witness generation
    evmStorage.EnableUBTReadLog()
    evmStorage.ClearUBTReadLog()

    // 3. Re-execute the batch (populates read log)
    var allStateChanges []byte
    activeRoot := phase1.EVMPreStateRoot
    for i, txs := range phase1.TxLists {
        result := executeEVMBlock(txs, phase1.BlockContexts[i])
        allStateChanges = append(allStateChanges, result.StateChanges...)
        nextRoot, _ := evmStorage.ApplyWritesToTree(activeRoot, result.StateChanges)
        activeRoot = nextRoot

        // Verify we match Phase 1 result
        if activeRoot != phase1.Blocks[i].StateRoot {
            return nil, ErrStateMismatch
        }
    }

    // 4. Verify final post-state matches
    postRoot := activeRoot
    if postRoot != phase1.EVMPostStateRoot {
        return nil, fmt.Errorf("post-state mismatch: expected %x, got %x",
            phase1.EVMPostStateRoot, postRoot)
    }

    // 5. Generate UBT witnesses (read log is now populated)
    ubtPre, ubtPost, err := evmStorage.BuildUBTWitness(allStateChanges)
    if err != nil {
        return nil, fmt.Errorf("witness generation failed: %w", err)
    }

    // 6. Build work package with fresh RefineContext
    wp := buildWorkPackage(phase1, ubtPre, ubtPost)
    wp.RefineContext = getFreshRefineContext() // Fresh anchor for JAM validity window

    // 7. Attach builder agreement (finality proof for light clients)
    // This is stored at accumulate time (finality details TBD)

    return buildBundle(wp, ubtPre, ubtPost)
}
```

---

## Critical Requirements

### 1. State Root Pinning
- Phase 1 captures `EVMPreStateRoot` before execution
- Phase 2 must pin to the exact same root
- Verification by other builders requires pinning to proposal's root

### 2. Deterministic Execution
Identical inputs must produce identical outputs:
- Transaction list (ordered)
- Block context (timestamp, number, coinbase, gas limit, etc.)
- Contract code versions (code hash in work item)
- EVM version / hard fork rules

### 3. Re-execution Requirement
Phase 2 **must** re-execute if Phase 1 skipped UBT read logging.
The read log is required for `BuildUBTWitness` to know which keys need proofs.

### 4. Mismatch Handling
If Phase 2 produces a different post-state root than Phase 1:
- Abort submission
- Log the discrepancy
- (Future) Treat as builder fault if slashing is implemented

---

## Integration with Existing Code

### Storage Layer Changes

The root-first storage model provides explicit pre/post roots per bundle:

| Requirement | Existing Support | Notes |
|-------------|------------------|-------|
| UBT read log | `ubtReadLog`, `EnableUBTReadLog()` | Need `DisableUBTReadLog()` |
| Root-based trees | `treeStore`, `GetTreeByRoot()` | Copy-on-write per root |
| Active tree selection | `GetActiveTree()`, `SetActiveRoot()` | Explicit root pinning |
| Witness generation | `BuildUBTWitness()` | Uses read log |

### New Methods Needed

```go
// Disable UBT read logging for Phase 1 fast execution
func (s *StateDBStorage) DisableUBTReadLog()

// Pin execution to a specific state root
func (s *StateDBStorage) PinToStateRoot(root common.Hash) error

// Get canonical root without tree access
func (s *StateDBStorage) GetCanonicalRoot() common.Hash
```

### Work Package Structure for Batched Blocks

Each rollup block becomes a **work item** within the work package:

```go
WorkPackage {
    WorkItems: [
        { /* Block 1: txs, payload, extrinsics */ },
        { /* Block 2: txs, payload, extrinsics */ },
        ...
        { /* Block N: txs, payload, extrinsics */ },
    ]
    // UBT witnesses cover the entire batch (pre-state to final post-state)
}
```

---

## Benefits

1. **Fast builder agreement** - No witness overhead in Phase 1
2. **Deterministic ordering** - Builder consensus before JAM sees anything
3. **Maximized validity window** - RefineContext anchor set at Phase 2
4. **Batching efficiency** - Multiple rollup blocks per work package
5. **Light client friendly** - Builder agreement provides finality signal

---

## RPC Architecture: Builder Primary, DA Fallback

### Design Principle

Builders are the **PRIMARY** source for EVM RPC. Validators/DA serve only as **FALLBACK** after blocks are on-chain via JAM.

```
User RPC Request
      │
      ▼
┌─────────────────────────────────────┐
│  Builder RPC (PRIMARY)              │
│  ─────────────────────────────────  │
│  • Soft finality (builder consensus)│
│  • Low latency (local cache)        │
│  • Recent blocks + pending state    │
│                                     │
│  Serves:                            │
│  - eth_getBlockByNumber (recent)    │
│  - eth_getBlockByHash              │
│  - eth_getBalance                  │
│  - eth_call                        │
│  - eth_getTransactionReceipt       │
│  - eth_sendRawTransaction          │
└────────────┬────────────────────────┘
             │ fallback (if builder unavailable
             │           or historical query)
             ▼
┌─────────────────────────────────────┐
│  Validator RPC (FALLBACK)           │
│  ─────────────────────────────────  │
│  • Hard finality (JAM on-chain)     │
│  • Higher latency (DA lookup)       │
│  • Historical/archival data         │
│                                     │
│  Serves:                            │
│  - Historical blocks (after JAM)    │
│  - Canonical state verification     │
│  - Archival queries                 │
└─────────────────────────────────────┘
```

### Why DA Cannot Be Primary

1. **Latency**: DA requires full JAM pipeline (work package → guarantee → assurance → accumulate). This adds 6+ seconds minimum.

2. **Finality Types**:
   - **Builder RPC (soft finality)**: Blocks available immediately after Phase 1 execution + builder consensus
   - **DA/Validator RPC (hard finality)**: Blocks cryptographically guaranteed on-chain, but delayed

3. **Use Cases**:
   | Use Case | Source | Reason |
   |----------|--------|--------|
   | Wallet balance check | Builder | Low latency required |
   | dApp state reads | Builder | Real-time UX |
   | Transaction submission | Builder | TxPool lives on builder |
   | Historical audit | DA | Canonical state matters |
   | Cross-chain bridge verification | DA | Hard finality required |

### Builder Block Cache

Builders maintain a local cache of `EvmBlockPayload` for RPC serving:

```go
type BuilderBlockCache struct {
    // Index by block number (most common query)
    blocksByNumber map[uint64]*EvmBlockPayload

    // Index by block hash (for eth_getBlockByHash)
    blocksByHash map[common.Hash]*EvmBlockPayload

    // Track latest block for "latest" queries
    latestBlock uint64

    // Retention: keep N blocks before pruning
    // (older blocks should be queryable from DA)
    maxBlocks int
}
```

### EvmBlockPayload for RPC

Each block stores enough data for common RPC queries:

```go
type EvmBlockPayload struct {
    Number           uint32
    Timestamp        uint32
    GasUsed          uint64
    UBTRoot          common.Hash           // State root
    TransactionsRoot common.Hash
    ReceiptRoot      common.Hash
    TxHashes         []common.Hash         // For eth_getBlockByNumber with txns
    Transactions     []TransactionReceipt  // Full receipt data for each tx
}
```

### Receipt Extraction from Refine Output

Receipts are extracted locally from refine output during Phase 1 execution, enabling `eth_getTransactionReceipt` without DA lookup.

**Data Flow:**

```
Rust EVM Service (refine)
    │
    ├─► Receipts written to segments via WriteIntent
    │
    └─► Refine output contains MetaShard entries
              │
              ▼
        MetaShard payload (in segments)
              │
              ├─► [ld:1][prefix56:7][merkle_root:32][count:2]
              │
              └─► ObjectRefEntry[] (69 bytes each)
                      │
                      ├─► object_id: 32 bytes (tx hash for receipts)
                      │
                      └─► ObjectRef: 37 bytes
                              ├─► work_package_hash: 32 bytes
                              └─► packed: 5 bytes (index_start, index_end, last_seg_size, kind)
```

**Extraction Algorithm (`ExtractReceiptsFromRefineOutput`):**
1. Parse meta-shard entries from refine output (objectKind=MetaShard=0x04)
2. Read meta-shard payload from segments
3. Parse ObjectRefEntry mappings within meta-shard
4. Filter for Receipt entries (objectKind=0x03)
5. Read receipt payloads from segments using ObjectRef index/length
6. Sort receipts by TransactionIndex (meta-shards sort by object_id/hash)

**ObjectRef Packed Format (5 bytes, big-endian):**

```
Bits 28-39: index_start (12 bits) - starting segment index
Bits 16-27: index_end   (12 bits) - ending segment index
Bits  4-15: last_seg_sz (12 bits) - bytes in last segment (0 = full 4096)
Bits  0-3:  object_kind (4 bits)  - 0x03 for Receipt
```

**Receipt Payload Format:**

```
[logs_payload_len:4][logs_payload:variable]
[tx_hash:32][tx_type:1][success:1][used_gas:8]
[cumulative_gas:4][log_index_start:4][tx_index:4]
[tx_payload_len:4][tx_payload:variable]
```

**Sorting:**

- Receipts sorted by `TransactionIndex` (ascending)
- Logs within each receipt sorted by `LogIndex` (ascending)

### BlockCommitment for Builder Consensus

Builders vote on deterministic block content (not work package payload which contains witnesses):

```go
func (p *EvmBlockPayload) BlockCommitment() common.Hash {
    // Hash of 148-byte fixed header (deterministic fields only)
    // Excludes TxHashes array (variable length)
    data := make([]byte, 148)
    // ... pack PayloadLength, NumTransactions, Timestamp, GasUsed,
    //     UBTRoot, TransactionsRoot, ReceiptRoot, BlockAccessListHash
    return common.Blake2Hash(data)
}
```

---

## Known Issues

### 1. Transaction Receipt Lookup (objectID Overwrite) - RESOLVED

**Problem**: When `global_depth=0` (current default), all receipts route to the same meta-shard `object_id = 0x00...00`. Each bundle's accumulate overwrites the previous bundle's ObjectRef.

```
Bundle A receipts: tx1, tx2, tx3 → meta-shard 0x00...00 → ObjectRef points to DA segment [0..N]
Bundle B receipts: tx4, tx5, tx6 → meta-shard 0x00...00 → ObjectRef OVERWRITES Bundle A's pointer!
```

**Impact**: `eth_getTransactionReceipt` via DA fails for transactions in older bundles.

**Solution (Implemented)**: Builder extracts receipts locally from refine output and stores them in `EvmBlockPayload.Transactions`:

1. `ExtractReceiptsFromRefineOutput()` parses meta-shard entries from refine output
2. Reads meta-shard payloads from segments to find Receipt ObjectRefEntries
3. Extracts receipt payloads and sorts by TransactionIndex
4. Receipts stored in `EvmBlockPayload.Transactions[]` for local lookup
5. `GetTransactionReceipt()` uses local `txHash → (blockNumber, txIndex)` index

This bypasses the DA overwrite issue entirely - builders serve receipts from local storage.

### 2. Intermediate State Correctness

**Status**: Addressed by root-first execution.

Each bundle is built against an explicit `PreRoot` and produces a `PostRoot` via copy-on-write. Parallel builds no longer share mutable state, so intermediate reads are isolated by root.

### 3. Invalidation and Resubmission

**Status**: Addressed by root-first execution.

Failed bundles discard only their `PostRoot`, leaving other in-flight bundles untouched. Resubmission rebuilds from the original `PreRoot` without requiring any snapshot invalidation.

---

## Open Questions (Deferred)

1. **Finality signature scheme** - BLS, multisig, or other?
2. **What JAM stores** - Just validity, or also finality proofs?
3. **Builder set management** - How are builders added/removed?
4. **Slashing conditions** - What constitutes builder fault?
5. **Fallback if builder network stalls** - User escape hatch?
6. **Receipt index persistence** - LevelDB? In-memory with checkpoint?
7. **Snapshot dependency graph** - How to track which snapshots depend on which?
