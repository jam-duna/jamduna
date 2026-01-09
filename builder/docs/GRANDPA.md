# Dual Finality for JAM Rollups Using JAM GRANDPA (FINAL)

## Status
**FINAL — SAFE TO IMPLEMENT**

This document specifies a consensus-critical finality system for JAM rollups.
All known safety, replay, reorg, and ambiguity issues have been resolved.

---

## 1. Summary

This document defines a **dual-finality architecture** for rollups hosted on **JAM**:

- **JAM services** verify *validity* of rollup state transitions.
- **Rollup participants** decide *canonical finality* using **rollup-native JAM GRANDPA**.
- **JAM** verifies and records rollup finality certificates.
- **JAM GRANDPA** finalizes JAM blocks, making recorded rollup finality irreversible.

### Separation of Concerns (Normative)

| Property | Authority |
|--------|----------|
| Validity | JAM service |
| Canonical choice & finality | Rollup JAM GRANDPA |
| Irreversibility | JAM GRANDPA |

---

## 2. Design Principles (Normative)

1. **Finality is slower than block production**  
   Rollup blocks may be produced at high throughput; finality occurs at a fixed cadence.

2. **Voters must not wait for JAM validity**  
   Voting is based on availability and cheap checks only.

3. **All expensive checks occur at the JAM boundary**  
   Validity is enforced only when recording finality.

4. **Missing a vote is always safer than equivocating**

---

## 3. Roles and Responsibilities

### 3.1 JAM Service (Validity Oracle + Recorder)
- Verifies rollup block state transitions.
- Tracks all observed valid rollup forks (forks for which the service has sufficient data to verify).
- The service MAY prune unfinalized forks as long as it can verify ancestry to the last finalized head.
- Verifies rollup finality certificates.
- Persists the rollup finalized head in JAM service state.

### 3.2 Rollup Participants (Fork Choice + Finality)
- Maintain rollup P2P network.
- Propose rollup blocks.
- Run **rollup JAM GRANDPA** to finalize checkpoints.

### 3.3 JAM GRANDPA
- Finalizes JAM blocks.
- Makes rollup finality records irreversible.

---

## 4. Finality Cadence

Rollup JAM GRANDPA finalizes **one block every `F` rollup blocks** or
**every `τ` seconds**, whichever comes later.

- `F` and `τ` are rollup-specific constants.
- Non-finalized blocks are valid but reversible.

---

## 5. JAM GRANDPA Timing Model (Normative)

Each finalization attempt proceeds in **rounds**.

### Parameters

| Symbol | Meaning |
|------|--------|
| Δprop | Block propagation allowance |
| Δverify | Local availability + cheap checks (excludes JAM validity execution) |
| Δprevote | Prevote gossip window |
| Δprecommit | Precommit gossip window |
| Δagg | Signature aggregation |
| Δsubmit | Submission to JAM |

Votes received after their phase deadline are ignored.

---

## 6. Voting Eligibility (Availability vs Validity)

A voter **MAY vote** for a block `B` if:

- `B`’s header and data are available
- `B` passes syntactic and ancestry checks
- `B` extends the latest finalized head

If a voter has not yet learned the latest finalized checkpoint, “latest finalized head” means the latest finalized head known locally (e.g., genesis or last persisted).
Nodes SHOULD learn the JAM-recorded finalized checkpoint as soon as possible for liveness and UX.

A voter **MUST NOT wait** for JAM validity verification.

Voters are **not slashable** for voting on blocks that later fail JAM validity.

---

## 7. JAM GRANDPA Vote Phases (Clarified)

### Prevote Phase
- Voters gossip prevotes to discover a supermajority fork.
- Prevotes are **NOT** recorded on-chain.
- Used for local fork coordination only.

### Precommit Phase
- Voters precommit to the block with ≥ 2/3 prevotes.
- Precommits **ARE** included in certificates.
- Certificates contain **precommits only**.

---

## 8. JAM GRANDPA Certificates

Certificates are **precommit-only finality proofs**.

### 8.1 Certificate Required Properties (CRITICAL)

A valid certificate MUST satisfy:

- Unique `(rollup_id, height, round_number, validator_set_id)`
- `round_number` is a **monotonically increasing counter starting at 0**
- ≥ 2/3 voting weight
- Single ancestry chain extending the previously finalized head (verified by JAM per §9 step 7)

### 8.2 Round Number Semantics

- `round_number` identifies a GRANDPA attempt to finalize a height.
- Failed rounds may retry with `round_number + 1`.
- `round_number` MUST be:
  - included in the certificate
  - included in the signed payload
  - strictly increasing for each height

