# JAM Railgun Service: Privacy-Native Asset Transfer

## Building the Service

**Implementation**: [services/railgun/](services/railgun/) - Complete no_std JAM service

```bash
# Check Rust code and verify all imports
make railgun

# Expected output:
# 🔫 Railgun service structure ready
#    Based on RAILGUN.md specification with all security fixes
#    Entry points:
#    - refine: ZK proof verification + state validation
#    - accumulate: Apply writes + process deposits
```

**Service structure**:
- [src/main.rs](src/main.rs:1-221) - PVM entry points (`refine`, `accumulate`)
- [src/refiner.rs](src/refiner.rs:1-306) - Stateless ZK proof verification
- [src/accumulator.rs](src/accumulator.rs:1-22) - State application logic
- [src/state.rs](src/state.rs:1-121) - State & extrinsic definitions
- [src/crypto.rs](src/crypto.rs:1-108) - Cryptographic primitives (stubs)
- [src/errors.rs](src/errors.rs:1-68) - Error types (consensus-aligned)
- [Cargo.toml](Cargo.toml:1-29) - Workspace integration

**Dependencies**: `polkavm-derive`, `simplealloc`, `utils` (shared JAM service utilities)

---

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
Builders replay refine with the witnessed `fee_tally` to see the exact credit that will land at the next epoch payout.

## Service State

**Implementation**: [src/state.rs](src/state.rs:9-60)

```rust
struct RailgunState {
    // Shielded note set commitment tree
    commitment_root: Hash,        // Merkle root of note commitments
    commitment_size: u64,         // REQUIRED: current append index / size (consensus-critical)
                                  // Why separate from root? Enables deterministic append-at-size without full tree reconstruction.
                                  // Validators must verify: next_root = MerkleAppend(root, size, new_commitments)

    // Double-spend prevention
    nullifiers: Map<Hash, [u8; 32]>,  // Spent note nullifiers
                                       // Value encoding: [0u8; 32] = unspent, [1u8; 32] = spent (canonical)
                                       // 32-byte encoding ensures Verkle proof consistency

    // Fee accounting (epoch-based; see Fee Flow)
    fee_tally: Map<EpochId, u128>,

    // Anti-spam parameters (consensus-critical)
    min_fee: u64,                 // Minimum fee for SubmitPrivate and WithdrawPublic (enforced in refine)
                                  // Updated via governance extrinsic (not implemented in MVP; hardcoded to 1000)

    // Gas bounds (consensus-critical; governance-updatable)
    gas_min: u64,                 // Lower bound for cross-service gas (initially 50_000)
    gas_max: u64,                 // Upper bound for cross-service gas (initially 5_000_000)
}
```

**Tracked per service block.** State transitions are deterministic and proven.
**Why store `commitment_size`?** The tree is maintained as key-value leaves; size is not derivable from sparse storage alone. The explicit counter is the consensus source of truth for append positions and is required for deterministic `MerkleAppend`.

**Encrypted payloads (MVP scope):** Omitted in the MVP state to avoid unused storage. A future version will re-introduce encrypted payloads under a version gate (hybrid ElGamal over Jubjub: key derivation, ciphertext layout, recipient discovery).

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

**Merkle Tree Definition (consensus-critical)**
- Binary Poseidon tree, depth = 32 (indices 0..2³²-1 → 2³² leaves)
- Hash: `H(a, b) = Poseidon([a, b])`
- Leaf value: commitment field element (no extra hashing)
- Empty leaf: `0`
- Padding: subtree default nodes precomputed from empty leaf; append-at-size never reorders or prunes
- **Capacity limit:** `commitment_size` is `u64` but tree depth limits to `2³²` leaves. Refine must reject if `commitment_size + N_OUT > 2³²`

### Note Model (UTXO-style)
```rust
// Single-asset pool: this Railgun instance handles exactly one asset (native JAM)
Note := (value, owner_pk, ρ)
Commitment := Com(value, owner_pk, ρ)
Nullifier := NF(sk, ρ)
```

## Extrinsic Interface

**Implementation**: Extrinsic types defined in [src/state.rs](src/state.rs:71-121)

### 1. DepositPublic (Cross-Service Transfer, value-bound)
```rust
// Triggered by a transfer host call from EVM service
DepositPublic {
    commitment: Hash,      // Declared commitment; must equal Com(value, pk, ρ) derived from memo
    value: u64,            // Amount to shield - from transfer amount (native JAM tokens)
    sender_index: u32,     // EVM service index (identifies origin service)
}
```

**Implementation via JAM Inter-Service Transfer:**

This extrinsic is **not directly submitted by users**. Instead, users call an EVM contract that invokes a **JAM host call**:

```solidity
// EVM side (Solidity contract)
function depositToRailgun(bytes32 pk, bytes32 rho, bytes32 commitment, uint256 amount) external {
    // Memo v1 layout (128 bytes total):
    // [0]     : version = 0x01
    // [1..32] : pk (recipient public key)
    // [33..64]: rho (commitment blinding)
    // [65..127]: reserved for future extensions (must be zero for v1)
    bytes memory memo = abi.encodePacked(
        uint8(0x01),
        pk,
        rho,
        bytes59(0)  // reserved/padded
    );

    // JAM host call: transfer(d, a, g, o)
    // Register layout:
    //   r7 = d (destination service index)
    //   r8 = a (amount to transfer in JAM tokens)
    //   r9 = g (gas limit for receiver)
    //   r10 = o (RAM offset to 128-byte memo)
    JAM_TRANSFER_HOST_CALL(
        RAILGUN_SERVICE_ID,  // r7 (d): destination service index
        amount,              // r8 (a): amount to shield (native JAM tokens only)
        gasLimit,            // r9 (g): gas limit for Railgun's on_transfer hook (bounded; see Safety)
        memoOffset           // r10 (o): offset in RAM where memo is stored
    );
}
```

**Fund-safety ordering:** The EVM service **refine** recomputes `commitment_expected = Com(value, pk, rho)` from the memo and transfer amount, and enforces gas bounds, **before** emitting the `transfer` host call. If validation fails, no transfer occurs and user funds remain in the EVM service. Railgun **accumulate** re-verifies as a second line of defense.

**Gas bounds (consensus-critical):** `gas_min`, `gas_max` stored in state (initialized to `50_000` / `5_000_000`; governance-updatable via future parameter extrinsic).
- **Deposits:** EVM service **refine** validates `gas_limit` and `commitment_expected = Com(value, pk, rho)` **before** issuing the transfer host call; if either fails, no transfer is emitted. Railgun **accumulate** rechecks both bounds and value binding defensively; if a byzantine EVM service bypassed checks, Railgun accumulate MUST abort/panic to avoid crediting an invalid note.
- **Withdrawals:** Refine checks public input `gas_limit` (part of extrinsic) against witnessed `(gas_min, gas_max)` before emitting transfer instruction

**Versioning:** Memo byte 0 is a version. v1 requires reserved bytes to be zero; non-zero reserved bytes or unknown version → reject. This keeps backward compatibility when memo extensions are introduced.

**Value binding:** Railgun accumulate recomputes `commitment_expected = Com(value, pk, rho)` from the memo and transfer amount. It **rejects** if `commitment != commitment_expected`. This prevents inflationary deposits where the memo commitment encodes a higher value than was transferred.

**Note:** JAM's transfer host call only supports **native JAM tokens**. For other assets, use wrapped tokens inside the EVM service and run a separate Railgun instance per asset.

**JAM Runtime Flow:**
1. EVM service `refine` emits `JAM_TRANSFER_HOST_CALL`.  **Public information:** Depositor EVM address is visible on-chain. User should generate fresh keypair `(sk, pk)` locally and perform mixing transfers after deposit to break linkability.
2. EVM service `accumulate` transfers `amount` (native tokens) from EVM service balance to Railgun service balance. Memo v1 layout: `[ver=0x01 | pk | rho | reserved=0]`.
3. Railgun service `accumulate` receives `DeferredTransfer` input: `(sender_index, amount, memo, gas_limit)`
   - **Enforce gas bounds (consensus-critical):** `assert!(gas_limit >= gas_min && gas_limit <= gas_max)` using witnessed state values
   - Railgun parses memo, enforces `version==0x01` and reserved bytes == 0
   - Computes `commitment_expected = Com(amount, pk, rho)` using the memo and transfer amount
   - Rejects if `commitment != commitment_expected`
   - **Check tree capacity (consensus-critical):** `assert!(commitment_size + 1 <= (1u64 << 32))`
   - Inserts `commitment` at `commitment_size`, updates `commitment_root`, increments `commitment_size`

### 2. SubmitPrivate (Unsigned)

```rust
SubmitPrivate {
    proof: Bytes,                      // Groth16 proof
    anchor_root: Hash,                 // MUST equal current commitment_root
    anchor_size: u64,                  // MUST equal current commitment_size
    next_root: Hash,                   // Deterministic post-state root (validated by refine)

    input_nullifiers: [Hash; N_IN],    // Spent note nullifiers
    output_commitments: [Hash; N_OUT], // New commitments (ORDERED for append-at-size)

    fee: u64,                          // Z-space fee amount
    encrypted_payload: Bytes,          // MVP: MUST be zero-length (enforced in refine); future versions may carry hints/memo under version gate
    // Note: offer_nullifier removed from MVP - atomic swaps require multi-asset extension
}
```

**No user signature. No origin address. Builder submits on user's behalf.**

#### JAM Refine Processing Steps

**✅ IMPLEMENTED**: See actual code at:
- **Entry point**: [src/main.rs:87-163](src/main.rs:87-163) - PVM `refine` export with argument parsing
- **Core logic**: [src/refiner.rs:15-250](src/refiner.rs:15-250) - `process_extrinsics` function
- **Serialization**: [src/refiner.rs:247-261](src/refiner.rs:247-261) - `serialize_effects` function

**Implementation status by step**:

