# JAM Private ZK Voting (Binary YES/NO) with Quorum + Majority Execution

This document specifies a JAM-native private voting system for **binary proposals** (YES/NO) and highlights how to stand up a **proof-of-concept (PoC)** implementation.

### Scope of the PoC
- Demonstrate end-to-end flow on a single JAM runtime with the EVM service providing the governance interface.
- Use a fixed validator set that also acts as the decryption committee.
- Assume honest-majority validators for threshold decryption during the PoC.

### Design Goals
- Individual votes (and voter identities) are not revealed on-chain.
- Total governance voting power is **fixed** (supply fixed at genesis / issuance rules).
- Votes are validated by **zero-knowledge proofs**.
- Tally is performed via **homomorphic encrypted accumulation** and **threshold decryption**.
- Proposal outcome is determined by **quorum + majority**, and **execution is deterministic** once passed.

---

## 1. Roles and Privacy Goals

### Roles
- **Voters**: Hold governance voting power and cast private votes.
- **Builders**: Relay and include unsigned vote extrinsics; can precheck proofs.
- **Validators**: Verify proofs, enforce nullifier uniqueness, update encrypted tallies.
- **Tally Committee**: A threshold set (typically validators) that jointly decrypts tallies at proposal close.

### Privacy Goals
- Hide who voted and how they voted.
- Hide vote weights per voter.
- Keep only final tallies (or at minimum pass/fail) public.

---

## 2. Governance Power Model (Fixed Total Supply)

Governance voting power is represented as shielded **governance notes** (UTXO-style).

### Governance Note
```

GovNote := (W, owner_pk, ρ)
cm      := Com(W, owner_pk, ρ)

```

- `W` is the vote weight/value of the governance note.
- `ρ` is note randomness.
- `cm` is inserted into a governance commitment tree.

**Fixed supply** means the protocol constrains creation of governance notes to a predefined issuance rule
(e.g., minted once at genesis, or minted only by a governance-controlled process).

---

## 3. Per-Proposal Double-Vote Prevention

Each governance note can vote **at most once per proposal**, enforced by a per-proposal nullifier:

```

nf := H(sk || pid || ρ)

```

- `sk` is the secret spending key corresponding to `owner_pk`.
- `pid` is the proposal identifier.
- `ρ` is the note randomness from the spent governance note.

Validators maintain a `NullifierSet[pid]` to reject repeated votes.

---

## 4. Proposal Lifecycle

### Proposal State
```

Proposal {
pid: Hash,
start_epoch: u64,
end_epoch: u64,

// static configuration for PoC
quorum: u128,          // absolute quorum threshold
majority_num: u64,     // numerator for majority fraction
majority_den: u64,     // denominator for majority fraction

// encrypted tallies
enc_yes: Ciphertext,
enc_no:  Ciphertext,

nullifiers: Set<Hash>,

status: Open | FinalizedPassed | FinalizedFailed | Executed,

// deterministic execution payload
action: Bytes,

// optional record of outcome
yes_total: Option<u128>,
no_total:  Option<u128>,
}

```

For the PoC, proposals can be created by a governance authority contract (or runtime method) that writes the parameters above a single time and cannot be mutated after `start_epoch`.

### Timing
- Proposal accepts votes when `start_epoch <= current_epoch < end_epoch`.
- Proposal is finalizable when `current_epoch >= end_epoch`.

---

## 5. Cryptographic Building Blocks

### 5.1 Additively Homomorphic Encryption (AHE)
Use an additive-homomorphic scheme (e.g., EC ElGamal) with group law `⊕`:

- `Enc(x) ⊕ Enc(y) = Enc(x + y)`
- `Enc(0)` is the neutral element.

Tallies are stored as ciphertexts:
- `enc_yes = Enc(Σ vote_weight_yes)`
- `enc_no  = Enc(Σ vote_weight_no)`

### 5.2 Threshold Decryption
A committee (typically validators) holds shares of a decryption key.
At finalization time, they produce threshold decryption shares to recover `(YES, NO)`.

---

## 6. Vote Extrinsic (Unsigned)

### Extrinsic
```

Vote {
pid: Hash,
anchor_root: Hash,
anchor_size: u64,

proof: Bytes,

nf: Hash,

delta_yes: Ciphertext,
delta_no:  Ciphertext,
}

```

