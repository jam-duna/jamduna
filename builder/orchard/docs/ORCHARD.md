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
The Orchard NU5 circuit exposes **9 public inputs per action** (not 7, not 44). The layout is fixed and enforced by the refiner:
```
[0] anchor        (Orchard tree root, 32 bytes)
[1] cv_net_x      (Net value commitment x-coordinate, field element)
[2] cv_net_y      (Net value commitment y-coordinate, field element)
[3] nf_old        (Nullifier, 32 bytes)
[4] rk_x          (Randomized verification key x-coordinate, field element)
[5] rk_y          (Randomized verification key y-coordinate, field element)
[6] cmx           (Extracted note commitment, 32 bytes)
[7] enable_spend  (Spend enabled flag, 0 or 1)
[8] enable_output (Output enabled flag, 0 or 1)
```

**IMPORTANT**:
- ✅ **Correct layout**: 9 fields per action (Orchard NU5)
- ❌ **Incorrect layout**: 44 fields (Railgun, rejected by refiner)
- ❌ **Incorrect layout**: 7 fields with compressed `cv_net`/`rk` (rejected by refiner)

Instances are encoded as 32-byte field elements (Pallas base field / Vesta scalar field).

### Instance Mapping
Given an Orchard bundle (below), each action instance is derived as:
- `anchor` = bundle.anchor
- `cv_net_x/cv_net_y` = action.cv_net x/y coordinates
- `nf_old` = action.nullifier
- `rk_x/rk_y` = action.rk x/y coordinates
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

## JAM Extrinsics

The Orchard service uses **three extrinsics per work package**:

1. **PreStateWitness** - Merkle proofs for nullifier absence
2. **PostStateWitness** - Optional consistency checks for post-state
3. **BundleProof** - ZK proof + public inputs + Orchard bundle bytes

**Key Design**: CompactBlock is **derived from bundle_bytes**, not submitted separately. This eliminates redundancy since the bundle already contains all nullifiers and commitments.

### Extrinsic Definitions

```rust
pub enum OrchardExtrinsic {
    /// Submit pre-state witness with Merkle proofs for reads
    PreStateWitness {
        witness_bytes: Vec<u8>,
    },

    /// Submit post-state witness with Merkle proofs for writes
    PostStateWitness {
        witness_bytes: Vec<u8>,
    },

    /// Submit the bundle proof, public inputs, and Orchard bundle
    /// CompactBlock is derived from bundle_bytes during verification
    BundleProof {
        vk_id: u32,
        public_inputs: Vec<[u8; 32]>,
        proof_bytes: Vec<u8>,
        bundle_bytes: Vec<u8>,  // Zcash v5 Orchard bundle
    },
}
```

### Bundle Bytes → CompactBlock Derivation

The refiner parses `bundle_bytes` and extracts:
- **Nullifiers**: From `action.nullifier` fields
- **Commitments**: From `action.cmx` + `action.epk_bytes` + `action.enc_ciphertext`

This derived CompactBlock is used for state transitions (nullifier set updates, commitment tree appends).

## Verification Rules (Refine)
For each work package:
1) **Parse bundle bytes**: Decode `OrchardBundleV5` from BundleProof extrinsic
2) **Derive CompactBlock**: Extract nullifiers and commitments from parsed bundle
3) **Fee/gas validation** (SECURITY FIX #4):
   - Check `fee >= MIN_FEE_PER_ACTION × action_count`
   - Check `estimated_gas <= MAX_GAS_PER_WORK_PACKAGE`
   - Enforce `value_balance > 0` (positive means fee payment)
4) **Public input layout validation** (SECURITY FIX #3):
   - Validate public inputs use Orchard NU5 layout (9 fields per action)
   - Fields per action: `anchor`, `cv_net`, `nf_old`, `rk`, `cmx`, `enable_spend`, `enable_output`
   - Total fields must equal `9 × action_count`
5) **Bundle consistency validation** (SECURITY FIX #2):
   - Verify `anchor` from bundle matches `anchor` from public inputs
   - Verify `cv_net` values from bundle match public inputs (field 1 of each action)
   - Verify nullifiers match public inputs (field 2 of each action)
   - Verify `rk` values from bundle match public inputs (field 3 of each action)
   - Verify commitments match public inputs (field 4 of each action)
6) **Signature verification** (SECURITY FIX #1):
   - Compute sighash: ZIP-244 bundle commitment (txid_digest)
   - Verify spend authorization signature for each action (RedPallas)
   - Verify binding signature for bundle (RedPallas)
   - Binding signature includes value_balance term: `bvk = Σ cv_net + value_balance * V`
7) **ZK proof verification**:
   - Verify Orchard proof against all public inputs using VK (K=11)
