# Orchard FFI for JAM Integration

This document describes the FFI (Foreign Function Interface) integration for Orchard's proof generation and cryptographic operations with Go's JAM integration.

## Overview

The Orchard service currently exposes a **limited FFI surface**:
- Sinsemilla hashing and Merkle tree utilities (builder/orchard/ffi)
- Commitment/nullifier helpers and witness bundle helpers (services/orchard/src/ffi.rs)
- Halo2 proof **verification only** (services/orchard/src/ffi.rs)

Not yet implemented via FFI:
- Orchard key derivation and address generation
- Orchard bundle proof generation
- Full Orchard bundle verification via FFI

The FFI implementation provides C-compatible functions exposed from Rust that can be called from Go via CGO bindings.

## Implementation Status

✅ **IMPLEMENTED**: Sinsemilla Merkle Tree FFI (CGO bindings complete)
✅ **IMPLEMENTED**: Commitment/nullifier helpers + witness bundle helpers
✅ **IMPLEMENTED**: Halo2 proof verification (verify-only)
🚧 **IN PROGRESS**: Key/address derivation and bundle prove/verify FFI

## Current Exported Functions (services/orchard/src/ffi.rs)

- `orchard_init`, `orchard_cleanup`, `orchard_get_service_id`
- `orchard_nullifier_with_service_id`, `orchard_nullifier`
- `orchard_commitment_with_service_id`, `orchard_commitment`
- `orchard_poseidon_hash` (Blake2b placeholder; legacy compatibility)
- `orchard_create_witness_bundle`, `orchard_witness_add_read`, `orchard_serialize_witness`
- `orchard_verify_halo2_proof`

## New CGO Sinsemilla FFI Implementation

### C Export Functions

**File**: `builder/orchard/ffi/src/lib.rs`

```rust
/// Compute Sinsemilla Merkle tree root from leaves
#[no_mangle]
pub unsafe extern "C" fn sinsemilla_compute_root(
    leaves: *const u8,        // Flattened leaf array (leaves_len * 32 bytes)
    leaves_len: usize,        // Number of 32-byte leaves
    root_out: *mut u8,        // 32-byte output buffer for computed root
) -> i32                      // FFI_SUCCESS (0) or error code

/// Compute Sinsemilla Merkle root + frontier from leaves
#[no_mangle]
pub unsafe extern "C" fn sinsemilla_compute_root_and_frontier(
    leaves: *const u8,         // Flattened leaf array (leaves_len * 32 bytes)
    leaves_len: usize,         // Number of 32-byte leaves
    root_out: *mut u8,         // 32-byte output buffer for computed root
    frontier_out: *mut u8,     // Frontier output buffer (TREE_DEPTH * 32 bytes)
    frontier_len_out: *mut usize, // Output: frontier length
) -> i32                       // FFI_SUCCESS (0) or error code

/// Append new leaves to existing tree using frontier
#[no_mangle]
pub unsafe extern "C" fn sinsemilla_append_with_frontier(
    old_root: *const u8,      // Current tree root (32 bytes)
    old_size: u64,            // Current number of leaves
    frontier: *const u8,      // Frontier hashes (frontier_len * 32 bytes)
    frontier_len: usize,      // Number of frontier elements
    new_leaves: *const u8,    // New leaves to append (new_leaves_len * 32 bytes)
    new_leaves_len: usize,    // Number of new leaves
    new_root_out: *mut u8,    // 32-byte output buffer for new root
) -> i32                      // FFI_SUCCESS (0) or error code

/// Append new leaves and return updated frontier
#[no_mangle]
pub unsafe extern "C" fn sinsemilla_append_with_frontier_update(
    old_root: *const u8,       // Current tree root (32 bytes)
    old_size: u64,             // Current number of leaves
    frontier: *const u8,       // Frontier hashes (frontier_len * 32 bytes)
    frontier_len: usize,       // Number of frontier elements
    new_leaves: *const u8,     // New leaves to append (new_leaves_len * 32 bytes)
    new_leaves_len: usize,     // Number of new leaves
    new_root_out: *mut u8,     // 32-byte output buffer for new root
    new_frontier_out: *mut u8, // Updated frontier output buffer (TREE_DEPTH * 32 bytes)
    new_frontier_len_out: *mut usize, // Output: frontier length
) -> i32                       // FFI_SUCCESS (0) or error code

/// Generate Merkle proof for commitment at given position
#[no_mangle]
pub unsafe extern "C" fn sinsemilla_generate_proof(
    commitment: *const u8,         // Target commitment (32 bytes)
    tree_position: u64,            // Position in tree
    all_commitments: *const u8,    // All tree commitments (commitments_len * 32 bytes)
    commitments_len: usize,        // Number of commitments
    proof_out: *mut u8,           // Output buffer for proof siblings
    proof_len_out: *mut usize,    // Output: actual proof length
) -> i32                          // FFI_SUCCESS (0) or error code
```