- No signature; builder/validator does not learn voter identity from origin.
- `anchor_root/anchor_size` refer to the governance note tree state used for membership proving.

---

## 7. ZK Vote Proof Statement

Each vote proves:
- The voter owns a valid governance note of weight `W`.
- The governance note exists in the note tree at `anchor_root`.
- The voter casts **exactly one** of YES or NO with full weight `W`.
- The nullifier `nf` is correctly derived for this proposal.

### Public Inputs
```

x := (
pid,
anchor_root, anchor_size,
nf,
delta_yes, delta_no
)

```

### Private Witness
```

w := (
sk, owner_pk,
W, ρ,
merkle_path,
b
)

```

- `b ∈ {0,1}` is the private vote bit (YES=1, NO=0).

### Constraints
1) **Membership**
```

VerifyMerklePath(cm, merkle_path, anchor_root) = 1
where cm = Com(W, owner_pk, ρ)

```

2) **Authorization / Nullifier correctness**
```

owner_pk = PublicKey(sk)
nf = H(sk || pid || ρ)

```

3) **Binary vote**
```

b * (b - 1) = 0

```

4) **Correct encrypted deltas**
Define:
```

v_yes = b * W
v_no  = (1 - b) * W

```
Enforce:
```

delta_yes = Enc(v_yes)
delta_no  = Enc(v_no)

```

This ensures the full weight `W` is allocated privately to exactly one side.

---

## 8. Validator Processing Rules for `Vote`

Given `Vote{...}` and `Proposal(pid)`:

1) Check proposal is open:
```

start_epoch <= current_epoch < end_epoch

```

2) Verify `proof` against public inputs.

3) Check `nf ∉ NullifierSet[pid]`.

4) Update encrypted tallies:
```

enc_yes ← enc_yes ⊕ delta_yes
enc_no  ← enc_no  ⊕ delta_no

```

5) Insert `nf` into `NullifierSet[pid]`.

---

## 9. Quorum + Majority Rule (Chosen Rule)

At proposal close (after `end_epoch`), decrypt totals:
- `YES := Dec(enc_yes)`
- `NO  := Dec(enc_no)`

Define:
```

TOTAL_CAST := YES + NO

```

### Quorum condition
Proposal meets quorum iff:
```

TOTAL_CAST >= Q

```
where `Q` is the quorum threshold (configured per proposal or globally), e.g.:
- absolute weight: `Q = 10_000_000`
- or fraction of total supply: `Q = floor(q_pct * TOTAL_SUPPLY)`

### Majority condition
Proposal passes iff:
```

YES / TOTAL_CAST >= θ

```
where `θ` is majority threshold, e.g.:
- simple majority: `θ = 0.5` (strictly greater than 1/2; specify tie handling)
- supermajority: `θ = 2/3`

### Tie handling (recommended)
- If `YES == NO`, treat as **failed** unless policy requires otherwise.

### Final decision
```

Passed iff (TOTAL_CAST >= Q) AND (YES * denom >= θ_num * TOTAL_CAST)

```
Implement majority using integer arithmetic via `(θ_num, denom)` (e.g., 1/2, 2/3).

---

## 10. Finalization and Execution Extrinsics

### 10.1 FinalizeProposal (Permissionless)
```

FinalizeProposal { pid }

```

Semantics:
1) Require `current_epoch >= end_epoch`.
2) Collect threshold decryption shares and decrypt `(YES, NO)`.
3) Apply quorum + majority rule.
4) Set:
- `status = FinalizedPassed` or `FinalizedFailed`
- optionally store `yes_total` and `no_total`.

> Optional privacy enhancement: Instead of storing `(YES, NO)`, store only `passed: bool`
> plus a proof that the rule was applied correctly. (More complex; not required initially.)

### 10.2 ExecuteProposal
```

ExecuteProposal { pid }

```

Semantics:
1) Require `status == FinalizedPassed`.
2) Execute `action` deterministically in the relevant subsystem/service.
3) Set `status = Executed`.

Execution must be **purely deterministic** and independent of any voter identity or hidden data.

---

## 11. Anti-Spam and Fee Policy

- Vote extrinsics are unsigned; spam resistance should rely on:
  - a per-vote protocol fee (can be Z-space fee or proposal-specific fee),
  - and/or per-proposal rate limiting (optional),
  - and mandatory proof verification (invalid proofs dropped by builders/validators).

