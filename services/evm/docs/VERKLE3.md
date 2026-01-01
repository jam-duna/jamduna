# Verkle Witness v2: Single-Proof Transition Verification

## Goal

Reduce witness **verification cost** by ~2× while preserving the security guarantees between a **malicious builder** and the **guarantor**. We do this by collapsing the current **pre-state + post-state** witness into a **single transition witness** that still proves:

1. **All reads** came from the claimed pre-state root.
2. **All writes** are the exact result of re-execution.
3. The **post-state root** is the correct commitment after applying those writes.

**Important**: This optimization reduces **IPA verification time** (one transcript instead of two), but proof size remains substantial because post-state path commitments must still be supplied.

## Key Idea

Instead of verifying two independent witnesses, we verify **one multiproof** that binds **both old and new values** for every accessed key. This yields a **transition proof**: a single IPA verification that simultaneously proves the pre-state and post-state evaluations for all touched leaves.

Conceptually, each touched key contributes:

- **Read-only** key: `(old_value)`
- **Write** key: `(old_value, new_value)`

The proof is constructed so that **both values are bound to their respective roots** inside the *same* IPA transcript, amortizing all expensive verification work.

---

## Architectural Changes

### 1) Replace `pre_witness` + `post_witness` with a **Transition Witness**

A new witness format includes a single multiproof for all touched keys and both roots:

```
TransitionWitness {
  pre_state_root: [u8; 32]
  post_state_root: [u8; 32]

  // Compact proof that binds BOTH roots at once
  proof_data: Vec<u8>

  // Access list entries (BAL)
  // For each key, we include old and optional new value
  entries: Vec<TransitionEntry>
}

TransitionEntry {
  key: [u8; 32]
  old_value: [u8; 32]
  new_value: Option<[u8; 32]>  // present only for writes
}
```

### 2) BAL-Driven Proof Construction

The refine execution already computes:

- **BAL (Block Access List)**
- **BAL hash**
- **State diff (write set)**

We turn that into a **Transition BAL (TBAL)**:

```
TBAL = [ (key, old_value, new_value?) ... ]
```

This is strictly stronger than the current read-only witness, because it binds every touched key to both the **old** and **new** values.

### 3) Single IPA Verification

We reuse the existing Verkle multiproof pipeline but extend it to emit **two sets of evaluations** in one transcript:

- Evaluations at `pre_state_root`
- Evaluations at `post_state_root`

This is possible because IPA verification is already a **batch commitment check**. We simply expand the list of `(commitment, z, y)` pairs to include both roots while using a **single proof transcript**.

**Critical requirement for write keys**: The proof must include **path commitments for both roots**:
- Pre-state path commitments (to verify old values for **all** BAL keys)
- Post-state path commitments (to verify new values for **write keys only** and authenticate the post-root)

Read-only keys (no writes) only require pre-state path commitments. Without post-state path commitments for writes, the guarantor has no cryptographic way to verify that `post_state_root` is correct—it can compute a candidate root using deltas, but cannot prove it matches the builder's claimed root.

**Outcome**: one IPA verification call instead of two, with shared path reconstruction and commitment parsing.

---

## Witness Sufficiency Rules

To make a single transition witness complete and safe:

1. **BAL must cover all write targets**
   - Any write key must appear in the BAL (read set).
   - For new keys, the BAL must include a proof-of-absence (old_value = 0).
   - This prevents the builder from omitting a write target from the witness.

2. **Witness must provide per-node child values and path commitments** at touched indices
   - For pre-state: path commitments proving old values under `pre_state_root` (for **all** BAL keys)
   - For post-state: path commitments proving new values under `post_state_root` (for **write keys only**)
   - Read-only keys only require pre-state path commitments
   - These commitments enable cryptographic verification of both roots via dual-root opening

3. **BAL hash binds the access set**
   - The service should include the BAL hash in the work package to prevent the builder from omitting keys.
   - Guarantor recomputes BAL from execution; mismatch invalidates the work package.

> In Ethereum execution, every write is semantically preceded by a read of the old value (e.g. SSTORE, balance/nonce updates, account creation checks). The BAL can therefore be defined to include every write-target as a read, which guarantees the witness is sufficient.

---

## BAL Hash Construction

Use a domain-separated hash over sorted lists:

