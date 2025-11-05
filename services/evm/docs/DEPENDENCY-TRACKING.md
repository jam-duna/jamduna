# Overlay-Native Dependency Tracking

This document explains how Majik captures read and write dependencies now that
the overlay itself is the single point of interaction with the vendor EVM. The
previous approach introduced a separate `TrackingOverlayedBackend` observer that
mirrored every call into the vendor overlay. The new design folds tracking into
`MajikOverlay` so all bookkeeping lives beside the actual state mutations.

## Goals

- **Zero duplicated state** – The vendor overlay already tracks per-transaction
  substates and final diffs. We reuse those structures directly.
- **Deterministic dependency sets** – Reads and writes are recorded at the point
  where they happen, inside the same functions that mutate or populate the
  overlay.
- **Simple integration points** – Only `MajikOverlay` talks to the tracker data
  structures. `MajikBackend` simply returns DA objects and receives diffs.

## Data Structures

Location: [services/evm/src/backend.rs:58-66](../services/evm/src/backend.rs#L58-L66)

```rust
struct OverlayTracker {
    /// All ObjectIDs read during the block, used by the work package builder
    block_reads: BTreeSet<ObjectDependency>,

    /// Mirrors the vendor substate stack; top entry corresponds to the active frame
    frame_reads: Vec<BTreeSet<ObjectDependency>>,

    /// Collected when a transaction commits; indexed by transaction ordinal
    tx_reads: Vec<BTreeSet<ObjectDependency>>,

    /// Pending reads for the current transaction (merged to tx_reads on commit)
    pending_tx_reads: BTreeSet<ObjectDependency>,

    /// Each write is identified by (address, slot/code/balance) and a monotonic counter
    writes: Vec<WriteRecord>,

    /// Per-key instance counters to distinguish multiple writes to same location
    write_counters: BTreeMap<WriteKey, u32>,

    /// Current transaction index (set by begin_transaction)
    current_tx: Option<usize>,
}

pub struct WriteRecord {
    pub tx_index: usize,
    pub key: WriteKey,
    pub instance: u32,
}
```

`OverlayTracker` lives inside `MajikOverlay` as a `RefCell` to allow interior mutability during read operations. Because the overlay is shared by all transactions within the block, the tracker can observe every read or write as soon as it happens.

## Read Recording

Location: [services/evm/src/backend.rs:588-631](../services/evm/src/backend.rs#L588-L631)

Example for `storage()`:

```rust
fn storage(&self, address: H160, index: H256) -> H256 {
    let value = self.overlay.storage(address, index);  // Delegate to vendor overlay
    let dependency = self.overlay.backend().object_dependency_for_storage(address, index);
    self.tracker.borrow_mut().record_read(dependency);
    value
}
```

**Flow:**

1. A runtime call (e.g. `storage(address, key)`) reaches `MajikOverlay`.
2. `MajikOverlay` delegates to the vendor `OverlayedBackend`, which checks its cache and falls back to `MajikBackend` on miss.
3. **Regardless of cache hit or miss**, `MajikOverlay` calls a helper method on `MajikBackend` (e.g., `object_dependency_for_storage()`) to resolve the ObjectID and required version.
4. The dependency is passed to `tracker.record_read(dependency)`, which inserts it into:
   - `block_reads` (accumulated for the entire block)
   - `frame_reads.last()` (current call frame)
   - `pending_tx_reads` (current transaction)

**Key point:** Dependencies are always recorded to the base DA object (shard/code), even if the value came from the overlay cache. This is correct for JAM because guarantors need to know which DA objects must be available when rebuilding the block.

**Implementation:** [services/evm/src/backend.rs:175-181](../services/evm/src/backend.rs#L175-L181)

```rust
fn record_read(&mut self, dependency: ObjectDependency) {
    self.block_reads.insert(dependency);
    if let Some(frame) = self.frame_reads.last_mut() {
        frame.insert(dependency);
    }
    self.pending_tx_reads.insert(dependency);
}
```

## Write Recording

Location: [services/evm/src/backend.rs:658-722](../services/evm/src/backend.rs#L658-L722)

Example for `set_storage()`:

```rust
fn set_storage(&mut self, address: H160, key: H256, value: H256) -> Result<(), ExitError> {
    self.overlay.set_storage(address, key, value)?;  // Update vendor overlay first
    self.tracker.borrow_mut().record_write(WriteKey::storage(address, key));
    Ok(())
}
```

**Flow:**

1. The runtime invokes a mutating trait method (e.g., `set_storage`, `deposit`, `inc_nonce`, `set_code`).
2. `MajikOverlay` updates the vendor overlay with the new value **first**.
3. `MajikOverlay` creates a `WriteKey` containing the account address and the specific field being mutated (storage key, balance, nonce, code, etc.).
4. `tracker.record_write(key)` increments a per-key instance counter and pushes a `WriteRecord` into the `writes` vector.

**Implementation:** [services/evm/src/backend.rs:183-189](../services/evm/src/backend.rs#L183-L189)

```rust
fn record_write(&mut self, key: WriteKey) {
    if let Some(tx_index) = self.current_tx {
        let counter = self.write_counters.entry(key.clone()).or_insert(0);
        *counter += 1;
        self.writes.push(WriteRecord { tx_index, key, instance: *counter });
    }
}
```

**Properties:**

- Writes are appended to `writes` in execution order
- Each `WriteKey` gets a monotonic instance counter (1st write = instance 1, 2nd write = instance 2, etc.)
- Multiple writes to the same slot are distinguishable
- Transaction index is captured at write time (from `current_tx`)

These records are consumed at block finalization to produce deterministic write ordering for `MajikBackend::apply_overlay_changes()` and for guarantor dependency reports.

## Substate Hooks

Location: [services/evm/src/backend.rs:525-540](../services/evm/src/backend.rs#L525-L540)

`MajikOverlay` implements `TransactionalBackend` to intercept substate pushes and pops, keeping `frame_reads` aligned with the vendor substate stack:

```rust
fn push_substate(&mut self) {
    self.overlay.push_substate();
    self.tracker.borrow_mut().enter_frame();
}

fn pop_substate(&mut self, strategy: MergeStrategy) -> Result<(), ExitError> {
    self.overlay.pop_substate(strategy)?;
    let mut tracker = self.tracker.borrow_mut();
    match strategy {
        MergeStrategy::Commit => tracker.commit_frame(),
        MergeStrategy::Revert | MergeStrategy::Discard => tracker.revert_frame(),
    }
    Ok(())
}
```

**Frame operations:**

- **`enter_frame()`** – Pushes a new, empty `BTreeSet<ObjectDependency>` onto `frame_reads`. Called for nested CALL/CREATE operations.

- **`commit_frame()`** ([backend.rs:161-169](../services/evm/src/backend.rs#L161-L169)) – Pops the top frame and unions its reads into the parent frame:
  ```rust
  fn commit_frame(&mut self) {
      if let Some(child) = self.frame_reads.pop() {
          if let Some(parent) = self.frame_reads.last_mut() {
              for dependency in child {
                  parent.insert(dependency);
              }
          }
      }
  }
  ```

- **`revert_frame()`** – Simply pops and discards the top frame without merging.

**Transaction-level framing:**

Transaction boundaries are managed separately via `begin_transaction()` and `commit_transaction()` / `revert_transaction()`:

- **`begin_transaction(tx_index)`** ([backend.rs:129-138](../services/evm/src/backend.rs#L129-L138)) – Sets `current_tx`, clears `pending_tx_reads`, and pushes the root frame.
- **`commit_transaction()`** ([backend.rs:140-149](../services/evm/src/backend.rs#L140-L149)) – Snapshots `pending_tx_reads` into `tx_reads[tx_index]`.
- **`revert_transaction()`** ([backend.rs:151-155](../services/evm/src/backend.rs#L151-L155)) – Clears `current_tx` and `pending_tx_reads` without saving.

Because the hooks execute inside `MajikOverlay` (immediately after vendor overlay operations), there is no risk of the tracker getting out of sync with the actual runtime state.

## Block Finalisation

Location: [services/evm/src/backend.rs:339-343](../services/evm/src/backend.rs#L339-L343)

At the end of the block, `MajikOverlay::deconstruct()` extracts data from both the vendor overlay and the tracker:

```rust
pub fn deconstruct(self) -> (MajikBackend, OverlayedChangeSet, TrackerOutput) {
    let tracker = self.tracker.into_inner().into_output();
    let (backend, change_set) = self.overlay.deconstruct();
    (backend, change_set, tracker)
}
```

**Flow:**

1. The tracker is consumed and converted to `TrackerOutput` ([backend.rs:191-197](../services/evm/src/backend.rs#L191-L197)):
   ```rust
   fn into_output(self) -> TrackerOutput {
       TrackerOutput {
           block_reads: self.block_reads,
           tx_reads: self.tx_reads,
           writes: self.writes,
       }
   }
   ```

2. The vendor overlay is deconstructed to extract the `MajikBackend` and `OverlayedChangeSet` (containing all storage, balance, code, and log changes).

3. The orchestrator receives all three components:
   - `MajikBackend` – Contains the base DA state (shards, code, SSRs)
   - `OverlayedChangeSet` – The diff to apply
   - `TrackerOutput` – Dependency metadata (block reads, per-tx reads, ordered writes)

4. The orchestrator calls `backend.apply_overlay_changes(&change_set)` ([backend.rs:423-450](../services/evm/src/backend.rs#L423-L450)) to persist the changes to DA.

5. The orchestrator forwards `TrackerOutput` to the work package builder for guarantor consensus and parallel execution analysis.

**Note:** There is no `reset()` method because `MajikOverlay` is consumed by `deconstruct()`. For the next block, a fresh `MajikOverlay` instance is created.

## ObjectID Resolution Helpers

Location: [services/evm/src/backend.rs:398-421](../services/evm/src/backend.rs#L398-L421)

`MajikBackend` provides helper methods to compute `ObjectDependency` values without performing actual DA imports. These are called by `MajikOverlay` during read tracking:

```rust
pub fn object_dependency_for_storage(&self, address: H160, key: H256) -> ObjectDependency {
    let shard_id = self.resolve_shard_id(address, key);
    let required_version = self.storage_shards.get(&address)
        .map(|cs| cs.ssr.header.version).unwrap_or(0);
    ObjectDependency {
        object_id: shard_object_id(address, shard_id),
        required_version
    }
}

pub fn object_dependency_for_balance(&self, address: H160) -> ObjectDependency {
    let system_contract = h160_from_low_u64_be(0x01);
    let balance_key = balance_storage_key(address);
    self.object_dependency_for_storage(system_contract, balance_key)
}

pub fn object_dependency_for_nonce(&self, address: H160) -> ObjectDependency {
    let system_contract = h160_from_low_u64_be(0x01);
    let nonce_key = nonce_storage_key(address);
    self.object_dependency_for_storage(system_contract, nonce_key)
}

pub fn object_dependency_for_code(&self, address: H160) -> ObjectDependency {
    let object_id = code_object_id(address);
    let required_version = self.code_versions.get(&address).copied().unwrap_or(0);
    ObjectDependency { object_id, required_version }
}
```

**Design rationale:**

- **Separation of concerns:** `MajikBackend` owns SSR metadata and versioning, so it's the only component that can correctly resolve shard IDs and versions.
- **No double tracking:** The overlay reads values from the vendor cache/backend, then separately asks for the dependency metadata. No state is duplicated.
- **Balance/nonce as storage:** JAM stores balances and nonces in the system contract (address 0x01) using storage shards, so they reuse the storage dependency helper.
- **Version tracking:** Storage shards track `ssr.header.version`, code objects track `code_versions[address]`.

## Benefits

- **One code path** – The runtime invokes exactly one struct, which means anyone
  debugging dependency emission only needs to read `MajikOverlay`.
- **No redundant abstraction** – Tracking lives alongside the state mutations,
  eliminating the previously duplicated overlay wrapper.
- **Deterministic output** – Writes are recorded in execution order with stable
  counters. Read sets mirror call frame boundaries without extra ceremony.

This streamlined design keeps the vendor overlay intact while letting Majik own
the execution surface area required for JAM-specific bookkeeping.

## Implementation Summary

**Key Invariants:**

1. **All reads record dependencies** – Even if the value comes from the overlay cache, the dependency to the base DA object (shard/code) is always recorded. This is correct for JAM because guarantors need to know which DA objects to import when rebuilding the block.

2. **Reads are frame-scoped** – Every read goes into three sets simultaneously:
   - `block_reads` (never cleared mid-block)
   - `frame_reads.last()` (current call frame)
   - `pending_tx_reads` (current transaction)

3. **Writes are transaction-scoped** – Each write records its `tx_index` at write time, ensuring correct attribution even if the same slot is written by multiple transactions.

4. **Frames mirror vendor substates** – `push_substate`/`pop_substate` hooks ensure `frame_reads` stack depth always matches the vendor overlay's substate stack depth.

5. **Reverts discard cleanly** – Failed transactions and reverted nested calls don't pollute the final dependency sets because their frames are popped without merging.

**Testing:**

See [backend.rs:1262-1280](../services/evm/src/backend.rs#L1262-L1280) for a basic test that validates read and write tracking through a complete transaction lifecycle.

**Future Work:**

- Integration with work package builder to emit dependency graphs
- Guarantor consensus on `block_reads` to prove DA availability
- Parallel execution scheduler using `tx_reads` to detect conflicts
