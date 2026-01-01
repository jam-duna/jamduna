# Optimistic Funding with Private ZK Voting

This document specifies a **JAM-native optimistic funding mechanism** with **private positive/negative voting**, **fixed governance supply**, and **continuous (hourly) funding**, where voters update preferences infrequently (≈2×/month).

The write-up is now oriented toward a **proof-of-concept (PoC) implementation**. It identifies the minimal scope required to ship a working prototype, plus guardrails to keep the work tractable.

The design prioritizes:

* strong privacy (no individual vote revelation)
* bounded, predictable costs
* simple, deterministic state transitions
* compatibility with JAM services and ZK tooling
* a clear path to a small Proof of Concept (PoC)

### PoC goal & guardrails

* Deliver an **end-to-end demo**: CLI-driven vote updates, hourly funding rollups, and basic telemetry of aggregate state.
* Bias toward **fixed parameters** and **static project lists** to avoid governance bootstrapping during the PoC.
* Reuse existing JAM primitives (note commitments, nullifiers, bucketed execution) instead of introducing new gadgets.
* Ship a **single proving circuit** (UpdateVote) with deterministic sizing; avoid dynamic/recursive proofs in the prototype.

**Accelerated PoC cadence (demo-friendly)**
- Funding buckets every **10 minutes** (6 buckets/hour) with a fixed list of **5 projects**.
- Voters can update at most once per **2-hour** window (`MIN_INTERVAL`) unless they consume a ticket.
- Each bucket spends a fixed **100 tokens**; outputs should be visible in a dashboard/CLI within 1 bucket.
- Acceptance: end-to-end run over 2 hours should produce 12 consecutive buckets with monotonically advancing snapshots.

---

## 1. High-Level Model

* The protocol maintains a fixed list of projects `P[0..N-1]`.
* A fixed percentage of protocol revenue flows into an **optimistic funding pot** every hour.
* Allocation is based on **aggregate private preferences**:

  * Positive support increases a project’s share.
  * Negative support can reduce a project’s share down to zero, but never below.
* Individual votes are **never revealed**.
* Voters update preferences rarely; funding uses the **current aggregate** continuously.

---

## 2. Voting Power

Governance power is represented as **shielded governance notes** with fixed total supply.

Each voter has power `W` proven inside ZK circuits. Power can be:

* proportional to stake,
* delegated via private transfers of governance notes, or
* snapshotted from another system.

The total governance supply is fixed by construction.

---

## 3. Sparse Preference Constraints

Each voter may allocate preferences to at most:

* **8 positive projects**
* **4 negative projects**

Hard limits are enforced **structurally** in the circuit.

Rules:

* A project may not appear in both positive and negative lists.
* Unused slots are padded with `(project_id = 0, weight = 0)`.
* All weights are integers.

This gives a **maximum of 12 touched projects per voter**, keeping circuits and updates bounded.

---

## 4. VoteNote (Persistent Private State)

### PoC Storage Layout

For a PoC, store VoteNotes in a dedicated **VoteNote Tree** under a single object prefix, separate from payment notes. Use a compact fixed-width leaf encoding so validators can statically compute the hash path inside circuits without dynamic sizing.

Each voter maintains exactly one private preference note.

### Private fields

* `W`: voting power
* `pos[8]`: `(project_id, weight)`
* `neg[4]`: `(project_id, weight)`
* `last_update_epoch`: hour-based epoch
* `ρ`: randomness

### Commitment

```
cm = Com(
  owner_pk,
  W,
  H(pos[8]),
  H(neg[4]),
  last_update_epoch,
  ρ
)
```

VoteNotes live in a commitment tree like other shielded notes.

---

## 5. Global Aggregate State

The service maintains **public aggregate scores**:

```
S_pos[project_id] : u128
S_neg[project_id] : u128
```

Derived net support:

```
S_net[i] = max(0, S_pos[i] - S_neg[i])
```

Only aggregates are visible. Individual allocations are never revealed.

---