```
BAL_read_hash  = H("BAL_READ"  || concat(sorted(read_key || read_value)))
BAL_write_hash = H("BAL_WRITE" || concat(sorted(write_key || write_value)))
```

- Sort by key lexicographically (same order as Verkle proof extraction).
- Include full value bytes for canonicalization.
- Hash function should match the system's existing DA/commitment hash.

For the transition witness model, we compute a combined:

```
TBAL_hash = H("TBAL" || concat(sorted(key || old_value || new_value?)))
```

This binds the full transition set (reads + writes) to the work package.

---

## Transition Proof Semantics

### For Read-Only Keys

- The transition witness proves `old_value` against `pre_state_root`.
- No post-state evaluation is needed.

### For Write Keys

- The witness proves `old_value` against `pre_state_root`.
- The witness proves `new_value` against `post_state_root`.

This binds **each write** to the exact state change committed in the post-state root.

---

## Security Argument

### Threat Model

- **Builder is malicious** and may try to forge reads or writes.
- **Guarantor** re-executes deterministically but relies on witness proofs.

### Why a Single Proof Is Safe

1. **Read integrity**: `old_value` is proven against `pre_state_root` for every touched key.
2. **Write integrity**: re-execution determines `new_value`. The proof binds `new_value` to `post_state_root`.
3. **Root binding**: both roots appear as explicit commitments in the same IPA transcript, so the proof cryptographically ties both sets of evaluations to their claimed roots.

If a builder tries to:

- **Forge a read** → fails pre-root proof.
- **Forge a write** → fails post-root proof.
- **Forge post-state root** → fails because `new_value` evaluations are bound to that root in the same proof.

Thus, correctness is preserved even with a single proof verification.

---

## Concrete Flow (Builder → Guarantor)

### Builder

1. Execute transactions, producing:
   - BAL (all reads, including write targets)
   - State diff (writes)
2. Build TBAL: `(key, old_value, new_value?)` for each touched key.
3. Compute `TBAL_hash = H("TBAL" || concat(sorted(key || old_value || new_value?)))`.
4. Construct a **single transition proof** with:
   - all `old_value` evaluations at `pre_state_root` (for all BAL keys)
   - all `new_value` evaluations at `post_state_root` (for write keys only)
5. Submit work package containing:
   - `TransitionWitness` (proof data)
   - `TBAL_hash` (binding commitment to the access set)
   - `pre_state_root` and `post_state_root`

### Guarantor

1. Verify **one** transition proof:
   - Ensures reads are valid under `pre_state_root`.
   - Ensures writes are valid under `post_state_root`.
2. Re-execute transactions using TBAL cache:
   - Every read must match `old_value`.
   - Every write must match `new_value`.
3. Recompute `TBAL_hash'` from re-execution.
4. **Check `TBAL_hash' == TBAL_hash`** (detects omitted keys or forged values).
5. Accept the work package if all checks pass.

---

## Gas Optimization Impact

Compared to two full proofs:

| Step | Old Model | New Model | Savings | Notes |
|------|-----------|-----------|---------|-------|
| Proof parsing | 2× | ~1.5× | **~1.3×** | Proof size is ~1.3-1.7× due to dual-root paths |
| Tree reconstruction | 2× | ~1.5× | **~1.3×** | Scales with commitment count (more for dual-root) |
| IPA verification | 2× | **1×** | **~2×** | Single transcript dominates savings |
| Root commitment checks | 2× | 2× | 1× | Must verify both pre and post roots |

Because the IPA verification dominates the gas cost (~80-90% of total), this yields **~1.6-1.8× witness verification cost savings** overall, despite proof size increasing to ~1.3-1.7×.

### Proof Size Overhead

**Important**: While verification cost is halved, proof size is **not** reduced by 2×:

- **Path commitments**: Must include commitments for both pre-state and post-state paths
- **Deduplication**: Shared stems and internal nodes are included only once, providing some compression
- **Typical overhead**: Proof size is roughly **1.3-1.7×** a single witness (not 2×) due to stem overlap in sequential state transitions

The verification cost savings (gas) are the primary benefit, not proof size reduction.

---

## Proof Construction Details

### Dual-Root Opening (Primary Approach)

The transition witness uses a **single IPA transcript** to prove evaluations against **both roots simultaneously**. This is the core mechanism that enables ~2× IPA verification savings.

