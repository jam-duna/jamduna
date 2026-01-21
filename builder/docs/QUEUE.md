# Builder Work Package Queues for Fast Proofs of Finality

For full throughput, JAM Rollups should NOT wait to submit one work package because a prerequisite work package
has not been guaranteed, accumulated, and finalized.  Instead, work package builders should put work packages
in a queue and submit them as soon as they are ready.  Provided sufficient cores are available, this supports rollup high throughput (1 block per 6 seconds) to support finality in < 30s from JAM for all rollups.

---

## Contiguous Accumulation (2026-01-20)

JAM accumulates work packages out of order across cores. For example, blocks may accumulate in sequence: 2,3,5,4,7,6,1,8,10,11,9. The canonical state must only advance when we have a contiguous chain of accumulated blocks.

### Problem

If `CommitAsCanonical` is called on every accumulation event, out-of-order accumulation causes incorrect canonical state:

- Block 11 accumulates → canonical = block 11's postRoot
- Block 9 accumulates LATER → canonical = block 9's postRoot (WRONG!)

Result: `eth_getBalance("latest")` returns stale state.

### Solution: Contiguous Tracking

The queue runner tracks accumulated blocks and only commits canonical state for the highest contiguous block:

```go
// In Runner:
accumulatedRoots  map[uint64]common.Hash // blockNumber -> postRoot
highestContiguous uint64                 // highest N where blocks 1..N are all accumulated
```

**Algorithm on each accumulation:**

1. Store `accumulatedRoots[blockNumber] = postRoot`
2. Advance: `while accumulatedRoots[highestContiguous+1] exists { highestContiguous++ }`
3. Commit only `accumulatedRoots[highestContiguous]` as canonical

**Example with sequence 2,3,5,4,7,6,1,8,10,11,9:**

| Accumulated | highestContiguous | Canonical Block |
|-------------|-------------------|-----------------|
| 2 | 0 (missing 1) | none |
| 3 | 0 | none |
| 1 | → 7 | 7 |
| 8 | → 8 | 8 |
| 10, 11 | 8 (missing 9) | 8 |
| 9 | → 11 | **11** |

Final canonical = block 11's postRoot (correct).

### BlockCommitment as Authoritative Root Source

When bundles are rebuilt (new RefineContext), the wpHash changes but the payload is immutable. `BlockCommitment` (148-byte header hash) remains stable across rebuilds:

```go
// In QueueItem:
BlockCommitment common.Hash

// In QueueState:
BlockCommitmentData map[common.Hash]*BlockCommitmentInfo

type BlockCommitmentInfo struct {
    PreRoot     common.Hash
    PostRoot    common.Hash
    TxHashes    []common.Hash
    BlockNumber uint64
}
```

At Phase 1 completion, `StoreBlockCommitmentData(commitment, info)` saves the authoritative roots. On accumulation, the PostRoot lookup order is:
1) `BlockCommitmentData[item.BlockCommitment]`
2) `BlockCommitmentData` by blockNumber (fallback if item is pruned)
3) `item.PostRoot` (last-resort legacy fallback with a warning)

### Block Tag Semantics

| Tag | Meaning | Returns |
|-----|---------|---------|
| `pending` | Latest Phase 1 executed block | All txs (optimistic state) |
| `latest` | Highest contiguous accumulated block | Canonical JAM state |
| `finalized` | Not yet implemented | Panics |

---

Users can obtain proofs of finality through the combination of a JAM light client and a rollup light client
as described below.

## Data Structures

Builder Rollup:

```go
type WorkPackageBundleStatus int

const (
	Submitted WorkPackageBundleStatus = iota
	Guaranteed
	Accumulated
	Finalized
)

type WPQueueItem struct {
	BlockNumber int
	Version     int
	EventID     uint64
	AddTS       uint64
}

type QueueState struct {
	Queued     map[int]*WPQueueItem
	Bundles    map[int]*WorkPackageBundle
	Status     map[int]WorkPackageBundleStatus
	CurrentVer map[int]int
	HashByVer  map[int]map[int][32]byte
}
```

## Queueing Transactions and Bundle

Every 6 seconds (matching JAM block time), if there are any txs in the txpool for the rollup and `Queued` has
not reached its max (say, H=8), the rollup node creates a work-package bundle
from those txs and pushes it into `Queued` as a `WPQueueItem`.  