### Go CGO Wrapper

**File**: `builder/orchard/ffi/sinsemilla.go`

```go
/*
#cgo LDFLAGS: -L${SRCDIR} -lorchard_ffi
#include "orchard_ffi.h"
*/
import "C"

// SinsemillaMerkleTree maintains Orchard commitment tree state
type SinsemillaMerkleTree struct {
    root     [32]byte      // Current tree root
    size     uint64        // Number of leaves in tree
    frontier [][32]byte    // Frontier for efficient updates
}

func NewSinsemillaMerkleTree() *SinsemillaMerkleTree
func (t *SinsemillaMerkleTree) ComputeRoot(leaves [][32]byte) error
func (t *SinsemillaMerkleTree) AppendWithFrontier(newLeaves [][32]byte) error
func (t *SinsemillaMerkleTree) GenerateProof(commitment [32]byte, position uint64, allCommitments [][32]byte) ([][32]byte, error)
func (t *SinsemillaMerkleTree) GetRoot() [32]byte
func (t *SinsemillaMerkleTree) GetSize() uint64
func (t *SinsemillaMerkleTree) GetFrontier() [][32]byte
func (t *SinsemillaMerkleTree) SetFrontier(frontier [][32]byte)
```

### Build System Integration

**File**: `builder/orchard/ffi/Makefile`

```make
.PHONY: all clean ffi test

# Build the FFI library
ffi:
	cargo build --release --lib
	cp target/release/liborchard_ffi.a .
	cp orchard_ffi.h .

# Test the FFI
test: ffi
	go test -v .

# Clean artifacts
clean:
	cargo clean
	rm -f liborchard_ffi.a orchard_ffi.h
```

**File**: `builder/orchard/ffi/Cargo.toml`

```toml
[package]
name = "orchard_ffi"
version = "0.1.0"
edition = "2021"

[lib]
crate-type = ["cdylib", "staticlib"]

[dependencies]
orchard = { path = "../zcash-orchard", default-features = false, features = ["std"] }
incrementalmerkletree = "0.8.1"

[build-dependencies]
cbindgen = "0.24"
```

### Error Handling

```rust
// FFI Error codes
pub const FFI_SUCCESS: i32 = 0;
pub const FFI_ERROR_NULL_POINTER: i32 = -1;
pub const FFI_ERROR_INVALID_INPUT: i32 = -2;
pub const FFI_ERROR_COMPUTATION_FAILED: i32 = -3;
```

```go
// Go error handling
func ffiErrorToGo(code C.int) error {
    switch code {
    case FFI_ERROR_NULL_POINTER:
        return errors.New("FFI null pointer error")
    case FFI_ERROR_INVALID_INPUT:
        return errors.New("FFI invalid input error")
    case FFI_ERROR_COMPUTATION_FAILED:
        return errors.New("FFI computation failed")
    default:
        return errors.New("unknown FFI error")
    }
}
```

### Usage Example

```go
// Create tree and compute initial root
tree := NewSinsemillaMerkleTree()
leaves := [][32]byte{commitment1, commitment2, commitment3, commitment4}
err := tree.ComputeRoot(leaves)

// Efficiently append new commitments (frontier is updated by FFI)
newLeaves := [][32]byte{commitment5, commitment6}
err = tree.AppendWithFrontier(newLeaves)

// Generate membership proof
proof, err := tree.GenerateProof(commitment1, 0, append(leaves, newLeaves...))
```

## FFI Functions (Implemented + Planned)

**Status key**:
- ✅ Implemented today
- ❌ Planned / not exported

### 1. System Functions

**Status**: ✅ Implemented

