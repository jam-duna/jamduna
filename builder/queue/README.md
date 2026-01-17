# EVM Builder Queue System

This document describes the queue and state management system for the EVM builder, covering both the transaction pool level and the bundle submission queue.

## Overview

The EVM builder has two levels of queuing:

1. **Transaction Pool (TxPool)** - Holds pending EVM transactions waiting to be bundled
2. **Bundle Queue (QueueState)** - Manages work package bundles through submission lifecycle

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   TxPool    │ ──► │   Bundle    │ ──► │   Queue     │ ──► │  Network    │
│  (pending)  │     │  Building   │     │  (states)   │     │ (validators)│
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
```

## Transaction Pool (TxPool)

### Purpose
Holds EVM transactions submitted via JSON-RPC (`eth_sendRawTransaction`) until they are included in a work package bundle.

### Data Structures

```go
type TxPool struct {
    // Transaction storage by hash
    pending map[common.Hash]*TxPoolEntry  // Ready to bundle
    queued  map[common.Hash]*TxPoolEntry  // Future nonces (not yet executable)

    // Organization by sender for nonce ordering
    pendingBySender map[common.Address]map[uint64]*TxPoolEntry
    queuedBySender  map[common.Address]map[uint64]*TxPoolEntry

    config TxPoolConfig
    stats  TxPoolStats
}

type TxPoolEntry struct {
    Tx       *EthereumTransaction
    Status   TxPoolStatus  // Pending, Queued, Dropped, Included
    AddedAt  time.Time
    Attempts int
}
```

### Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `MaxPendingTxs` | 1000 | Maximum transactions in pending pool |
| `MaxQueuedTxs` | 1000 | Maximum transactions in queued pool |
| `MaxTxsPerSender` | 16 | Maximum transactions per sender address |
| `TxTTL` | 1 hour | Time to live before expiry |
| `MinGasPrice` | 1 Gwei | Minimum gas price to accept |
| `MaxTxSize` | 32 KB | Maximum transaction size |

### Operations

| Method | Description |
|--------|-------------|
| `AddTransaction(tx)` | Validate and add to pending/queued pool |
| `GetTransaction(hash)` | Retrieve transaction by hash |
| `GetPendingTransactions()` | Get all pending transactions |
| `GetPendingTransactionsLimit(n)` | Get up to N pending transactions |
| `RemoveTransaction(hash)` | Remove transaction after bundling |
| `Size()` | Total count (pending + queued) |
| `CleanupExpiredTransactions()` | Remove transactions older than TTL |

### Transaction Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        TRANSACTION LIFECYCLE                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  eth_sendRawTransaction                                                     │
│         │                                                                   │
│         ▼                                                                   │
│  ┌─────────────┐                                                            │
│  │  Validate   │ ─── size, gas price, gas limit, value                      │
│  └─────────────┘                                                            │
│         │                                                                   │
│         ├──── nonce gap? ────► Queued Pool (wait for missing nonces)        │
│         │                                                                   │
│         ▼                                                                   │
│  ┌─────────────┐                                                            │
│  │   Pending   │ ◄──── ready for bundling                                   │
│  │    Pool     │                                                            │
│  └─────────────┘                                                            │
│         │                                                                   │
│         │ GetPendingTransactionsLimit(N)                                    │
│         ▼                                                                   │
│  ┌─────────────┐                                                            │
│  │   Bundle    │ ─── BuildBundle() processes transactions                   │
│  │  Building   │                                                            │
│  └─────────────┘                                                            │
│         │                                                                   │
│         │ Enqueue with txHashes - transactions STAY in pool                 │
│         ▼                                                                   │
│  ┌─────────────┐                                                            │
│  │   Queued    │ ─── bundle in queue (assigned to core 0/1/2...)           │
│  │   Bundle    │                                                            │
│  └─────────────┘                                                            │
│         │                                                                   │
│         │ Submit to validators                                              │
│         ▼                                                                   │
│  ┌─────────────┐                                                            │
│  │ Guaranteed  │ ─── bundle included in E_G                                 │
│  │             │                                                            │
│  └─────────────┘                                                            │
│         │                                                                   │
│         │ Wait for E_A (accumulation extrinsic)                             │
│         ▼                                                                   │
│  ┌─────────────┐                                                            │
│  │ Accumulated │ ─── onAccumulated callback fires (outside lock!)           │
│  └─────────────┘                                                            │
│         │                                                                   │
│         │ RemoveTransaction(hash) - called AFTER on-chain accumulation      │
│         ▼                                                                   │
│  ┌─────────────┐                                                            │
│  │   Removed   │ ─── transaction no longer in pool                          │
│  └─────────────┘                                                            │
│                                                                             │
│  Note: If bundle fails/times out, transactions stay in pool for retry!     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### When Transactions Are Removed

Transactions are removed from TxPool **after on-chain accumulation**, not after enqueue:

```go
// Transaction hashes are tracked in QueueItem
type QueueItem struct {
    TransactionHashes []common.Hash  // Transactions to remove after accumulation
    // ... other fields
}

// Callback is set up during builder initialization
queueRunner.SetOnAccumulated(func(wpHash common.Hash, txHashes []common.Hash) {
    log.Info("Removing accumulated transactions from txpool",
        "wpHash", wpHash.Hex(),
        "txCount", len(txHashes))
    for _, txHash := range txHashes {
        txPool.RemoveTransaction(txHash)
    }
})

// Transaction hashes are collected and passed to enqueue
txHashes := make([]common.Hash, len(pendingTxs))
for i, tx := range pendingTxs {
    txHashes[i] = tx.Hash
}

// Enqueue with transaction hashes
blockNumber, err := queueRunner.EnqueueBundleWithOriginalExtrinsics(
    bundle, originalExtrinsics, originalWorkItemExtrinsics, coreIdx, txHashes)

