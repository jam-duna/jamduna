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
│         │ RemoveTransaction(hash) - called IMMEDIATELY after BuildBundle    │
│         ▼                                                                   │
│  ┌─────────────┐                                                            │
│  │   Removed   │ ─── transaction no longer in pool                          │
│  └─────────────┘                                                            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### When Transactions Are Removed

Transactions are removed from TxPool **after successful enqueue**, not after on-chain confirmation:

```go
// In buildAndEnqueueWorkPackageForCore():
bundle, workReport, err := n.BuildBundle(workPackage, extrinsicsBlobs, ...)
if err != nil {
    return fmt.Errorf("failed to build bundle: %w", err)
}

// Enqueue FIRST
blockNumber, err := queueRunner.EnqueueBundleWithOriginalExtrinsics(bundle, ...)
if err != nil {
    return fmt.Errorf("failed to enqueue bundle: %w", err)
}

// Remove transactions ONLY after successful enqueue
// CRITICAL: If enqueue fails (queue full), txs stay in pool for retry
for _, tx := range pendingTxs {
    txPool.RemoveTransaction(tx.Hash)
}
```

**Why remove after enqueue (not after build)?**
1. If enqueue fails (queue full), transactions stay in pool and retry next tick
2. Prevents losing transactions if queue is temporarily full
3. Transactions are committed to a bundle that is guaranteed to be tracked
4. If bundle fails/requeues, it rebuilds with same transactions (stored in `OriginalExtrinsics`)

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
| `MaxInflight` | 2 | Max bundles in Submitted state (= number of cores) |
| `GuaranteeTimeout` | 18s | Time to wait for E_G before requeue |
| `MaxRetries` | 3 | Max resubmission attempts |

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

### Duplicate Execution Prevention

**Critical:** Before requeuing a timed-out bundle, the system checks if ANY version of that bundle has already been guaranteed or accumulated. This prevents a race condition where:

1. Bundle v1 is submitted
2. Timeout fires (18s), v1 is requeued as v2
3. v1's guarantee arrives (delayed)
4. v2 is rebuilt and submitted
5. **BUG:** Both v1 and v2 get guaranteed → duplicate transaction execution!

**Fix:** Status checks at two points:

```go
// In CheckTimeouts - before triggering requeue
currentStatus := qs.Status[bn]
if currentStatus == StatusGuaranteed || currentStatus == StatusAccumulated || currentStatus == StatusFinalized {
    // Skip requeue - original version was already guaranteed
    continue
}

// In OnTimeoutOrFailure - double-check before actually requeuing
currentStatus := qs.Status[bn]
if currentStatus == StatusGuaranteed || currentStatus == StatusAccumulated || currentStatus == StatusFinalized {
    // Skip - guarantee arrived between timeout detection and requeue
    continue
}
```

This works because `qs.Status[bn]` is updated by `OnGuaranteed()` for ANY version of the bundle.

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
Queue Runner: In submission window    queued=X inflightMapSize=Y effectiveInflight=Z canSubmit=true/false
Queue: Marked submitted               bundleID=B-N blockNumber=N version=V
Queue: Marked guaranteed              bundleID=B-N (core now free!)
Queue: Marked accumulated             bundleID=B-N (cleanup)
Queue: Timeout detected - will requeue
Queue Runner: Bundle anchor getting stale, will rebuild
```