```rust
// File: src/ffi.rs

/// Initialize Orchard FFI system
#[no_mangle]
pub extern "C" fn orchard_init() -> u32

/// Cleanup Orchard FFI system
#[no_mangle]
pub extern "C" fn orchard_cleanup()

/// Get Orchard service ID for JAM
#[no_mangle]
pub extern "C" fn orchard_get_service_id() -> u32
```

### 2. Key Management

**Status**: ❌ Not implemented (planned)

```rust
/// Generate Orchard spending key from seed
#[no_mangle]
pub extern "C" fn orchard_generate_spending_key(
    seed: *const u8,              // 32 bytes
    output: *mut u8,              // 32 bytes output buffer
) -> u32

/// Derive full viewing key from spending key
#[no_mangle]
pub extern "C" fn orchard_derive_full_viewing_key(
    spending_key: *const u8,      // 32 bytes
    output: *mut u8,              // 32 bytes output buffer
) -> u32

/// Derive incoming viewing key from full viewing key
#[no_mangle]
pub extern "C" fn orchard_derive_incoming_viewing_key(
    full_viewing_key: *const u8,  // 32 bytes
    output: *mut u8,              // 32 bytes output buffer
) -> u32

/// Generate diversified address from incoming viewing key
#[no_mangle]
pub extern "C" fn orchard_generate_address(
    incoming_viewing_key: *const u8, // 32 bytes
    diversifier_index: u64,
    output: *mut u8,              // 43 bytes output buffer (diversifier + pk_d)
) -> u32
```

### 3. Orchard Cryptographic Operations

**Status**: ✅ Implemented (limited surface)

```rust
/// Generate Orchard nullifier (legacy wrapper)
#[no_mangle]
pub extern "C" fn orchard_nullifier(
    sk_spend: *const u8,          // 32 bytes
    rho: *const u8,               // 32 bytes
    commitment: *const u8,        // 32 bytes
    output: *mut u8,             // 32 bytes output buffer
) -> u32

/// Generate Orchard nullifier with explicit service ID
#[no_mangle]
pub extern "C" fn orchard_nullifier_with_service_id(
    sk_spend: *const u8,          // 32 bytes
    rho: *const u8,               // 32 bytes
    commitment: *const u8,        // 32 bytes
    output: *mut u8,             // 32 bytes output buffer
    service_id: u32,
) -> u32

/// Generate note commitment with explicit fields + service ID
#[no_mangle]
pub extern "C" fn orchard_commitment_with_service_id(
    asset_id: u32,
    amount_lo: u64,
    amount_hi: u64,
    owner_pk: *const u8,          // 32 bytes
    rho: *const u8,               // 32 bytes
    note_rseed: *const u8,        // 32 bytes
    unlock_height: u64,
    memo_hash: *const u8,         // 32 bytes
    output: *mut u8,             // 32 bytes output buffer
    service_id: u32,
) -> u32

/// Legacy commitment wrapper (service_id = 1)
#[no_mangle]
pub extern "C" fn orchard_commitment(/* same args as above */) -> u32

/// Domain-separated hash placeholder (Blake2b-based, legacy compatibility)
#[no_mangle]
pub extern "C" fn orchard_poseidon_hash(/* ... */) -> u32
```

**Note**: Sinsemilla Merkle operations are exposed via `sinsemilla_*` FFI in
`builder/orchard/ffi`, not via `orchard_sinsemilla_hash`.

### 4. Bundle Proof Operations

**Status**: ❌ Not implemented (planned). Only `orchard_verify_halo2_proof` exists today.

```rust
/// Generate Orchard bundle proof
#[no_mangle]
pub extern "C" fn orchard_prove_bundle(
    actions_count: u32,
    actions_data: *const u8,      // Serialized action data
    actions_len: u32,             // Actions data length
    sighash: *const u8,           // 32 bytes bundle sighash
    proof_output: *mut u8,        // Variable-length proof output
    proof_len: *mut u32,          // Proof length (input: buffer size, output: actual length)
) -> u32

/// Verify Orchard bundle proof
#[no_mangle]
pub extern "C" fn orchard_verify_bundle(
    actions_count: u32,
    actions_data: *const u8,      // Serialized action data
    actions_len: u32,             // Actions data length
    proof_data: *const u8,        // Proof bytes
    proof_len: u32,               // Proof length
    sighash: *const u8,           // 32 bytes bundle sighash
) -> u32
```

## Go Integration Implementation

