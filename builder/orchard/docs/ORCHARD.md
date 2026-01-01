# ORCHARD.md - Orchard-Only Circuits and Extrinsics (Zcash)

This document replaces all Railgun-specific protocol features with the Zcash Orchard
protocol. Orchard is the only source of truth for circuits, data formats, and
transaction semantics. We do not extend Orchard or introduce any new public inputs.

## Overview
- Circuit system: Zcash Orchard (Halo2/IPA on Pasta curves)
- Circuit scope: Orchard Action circuit (one spend + one output per action)
- Proofs: Orchard bundle proof (single proof covering N actions)
- Serialization: Zcash v5 Orchard bundle encoding (byte-for-byte compatible)
- Circuit size: fixed by Orchard, not configurable

## Authoritative Sources
- Zcash Protocol Spec, Orchard section (transaction encoding and consensus rules)
- `orchard` crate (used by `librustzcash`)
- `zcash_primitives::transaction::components::orchard::{read_v5_bundle, write_v5_bundle}`

## Proof System and Parameters
- Library: `orchard` crate (Halo2, IPA commitment scheme)
- Curves: Pallas/Vesta (cycle-friendly Halo2 pairing)
- Hashes: Orchard Sinsemilla and domain-separated group hashes
- Circuit size: `K = 11` (constant in `orchard` crate)
- Params: `Params::new(K)`; PK/VK derived from the circuit code
- VK selection: single fixed VK for Orchard Action circuit (no `vk_id` routing)

## Orchard Data Model
- Keys: spending key, full viewing key, incoming viewing key, diversified address
- Notes: Orchard note structure and note encryption (as per spec)
- Commitments:
  - `cm` is the note commitment computed by Orchard note commitment gadget
  - `cmx` is the extracted note commitment used in public inputs
- Nullifiers:
  - `nf_old` computed by Orchard nullifier derivation
  - Each action spends one old note and creates one new note

## Merkle Tree
- Depth: `MERKLE_DEPTH_ORCHARD = 32`
- Hash: Orchard Sinsemilla with Orchard hash domains
- Anchor: 32-byte Orchard commitment tree root

## Orchard Action Circuit
Each action proves:
- Membership of the spent note in the Orchard commitment tree under `anchor`
- Nullifier correctness (`nf_old`) for the spent note
- Spend authorization key randomization (`rk`)
- New note commitment correctness (`cmx`)
- Per-action value net is enforced through `cv_net`; bundle-level `value_balance`
  is enforced by the binding signature and bundle verification
- If spends or outputs are disabled by flags, the circuit enforces dummy notes

### Witness Inputs (per action)
- Merkle path and position for the old note
- Old note fields and witnesses (`rho_old`, `psi_old`, `rcm_old`, `cm_old`, etc.)
- New note fields and witnesses (`rho_new`, `psi_new`, `rcm_new`)
- Spend authorization secret (`alpha`, `ak`, `nk`, `rivk`)
- Value commitment trapdoor (`rcv`)

### Public Inputs (per action, fixed layout)
The Orchard circuit exposes 9 public inputs per action. The layout is fixed and
matches the `orchard` crate:
```
[0] anchor      (Orchard tree root)
[1] cv_net_x    (ValueCommitment x-coordinate)
[2] cv_net_y    (ValueCommitment y-coordinate)
[3] nf_old      (Nullifier)
[4] rk_x        (Randomized spend auth key x-coordinate)
[5] rk_y        (Randomized spend auth key y-coordinate)
[6] cmx         (Extracted note commitment)
[7] enable_spend  (0 or 1)
[8] enable_output (0 or 1)
```
Instances are encoded as Vesta scalars (the scalar field of Vesta, equivalent
to the Pallas base field).

### Instance Mapping
Given an Orchard bundle (below), each action instance is derived as:
- `anchor` = bundle.anchor
- `cv_net` = action.cv_net
- `nf_old` = action.nullifier
- `rk` = action.rk
- `cmx` = action.cmx
- `enable_spend` = bundle.flags.spends_enabled
- `enable_output` = bundle.flags.outputs_enabled