## 6. Funding Allocation (Hourly)

Let:

* `R_h` = hourly funding pot (fixed % of protocol revenue)

Allocation rule:

```
if Σ S_net == 0:
    fallback (equal split or carry-forward)
else:
    alloc[i] = R_h * S_net[i] / Σ_j S_net[j]
```

This runs every hour using the **current aggregate state**, regardless of when voters last updated.

---

## 7. UpdateVote Extrinsic (Unsigned)

Voters submit updates only when they want to change preferences.

```rust
UpdateVote {
  proof: Bytes,
  anchor_root: Hash,
  anchor_size: u64,

  vote_nf: Hash,        // nullifier for old VoteNote
  new_vote_cm: Hash,   // commitment for new VoteNote

  deltas: [DeltaEntry; 12],
  apply_bucket: HourId,
}
```

Where:

```rust
DeltaEntry {
  project_id: u32,
  delta_pos: i64,
  delta_neg: i64,
}
```

* Exactly 12 entries (8 pos + 4 neg union), padded with zeros.
* Each project appears at most once.

---

## 7.1 PoC flow of data

1. **Client side**
   * User selects up to 12 projects with integer weights.
   * A CLI builds the witness using the latest `anchor_root` and `anchor_size` fetched from the chain.
   * The CLI emits the `UpdateVote` extrinsic payload and keeps the new commitment locally for future proofs.
2. **Sequencer / builder**
   * Accepts unsigned `UpdateVote` transactions and places them into the next **hourly bucket**.
   * Optionally injects **cover updates** (zero deltas) to smooth out inference.
3. **Service runtime**
   * Verifies the proof, applies deltas to `S_pos`/`S_neg`, updates nullifiers, inserts the new commitment.
   * Emits **events** with bucket ID and per-project delta summaries for off-chain telemetry.
4. **Funding rollup**
   * An hourly job (can be a simple runtime timer) computes `alloc[i]` for the bucket and writes a `FundingSnapshot` object.
   * A CLI command can fetch the latest snapshot and render per-project hourly totals.

### PoC storage layout (public state)

* `S_pos` / `S_neg`: fixed-length arrays sized to `N` (static project list). Stored as 128-bit integers.
* `Nullifiers`: append-only bitset keyed by `vote_nf`. Consider a simple bitmap in DA for the prototype.
* `VoteNote tree`: reuse the existing shielded note Merkle tree implementation.
* `FundingSnapshot{bucket, R_h, S_net_root, alloc_root}`: persisted hourly for auditing and to drive dashboards.

### PoC parameters (proposed defaults)

| Parameter                  | Value                  | Rationale                                   |
|----------------------------|------------------------|---------------------------------------------|
| Projects `N`               | 5                      | Minimal for demo (expand to 32 later)      |
| MIN_INTERVAL               | 2 hours                | Shortened for PoC testing (14 days in prod) |
| Governance supply `Σ W`    | 1_000_000 units        | Round number for UX                         |
| Funding cadence            | 10 minutes             | Fast feedback for PoC (1 hour in prod)      |
| Funding pot share          | 100 tokens/bucket      | Fixed amount for predictable PoC demo       |
| Proof system               | Groth16 (precompiled)  | Small, fast verification path               |
| Hash function              | Poseidon               | Matches existing JAM shielded notes         |

**Performance targets for PoC:**
- Circuit proving time: <3s per vote update
- Bucket processing: <100ms for all projects
- Memory footprint: <512MB for full state
- Storage growth: <1MB per 100 updates

These can be hard-coded in the prototype and promoted to configuration later.

### ZK prover interface

* Input: old note witness (path, leaf), new preference list, timestamps, `MIN_INTERVAL`, `anchor_root`, `anchor_size`.
* Output: proof bytes plus `vote_nf`, `new_vote_cm`, and per-project deltas ready to feed into the extrinsic.
* A reference Rust gadget can live under `services/evm/prover/` to keep the CLI and runtime consistent.

### Testing the PoC