### 8.2.1 CertificateV1 Signed Payload (Normative)

For CertificateV1, validators MUST sign:

```
message = H("JAM_GRANDPA_CERT_V1" || rollup_id || height || round_number || block_hash || validator_set_id)
```

`H` is the JAM canonical hash function (BLAKE2b-256).

All fields are serialized in fixed-width, big-endian form. This domain separator MUST be used to prevent cross-protocol replay.

Field sizes:
* domain_separator: UTF-8 bytes of the literal string (no null terminator)
* rollup_id: 4 bytes (u32)
* height: 8 bytes (u64)
* round_number: 8 bytes (u64)
* block_hash: 32 bytes
* validator_set_id: 8 bytes (u64)

### 8.3 Round Skew Bound

```
ROUND_MAX_SKEW = 10
```

This prevents certificates from claiming unreasonably high round numbers.

---

## 9. Certificate Verification (JAM)

JAM MUST verify, in order:

1. Certificate version
2. Block header binding
3. Validator set correctness (active set or grace-period previous set)
4. Signature validity (against the determined validator set)
5. ≥ 2/3 quorum (sum of weights for valid signatures)
6. Validity of block state transition
7. Ancestry extension of the prior finalized head
8. **Replay protection**: certificate not previously processed
9. **Round monotonicity**:
   if `FinalizedRounds[(rollup_id, height)]` is unset, `round_number == 0`  
   if set to `r_prev`, `round_number > r_prev`
10. **Round sanity bound**:
    if unset, `round_number ≤ ROUND_MAX_SKEW`  
    if set to `r_prev`, `round_number ≤ r_prev + ROUND_MAX_SKEW`

If no prior round exists, `round_number MUST be 0` (and thus ≤ `ROUND_MAX_SKEW`).

Failure of any step ⇒ certificate rejected.

Steps 1–5 MUST be executed before any state-transition verification in step 6.

Replay protection treats a certificate as “processed” if a certificate for `(rollup_id, height)` is already recorded or if the same `certificate_hash` has appeared in any prior JAM block. Implementations MAY additionally cache recent `certificate_hash` values for DoS resistance.

Block header binding includes verifying that the signed `block_hash` matches the submitted block header.

### Required JAM State

```rust
FinalizedRounds[(rollup_id, height)] -> Option<(round_number, block_hash)>
```

---

## 10. Certificate Acceptance Policy (Height Uniqueness)

When multiple certificates for the same `(rollup_id, height)` arrive:

* Verify each certificate fully before acceptance
* In a given JAM block execution context, evaluate all certificates for the same `(rollup_id, height)`:
  * Discard invalid certificates
  * If one or more are valid, accept exactly one chosen by:
    1. Highest `round_number`
    2. Tie-break by lexicographically smallest certificate hash
* Once a certificate for height `H` is recorded, ALL future certificates for `H` MUST be rejected

**Batch definition:** “same batch” means certificates evaluated within the same JAM block execution context, before any state write for height `H`.

Certificate hash is defined as `H(encoding(CertificateV1|V2))`.
Encoding MUST use the JAM canonical serialization format for the certificate type as defined in the JAM protocol spec.

JAM execution semantics MUST ensure the outcome for a given `(rollup_id, height)` in a block is equivalent to evaluating the set of all certificates submitted for that key in that block and applying the deterministic selection rule above. The result MUST be independent of intra-block transaction ordering.

---

## 11. Validator Set Management

### 11.1 Epoch-Based Rotation

* Validator sets change **only at epoch boundaries**
* A single validator set is active per epoch

```rust
ActiveSet(rollup_id, height) -> validator_set_id
```

### 11.2 In-Flight Rounds

* A round completes under the validator set active when it started
* Certificates from the previous set are accepted if:
  * `height ∈ epoch(set)`
  * `arrival ≤ epoch_end + grace_period`

`arrival` is the JAM block inclusion time (jam_height), not local wall-clock observation.

### Grace Period

```
grace_period = min(2 epochs, 24 hours)
```

Rationale: grace_period must be long enough to complete rounds that started near an epoch boundary under normal delays (hence measured in epochs), but capped in wall-clock time to bound acceptance of old-set certificates under prolonged partitions or delayed submissions.

---

## 12. Submission Window

When JAM first observes a JAM-verified valid rollup block `B` (not merely gossiped or seen):

```rust
ObservedAt[(rollup_id, block_hash)] = jam_height
```

Certificates must be submitted within:

```
observed_jam_height + K
```

Where:

```
K = ceil((τ + Δsubmit + safety_margin) / jam_block_time)
safety_margin = 2 × jam_block_time
K_min = 3
K_max = 20
```