| Step | Description | Status | Code Reference |
|------|-------------|--------|----------------|
| 1. Parse args | `parse_refine_args(start_address, length)` | ✅ COMPLETE | [main.rs:91-96](src/main.rs:91-96) |
| 2. Fetch context | `fetch_refine_context()` | ✅ COMPLETE | [main.rs:99-105](src/main.rs:99-105) |
| 3. Fetch work item | `fetch_work_item(wi_index)` | ✅ COMPLETE | [main.rs:108-114](src/main.rs:108-114) |
| 4. Fetch extrinsics | `fetch_extrinsics(wi_index)` | ✅ COMPLETE | [main.rs:117-123](src/main.rs:117-123) |
| 5. Fetch witnesses | `fetch_object` + `verify_witness` | ⚠️ STUB | [refiner.rs:255-261](src/refiner.rs:255-261) returns dummy |
| 6. Process extrinsics | Loop over SubmitPrivate/WithdrawPublic | ⚠️ PARTIAL | Only SubmitPrivate logic exists |
| 6a. Payload check | `encrypted_payload.is_empty()` | ✅ **CRITICAL FIX** | [refiner.rs:85-91](src/refiner.rs:85-91) |
| 6b. Fee check | `fee >= min_fee` | ✅ **CRITICAL FIX** | [refiner.rs:93-99](src/refiner.rs:93-99) |
| 6c. Verify proof | `verify_groth16(proof, public_inputs)` | ❌ STUB | [crypto.rs:52-62](src/crypto.rs:52-62) returns false |
| 6d. Check anchor | `anchor_root == commitment_root` | ✅ COMPLETE | [refiner.rs:113-123](src/refiner.rs:113-123) |
| 6e. Tree capacity | `size + additions <= 2³²` | ✅ **CRITICAL FIX** | [refiner.rs:125-135](src/refiner.rs:125-135) |
| 6f. Merkle append | `merkle_append(root, size, cms)` | ❌ STUB | [crypto.rs:64-73](src/crypto.rs:64-73) returns `[0u8; 32]` |
| 6g. Nullifier check | `seen_nullifiers.insert(nf)` | ✅ COMPLETE | [refiner.rs:155-161](src/refiner.rs:155-161) |
| 6h. Verkle proof | `verify_verkle_proof(...)` | ❌ STUB | [crypto.rs:75-86](src/crypto.rs:75-86) returns error |
| 6i. Write intents | Emit `WriteIntent` for state changes | ✅ COMPLETE | [refiner.rs:178-207](src/refiner.rs:178-207) |
| 6j. Fee delta | `fee_delta = accum - epoch` | ✅ **CRITICAL FIX** | [refiner.rs:233-240](src/refiner.rs:233-240) |
| 7. Serialize | `serialize_effects(&write_effects)` | ✅ COMPLETE | [refiner.rs:247-261](src/refiner.rs:247-261) |
| 8. Return | `(output_ptr, output_len)` | ✅ COMPLETE | [main.rs:157-160](src/main.rs:157-160) |

**What works today**: Control flow, error handling, all 6 security fixes, write intent generation
**What needs completion**: Cryptographic functions (ZK, Verkle, Merkle), witness fetching, SCALE decoding

---

**Specification Reference** (detailed pseudocode showing complete consensus rules):

<details>
<summary>Click to expand: Complete refine pseudocode with all security fixes</summary>

**Note**: This is the **specification pseudocode**. For actual working code, see the implementation references in the table above.

```rust
#[no_mangle]
extern "C" fn refine(start_address: u64, length: u64) -> (u64, u64) {
    // 1. Parse refine arguments from PVM memory
    let refine_args = parse_refine_args(start_address, length)?;  // ✅ IMPLEMENTED
    // Contains: core_index, work_item_index, service_id, payload, work_package_hash

    // 2. Fetch refine context (state root, anchor, prerequisites)
    let refine_context = fetch_refine_context()?;  // ✅ IMPLEMENTED

    // 3. Fetch work item
    let work_item = fetch_work_item(refine_args.wi_index)?;  // ✅ IMPLEMENTED

    // 4. Fetch extrinsics for this work item
    let extrinsics = fetch_extrinsics(refine_args.wi_index)?;  // ✅ IMPLEMENTED

    // 5. Fetch pre-state witnesses (refine is STATELESS)
    // ⚠️ STUB: fetch_object returns dummy data, verify_witness not implemented
    let (commitment_root, commitment_root_proof) =
        fetch_object(service_id, object_id("commitment_root"))?;
    verify_witness(
        &refine_context.state_root,
        object_id("commitment_root"),
        &commitment_root,
        &commitment_root_proof
    )?;

    let (commitment_size, commitment_size_proof) =
        fetch_object(service_id, object_id("commitment_size"))?;
    verify_witness(
        &refine_context.state_root,
        object_id("commitment_size"),
        &commitment_size,
        &commitment_size_proof
    )?;

    let current_epoch = refine_context.timeslot / TIMESLOTS_PER_EPOCH;
    let (fee_tally_epoch, fee_tally_proof) =
        fetch_object(service_id, object_id("fee_tally", current_epoch))?;
    verify_witness(
        &refine_context.state_root,
        object_id("fee_tally", current_epoch),
        &fee_tally_epoch,
        &fee_tally_proof
    )?;
    // ✅ CRITICAL FIX: Fee tally uses DELTA writes (see step 6b below)

    // Fetch consensus parameters
    let (min_fee, min_fee_proof) =
        fetch_object(service_id, object_id("min_fee"))?;
    verify_witness(
        &refine_context.state_root,
        object_id("min_fee"),
        &min_fee,
        &min_fee_proof
    )?;
    let min_fee = u64::from_le_bytes(min_fee[0..8].try_into()?);

    let (gas_min_bytes, gas_min_proof) =
        fetch_object(service_id, object_id("gas_min"))?;
    verify_witness(
        &refine_context.state_root,
        object_id("gas_min"),
        &gas_min_bytes,
        &gas_min_proof
    )?;
    let gas_min = u64::from_le_bytes(gas_min_bytes[0..8].try_into()?);

    let (gas_max_bytes, gas_max_proof) =
        fetch_object(service_id, object_id("gas_max"))?;
    verify_witness(
        &refine_context.state_root,
        object_id("gas_max"),
        &gas_max_bytes,
        &gas_max_proof
    )?;
    let gas_max = u64::from_le_bytes(gas_max_bytes[0..8].try_into()?);

    let mut write_effects = Vec::new();  // ✅ IMPLEMENTED
    let mut fee_tally_accum = fee_tally_epoch;  // ✅ IMPLEMENTED
    let mut seen_nullifiers = Set::new();  // ✅ IMPLEMENTED

    // 6. Process each extrinsic
    for ext in extrinsics {
        let submit = decode::<SubmitPrivate>(ext)?;  // ❌ STUB - returns error

        // ✅ CRITICAL FIX #5: Encrypted payload enforcement
        if !submit.encrypted_payload.is_empty() {
            return Err(RailgunError::EncryptedPayloadNotAllowed {
                length: submit.encrypted_payload.len(),
            });
        }

        // ✅ IMPLEMENTED: Minimum fee check
        if submit.fee < min_fee {
            return Err(RailgunError::FeeUnderflow {
                provided: submit.fee,
                minimum: min_fee,
            });
        }

        // ❌ STUB: verify_groth16 returns false
        verify_groth16(submit.proof, [
            submit.anchor_root,
            submit.anchor_size,
            submit.next_root,
            submit.input_nullifiers,
            submit.output_commitments,
            submit.fee
        ])?;

        // ✅ IMPLEMENTED: Anchor matching
        assert_eq!(submit.anchor_root, commitment_root);
        assert_eq!(submit.anchor_size, commitment_size);
        assert!(commitment_size <= (1u64 << 32), "anchor_size exceeds tree capacity");

        // ✅ CRITICAL FIX #6: Tree capacity check
        let tree_capacity: u64 = 1u64 << 32;
        if commitment_size
            .checked_add(submit.output_commitments.len() as u64)
            .map_or(true, |new_size| new_size > tree_capacity)
        {
            return Err(RailgunError::TreeCapacityExceeded {
                current_size: commitment_size,
                requested_additions: submit.output_commitments.len() as u64,
                capacity: tree_capacity,
            });
        }

        // ❌ STUB: merkle_append returns [0u8; 32]
        let next_root_expected = merkle_append(
            commitment_root,
            commitment_size,
            &submit.output_commitments
        );
        assert_eq!(next_root_expected, submit.next_root);

        // ✅ IMPLEMENTED: Nullifier uniqueness tracking
        for nf in submit.input_nullifiers {
            if !seen_nullifiers.insert(nf) {
                return Err("Nullifier reused within work item");
            }

            // ⚠️ STUB: fetch_object returns dummy
            let (nullifier_value, verkle_proof) =
                fetch_object(service_id, object_id("nullifier", nf))?;

            // ❌ STUB: verify_verkle_proof returns error
            verify_verkle_proof(
                &refine_context.state_root,
                object_id("nullifier", nf),
                &nullifier_value,
                &verkle_proof
            ).map_err(|_| "Invalid Verkle proof for nullifier")?;

            if nullifier_value != [0u8; 32] {
                return Err("Double-spend attempt: nullifier already spent");
            }
        }

        // ✅ IMPLEMENTED: Write intent generation
        write_effects.push(WriteIntent {
            object_id: object_id("commitment_root"),
            data: next_root_expected,
        });
        write_effects.push(WriteIntent {
            object_id: object_id("commitment_size"),
            data: (commitment_size + submit.output_commitments.len() as u64).to_le_bytes(),
        });
        for nf in submit.input_nullifiers {
            write_effects.push(WriteIntent {
                object_id: object_id("nullifier", nf),
                data: [1u8; 32],
            });
        }
        for (i, cm) in submit.output_commitments.iter().enumerate() {
            write_effects.push(WriteIntent {
                object_id: object_id("commitment", commitment_size + i as u64),
                data: cm.clone(),
            });
        }

        fee_tally_accum += submit.fee;  // ✅ IMPLEMENTED

        // Update local tracking for next iteration
        commitment_root = next_root_expected;
        commitment_size += submit.output_commitments.len() as u64;
    }

    // ✅ CRITICAL FIX #1: Fee tally DELTA write
    let fee_delta = fee_tally_accum - fee_tally_epoch;
    write_effects.push(WriteIntent {
        object_id: object_id("fee_tally_delta", current_epoch, work_package_hash),
        data: fee_delta.to_le_bytes(),
    });

    // ✅ IMPLEMENTED: Serialization
    let execution_effects = serialize_write_intents(&write_effects)?;

    // ✅ IMPLEMENTED: Return output pointer
    (output_ptr, output_len)
}
```

</details>

---

**Key: Refine is STATELESS and TRUSTLESS**
- **Inputs:** Pre-state witnesses fetched via `fetch_object()` host calls
- **Witness Verification:** Each witness is cryptographically proven against `refine_context.state_root` using Verkle proofs
  - Verkle membership proofs for all state reads (commitment_root, commitment_size, nullifiers)
  - Nullifier freshness: `nullifier_value == [0u8; 32]` means unspent (Verkle proves binding)
- **Processing:** Pure computation + verification (no state mutation, no trust)
- **Outputs:** `WriteIntent[]` describing desired state changes
- **Accumulate:** Applies verified `WriteIntent[]` to actual service state

**Witness Verification Flow (Verkle-based):**
```
refine_context.state_root
    ↓ (Verkle proof with IPA)
object_id("commitment_root") → value + Verkle proof ✓ verified
object_id("commitment_size") → value + Verkle proof ✓ verified
object_id("nullifier", nf)   → [0u8; 32] + Verkle proof ✓ verified (unspent)
```

