# JAM Railgun Service: Privacy-Native Asset Transfer

## Overview

A JAM service enabling private asset transfers with builder-sponsored execution and zero-knowledge proof authorization. Unlike Ethereum-based privacy systems, JAM Railgun has no gas payer identity, no public calldata, and no mempool leakage.

## Core Architecture

### Three-Role Separation
| Role | Knows Identity | Pays Cost | Authorizes |
|------|---------------|-----------|------------|
| User | Yes | No | Yes (privately) |
| Builder | No | Yes | No |
| Validator | No | No | Verifies proof only |


**Reimbursement is proven, not promised.** Builders verify—before inclusion—that the protocol will reimburse them if execution succeeds.

## Service State

```rust
struct RailgunState {
    // Shielded note set commitment tree
    commitment_root: Hash,        // Merkle root of note commitments
    commitment_size: u64,         // REQUIRED: current append index / size (consensus-critical)

    // Double-spend prevention
    nullifiers: Set<Hash>,        // Spent note nullifiers
    offer_nullifiers: Set<Hash>,  // Consumed swap offers (optional)

    // Fee accounting (scoped; see Fee Flow)
    fee_accumulator: Map<(ServiceId, EpochId), u128>,

    outputs: Vec<Bytes>,          // Optional encrypted payloads for off-chain recipients
}
```

**Tracked per service block.** State transitions are deterministic and proven.

### Commitment Tree Semantics (Deterministic)
**Fixed insertion rule: append-at-size.**

Let `size = commitment_size` in state prior to processing an extrinsic. For an extrinsic that adds `N_OUT` commitments:
- `output_commitments[0]` is inserted at position `size + 0`
- `output_commitments[1]` at `size + 1`, …

**Define:**
```
MerkleAppend(root, size, output_commitments[]) -> root_next
```

**Validators MUST recompute `root_next` using this rule and reject if it does not match the proof's declared next root.**

### Note Model (UTXO-style)
```rust
Note := (asset_id, value, owner_pk, ρ)
Commitment := Com(asset_id, value, owner_pk, ρ)
Nullifier := NF(sk, asset_id, ρ)
```

## Extrinsic Interface

### 1. DepositPublic (Signed, One-Time)
```rust
DepositPublic {
    commitment: Hash,   // Com(pk, asset, value, ρ)
    value: u64,         // Amount to shield
}
```
**Optional public entry point.** Leaks depositor identity but funds the private pool. User generates `(sk, pk)` locally first.

### 2. SubmitPrivate (Unsigned)
```rust
SubmitPrivate {
    proof: Bytes,                      // Groth16 proof
    anchor_root: Hash,                 // MUST equal current commitment_root
    anchor_size: u64,                  // MUST equal current commitment_size

    input_nullifiers: [Hash; N_IN],    // Spent note nullifiers
    output_commitments: [Hash; N_OUT], // New commitments (ORDERED for append-at-size)

    fee: u64,                          // Z-space fee amount
    encrypted_payload: Bytes,          // Optional: recipient hints, memo
    offer_nullifier: Option<Hash>,     // Optional: atomic swaps
}
```
**No user signature. No origin address. Builder submits on user's behalf.**

### 3. WithdrawPublic (Unsigned)
```rust
WithdrawPublic {
    proof: Bytes,        // Proves note ownership AND binds to recipient/amount/asset
    anchor_root: Hash,   // Current commitment_root (or allowed recent root per policy)
    nullifier: Hash,     // Spent note nullifier
    recipient: Address,  // Public exit address
    asset_id: AssetId,
    amount: u64,         // Deshielded amount
    fee: u64,            // Optional withdraw fee (credited in Z-space)
}
```
**Optional exit path.** Reveals amount and recipient, but not transaction history.

## ZK Proof Specification

### Spend Circuit

**Public Inputs:**
```
x := (anchor_root, anchor_size, next_root, {nf_i}, {cm'_j}, F)
  anchor_root  Current note tree root
  anchor_size  Current note tree size (append index)
  next_root    MerkleAppend(anchor_root, anchor_size, {cm'_j})  // DERIVED, not free
  nf_i         Input nullifiers (prove authorization)
  cm'_j        Output commitments (ORDERED)
  F            Fee amount
```

**Private Witness:**
```
w := ({sk_i, pk_i, v_i, ρ_i, path_i}, {pk'_j, v'_j, ρ'_j})
  sk_i         Secret keys for input notes
  v_i          Input note values
  ρ_i          Input note randomness
  path_i       Merkle paths (prove note existence)
  pk'_j        Output recipient public keys
  v'_j         Output values
  ρ'_j         Output randomness
```