8) **Fee intent emission (delayed JAM transfer)**:
   - Emit a `FeeTally` intent that records `value_balance` (fee amount)
   - Accumulate executes the JAM transfer to service 0 (JAM core) **after** validating old roots, preventing malicious builders from triggering payments on rejected work packages
9) **State transitions**:
   - Insert nullifiers from derived CompactBlock and update `nullifier_root`
   - Append new commitments from derived CompactBlock and update `commitment_root` and `commitment_size`

**Security Properties**:
- ✅ Enforces ZIP-244 signature validity (sighash = bundle commitment)
- ✅ Prevents fee tampering (fee validation + binding signature over value_balance)
- ✅ Prevents economic DOS (minimum fee enforcement)
- ✅ Prevents resource exhaustion (maximum gas enforcement)
- ✅ Prevents parameter tampering (bundle consistency checks)
- ✅ Enforces correct circuit layout (Orchard NU5, not Railgun)
- ✅ Prevents fee transfer on rejected work packages (fee transfer executes in accumulate after old-root validation)

## Security Implementation

### Critical Security Fixes (2026-01-02)

All 4 critical security vulnerabilities identified in the security audit have been fixed:

#### Fix #1: RedPallas Signature Verification ✅
**Files**: [`services/orchard/src/signature_verifier.rs`](../../../services/orchard/src/signature_verifier.rs), [`services/orchard/src/bundle_parser.rs`](../../../services/orchard/src/bundle_parser.rs)

- Implements complete RedPallas signature verification (Schnorr over Pallas curve)
- Verifies spend authorization signature for each action
- Verifies binding signature for the bundle
- Computes sighash as the ZIP-244 bundle commitment (txid_digest)
- **Replay Protection**: Bundle-specific sighash prevents signature reuse across different bundles (does not bind to work package hash)
- **Prevents**: Unauthorized spending, cross-bundle signature replay

**Signature Verification Equation**:
```
s * B == R + c * VK

where:
  s = signature scalar (32 bytes)
  B = Pallas generator point
  R = signature point (32 bytes)
  c = Blake2b challenge hash
  VK = verification key (rk for spend auth, cv sum for binding)
```