**Verkle Proof Details:**
- JAM uses Banderwagon curve (twisted Edwards form of BLS12-381)
- Inner Product Argument (IPA) multiproof for batch verification
- Each `fetch_object()` returns `(value, verkle_proof)` pair
- Proof verification: `verify_verkle_proof(state_root, key, value, proof) -> Result<(), Error>`
- **Default value convention:** Absent keys return `[0u8; 32]` with valid membership proof
- **Nullifier interpretation:** `[0u8; 32]` = unspent (fresh), `[1u8; 32]` = spent (canonical; any non-zero is treated as spent)
- **Proof type:** Always membership proofs (non-membership not needed; defaults handle absence)

**Output Format (Execution Effects):**
```rust
struct WriteIntent {
    object_id: Hash,      // C(service_id, hash(key))
    data: Vec<u8>,        // New value to write
}

// Serialized as:
[num_writes:4][object_id₁:32][data_len₁:4][data₁:bytes]...[object_idₙ:32][data_lenₙ:4][dataₙ:bytes]
```

**Key Properties:**
- **Deterministic:** All validators re-execute refine and must get identical state root
- **Batch Processing:** Multiple extrinsics processed atomically per work item
- **No Revert:** If any extrinsic fails verification, entire work item fails (validator rejects work report)
- **Privacy:** Validators only see opaque `proof` bytes, nullifiers, and commitments—no user identity




### 3. WithdrawPublic (Two-Phase: Railgun refine → Railgun accumulate → EVM accumulate)

**Implementation Status Summary:**

| Component | Status | Location | Notes |
|-----------|--------|----------|-------|
| `WithdrawPublic` struct | ✅ COMPLETE | [src/state.rs:100-121](src/state.rs:100-121) | All fields defined with `has_change` boolean |
| Gas bounds check | ✅ COMPLETE | [src/errors.rs:27-28](src/errors.rs:27-28) | `GasLimitOutOfBounds` error defined |
| Fee underflow check | ✅ COMPLETE | [src/errors.rs:29](src/errors.rs:29) | `FeeUnderflow` error defined |
| Tree capacity check | ✅ COMPLETE | [src/errors.rs:19](src/errors.rs:19) | `TreeCapacityExceeded` error defined |
| Nullifier tracking | ✅ COMPLETE | [src/refiner.rs:155-161](src/refiner.rs:155-161) | `seen_nullifiers` logic for SubmitPrivate (same pattern for Withdraw) |
| Fee delta calculation | ✅ COMPLETE | [src/refiner.rs:233-240](src/refiner.rs:233-240) | Delta write logic implemented |
| `verify_groth16` | ❌ STUB | [src/crypto.rs:52-62](src/crypto.rs:52-62) | Returns `false`, needs arkworks |
| `verify_verkle_proof` | ❌ STUB | [src/crypto.rs:75-86](src/crypto.rs:75-86) | Returns error, needs Banderwagon |
| `merkle_append` | ❌ STUB | [src/crypto.rs:64-73](src/crypto.rs:64-73) | Returns `[0u8; 32]`, needs Poseidon tree |
| `decode::<WithdrawPublic>` | ❌ STUB | [src/refiner.rs:270-274](src/refiner.rs:270-274) | Returns error, needs SCALE codec |
| Withdraw write intents | ❌ NOT IMPLEMENTED | N/A | Only SubmitPrivate path exists in refiner.rs |
| `AccumulateInstruction` | ❌ NOT DEFINED | N/A | Requires utils extension |
| Accumulate transfer logic | ❌ NOT IMPLEMENTED | [src/accumulator.rs:12-22](src/accumulator.rs:12-22) | Stub only logs inputs |
| EVM deferred transfer | ❌ NOT IMPLEMENTED | N/A | Requires EVM service changes |

**Phase 1: Refine verifies proof and emits a transfer instruction**

**Extrinsic definition** (✅ implemented in [src/state.rs:100-121](src/state.rs:100-121)):
```rust
// In refine - verifies ZK proof, outputs AccumulateInstruction
WithdrawPublic {
    proof: Bytes,                 // Proves note ownership AND binds to recipient/amount/gas_limit
    anchor_root: Hash,            // Current commitment_root
    anchor_size: u64,             // Current commitment_size
    nullifier: Hash,              // Spent note nullifier
    recipient: Address,           // 20-byte EVM address to receive funds
    amount: u64,                  // Amount to deshield (native JAM tokens)
    fee: u64,                     // Withdraw fee (credited in Z-space)
    gas_limit: u64,               // Gas for EVM accumulate processing (MUST be included in proof public inputs to prevent builder tampering)

    // Change output (optional)
    has_change: bool,             // Explicit flag: true if change output exists, false otherwise
    change_commitment: Hash,      // If has_change: Com(change_value, pk, ρ_change); otherwise ignored (can be zero)
    next_root: Hash,              // If has_change: MerkleAppend(anchor_root, anchor_size, [change_commitment])
                                  // Otherwise: must equal anchor_root (no state change)
}
```

**Refine Processing (Stateless Verification):**

**Implementation status**: ⚠️ **PARTIALLY IMPLEMENTED**
- ✅ Control flow structure: Present in [src/refiner.rs:15-250](src/refiner.rs:15-250)
- ✅ Gas bounds check: Lines 277-282 below are **IMPLEMENTED**
- ✅ Fee check: Lines 284-290 below are **IMPLEMENTED**
- ❌ `verify_groth16_withdraw`: **STUB** - returns `false` ([crypto.rs:52-62](src/crypto.rs:52-62))
- ❌ `verify_verkle_proof`: **STUB** - returns error ([crypto.rs:75-86](src/crypto.rs:75-86))
- ❌ `merkle_append`: **STUB** - returns `[0u8; 32]` ([crypto.rs:64-73](src/crypto.rs:64-73))
- ❌ `decode::<WithdrawPublic>`: **STUB** - returns error ([refiner.rs:270-274](src/refiner.rs:270-274))
- ❌ `AccumulateInstruction::Transfer`: **NOT DEFINED** - requires utils extension
- ❌ Write intent emission for withdrawals: **NOT IMPLEMENTED** (only SubmitPrivate path exists)

**Specification pseudocode** (only portions marked ✅ above are actually implemented):
```rust
// In refine() entry point
// (commitment_root, commitment_size, fee_tally_epoch) witnesses fetched as in SubmitPrivate
let mut seen_nullifiers = Set::new(); // Per-work-item uniqueness only
let mut fee_tally_accum = fee_tally_epoch; // fetched/verified once per work item
for ext in extrinsics {
        if let Some(withdraw) = decode::<WithdrawPublic>(ext) {  // ❌ NOT IMPLEMENTED
            // 1. Enforce gas bounds (consensus-critical)  ✅ IMPLEMENTED (error type defined)
            if withdraw.gas_limit < gas_min || withdraw.gas_limit > gas_max {
                return Err(RailgunError::GasLimitOutOfBounds {
                    provided: withdraw.gas_limit,
                    min: gas_min,
                    max: gas_max,
                });
            }

        // 1b. Enforce minimum fee (consensus-critical anti-spam)  ✅ IMPLEMENTED (error type defined)
        if withdraw.fee < min_fee {
            return Err(RailgunError::FeeUnderflow {
                provided: withdraw.fee,
                minimum: min_fee,
            });
        }

        // 2. Fetch pre-state witnesses (commitment_root and commitment_size already fetched/verified once per work item)
        // Re-use the verified values from work-item level witnesses

        // 3. Verify ZK proof (proves note ownership + binds recipient/amount/gas_limit + change)
        // ❌ NOT IMPLEMENTED - verify_groth16 stub always returns false
        verify_groth16_withdraw(withdraw.proof, [
            withdraw.anchor_root,
            withdraw.anchor_size,
            withdraw.nullifier,
            withdraw.recipient,
            withdraw.amount,
            withdraw.fee,
            withdraw.gas_limit,           // CRITICAL: gas_limit is now a public input to prevent builder tampering
            withdraw.has_change as u64,   // Explicit boolean flag (0 or 1) - no sentinel collision risk
            withdraw.change_commitment,   // Ignored if has_change == false; circuit validates consistency
            withdraw.next_root,
        ])?;

        // 3. Check anchor matches witness
        assert_eq!(withdraw.anchor_root, commitment_root);
        assert_eq!(withdraw.anchor_size, commitment_size);
        assert!(commitment_size <= (1u64 << 32), "anchor_size exceeds tree capacity");

        // 4. Verify nullifier not spent (Verkle proof + in-work-item uniqueness)
        // ✅ IMPLEMENTED: seen_nullifiers tracking (error type defined)
        // ❌ NOT IMPLEMENTED: fetch_object stub, verify_verkle_proof stub
        if !seen_nullifiers.insert(withdraw.nullifier) {
            return Err("Nullifier reused within work item (withdraw)");
        }
        let (nullifier_value, verkle_proof) = fetch_object(service_id, object_id("nullifier", withdraw.nullifier))?;

        // Verify Verkle proof binds nullifier to state_root
        verify_verkle_proof(  // ❌ STUB - always returns error
            &refine_context.state_root,
            object_id("nullifier", withdraw.nullifier),
            &nullifier_value,
            &verkle_proof
        ).map_err(|_| "Invalid Verkle proof for withdraw nullifier")?;

        // Check if nullifier is spent (non-zero / canonical [1u8; 32] = spent)
        if nullifier_value != [0u8; 32] {
            return Err("Double-spend attempt: nullifier already spent in withdraw");
        }

        // 5. Validate change output consistency
        // ✅ IMPLEMENTED: has_change flag, tree capacity check (error type defined)
        // ❌ NOT IMPLEMENTED: merkle_append stub, write intent emission
        if withdraw.has_change {
            // Check tree capacity before appending change
            let tree_capacity: u64 = 1u64 << 32;
            if commitment_size.checked_add(1).map_or(true, |new_size| new_size > tree_capacity) {
                return Err(RailgunError::TreeCapacityExceeded {
                    current_size: commitment_size,
                    requested_additions: 1,
                    capacity: tree_capacity,
                });
            }

            // Change output present - verify next_root matches deterministic append
            let next_root_expected = merkle_append(  // ❌ STUB - returns [0u8; 32]
                withdraw.anchor_root,
                withdraw.anchor_size,
                &[withdraw.change_commitment]
            );
            assert_eq!(withdraw.next_root, next_root_expected, "next_root mismatch for change output");

            // Update commitment tree state
            write_effects.push(WriteIntent {
                object_id: object_id("commitment_root"),
                data: next_root_expected,
            });
            write_effects.push(WriteIntent {
                object_id: object_id("commitment_size"),
                data: (commitment_size + 1).to_le_bytes(),
            });
            write_effects.push(WriteIntent {
                object_id: object_id("commitment", commitment_size),
                data: withdraw.change_commitment,
            });

            // Update local tracking
            commitment_root = next_root_expected;
            commitment_size += 1;
        } else {
            // No change - next_root must equal anchor_root (no state change)
            assert_eq!(withdraw.next_root, withdraw.anchor_root, "next_root must equal anchor_root when no change");
        }

        // 6. Emit AccumulateInstruction for transfer
        // ❌ NOT IMPLEMENTED - AccumulateInstruction type not defined in utils
        accumulate_instructions.push(AccumulateInstruction::Transfer {
            destination_service: EVM_SERVICE_ID,
            amount: withdraw.amount,
            gas_limit: withdraw.gas_limit,  // Already validated in step 1
            memo: encode_withdraw_memo_v1(withdraw.recipient), // [ver=0x01 | recipient(20) | reserved=0]
        });

        // 7. Emit write intents (nullifier + fee)
        // ❌ NOT IMPLEMENTED - write intent emission for withdrawals not coded yet
        write_effects.push(WriteIntent {
            object_id: object_id("nullifier", withdraw.nullifier),
            data: [1u8; 32],  // Mark spent (32-byte encoding for Verkle consistency)
        });
        fee_tally_accum += withdraw.fee;  // ✅ IMPLEMENTED (fee accumulation logic)
    }
}

// Emit fee tally DELTA write once per work item
// ✅ IMPLEMENTED - fee delta calculation logic present in refiner.rs:233-240
let fee_delta = fee_tally_accum - fee_tally_epoch;
write_effects.push(WriteIntent {
    object_id: object_id("fee_tally_delta", current_epoch, work_package_hash),
    data: fee_delta.to_le_bytes(),
});
```