**Note**: The CGO bindings below are **planned** for the full Orchard FFI surface.
Today, Go integration is limited to the Sinsemilla CGO bindings in
`builder/orchard/ffi` and the minimal FFI functions listed above.

### CGO Bindings

**File**: `builder/orchard/ffi/orchard_ffi.go`

```go
/*
#cgo LDFLAGS: -L../../../services/orchard/target/release -lorchard_service
#include <stdint.h>

// FFI Result codes
#define FFI_SUCCESS 0
#define FFI_INVALID_INPUT 1
#define FFI_PROOF_GENERATION_FAILED 2
#define FFI_VERIFICATION_FAILED 3
#define FFI_SERIALIZATION_ERROR 4
#define FFI_INTERNAL_ERROR 5

// Function declarations
uint32_t orchard_init();
void orchard_cleanup();
uint32_t orchard_get_service_id();
uint32_t orchard_generate_spending_key(const uint8_t* seed, uint8_t* output);
uint32_t orchard_derive_full_viewing_key(const uint8_t* spending_key, uint8_t* output);
uint32_t orchard_derive_incoming_viewing_key(const uint8_t* full_viewing_key, uint8_t* output);
uint32_t orchard_generate_address(const uint8_t* incoming_viewing_key, uint64_t diversifier_index, uint8_t* output);
uint32_t orchard_nullifier(const uint8_t* spending_key, const uint8_t* rho, const uint8_t* psi, const uint8_t* commitment, uint8_t* output);
uint32_t orchard_commitment(uint64_t value, const uint8_t* diversifier, const uint8_t* pk_d, const uint8_t* rho, const uint8_t* psi, uint8_t* output);
uint32_t orchard_prove_bundle(uint32_t actions_count, const uint8_t* actions_data, uint32_t actions_len, const uint8_t* sighash, uint8_t* proof_output, uint32_t* proof_len);
uint32_t orchard_verify_bundle(uint32_t actions_count, const uint8_t* actions_data, uint32_t actions_len, const uint8_t* proof_data, uint32_t proof_len, const uint8_t* sighash);
*/
import "C"
```

### Go Wrapper Implementation

```go
// OrchardFFI provides access to Rust Orchard cryptographic operations
type OrchardFFI struct {
    initialized bool
}

// High-level wrapper functions
func (o *OrchardFFI) GenerateSpendingKey(seed [32]byte) ([32]byte, error)
func (o *OrchardFFI) DeriveFullViewingKey(spendingKey [32]byte) ([32]byte, error)
func (o *OrchardFFI) DeriveIncomingViewingKey(fullViewingKey [32]byte) ([32]byte, error)
func (o *OrchardFFI) GenerateAddress(incomingViewingKey [32]byte, diversifierIndex uint64) (OrchardAddress, error)
func (o *OrchardFFI) GenerateNullifier(spendingKey [32]byte, rho, psi, commitment [32]byte) ([32]byte, error)
func (o *OrchardFFI) GenerateCommitment(value uint64, diversifier [11]byte, pkD, rho, psi [32]byte) ([32]byte, error)
func (o *OrchardFFI) ProveBundleProof(actions []OrchardAction, sighash [32]byte) ([]byte, error)
func (o *OrchardFFI) VerifyBundleProof(actions []OrchardAction, proof []byte, sighash [32]byte) error

// Orchard-specific data structures
type OrchardAddress struct {
    Diversifier [11]byte
    PkD        [32]byte
}

type OrchardAction struct {
    CvNet     [32]byte  // Value commitment
    Nullifier [32]byte  // Old note nullifier
    Rk        [32]byte  // Randomized verification key
    Cmx       [32]byte  // New note commitment (extracted)
    // Note ciphertexts handled separately
}
```

## JAM Integration Points

### Builder Integration

**File**: `builder/orchard/witness/orchard_builder.go`

```go
// OrchardBuilder integrates Orchard FFI with JAM builder operations
type OrchardBuilder struct {
    ffi        *OrchardFFI
    wallet     OrchardWallet
    treeState  *OrchardTreeState
}

func (b *OrchardBuilder) BuildOrchardBundle(req *OrchardBundleRequest) (*OrchardBundle, error) {
    var actions []OrchardAction

    for _, spend := range req.Spends {
        // Generate action for spend
        action, err := b.buildSpendAction(spend)
        if err != nil {
            return nil, err
        }
        actions = append(actions, action)
    }

    for _, output := range req.Outputs {
        // Generate action for output
        action, err := b.buildOutputAction(output)
        if err != nil {
            return nil, err
        }
        actions = append(actions, action)
    }

    // Generate bundle proof
    sighash := computeBundleSighash(actions, req.ValueBalance)
    proof, err := b.ffi.ProveBundleProof(actions, sighash)
    if err != nil {
        return nil, err
    }

    return &OrchardBundle{
        Actions:      actions,
        ValueBalance: req.ValueBalance,
        Anchor:       req.Anchor,
        Proof:        proof,
    }, nil
}
```