**How it works**:

1. **Extract evaluations for pre-state root**:
   - For all BAL keys, prove `old_value` against `pre_state_root`
   - Include all path commitments from touched stems

2. **Extract evaluations for post-state root**:
   - For **write keys only**, prove `new_value` against `post_state_root`
   - Include post-state path commitments for modified stems
   - Read-only keys skip post-state evaluation

3. **Batch into single IPA transcript**:
   - Combine both evaluation sets: `[(C_pre, z_i, old_i), (C_post, z_j, new_j)]`
   - Generate **one IPA proof** binding all evaluations
   - Deduplicate shared stem commitments (pre/post paths often overlap)

This yields a single proof that cryptographically binds both roots while amortizing the dominant cost (IPA verification).

**Commitment delta formula** (conceptual):

For implementers, it's useful to understand that Verkle internal node updates follow:

```
C' = C + Σ (new_i - old_i) * G_i
```

Where `C` is the pre-state commitment, `new_i - old_i` are value deltas, and `G_i` are basis points. However, this formula is **not used in verification**—the dual-root opening proves both `C` (pre) and `C'` (post) cryptographically via IPA evaluations.

### Alternative: Delta-Update Witness (Future Optimization)

An alternative approach that **replaces** the dual-root opening:

- Include **only** pre-state path commitments in the witness
- Guarantor computes `C' = C + Σ (new_i - old_i) * G_i` locally using deltas
- Guarantor derives `post_state_root` from updated commitments
- **No cryptographic proof of post-root** (relies on deterministic execution)

**Trade-off**: Saves proof size (~30-40% smaller) but requires the guarantor to trust their local root computation. This is **not recommended** for the adversarial builder/guarantor model unless combined with fraud proofs.

---

## Implementation Notes

### Proof Encoding

`proof_data` can remain compatible with the existing compact proof format by extending the multiproof builder to:

- Emit **both** root evaluations (pre + post) in the evaluation list.
- Deduplicate shared internal commitments as usual.
- Use the IPA batch mechanism described above.

### State Diff Recording

The existing write set is already produced by refine. We simply treat it as the authoritative `new_value` set for the transition witness and keep it in the Verkle trie as usual.

**Important**: State diff should be **sorted by key** to align with Verkle witness ordering requirements.

### Compatibility

This design does **not** require changes to Verkle cryptography. It only changes:

- **which evaluations are bundled into one proof**
- **how the witness is structured**

**Guarantor execution stays cache-only**:
- Re-execution uses only witness values (no external state access)
- All reads must match `old_value` from the transition witness
- All writes must match `new_value` computed deterministically

---

## Optional Compression (Gas Optimization)

The transition witness is the only verified proof, so we can focus gas savings here:

### Stem Deduplication
- Already required by go-verkle format
- Internal nodes shared between pre/post evaluations are included only once

### Batch Witnesses Across Contiguous Rollup Blocks
- If multiple work packages are submitted together, allow a *single aggregated transition witness* covering all transitions in order
- Each work package still has its own TBAL hash and state diffs
- This amortizes proof parsing and IPA verification across multiple blocks

### Path Compression
- For long sequences of writes to the same account/contract, the path commitments can be heavily deduplicated
- This is especially effective for contract-heavy workloads (e.g., DeFi protocols)

---

## Summary

By moving from **two independent witnesses** to a **single transition witness**, we:

- **Cut witness verification cost roughly in half** (2× savings in IPA verification gas)
- **Preserve all security guarantees** (reads + writes + root binding via authenticated path commitments)
- **Reuse existing BAL and state diff outputs** from refine
- **Avoid any new cryptographic primitives** (uses existing IPA batch verification)

### Key Trade-offs

**Benefits**:
- ~2× reduction in witness verification gas cost (IPA verification dominates)
- Single proof transcript simplifies verification logic
- Shared stem deduplication provides some proof size compression

**Costs**:
- Proof size is ~1.3-1.7× a single witness (not halved) due to dual-root path commitments
- Builder must construct evaluations for both roots
- Slightly more complex proof construction (batch dual-root evaluations)

This provides a clean, practical path to single-proof verification in JAM's EVM rollup model with honest accounting of the verification cost vs. proof size trade-off.