**Constraints:**
1. **Input Validity:** Each input note commitment exists in Merkle tree at `anchor_root`
   ```
   ∀i: VerifyMerklePath(Com(pk_i, asset, v_i, ρ_i), path_i, anchor_root) = 1
   ```

2. **Authorization:** Nullifiers prove secret key ownership
   ```
   ∀i: nf_i = NF(sk_i, asset, ρ_i) ∧ pk_i = PublicKey(sk_i)
   ```

3. **Output Well-formedness:** Output commitments correctly formed
   ```
   ∀j: cm'_j = Com(pk'_j, asset, v'_j, ρ'_j)
   ```

4. **Value Conservation with Fee:**
   ```
   ∑v_i = ∑v'_j + F
   ```

5. **State Update (Deterministic):** New root includes output commitments
   ```
   next_root = MerkleAppend(anchor_root, anchor_size, [cm'_0, ..., cm'_(N_OUT-1)])
   ```

**Verifier requirement:** validators MUST recompute `MerkleAppend(anchor_root, anchor_size, output_commitments)` and reject if it does not match `next_root` from the proof public inputs.

## Fee Flow (Z-Space)

### User Perspective
- Fee deducted from note values inside zk proof
- No gas wallet, no ETH, no visible payer

### Builder Perspective
1. Receives `(proof, public_inputs)` from user
2. Verifies locally:
   - Proof is valid
   - Fee `F ≥ min_fee`
   - State transition is deterministic
3. Includes extrinsic only if reimbursement guaranteed

### Protocol Settlement
```
fee_accumulator[(service_id, epoch_id)] += F
```

#### Scoped settlement hook (binds payout to service/epoch and the actual block builder)

At block finalization, the protocol executes a deterministic settlement hook:

```
let b = block_builder_id(block_header)
let e = epoch_id(block_header)

builder_reward[b] += α * fee_accumulator[(service_id, e)]
protocol_treasury += (1-α) * fee_accumulator[(service_id, e)]
fee_accumulator[(service_id, e)] = 0
```

Where:
- `service_id` is the JAM service identifier
- `epoch_id` is a protocol-defined settlement window (e.g., epoch index / height bucket)
- `block_builder_id` is derived from the block header per JAM consensus rules
- `α` is a fixed parameter (e.g., 0.8)

**Builders paid deterministically by protocol, not by users.**

## Privacy Properties

### What's Hidden
- ✅ Who paid
- ✅ Who received
- ✅ Amount transferred
- ✅ Asset type (in multi-asset version)
- ✅ Gas payer identity
- ✅ Action purpose

### What's Public
- ❌ Proof validity
- ❌ Total fees per block
- ❌ Builder payout (aggregated)
- ❌ State root transitions

### What Builder Learns
**Nothing.** Builder sees:
- Opaque proof bytes
- Nullifiers (random-looking hashes)
- Commitments (random-looking hashes)
- Fee amount (but not who paid it)

## Anti-Spam & Safety

### Spam Prevention
1. **Fee Floor:** Proof must allocate `F ≥ min_fee` to `FeeAccumulator`
2. **Nullifier Enforcement:** Double-spend attempts fail verification
3. **Proof Verification Cost:** Invalid proofs rejected pre-inclusion (builder pre-check)
4. **Deposit Model:** Users lock value when creating notes (not identity-based)

### Safety Guarantees
- **Double-spend prevention:** `NullifierSet` enforced in proof and on-chain
- **Replay protection:** Nullifier uniqueness + block hash binding in proof
- **Censorship resistance:** Builders cannot filter by intent—payload is opaque; only fee and proof validity visible

## Atomic Swaps (Two-Party)

### Offer-Take Pattern
```rust
SwapOffer {
    asset_a: AssetId,
    value_a: u64,
    asset_b: AssetId,
    value_b: u64,
    maker_pk: PublicKey,
    taker_pk: Option<PublicKey>,  // Optional: restrict to specific taker
    expiry: BlockHeight,
    nonce: u64,
}

offer_id = Com(offer_terms, ρ_offer)
```

### Single Extrinsic Atomicity
Taker submits one `SubmitPrivate` extrinsic with proof that:
1. Spends maker's note (asset A) with maker authorization
2. Spends taker's note (asset B) with taker authorization
3. Creates output: `asset_a → taker`, `asset_b → maker`
4. Consumes offer (nullifies `offer_id`)
5. Pays fee in Z-space