---

## 12. Security Notes

- **Double voting** prevented by `NullifierSet[pid]`.
- **Fixed supply** enforced by governance note issuance constraints.
- **Vote privacy** depends on:
  - ZK soundness (cannot forge votes),
  - encryption semantic security (ciphertexts reveal nothing),
  - threshold decryption security (no single party can decrypt early).
- **Censorship**: voters can submit to multiple builders; votes are unsigned and opaque.

---

## 12.1 PoC Demo Flow

1) **Genesis bootstrapping**
   - Mint a fixed set of governance notes to test accounts; populate the commitment tree in DA.
   - Publish validator encryption keys and decryption share verification keys in genesis config.
2) **Proposal creation**
   - Call `create_proposal` with a short `start_epoch`/`end_epoch` window (e.g., 10–20 blocks) and an `action` that emits an event so execution is observable in the demo.
3) **Client proof generation**
   - Fetch `anchor_root/anchor_size` via RPC.
   - Create a vote proof using the Groth16 circuit; encrypt the vote weight into `(delta_yes, delta_no)`.
4) **Vote submission**
   - Send unsigned `vote` extrinsics through a builder; validators update encrypted tallies and nullifiers.
5) **Finalize**
   - After `end_epoch`, run the validator helper to submit decryption shares and call `finalize`.
6) **Execute**
   - Invoke `execute` and verify the deterministic action output (event/log or state change).
7) **Test assertions**
   - Confirm: (a) duplicate nullifiers are rejected, (b) quorum/majority gates are enforced, (c) ciphertexts never reveal per-vote data.

---

## 13. Implementation Notes (Practical Defaults)

- Proof system: Groth16 (BN254) for v1, with a migration path later.
- In-circuit hash: Poseidon.
- Encryption: EC ElGamal with threshold decryption among validators.

---

## 13.1 PoC Implementation Checklist

**Smart contracts / runtime calls**
- `create_proposal(pid, start_epoch, end_epoch, quorum, majority_num, majority_den, action)`: stores proposal params and initializes ciphertexts to `Enc(0)`.
- `vote(Vote)`: unsigned call that enforces proposal window, proof verification, nullifier uniqueness, and encrypted tally updates.
- `finalize(pid)`: permissionless call that requests threshold decryption shares from validators and writes `YES/NO` results.
- `execute(pid)`: triggers the deterministic `action` payload when `status == FinalizedPassed`.

**Circuits and cryptography**
- Implement Groth16 circuit described in §7 using Poseidon hash and EC ElGamal encryption gadgets.
- **Circuit constraints target**: <100k constraints for fast proving (~1-2s on consumer hardware)
- Generate a **single trusted setup** for the vote circuit; document toxic waste handling for the PoC.
- Provide a CLI to produce proofs from a governance note witness and serialize to the `Vote` extrinsic format.

**Off-chain components**
- Lightweight **builder pre-checker** that rejects malformed votes before gossiping.
- **Validator helper** that watches proposals past `end_epoch`, aggregates decryption shares, and submits `finalize` calls.
- **Integration test harness** that mints governance notes, casts votes for both sides, and asserts quorum/majority outcomes.

**Storage choices**
- Store the governance note commitment tree in DA; expose `anchor_root` and `anchor_size` via RPC for clients generating proofs.
- Maintain `NullifierSet` as a sparse Merkle structure so inclusion/exclusion proofs can be added later if needed.

**Performance targets for PoC**
- Vote proof generation: <5s on standard laptop (target: 1-2s)
- Vote verification: <50ms in validator
- Threshold decryption: <500ms per validator share
- Memory usage: <1GB for 1000 governance notes
- Storage: <10MB for complete voting history (100 proposals)

**Measurement + bounds for the PoC**
- Enforce `anchor_size <= 2^16` and reject proofs larger than 192KB to keep validator memory predictable.
- Add a `vote-bench` CLI that generates a synthetic note, produces a proof, and logs proving/verification latencies against the targets above.
- Capture Groth16 constraint counts in CI and fail if they exceed 110k to guard against accidental circuit bloat.

---

## 14. Summary

This design enables:
- private, weighted YES/NO voting,
- enforceable one-vote-per-note-per-proposal,
- quorum + majority decision rules,
- deterministic proposal execution,
- and compatibility with builder-relayed, unsigned extrinsics in JAM.