// NO transaction removal here! Callback handles it after accumulation.
```

**Why remove after accumulation (not after enqueue)?**

1. **Preserves transactions if bundle fails**: If bundle times out or is dropped, transactions stay in pool for retry
2. **Handles bundle rebuilds correctly**: When bundles are rebuilt with new hashes, original transactions remain available
3. **Prevents premature loss**: Builder may create 10+ bundles but only some accumulate; others need their transactions preserved
4. **Thread-safe cleanup**: Callback is invoked **outside queue lock** to prevent I/O blocking and deadlocks

**Callback Threading Safety:**

The `onAccumulated` callback is invoked with the queue lock **released** to prevent blocking:

- Queue state updates complete first (mark as accumulated, move to finalized)
- Lock is explicitly released before callback invocation
- Callback can safely perform I/O (remove from transaction pool, logging) without blocking queue operations
- See [queue.go:577-668](queue.go#L577-L668) for implementation details

### Validation Rules

```go
func (pool *TxPool) validateTransaction(tx *EthereumTransaction) error {
    // 1. Check transaction size
    if tx.Size > pool.config.MaxTxSize { return error }

    // 2. Check gas price meets minimum
    if tx.GasPrice < pool.config.MinGasPrice { return error }

    // 3. Check gas limit is non-zero
    if tx.Gas == 0 { return error }

    // 4. Check value is non-negative
    if tx.Value < 0 { return error }

    return nil
}
```

### Sender-Based Organization

Transactions are indexed by sender address and nonce for:
- Efficient nonce ordering during bundling
- Per-sender transaction limits
- Gap detection (queued vs pending)

```go
// Example: Get all pending txs for an address
senderTxs := pool.pendingBySender[address]
for nonce, entry := range senderTxs {
    // Process in nonce order
}
```

### Expiry and Cleanup

Transactions that exceed TTL (default 1 hour) are automatically cleaned up:

```go
func (pool *TxPool) CleanupExpiredTransactions() {
    now := time.Now()
    for hash, entry := range pool.pending {
        if now.Sub(entry.AddedAt) > pool.config.TxTTL {
            pool.removeFromPending(entry)
            pool.stats.TotalDropped++
        }
    }
    // Same for queued pool
}
```

## Bundle Queue (QueueState)

### Purpose
Manages work package bundles through their submission lifecycle, tracking state transitions and handling timeouts/resubmissions.

### Bundle States

```
┌──────────┐    Submit    ┌───────────┐   E_G signal   ┌────────────┐   E_A signal   ┌─────────────┐
│  Queued  │ ──────────►  │ Submitted │ ─────────────► │ Guaranteed │ ─────────────► │ Accumulated │
└──────────┘              └───────────┘                └────────────┘                └─────────────┘
     ▲                          │                                                          │
     │                          │ timeout                                                  │
     │                          ▼                                                          ▼
     │                    ┌───────────┐                                              ┌───────────┐
     └────────────────────│  Requeue  │                                              │ Finalized │
                          └───────────┘                                              └───────────┘
```

| State | Description | Core Blocked? | Tracked In |
|-------|-------------|---------------|------------|
| **Queued** | Bundle built, waiting to be submitted | No | `Queued` map |
| **Submitted** | Sent to validators, awaiting guarantee | **Yes** | `Inflight` map |
| **Guaranteed** | Included in E_G, awaiting accumulation | No | `Inflight` map |
| **Accumulated** | Included in E_A, processing complete | No | `Finalized` map |
| **Finalized** | Cleanup state, can be pruned | No | `Finalized` map |

### Signals Tracked

| Signal | Source | Triggers State Change |
|--------|--------|----------------------|
| **E_G (Guarantee Extrinsic)** | Block import | Submitted → Guaranteed |
| **E_A (Accumulation Extrinsic)** | Block import | Guaranteed → Accumulated |
| **Timeout (18s)** | Timer | Submitted → Requeue |
| **Anchor Staleness** | Slot check | Triggers rebuild before submit |

### Key Insight: Core Availability

A core is **blocked** only while a bundle is in `Submitted` state (waiting for guarantee). Once guaranteed:
- The core is **immediately available** for new submissions
- The bundle stays in `Inflight` map for tracking until accumulated
- This allows maximum throughput: submit new bundle as soon as previous is guaranteed

### Inflight Calculation

```go
// Only Submitted items block core slots
func (qs *QueueState) inflight() int {
    count := 0
    for _, status := range qs.Status {
        if status == StatusSubmitted {
            count++
        }
    }
    return count
}
```

### Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `MaxInflight` | 3 | Max bundles in Submitted state (= number of cores) |
| `SubmitRetryInterval` | 6s | Fast retry: resubmit same version every N seconds |
| `MaxSubmitRetries` | 8 | Fast retry: max submission attempts (1 initial + 7 retries) |
| `GuaranteeTimeout` | 54s | Slow timeout: increment version after refine expiry |
| `MaxVersionRetries` | 5 | Max version increments before dropping block |

#### Two-Tier Timeout Strategy

The queue uses a **two-tier timeout strategy** to balance fast recovery from network failures with safety against duplicate execution:

**1. Fast Retry (Same Version)** - Network Failure Recovery:

- **Window**: `RecentHistorySize` × `SecondsPerSlot` = 8 × 6s = **48 seconds**
- **Interval**: Retry every **6 seconds** (1 JAM block)
- **Max Attempts**: **8 total** (1 initial + 7 retries)
- **Safety**: Same hash = idempotent (no duplicate execution risk)
- **Use Case**: Recover from transient QUIC failures, validator unavailability

```
T=0s:   Submit v1 (attempt 1) → QUIC timeout
T=6s:   Fast retry v1 (attempt 2) → network failure
T=12s:  Fast retry v1 (attempt 3) → success, guaranteed ✓
```

**2. Slow Timeout (New Version)** - Refine Expiry:

- **Timeout**: **54 seconds** = rotation period maximum (42s) + safety margin (12s)
- **Action**: Increment version, rebuild bundle with new refine context
- **Safety**: v1 expired on-chain before v2 submitted, prevents duplicate guarantees
- **Use Case**: Bundle genuinely failed or censored, needs new prerequisites

**CRITICAL**: `GuaranteeTimeout` (54s) is **not arbitrary** - it's derived from the JAM protocol's **strictest** validation constraint:

**Three validation checks exist for guarantee age** (all in `VerifyGuarantee`):

1. `checkAssignment` - Rotation period check: `diff <= (currentSlot % 4) + 4` slots
   - Varies by rotation position: **4-7 slots** (24-42 seconds)
   - Fails with `ErrGReportEpochBeforeLast`
2. `checkRecentBlock` - RecentBlocks check: anchor/state/beefy in last 8 blocks
   - Fixed window: **8 slots** (48 seconds)
   - Fails with `ErrGAnchorNotRecent`
3. `checkTimeSlotHeader` - LookupAnchor check: age < 24 slots
   - Fixed window: **24 slots** (144 seconds)
   - Fails with `ErrGReportEpochBeforeLast`

**The binding constraint is `checkAssignment` (runs first, strictest minimum):**

- `checkAssignment()` validates guarantee age based on validator rotation period
- Formula: `currentSlot - g.Slot <= (currentSlot % RotationPeriod) + RotationPeriod`
- With `RotationPeriod = 4`, validity ranges from **4 to 7 slots**
- **Minimum validity**: 4 slots = 24 seconds (when `currentSlot % 4 = 0`)
- **Maximum validity**: 7 slots = 42 seconds (when `currentSlot % 4 = 3`)

**Configuration (uses worst-case maximum for safety):**

- **Maximum rotation window**: 7 × 6 = 42 seconds (latest valid case)
- **Safety margin**: 2 slots = 12 seconds (for network delays)
- **Total timeout** = 42s + 12s = **54 seconds** (9 JAM blocks)

This ensures that when v1 times out and v2 is submitted, v1's guarantee slot is **too old for checkAssignment**, preventing validators from guaranteeing both versions. Without this, duplicate execution is possible even with local `WinningVer` gating.

**Reference**: See `statedb/guarantees.go:183-188` (`checkAssignment`), `statedb/guarantees.go:388-426` (`checkRecentBlock`), and `statedb/guarantees.go:429-441` (`checkTimeSlotHeader`).

## Submission Window Timing

Bundles are submitted during a specific window before each timeslot:

```
         Timeslot N                    Timeslot N+1
    ◄─────────────────────────────►◄─────────────────────────────►
    │                              │                              │
    │     [====Submission====]     │     [====Submission====]     │
    │     Window: 3-1s before      │     Window: 3-1s before      │
    │                              │                              │