### RPC Integration

**File**: `builder/orchard/rpc/orchard_handler.go`

```go
// OrchardRPCHandler implements Zcash-compatible RPC for Orchard operations
type OrchardRPCHandler struct {
    ffi     *OrchardFFI
    wallet  OrchardWallet
    builder *OrchardBuilder
}

func (h *OrchardRPCHandler) ZGetNewAddress(addressType string) (string, error) {
    if addressType != "orchard" {
        return "", fmt.Errorf("unsupported address type: %s", addressType)
    }

    // Get wallet's current incoming viewing key
    ivk, err := h.wallet.GetIncomingViewingKey()
    if err != nil {
        return "", err
    }

    // Generate new diversified address
    diversifierIndex := h.wallet.GetNextDiversifierIndex()
    addr, err := h.ffi.GenerateAddress(ivk, diversifierIndex)
    if err != nil {
        return "", err
    }

    // Return unified address format
    return encodeUnifiedAddress(addr), nil
}

func (h *OrchardRPCHandler) ZSendMany(fromAddress string, amounts []RecipientAmount, fee uint64) (string, error) {
    // Build Orchard bundle request
    bundleReq := &OrchardBundleRequest{
        Spends:       h.selectSpends(amounts, fee),
        Outputs:      buildOutputs(amounts),
        ValueBalance: int64(fee), // Net value out for fee
        Anchor:       h.getCurrentAnchor(),
    }

    // Generate bundle via FFI
    bundle, err := h.builder.BuildOrchardBundle(bundleReq)
    if err != nil {
        return "", err
    }

    // Submit to JAM
    txId, err := h.submitToJAM(bundle)
    if err != nil {
        return "", err
    }

    return txId, nil
}
```

## Building FFI Library for Orchard

### Cargo Configuration

**File**: `services/orchard/Cargo.toml`

```toml
[package]
name = "orchard_service"
version = "0.1.0"
edition = "2021"

[lib]
name = "orchard_service"
path = "src/lib.rs"
crate-type = ["rlib", "cdylib"]

[features]
default = []
ffi = []

[dependencies]
orchard = "0.6"
halo2_proofs = "0.6"
pasta_curves = "0.5"

[profile.release]
panic = "unwind"  # FFI requires unwinding panic mode

[profile.dev]
panic = "unwind"  # FFI requires unwinding panic mode
```

### Memory Management

**File**: `services/orchard/src/lib.rs`

```rust
#![no_std]

#[macro_use]
extern crate alloc;

// Use standard allocator for FFI
#[cfg(feature = "ffi")]
extern crate std;

#[cfg(feature = "ffi")]
pub mod ffi;

// Re-export orchard crate functionality
pub use orchard::{
    keys::{SpendingKey, FullViewingKey, IncomingViewingKey},
    Address, Note, Nullifier,
    bundle::{Action, Bundle},
    tree::MerkleHashOrchard,
};
```

## Implementation TODOs

### High Priority

#### 1. **Orchard Crate Integration**
**Status**: Not started | **Priority**: Critical | **Timeline**: 1-2 weeks

**Scope**: Direct integration with `orchard` crate for all cryptographic operations

```rust
// Replace custom implementations with orchard crate
use orchard::{
    keys::{SpendingKey, FullViewingKey, IncomingViewingKey, DiversifiedTransmissionKey},
    Address, Note, Nullifier,
    bundle::{Action, Bundle, Authorized},
    circuit::prove_bundle,
    value::ValueCommitment,
};

#[no_mangle]
pub extern "C" fn orchard_prove_bundle(/* params */) -> u32 {
    // Use orchard::circuit::prove_bundle directly
    let bundle = Bundle::from_parts(/* ... */);
    let proof = prove_bundle(&proving_key, bundle, sighash)?;
    // Serialize proof to output buffer
}
```