## Orchard Bundle Semantics
An Orchard bundle is a non-empty list of actions plus bundle-level metadata:
- `actions`: Non-empty list of Orchard actions
- `flags`: spend/output enable flags
- `value_balance`: net value moved out of Orchard pool (i64)
- `anchor`: Orchard commitment tree root
- `proof`: Orchard bundle proof (single proof for all actions)
- `spend_auth_sigs`: per-action spend authorization signatures
- `binding_sig`: bundle-level binding signature

The bundle proof verifies all action circuits in one proof, using the action
instances derived above.

## Extrinsic Layout (Orchard Bundle, Zcash v5 encoding)
We use the Zcash v5 Orchard bundle encoding directly. This is the canonical
layout used by `read_v5_bundle`/`write_v5_bundle` in `zcash_primitives`.

### Bundle Encoding
```
OrchardBundleV5 =
  CompactSize(action_count)
  ActionWithoutAuth[action_count]
  flags (1 byte)
  value_balance (i64, little-endian)
  anchor (32 bytes)
  CompactSize(proof_len)
  proof_bytes[proof_len]
  spend_auth_sig[action_count] (each 64 bytes)
  binding_sig (64 bytes)
```

If `action_count == 0`, the bundle is absent (no further bytes).

### ActionWithoutAuth Encoding
```
ActionWithoutAuth =
  cv_net (32 bytes)        // ValueCommitment (Pallas point encoding)
  nf_old (32 bytes)        // Nullifier
  rk (32 bytes)            // SpendAuth verification key
  cmx (32 bytes)           // Extracted note commitment
  note_ciphertext:
    epk_bytes (32 bytes)
    enc_ciphertext (580 bytes)
    out_ciphertext (80 bytes)
```

### Flags
```
flags bit 0: spends_enabled
flags bit 1: outputs_enabled
all other bits must be zero
```

## JAM Extrinsics (Orchard-Only)
We replace Railgun-specific extrinsics with a single Orchard bundle submission.

### SubmitOrchard (single bundle)
```
SubmitOrchard {
  bundle_bytes: OrchardBundleV5
  fee: u64
  gas_limit: u64
}
```

### BatchSubmitOrchard (optional)
If batching is desired at the chain level, use a list of bundles with the same
encoding:
```
BatchSubmitOrchard {
  bundles: [OrchardBundleV5]
}
```

## Verification Rules (Refine)
For each submitted bundle:
1) Decode `OrchardBundleV5` using the Zcash encoding rules.
2) Check `fee >= min_fee` and `gas_min <= gas_limit <= gas_max`.
3) Enforce `value_balance == fee` (bundle-only mode forbids withdrawals).
4) For each action, derive the 9-field instance from action + flags + anchor.
5) Verify the Orchard proof against all instances using the Orchard VK (K=11).
6) Verify all spend authorization signatures and the binding signature.
7) Apply state transitions:
   - Insert nullifiers and update `nullifier_root`
   - Append new commitments and update `commitment_root` and `commitment_size`
   - Update `fee_root` with the builder fee credit

## Testing
- Use Orchard test vectors from the `orchard` crate.
- Add proof verification tests that use the canonical bundle encoding.
- No Railgun-specific circuit tests or mock proofs are retained.

## JAM Integration Architecture

### Three-Role Separation
The JAM integration follows a builder/guarantor model with privacy preservation:

| Role | Knows Identity | Pays Cost | Authorizes |
|------|---------------|-----------|------------|
| User | Yes | No | Yes (via Orchard proof) |
| Builder | No | Yes | No |
| Validator | No | No | Verifies proof only |

### Builder-Sponsored Execution
- Users submit Orchard bundles to builders without revealing identity
- Builders pay for JAM execution costs and are reimbursed via fee mechanism
- No mempool identity leakage since users never interact with JAM directly

## JAM Service State

The JAM service maintains minimal consensus-critical state for Orchard:

```rust
struct OrchardState {
    // Orchard commitment tree (single asset)
    commitment_root: [u8; 32],    // Sinsemilla Merkle root, depth 32
    commitment_size: u64,         // Tree size for deterministic appends
    nullifier_root: [u8; 32],     // Nullifier set root (builder-maintained tree)
    fee_root: [u8; 32],           // Builder fee ledger root

    // Parameters
    min_fee: u64,                 // Minimum transaction fee
    gas_min: u64,                 // Minimum gas limit
    gas_max: u64,                 // Maximum gas limit
}
```