```

- **Window Start**: 3 seconds before next timeslot
- **Window End**: 1 second before next timeslot
- Submissions outside window are held until next window

## Anchor Management

Work packages reference a recent block as their "anchor". The anchor must exist in `RecentBlocks` (last 8 blocks = 48 seconds).

### Dynamic Anchor Offset

```go
// Calculate offset based on queue position
// Later bundles get fresher anchors to avoid staleness
anchorOffset := types.RecentHistorySize - 2 - bundlePairsAhead
if anchorOffset < 1 {
    anchorOffset = 1
}
```

| Queue Position | Anchor Offset | Anchor Age |
|----------------|---------------|------------|
| 0-1 bundles | 6 | ~36s old |
| 2-3 bundles | 5 | ~30s old |
| 4-5 bundles | 4 | ~24s old |
| 6+ bundles | 1 | ~6s old (freshest) |

### Staleness Check

Before submission, bundles are checked for anchor staleness:

```go
// Rebuild if anchor age > 6 blocks (leaving 2 blocks headroom)
const recentHistorySize uint32 = 8
const anchorSafetyMargin uint32 = 2
if anchorAge > (recentHistorySize - anchorSafetyMargin) {
    needsRebuild = true
}
```

## Bundle Rebuild on Requeue

When a bundle times out or has a stale anchor, it must be rebuilt:

1. **Restore original extrinsics** (remove UBT witnesses)
2. **Restore original payload type** (Builder, not Transactions)
3. **Get fresh RefineContext** with new anchor
4. **Rebuild via StateDB.BuildBundle** (re-runs refine, prepends new witnesses)
5. **Resubmit** with incremented version number

### Deep Copy Requirement

Original extrinsics must be deep-copied before rebuild because `BuildBundle` modifies the slice in place:

```go
extrinsicsCopy := make([]types.ExtrinsicsBlobs, len(item.OriginalExtrinsics))
for i, blobs := range item.OriginalExtrinsics {
    extrinsicsCopy[i] = make(types.ExtrinsicsBlobs, len(blobs))
    for j, blob := range blobs {
        extrinsicsCopy[i][j] = make([]byte, len(blob))
        copy(extrinsicsCopy[i][j], blob)
    }
}
```

## Complete Flow Example

```
1. TxPool receives 51 transactions via RPC

2. Builder loop detects pending txns, builds bundles:
   - Bundle B-1: txns 1-5, core 0, anchorOffset=6
   - Bundle B-2: txns 6-10, core 1, anchorOffset=6
   → Both added to Queued map

3. Submission window opens:
   - B-1: Queued → Submitted (sent to validators)
   - B-2: Queued → Submitted
   - inflight() = 2, CanSubmit() = false

4. Block N arrives with E_G containing B-1, B-2:
   - B-1: Submitted → Guaranteed
   - B-2: Submitted → Guaranteed
   - inflight() = 0, CanSubmit() = true
   - Cores immediately available!

5. Next bundles submitted:
   - B-3: Queued → Submitted
   - B-4: Queued → Submitted

6. Block N+1 arrives with E_A containing B-1, B-2:
   - B-1: Guaranteed → Accumulated → Finalized
   - B-2: Guaranteed → Accumulated → Finalized
   - Moved from Inflight to Finalized map

7. Continue until all bundles processed...
```

## Bundle Resubmission (Requeue)

When a bundle fails to get guaranteed within the timeout period, it is requeued for resubmission.

### Requeue Triggers

| Trigger | Condition | Action |
|---------|-----------|--------|
| **Guarantee Timeout** | 18 seconds in `Submitted` state without E_G | Requeue with rebuild |
| **Stale Anchor** | Anchor age > 6 blocks at submission time | Rebuild before submit |
| **Network Failure** | QUIC stream error during submission | Requeue for retry |

### Requeue Flow

```
┌───────────┐
│ Submitted │ ─── timeout (18s) ───┐
└───────────┘                      │
                                   ▼
                           ┌──────────────┐
                           │ CheckTimeout │
                           └──────────────┘
                                   │
                    ┌──────────────┼──────────────┐
                    │              │              │
                    ▼              ▼              ▼
            already guaranteed?   version < Max   version >= Max
                    │              │              │
                    ▼              ▼              ▼
            ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
            │   Skip       │ │   Requeue    │ │    Drop      │
            │ (no action)  │ │ version++    │ │  (give up)   │
            └──────────────┘ └──────────────┘ └──────────────┘
                                   │
                                   ▼
                           ┌──────────────┐
                           │   Queued     │ ─── wait for submission window
                           └──────────────┘
                                   │
                                   ▼
                           ┌──────────────┐
                           │   Rebuild    │ ─── fresh RefineContext + new anchor
                           └──────────────┘
                                   │
                                   ▼
                           ┌──────────────┐
                           │  Submitted   │
                           └──────────────┘
```

### Duplicate Execution Prevention: "First Guarantee Wins"

**Critical Policy:** The queue implements a **"first guarantee wins"** policy to prevent duplicate execution. Once ANY version of a bundle receives a guarantee, that version is the winner and all other versions are rejected.

#### Race Condition Scenario

1. Bundle v1 is submitted at 12:00:00
2. Timeout fires at 12:00:18, v1 is requeued as v2
3. v2 is rebuilt and submitted at 12:00:20
4. v1's guarantee arrives at 12:00:21 (delayed network)
5. v2's guarantee arrives at 12:00:22
6. **Without "first wins" policy:** Both v1 and v2 could accumulate → duplicate transaction execution!

#### Solution: WinningVer Tracking

The queue maintains a `WinningVer` map that tracks which version was first guaranteed:

```go
// In QueueState struct
WinningVer map[uint64]int // Immutable once set - first guaranteed version wins