#### 2. **Zcash v5 Bundle Serialization**
**Status**: Not started | **Priority**: High | **Timeline**: 1 week

**Scope**: Implement exact Zcash v5 Orchard bundle encoding/decoding

```rust
// Use zcash_primitives for bundle serialization
use zcash_primitives::transaction::components::orchard::{
    read_v5_bundle, write_v5_bundle
};

#[no_mangle]
pub extern "C" fn orchard_serialize_bundle(
    bundle_ptr: *const Bundle<Authorized>,
    output: *mut u8,
    output_len: *mut u32,
) -> u32 {
    let bundle = unsafe { &*bundle_ptr };
    let mut cursor = Cursor::new(Vec::new());
    write_v5_bundle(bundle, &mut cursor)?;
    // Copy to output buffer
}
```

#### 3. **JAM Service Integration**
**Status**: Design phase | **Priority**: High | **Timeline**: 2 weeks

**Scope**: Wire FFI to JAM refine/accumulate with witness-based execution

```go
// Integration with JAM witness-based execution model
func (s *OrchardService) Refine(
    extrinsic *SubmitOrchard,
    witnesses *StateWitnesses,
) (*WriteIntents, error) {
    // Decode Orchard bundle via FFI
    bundle, err := s.ffi.DecodeOrchardBundle(extrinsic.BundleBytes)
    if err != nil {
        return nil, err
    }

    // Verify bundle proof via FFI
    err = s.ffi.VerifyBundleProof(bundle.Actions, bundle.Proof, bundle.SigHash)
    if err != nil {
        return nil, err
    }

    // Generate write intents for JAM state updates
    return s.computeStateUpdates(bundle, witnesses), nil
}
```

### Medium Priority

#### 4. **WASM Target Support**
**Status**: Planning | **Priority**: Medium | **Timeline**: 2-3 weeks

**Scope**: Enable browser-based proof generation via WASM

```toml
# Add WASM support to Cargo.toml
[features]
wasm = ["wasm-bindgen", "orchard/wasm"]

[target.'cfg(target_arch = "wasm32")'.dependencies]
wasm-bindgen = "0.2"
console_error_panic_hook = "0.1"
```

#### 5. **Performance Optimization**
**Status**: Not started | **Priority**: Medium | **Timeline**: 1-2 weeks

**Scope**: Optimize proof generation times and memory usage

```rust
// Implement caching for proving/verifying keys
static ORCHARD_PROVING_KEY: OnceCell<ProvingKey> = OnceCell::new();
static ORCHARD_VERIFYING_KEY: OnceCell<VerifyingKey> = OnceCell::new();

fn get_proving_key() -> &'static ProvingKey {
    ORCHARD_PROVING_KEY.get_or_init(|| {
        // Load from embedded bytes or generate
    })
}
```

### Low Priority

#### 6. **Hardware Wallet Support**
**Status**: Future work | **Priority**: Low | **Timeline**: TBD

**Scope**: Integration with Ledger/Trezor for key management

#### 7. **Advanced Features**
**Status**: Future work | **Priority**: Low | **Timeline**: TBD

**Scope**: Note scanning optimization, batch operations, etc.

## Security Considerations

### Memory Safety
- All FFI functions validate pointer bounds and perform null checks
- Sensitive key material is cleared after use
- Use of `unsafe` blocks is minimized and audited

### Input Validation
```rust
// Example validation pattern
if spending_key.is_null() || output.is_null() {
    return FFIResult::InvalidInput as u32;
}

// Validate key lengths
let sk_slice = unsafe {
    std::slice::from_raw_parts(spending_key, 32)
};
```

### Integration Security
- FFI boundary isolates cryptographic operations
- No private keys cross FFI boundary unnecessarily
- All operations use deterministic algorithms from `orchard` crate

## Success Criteria

✅ **Phase 1**: Basic FFI functions working with `orchard` crate integration
⏳ **Phase 2**: Full bundle proof generation and verification
⏳ **Phase 3**: JAM service integration with witness-based execution
⏳ **Phase 4**: Performance meets production requirements (<5s proof generation)
⏳ **Phase 5**: WASM support for browser-based operations

This Orchard FFI implementation provides a limited bridge between Go's JAM integration and Rust's Orchard cryptographic operations. Full Zcash-compatible Orchard proof/key functionality remains planned.