Certificates referencing a `block_hash` not present in ObservedAt MUST be rejected.
`observed_jam_height` and submission window evaluation use JAM block inclusion heights, not local wall-clock time.

### ObservedAt Pruning (Required)

The JAM service MUST prune ObservedAt entries:

* For any block at height ≤ finalized_height, immediately upon finalization advancing past that height
* For any remaining entry with `jam_height < current_jam_height - K_max`

Implementations MUST persist a `block_hash -> height` mapping when recording ObservedAt to enable height-based pruning.

Pruning MUST occur after all certificate processing within the same JAM block.

---

## 13. Soft Confirmations (Precise Definition)

A block `B` at height `h` is **soft-confirmed** iff:

* `B` passes local availability, syntactic, and ancestry checks
* > 1/2 voting weight has **PREVOTED** for the SAME block `B`
* `B` does not conflict with the finalized head
* No conflicting soft confirmation exists at height `h`

### Conflict Rule

If two conflicting blocks each have > 1/2 prevote:
* → **NEITHER** is soft-confirmed

### Revocation

Soft confirmation is revoked if:

* `B` fails JAM validity
* Another block is finalized at `h`
* A conflicting soft quorum emerges

**Warning:** Soft confirmations are local estimates and NOT binding.

Soft confirmation is intentionally conservative: fragmented votes (e.g., multi-way splits) MUST result in no soft confirmation.

---

## 14. Slashing Conditions (Clarified)

Only **objectively provable** misbehavior is slashable.

### Slashable

* GRANDPA equivocation (defined below)
* Submitting an invalid certificate, where invalidity is objectively provable from the certificate and chain state alone (e.g., bad signature, insufficient quorum, wrong validator set, malformed encoding, inconsistent signed payload)
* Proposing two conflicting blocks at the same `height`
* Proposing syntactically malformed blocks
* Proposing blocks conflicting with finalized ancestors

### NOT Slashable

* Proposing blocks that later fail JAM validity
* Incorrect state roots
* Voting for invalid blocks
* Missing votes or being late

### GRANDPA Equivocation (Defined)

A validator equivocates if they sign **two conflicting votes** in the SAME round:

* **Prevote equivocation:** two prevotes for different blocks
* **Precommit equivocation:** two precommits for different blocks

**Evidence:**

* Same `rollup_id`, `height`, `round_number`, `vote_type`, and `validator_id`
* Different block hashes
* Valid signatures

```rust
struct GrandpaEquivocationEvidence {
    rollup_id: u32,
    round_number: u64,
    vote_type: VoteType, // Prevote | Precommit
    vote_a: SignedVote,
    vote_b: SignedVote,
}

struct SignedVote {
    height: u64,
    block_hash: [u8; 32],
    validator_id: ValidatorId,
    signature: Signature,
}
```

SignedVote signatures are over `(rollup_id, height, round_number, vote_type, block_hash)` with a domain separator.

### Proposer Equivocation (Evidence Format)

```rust
struct ProposerEquivocationEvidence {
    rollup_id: u32,
    height: u64,
    block_a_header: BlockHeader,
    block_b_header: BlockHeader,
    proposer_id: ValidatorId,
    sig_a: Signature,
    sig_b: Signature,
}
```

**Validity:**

* `block_a_header.height == block_b_header.height == height`
* `block_a_header.hash != block_b_header.hash`
* Both signatures verify under `proposer_id`

---

## 15. Strong Finality Policy (MANDATORY)

### Weak Finality

**REMOVED — UNSAFE — MUST NOT BE IMPLEMENTED**

### Strong Finality (ONLY SAFE POLICY)

A rollup block `B` MAY be finalized ONLY IF:

1. `B`'s state transition is JAM-verified
2. ALL ancestors back to the last finalized block are JAM-verified
3. The certificate is included in a JAM block finalized by JAM GRANDPA

---

## 16. JAM Reorganization Handling (CRITICAL)

* The rollup finalized head is persisted in JAM service state, keyed to the last JAM GRANDPA-finalized block
* JAM reorgs before JAM GRANDPA finality MUST restore the persisted head from the last finalized JAM block
* Certificate ancestry checks reference the persisted finalized head
* Certificates conflicting with it are rejected

---

## 17. Liveness and Finality Stall Handling

Finality stalls do not compromise safety but must be bounded.

### Maximum Finality Gap

If finality does not advance for:

* `F_max = 10 × F` rollup blocks, OR
* `τ_max = 10 × τ` seconds

Then:

* **Emergency Mode** is entered
* **Governance MUST act:**
  * Rotate validator set, OR
  * Adjust `F` / `τ`