* **Unit tests**: circuit constraint tests (duplicate IDs, overflow, negative weights), serialization of `UpdateVote`.
* **Integration**: scenario with 3 voters and 5 projects where votes shift after the 14-day window; assert hourly allocation math.
* **Privacy sanity**: ensure the runtime only emits aggregated deltas and bucket IDs, not per-user details.

---

## 8. ZK Statement (UpdateVote)

The proof enforces:

### 1. Membership & Ownership

* Old VoteNote commitment exists at `anchor_root`.
* `vote_nf = NF(sk, epoch_domain, ρ_old)`.

### 2. Rate Limiting

Either:

* `current_epoch - last_update_epoch ≥ MIN_INTERVAL` (≈14 days), or
* spending a separate "update ticket" note (2 per month).

### 3. List Well-Formedness

* At most 8 positive and 4 negative entries (by structure).
* `project_id = 0 ⇔ weight = 0`.
* `0 ≤ weight ≤ W`.
* No duplicate project IDs.
* No overlap between positive and negative lists.

### 4. Budget Constraint

Choose one policy:

```
Σ pos_weights + Σ neg_weights ≤ W
```

(or separate caps if desired).

### 5. Delta Correctness

For every touched project:

```
delta_pos = new_w_pos - old_w_pos
delta_neg = new_w_neg - old_w_neg
```

Negative deltas are only allowed down to zero (no underflow).

### 6. New Commitment

```
new_vote_cm = Com(
  owner_pk,
  W,
  H(new_pos),
  H(new_neg),
  current_epoch,
  ρ_new
)
```

---

## 9. State Transition (Validators)

On successful verification:

1. Ensure `vote_nf` is unused.
2. Apply deltas:

```
S_pos[i] += delta_pos[i]
S_neg[i] += delta_neg[i]
```

3. Mark `vote_nf` as spent.
4. Insert `new_vote_cm` into the VoteNote tree.

Validators never learn per-voter preferences.

---

### PoC validator checklist

1. **Parsing & bounds**: reject any `project_id ≥ N`, malformed padding, or non-zero `delta_neg` that would underflow `S_neg`.
2. **Proof verification**: use the hard-coded Groth16 verifying key; only one circuit ID is supported in the prototype.
3. **Bucket alignment**: reject any `apply_bucket` older than the current bucket; allow only `current` or `current+1` to minimize queue growth.
4. **Atomicity**: apply all deltas or none; failed updates must not consume the nullifier.
5. **Event emission**: emit `(bucket, project_id, delta_pos, delta_neg)` for each non-zero delta to ease off-chain reconstruction.

---

## 10. Batching & Anti-Leakage

To reduce inference from single updates:

* Updates are **applied in hourly buckets**.
* Builders may add zero-delta cover updates.
* Aggregates change only at bucket boundaries.

This limits timing-based linkage.

---

## 11. Properties

### Privacy

* Individual votes are hidden.
* Only aggregate effects are visible.
* No per-epoch voting requirement.

### Efficiency

* Fixed-size circuits.
* Fixed-size extrinsics.
* No O(N) operations.

### Governance Guarantees

* Fixed total voting supply.
* No double updates within interval.
* No negative allocations.

---

## 12. PoC milestones (7-day sprint)

**Day 1-2: Foundation**
1. **Static config + mock data**
   * Implement constant project list (5 projects), funding share, and bucket interval.
   * Provide a fixture that seeds three governance notes for integration tests.
   * Create deterministic test vectors with expected funding outcomes.

**Day 3-4: Core Logic**
2. **Circuit + verifier**
   * Land the Groth16 circuit and verifying key; wire the verifier into the EVM service extrinsic handler.
   * Target <50k constraints for sub-second proving on laptop hardware.
3. **Runtime plumbing**
   * Enforce bucket alignment, apply deltas, emit events, and store `FundingSnapshot` per bucket.
   * Add invariant checking: no negative allocations, total weight conservation.