// In OnGuaranteed
if winningVer, exists := qs.WinningVer[bn]; exists {
    if ver != winningVer {
        // REJECT - not the winning version
        log.Warn("Rejecting guarantee for non-winning version")
        return
    }
} else {
    // First guarantee - this version wins!
    qs.WinningVer[bn] = ver
    qs.cancelOtherVersions(bn, ver) // Cancel queued v2, invalidate its hash
}
```

#### Enforcement at Multiple Stages

1. **OnGuaranteed:** First guarantee sets `WinningVer[bn]`, all subsequent guarantees for different versions are rejected
2. **OnAccumulated:** Only the winning version can accumulate; non-winning versions are rejected
3. **OnFinalized:** Only the winning version can finalize

#### Example Timeline

```text
12:00:00 - v1 submitted
12:00:18 - timeout, v1 → v2 (requeued)
12:00:20 - v2 submitted
12:00:21 - v1 guarantee arrives → WinningVer[bn]=1, v2 canceled
12:00:22 - v2 guarantee arrives → REJECTED (not the winner)
12:00:27 - v1 accumulates → SUCCESS
12:00:28 - v2 accumulate attempt → REJECTED (not the winner)
```

This guarantees **only one version of each block can reach Accumulated state**, preventing duplicate transaction execution.

#### Critical Limitations: Local Gating vs On-Chain Behavior

**IMPORTANT**: The `WinningVer` mechanism provides **local consistency** but **cannot prevent on-chain duplicates** if resubmission happens before refine expiry:

**The Problem**:

1. v1 submitted at T=0, times out at T=18s (hypothetical old timeout)
2. v2 submitted at T=20s (v1's guarantee slot still valid via `checkAssignment` until T=24-42s)
3. v1's guarantee arrives at T=25s → `WinningVer[bn]=1` locally
4. **But validators already have v2** and may guarantee it too
5. Result: v1 wins locally, but **both v1 and v2 could be guaranteed on-chain**

**The Solution** (implemented):

* `GuaranteeTimeout` is set to **54 seconds** (rotation period maximum 42s + 12s safety margin)
* This ensures v1's guarantee slot is **too old for `checkAssignment()`** before v2 is submitted
* Validators **reject v1 with `ErrGReportEpochBeforeLast`** after expiry, eliminating duplicate risk

**Why This Matters**:

* Local `WinningVer` gating rejects v2 events locally
* But it cannot stop validators from processing v2 that was already submitted
* Only by waiting for the **rotation period expiry** (strictest constraint) can we ensure single-version execution

**Which Error Condition Determines the Window**:

The binding constraint is **`checkAssignment()` rotation period check** (`statedb/guarantees.go:183-188`):

* Returns `ErrGReportEpochBeforeLast` when: `currentSlot - g.Slot > (currentSlot % 4) + 4`
* Minimum validity: **4 slots = 24 seconds** (strictest case)
* This check **runs first** in the validation pipeline, before `checkRecentBlock` (48s) or `checkTimeSlotHeader` (144s)
* Once a guarantee exceeds this rotation period age, validators **immediately reject** it

**Trade-off**:

* **Safety**: 54s timeout eliminates duplicate execution risk (uses protocol's strictest constraint)
* **Latency**: Failed bundles retry in 54s (much faster than previous 156s)
* **Throughput**: Improved recovery time while maintaining correct behavior

## Protocol-Level Duplicate Prevention

### Builder Cannot Control On-Chain Behavior

**Key Limitation**: The builder **cannot prevent validators from accepting multiple guarantees** for different work package hashes targeting the same rollup block. The JAM protocol does not enforce rollup-level uniqueness at the consensus layer - validators may guarantee any work package that passes validation, regardless of whether other versions exist.

### What the Builder Controls

The builder can only control its **local submission behavior**:

1. ✅ Minimize duplicate creation via proper timeout configuration (54s)
2. ✅ Cancel non-winning versions locally when a winner is detected
3. ✅ Track and reject duplicate events in local queue state
4. ✅ Observe and log when duplicates occur for operator awareness

### What the Builder Cannot Control

The builder **cannot prevent**:

1. ❌ Validators from guaranteeing multiple versions if both are submitted within validity window
2. ❌ Network partitions causing split guarantees across validator subsets
3. ❌ Other builders (in decentralized setup) submitting different versions
4. ❌ Race conditions between builder instances if not properly coordinated

### Operational Recommendations

**For Single-Builder Deployments** (current setup):

* Monitor `DuplicateGuaranteeRejected` and `DuplicateAccumulateRejected` metrics
* If these counters increase, investigate:
  - Network delays causing slow guarantee arrival
  - Timeout configuration (ensure 54s minimum)
  - QUIC connectivity to validators
  - System clock synchronization

**For Multi-Builder Deployments** (future work):

* Will require builder coordination/consensus protocol
* Builders must agree on canonical version before submission
* Consider "primary builder" election to serialize submissions
* Note: Multi-builder setup is not currently implemented

**Downstream System Design**:

* Design all downstream consumers to handle duplicate guarantees gracefully
* Use "first guarantee wins" semantics consistently
* Index by work package hash or rollup block number and reject duplicates
* Log duplicate detection for forensic analysis
* Note: `bundleID` is a builder-local string (e.g., "B-7"), not protocol-visible

### Observability

The queue exposes metrics to detect when duplicates occur:

```go
stats := queueState.GetStats()
log.Info("Queue Metrics",
    "duplicateGuaranteesRejected", stats.DuplicateGuaranteeRejected,
    "duplicateAccumulatesRejected", stats.DuplicateAccumulateRejected,
    "nonWinningVersionsCanceled", stats.NonWinningVersionsCanceled)