A WPQueueItem has an `AddTS` and a local `EventID`. It does not have a stable
`workPackageHash` yet because the hash is computed after the `RefineContext`
and `CoreIndex` are finalized. While the block number is technically assigned in the rollups
`accumulate` process, the builder assigns a provisional `blockNumber` from a local
monotonic counter so the maps have a stable key. If consensus assigns a different
number, the builder rekeys the maps to the canonical block number.

## Bundle Build Window

**TWO TO THREE** seconds BEFORE the next JAM timeslot, IF there is
an element at the head of the queue, that element (call it W) is popped from
`Queued` and `BuildBundle` is called with W. During this step:

	(1) W.RefineContext is updated via `GetRefineContext`
	(2) W.RefineContext.Prerequisites is updated with the last in-flight `workPackageHash` (status Submitted or Guaranteed), which is expected to be accumulated
	(3) CoreIndex is assigned
	(4) `workPackageHash` is computed from the finalized bundle contents

`CE146` (SubmitWorkPackage) is called to submit the work package from the Builder
to a Guarantor on the Core, and W is stored in `Bundles` with `Status[blockNumber] = Submitted`.

If submission fails, W is returned to `Queued` as a fresh WPQueueItem and the
bundle is rebuilt later with a new `RefineContext` and prerequisites.

## Event Tracking

After submission succeeds, the Builder subscribes to work package hash events:

	(1) guaranteed
	(2) accumulated
	(3) finalized

On `finalized`, the Builder may request `State` elements relative to that block,
specifically C16 (containing the _accumulate root_).

## User Finality Proof Path