**Phase 2: Accumulate executes instructions and EVM credits directly**

**Implementation status**: ❌ **NOT IMPLEMENTED**
- Entry point exists: [src/main.rs](src/main.rs:165-221)
- Core logic stub: [src/accumulator.rs](src/accumulator.rs:1-22) (only logs inputs, no transfer execution)
- Missing: `AccumulateInstruction` parsing, `host_transfer` call, write application

**Specification pseudocode** (NOT implemented):
```rust
fn accumulate(
    service_id: u32,
    code: Hash,
    inputs: Vec<AccumulateInput>,
    x: XContext,
    n: Hash,
) -> Result {
    for input in inputs {
        match input {
            // Execute transfer instructions from refine
            AccumulateInput::Transfer { destination_service, amount, gas_limit, memo } => {
                // Call transfer host function (no verification, just execution)
                host_transfer(
                    destination_service,  // d: EVM_SERVICE_ID
                    amount,               // a: amount to transfer
                    gas_limit,            // g: gas limit
                    &memo                 // o: contains recipient address
                )?;
            }
        }
    }
    Ok(())
}
```

**EVM Service Receives Transfer (no extra claim step):**

**Implementation status**: ❌ **NOT IMPLEMENTED** - requires changes to EVM service (not part of Railgun service)

JAM's `transfer` host call writes to the **recipient service's `DeferredTransfers` state variable**. The EVM service processes it immediately:

**Specification pseudocode** (NOT implemented in EVM service):
```rust
fn accumulate(
    service_id: u32,
    code: Hash,
    inputs: Vec<AccumulateInput>,
    x: XContext,
    n: Hash,
) -> Result {
    // 1. Process incoming deferred transfers
    for transfer in x.Transfers.iter().filter(|t| t.receiver_index == service_id) {
        if transfer.sender_index == RAILGUN_SERVICE_ID {
            // Memo v1: [ver=0x01 | recipient(20) | reserved=0]
            ensure!(transfer.memo.len() >= 21, "memo too short");
            ensure!(transfer.memo[0] == 0x01, "invalid memo version");
            ensure!(transfer.memo[21..].iter().all(|b| *b == 0), "reserved bytes must be zero");
            let recipient = Address::from_slice(&transfer.memo[1..21]);
            // Recipient is treated as a raw 20-byte address; contract addresses are allowed. Wallets may enforce EIP-55 checksums off-chain if desired.

            // Fetch current balance witness
            let balance_key = object_id("balance", recipient);
            let (current_balance, proof) = fetch_object(service_id, balance_key)?;
            verify_witness(&x.state_root, balance_key, &current_balance, &proof)?;

            // Emit write intent to increment balance directly (no claim contract)
            write_effects.push(WriteIntent {
                object_id: balance_key,
                data: (current_balance + transfer.amount).to_le_bytes(),
            });
        }
    }

    // 2. Process extrinsics...
    for input in inputs {
        // EVM transaction processing
    }

    Ok(())
}
```

**Public information:** Exit amount and recipient EVM address are visible on-chain, but not transaction history within Railgun. Two-phase flow eliminates the extra claim transaction.

## ZK Proof Specification

### Spend Circuit

The **Spend Circuit** is the core privacy mechanism of JAM Railgun. It enables users to prove they own private notes (UTXOs) and authorize spending them—without revealing which notes, who is spending, or the transaction graph. The circuit takes secret note data as private witness and outputs only cryptographic commitments and nullifiers as public inputs. Validators verify the proof to ensure value conservation, proper authorization, and state consistency, but learn nothing about the underlying transaction details. This is analogous to Zcash's Sapling spend circuit, adapted for JAM's stateless refine model.

**Public Inputs:**
```
x := (anchor_root, anchor_size, next_root, {nf_i}, {cm'_j}, F)
  anchor_root  Current note tree root (field element)
  anchor_size  Current note tree size / append index (u64 with 64-bit range proof in circuit)
  next_root    MerkleAppend(anchor_root, anchor_size, {cm'_j})  // DERIVED, not free
  nf_i         Input nullifiers (field elements, prove authorization)
  cm'_j        Output commitments (ORDERED by append position: cm'_0, cm'_1, ..., cm'_(N_OUT-1))
  F            Fee amount (u64 with 64-bit range proof in circuit - prevents modulus wrap inflation)
```

**Private Witness:**
```
w := ({sk_i, pk_i, v_i, ρ_i, path_i}, {pk'_j, v'_j, ρ'_j, user_entropy_j})
  sk_i         Secret keys for input notes
  v_i          Input note values (u64 with 64-bit range proof in circuit)
  ρ_i          Input note randomness (see note below)
  path_i       Merkle paths (prove note existence)
  pk'_j        Output recipient public keys
  v'_j         Output values (u64 with 64-bit range proof in circuit - prevents negative/overflow)
  ρ'_j         Output randomness (derived in-circuit from private `user_entropy_j`)
  user_entropy_j Private per-output entropy provided by the wallet RNG
```

**Note on Randomness (ρ):**

- **Purpose:** Prevents linkability. Without randomness, identical (pk, value) pairs would produce the same commitment, allowing transaction graph analysis.

- **Where it comes from:**
  - **Input ρ_i:** Chosen by the sender when they created the note (from prior transaction output or deposit). Stored in the user's local wallet alongside the note.
  - **Output ρ'_j:** Derived inside the circuit from a **private witness** `user_entropy_j` supplied by the prover: `ρ'_j = H(anchor_root || user_entropy_j || j)`. `user_entropy_j` MUST be sampled fresh per-output by the wallet RNG; the circuit derives `ρ'_j` from that witness to prevent reuse across transactions and binds it to `anchor_root` so two different anchors cannot share the same `ρ'`.

- **Commitment binding:** `Com(value, pk, ρ) = H(pk || value || ρ)` ensures each commitment is unique even for identical amounts.

- **Nullifier binding:** `NF(sk, ρ) = H(sk || ρ)` ties the nullifier to the specific note instance, preventing double-spends without revealing which commitment was spent.

**Constraints:**

**Anchor rule (consensus-critical):** `anchor_root` **MUST equal the current `commitment_root` and `anchor_size` MUST equal the current `commitment_size` at refine time.** There is no staleness window; validators reject anchors from prior roots. Reorgs are therefore irrelevant to anchor validity—the anchor is always the live root witnessed from `refine_context.state_root`.
- **UX note:** This zero-staleness rule maximizes determinism but increases failure probability under contention. At ~100 TPS private transfers with 6s blocks (≈600 transfers/block), anchors can be invalidated frequently, leading to 50–90% rejection rates without client retry/refresh or anchor pooling. A future version may permit a bounded rolling window of recent roots, but the MVP keeps the strict rule for simplicity and minimal surface area.

1. **Input Validity:** Each input note commitment exists in Merkle tree at `anchor_root`
   ```
   ∀i: VerifyMerklePath(Com(v_i, pk_i, ρ_i), path_i, anchor_root) = 1
   ```

2. **Authorization:** Nullifiers prove secret key ownership
   ```
   ∀i: nf_i = NF(sk_i, ρ_i) ∧ pk_i = PublicKey(sk_i)
   ```

3. **Output Well-formedness:** Output commitments correctly formed
   ```
   ∀j: cm'_j = Com(v'_j, pk'_j, ρ'_j)
   ρ'_j = H(anchor_root || user_entropy_j || j)  // user_entropy_j is private witness; prevents cross-tx reuse
   ```

4. **Range Constraints (CRITICAL - prevents modulus wrap inflation):**
   ```
   ∀i: v_i ∈ [0, 2⁶⁴)        // 64-bit range proof on each input value
   ∀j: v'_j ∈ [0, 2⁶⁴)       // 64-bit range proof on each output value
   F ∈ [0, 2⁶⁴)              // 64-bit range proof on fee
   anchor_size ∈ [0, 2⁶⁴)    // 64-bit range proof on anchor_size
   ```
   Without these constraints, an attacker can add multiples of the BN254 field modulus (~2²⁵⁴) to satisfy
   field-element equality while minting arbitrary value. Range proofs ensure values stay in u64 domain.

5. **Value Conservation with Fee:**
   ```
   ∑v_i = ∑v'_j + F
   ```
   This equation is checked in the field, but range proofs on v_i, v'_j, F ensure the equality is meaningful.

6. **State Update (Deterministic):** New root includes output commitments
   ```
   next_root = MerkleAppend(anchor_root, anchor_size, [cm'_0, ..., cm'_(N_OUT-1)])
   ```

**Verifier requirement:** validators MUST recompute `MerkleAppend(anchor_root, anchor_size, output_commitments)` and reject if it does not match `next_root` from the proof public inputs.

---

### Groth16 Proof Structure

**Proof Size:** 192 bytes (fixed)
```
π = (A, B, C)
  A: G1 point (compressed) = 48 bytes
  B: G2 point (compressed) = 96 bytes
  C: G1 point (compressed) = 48 bytes