Builders store the full nullifier set and fee ledger off-chain; their Merkle
roots are committed in JAM state as `nullifier_root` and `fee_root`.

## Witness-Based Execution Model

### Security Architecture
JAM uses a **witness-based execution model** where builders provide cryptographic
proofs of state transitions without requiring JAM to maintain full service state:

1. **Builder Phase**:
   - Executes Orchard transactions
   - Records state access patterns (commitment tree reads, nullifier checks)
   - Generates cryptographic witnesses for all reads/writes

2. **Guarantor Phase**:
   - Verifies pre-witnesses against pre-state root
   - Re-executes transactions deterministically using cache-only backend
   - Verifies post-witnesses against computed post-state

### State Access Requirements
**Pre-witness (must be proven via Merkle proofs):**
- `commitment_root`, `commitment_size` - for anchor validation
- `nullifier_root` and non-membership proofs for each `nf_old`
- `min_fee`, `gas_min`, `gas_max` - for fee policy validation
- `fee_root` entry for the submitting builder

**Post-witness (must be proven after execution):**
- Updated `commitment_root`, `commitment_size` - after tree appends
- Updated `nullifier_root` with new spent markers
- Updated `fee_root` with fee accumulation

### Security Properties
- **State Integrity**: Merkle witnesses cryptographically bind reads/writes to state roots
- **Execution Determinism**: Identical transactions + pre-state → identical post-state
- **Non-Forgery**: Invalid state transitions cannot produce valid witnesses
- **Byzantine Tolerance**: Up to 1/3 builders can be malicious without compromising safety

## Cross-Service Transfers (Optional)

Cross-service transfers are not supported in bundle-only mode because Orchard
bundles do not carry public recipient data. To support deposits/withdrawals,
either:
1) Switch to full Zcash v5 transaction encoding (transparent inputs/outputs are
   included in the sighash and binding signature), or
2) Define a separate bridge protocol that cryptographically binds the recipient
   to a bundle hash.

## Implementation Integration

### Refine (Stateless Verification)
```rust
pub fn refine_orchard(
    extrinsic: &SubmitOrchard,
    witnesses: &StateWitnesses
) -> Result<WriteIntents> {
    // 1. Witness validation
    verify_commitment_tree_witnesses(&witnesses)?;
    verify_nullifier_witnesses(&witnesses)?;

    // 2. Fee/gas policy
    verify_fee_and_gas(extrinsic, &witnesses)?;

    // 3. Orchard verification
    let bundle = OrchardBundle::decode(&extrinsic.bundle_bytes)?;
    verify_orchard_bundle(&bundle)?;  // Uses orchard crate
    verify_value_balance_is_fee(&bundle, extrinsic.fee)?;

    // 4. State transition computation
    let write_intents = compute_state_updates(&bundle, &witnesses)?;
    Ok(write_intents)
}
```

### Accumulate (State Application)
```rust
pub fn accumulate_orchard(
    write_intents: &WriteIntents
) -> Result<StateUpdates> {
    // Apply all write intents to JAM state
    apply_commitment_tree_updates(&write_intents.tree_updates)?;
    apply_nullifier_updates(&write_intents.nullifier_updates)?;
    apply_fee_updates(&write_intents.fee_updates)?;
    Ok(StateUpdates::success())
}
```

## Orchard Key Generation (Single Step)
Orchard has a single Action circuit with a fixed size (`K = 11`). Keygen should
be a single step that produces one PK/VK pair for the Orchard Action circuit.

### Production Keygen Command (long timeout)
```
timeout 6h cargo run --manifest-path builder/orchard/Cargo.toml --bin orchard-keygen -- --k 11 --out keys/orchard
```

Expected outputs:
- `keys/orchard/orchard.vk`
- `keys/orchard/orchard.pk`

Notes:
- `K = 11` is fixed by the Orchard circuit; do not change it.
- The command assumes an `orchard-keygen` binary that wraps the Orchard circuit
  keygen APIs. If it does not exist yet, implement it before running this step.
- Last run generated `keys/orchard/orchard.vk` and `keys/orchard/orchard.pk`.