```

**What these metrics mean**:

* `DuplicateGuaranteeRejected`: A guarantee arrived for a non-winning version (indicates network delays or timeout issues)
* `DuplicateAccumulateRejected`: An accumulation arrived for a non-winning version (should be rare, investigate if frequent)
* `NonWinningVersionsCanceled`: Non-winning versions were removed from Queued/Inflight (cleanup working correctly)

**Expected behavior**: The 54s timeout is specifically designed to prevent duplicates by ensuring old versions expire before new versions are submitted. Non-zero counters indicate duplicates were observed.

**If counters increase**: Investigate network delays, timeout configuration (ensure 54s minimum), QUIC connectivity to validators, or system clock synchronization.

### Timeout Detection

The runner periodically checks for timed-out bundles:

```go
func (qs *QueueState) CheckTimeouts() []*QueueItem {
    var timedOut []*QueueItem
    now := time.Now()

    for bn, item := range qs.Inflight {
        if item.Status == StatusSubmitted {
            elapsed := now.Sub(item.SubmittedAt)
            if elapsed > qs.config.GuaranteeTimeout {
                timedOut = append(timedOut, item)
            }
        }
    }
    return timedOut
}
```

### Requeue Operation

```go
func (qs *QueueState) Requeue(blockNumber uint64) {
    item := qs.Inflight[blockNumber]

    // Increment version for tracking
    item.Version++
    item.Status = StatusQueued

    // Move from Inflight back to Queued
    delete(qs.Inflight, blockNumber)
    qs.Queued[blockNumber] = item
}
```

### Version Tracking

Each bundle has a `Version` field incremented on each resubmission:

| Version | Meaning |
|---------|---------|
| 1 | Original submission |
| 2 | First resubmission |
| 3 | Second resubmission |
| >MaxRetries | Dropped (too many failures) |

The version is used to:
1. Track retry count
2. Determine if rebuild is needed (version > 1 always rebuilds)
3. Log/debug submission history

### Why Rebuild is Required

When requeuing, the bundle **must be rebuilt** because:

1. **Anchor Expiry**: The original anchor may have expired (>8 blocks old)
2. **State Changes**: UBT state may have changed if other bundles were accumulated
3. **Fresh Context**: New `RefineContext` ensures valid lookup anchor slot

### Rebuild Process

```go
bundleBuilder := func(item *QueueItem, stats QueueStats) (*WorkPackageBundle, error) {
    // 1. Calculate fresh anchor offset
    anchorOffset := types.RecentHistorySize - 2 - (stats.QueuedCount / types.TotalCores)

    // 2. Get new RefineContext
    refineCtx, _ := n.GetRefineContextWithBuffer(anchorOffset)
    item.Bundle.WorkPackage.RefineContext = refineCtx

    // 3. Restore original extrinsics (deep copy)
    extrinsicsCopy := deepCopy(item.OriginalExtrinsics)

    // 4. Restore original WorkItem metadata
    for i := range item.Bundle.WorkPackage.WorkItems {
        item.Bundle.WorkPackage.WorkItems[i].Extrinsics = item.OriginalWorkItemExtrinsics[i]
        item.Bundle.WorkPackage.WorkItems[i].Payload = BuildPayload(PayloadTypeBuilder, ...)
    }

    // 5. Rebuild bundle (re-runs refine, generates new UBT witnesses)
    bundle, _, _ := n.BuildBundle(item.Bundle.WorkPackage, extrinsicsCopy, item.CoreIndex, nil)

    return bundle, nil
}
```

## UBT State and Parallel Bundles

The builder supports **multi-snapshot UBT** for parallel bundle building, allowing multiple bundles to be built concurrently with correct state chaining.

### Multi-Snapshot Architecture

```
                     canonicalUBT (S0)
                           │
             ┌─────────────┴─────────────┐
             │                           │
         snapshot[1]                 snapshot[2]
          (S0→S1)                     (S1→S2)
         Bundle A                    Bundle B
             │                           │
             ▼                           ▼
     OnAccumulated(1)            OnAccumulated(2)
    canonicalUBT=S1              canonicalUBT=S2
```

Each bundle gets its own **UBT snapshot** that chains from the previous bundle's post-state (or canonical state if no prior pending snapshot exists). This eliminates the "CurrentUBT drift" problem.

### Snapshot Lifecycle

| Phase | Method | Description |
|-------|--------|-------------|
| **Create** | `CreateSnapshotForBlock(bn)` | Clone from parent snapshot or canonical (idempotent) |
| **Activate** | `SetActiveSnapshot(bn)` | Route reads to this snapshot |
| **Write** | `ApplyWritesToActiveSnapshot(blob)` | Apply contract writes to snapshot |
| **Commit** | `CommitSnapshot(bn)` | On accumulation: merge to canonical |
| **Invalidate** | `InvalidateSnapshotsFrom(bn)` | On failure: discard snapshot + descendants |
| **Invalidate Single** | `InvalidateSnapshot(bn)` | Discard only one snapshot (not descendants) |

**Idempotent Snapshot Creation:** `CreateSnapshotForBlock` is idempotent - if a snapshot already exists for the given block number, it simply reuses the existing snapshot instead of erroring. This eliminates the need to delete snapshots before rebuilds, which was problematic because `InvalidateSnapshotsFrom` would delete ALL snapshots >= blockNumber, destroying valid snapshots for other in-flight bundles.

### How Snapshots Chain

When building Bundle B (block 2) while Bundle A (block 1) is still pending:

1. `CreateSnapshotForBlock(2)` checks if `pendingSnapshots[1]` exists
2. If yes: clone from `pendingSnapshots[1]` (Bundle A's post-state)
3. If no: clone from `CurrentUBT` (canonical state)

This ensures each bundle reads from its correct pre-state, even when built in parallel.

### Callback Setup (Recommended)

```go
// Use snapshot-aware callbacks for proper state management
runner.SetOnAccumulatedWithSnapshots(storage, func(wpHash common.Hash, txHashes []common.Hash) {
    // Called AFTER snapshot committed to canonical
    for _, txHash := range txHashes {
        txPool.RemoveTransaction(txHash)
    }
})