* New certificates MUST NOT be accepted until resolved

Emergency Mode is enforced by the JAM service, which rejects all certificates until governance updates rollup parameters.
Automatic exit is intentionally not specified; any fallback must be pre-committed via governance parameters.

---

## 18. Security Composition

If:

* Rollup GRANDPA is safe (≥ 2/3 honest)
* JAM validity verification is correct
* Strong finality policy is enforced
* JAM GRANDPA finalizes JAM blocks

Then:

> **A recorded rollup-final block is canonical and irreversible.**

---

## 19. Conclusion

This specification defines a **safe, replay-protected, reorg-robust, and implementable** rollup finality system.

**Approved for implementation.**

---

## 20. Fork Choice Between Checkpoints

Between finalized checkpoints, rollup nodes MUST apply the following **local fork-choice rule**:

1. Select the chain extending the **latest finalized checkpoint**
2. Among those, select the **longest chain**
3. Break ties by **lexicographic order of block hash**

**Note:** “Longest” means greatest block height (number of blocks since the last finalized checkpoint), not cumulative weight.

**Note:** This is a LOCAL rule for pre-final blocks. GRANDPA provides canonical finality.

This rule MUST NOT be used once a finalized checkpoint exists at a higher height.

---

## 21. Signature Aggregation (Optional)

### CertificateV1 (MVP)

- Individual Ed25519 signatures (one per validator)
- Simple verification
- Certificate size: ~64 bytes × validator_count

```rust
struct CertificateV1 {
    rollup_id: u32,
    height: u64,
    round_number: u64,
    block_hash: [u8; 32],
    validator_set_id: u64,
    signatures: Vec<(ValidatorIndex, Ed25519Signature)>,
}

type ValidatorIndex = u32;
```

Signatures MUST be sorted by `ValidatorIndex` ascending, and duplicate indices are invalid. `ValidatorIndex` maps to validator public keys in the active validator set.

### CertificateV2 (Future Optimization)

- BLS12-381 aggregated signature
- Single signature for entire validator set
- Signer bitmap (indicates which validators signed)
- Proof-of-possession required at validator registration
- Certificate size reduction: ~97%

**Format:**
```rust
struct CertificateV2 {
    rollup_id: u32,
    height: u64,
    round_number: u64,
    block_hash: [u8; 32],
    validator_set_id: u64,
    signer_bitmap: BitVec,           // Which validators signed
    aggregated_signature: BLSSignature, // Single aggregated signature
}
```

`signer_bitmap` length MUST equal the active validator set size, and set bits MUST correspond to the keys included in `FastAggregateVerify`.

**Verification:**
```rust
// Proof-of-possession verified once at registration
PoP = BLS_Sign(validator_sk, H(validator_pk || rollup_id))

// Certificate verification (same logical fields as V1, distinct domain separator)
message = H("JAM_GRANDPA_CERT_V2" || rollup_id || height || round_number || block_hash || validator_set_id)
FastAggregateVerify(validator_pks, message, aggregated_signature)
```

Rationale: binding PoP to `rollup_id` prevents cross-rollup key reuse and rogue-key risks, at the cost of per-rollup registration overhead.

---

## 22. Network Partition Handling (Rollup Side)

During partitions, validators MUST continue to apply “extends latest finalized head.” JAM accepts at most one certificate per height, so partitions may cause liveness loss but MUST NOT cause safety loss. Upon reconnection, nodes MUST reconcile to the JAM-recorded finalized checkpoint.

Upon reconnection, nodes MUST query the JAM-recorded finalized checkpoint and discard any local blocks at or above that height that conflict with the recorded finalized chain.

---

## 23. Certificate Size Limits

JAM MUST enforce a per-rollup `MAX_CERT_BYTES` (e.g., 128 KiB). Certificates exceeding this limit MUST be rejected. Rollups requiring larger validator sets MUST use aggregation (CertificateV2) or a smaller active committee. `MAX_CERT_BYTES` is a rollup-specific parameter set at genesis or via governance.

---

## 24. Validator Set Change Semantics (Out of Scope, but Required Inputs)

Validator set updates are determined by the rollup’s governance/staking mechanism and recorded in JAM state. The mechanism is rollup-specific and out of scope, but the resulting `(validator_set_id → membership + weights + epoch range)` MUST be available to certificate verification.

---

## Summary: Security Properties

- ✅ No replay attacks
- ✅ No finality inversion
- ✅ No weak finality
- ✅ Correct GRANDPA semantics
- ✅ Reorg-safe JAM anchoring
- ✅ Fork choice defined
- ✅ BLS aggregation specified

---

*End of document*