**Either both transfers happen or neither. No pools, no partial fills, no contracts.**

## Implementation Architecture

### Prover Side (std, user's machine)
```
zk-circuits/      Circuit definitions, proving key
prover-app/       Generates proofs from user inputs
wallet-lib/       Note management, Merkle path caching
```

### Verifier Side (no_std, JAM runtime)
```
zk-verifier/      Groth16 verification (BN254 curve)
jam-service/      State transition logic, nullifier checks
```

### Tooling
- **Proof System:** Groth16 (smallest proof, fastest verifier)
- **Curve:** BN254 (optimal for no_std verification)
- **Hash Function:** Poseidon (circuit-friendly, not Keccak)
- **Signature Scheme:** EdDSA over Jubjub (not secp256k1)

## Execution Flow

### 1. Deposit (Bootstrap)
1. User generates keypair `(sk, pk)` locally (never shared)
2. User submits signed `DepositPublic{commitment, value}`
3. Service creates note: `cm = Com(pk, asset, value, ρ)`, inserts into tree
4. User stores `(pk, value, ρ, asset)` in local wallet

**Leakage:** Depositor identity visible. Use mixing after deposit for privacy.

### 2. Transfer (Private)
1. **User (off-chain):**
   - Selects input notes (own UTXOs)
   - Generates output notes for recipients
   - Computes Merkle paths from local tree cache
   - Proves spend circuit locally
   - Hands `{encrypted_payload, proof, fee_hint}` to builder (off-chain channel)

2. **Builder (pre-inclusion check):**
   - Verifies zk proof validity against current commitment root and nullifier set
   - Checks `fee ≥ min_fee` and `FeeAccumulator` credit is proven
   - Wraps into unsigned `SubmitPrivate` extrinsic
   - Includes in block if valid

3. **Validators (consensus):**
   - Verify proof
   - Check `anchor_root == commitment_root` and `anchor_size == commitment_size`
   - Recompute `next_root_expected = MerkleAppend(commitment_root, commitment_size, output_commitments)`
   - Reject if `next_root_expected != next_root` (from proof public inputs)
   - Check nullifiers not in `NullifierSet`
   - Update state:
     - Insert `input_nullifiers` into `NullifierSet`
     - Insert `output_commitments` into tree via append-at-size
     - Update `commitment_root = next_root_expected`
     - Update `commitment_size += N_OUT`
     - Increment `fee_accumulator[(service_id, epoch_id)] += fee`

4. **Recipients (off-chain scanning):**
   - Monitor new `output_commitments` each block
   - Trial-decrypt each commitment with own `sk`
   - Add detected notes to local wallet

**At block settlement:** Builder credited from `FeeAccumulator` (protocol logic, not user-directed).

### 3. Withdraw (Optional Exit)
1. User proves ownership of note via `WithdrawPublic{proof, anchor_root, nullifier, recipient, asset_id, amount, fee}`
2. Service verifies proof, marks nullifier spent
3. Funds transferred to public `recipient` address

**Leakage:** Exit amount and recipient visible, but not transaction history.

#### Withdraw Circuit (minimal spec)

**Public Inputs:**
```
x := (anchor_root, nf, recipient, asset_id, amount, F_withdraw)
```

**Private Witness:**
```
w := (sk, pk, v, ρ, path, [optional change witness])
```

**Constraints:**

1) **Note inclusion:**
   ```
   cm = Com(pk, asset_id, v, ρ)
   VerifyMerklePath(cm, path, anchor_root) = 1
   ```

2) **Authorization / nullifier:**
   ```
   nf = NF(sk, asset_id, ρ)
   pk = PublicKey(sk)
   ```

3) **Binding to public exit:**
   ```
   amount ≤ v
   Bind = H("withdraw" || recipient || asset_id || amount || nf)
   ```
   (Bind is computed inside the circuit to prevent replay/mismatched recipient/amount/asset for the same witness.)

4) **Value split (recommended):**
   If `v > amount + F_withdraw`, the remainder becomes a private change note that is appended deterministically:
   ```
   v = amount + F_withdraw + change_value
   cm_change = Com(change_pk, asset_id, change_value, change_ρ)
   ```