At the end of the day, users want their transactions finalized according to JAM, and will have a JAM light client and a rollup light client.
While light clients are optional for EVM rollups, for Orchard rollups, they are definitely not, and JAM's rollups fast finality is a powerful feature
relative to Ethereum "2 epoch" (or "7-day") finality (or ZCash's 1 hour).  

A user who submits a tx to a JAM Rollup (EVM, Orchard) obtains a JAM **proof of finality** by having
a JAM Header Hash signed by a 2/3 majority, known to the user's light client
that tracks the validators from JAM's recent epoch.

This Header Hash has a JAM state root, and the accumulate root (C16) must contain
the `workPackageHash`. The `workPackageHash` must also be sufficient to locate the
rollup block in JAM DA (if multiple items are exported, include an index or a
Merkle proof that binds the `workPackageHash` to the exported item).

The user may verify the JAM proof of finality by:

	(0) obtaining the JAM header hash, JAM state root, and JAM accumulate root
	(1) verifying the header hash is signed by 2/3 majority of validators
	(2) verifying the accumulate root contains the `workPackageHash`
	(3) verifying the rollup block from JAM DA contains their tx in the txroot

Given the work package hash, the user's JAM light client can:
(A) reconstruct the rollup block from JAM DA by asking all the validators for their shards using CE139, using (workpackagehash, 0...) doing erasure decoding. 
(B) find their `workPackageHash` in some C16 element of JAM, using JIP-2 to get `GetState` for every recent JAM header hash, or subscribing to JAM state events
(C) subscribe to BLS aggregate signature events to verify that a JAM block has been finalized

### EVM

Typically, the user just trusts their EVM rollup's JSON-RPC endpoint:
* `eth_getTransactionByHash` will provide the `workPackageHash` which will contain a block hash + block number
* `eth_getBlockByNumber` or `eth_getBlockByHash` will provide the rollup block finalization status

```
{"jsonrpc":"2.0","id":2,"method":"eth_getBlockByNumber","params":["finalized",false]}
```

Within the header is a txroot, and the user can obtain a proof of inclusion of their tx in that txroot themselves.   If `true` is provided, the `transactionRoot` and the rollup blocks `txhashes` will be visible.  
The connection from the rollup's `workPackageHash` and JAM's `headerHash` should be made visible to the user, so that they can verify further with (B)+(C).

### Orchard

[getblock](https://zcash.github.io/rpc/getblock.html) requires trial decoding by users with verbosity 2

```bash
Result (for verbosity = 1):
{
  "hash" : "hash",       (string) the block hash (same as provided hash)
  "confirmations" : n,   (numeric) The number of confirmations, or -1 if the block is not on the main chain
  "size" : n,            (numeric) The block size
  "height" : n,          (numeric) The block height or index (same as provided height)
  "version" : n,         (numeric) The block version
  "merkleroot" : "xxxx", (string) The merkle root
  "finalsaplingroot" : "xxxx", (string) The root of the Sapling commitment tree after applying this block
  "finalorchardroot" : "xxxx", (string, optional) The root of the Orchard commitment tree after
                               applying this block. Omitted for blocks prior to NU5 activation. This
                               will be the null hash if this block has never been connected to a
                               main chain.
                               NB: The serialized representation of this field returned by this method
                                   was byte-flipped relative to its representation in the `getrawtransaction`
                                   output in prior releases up to and including v5.2.0. This has now been
                                   rectified.
  "tx" : [               (array of string) The transaction ids
     "transactionid"     (string) The transaction id
     ,...
  ],
  "time" : ttt,          (numeric) The block time in seconds since epoch (Jan 1 1970 GMT)
  
  "trees": {                 (object) information about the note commitment trees
      "sapling": {             (object, optional)
          "size": n,             (numeric) the total number of Sapling note commitments as of the end of this block
      },
      "orchard": {             (object, optional)
          "size": n,             (numeric) the total number of Orchard note commitments as of the end of this block
      },
  },
  "previousblockhash" : "hash",  (string) The hash of the previous block
  "nextblockhash" : "hash"       (string) The hash of the next block
}
```

Result (for verbosity = 2):
```json
{
  ...,                     Same output as verbosity = 1.
  "tx" : [               (array of Objects) The transactions in the format of the getrawtransaction RPC. Different from verbosity = 1 "tx" result.
         ,...
  ],
  ,...                     Same output as verbosity = 1.
}
```

Examples:
```bash
> curl --user myusername --data-binary '{"jsonrpc": "1.0", "id":"curltest", "method": "getblock", "params": ["00000000febc373a1da2bd9f887b105ad79ddc26ac26c2b28652d64e5207c5b5"] }' -H 'content-type: text/plain;' http://127.0.0.1:8232/
> curl --user myusername --data-binary '{"jsonrpc": "1.0", "id":"curltest", "method": "getblock", "params": [12800] }' -H 'content-type: text/plain;' http://127.0.0.1:8232/
```

An Orchard light client needs to scan every rollup block for:
* `tx.orchard.actions[i].encryptedNote` (for **receivers**) - This is the Orchard note ciphertext that wallets trial-decrypt with the incoming viewing key (IVK). This is exactly the data wallets trial-decrypt to see if anyone has sent them anything.  
* `tx.orchard.actions[i].nullifier` (for **senders**) This is what senders will check to see if their note has been sent.

To support Orchard service users getting fast finality these must be put into the above `getblock` response:
(1) `workpackagehash` from the JAM Rollup
(2) `headerhash` from JAM
so that when Orchard service users see their transaction included in the JAM Orchard rollup block, they may fetch it from JAM DA (to verify) and subscribe to JAM for finalized blocks, and for each finalized block's header 
getting the C16 state and checking if their workpackage hash is included as an accumulate root of C16.


# Queue States and Resubmissions

Work packages which are dequeued from `Queued` into `Submitted` status might not be finalized because they:
* may fail to be guaranteed, due to < 2 guarantees, e.g. through some core-guarantor mismatch or through some form of censorship
* may fail to be accumulated, due to < 2/3 nodes being available, failures on consensus eg through some liveness
* may fail to be finalized, due to any combination of the above

In addition, if the builder network is decentralized,
if there is a lack of liveness between these builders, any possibility of potential forks requires a rollup
consensus algorithm at the L2 level.
We will set aside this possibility until v3 and just support a single builder.

## Protocol Limitation: On-Chain Duplicate Prevention

**CRITICAL**: The builder can minimize but **cannot eliminate** on-chain duplicate guarantees.

### Why Duplicates Can Occur

The JAM protocol does **not enforce uniqueness** at the consensus layer for work package hashes targeting the same rollup block. Validators will guarantee any work package that:

1. Passes validation checks (anchor recent, valid signatures, etc.)
2. Is within the rotation period validity window (4-7 slots)
3. Meets resource requirements

This means multiple versions of the same rollup block **can be guaranteed on-chain** if:

* Both are submitted before the rotation period expires
* Network partitions cause split guarantees
* Multiple builders submit conflicting versions

### Builder Defense: 54-Second Timeout

The builder uses a **54-second GuaranteeTimeout** (rotation period max 42s + safety 12s) to ensure:

* Old versions **expire on-chain** before new versions are submitted
* Validators reject expired guarantees via `checkAssignment()` failure
* Single-builder deployments minimize duplicate risk

However, this is **defense-in-depth, not prevention**:

* The builder controls submission timing
* The builder **cannot control** validator acceptance
* Protocol-level enforcement would require consensus changes

### Operational Impact

**For operators**:

* Monitor metrics: `DuplicateGuaranteeRejected`, `DuplicateAccumulateRejected`, `NonWinningVersionsCanceled`
* These counters track when duplicates were observed
* If counters increase, investigate network delays or timeout configuration

**For downstream systems**:

* Design consumers to handle duplicate guarantees gracefully
* Use "first guarantee wins" semantics
* Index by work package hash or rollup block number and deduplicate
* The queue's `WinningVer` tracking enforces this locally (builder-side only)
* Note: `bundleID` is a builder-local string (e.g., "B-7"), not protocol-visible

**For multi-builder setups** (future work):

* Will require builder-level coordination/consensus protocol
* Builders must coordinate before submission
* Consider primary-secondary election
* Note: Multi-builder setup not currently implemented (planned for v3+)

The queue uses `blockNumber` as its key and a per-block version to handle resubmissions.

Because ordered accumulation respects the prerequisites of the work package, an out of order submission
will result in cascading effects: If blockNumber 1000 does not accumulate, neither can 1001, thus 1002
because the prereqs have not accumulated.    So a resubmit of 1000 with a different WPH requires a resubmit of 1001 and 1002 with their new WPHs.  Due to H=12 max queue depth and typical network conditions, in practice this implies at most 2-3 items having a `WorkPackageBundleStatus` of `Submitted` at any time (blocking cores). If the number of submitted items exceeds the MaxInflight limit (default 3), the builder will wait until cores are freed (via guarantee) before submitting more WPs.  

Consider these situations

* Low-Volume: 1 tx every once in a while, very fast refines
```
 Queued: 
 Submitted: 
 Guaranteed: 1000 
 Accumulated: 
 Finalized: 999
```

* Medium-Volume: > 1 tx every block, but fast refining (1 core)
```
 Queued: 1003
 Submitted: 1002
 Guaranteed: 1001
 Accumulated: 1000 
 Finalized: 999
```

* High-Volume: very full blocks, refining to the max (2 core)
```
 Queued: 1004
 Submitted: 1002, 1003
 Guaranteed: 1000, 1001
 Accumulated: 998, 999
 Finalized: 996, 997
```

* DANGER: 997v1 got lost, so 998+ must be rebuilt with new work package hashes. v1 hashes
may still appear in `Submitted`/`Guaranteed`, but they are stale and will never accumulate.

```
 Queued: 997v2, 998v2, 999v2, 1000v2, 1001v2, 1002v2, 1003v2, 1004v2
 Submitted: 997v2, 998v2, 999v2
 Guaranteed (stale): 998v1, 999v1, 1000v1, 1001v1
 Accumulated: 996v1
 Finalized: 994v1, 995v1
```

Naturally, as the number of cores increases from C=2 to C=341, the situation becomes both easier (because there are more
cores we can just have every WP done on its own core and have fewer limits) and harder (because we need robust resubmission algorithms).



## Robust Queue Algorithm

This Go sketch keeps throughput high while remaining safe under reorgs, timeouts, and resubmissions.
It tracks versions per block number and enforces "first guarantee wins" to prevent duplicate execution.

```go
func (qs *QueueState) Tick(now uint64, maxInflight int) {
	for qs.inflight() < maxInflight {
		item := qs.dequeueLowest()
		if item == nil {
			return
		}
		bundle := buildBundle(item)
		hash := submit(bundle)
		qs.Bundles[item.BlockNumber] = bundle
		qs.Status[item.BlockNumber] = Submitted
		if qs.HashByVer[item.BlockNumber] == nil {
			qs.HashByVer[item.BlockNumber] = make(map[int][32]byte)
		}
		qs.HashByVer[item.BlockNumber][item.Version] = hash
	}
}

func (qs *QueueState) OnGuaranteed(hash [32]byte) {
	bn, ver := qs.lookupByHash(hash)

	// CRITICAL: First guarantee wins - prevents duplicate execution
	if winningVer, exists := qs.WinningVer[bn]; exists {
		if ver != winningVer {
			log("Rejecting guarantee for non-winning version")
			return
		}
	} else {
		// First guarantee - this version wins!
		qs.WinningVer[bn] = ver
		qs.cancelOtherVersions(bn, ver)
	}
	qs.Status[bn] = Guaranteed
}

func (qs *QueueState) OnAccumulated(hash [32]byte) {
	bn, ver := qs.lookupByHash(hash)

	// Only accept accumulation for winning version
	if winningVer, exists := qs.WinningVer[bn]; exists && ver != winningVer {
		log("Rejecting accumulation for non-winning version")
		return
	}
	qs.Status[bn] = Accumulated
	qs.pruneOlder(bn - retentionWindow())
}

func (qs *QueueState) OnFinalized(hash [32]byte) {
	bn, ver := qs.lookupByHash(hash)

	// Only accept finalization for winning version
	if winningVer, exists := qs.WinningVer[bn]; exists && ver != winningVer {
		log("Rejecting finalization for non-winning version")
		return
	}
	qs.Status[bn] = Finalized
}

func (qs *QueueState) OnTimeoutOrFailure(bn int, now uint64) {
	for k := bn; k <= qs.maxInFlight(); k++ {
		qs.CurrentVer[k]++
		delete(qs.Status, k) // Remove from in-flight, will be re-submitted
		qs.Queued[k] = &WPQueueItem{
			BlockNumber: k,
			Version:     qs.CurrentVer[k],
			EventID:     newEventID(),
			AddTS:       now,
		}
		delete(qs.Bundles, k)
	}
}

// Open Question: Need to define when rekeying from provisional to canonical `blockNumber` happens (event trigger).
func (qs *QueueState) OnCanonicalBlockNumber(oldBn, newBn int) {
	rekey(qs.Queued, oldBn, newBn)
	rekey(qs.Bundles, oldBn, newBn)
	rekey(qs.Status, oldBn, newBn)
	rekey(qs.CurrentVer, oldBn, newBn)
	rekey(qs.HashByVer, oldBn, newBn)
	updatePrereqs(qs.Bundles, oldBn, newBn)
}
```

Invariants:

* Only the winning version (first guaranteed) for a `blockNumber` may advance to Accumulated/Finalized.
* Accumulation is strictly ordered by `blockNumber` and prerequisites.
* Non-winning versions may be guaranteed but are never accumulated (duplicate execution prevention).

## Configuration Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `MaxQueueDepth` (H) | 12 | Maximum items in `Queued` before blocking new bundles |
| `MaxInflight` | 3 | Maximum items with status `Submitted` (cores blocked) |
| `SubmitRetryInterval` | 6s | **Fast retry**: resubmit same version every N seconds within 48s window |
| `MaxSubmitRetries` | 8 | **Fast retry**: max submission attempts (1 initial + 7 retries) |
| `GuaranteeTimeout` | 54s | **Slow timeout**: increment version after refine expiry (**must be ≥ 42s**) |
| `AccumulateTimeout` | 60s | Time before a `Guaranteed` item without accumulation is considered failed (10 JAM blocks) |
| `RetentionWindow` | 100 | Number of finalized blocks to retain in maps before pruning |
| `MaxVersionRetries` | 5 | Maximum version increments before dropping a block |

### Two-Tier Timeout Strategy

The queue implements a **two-tier timeout strategy** to recover quickly from network failures while preventing duplicate execution:

**Fast Retry (Same Version)**:

* Resubmits the **same work package** (same hash) every 6 seconds
* Maximum 8 attempts over 48 seconds (`RecentHistorySize` window)
* Safe because identical hash = idempotent (can only accumulate once)
* Recovers from transient QUIC timeouts and validator unavailability

**Slow Timeout (New Version)**:

* After 54 seconds without guarantee, increment version
* Rebuilds bundle with new refine context and prerequisites
* **Critical**: 54s = rotation period maximum (42s) + safety margin (12s)
* Ensures old version expired on-chain before new version submitted
* Prevents validators from guaranteeing both v1 and v2 (duplicate execution)

**CRITICAL DESIGN NOTE**: `GuaranteeTimeout` (54s) is derived from the JAM protocol's **strictest** validation constraint - the validator rotation period check in `checkAssignment()`. This check enforces that guarantee age must be ≤ `(currentSlot % RotationPeriod) + RotationPeriod`, which varies from 4-7 slots (24-42 seconds) depending on rotation position. Using the maximum (42s) + safety margin (12s) ensures that when a bundle times out and is resubmitted with a new version, the old version's guarantee slot is **too old for validators to accept**, preventing validators from guaranteeing both versions. Without this constraint, duplicate transaction execution is possible even with local `WinningVer` gating, since the builder cannot control which bundles validators choose to guarantee.

**Three validation constraints** (in order of strictness):

1. **Rotation period** (`checkAssignment`): 4-7 slots (24-42s) - **strictest, runs first**
2. **RecentBlocks** (`checkRecentBlock`): 8 slots (48s)
3. **LookupAnchor** (`checkTimeSlotHeader`): 24 slots (144s)

The configuration uses constraint #1's maximum (42s) plus a 12s safety margin (total 54s).

---

## Phase 2 Concurrency and State Isolation (2026-01-21)

### Problem: Concurrent Phase 2 Builds Share ubtReadLog

The EVM builder executes Phase 2 (witness generation) for multiple blocks concurrently. Each `BuildBundle` call:

1. Executes PVM/EVM which reads from UBT (balance, nonce, storage, code)
2. Logs those reads to `ubtReadLog` on the storage instance
3. Calls `BuildUBTWitness()` which uses `ubtReadLog` to generate proofs
4. Calls `ClearUBTReadLog()` after witness generation

**Bug**: When multiple Phase 2 builds run concurrently on the **same shared StateDB**, they share the same `ubtReadLog`:

```
Phase 2 (Block 37): ExecuteRefine starts, logs reads...
Phase 2 (Block 38): ExecuteRefine starts, logs reads... (interleaved!)
Phase 2 (Block 36): ClearUBTReadLog() → clears ALL reads!
Phase 2 (Block 37): BuildUBTWitness() → empty read log → proofKeys=0 → FAIL
```

**Symptoms**:
- `VerifyMultiProof failed: invalid proof` with `proofKeys=0, proofNodes=0, proofStems=0`
- `expectedRoot` matches `witnessRoot` (roots are correct, proof is empty)
- Random Phase 2 failures for blocks that should succeed

### Solution: Isolated StateDB per Phase 2 Execution

Each Phase 2 build uses `CopyForPhase2()` to create a lightweight isolated StateDB:

```go
// In executePhase2ForBlock:
stateDB := n.GetStateDB().CopyForPhase2()  // Isolated copy
actualStorage := stateDB.GetStorage().(types.EVMJAMStorage)
actualStorage.PinToStateRoot(preStateRoot)
bundle, workReport, err := stateDB.BuildBundle(...)  // Uses isolated ubtReadLog
```

### CopyForPhase2 vs Full Copy

| Aspect | `Copy()` | `CopyForPhase2()` |
|--------|----------|-------------------|
| `ubtReadLog` | Isolated (new empty log) | Isolated (new empty log) |
| `pinnedTree/activeRoot` | Isolated | Isolated |
| `JamState` | Deep copied | **Shared** (read-only) |
| `treeStore` | Shared | Shared |
| `InitTrieAndLoadJamState` | Yes (expensive) | **No** |
| Use case | Block import, forking | Phase 2 witness gen |

**Why sharing JamState is safe**:

Phase 2 `BuildBundle` only reads from JamState:
- `JamState.SafroleState.Timeslot` - timestamp for VM
- `JamState.RecentBlocks.B_H` - anchor lookup in `GetRefineContext`

JamState is only mutated during block import/accumulation, never during `BuildBundle`. This assumes JamState is not mutated in-place while Phase 2 runs (the node swaps StateDB on import).

### What Gets Isolated

`CloneTrieView()` (called by `CopyForPhase2`) creates storage with:

```go
// ISOLATED per-execution:
ubtReadLog:        make([]types.UBTRead, 0),  // Fresh empty log
ubtReadLogEnabled: true,
pinnedStateRoot:   clonedPinnedStateRoot,     // Own pinning state
pinnedTree:        pinnedTree,
activeRoot:        common.Hash{},             // Own active root

// SHARED (thread-safe or immutable):
ubtState:     t.ubtState,      // treeStore is copy-on-write, safe to share
serviceState: t.serviceState,  // Synchronized via mutex
db:           t.db,            // LevelDB is thread-safe
```

### Key Invariants

1. **Each Phase 2 execution has its own `ubtReadLog`** - prevents cross-contamination
2. **`treeStore` trees are immutable** - `ApplyWritesToTree` clones before mutation (copy-on-write)
3. **Pin/unpin is per-instance** - each Phase 2 pins its own `pinnedTree`
4. **JamState is read-only during BuildBundle** - safe to share

### Pending Head Root for Cross-Batch Chaining

**Problem**: Phase 1 blocks must chain state roots across batches, but `GetCanonicalRoot()` only advances after accumulation.

**Solution**: `pendingHeadRoot` tracks the postRoot of the most recent Phase 1 block:

```go
// In queue/runner.go:
type Runner struct {
    pendingHeadRoot common.Hash  // Tracks Phase 1 chain head
}

// After each Phase 1 block:
queueRunner.SetPendingHeadRoot(blockData.phase1Result.EVMPostStateRoot)

// When starting next batch:
currentRoot := queueRunner.GetChainHeadRoot(canonicalRoot)
// Returns pendingHeadRoot if set, otherwise canonicalRoot
```

This ensures:
- Block 6 chains from Block 5's postRoot (even before Block 5 accumulates)
- Batches don't restart from genesis
- State roots form a proper chain: genesis → B1 → B2 → ... → BN

### Summary of State Isolation

| Component | Isolation Level | Reason |
|-----------|-----------------|--------|
| `ubtReadLog` | Per-execution | Prevents cross-contamination of read tracking |
| `pinnedTree` | Per-execution | Each Phase 2 pins to its own preRoot |
| `activeRoot` | Per-execution | Execution context isolation |
| `treeStore` | Shared | Copy-on-write makes trees immutable |
| `JamState` | Shared | Read-only during BuildBundle |
| `pendingHeadRoot` | Per-Runner | Cross-batch Phase 1 chaining |

---

## StateDB Isolation for Block Import/Authoring & Auditing

### Problem: Concurrent Block Processing and Auditing

JAM nodes run multiple concurrent operations that all access StateDB:

1. **Block Import** (`ApplyBlock`): Imports blocks from the network, applies state transitions
2. **Block Authoring** (`runAuthoring`): Creates new blocks when the node is a proposer
3. **Auditing** (`runAudit`): Verifies work reports by re-executing refine operations

These operations run in different goroutines and can overlap. Without isolation:
- Auditing could read partial state during block import
- Authoring could see inconsistent trie state during concurrent imports
- `SetRoot` calls could affect concurrent operations

### Solution: Full StateDB Copy for Auditing

When a block is imported or authored, a **full copy** of the StateDB is sent to the auditing channel:

```go
// In ApplyBlock (block import):
if n.AuditFlag {
    if snap, ok := n.statedbMap[n.statedb.HeaderHash]; ok {
        select {
        case n.auditingCh <- snap.Copy():  // FULL COPY
            log.Debug(log.Audit, "ApplyBlock: auditingCh", ...)
        default:
            log.Warn(log.Node, "auditingCh full, skipping audit")
        }
    }
}

// In runAuthoring (block production):
if n.AuditFlag {
    select {
    case n.auditingCh <- newStateDB.Copy():  // FULL COPY
    default:
        log.Warn(log.Node, "auditingCh full, dropping state")
    }
}
```

### Why Full Copy() is Required for Auditing

Auditing differs from Phase 2 witness generation:

| Aspect | Phase 2 (`CopyForPhase2`) | Auditing (`Copy`) |
|--------|---------------------------|-------------------|
| JamState access | Read-only (Timeslot, RecentBlocks) | Read-only but needs full snapshot |
| Block access | Read-only | May modify (e.g., disputes) |
| AvailableWorkReport | Read-only | May need to verify/access |
| AncestorSet | Read-only | Needs deep copy for isolation |
| Trie state | Pinned to specific root | Needs isolated view |
| Lifetime | Short (single BuildBundle) | Long (async audit process) |

Auditing runs **asynchronously in a separate goroutine** and may take a long time (re-executing all work packages). During this time, the main node continues importing blocks and mutating state. A full copy ensures:

1. **Consistent snapshot**: Auditing sees the exact state at the block being audited
2. **No interference**: Block imports don't affect the auditing StateDB
3. **Safe mutations**: `audit_statedb.Block.Copy()` allows auditing to mark blocks audited

### StateDB.Copy() Deep Copy Details

```go
func (s *StateDB) Copy() (newStateDB *StateDB) {
    // Deep copy mutable collections
    tmpAvailableWorkReport := make([]types.WorkReport, len(s.AvailableWorkReport))
    copy(tmpAvailableWorkReport, s.AvailableWorkReport)

    tmpAncestorSet := make(map[common.Hash]uint32, len(s.AncestorSet))
    for k, v := range s.AncestorSet {
        tmpAncestorSet[k] = v
    }

    // Isolated trie view with own Root pointer
    isolatedSdb := s.sdb.CloneTrieView()

    return &StateDB{
        Block:            s.Block.Copy(),     // Deep copy - auditing may mutate
        JamState:         s.JamState.Copy(),  // Deep copy - full state snapshot
        sdb:              isolatedSdb,        // Isolated trie view
        AvailableWorkReport: tmpAvailableWorkReport,
        AncestorSet:         tmpAncestorSet,
        // ... other fields
    }
}
```

### CloneTrieView() Trie Isolation

Both `Copy()` and `CopyForPhase2()` use `CloneTrieView()` for trie isolation:

```go
func (t *StateDBStorage) CloneTrieView() types.JAMStorage {
    return &StateDBStorage{
        trieDB:        t.trieDB.CloneView(),  // Isolated trie view
        Root:          t.Root,                 // Own Root pointer
        stagedInserts: make(map[common.Hash][]byte),  // Fresh staging
        stagedDeletes: make(map[common.Hash]bool),

        // SHARED (thread-safe):
        db:           t.db,           // LevelDB - thread-safe
        ubtState:     t.ubtState,     // Synchronized via mutex
        serviceState: t.serviceState, // Synchronized via mutex

        // Per-clone state:
        activeRoot:        common.Hash{},
        ubtReadLog:        make([]types.UBTRead, 0),
        ubtReadLogEnabled: true,
    }
}
```

### Key Invariants for Block Import/Authoring/Auditing

1. **Auditing always receives a full `Copy()`** - ensures complete isolation from ongoing imports
2. **Block and JamState are deep copied** - auditing may run for seconds/minutes asynchronously
3. **Trie views are isolated** - `SetRoot` on one doesn't affect others
4. **treeStore is copy-on-write** - multiple readers (audit + import) are safe
5. **statedbMap access is mutex-protected** - prevents data races on the map itself

### Comparison: Copy() vs CopyForPhase2()

| Operation | Use Case | JamState | Block | Trie | Overhead |
|-----------|----------|----------|-------|------|----------|
| `Copy()` | Auditing, Forking | Deep copy | Deep copy | Isolated | High |
| `CopyForPhase2()` | Phase 2 witness gen | Shared | Shared | Isolated | Low |

**When to use which:**
- `Copy()`: Long-lived async operations that need a complete, immutable snapshot
- `CopyForPhase2()`: Short-lived operations that only read from JamState/Block

### panicIfClone: Safety Guard for Canonical State Mutations

Cloned storage instances (`CloneTrieView`) are marked with `isClone = true`. A safety guard prevents clones from accidentally mutating shared canonical state:

```go
// In storage/storage.go:
func (s *StateDBStorage) panicIfClone(operation string) {
    if s.isClone {
        panic(fmt.Sprintf("operation %q modifies canonical state - not allowed on CloneTrieView", operation))
    }
}
```

**Protected operations** (will panic if called on a clone):

- `SetCanonicalRoot()` - would corrupt shared canonical root
- `CommitAsCanonical()` - would commit clone's state as canonical
- `DiscardTree()` - would remove trees needed by other operations
- `ResetTrie()` - would reset the entire treeStore

**Why this matters:**

- Phase 2 clones (`CopyForPhase2`) share `ubtState` including `canonicalRoot`
- If a clone accidentally called `SetCanonicalRoot()`, it would corrupt the canonical state for ALL operations
- The panic is a **fail-fast** mechanism - better to crash immediately than silently corrupt state

**What clones CAN do:**

- Read from `treeStore` (thread-safe with mutex)
- Pin to state roots (`PinToStateRoot`) - uses clone's own `pinnedTree`
- Build witnesses (`BuildUBTWitness`) - uses clone's own `ubtReadLog`
- Apply writes to trees (`ApplyWritesToTree`) - copy-on-write, creates new tree

### Summary: Three Levels of StateDB Isolation

| Scenario | Method | What's Isolated | What's Shared |
|----------|--------|-----------------|---------------|
| Auditing | `Copy()` | Everything (JamState, Block, AncestorSet, Trie) | treeStore (CoW), LevelDB |
| Phase 2 witness | `CopyForPhase2()` | ubtReadLog, pinnedTree, activeRoot | JamState, Block, treeStore |
| Block import | Fresh StateDB via STF | Everything | treeStore (CoW), LevelDB |