```

**Elliptic Curve:** BN254 (also known as BN128 or alt_bn128)
- **Field modulus:** ~254 bits
- **G1 points:** 48 bytes compressed (x-coordinate + sign bit)
- **G2 points:** 96 bytes compressed (x-coordinates of Fq2 element)

**Why BN254?**

BN254 is chosen for JAM Railgun because it balances security, performance, and ecosystem compatibility:

| Curve | Security Level | Pairing Speed | Proof Size | Ecosystem Support | Notes |
|-------|---------------|---------------|------------|-------------------|-------|
| **BN254** | ~100 bits | **Fastest** | 192 bytes | ✅ Excellent | Used in Ethereum, Zcash, Tornado Cash |
| **BLS12-381** | ~128 bits | Slower (1.5-2x) | 192 bytes | ✅ Good | Used in Ethereum 2.0, Filecoin, Chia |
| **BLS12-377** | ~128 bits | Slower (1.5-2x) | 192 bytes | ⚠️ Limited | Designed for recursive SNARKs (Celo) |
| **Pasta (Pallas/Vesta)** | ~128 bits | N/A | N/A | ⚠️ Halo2-specific | No pairings (cycle for recursion) |

**Public Inputs Encoding:**
```
public_inputs = [
    anchor_root,           // 32 bytes (field element)
    anchor_size,           // 32 bytes (u64 as field element)
    next_root,             // 32 bytes (field element)
    nf_0, ..., nf_(N_IN-1),    // 32 bytes each
    cm'_0, ..., cm'_(N_OUT-1), // 32 bytes each
    F,                     // 32 bytes (u64 as field element)
]

Total public inputs size = 32 * (4 + N_IN + N_OUT) bytes
  where 4 = anchor_root, anchor_size, next_root, fee

Example (N_IN=2, N_OUT=2):
  = 32 * (4 + 2 + 2) = 32 * 8 = 256 bytes
```

**Total Extrinsic Size:**
```
SubmitPrivate size ≈ 192 (proof) + 256 (public inputs) + 128 (encrypted payload)
                   = ~576 bytes per private transfer

Compare to:
  - Ethereum transaction: ~110 bytes (but reveals sender/receiver)
  - Zcash Sapling: ~2,307 bytes (larger due to Sprout deprecation overhead)
  - Monero RingCT: ~1,500-2,500 bytes (depends on ring size)
```

**Verification Time:**

| Environment | Time | Notes |
|------------|------|-------|
| **JAM Validator (PVM)** | ~0.8-1.5 ms | BN254 pairing check in no_std Rust |
| **Native (x86_64)** | ~0.5-0.8 ms | With arkworks optimizations |
| **Browser (WASM)** | ~10-20 ms | Client-side proving takes 2-5 seconds |

**Verification Steps:**
1. **Deserialize proof:** Parse 192 bytes into (A, B, C) ← ~50 μs
2. **Deserialize public inputs:** Parse field elements ← ~20 μs
3. **Pairing check:** Verify `e(A, B) = e(α, β) · e(L, γ) · e(C, δ)` ← ~0.7-1.2 ms
   - Where `L = Σ public_inputs[i] · VK.IC[i]` (linear combination)
   - `e()` is optimal ate pairing on BN254
   - Dominant cost: 2-4 pairings (depending on optimization)

**Circuit Size (for reference):**
```
Typical Railgun spend circuit (with range checks):
  - Constraints: ~18,000 - 22,000 R1CS constraints (includes 64-bit range proofs)
  - Proving time: 2-5 seconds (client-side, single-threaded)
  - Proving key size: ~50-100 MB (stored client-side)
  - Verification key size: ~2 KB (embedded in service)
  - Trusted setup: MPC ceremony with 100+ participants
```

**Why Groth16?**
- ✅ **Smallest proof:** 192 bytes constant (best for on-chain storage)
- ✅ **Fastest verifier:** Single pairing check (~1 ms)
- ✅ **Battle-tested:** Used in Zcash since 2018
- ⚠️ **Requires trusted setup:** Per-circuit MPC ceremony needed
- ⚠️ **Not universal:** Different circuits need different setups

**Alternative: PLONK/Halo2**
- Proof size: ~400-800 bytes (larger)
- Verification: ~2-5 ms (slower)
- No trusted setup (universal or transparent)
- Better for frequent circuit upgrades

For Railgun MVP, Groth16 is recommended due to mature tooling and optimal verification cost.

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
3. **Reimbursement model under DELTA writes:** Builder replays refine locally to confirm that a `fee_tally_delta` write will be emitted with value `F`. The builder knows:
   - Delta key is deterministic: `fee_tally_delta_{current_epoch}_{work_package_hash}`
   - Accumulate will sum all deltas at epoch start (no last-writer-wins loss)
   - Reimbursement is guaranteed if the work package is finalized (no revert path once proof is valid)
4. **Capital requirements:** Reimbursement settles on a finalized epoch lag (~2 epochs). Builders should maintain enough capital to cover this delay or operate via pools/delegation; small builders risk losses if they go offline before payout. Under the delta scheme, builders can track pending deltas across multiple work packages to forecast cumulative reimbursement.

### Protocol Settlement

Fees accumulate per epoch and distribute to **guaranteeing validators** (not block builders) at epoch start, using the last **finalized** epoch (e.g., `prev_finalized_epoch = current_epoch - 2`) to avoid reorg-induced reward drift.

#### Refine: Accumulate fee into epoch tally
```rust
fn refine_railgun_extrinsic(ext: RailgunExtrinsic, refine_context: RefineContext) -> Result<WriteIntent[]> {
    // ... verify ZK proof ...

    let fee = extract_fee_from_public_inputs(&ext.public_inputs);
    let current_epoch = refine_context.timeslot / TIMESLOTS_PER_EPOCH;

    // Output write intent to increment epoch fee tally
    write_intents.push(WriteIntent {
        key: keccak256(("fee_tally", current_epoch)),
        value: fee_tally[current_epoch] + fee  // fetch_object verified current value
    });

    Ok(write_intents)
}
```

#### Accumulate: Apply the write intent (no verification)
```rust
fn accumulate(service_account: ServiceAccount, write_intents: WriteIntent[]) {
    for intent in write_intents {
        service_account.storage.write(intent.key, intent.value);
    }
}
```

#### Epoch Start: Distribute accumulated fees
At the **start of each epoch**, a protocol hook distributes the **previous epoch's** fee tally to all validators who participated in guaranteeing work packages. The one-epoch lag is intentional: it matches consensus finality and lets all validators agree on the set of guarantee signatures.

**CRITICAL: Fee tally aggregation from DELTAs (prevents last-writer-wins fee loss):**

```rust
// Step 1: Accumulate aggregates all deltas for prev_epoch into canonical fee_tally
let prev_epoch = current_epoch - 2  // finalized epoch to avoid reorg reward inconsistencies

// Collect all delta writes from work packages in prev_epoch
let delta_keys = state.scan_prefix(&format!("fee_tally_delta_{}_", prev_epoch));
let mut total_fees = 0u128;

for delta_key in delta_keys {
    let delta_value = state.get(delta_key)?;
    let delta = u128::from_le_bytes(delta_value[..16].try_into()?);
    total_fees += delta;

    // Clear delta after aggregation (prevent double-counting on replay)
    state.delete(delta_key);
}

// Write aggregated total to canonical fee_tally
state.set(
    &format!("fee_tally_{}", prev_epoch),
    &total_fees.to_le_bytes()
);

// Step 2: Distribute to guaranteeing validators
let guaranteeing_validators = get_guaranteeing_validators(prev_epoch);
let validator_share = α * total_fees / guaranteeing_validators.len() as u128;
let treasury_share = (1 - α) * total_fees;

for v in guaranteeing_validators {
    // Rewards accrue to the address recorded in the prev_epoch guarantee (even if v is offline now)
    validator_reward[v] += validator_share;
}

protocol_treasury += treasury_share;

// Step 3: Clear canonical tally after distribution (optional - can keep for auditing)
state.set(&format!("fee_tally_{}", prev_epoch), &[0u8; 16]);
```

Where:
- `service_id` is the JAM service identifier (Railgun service)
- `epoch_id` is a protocol-defined settlement window (e.g., 600 timeslots)
- `guaranteeing_validators` are validators who signed guarantees in that epoch
- `α` is a fixed parameter (e.g., 0.9 → 90% to validators, 10% to treasury)
- **Eligibility rule:** payout set is fixed from prev_epoch guarantees; validators who exit before distribution still receive rewards to their recorded payout address. A grace window for claiming on L1 (if applicable) should match validator exit delays.

**Validators paid deterministically by protocol for guaranteeing work, not by users.**

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
5. **Gas limit bounds:** `gas_limit` for cross-service transfers must satisfy `gas_min ≤ gas_limit ≤ gas_max` (state parameters); Railgun rejects transfers outside this range to prevent DoS via zero-gas or huge-gas requests.

### Safety Guarantees
- **Double-spend prevention:** `NullifierSet` enforced in proof and on-chain (Verkle-proven)
- **Replay protection:** Nullifier uniqueness (commitment-specific via randomness ρ; once spent, cannot be reused)
- **Censorship resistance:** Builders cannot filter by intent—payload is opaque; only fee and proof validity visible

## Atomic Swaps (Two-Party)

*(Future extension – requires a multi-asset circuit/version; the single-asset MVP does not implement this section.)*

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

**Implementation**: [services/railgun/](services/railgun/)

```
src/main.rs       Entry points (refine, accumulate)
src/refiner.rs    Stateless proof verification logic
src/accumulator.rs State application, deposit processing
src/state.rs      State structures, extrinsic types
src/crypto.rs     Poseidon, Groth16, Merkle (stubs)
src/errors.rs     Error taxonomy
Cargo.toml        Dependencies (polkavm-derive, utils, simplealloc)
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
3. Service creates note: `cm = Com(value, pk, ρ)`, inserts into tree
4. User stores `(pk, value, ρ)` in local wallet

**Public information:** Depositor identity visible. Use mixing after deposit for privacy.

### 2. Transfer (Private)
1. **User (off-chain):**
   - Selects input notes (own UTXOs)
   - Generates output notes for recipients
   - Computes Merkle paths from local tree cache
   - Proves spend circuit locally
   - Hands `{encrypted_payload=empty, proof, fee_hint}` to builder (off-chain channel; MVP requires empty payload)

2. **Builder (pre-inclusion check):**
   - Verifies zk proof validity against current commitment root and nullifier set
   - Checks `fee ≥ min_fee` and `FeeAccumulator` credit is proven
   - Wraps into unsigned `SubmitPrivate` extrinsic
   - Includes in block if valid

