# JAM Orchard Service - Complete Architecture

This document provides comprehensive technical details for the JAM Orchard service implementation. For quick start and usage, see [README.md](README.md).

## Table of Contents

- [Overview](#overview)
- [JAM State Model](#jam-state-model)
- [Work Package Structure](#work-package-structure)
- [Verification Flow](#verification-flow)
- [Halo2 Proof System](#halo2-proof-system)
- [Merkle Witness Design](#merkle-witness-design)
- [Data Structures](#data-structures)
- [Integration Guide](#integration-guide)
- [Performance](#performance)

## Overview

The JAM Orchard service enables privacy-preserving transactions using Zcash's Orchard protocol within the JAM (Join-Accumulate Machine) blockchain architecture.

### Key Design Principles

1. **Stateless Verification**: JAM validators store only state roots + size, verifying all operations via Merkle witnesses
2. **Layered Security**: Pre-state witnesses + Halo2 zkSNARKs + deterministic recomputation
3. **User-Generated Proofs**: Each user's wallet creates Bundle<Authorized, i64> with Halo2 proof
4. **Deterministic State Transitions**: Builders propose, validators verify, no trust required

### Architecture Components

```
┌─────────────────────────────────────────────────────────────┐
│ User Wallets                                                 │
│ └─ Generate Bundle<Authorized, i64> with Halo2 proof       │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Builder (Off-Chain)                                          │
│ ├─ Maintains full Merkle trees (commitment, nullifier)      │
│ ├─ Collects user bundles                                    │
│ ├─ Generates pre-state witnesses (Merkle proofs for reads)  │
│ ├─ Simulates state transitions                              │
│ └─ Constructs work package                                  │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ JAM Work Package                                             │
│ ├─ 1. Pre-State Witnesses (Merkle proofs)                   │
│ ├─ 2. User Bundles (Bundle<Authorized, i64> with proofs)   │
│ └─ 3. Gas policy + limits                                   │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ JAM Validators (Refine - Stateless)                          │
│ ├─ Store: state roots + size                                │
│ ├─ Verify: Pre-state witnesses against current roots        │
│ ├─ Verify: Each user's Halo2 proof                          │
│ ├─ Recompute: State transitions deterministically           │
│ └─ Apply: New roots from deterministic recomputation        │
└─────────────────────────────────────────────────────────────┘
```

## JAM State Model

JAM validators store the minimal on-chain state required for stateless verification:

```rust
pub struct OrchardState {
    pub commitment_root: [u8; 32],  // All note commitments (spent + unspent)
    pub commitment_size: u64,        // Commitment tree size (8 bytes)
    pub nullifier_root: [u8; 32],   // Spent notes set (nullifiers)
    pub gas_min: u64,
    pub gas_max: u64,
}
```

### Why This Works

**Full state** (millions of nodes):
- Commitment tree: ~1M commitments × 96 bytes = 96 MB
- Nullifier set: ~500K nullifiers × 32 bytes = 16 MB
- **Total: ~112 MB**

**JAM state** (witness-based):
- Roots + size + gas policy fields (88 bytes total)
- **Reduction: 99.9999% smaller**

Validators verify everything through Merkle proofs without storing the trees.
Unspent notes are defined as commitments that are not present in the nullifier set.

### Chain History MMR (ZIP-221 Alignment)

Post-NU5 Zcash commits to `hashChainHistoryRoot` (a chain-history MMR root) rather
than a single Sapling/Orchard commitment root. To align with ZIP-221, the JAM
builder evolves a chain-history MMR over block or package boundaries, with each
leaf committing to the earliest/latest Sapling and Orchard roots for that height.
The resulting `chain_history_root` can be committed in the block header alongside
JAM state roots. This MMR is orthogonal to the Orchard service state: validators
still track `commitment_root` and `nullifier_root`, while builders maintain the
MMR frontier for incremental updates.

## Work Package Structure

### Minimal Data Structures (Canonical Model)

```rust
pub struct WorkPackage {
    pub service_id: u32,
    pub payload: WorkPackagePayload,
    pub authorization: Vec<u8>,
    pub gas_limit: u64,
}

pub enum WorkPackagePayload {
    Submit(OrchardExtrinsic),
    BatchSubmit(Vec<OrchardExtrinsic>),
}

pub struct OrchardExtrinsic {
    pub bundle_bytes: Vec<u8>,
    pub gas_limit: u64,
    pub witnesses: StateWitnesses,
}

pub struct StateWitnesses {
    pub commitment_root_proof: MerkleProof,
    pub commitment_size: u64,
    pub nullifier_absence_proofs: Vec<MerkleProof>,
    pub gas_min: u64,
    pub gas_max: u64,
}
```

### Key Simplifications

- **Serialized bundle bytes** use the Zcash v5 Orchard encoding (see `bundle_codec`).
- **No stored actions list** in the work package: actions are derived from the bundle when needed.
- **No post-state witnesses**: validators deterministically recompute new roots from the bundle contents.

### Witness Details

**Commitment Witness** (proves note exists in tree):
```rust
pub struct CommitmentWitness {
    pub commitment: [u8; 32],           // Note commitment
    pub merkle_path: Vec<[u8; 32]>,     // Merkle siblings (32 levels)
    pub position: u64,                  // Leaf index in tree
}
```

**Nullifier Absence Witness** (proves not double-spent):
```rust
pub struct MerkleProof {
    pub leaf: [u8; 32],
    pub siblings: Vec<[u8; 32]>,
    pub position: u64,
    pub root: [u8; 32],
}
```


## Verification Flow

### Complete Refine Function (Simplified Model)

```rust
pub fn refine_orchard(extrinsic: &OrchardExtrinsic, state: &OrchardState) -> Result<()> {
    // ========================================
    // STEP 1: Verify Pre-State Witnesses
    // ========================================
    // Ensure all reads are from current state
    verify_pre_state_witnesses(state, &extrinsic.witnesses)?;

    // For each nullifier:
    //   1. Verify sparse Merkle proof reaches empty hash
    //   2. Confirms nullifier NOT in current nullifier_root

    // ========================================
    // STEP 2: Verify Orchard bundle proofs + signatures
    // ========================================
    // Each user's Bundle<Authorized, i64> contains proof that:
    //   ✅ User owns the notes (spend auth signatures)
    //   ✅ Notes exist in Merkle tree (anchor matches commitment_root)
    //   ✅ Value is conserved (inputs = outputs)
    //   ✅ Nullifiers correctly derived (prevents double-spend)
    //   ✅ Commitments well-formed (pedersen commitments valid)

    let bundle = deserialize_bundle(&extrinsic.bundle_bytes)?;
    verify_orchard_bundle(&bundle)?;

    // ========================================
    // STEP 3: Verify Fee Policy (Zero-Sum)
    // ========================================
    // The service requires each Orchard bundle to have value_balance == 0
    // (fully shielded, no transparent in/out). Builders cannot modify user
    // bundles, so any fee is just a normal Orchard output that the user
    // includes for the builder, or a deferred transfer from another service.
    // There is no standalone "coinbase" bundle that mints value.

    verify_zero_sum_value_balance(&bundle)?;

    // ========================================
    // STEP 4: Compute State Updates
    // ========================================
    // Deterministically append commitments and insert nullifiers, then
    // apply the resulting roots to state.

    Ok(())
}
```

Batch payloads (`WorkPackagePayload::BatchSubmit`) are verified in order. Each
extrinsic is refined against the current temporary state, then applied before
the next extrinsic is checked. The final `WriteIntents` reflect the last root.

### Fee Handling (Zero-Sum)

Because bundles are signed by users, builders cannot insert outputs into an
already-authenticated bundle. Under the zero-sum rule (`value_balance == 0`),
builder compensation must be funded by user spends inside the same bundle or
via a deferred transfer from another service. There is no standalone coinbase
bundle that mints value.

```rust
fn verify_zero_sum_value_balance(bundle: &OrchardBundle) -> Result<()> {
    if *bundle.value_balance() != 0 {
        return Err(Error::InvalidBundle("Non-zero value balance".into()));
    }
    Ok(())
}
```

### Why Two Layers + Deterministic Transition?

1. **Pre-state witnesses**: Prevent builders from reading non-existent data
2. **Halo2 proofs**: Prevent users from stealing, double-spending, or breaking privacy
3. **Deterministic recomputation**: Prevent builders from corrupting state or lying about results

All three checks must pass for a work package to be valid.

## Halo2 Proof System

### What Is Halo2?

Halo2 is a zkSNARK system with **transparent setup** (no trusted setup ceremony). Orchard uses Halo2 with the IPA commitment scheme on the Pallas curve (not KZG).

### Key Generation (Transparent Setup)

**Important**: Unlike Sapling (Groth16 with MPC ceremony), Orchard uses Halo2 with transparent, deterministic key generation.

```rust
// 1. Generate universal IPA parameters (one-time, transparent)
let params: halo2_proofs::poly::commitment::Params<VestaAffine> =
    halo2_proofs::poly::commitment::Params::new(K);
// K = circuit size (e.g., 2^11 = 2048 gates)
// Generates IPA parameters deterministically, no ceremony needed
// Can ship params with app or generate locally

// 2. Build verifying key (fast, deterministic)
let vk = orchard::circuit::VerifyingKey::build();
// Derived from circuit structure + params
// ~1-2 seconds, same for everyone

// 3. Build proving key (expensive, one-time, cached)
let pk = orchard::circuit::ProvingKey::build();
// Derived from circuit + params + VK
// ~30 seconds - 2 minutes depending on hardware
// User wallets cache this, generate once
```

**NOT ceremony-based**: No need to download parameters from Zcash foundation or participate in MPC ceremony. Everything is deterministic and transparent.

### What Bundle<Authorized, i64> Proves

When user's wallet creates Bundle<Authorized, i64>, the Halo2 proof verifies:

```rust
// Each action in bundle proves:
pub struct Action {
    cv_net: ValueCommitment,        // ✅ Value commitment (hiding amount)
    nullifier: Nullifier,           // ✅ Nullifier (prevents double-spend)
    rk: VerificationKey,            // ✅ Randomized verification key
    cmx: ExtractedNoteCommitment,   // ✅ Note commitment (new output)
    encrypted_note: EncryptedNote,  // 🔒 Encrypted for recipient
    // ... other fields
}

// Halo2 proof verifies:
// 1. Authorization: Prover knows spending key for nullifier
// 2. Merkle membership: Note exists at claimed anchor (commitment_root)
// 3. Value conservation: Σ inputs = Σ outputs (in commitments)
// 4. Nullifier derivation: nf = PRF(sk, ρ) correctly computed
// 5. Commitment well-formedness: cmx = Commit(value, rcm) valid
// 6. Randomized key: rk derived correctly from spend auth key
//
// WITHOUT revealing:
// ❌ Input amounts
// ❌ Output amounts
// ❌ Sender identity
// ❌ Receiver identity
// ❌ Spending keys
```

### Verification in JAM

```rust
#[cfg(feature = "circuit")]
pub fn verify_orchard_bundle(bundle: &Bundle<Authorized, i64>) -> Result<()> {
    // Load/build verifying key (fast, deterministic)
    let vk = load_or_build_verifying_key();

    // Verify Halo2 proof + signatures
    bundle.verify_proof(&vk)
        .map_err(|e| Error::InvalidBundle(format!("Proof verification failed: {}", e)))?;
    let sighash: [u8; 32] = bundle.commitment().into();
    for action in bundle.actions() {
        action.rk().verify(&sighash, action.authorization())?;
    }
    bundle
        .binding_validating_key()
        .verify(&sighash, bundle.authorization().binding_signature())?;

    Ok(())
}
```

Validators need:
- ✅ Verifying key (build on-demand, ~1 second)
- ❌ NOT proving key (only users need this)
- ❌ NOT ceremony parameters (Halo2 is transparent)

## Merkle Witness Design

### Commitment Tree (Incremental Merkle Tree)

Append-only binary Merkle tree for all note commitments (spent + unspent).

```
Height 32:           ROOT
                    /    \
                   /      \
Height 31:       H1       H2
                / \      /  \
              ... ... ...  ...
                            /  \
Height 0:                 L_i  L_i+1

Leaves: Note commitments (32 bytes each)
```

**Witness Format**:
```rust
pub struct CommitmentWitness {
    pub commitment: [u8; 32],       // The leaf
    pub merkle_path: Vec<[u8; 32]>, // 32 siblings (one per level)
    pub position: u64,              // Leaf index
}

// Verification (Orchard Sinsemilla):
fn verify_commitment_witness(
    witness: &CommitmentWitness,
    expected_root: &[u8; 32],
) -> bool {
    use incrementalmerkletree::Level;
    use orchard::tree::MerkleHashOrchard;

    let mut current = MerkleHashOrchard::from_bytes(&witness.commitment)
        .expect("canonical commitment bytes");
    let mut pos = witness.position;

    for (level, sibling_bytes) in witness.merkle_path.iter().enumerate() {
        let sibling = MerkleHashOrchard::from_bytes(sibling_bytes)
            .expect("canonical sibling bytes");
        let (left, right) = if pos & 1 == 0 {
            (current, sibling)
        } else {
            (sibling, current)
        };
        current = MerkleHashOrchard::combine(Level::from(level as u8), &left, &right);
        pos >>= 1;
    }

    current.to_bytes() == *expected_root
}
```

### Nullifier Set (Sparse Merkle Tree)

Sparse binary Merkle tree for spent note tracking.
Internal node hashing uses the sparse-tree hash from `merkle_impl` (BLAKE2b).

```
Height 32:           ROOT
                    /    \
                   /      \
Height 31:       H1    EMPTY_HASH
                / \
              ... ...
                  /  \
Height 0:      nf_i  EMPTY_HASH

Leaves: Nullifiers (32 bytes) when present, EMPTY_HASH when absent
```

**Absence Proof** (proves nullifier NOT in set):
```rust
fn verify_nullifier_absence(
    proof: &MerkleProof,
    current_nullifier_root: &[u8; 32],
) -> bool {
    proof.root == *current_nullifier_root
        && proof.leaf == sparse_empty_leaf()
        && proof.verify()
}
```


## Data Structures

The architecture uses the minimal, canonical model defined above:

```rust
pub struct OrchardState {
    pub commitment_root: [u8; 32],
    pub commitment_size: u64,
    pub nullifier_root: [u8; 32],
    pub gas_min: u64,
    pub gas_max: u64,
}
```

## Integration Guide

### For JAM Validators

**Storage Requirements**:
```rust
// Store only this:
struct ValidatorState {
    orchard_state: OrchardState,
}
```

**Verification**:
```rust
use orchard_builder::workpackage::{refine_work_package, accumulate_orchard};

fn process_work_package(wp: &WorkPackage, current: &OrchardState) -> Result<OrchardState> {
    // 1. Stateless verification (handles Submit + BatchSubmit)
    let intents = refine_work_package(wp, current)?;

    // 2. Apply state transition
    let mut next = current.clone();
    accumulate_orchard(&mut next, &intents)?;

    Ok(next)
}
```

### For Builders

**Requirements**:
- Full Merkle trees (commitment, nullifier)
- Bundle bytes from user wallets (Zcash v5 Orchard encoding)

**Workflow**:
```rust
use orchard_builder::WorkPackageBuilder;

// Collect user bundles (application-specific)
let mut wp_builder = WorkPackageBuilder::new(chain_state, service_id);
let work_package = wp_builder.build_work_package(bundle, gas_limit)?;

// Or build a batch:
let work_package = wp_builder.build_batch_work_package(vec![
    (bundle_a, gas_a),
    (bundle_b, gas_b),
])?;
```

### For User Wallets

**Creating Bundles**:
```rust
use orchard::{
    builder::{Builder, BundleType},
    bundle::Authorized,
    keys::SpendAuthorizingKey,
    Bundle,
};
use rand::rngs::OsRng;

// 1. Build proving key (cache this!)
let proving_key = orchard::circuit::ProvingKey::build();
let mut rng = OsRng;

// 2. Create bundle builder
let mut builder = Builder::new(BundleType::DEFAULT, anchor);

// 3. Add spends (inputs)
for note in notes_to_spend {
    builder.add_spend(
        full_viewing_key,
        note,
        merkle_path,  // From builder's commitment tree
    )?;
}

// 4. Add outputs
builder.add_output(
    recipient_address,
    value,
    memo,
)?;

// 5. Build and authorize (generates Halo2 proof + signatures)
let (unauthorized, _) = builder
    .build(&mut rng)?
    .ok_or("Bundle builder returned None")?;
let proven = unauthorized.create_proof(&proving_key, &mut rng)?;
let sighash: [u8; 32] = proven.commitment().into();
let spend_auth_keys: Vec<SpendAuthorizingKey> = vec![/* wallet keys */];
let bundle: Bundle<Authorized, i64> =
    proven.apply_signatures(&mut rng, sighash, &spend_auth_keys)?;

// 6. Submit to builder
submit_to_mempool(bundle);
```

## Performance

### Verification Time

```
Per work package (10 actions):
  Pre-state witnesses:      ~1ms
    └─ 10 commitment proofs (32 levels each)
    └─ 10 nullifier absence proofs

  User Halo2 proofs:        ~100ms
    └─ 10ms per bundle (typical)
    └─ Scales with number of users, not actions

  State updates:            ~1ms
    └─ 10 commitment insertions
    └─ 10 nullifier insertions

  Total: ~101ms for 10-action package
```

### Proof Sizes

```
Bundle<Authorized, i64>:
  Halo2 proof:              ~15 KB
  Signatures:               ~64 bytes per action
  Actions:                  ~320 bytes per action
  Total per bundle:         ~15 KB + (384 × actions)

Merkle Witnesses:
  Commitment witness:       1 KB (32 × 32 bytes)
  Nullifier witness:        1 KB (sparse)

Work Package (10 actions):
  Pre-witnesses:            ~10 KB
  User bundles:             ~20 KB (1-2 bundles)
  Total:                    ~30 KB
```

### Storage Comparison

```
Full State Approach:
  Commitment tree:          96 MB (1M notes)
  Nullifier set:            16 MB (500K nullifiers)
  Total:                    ~112 MB per validator

Witness-Based Approach:
  JAM state:                ~88 bytes (roots + size + policy)
  Work package:             30 KB (ephemeral, not stored)
  Reduction:                99.9999%
```

### Scalability

```
Actions per second:       ~100 (limited by Halo2 verification)
Throughput:               ~1 MB/s work packages
State growth:             0 bytes (constant roots + size)
Validator requirements:   Minimal (CPU for verification only)
```

## Orchard Signing Digest

For this service, the Orchard bundle is the entire transaction, so spend and
binding signatures are verified against the bundle commitment:

```
sighash = bundle.commitment()
```

This matches the Orchard-only transaction model used in the builder and verifier.

## Security Properties

### Guaranteed by Halo2 Proofs

1. **Authorization**: Only note owner can spend (knows spending key)
2. **Conservation**: Value in = value out (no inflation)
3. **Uniqueness**: Nullifiers prevent double-spending
4. **Validity**: All commitments, nullifiers cryptographically well-formed
5. **Privacy**: Amounts, senders, receivers remain hidden

### Guaranteed by Merkle Witnesses

1. **Read correctness**: Spent notes actually exist in current state
2. **No double-spend**: Nullifiers not already in set
3. **Write correctness**: New state matches claimed transitions
4. **State integrity**: Roots cryptographically bind to full data

### Guaranteed by Deterministic Verification

1. **No builder trust**: Validators verify size + witnesses
2. **Reproducibility**: Same inputs → same roots always
3. **Fault attribution**: Invalid package identifies malicious builder
4. **Censorship resistance**: Valid packages must be accepted

## References

- **JAM Graypaper**: [graypaper.com](https://graypaper.com/)
- **Orchard Specification**: [Zcash Protocol Spec](https://zips.z.cash/protocol/protocol.pdf)
- **Halo2 Documentation**: [zcash.github.io/halo2](https://zcash.github.io/halo2/)
- **Zcash Orchard Crate**: [github.com/zcash/orchard](https://github.com/zcash/orchard)
- **Orchard Book**: [zcash.github.io/orchard](https://zcash.github.io/orchard/)
- **Incremental Merkle Trees**: [incrementalmerkletree crate](https://crates.io/crates/incrementalmerkletree)

## Source Code Structure

```
builder/orchard/
├── src/
│   ├── lib.rs                  # Public API, error types
│   ├── state.rs                # State roots + policy
│   ├── witness_based.rs        # Core verification (refine)
│   ├── merkle_impl.rs          # Merkle tree implementations
│   ├── bundle_codec.rs         # Zcash v5 bundle encoding/decoding
│   ├── workpackage.rs          # WorkPackage structures
│   ├── witness.rs              # Witness generation helpers
│   ├── sequence.rs             # Multi-package generation
│   └── bin/
│       └── orchard_builder.rs  # CLI tool
├── Cargo.toml
├── README.md                   # Quick start guide
└── ARCHITECTURE.md             # This file
```

Start with [README.md](README.md) for quick start, then explore [`src/witness_based.rs`](src/witness_based.rs) for verification implementation.