runner.SetOnFailedWithSnapshots(storage, func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash) {
    // Called AFTER snapshot + descendants invalidated
    for _, txHash := range txHashes {
        txPool.UnlockTransaction(txHash)
    }
})
```

### Failure Handling

When a bundle fails, snapshots are invalidated based on the failure type:

**Build/Enqueue Failure (use `InvalidateSnapshotsFrom`):**
When a bundle fails to build or enqueue, all pending snapshots from that block onwards are invalidated because later bundles may have been built on stale state:

```
Bundle A (blk#1): build fails
Bundle B (blk#2): pending    ← invalidated (may depend on A)
Bundle C (blk#3): pending    ← invalidated (may depend on B)
```

The `InvalidateSnapshotsFrom(1)` call removes snapshots for blocks 1, 2, and 3.

**Rebuild (invalidate + recreate):**
When a bundle needs to be rebuilt (e.g., stale anchor timeout), the existing snapshot must be invalidated and recreated from the parent state:

1. `InvalidateSnapshot(blockNumber)` - Delete the polluted snapshot (contains v1's writes)
2. `CreateSnapshotForBlock(blockNumber)` - Create fresh snapshot from parent (N-1 or CurrentUBT)
3. Build bundle with clean state

This prevents "snapshot pollution" where v2 would stack writes on top of v1's writes, causing balance double-counting.

```go
// In runner.go, before calling bundleBuilder:
if needsRebuild && r.snapshotManager != nil {
    r.snapshotManager.InvalidateSnapshot(item.BlockNumber)
}
```

### Concurrency Limitation: Global Active Snapshot

**Important**: The `activeSnapshotBlock` is a process-global variable. This means:

1. **Serialized builds required**: Only one bundle can be built at a time. If two builds overlap, `SetActiveSnapshot` calls will interfere with each other.
2. **Current design**: The EVM builder builds bundles sequentially in a single goroutine (`handleBlockNotifications` loop), so this is safe.
3. **Future work**: For true concurrent builds, each build would need its own snapshot context (e.g., pass snapshot reference through BuildBundle, or use goroutine-local storage).

```text
Thread 1: SetActiveSnapshot(1) → BuildBundle → ClearActiveSnapshot
Thread 2:          SetActiveSnapshot(2) → ...  ← RACE! Thread 1 now reads snapshot 2
```

The current implementation is correct for single-threaded sequential building, which is the expected use case for the EVM builder.

### Snapshot Retention Policy

**Current policy: Never delete snapshots.**

Snapshots are kept in memory indefinitely. This is intentional:

1. **Out-of-order accumulation**: Blocks can accumulate in any order (e.g., block 12 before block 9). If we delete snapshots on commit, rebuilding earlier blocks becomes impossible because the parent state is gone.

2. **No persistence layer yet**: Without a way to persist and recover snapshots from disk/database, deletion is permanent data loss. Deleting "for optimization" before having persistence just causes bugs.

3. **Memory vs correctness tradeoff**: Yes, this uses more memory. But incorrect balances are worse than high memory usage.

**Future optimization**: Once snapshot persistence is implemented:

- Snapshots can be written to disk after commit
- In-memory copies can be evicted using LRU
- Rebuilds can recover parent state from disk on demand

```text
Current:  snapshot created → kept forever (in memory)
Future:   snapshot created → committed → persisted to disk → evicted from memory
          rebuild needed → load from disk → use → evict again
```

### Legacy Mode (Deferred Writes Only)

For backwards compatibility, the system can operate with deferred writes instead of snapshots:

```go
// Legacy deferred writes approach (not recommended for parallel building)
runner.SetOnAccumulatedWithStorage(storage, userCallback)
runner.SetOnFailedWithStorage(storage, userCallback)
```

This mode defers `ApplyContractWrites` until accumulation, but **does not support parallel bundle building** because all bundles would read from the same `CurrentUBT` state.

### Balance Reads and "latest" Semantics

When querying balances with `"latest"` or `"pending"`, the builder should read from `CurrentUBT`, which is updated by `CommitSnapshot` on accumulation. The indexed block trees (`serviceUBTRoots`, `latestRollupBlock`) are only updated by `FinalizeEVMBlock`/`StoreServiceBlock`, which are not part of the snapshot-based flow.

**Implication:** For `"latest"`/`"pending"`, `GetUBTNodeForServiceBlock` must prefer `CurrentUBT` even if indexed blocks exist, otherwise balance lookups can return stale genesis state.

### Why Multi-Snapshot is Needed (Historical Context)

With a single `CurrentUBT`, parallel bundle building causes state drift:

```text
1. Build Bundle A: reads S0, produces (pre=S0, post=S1)
2. Build Bundle B: reads S0 (same!), produces (pre=S0, post=S1')  ← WRONG
   Should read S1 (A's post-state), not S0
```

If Bundle A accumulates first, Bundle B has incorrect pre-state. Multi-snapshot solves this by giving each bundle its own snapshot that chains from the previous bundle's post-state.

## Receipt Indexing and Meta-Shard Overwrites

Receipt lookups by `txHash` require a **global index** (meta-shards). With parallel bundles and no merge step:

- Each bundle refines independently and emits its own meta-shard payload.
- Accumulate writes a **single ObjectRef** for that meta-shard key.
- The last accumulated bundle **overwrites** earlier meta-shard refs.

This means receipts from earlier bundles can disappear from the meta-shard index, even though their DA payloads still exist.

**Root cause in current implementation:** meta-shard merging is effectively disabled for transaction bundles because `ObjectRef::fetch` returns `None` when `payload_type == 1` (Transactions). Each refine run therefore starts from an empty shard and emits a brand-new meta-shard payload, which then overwrites the JAM State pointer on accumulate.

**Implications:**
- `txHash → receipt` lookups are unreliable without a global merge/indexing step.
- If parallelism is required, prefer **block-scoped lookup** (block hash + tx index) or a **builder-side index** built from block payloads.
- Without merging, the RPC path for `eth_getTransactionByHash`/`eth_getTransactionReceipt` must use an alternate index (block-scoped or builder-local), because the meta-shard index is not stable across bundles.

### Receipt Lookup: Fast Path vs Slow Path

The system provides two paths for `eth_getTransactionReceipt` lookups:

#### Fast Path: Builder Block Cache (Builder Mode)

When running as a builder, receipts are served from the **in-memory block cache** via O(1) hash lookup:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      FAST PATH RECEIPT LOOKUP                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. User queries eth_getTransactionReceipt(txHash)                      │
│                                                                         │
│  2. Check builder's blockCache via GetByTxHash(txHash)                  │
│     byTxHash[txHash] → TxLocation{BlockNumber, TxIndex, BlockCommitment}   │
│                                                                         │
│  3. Use TxLocation to retrieve block and receipt:                       │
│     byNumber[blockNumber] → EvmBlockPayload                             │
│     block.Transactions[txIndex] → receipt                               │
│     └── If found → return receipt immediately (O(1))                    │
│     └── If not found → fall through to slow path                        │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

**Properties:**
- O(1) lookup via `byTxHash` map index
- Only available in builder mode (blockCache is nil in validator mode)
- Cache populated when bundles are built via `rollup.AddBlock(evmBlock)`
- No DA fetches required - purely in-memory

**Code Reference:** [builder/evm/rpc/rollup.go](../evm/rpc/rollup.go) - `getTransactionReceipt()` calls `blockCache.GetByTxHash()`

#### Slow Path: Block-Scoped Receipt Lookup via BlockCommitment

**Solution implemented:** Instead of per-transaction meta-shard indexing, receipts are stored as a **block-scoped blob** keyed by `BlockCommitment`. This eliminates the overwrite problem because each block has a unique, stable identifier.

**BlockCommitment Properties:**
- Blake2b hash of the 148-byte fixed block header
- Computed from deterministic fields: `PayloadLength`, `NumTransactions`, `Timestamp`, `GasUsed`, `UBTRoot`, `TransactionsRoot`, `ReceiptRoot`, `BlockAccessListHash`
- **Stable across bundle rebuilds** - unlike `WorkPackageHash` which changes based on `RefineContext` (anchor, prerequisites)
- No circular dependency: receipt content → receipt_hash → ReceiptRoot → BlockCommitment → storage ObjectID

**ObjectID Format:**
```
[0xFE][BlockCommitment bytes 1-31]  // First byte replaced with 0xFE prefix
```

This avoids collision with:
- Block metadata objects (`0xFF` prefix)
- Meta-shard objects (txHash-based)
- Other object types

**Refine Output Extraction:**

The refine output contains a Section 3 that points to the block receipts blob. When extracting the receipts pointer, the code filters by **both** `ObjectKind::Receipt` (0x03) **and** the `0xFE` prefix byte:

```rust
// services/evm/src/writes.rs:121-126
let receipt_intent = effects.write_intents
    .iter()
    .find(|intent| {
        intent.effect.ref_info.object_kind == ObjectKind::Receipt as u8
            && intent.effect.object_id[0] == 0xFE
    });
```

This dual filter ensures that:
1. Only receipt objects are considered (not storage, code, or meta-shards)
2. Only **block-scoped** receipts (with 0xFE prefix) are selected, not potential future per-tx receipt objects

Without the 0xFE prefix check, Section 3 could incorrectly grab a per-tx receipt object if that feature is reintroduced in the future.

**Slow Path Flow:**

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    SLOW PATH RECEIPT LOOKUP                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. User queries eth_getTransactionReceipt(txHash)                      │
│                                                                         │
│  2. PRIMARY: Check builder's local block cache (builder mode only)      │
│     └── If found → return receipt immediately                           │
│     └── Note: blockCache is only populated when running as builder.     │
│              In validator mode, blockCache is nil and this step is      │
│              skipped entirely, falling through to slow path.            │
│                                                                         │
│  3. SLOW PATH FALLBACK (validator mode / untrusted builder):            │
│                                                                         │
│     a) Get latest block number from DA                                  │
│                                                                         │
│     b) Scan blocks backwards (most recent first):                       │
│        for blockNum := latest; blockNum >= 1; blockNum-- {              │
│            block := ReadBlockByNumber(blockNum)                         │
│            for i, hash := range block.TxHashes {                        │
│                if hash == txHash {                                      │
│                    foundBlock = block                                   │
│                    foundTxIndex = i                                     │
│                    break                                                │
│                }                                                        │
│            }                                                            │
│        }                                                                │
│                                                                         │
│     c) Compute BlockCommitment from found block:                           │
│        votingDigest := block.BlockCommitment()  // Blake2b(148-byte header)│
│                                                                         │
│     d) Fetch block receipts from DA:                                    │
│        objectID := BlockReceiptsToObjectID(votingDigest)  // 0xFE prefix│
│        receiptsBlob := ReadObject(serviceID, objectID)                  │
│                                                                         │
│     e) Parse receipts blob and find matching txHash:                    │
│        receipts := ParseBlockReceiptsBlob(receiptsBlob)                 │
│        for _, receipt := range receipts {                               │
│            if receipt.TransactionHash == txHash {                       │
│                return receipt                                           │
│            }                                                            │
│        }                                                                │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

**Block Receipts Blob Format:**
```
[receipt_count:4][receipt_0][receipt_1]...[receipt_n]

Each receipt:
[receipt_len:4][receipt_data:receipt_len]

Receipt data format:
[logs_payload_len:4][logs_payload:variable][tx_hash:32][tx_type:1][success:1]
[used_gas:8][cumulative_gas:4][log_index_start:4][tx_index:4]
[tx_payload_len:4][tx_payload:variable]
```

**Code References:**
- Rust serialization: `services/evm/src/receipt.rs:serialize_block_receipts()`, `block_receipts_object_id()`
- Rust block: `services/evm/src/block.rs:block_commitment()`
- Go parsing: `builder/evm/types/receipt.go:ParseBlockReceiptsBlob()`, `parseReceiptData()`
- Go ObjectID: `builder/evm/types/contracts.go:BlockReceiptsToObjectID()`
- Go slow path: `builder/evm/rpc/rollup.go:getTransactionReceiptSlowPath()`

**Performance Characteristics:**
- O(N) in number of blocks for initial scan (worst case)
- Scan is backwards from latest (most recent txs queried more often)
- Once block is found, single DA fetch for all receipts in that block
- No meta-shard collision issues regardless of bundle ordering

**Trade-offs:**
| Aspect | Slow Path (BlockCommitment) | Meta-Shard Index |
|--------|--------------------------|------------------|
| Reliability | Always works | Overwrites cause misses |
| Latency | O(blocks) scan | O(1) if index valid |
| Storage | Block-scoped blobs | Per-tx entries |
| Parallelism | Safe | Collision-prone |

**When Slow Path is Used:**
1. Builder cache miss (validator mode)
2. Untrusted builder scenario
3. Historical transaction lookup
4. Fallback when meta-shard lookup fails

### Technical Details: Meta-Shard Collision at ld=0

**How receipt object_id is computed:**

```
Rust (refiner.rs):    tx_hash = keccak256(extrinsic)
                      receipt_object_id = tx_hash  // raw 32-byte hash

Go (contracts.go):    TxToObjectID(txHash) = txHash  // direct passthrough
```

Both sides use the raw transaction hash as the receipt `object_id`.

**How meta-shard routing works:**

1. `global_depth` (ld) is read from JAM State key `"SSR"`
2. `object_id` prefix is masked to `ld` bits
3. `meta_shard_object_id = [ld][prefix][zeros]`

**The collision problem:**

When `global_depth=0` (current default):
- `prefix_bytes = (0 + 7) / 8 = 0` (no prefix)
- All receipts route to meta-shard `object_id = 0x00...00`

```
Bundle A receipts: tx1, tx2, tx3 → meta-shard 0x00...00 → ObjectRef points to DA segment [0..N]
Bundle B receipts: tx4, tx5, tx6 → meta-shard 0x00...00 → ObjectRef points to DA segment [M..P]
                                                          ↑ OVERWRITES Bundle A's ObjectRef!
```

**Evidence from logs:**

```
🔎 LOOKUP_START serviceID=0 objectID=0xa4b888de... globalDepth=0
🔎 LOOKUP_FOUND_SHARD ld=0 metaShardObjectID=0x00...00
🔍 METASHARD_MISS lookupObjectID=0xa4b888de... numEntries=5
🔍 METASHARD_ENTRY index=0 entryObjectID=0x5481ac57...  ← Different bundle's receipts!
```

The lookup finds a meta-shard, but it contains entries from a **different bundle** than the one containing the requested receipt.

**Where the overwrite happens:**

```rust
// accumulator.rs:421
object_ref.write(&object_id, self.timeslot, self.block_number);
// This writes to JAM State, replacing whatever ObjectRef was there
```

Each bundle's accumulate calls `write()` with the same `object_id` (`0x00...00`), overwriting the previous bundle's meta-shard pointer.

### Fix Options

| Option | Description | Trade-offs |
|--------|-------------|------------|
| **Per-bundle key prefix** | Include `work_package_hash` or `block_number` in meta-shard object_id | Requires lookup to know which bundle contains the tx |
| **Higher base ld** | Start at `ld=8` so different tx hashes route to different shards | More shards, but natural distribution |
| **Append-only index** | Accumulate appends to a list rather than overwriting | Requires list traversal, more complex |
| **Block-scoped lookup** | Look up by `(block_hash, tx_index)` instead of `tx_hash` | Different API, requires knowing the block |
| **Builder-side index** | Maintain local `tx_hash → (block, index)` mapping | Works for builder's own txns only |

### Builder-Local Receipt Index (Recommended for Non-Merging)

If meta-shard merging is disabled, the practical way to support `eth_getTransactionReceipt` is a **builder-local index**:

**Data model**
- Key: `txHash` (32 bytes)
- Value: `{blockHash, blockNumber, txIndex}`

**Write path (incremental)**
1. On block finalization (or guarantee, if acceptable), read the block payload `TxHashes`.
2. For each hash `h` at position `i`, store `h → (blockHash, blockNumber, i)`.

**Read path**
1. Lookup `txHash` in the local index.
2. If found, fetch receipt by `(blockHash, txIndex)` from DA / block payload.
3. If not found, return `null` (or optionally scan a recent window).

**Notes**
- This avoids a global merge/indexing step.
- Works for a single builder or any deployment that shares the same index store.
- If reorgs are possible, only index finalized blocks.

### Current Workaround

Until a proper fix is implemented, receipt lookups may fail for transactions in bundles that accumulated before the most recent bundle. The DA payloads still exist and are valid - only the meta-shard index pointer is lost.

**Reliable alternatives:**
1. Query by block number + transaction index (if known)
2. Use builder's local transaction tracking (for transactions submitted to this builder)
3. Wait for all pending bundles to accumulate, then query (only last bundle's receipts work)

## Bundle Dequeuing

### Dequeue Operation

Bundles are dequeued from `Queued` map for submission:

```go
func (qs *QueueState) Dequeue() *QueueItem {
    // Check if we can submit more
    if qs.inflight() >= qs.config.MaxInflight {
        return nil
    }

    // Find lowest block number (FIFO by block number)
    var lowestBN uint64 = math.MaxUint64
    for bn := range qs.Queued {
        if bn < lowestBN {
            lowestBN = bn
        }
    }

    if lowestBN == math.MaxUint64 {
        return nil // Queue empty
    }

    // Remove from Queued, will be added to Inflight after submission
    item := qs.Queued[lowestBN]
    delete(qs.Queued, lowestBN)

    return item
}
```

### Dequeue Priority

Bundles are dequeued by **block number** (lowest first), ensuring:
- Sequential EVM block ordering
- Earlier transactions processed first
- Deterministic ordering

### Dequeue Conditions

A bundle can be dequeued when:

| Condition | Check |
|-----------|-------|
| Submission window open | `inSubmissionWindow()` |
| Core available | `inflight() < MaxInflight` |
| Queue not empty | `len(Queued) > 0` |

```go
func (qs *QueueState) CanSubmit() bool {
    return qs.inflight() < qs.config.MaxInflight && len(qs.Queued) > 0
}
```

### Post-Dequeue: Mark Submitted

After successful network submission:

```go
func (qs *QueueState) MarkSubmitted(blockNumber uint64, coreIndex uint16, wpHash Hash) {
    item := // get item
    item.Status = StatusSubmitted
    item.SubmittedAt = time.Now()
    item.CoreIndex = coreIndex
    item.WPHash = wpHash

    // Move to Inflight map
    qs.Inflight[blockNumber] = item
    qs.Status[blockNumber] = StatusSubmitted

    // Track hash for E_G matching
    qs.HashToBlock[wpHash] = blockNumber
}
```

## Bundle Lifecycle Summary

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          BUNDLE LIFECYCLE                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  BUILD           QUEUE           SUBMIT          GUARANTEE      ACCUMULATE │
│    │               │               │                │               │       │
│    ▼               ▼               ▼                ▼               ▼       │
│ ┌─────┐       ┌────────┐      ┌───────────┐   ┌────────────┐  ┌───────────┐│
│ │Build│ ────► │ Queued │ ───► │ Submitted │ ─►│ Guaranteed │ ─►│Accumulated││
│ │Bundle│      │        │      │           │   │            │  │           ││
│ └─────┘       └────────┘      └───────────┘   └────────────┘  └───────────┘│
│                   ▲               │                                   │     │
│                   │               │ timeout                           │     │
│                   │               ▼                                   ▼     │
│                   │          ┌─────────┐                        ┌─────────┐ │
│                   └──────────│ Requeue │                        │Finalized│ │
│                     rebuild  │ v++     │                        │ (done)  │ │
│                              └─────────┘                        └─────────┘ │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ MAPS:  Queued{}        Inflight{}         Inflight{}          Finalized{}  │
│ CORE:  not blocked     BLOCKED            free                free         │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Error Handling

| Error | Cause | Resolution |
|-------|-------|------------|
| G15 (AnchorNotRecent) | Anchor too old (>8 blocks) | Rebuild with fresh anchor |
| Guarantee Timeout | Validators didn't include bundle | Requeue with version++ |
| Network Error | QUIC stream failure | Retry submission |
| Max Retries Exceeded | Bundle failed too many times | Drop bundle, log error |

## Metrics & Logging

Key log messages to monitor:

```
Queue Runner: In submission window    queued=X submitted=Y guaranteed=Z canSubmit=true/false
Queue: Marked submitted               bundleID=B-N blockNumber=N version=V
Queue: Marked guaranteed              bundleID=B-N (core now free!)
Queue: Marked accumulated             bundleID=B-N (cleanup)
Queue: Timeout detected - will requeue
Queue Runner: Bundle anchor getting stale, will rebuild
```

## Design Decisions & FAQ

### Why accept guarantees for old versions?

**Decision:** The implementation accepts guarantees/accumulations for ANY version of a bundle, including older versions.

**Rationale:** Prevents duplicate transaction execution in network delay scenarios:
- If v1 times out and v2 is submitted, but then v1's guarantee arrives late
- Accepting v1's guarantee prevents v2 from executing the same transactions
- The first guarantee "wins" regardless of version number

**Implementation:** See [queue.go:424-438](queue.go#L424-L438) - explicitly accepts older versions.

### Why does inflight() only count Submitted, not Guaranteed?

**Decision:** `inflight()` and `CanSubmit()` only count `StatusSubmitted` items against the limit.

**Rationale:** Core availability optimization:
- Once a bundle is guaranteed (included in E_G), the core is **immediately free**
- The bundle stays in `Inflight` map for tracking until accumulated
- This allows maximum throughput: submit new bundle as soon as previous is guaranteed
- Guaranteed bundles don't block new submissions

**Implementation:** See [queue.go:297-308](queue.go#L297-L308).

### Why was SubmissionTimeout removed?

**Decision:** `SubmissionTimeout` was removed from configuration.

**Rationale:**
- Only `GuaranteeTimeout` and `AccumulateTimeout` are used in `CheckTimeouts()`
- Having an unused config parameter is misleading for operators
- The two specific timeouts provide clearer semantics for the lifecycle stages

**Implementation:** See [queue.go:706-788](queue.go#L706-L788).