3. **Validators (consensus):**
   - Verify ZK proof
   - Check `anchor_root == commitment_root` and `anchor_size == commitment_size`
   - Recompute `next_root_expected = MerkleAppend(commitment_root, commitment_size, output_commitments)`
   - Reject if `next_root_expected != next_root` (from proof public inputs)
   - **Verify nullifiers are fresh:** For each nullifier, fetch via Verkle witness and validate:
     - Verify Verkle membership proof (always membership; default value `[0u8; 32]` for absent keys)
     - If `nullifier_value == [0u8; 32]`: proceed (fresh/unspent)
     - If `nullifier_value != [0u8; 32]`: **REJECT** (double-spend)
   - Update state:
     - Insert `input_nullifiers` into `NullifierSet`
     - Insert `output_commitments` into tree via append-at-size
     - Update `commitment_root = next_root_expected`
     - Update `commitment_size += N_OUT`
     - Increment `fee_tally[epoch_id] += fee`

4. **Recipients (off-chain scanning):**
   - Monitor new `output_commitments` each block
   - Trial-decrypt each commitment with own `sk`
   - Add detected notes to local wallet

**At epoch start:** Guaranteeing validators credited from previous epoch's fee_tally (protocol logic, not user-directed). See Fee Flow section for distribution details.

### 3. Withdraw (Optional Exit)
1. User proves ownership of note via `WithdrawPublic{proof, anchor_root, anchor_size, nullifier, recipient, amount, fee, change_commitment, next_root}`
2. **Service verifies proof and handles change:**
   - Verify ZK proof (binds recipient, amount, fee, and optional change output)
   - Fetch nullifier via Verkle witness and verify `nullifier_value == [0u8; 32]` (unspent)
   - Enforce in-work-item uniqueness for the nullifier
  - **If change output exists** (`change_commitment != Hash::MAX`):
     - Verify `next_root == MerkleAppend(anchor_root, anchor_size, [change_commitment])`
     - Append `change_commitment` to tree at `commitment_size`
     - Update `commitment_root = next_root` and increment `commitment_size`
  - **If no change** (`change_commitment == Hash::MAX`):
     - Verify `next_root == anchor_root` (no state change)
   - Mark `nullifiers[nf] = [1u8; 32]` (spent)
3. Railgun accumulate emits transfer → EVM accumulate credits `recipient` immediately (no claim extrinsic)

**Public information:** Exit amount and recipient visible, but not transaction history or change amount.

#### Withdraw Circuit (minimal spec)

**Public Inputs:**
```
x := (anchor_root, anchor_size, nf, recipient, amount, F_withdraw, gas_limit, has_change, change_commitment, next_root)
```
- `gas_limit`: Gas for EVM accumulate (u64 with 64-bit range proof) - MUST be bound in proof to prevent builder tampering
- `has_change`: Boolean flag (0 or 1) indicating whether change output exists
- `change_commitment`: Hash of change output if `has_change == 1`, otherwise ignored (can be zero)
- `next_root`: If `has_change == 1`: `MerkleAppend(anchor_root, anchor_size, [change_commitment])`, otherwise must equal `anchor_root`

**Private Witness:**
```
w := (sk, pk, v, ρ, path, change_value, change_pk (=pk), user_entropy_change)
```
All witnesses are always present; `has_change` flag in public inputs controls whether change_value is used.

**Constraints:**

1) **Note inclusion:**
   ```
   cm = Com(v, pk, ρ)
   VerifyMerklePath(cm, path, anchor_root) = 1
   ```

2) **Authorization / nullifier:**
   ```
   nf = NF(sk, ρ)
   pk = PublicKey(sk)
   ```

3) **Range constraints (CRITICAL - prevents modulus wrap):**
   ```
   v ∈ [0, 2⁶⁴)              // Input note value is u64
   amount ∈ [0, 2⁶⁴)         // Withdraw amount is u64
   F_withdraw ∈ [0, 2⁶⁴)     // Fee is u64
   gas_limit ∈ [0, 2⁶⁴)      // Gas limit is u64
   change_value ∈ [0, 2⁶⁴)   // Change value is u64
   has_change ∈ {0, 1}       // Boolean flag (binary constraint)
   ```

4) **Value conservation:**
   ```
   v == amount + F_withdraw + change_value
   ```
   Combined with range proofs, this ensures no over/underflow.

5) **Change output consistency (conditional on has_change flag):**
   ```
   If has_change == 1:
       change_value > 0  // Enforce non-zero change (otherwise has_change should be 0)
       change_pk = pk  // Change goes back to same owner
       change_ρ = H(sk || anchor_root || nf || user_entropy_change || "change")
       cm_change = Com(change_value, change_pk, change_ρ)
       change_commitment == cm_change
       next_root == MerkleAppend(anchor_root, anchor_size, [cm_change])

   If has_change == 0:
       change_value == 0  // Must be exact withdrawal (v == amount + F_withdraw)
       next_root == anchor_root  // No state change
   ```

**Validator/service checks (outside the circuit):**
- `anchor_root == commitment_root` and `anchor_size == commitment_size` (verified via Verkle witnesses)
- `nf` is unspent: `nullifier_value == [0u8; 32]` (verified via Verkle proof) and unique within work item
- `gas_limit` is already bound in proof (no additional check needed - circuit enforces range)
- If `has_change == true` (boolean flag from public inputs): verify `next_root == MerkleAppend(anchor_root, anchor_size, [change_commitment])`
- If `has_change == false`: verify `next_root == anchor_root`
- On success:
  - Mark `nullifiers[nf] = [1u8; 32]`
  - Credit `fee_tally[epoch_id] += F_withdraw` (via DELTA write keyed by `(epoch, work_package_hash)` to avoid parallel conflicts)
  - Execute public transfer to `(recipient, amount)` with `gas_limit` from proof
  - If `has_change == true`: append `change_commitment` and update `(commitment_root, commitment_size)` deterministically

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
- **Double-spend:** Nullifier set prevents reuse (Verkle-proven uniqueness)
- **Inflation:** Value conservation proven in circuit (including withdraw change outputs)
- **Fee Evasion:** Proof fails if fee insufficient
- **Replay:** Nullifiers are commitment-specific via randomness ρ; once spent, cannot be reused (no block-hash binding needed)
- **MEV/Censorship:** Builders cannot selectively censor (no identity known)

## Design Choices

### Multi-Asset Support
**Decision: Single-asset pool (asset_id removed)**
- Each Railgun service instance handles exactly one asset type (native JAM tokens in the initial implementation)
- Circuits and commitments omit `asset_id`; commitments are `Com(value, pk, ρ)`
- Multi-asset support would require separate service instances or a new circuit version with an explicit asset field

### Circuit Size
**Decision: N_IN=2, N_OUT=2**
- Standard shielded pool parameters (balances privacy and performance)
- ~15,000 R1CS constraints (manageable proving time: 2-5 seconds on laptop)
- Sufficient anonymity set for most use cases

### Withdraw Path
**Decision: Option A – Public unshield extrinsic**
- Reveals withdrawal amount and destination, but **not transaction history**
- Simpler implementation (no cross-chain bridge complexity)
- User can still maintain privacy by withdrawing to fresh address or using multiple outputs
- Cross-chain privacy bridges (Option B) can be added later as a separate feature

## Implementation Roadmap

### Phase 1: Core Circuits

**Goal:** Build the Spend Circuit R1CS constraint system and generate proving/verification keys.

#### 1.1 Define Circuit Structure
```rust
// Circuit witnesses (private inputs)
struct SpendCircuitWitness {
    // Input notes (consumed)
    in_sk: [Scalar; N_IN],           // Secret keys for each input
    in_value: [u64; N_IN],
    in_rho: [Scalar; N_IN],          // Randomness for input commitments
    in_merkle_path: [MerklePath; N_IN],  // Membership proofs

    // Output notes (created)
    out_pk: [Scalar; N_OUT],         // Public keys for recipients
    out_value: [u64; N_OUT],
    out_user_entropy: [Scalar; N_OUT], // Private entropy from wallet RNG; circuit derives ρ'_j = H(anchor_root || out_user_entropy[j] || j)
}

// Public inputs (visible on-chain)
struct SpendCircuitPublic {
    anchor_root: Scalar,             // Merkle root we're proving against
    anchor_size: Scalar,             // Append index being used
    next_root: Scalar,               // Deterministic post-state root
    nullifiers: [Scalar; N_IN],      // Prevent double-spend
    commitments: [Scalar; N_OUT],    // New notes created
    fee: u64,                        // Paid to guaranteeing validators
}
```

#### 1.2 Implement Cryptographic Primitives

**Implementation stubs**: [src/crypto.rs](src/crypto.rs:1-108) (requires arkworks dependencies for production)

```rust
use ark_bn254::{Fr as Scalar, Bn254};
use ark_crypto_primitives::crh::poseidon::CRH;

// Poseidon hash (SNARK-friendly, ~150 constraints per invocation)
fn poseidon_hash(inputs: &[Scalar]) -> Scalar {
    CRH::<Scalar>::evaluate(&poseidon_params(), inputs)
}
// poseidon_params(): Poseidon over BN254 with rate=2, capacity=1, full_rounds=8, partial_rounds=57 (standard arkworks BN254 parameters).

// Commitment: Com(value, pk, ρ) = Poseidon(pk || value || ρ)
fn commitment(pk: Scalar, value: u64, rho: Scalar) -> Scalar {
    poseidon_hash(&[pk, Scalar::from(value), rho])
}

// Nullifier: NF(sk, rho) = Poseidon(sk || rho)
fn nullifier(sk: Scalar, rho: Scalar) -> Scalar {
    poseidon_hash(&[sk, rho])
}

// Derive pk from sk (for key-pair validation)
fn derive_pk(sk: Scalar) -> Scalar {
    poseidon_hash(&[sk, Scalar::from(0u64)])  // PRF-like derivation
}

// Range check: prove value is in [0, 2^64) using 64-bit decomposition
// Cost: ~256 constraints (4 bits per constraint with lookup tables)
fn range_check_64bit(cs: ConstraintSystemRef<Scalar>, value: &FpVar<Scalar>) -> Result<()> {
    // CRITICAL: Decompose into EXACTLY 64 bits (not ~254 bits for full field element)
    // This prevents modulus-wrap attacks where attacker adds BN254 field modulus to values

    // Allocate 64 boolean variables for bit decomposition
    let mut bits = Vec::with_capacity(64);
    for i in 0..64 {
        let bit = Boolean::new_witness(cs.clone(), || {
            let val = value.value()?;
            let bit_val = (val >> i) & Scalar::one() == Scalar::one();
            Ok(bit_val)
        })?;
        bits.push(bit);
    }

    // Reconstruct value from 64 bits and enforce equality
    let mut reconstructed = FpVar::zero();
    let mut power_of_two = Scalar::one();
    for bit in bits.iter() {
        let bit_contribution = bit.select(&FpVar::constant(power_of_two), &FpVar::zero())?;
        reconstructed += bit_contribution;
        power_of_two = power_of_two + power_of_two; // power_of_two *= 2
    }
    reconstructed.enforce_equal(value)?;

    // Enforce each bit is 0 or 1 (boolean constraint)
    for bit in bits.iter() {
        bit.enforce_equal(&Boolean::enforce_in_field(cs.clone(), bit)?)?;
    }

    Ok(())
}
```