**Validator/service checks (outside the circuit):**
- `anchor_root == commitment_root` (or allowed recent root per reorg policy)
- `nf ∉ nullifiers` before applying
- On success:
  - `nullifiers.add(nf)`
  - credit `fee_accumulator[(service_id, epoch_id)] += F_withdraw`
  - execute public transfer to `(recipient, asset_id, amount)`
  - if change output exists: append `cm_change` via append-at-size and update `(commitment_root, commitment_size)` deterministically

## Comparison to Ethereum Railgun

| Feature | Ethereum Railgun | JAM Railgun |
|---------|-----------------|-------------|
| Gas payer visible | Yes (relayer address) | No |
| Builder sees intent | Yes (calldata) | No |
| Mempool leakage | Yes | No channels |
| Fee model | External relayer trust | Proof-enforced reimbursement |
| Spam resistance | Relayer policies | zk-verified fees |
| Contract dependency | Required | Native service |
| Composability | EVM contracts | Service-to-service |

## Security Model

### Trust Assumptions
1. **zk Soundness:** Cannot forge invalid proofs
2. **Hash Collision Resistance:** Poseidon is secure
3. **Honest Validator Majority:** Standard JAM assumption

### Attack Resistance
- **Double-spend:** Nullifier set prevents reuse
- **Inflation:** Value conservation proven in circuit
- **Fee Evasion:** Proof fails if fee insufficient
- **Replay:** Nullifiers + offer_nullifiers prevent
- **MEV/Censorship:** Builders cannot selectively censor (no identity known)

## Open Design Choices

### Multi-Asset Support
- **Option A:** Single asset_id per service (simplest)
- **Option B:** Multi-asset notes with asset_id in commitment (more complex circuits)

### Privacy/Performance Tradeoff
- **More inputs/outputs:** Better privacy (more mix), slower proving
- **Fewer inputs/outputs:** Weaker privacy, faster proving
- Recommended: N_IN=2, N_OUT=2 (standard shielded pool parameters)

### Withdraw Path
- **Option A:** Public unshield extrinsic (reveals amount, not history)
- **Option B:** Bridge to Ethereum via zk proof (maintains privacy)

## Implementation Roadmap

### Phase 1: Core Circuits
- [ ] Spend circuit with arkworks (Poseidon hash, Merkle membership)
- [ ] Commitment scheme: `Com(pk, asset_id, value, ρ)` using Poseidon
- [ ] Nullifier scheme: `NF(sk, asset_id, ρ)` using Poseidon
- [ ] Value conservation + fee constraint enforcement
- [ ] Groth16 proving/verification key generation

### Phase 2: Verifier Integration (no_std)
- [ ] Extract verification key as `const VK_BYTES`
- [ ] Implement `verify_groth16(proof, public_inputs)` with BN254
- [ ] Embed verifier in JAM service runtime
- [ ] Test round-trip: deposit → transfer → withdraw

### Phase 3: Builder Infrastructure
- [ ] Define off-chain proof submission protocol
- [ ] Builder pre-check: proof verify + fee policy + nullifier cache
- [ ] Wire `FeeAccumulator` to JAM builder rewards
- [ ] Invalid proof rejection (no on-chain spam)

### Phase 4: User Tooling
- [ ] Wallet SDK: local keypair management
- [ ] Merkle tree syncing (light client or indexer)
- [ ] Prover app: select notes, compute paths, generate proof
- [ ] Recipient scanning: trial-decrypt commitments each block

### Phase 5: Testing
- [ ] Deposit → SubmitPrivate → WithdrawPublic round-trip
- [ ] Invalid proof rejected by builder and validator
- [ ] Nullifier replay rejected
- [ ] Fee underpayment rejected
- [ ] Multiple opaque extrinsics unlinkable (no side-channel leaks)

### Phase 6: Production Hardening
- [ ] Trusted setup: MPC ceremony for production keys
- [ ] Formal audit of circuits and no_std verifier
- [ ] Encrypted payload format (hybrid ElGamal over Jubjub)
- [ ] Rate limiting / dynamic fee policy

## What to Avoid (Anti-Patterns)

- ❌ **Do NOT include signer or public key** in `SubmitPrivate`
- ❌ **Do NOT leak fee payer identity**—fee is protocol-credited, not user-directed
- ❌ **Do NOT rely on EVM contracts**—all logic lives in JAM service transition
- ❌ **Do NOT use Ethereum-style gas**—use Z-space fee accounting
- ❌ **Do NOT expose calldata**—use encrypted payloads + zk proofs

---

**Design Philosophy:** JAM Railgun treats privacy as a first-class protocol feature, not a smart contract afterthought. Builders, users, and validators never see the full picture—only math proves correctness.