**Day 5-6: Tooling**
4. **Client tools**
   * Add a CLI command to build proofs (using the reference gadget) and submit `UpdateVote`.
   * Add a CLI command to fetch and display the latest snapshot/allocation.
   * Include demo governance note generation for testing.

**Day 7: Integration**
5. **Demo script + validation**
   * Compose an end-to-end script: seed notes → submit two updates → advance buckets → print funding table.
   * Add automated test that validates funding math against hand-calculated expectations.
   * Add observability metrics for proof verification time, bucket lag, and per-project funding.

**Success criteria:** Complete funding cycle with 3 voters, 5 projects, demonstrating privacy-preserving preference updates and deterministic hourly allocations.

---

## 13. Summary

This optimistic funding design provides:

* Continuous funding with infrequent participation
* Private ± voting with bounded expressiveness
* Simple deterministic aggregation
* Clean JAM-native ZK enforcement

It is suitable for protocol treasuries, public goods funding, and long-lived ecosystems where **preferences change slowly but funding flows continuously**.

---

## 13. PoC Implementation Notes

The goal is to validate **end-to-end plumbing** (circuit → extrinsic → state transition → hourly funding) without building full production tooling. A thin Jam service or a standalone harness can host the logic.

### PoC Success Criteria

* Ability to create an initial VoteNote and submit `UpdateVote` to rotate preferences.
* Aggregates `S_pos/S_neg` update deterministically and influence hourly allocations for a small fixed project set (e.g., 5 projects).
* Nullifier reuse is rejected and the VoteNote tree grows as expected.
* Circuits prove deltas correctly and enforce the list bounds/rate limit checks.

### Minimal Data Model

* **VoteNote Tree**: Merkle tree of `new_vote_cm`. For the PoC, a simple poseidon/keccak tree with fixed depth (e.g., 2¹⁶ leaves) is sufficient.
* **Nullifier Set**: Hash set or bitmap keyed by `vote_nf` (in-memory for testing, persisted for realistic runs).
* **Aggregate Vectors**: In-memory `Vec<u128>` for `S_pos/S_neg`, length = number of projects; persisted into the JAM object store between blocks.
* **Funding Pot**: Simulated hourly revenue input (constant or small random walk) to exercise allocation math.

### PoC Circuits

* **Update Circuit**: Proves membership of `vote_nf`, enforces rate limit, recomputes `(delta_pos, delta_neg)`, and outputs `new_vote_cm`.
* **Public Inputs**: `anchor_root`, `anchor_size`, `vote_nf`, `new_vote_cm`, `deltas[12]`, `apply_bucket`.
* **Constraints**: Use fixed arrays for project slots and reuse existing gadgets for membership, range checks, and Poseidon hashing where available in your toolkit (Halo2/Plonk-ish).

### PoC Extrinsic Flow

1. Client constructs proof and `UpdateVote` payload.
2. Service verifies proof (off-the-shelf verifier) and checks nullifier uniqueness.
3. Apply aggregate deltas in-memory; append `new_vote_cm` to the tree; mark `vote_nf` as spent.
4. At hour boundaries, run allocation formula and emit per-project allocation events/logs for inspection.

### Testing Checklist

* **Happy Path**: One voter updates twice; funding buckets reflect new weights after the scheduled bucket.
* **Boundary Lists**: Exactly 8 positive / 4 negative entries, plus zero-padding, are accepted; >8/>4 rejected by circuit or pre-check.
* **Negative Weight Clamp**: Attempt to reduce below zero fails; aggregate never underflows.
* **Rate Limit**: Second update inside `MIN_INTERVAL` fails unless accompanied by a ticket note.
* **Bucket Delay**: Allocation for `apply_bucket` occurs exactly at the configured hour boundary (no early application).

### Stretch Goals (If Time Allows)

* Integrate with an event-driven front-end that visualizes per-hour allocations.
* Add a small "cover traffic" mode where builders insert zero-delta updates to mask timing.
* Swap the in-memory aggregates for persisted JAM objects to exercise import/export paths.