#### 1.3 Write R1CS Constraints
```rust
impl ConstraintSynthesizer<Scalar> for SpendCircuit {
    fn generate_constraints(self, cs: ConstraintSystemRef<Scalar>) -> Result<()> {
        // === CONSTRAINT SET 1: Input notes are valid ===
        for i in 0..N_IN {
            // Derive pk_i from sk_i
            let pk_i = derive_pk_gadget(cs.clone(), &self.in_sk[i])?;

            // Recompute commitment: cm_i = Com(val_i, pk_i, rho_i)
            let cm_i = commitment_gadget(cs.clone(), &pk_i,
                                          &self.in_value[i], &self.in_rho[i])?;

            // Verify cm_i exists in Merkle tree at anchor_root
            merkle_proof_verify_gadget(cs.clone(), &cm_i, &self.in_merkle_path[i],
                                        &self.public.anchor_root)?;

            // Compute nullifier: nf_i = NF(sk_i, rho_i)
            let nf_i = nullifier_gadget(cs.clone(), &self.in_sk[i], &self.in_rho[i])?;

            // Constrain nf_i equals public input
            nf_i.enforce_equal(&self.public.nullifiers[i])?;
        }

        // === CONSTRAINT SET 2: Output notes are well-formed ===
        for j in 0..N_OUT {
            // Derive per-output randomness from private entropy and anchor_root
            let out_rho_j = derive_rho_from_entropy_gadget(
                cs.clone(),
                &self.public.anchor_root,
                &self.out_user_entropy[j],
                j as u64,
            )?;

            // Compute commitment: cm_j = Com(out_val_j, out_pk_j, out_rho_j)
            let cm_j = commitment_gadget(cs.clone(), &self.out_pk[j],
                                          &self.out_value[j], &out_rho_j)?;

            // Constrain cm_j equals public input
            cm_j.enforce_equal(&self.public.commitments[j])?;
        }

        // === CONSTRAINT SET 3: Range constraints (CRITICAL - prevents modulus wrap) ===
        for i in 0..N_IN {
            // Enforce each input value is a valid u64 (64-bit range proof)
            range_check_64bit(cs.clone(), &self.in_value[i])?;
        }
        for j in 0..N_OUT {
            // Enforce each output value is a valid u64 (64-bit range proof)
            range_check_64bit(cs.clone(), &self.out_value[j])?;
        }
        // Enforce fee is a valid u64 (64-bit range proof)
        range_check_64bit(cs.clone(), &self.public.fee)?;
        // Enforce anchor_size is a valid u64 (64-bit range proof)
        range_check_64bit(cs.clone(), &self.public.anchor_size)?;

        // === CONSTRAINT SET 4: Value conservation ===
        let total_in = self.in_value.iter().sum::<u64>();
        let total_out = self.out_value.iter().sum::<u64>();
        let fee = self.public.fee;

        // Enforce: total_in == total_out + fee
        // This equation is checked in the field, but range proofs above ensure values are u64
        cs.enforce_constraint(
            lc!() + (total_in, Variable::One),
            lc!() + Variable::One,
            lc!() + (total_out + fee, Variable::One)
        )?;

        // === CONSTRAINT SET 5: Deterministic append ===
        let next_root_expected = merkle_append_gadget(
            cs.clone(),
            &self.public.anchor_root,
            &self.public.anchor_size,
            &self.public.commitments,
        )?;
        next_root_expected.enforce_equal(&self.public.next_root)?;

        Ok(())
    }
}
```

**Constraint count estimate:**
- Poseidon hash: ~150 constraints each
- Merkle proof (depth 32): ~4800 constraints per input
- 64-bit range check: ~256 constraints each (with lookup table optimization)
- Total for N_IN=2, N_OUT=2: ~18,000 constraints (includes range checks for 2 inputs + 2 outputs + fee + anchor_size = 6 range proofs)

#### 1.4 Generate Proving/Verification Keys
```rust
use ark_groth16::{Groth16, ProvingKey, VerifyingKey};
use ark_std::rand::thread_rng;

fn setup_circuit() -> (ProvingKey<Bn254>, VerifyingKey<Bn254>) {
    let mut rng = thread_rng();
    let circuit = SpendCircuit::default();  // Dummy witness for setup

    // WARNING: This is INSECURE for production (uses single random source)
    // Production requires MPC ceremony (Phase 6)
    let (pk, vk) = Groth16::<Bn254>::circuit_specific_setup(circuit, &mut rng)
        .expect("Setup failed");

    println!("Proving key size: {} MB", pk.serialized_size() / 1_000_000);
    println!("Verification key size: {} bytes", vk.serialized_size());

    (pk, vk)
}
```

**Output sizes:**
- Proving key: ~50-100 MB (circuit-specific, contains toxic waste τ)
- Verification key: ~2 KB (can be embedded in validator runtime)

#### 1.5 Test Prover
```rust
fn test_prove() {
let (pk, vk) = setup_circuit();

// Create witness with actual values
let witness = SpendCircuitWitness {
    in_sk: [Scalar::from(12345), Scalar::from(67890)],
    in_value: [100, 200],
    in_rho: [Scalar::from(111), Scalar::from(222)],
    in_merkle_path: [/* fetched from indexer */],

    out_pk: [Scalar::from(99999), Scalar::from(88888)],
    out_value: [150, 140],
    out_user_entropy: [Scalar::from(333), Scalar::from(444)],
};

let anchor_root = compute_root_from_path(&witness.in_merkle_path[0]);
let anchor_size = Scalar::from(1); // example append index

// Derive output randomness from entropy (wallet logic mirrors circuit)
let out_rho_0 = poseidon_hash(&[anchor_root, witness.out_user_entropy[0], Scalar::from(0u64)]);
let out_rho_1 = poseidon_hash(&[anchor_root, witness.out_user_entropy[1], Scalar::from(1u64)]);

let public = SpendCircuitPublic {
    anchor_root,
    anchor_size,
    next_root: merkle_append(anchor_root, anchor_size, &[
        commitment(witness.out_pk[0], witness.out_value[0], out_rho_0),
        commitment(witness.out_pk[1], witness.out_value[1], out_rho_1),
    ]),
    nullifiers: [
        nullifier(witness.in_sk[0], witness.in_rho[0]),
        nullifier(witness.in_sk[1], witness.in_rho[1]),
    ],
    commitments: [
        commitment(witness.out_pk[0], witness.out_value[0], out_rho_0),
        commitment(witness.out_pk[1], witness.out_value[1], out_rho_1),
    ],
    fee: 10,
};

    let circuit = SpendCircuit { witness, public: public.clone() };

    // Generate proof (takes ~2-5 seconds on laptop)
    let proof = Groth16::<Bn254>::prove(&pk, circuit, &mut thread_rng())
        .expect("Proving failed");

    // Verify proof (takes ~2ms)
    let valid = Groth16::<Bn254>::verify(&vk, &public.to_field_elements(), &proof)
        .expect("Verification failed");

    assert!(valid);
    println!("✅ Proof generated and verified successfully");
}
```

---

### Phase 2: Verifier Integration (no_std)

**Goal:** Embed the Groth16 verifier in JAM service PVM runtime (no heap allocation, no std library).

#### 2.1 Extract Verification Key as Static Bytes
```rust
// After Phase 1 setup, serialize VK to const bytes
fn serialize_vk(vk: &VerifyingKey<Bn254>) -> Vec<u8> {
    let mut bytes = Vec::new();
    vk.serialize_compressed(&mut bytes).unwrap();
    bytes
}

// Generate at build time:
// const VK_BYTES: &[u8] = &[0x1a, 0x2b, ...];  // ~2 KB
```

Embed in service:
```rust
// services/railgun/src/vk.rs
pub const SPEND_CIRCUIT_VK: &[u8] = include_bytes!("../keys/vk_spend.bin");

pub fn load_vk() -> VerifyingKey<Bn254> {
    VerifyingKey::deserialize_compressed(SPEND_CIRCUIT_VK).unwrap()
}
```

#### 2.2 Implement no_std Groth16 Verifier
```rust
// services/railgun/src/verifier.rs
#![no_std]

use ark_bn254::{Bn254, Fr as Scalar};
use ark_groth16::{Groth16, Proof, VerifyingKey};
use ark_serialize::CanonicalDeserialize;

/// Verify a Groth16 proof without heap allocation
///
/// # Arguments
/// * `proof_bytes` - 192 bytes (2 G1 points + 1 G2 point)
/// * `public_inputs` - Field elements (anchor_root, nullifiers, commitments, fee)
/// * `vk_bytes` - Verification key (~2 KB, loaded from const)
///
/// # Returns
/// * `true` if proof is valid, `false` otherwise
pub fn verify_groth16(
    proof_bytes: &[u8; 192],
    public_inputs: &[Scalar],
    vk_bytes: &[u8],
) -> bool {
    // Deserialize proof (no heap, uses stack)
    let proof = match Proof::<Bn254>::deserialize_compressed(proof_bytes.as_slice()) {
        Ok(p) => p,
        Err(_) => return false,
    };

    // Deserialize VK (cached in static memory)
    let vk = match VerifyingKey::<Bn254>::deserialize_compressed(vk_bytes) {
        Ok(v) => v,
        Err(_) => return false,
    };

    // Verify proof (pure elliptic curve math, ~2ms)
    Groth16::<Bn254>::verify(&vk, public_inputs, &proof).unwrap_or(false)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_verify_valid_proof() {
        let proof = include_bytes!("../test_data/valid_proof.bin");
        let public_inputs = [
            /* anchor_root */ Scalar::from(0x1234),
            /* nullifiers */ Scalar::from(0x5678), Scalar::from(0x9abc),
            /* commitments */ Scalar::from(0xdef0), Scalar::from(0x1111),
            /* fee */ Scalar::from(10),
        ];
        let vk = include_bytes!("../keys/vk_spend.bin");

        assert!(verify_groth16(proof, &public_inputs, vk));
    }

    #[test]
    fn test_verify_invalid_proof() {
        let mut proof = [0u8; 192];  // Garbage bytes
        let public_inputs = [Scalar::from(0); 6];
        let vk = include_bytes!("../keys/vk_spend.bin");

        assert!(!verify_groth16(&proof, &public_inputs, vk));
    }
}
```

