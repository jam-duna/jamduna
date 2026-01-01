# Builder Work Package Queues for Fast Proofs of Finality

For full throughput, JAM Rollups should NOT wait to submit one work package because a prerequisite work package
has not been guaranteed, accumulated, and finalized.  Instead, work package builders should put work packages
in a queue and submit them as soon as they are ready.  Provided sufficient cores are available, this supports rollup high throughput (1 block per 6 seconds) to support finality in < 30s from JAM for all rollups.

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
consensus algorithm at the L2 level, motivating the use of Safrole, GRANDPA for builders finalizing the L2 rollup.
We will set aside this possibility until v3 and just support a single builder.

The queue uses `blockNumber` as its key and a per-block version to handle resubmissions.

Because ordered accumulation respects the prerequisites of the work package, an out of order submission 
will result in cascading effects: If blockNumber 1000 does not accumulate, neither can 1001, thus 1002
because the prereqs have not accumulated.    So a resubmit of 1000 with a different WPH requires a resubmit of 1001 and 1002 with their new WPHs.  Due to H=8 max queue depth and typical network conditions, in practice this implies at most 2-3 items having a `WorkPackageBundleStatus` of `Submitted` or `Guaranteed` at any time. If the number of in-flight items exceeds 3, the builder should avoid submitting more WPs until the pipeline clears.  

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
It tracks versions per block number and treats older hashes as stale.

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
	if ver != qs.CurrentVer[bn] {
		markStale(hash)
		return
	}
	qs.Status[bn] = Guaranteed
}

func (qs *QueueState) OnAccumulated(hash [32]byte) {
	bn, ver := qs.lookupByHash(hash)
	if ver != qs.CurrentVer[bn] {
		markStale(hash)
		return
	}
	qs.Status[bn] = Accumulated
	qs.pruneOlder(bn - retentionWindow())
}

func (qs *QueueState) OnFinalized(hash [32]byte) {
	bn, ver := qs.lookupByHash(hash)
	if ver != qs.CurrentVer[bn] {
		markStale(hash)
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
* Only the latest version for a `blockNumber` may advance to Accumulated/Finalized.
* Accumulation is strictly ordered by `blockNumber` and prerequisites.
* Stale hashes may be guaranteed but are never accumulated.

## Configuration Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `MaxQueueDepth` (H) | 8 | Maximum items in `Queued` before blocking new bundles |
| `MaxInflight` | 3 | Maximum items with status `Submitted` or `Guaranteed` |
| `SubmissionTimeout` | 30s | Time before a `Submitted` item is considered failed |
| `GuaranteeTimeout` | 18s | Time before a `Guaranteed` item is considered failed (3 JAM blocks) |
| `RetentionWindow` | 100 | Number of finalized blocks to retain in maps before pruning |
| `MaxVersionRetries` | 5 | Maximum resubmission attempts before dropping a block |