#### Fix #2: Public Input Policy Enforcement ✅
**Files**: [`services/orchard/src/refiner.rs:586-625`](../../../services/orchard/src/refiner.rs#L586-L625)

- Validates bundle anchor matches public input anchor
- Validates rk x/y values from bundle match public inputs (fields 4-5)
- Validates nullifiers match public inputs (field 3)
- Validates commitments match public inputs (field 6)
- **Prevents**: Parameter tampering, fee forgery, proof-to-payload mismatch

#### Fix #3: Orchard NU5 Layout Enforcement ✅
**Files**: [`services/orchard/src/vk_registry.rs`](../../../services/orchard/src/vk_registry.rs), [`services/orchard/src/refiner.rs:507-584`](../../../services/orchard/src/refiner.rs#L507-L584)

- Enforces correct Orchard NU5 layout: 9 fields per action
- Rejects incorrect Railgun layout (44 fields)
- Validates total fields = 9 × action_count
- **Prevents**: Layout mismatch exploits, index confusion attacks

**Orchard NU5 Layout**:
```rust
pub const ORCHARD_NU5_ACTION_LAYOUT: [&str; 9] = [
    "anchor",        // Field 0: Commitment tree root
    "cv_net_x",      // Field 1: Net value commitment x-coordinate
    "cv_net_y",      // Field 2: Net value commitment y-coordinate
    "nf_old",        // Field 3: Nullifier of spent note
    "rk_x",          // Field 4: Randomized verification key x-coordinate
    "rk_y",          // Field 5: Randomized verification key y-coordinate
    "cmx",           // Field 6: Extracted commitment of new note
    "enable_spend",  // Field 7: Spend enabled flag
    "enable_output", // Field 8: Output enabled flag
];
```

#### Fix #4: Fee/Gas Bounds Enforcement ✅
**Files**: [`services/orchard/src/refiner.rs:33-37,438-485`](../../../services/orchard/src/refiner.rs#L33-L37)

- Enforces minimum fee: `1000 units × action_count`
- Enforces maximum gas: `10,000,000 per work package`
- Validates fee extracted from `value_balance` (positive = fee payment)
- **Prevents**: Economic DOS attacks, resource exhaustion

**Policy Constants**:
```rust
const MIN_FEE_PER_ACTION: u64 = 1000;
const MAX_GAS_PER_WORK_PACKAGE: u64 = 10_000_000;
const GAS_PER_ACTION: u64 = 50_000;
const GAS_BASE_COST: u64 = 100_000;
const FEE_TRANSFER_GAS: u64 = 10_000;
```

#### Fix #5: Nullifier Proof Depth Enforcement ✅
**Files**: [`services/orchard/src/refiner.rs:110-157`](../../../services/orchard/src/refiner.rs#L110-L157), [`services/orchard/src/refiner.rs:528-548`](../../../services/orchard/src/refiner.rs#L528-L548)

- Requires **full-depth (32)** sparse Merkle proofs for every nullifier absence check
- Rejects truncated or oversized nullifier paths before validation
- Prevents malicious builders from shrinking the nullifier tree or forging shallow proofs while still matching the stored root

### Attack Vectors Mitigated

| Attack Vector | Mitigation | Implementation |
|--------------|------------|----------------|
| **Proof replay across work packages** | Nullifier absence proofs + accumulate old-root check (ZIP-244 sighash prevents cross-bundle signature reuse but does not bind to work package hash) | refiner.rs:329-377, signature_verifier.rs:323-383 |
| **Fee bypass/forgery** | Binding signature over value_balance + fee validation | refiner.rs:438-485, signature_verifier.rs |
| **Economic DOS (zero fees)** | Minimum fee enforcement | refiner.rs:460-466 |
| **Resource exhaustion (excessive gas)** | Maximum gas limit | refiner.rs:472-477 |
| **Parameter tampering** | Bundle consistency checks | refiner.rs:586-625 |
| **Layout confusion** | Strict Orchard NU5 validation | vk_registry.rs, refiner.rs:507-584 |

### Transparent path trust model (malicious builder risk)

- Transparent UTXO verification **relies on builder-supplied roots/proofs** (optional snapshots) matching the claimed `transparent_utxo_root`/`transparent_utxo_size`. There is **no JAM witness or genesis binding** for the transparent UTXO set, so a malicious builder can seed an arbitrary snapshot/root and fabricate spendable inputs. Production hardening must add a JAM-committed checkpoint (or Merkle witness) for the transparent UTXO root/size before accepting builder-supplied proofs.

### Security Testing Checklist

Before production deployment:

- [ ] Valid bundle with valid signatures → ACCEPT
- [ ] Valid proof with invalid spend_auth_sig → REJECT
- [ ] Valid proof with invalid binding_sig → REJECT
- [ ] Valid proof with forged value_balance → REJECT
- [ ] Valid proof with forged fee → REJECT
- [ ] Zero fee transaction → REJECT
- [ ] Excessive gas limit → REJECT
- [ ] Mismatched public inputs → REJECT
- [ ] Wrong layout (44 fields) → REJECT
- [ ] Bundle without signatures → REJECT

## Testing
- Use Orchard test vectors from the `orchard` crate.
- Add proof verification tests that use the canonical bundle encoding.
- Test all security fixes with attack vectors from SECURITY-AUDIT.md.
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

    // Parameters (optional - can use hardcoded values)
    min_fee: u64,                 // Minimum transaction fee
    gas_min: u64,                 // Minimum gas limit
    gas_max: u64,                 // Maximum gas limit
}
```

**Fee Handling**: Fees are recorded as `FeeTally` intents during refine and transferred to service 0 (JAM core) during accumulate after old-root validation. No fee accounting state is required in the Orchard service.

Builders store the full nullifier set off-chain; its Merkle root is committed in JAM state as `nullifier_root`.

## Witness-Based Execution Model

### Security Architecture
JAM uses a **witness-based execution model** where builders provide cryptographic
proofs of state transitions without requiring JAM to maintain full service state:

1. **Builder Phase**:
   - Executes Orchard transactions
   - Records state access patterns (commitment tree reads, nullifier checks)
   - Generates cryptographic proofs for spent commitments and nullifier absence, and includes pre/post state payloads for consistency checks

2. **Guarantor Phase**:
   - Verifies spent-commitment membership and nullifier absence proofs against the claimed pre-state root (no JAM Merkle proofs for `commitment_root`/`commitment_size`/`nullifier_root`)
   - Re-executes transactions deterministically using cache-only backend
   - Computes post-state roots from the frontier and checks consistency with any post-state witness values (not JAM proofs)
   - Frontier validation is strict: refine recomputes the current root from `commitment_frontier` and `commitment_size` and rejects if it does not match `commitment_root`; frontier length must equal the Orchard tree depth (32) or refine fails
   - If the witness includes an `orchard_state` read, refiner binds the pre-state payload roots to that state value

### State Access Requirements
**Pre-state payload + Orchard proofs (no JAM Merkle proofs for roots):**
- `commitment_root`, `commitment_size`, `commitment_frontier`
- `nullifier_root` and non-membership proofs for each `nf_old`
- spent-commitment membership proofs against `commitment_root`
- optional `orchard_state` read witness to bind pre-state payload roots to JAM state

**Post-state consistency checks (no JAM Merkle proofs for roots):**
- computed `commitment_root`, `commitment_size`, `nullifier_root` compared against post-state witness values when present

**Fee handling:**
- Fees recorded as FeeTally intents during refine; transfer executed in accumulate after old-root validation
- No fee-related state reads or writes required beyond the intent payload

### Security Properties
- **State Integrity**: Orchard proofs bind reads/writes to the claimed pre-state root; accumulate binds that root to JAM state via old/new transition checks
- **Transition Policy**: accumulate enforces monotonic `commitment_size` and commitment intent count
- **Execution Determinism**: Identical transactions + pre-state → identical post-state
- **Non-Forgery**: Invalid state transitions cannot produce valid witnesses
- **Byzantine Tolerance**: Up to 1/3 builders can be malicious without compromising safety

## Fee Payment Mechanism

### User → JAM Core Fee Transfer

Orchard uses a **delayed transfer model** where fees are validated in refine and paid during accumulate via JAM's native transfer system:

#### 1. **User Fee Specification**
```rust
// In Orchard bundle
struct OrchardBundle {
    value_balance: i64,  // Positive = value leaving pool (fee payment)
    // ... other fields
}
```

Users specify fees through positive `value_balance`, which represents transparent value leaving the shielded pool.

#### 2. **Fee Validation During Refine**
```rust
// Extract fee from value_balance
let fee = if bundle.value_balance > 0 {
    bundle.value_balance as u64
} else {
    0u64  // No fee if value_balance ≤ 0
};

// Validate fee meets requirements
if fee < MIN_FEE_PER_ACTION * action_count {
    return Err("Fee below minimum");
}
if fee < estimated_gas_cost {
    return Err("Fee insufficient for gas");
}
```

#### 3. **Emit FeeTally Intent (Refine)**
```rust
// Record fee for accumulate to process after state binding
if fee > 0 {
    let fee_intent = build_fee_intent(fee); // object_kind=FeeTally, payload=u128 LE
    intents.push(fee_intent);
}
```

#### 4. **Execute JAM Transfer (Accumulate)**
```rust
// After old_root/new_root checks succeed in accumulate
if fee > 0 {
    host_functions::transfer(
        0,          // destination: service 0 (JAM core)
        fee,        // amount to transfer
        10_000,     // gas limit for transfer
        memo_ptr,   // zeroed memo
    );
}
```

### Key Properties

- ✅ **State-Bound Payment**: Fees transfer only after accumulate validates old roots
- ✅ **No State Required**: No fee accounting in Orchard service state
- ✅ **JAM Native**: Uses JAM's built-in transfer mechanism
- ✅ **Stateless Refine**: Fee intent is deterministic and avoids side effects
- ✅ **Gas Controlled**: Transfer has its own gas limit (10k units)

### Fee Flow Summary

1. **User** creates bundle with positive `value_balance` (fee)
2. **Builder** submits bundle in work package to JAM network
3. **Refiner** validates fee meets minimum + gas requirements and emits `FeeTally`
4. **Accumulate** validates old roots and executes JAM fee transfer
5. **JAM Core** receives fee payment in native JAM tokens
6. **No Cleanup Required** - JAM handles all service balance accounting

This model ensures builders are compensated only when the work package binds to current JAM state, closing the malicious-builder gap from pre-accumulate transfers.

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

    // 2. Parse bundle
    let bundle = parse_bundle(&extrinsic.bundle_bytes)?;

    // 3. SECURITY FIX #4: Fee/gas policy enforcement
    validate_fee_and_gas(&bundle)?;  // MIN_FEE_PER_ACTION, MAX_GAS

    // 4. SECURITY FIX #3: Public input layout validation
    validate_public_input_layout(&public_inputs, bundle.action_count)?;  // 9 fields per action

    // 5. SECURITY FIX #2: Bundle consistency validation
    verify_bundle_consistency(&bundle, &public_inputs, &witnesses)?;

    // 6. SECURITY FIX #1: Signature verification
    let sighash = compute_bundle_commitment(&bundle);
    verify_bundle_signatures(&bundle, &sighash)?;  // RedPallas

    // 7. ZK proof verification
    verify_orchard_proof(&bundle.proof, &public_inputs)?;  // Halo2, K=11

    // 8. State transition computation
    let mut write_intents = compute_state_updates(&bundle, &witnesses)?;

    // 9. Fee intent emission (transfer happens in accumulate)
    if bundle.value_balance > 0 {
        write_intents.push(build_fee_intent(bundle.value_balance as u64));
    }

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
    // Execute fee transfer after state binding succeeds
    let fee_total = sum_fee_tally_intents(&write_intents)?;
    if fee_total > 0 {
        jam_transfer(0, fee_total, 10_000)?;
    }
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

## Transparent Transaction Support (2026-01-04)

### ⚠️ CURRENT STATUS: TRUSTED-BUILDER ONLY (NOT FULLY PRODUCTION READY)

Transparent Zcash transaction support is implemented (v5 parsing, mempool validation,
tag=4 verification), but it is **not production-ready for an untrusted builder**.

**Known Limitations**:
- UTXO roots/sizes are not JAM-anchored yet; validators must trust builder-supplied roots/proofs
- No persistent transparent store or chain scan (in-memory only)
- Tag=4 extrinsic is required for trustless verification; Go builder emits proofs but not snapshots yet

### Overview

This implements transparent transaction RPC methods to enable compatibility with web wallets that build transparent transactions locally. **Use in trusted/dev environments until UTXO roots are JAM-anchored and persistence is added.**

**Work-package note**: When transparent state roots/sizes are non-zero, work packages must include the tag=4 `TransparentTxData` extrinsic (raw txs + proofs; snapshots are optional and not emitted by the Go builder yet). See `builder/orchard/docs/SERVICE-TESTING.md` for the current serialization and validation requirements.
If no tag=4 extrinsic is provided (e.g., no transparent transactions in the package), the validator still emits no-op transparent state transitions so accumulate can bind the builder-supplied transparent roots/sizes to JAM state—preventing builders from zeroing these fields to dodge verification.

### Dual Pool Architecture

The service maintains two independent transaction pools:

**Transparent Pool** (`TransparentTxStore`):
- Standard Zcash transparent transactions
- UTXO-based model
- Visible amounts and addresses
- Raw hex storage and mempool management

**Orchard Pool** (`OrchardTxPool`):
- Privacy-preserving shielded transactions
- Note-based model (commitment tree)
- Hidden amounts and recipients
- ZK proof validation

### RPC Methods Added

The following Zcash-compatible RPC methods are now available:

#### `getrawtransaction`
```bash
curl -X POST http://localhost:8232 -H "Content-Type: application/json" \
  -d '{"jsonrpc":"1.0","method":"getrawtransaction","params":["txid",false],"id":1}'
```
Returns raw transaction hex (non-verbose) or full transaction object (verbose).

#### `sendrawtransaction`
```bash
curl -X POST http://localhost:8232 -H "Content-Type: application/json" \
  -d '{"jsonrpc":"1.0","method":"sendrawtransaction","params":["0x..."],"id":1}'
```
Broadcasts a raw transparent transaction to the mempool.

#### `getrawmempool`
```bash
curl -X POST http://localhost:8232 -H "Content-Type: application/json" \
  -d '{"jsonrpc":"1.0","method":"getrawmempool","params":[false],"id":1}'
```
Returns transaction IDs in the transparent mempool.

#### `getmempoolinfo`
```bash
curl -X POST http://localhost:8232 -H "Content-Type: application/json" \
  -d '{"jsonrpc":"1.0","method":"getmempoolinfo","params":[],"id":1}'
```
Returns mempool statistics (size, bytes, usage).

### Web Wallet Compatibility

**Zero Code Changes Required**: Web wallets that use the standard Zcash RPC interface work without modification:

1. **Local Transaction Building**: Web wallet builds transparent transactions using WASM
2. **Private Key Isolation**: All signing happens client-side, private keys never leave browser
3. **Standard RPC Interface**: Uses `sendrawtransaction` to broadcast, `getrawtransaction` to query
4. **Drop-in Replacement**: Orchard RPC service can replace Zcash full node for wallet purposes

### Implementation Details

**Transaction Store** (`builder/orchard/rpc/transparent_tx.go`):
```go
type TransparentTxStore struct {
    transactions map[string]*TransparentTransaction // txid -> transaction
    mempool      map[string]*TransparentTransaction // txid -> pending
    utxos        map[string][]UTXO                  // address -> unspent outputs
    blockTxs     map[uint32][]string                // block height -> txids
}
```

**Key Features**:
- Raw transaction hex storage
- Persistent mempool storage (LevelDB) with in-memory cache
- UTXO set tracking for balance queries
- Double-spend prevention
- Transaction confirmation handling

**RPC Methods** (`builder/orchard/rpc/handler.go`):
- `GetRawTransaction(txid, verbose)` - Retrieve transaction by ID
- `SendRawTransaction(hexstring)` - Broadcast raw transaction
- `GetRawMempool(verbose)` - Query mempool contents
- `GetMempoolInfo()` - Mempool statistics

**HTTP Routing** (`builder/orchard/rpc/server.go`):
```go
case "getrawtransaction":
    err = s.handler.GetRawTransaction(stringParams, &result)
case "sendrawtransaction":
    err = s.handler.SendRawTransaction(stringParams, &result)
case "getrawmempool":
    err = s.handler.GetRawMempool(stringParams, &result)
case "getmempoolinfo":
    err = s.handler.GetMempoolInfo(stringParams, &result)
```

### How Web Wallets Work

1. **Local Transaction Building**:
   - Web wallet uses WASM module for transaction construction
   - User provides UTXOs, recipients, amounts
   - WASM builds and signs transaction locally (no server access to keys)

2. **Blockchain Query**:
   - `getblockchaininfo` - Check sync status
   - `getrawtransaction` - Fetch UTXO details

3. **Transaction Broadcast**:
   - `sendrawtransaction` with signed hex
   - Server adds to mempool and propagates

4. **Transaction Verification**:
   - `getrawtransaction` - Verify broadcast
   - `getrawmempool` - Check mempool status

### Security Properties

**Private Key Isolation**:
- ✅ Private keys **never leave the browser**
- ✅ Transaction signing happens **client-side in WASM**
- ✅ Only signed raw transaction hex sent to server
- ✅ Server cannot access or derive private keys

**Mempool Security**:
- Double-spend prevention via UTXO tracking
- Transaction size limits (prevent memory exhaustion)
- Malformed transaction rejection
- UTXO state consistency

### Performance Characteristics

**Memory Usage**:
- Per transaction: ~1-10 KB (varies with inputs/outputs)
- Mempool capacity: Configurable (default unlimited)
- UTXO index: Map-based, O(1) lookups

**Latency**:
- `sendrawtransaction`: < 10ms (mempool addition)
- `getrawtransaction`: < 1ms (map lookup)
- `getrawmempool`: < 5ms (map iteration)

### Usage Example

Point a Zcash web wallet to the Orchard RPC endpoint:

```javascript
// In web wallet configuration
const RPC_URL = "http://localhost:8232";

// No code changes needed - wallet works as-is
// Builds transparent transactions locally
// Broadcasts via sendrawtransaction
```

### Future Enhancements

**Phase 2: Full Transaction Parsing**
- Full Zcash transaction deserialization
- Input/output parsing
- Script validation
- Signature verification

**Phase 3: Block Confirmation**
- Transaction confirmation in blocks
- Confirmation count tracking
- Block reorganization handling
- Persistent storage (currently in-memory)

**Phase 4: Advanced Features**
- Transaction fee estimation
- Replace-by-fee (RBF) support
- Batch transaction handling
- Mempool priority management

### Comparison with Zcash Full Node

| Feature | Zcash Full Node | Orchard Service |
|---------|----------------|-----------------|
| **Transparent Transactions** | Full validation | Storage + broadcast |
| **Shielded Transactions** | Sapling + Orchard | Orchard only |
| **Script Validation** | Full | Minimal (future) |
| **Consensus Rules** | Full enforcement | Delegated to validators |
| **P2P Network** | Full participation | JAM network only |
| **Block Storage** | Complete blockchain | Recent blocks only |