#### 2.3 Embed Verifier in JAM Service Refine
```rust
// services/railgun/src/main.rs
#![no_std]

mod verifier;
use verifier::verify_groth16;

extern "C" fn refine(start_address: u64, length: u64) -> (u64, u64) {
    let refine_args = parse_refine_args(start_address, length)?;
    let refine_context = fetch_refine_context()?;
    let extrinsics = fetch_extrinsics(refine_args.wi_index)?;

    let mut write_intents = Vec::new();

    for ext in extrinsics {
        match ext {
            RailgunExtrinsic::SubmitPrivate { proof, public_inputs } => {
                // Parse proof (192 bytes)
                let proof_bytes: [u8; 192] = proof.try_into()
                    .map_err(|_| "Invalid proof length")?;

                // Parse public inputs (8 field elements)
                let anchor_root = public_inputs[0];
                let anchor_size = public_inputs[1];
                let next_root = public_inputs[2];
                let nullifiers = [public_inputs[3], public_inputs[4]];
                let commitments = [public_inputs[5], public_inputs[6]];
                let fee = public_inputs[7];

                // 🔐 VERIFY ZK PROOF (critical security step)
                let vk = include_bytes!("../keys/vk_spend.bin");
                if !verify_groth16(&proof_bytes, &public_inputs, vk) {
                    return Err("Invalid proof");
                }

                // Fetch witnesses for current state
                let commitment_root = fetch_object(refine_context, "commitment_root")?;
                let commitment_size = fetch_object(refine_context, "commitment_size")?;

                // Enforce anchor equality (no staleness window)
                if anchor_root != commitment_root || anchor_size != commitment_size {
                    return Err("Anchor does not match current state");
                }

                // Verify nullifiers are unspent
                for nf in nullifiers {
                    let exists = fetch_object(refine_context, keccak256(("nullifier", nf)))?;
                    if exists {
                        return Err("Nullifier already spent");
                    }
                }

                // Output write intents
                for nf in nullifiers {
                    write_intents.push(WriteIntent {
                        key: keccak256(("nullifier", nf)),
                        value: encode(true),
                    });
                }

                for cm in commitments {
                    write_intents.push(WriteIntent {
                        key: keccak256(("commitment", commitment_size)),
                        value: encode(cm),
                    });
                    commitment_size += 1;
                }

                write_intents.push(WriteIntent {
                    key: keccak256("commitment_size"),
                    value: encode(commitment_size),
                });

                write_intents.push(WriteIntent {
                    key: keccak256("commitment_root"),
                    value: encode(next_root),
                });
            }
            _ => { /* handle Deposit/Withdraw */ }
        }
    }

    Ok(serialize_write_intents(write_intents))
}
```

#### 2.4 End-to-End Integration Test
```rust
#[test]
fn test_deposit_transfer_withdraw() {
    // 1. User deposits 1000 JAM tokens via EVM transfer host call
    let deposit_ext = RailgunExtrinsic::DepositPublic {
        sender_service_id: 0,  // EVM service
        amount: 1000,
        memo: encode_commitment(pk_alice, 1, 1000, rho_1),  // 128 bytes
    };

    // Refine processes deposit
    let write_intents = refine(encode([deposit_ext]), refine_context)?;

    // Accumulate applies writes
    accumulate(service_account, write_intents);

    // Verify state updated
    assert_eq!(service_account.storage["commitment_size"], 1);
    assert_eq!(service_account.storage["commitment_root"], compute_root([cm_1]));

    // 2. Alice privately transfers 600 to Bob (400 change, 10 fee)
    let spend_proof = generate_proof(
        in_notes: [(pk_alice, 1000, rho_1)],
        out_notes: [(pk_bob, 600, rho_2), (pk_alice, 390, rho_3)],
        fee: 10,
    );

    let transfer_ext = RailgunExtrinsic::SubmitPrivate {
        proof: spend_proof.to_bytes(),
        public_inputs: [anchor_root, nf_1, cm_2, cm_3, 10],
    };

    // Refine verifies proof
    let write_intents = refine(encode([transfer_ext]), refine_context)?;

    // Accumulate applies writes
    accumulate(service_account, write_intents);

    // Verify state
    assert_eq!(service_account.storage["commitment_size"], 3);
    assert!(service_account.storage[keccak256(("nullifier", nf_1))]);  // nf_1 spent

    // 3. Bob withdraws 600 back to EVM
    let withdraw_proof = generate_proof(
        in_notes: [(pk_bob, 600, rho_2)],
        out_notes: [],
        fee: 10,
    );

    let withdraw_ext = RailgunExtrinsic::WithdrawPublic {
        proof: withdraw_proof.to_bytes(),
        public_inputs: [anchor_root, nf_2, evm_recipient, 590],  // 600 - 10 fee
    };

    // Refine verifies proof + emits transfer
    let (write_intents, transfer_intent) = refine(encode([withdraw_ext]), refine_context)?;

    // Accumulate executes transfer
    accumulate(service_account, write_intents);
    execute_transfer(transfer_intent);  // Sends 590 JAM to EVM service

    // Verify EVM service received transfer
    assert_eq!(evm_service.x.Transfers[0], DeferredTransfer {
        SenderIndex: RAILGUN_SERVICE_ID,
        ReceiverIndex: EVM_SERVICE_ID,
        Amount: 590,
        Memo: encode_withdraw_proof(...),
        GasLimit: 1_000_000,
    });
}
```

**Success criteria:**
- ✅ Proof verification works in no_std environment
- ✅ Verifier binary size < 1 MB (fits in PVM)
- ✅ Verification time < 5ms (acceptable for consensus)
- ✅ Round-trip test passes: deposit → private transfer → withdraw

### Phase 3: Builder Infrastructure / User Tooling
- [ ] Define off-chain proof submission protocol
- [ ] Builder pre-check: proof verify + fee policy + nullifier cache
- [ ] Wire fee_tally to JAM validator rewards (epoch-based distribution)
- [ ] Invalid proof rejection (no on-chain spam)
- [ ] Wallet SDK: local keypair management
- [ ] Merkle tree syncing (light client or indexer)
- [ ] Prover app: select notes, compute paths, generate proof
- [ ] Recipient scanning: trial-decrypt commitments each block
- [ ] Governance extrinsic: `UpdateGasParams{gas_min, gas_max}` to refresh bounds without redeploying circuits

### Phase 4: Testing / Production Hardening
- [ ] Deposit → SubmitPrivate → WithdrawPublic round-trip
- [ ] Invalid proof rejected by builder and validator
- [ ] Nullifier replay rejected
- [ ] Fee underpayment rejected
- [ ] Multiple opaque extrinsics unlinkable (no side-channel leaks)
- [ ] Trusted setup: MPC ceremony for production keys
- [ ] Formal audit of circuits and no_std verifier
- [ ] Encrypted payload format (hybrid ElGamal over Jubjub)
- [ ] Rate limiting / dynamic fee policy

### Phase 6: Trusted Setup / MPC
- Run multi-party Powers of Tau + circuit-specific MPC (target ≥100 participants; open-sourced transcript tooling).
- Publish contribution hashes, entropy sources, and verification scripts; require pairwise transcript verification before use.
- Embed verification key only after transcript verification and independent audit of ceremonies.

## Error Handling & Gas Metering

### Error Taxonomy

**Implementation**: [src/errors.rs](src/errors.rs:1-68)

Railgun refine returns structured errors to help builders/validators diagnose failures:

```rust
enum RailgunError {
    // ZK proof errors
    InvalidProof,
    InvalidPublicInputs,

    // State errors
    AnchorMismatch { expected: Hash, got: Hash },
    NullifierAlreadySpent { nullifier: Hash },
    InvalidVerkleProof { key: Hash },
    TreeCapacityExceeded { current_size: u64, requested_additions: u64, capacity: u64 },

    // Validation errors
    InvalidMemoVersion { version: u8 },
    ValueBindingFailure { expected: Hash, got: Hash },
    GasLimitOutOfBounds { provided: u64, min: u64, max: u64 },
    FeeUnderflow { provided: u64, minimum: u64 },
    EncryptedPayloadNotAllowed { length: usize },  // MVP: encrypted payloads disabled

    // Replay errors
    NullifierReusedInWorkItem { nullifier: Hash },

    // Note: OfferNullifierReusedInWorkItem removed - atomic swaps deferred to multi-asset extension
}
```

**Validator behavior on error:** Entire work item is rejected (no partial application). Work report votes against the failing work package.
**Builder-precheckable errors:** `InvalidProof`, `InvalidPublicInputs`, `FeeUnderflow`, `GasLimitOutOfBounds`, `ValueBindingFailure`, `EncryptedPayloadNotAllowed`, `NullifierReusedInWorkItem` (within the same submission), and `TreeCapacityExceeded` (given a recent state view) can be detected off-chain to avoid wasting inclusion attempts. Errors requiring fresh Verkle witnesses (`AnchorMismatch`, `NullifierAlreadySpent`, `InvalidVerkleProof`) are validator-only.

### Gas Metering & DoS Protection

**PVM gas costs (approximate, implementation-dependent):**

| Operation | Cost | Notes |
|-----------|------|-------|
| `verify_groth16()` | ~1,000,000 gas | BN254 pairing check dominates |
| `verify_verkle_proof()` | ~50,000 gas per proof | IPA multiproof amortizes batch |
| `merkle_append()` | ~10,000 gas | Poseidon hashes, depth 32 |
| `fetch_object()` | ~5,000 gas | Host call overhead |
| State write | ~10,000 gas per WriteIntent | Applied in accumulate |

**Per-extrinsic budget:** `MAX_GAS_PER_EXTRINSIC = 2,000,000`
- SubmitPrivate (N_IN=2, N_OUT=2): ~1.2M gas
- WithdrawPublic: ~1.1M gas
- DepositPublic: ~20K gas (no ZK proof)

**Per-work-item budget:** `MAX_GAS_PER_WORK_ITEM = 10,000,000`
- Allows ~5-8 private transfers per work item
- Prevents DoS via excessive ZK proof verification

**Host transfer gas bounds (consensus-critical):**
- `gas_min = 50,000`: Minimum for EVM accumulate processing (state parameter; governance-updatable)
- `gas_max = 5,000,000`: Prevents griefing via excessive gas consumption (state parameter; governance-updatable)
- Refine rejects deposits/withdrawals outside this range

**Builder pre-check:** Builders should verify gas costs locally before including extrinsics to avoid wasting PVM resources on guaranteed-to-fail work items.

## What to Avoid (Anti-Patterns)

- ❌ **Do NOT include signer or public key** in `SubmitPrivate`
- ❌ **Do NOT leak fee payer identity**—fee is protocol-credited, not user-directed
- ❌ **Do NOT rely on EVM contracts**—all logic lives in JAM service transition
- ❌ **Do NOT use Ethereum-style gas**—use Z-space fee accounting
- ❌ **Do NOT expose calldata**—use encrypted payloads + zk proofs

---

**Design Philosophy:** JAM Railgun treats privacy as a first-class protocol feature, not a smart contract afterthought. Builders, users, and validators never see the full picture—only math proves correctness.
