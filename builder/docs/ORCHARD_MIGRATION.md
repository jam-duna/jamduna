# Orchard Service Migration

**Date**: 2025-12-30
**Change**: Replaced custom Railgun protocol with Zcash Orchard protocol

---

## Summary

The JAM privacy service has been completely redesigned from a custom "Railgun" protocol to the **Zcash Orchard protocol**. This change provides:

✅ **Production-ready cryptography** from Zcash's battle-tested implementation
✅ **Standard compliance** with Zcash v5 Orchard specification
✅ **Ecosystem compatibility** with existing Zcash tools and wallets
✅ **Simplified maintenance** by leveraging the `orchard` crate

---

## Key Changes

### 1. Protocol Replacement

| Aspect | Railgun (Old) | Orchard (New) |
|--------|---------------|---------------|
| **Circuit System** | Custom Halo2 circuits | Zcash Orchard Action circuit |
| **Curves** | Pasta (Pallas/Vesta) | Pasta (Pallas/Vesta) ✅ Same |
| **Hash Function** | Poseidon | Sinsemilla (Orchard standard) |
| **Tree Depth** | 32 levels | 32 levels ✅ Same |
| **Proof System** | Custom batching | Orchard bundle proofs |
| **Note Structure** | Custom 7-field notes | Orchard note format |
| **Public Inputs** | 44 fields (spend), 17 fields (withdraw) | 9 fields per action (fixed) |

### 2. Circuit Design

**Railgun** used multiple circuit types:
- `vk_id=1`: spend_v1 (44 public inputs)
- `vk_id=2`: withdraw_v1 (17 public inputs)
- `vk_id=3`: issuance_v1 (11 public inputs)
- `vk_id=4`: batch_agg_v1 (17 public inputs)

**Orchard** uses a single Action circuit:
- **9 public inputs per action**: `[anchor, cv_net_x, cv_net_y, nf_old, rk_x, rk_y, cmx, enable_spend, enable_output]`
- **Fixed circuit size**: `K = 11` (not configurable)
- **Bundle proofs**: Single proof covering N actions

### 3. Transaction Model

**Railgun** (Custom):
```
SubmitPrivate: Single shielded transfer
BatchSubmit: Aggregated proof batch
WithdrawPublic: Bridge withdrawal
DepositPublic: Bridge deposit
```

**Orchard** (Zcash Standard):
```
SubmitOrchard: Orchard bundle (v5 encoding)
  - actions: List of Orchard actions (spend + output pairs)
  - proof: Single Orchard bundle proof
  - signatures: Spend authorization + binding signature
```

### 4. RPC Interface

**Namespace**: `z_*` (Zcash-compatible)

All RPC methods now follow Zcash conventions:
- `z_getnewaddress` - Orchard address generation
- `z_gettreestate` - Orchard commitment tree state
- `z_sendmany` - Orchard bundle submission
- `getblockchaininfo` - Orchard pool statistics

See [orchard/docs/RPC.md](../orchard/docs/RPC.md) for complete API reference.

### 5. FFI Integration

**Orchard requires FFI** for cryptographic operations:
- `orchard` crate (Rust) ↔ Go via CGO
- Key generation, note encryption, proof generation
- Sinsemilla hashing for Merkle tree

See [orchard/docs/FFI.md](../orchard/docs/FFI.md) for FFI design.

---

## Directory Structure Changes

### Before (Railgun)
```
builder/
├── railgun/
│   ├── merkle/      # Custom Merkle (Poseidon)
│   ├── witness/     # Custom proof batching
│   └── rpc/         # Custom z_* RPC
```

### After (Orchard)
```
builder/
├── orchard/
│   ├── merkle/      # Orchard Sinsemilla tree
│   ├── witness/     # Orchard bundle building
│   ├── rpc/         # Zcash-compatible z_* RPC
│   ├── src/         # Rust FFI implementation
│   ├── Cargo.toml   # Orchard crate dependencies
│   └── docs/
│       ├── ORCHARD.md      # Protocol specification
│       ├── RPC.md          # Zcash RPC reference
│       ├── FFI.md          # FFI integration
│       ├── WALLET.md       # Wallet integration
│       └── LIGHTCLIENT.md  # Light client support
```

---

## Implementation Status

### ✅ Completed

1. **Protocol Documentation**
   - ORCHARD.md: Complete Zcash Orchard specification
   - RPC.md: Full Zcash-compatible RPC reference
   - FFI.md: FFI architecture and interface design

2. **Directory Structure**
   - `builder/orchard/` created with subdirectories
   - `builder/railgun/` removed completely
   - README files updated for Orchard context

3. **Documentation Updates**
   - ROLLUP.md updated (Railgun → Orchard)
   - STATUS.md updated with Orchard phases

### 🚧 In Progress

1. **FFI Implementation** (Priority: High)
   - Basic FFI stubs exist
   - Need full `orchard` crate integration
   - Bundle proof generation via Halo2

2. **Merkle Tree** (Priority: High)
   - Orchard Sinsemilla hash implementation
   - 32-level commitment tree
   - Subtree root management for light clients

3. **RPC Handlers** (Priority: Medium)
   - Most handlers implemented
   - Need Orchard bundle encoding/decoding
   - Address generation via FFI

### 📋 Pending

1. **JAM Integration**
   - Refine: Orchard bundle verification
   - Accumulate: Tree state updates
   - Witness generation for guarantors

2. **Builder Binary**
   - `cmd/orchard-builder/main.go`
   - Bundle construction from user notes
   - Submission to JAM network

---

## Migration Benefits

### 1. **Security**
- Battle-tested Zcash cryptography (in production since 2021)
- No custom circuit bugs or vulnerabilities
- Ongoing Zcash security audits benefit JAM

### 2. **Ecosystem**
- Compatible with existing Zcash wallets
- Light client support via standard Orchard tree
- Interoperability with Zcash infrastructure

### 3. **Maintenance**
- Leverage `orchard` crate updates automatically
- No custom circuit maintenance
- Smaller attack surface

### 4. **Standards Compliance**
- ZIP-224: Orchard Action circuit
- ZIP-225: Orchard key components
- ZIP-32: Orchard key derivation
- Zcash v5 transaction format

---

## Breaking Changes

### 1. **No Custom Asset Support**
Orchard uses a **single-asset pool** (ZEC equivalent). The multi-asset UTXO design from Railgun is not supported.

**Mitigation**: Each asset would require a separate Orchard service instance.

### 2. **No Public Withdrawals**
Orchard bundles in "bundle-only mode" cannot include transparent outputs.

**Mitigation**: Bridge protocol needed for cross-service transfers (see ORCHARD.md).

### 3. **Fixed Circuit Size**
Orchard Action circuit is fixed at `K = 11`. Cannot be configured.

**Impact**: Proof generation time and key size are fixed by Zcash.

### 4. **Address Format Change**
Orchard addresses use Zcash's unified address format, not custom encoding.

**Impact**: Users need Zcash-compatible wallets or our FFI-based address generation.

---

## Next Steps

### Week 1-2: FFI Core
- [ ] Integrate `orchard` crate via FFI
- [ ] Implement key generation functions
- [ ] Implement Orchard bundle encoding/decoding

### Week 3-4: Merkle Tree
- [ ] Sinsemilla hash implementation
- [ ] Commitment tree operations
- [ ] Subtree root management

### Week 5-6: RPC Integration
- [ ] Complete RPC handlers with Orchard encoding
- [ ] Address generation via FFI
- [ ] Bundle submission to JAM

### Week 7-8: JAM Integration
- [ ] Refine: Orchard bundle verification
- [ ] Accumulate: State updates
- [ ] Builder binary implementation

---

## Go Code Migration Guide

### 1. Merkle Tree (`builder/orchard/merkle/`)

The Orchard Merkle tree uses **Sinsemilla hashing** instead of Poseidon. The Go code must interface with Rust FFI for all tree operations.

#### Files to Update/Create

**`builder/orchard/merkle/tree.go`**
```go
package merkle

import (
    "github.com/colorfulnotion/jam/common"
    "github.com/colorfulnotion/jam/builder/orchard/ffi"
)

// OrchardTree manages the Orchard commitment tree (Sinsemilla, depth 32)
type OrchardTree struct {
    root [32]byte
    size uint64
    db   *LevelDB  // Persistent storage
}

// NewOrchardTree creates a new Orchard tree
func NewOrchardTree(dataDir string) (*OrchardTree, error) {
    db, err := OpenLevelDB(dataDir + "/orchard_tree")
    if err != nil {
        return nil, err
    }

    tree := &OrchardTree{
        root: [32]byte{},  // Empty tree root
        size: 0,
        db:   db,
    }

    // Load persisted state
    if err := tree.loadState(); err != nil {
        return nil, err
    }

    return tree, nil
}

// AppendCommitment adds a new note commitment to the tree
func (t *OrchardTree) AppendCommitment(cm [32]byte) error {
    // Call Rust FFI for Sinsemilla tree append
    newRoot, err := ffi.OrchardTreeAppend(t.root, t.size, cm)
    if err != nil {
        return err
    }

    t.root = newRoot
    t.size++

    // Persist state
    return t.saveState()
}

// GetMerklePath returns the authentication path for a given index
func (t *OrchardTree) GetMerklePath(index uint64) ([][32]byte, error) {
    if index >= t.size {
        return nil, ErrIndexOutOfBounds
    }

    // Call Rust FFI for Sinsemilla Merkle path
    return ffi.OrchardTreeGetPath(t.root, t.size, index)
}

// GetRoot returns the current tree root
func (t *OrchardTree) GetRoot() [32]byte {
    return t.root
}

// GetSize returns the number of commitments in the tree
func (t *OrchardTree) GetSize() uint64 {
    return t.size
}
```

**`builder/orchard/merkle/nullifiers.go`**
```go
package merkle

import "github.com/colorfulnotion/jam/common"

// NullifierSet tracks spent nullifiers
type NullifierSet struct {
    spent map[[32]byte]bool
    root  [32]byte  // Merkle root of nullifier set (for JAM state)
    db    *LevelDB
}

// NewNullifierSet creates a new nullifier tracker
func NewNullifierSet(dataDir string) (*NullifierSet, error) {
    db, err := OpenLevelDB(dataDir + "/nullifiers")
    if err != nil {
        return nil, err
    }

    ns := &NullifierSet{
        spent: make(map[[32]byte]bool),
        db:    db,
    }

    // Load persisted nullifiers
    if err := ns.loadState(); err != nil {
        return nil, err
    }

    return ns, nil
}

// IsSpent checks if a nullifier has been spent
func (ns *NullifierSet) IsSpent(nf [32]byte) bool {
    return ns.spent[nf]
}

// MarkSpent marks a nullifier as spent
func (ns *NullifierSet) MarkSpent(nf [32]byte) error {
    if ns.spent[nf] {
        return ErrDoubleSpend
    }

    ns.spent[nf] = true

    // Update Merkle root (for JAM state commitment)
    if err := ns.updateRoot(); err != nil {
        return err
    }

    // Persist
    return ns.saveState()
}

// GetRoot returns the Merkle root of the nullifier set
func (ns *NullifierSet) GetRoot() [32]byte {
    return ns.root
}
```

#### Key Changes from Railgun

| Railgun | Orchard |
|---------|---------|
| `blake2Hash(commitment)` | `ffi.OrchardCommitment(...)` via FFI |
| Poseidon Merkle hash | Sinsemilla hash via FFI |
| Custom tree implementation | Leverage `orchard` crate tree |

---

### 2. RPC Handlers (`builder/orchard/rpc/`)

Orchard RPC must be **Zcash-compatible** and use Orchard bundle encoding.

#### Files to Update/Create

**`builder/orchard/rpc/zcash_rpc.go`**
```go
package rpc

import (
    "github.com/colorfulnotion/jam/builder/orchard/witness"
    "github.com/colorfulnotion/jam/builder/orchard/ffi"
)

// OrchardRPC implements Zcash-compatible z_* methods
type OrchardRPC struct {
    builder *witness.OrchardBuilder
}

// ZGetNewAddress generates a new Orchard address
func (r *OrchardRPC) ZGetNewAddress(req []string, res *string) error {
    addrType := req[0]
    if addrType != "orchard" {
        return fmt.Errorf("unsupported address type: %s (must be 'orchard')", addrType)
    }

    // Generate Orchard address via FFI
    address, err := ffi.OrchardGenerateAddress()
    if err != nil {
        return err
    }

    *res = address
    return nil
}

// ZGetTreeState returns Orchard commitment tree state
func (r *OrchardRPC) ZGetTreeState(req []string, res *map[string]interface{}) error {
    height := req[0]  // Block height

    // Get tree state from builder
    treeState, err := r.builder.GetTreeState(height)
    if err != nil {
        return err
    }

    *res = map[string]interface{}{
        "height": height,
        "hash":   treeState.BlockHash.Hex(),
        "time":   treeState.Timestamp,
        "sapling": map[string]interface{}{
            "commitments": map[string]interface{}{
                "finalState": "0x" + hex.EncodeToString(make([]byte, 32)), // Empty for Orchard-only
            },
        },
        "orchard": map[string]interface{}{
            "commitments": map[string]interface{}{
                "finalRoot":  "0x" + hex.EncodeToString(treeState.Root[:]),
                "finalState": encodeOrchardTreeState(treeState),
            },
        },
    }

    return nil
}

// ZSendMany constructs and submits an Orchard bundle
func (r *OrchardRPC) ZSendMany(req []interface{}, res *string) error {
    // Parse recipients
    recipients := parseRecipients(req)

    // Build Orchard bundle via FFI
    bundleBytes, err := ffi.OrchardBuildBundle(recipients)
    if err != nil {
        return err
    }

    // Submit to JAM via builder
    opid, err := r.builder.SubmitBundle(bundleBytes)
    if err != nil {
        return err
    }

    *res = opid  // Operation ID
    return nil
}

// ZGetBalance sums unspent note values
func (r *OrchardRPC) ZGetBalance(req []int, res *float64) error {
    minConf := req[0]

    // Get unspent notes from builder
    notes, err := r.builder.GetUnspentNotes(minConf)
    if err != nil {
        return err
    }

    // Sum values (convert from zatoshi to ZEC equivalent)
    var total uint64
    for _, note := range notes {
        total += note.Value
    }

    *res = float64(total) / 1e8  // Convert to ZEC-like units
    return nil
}
```

**`builder/orchard/rpc/server.go`**
```go
package rpc

import (
    "net/http"
    "github.com/colorfulnotion/jam/builder/orchard/witness"
)

// StartOrchardRPC starts the Orchard RPC server on port 8232
func StartOrchardRPC(builder *witness.OrchardBuilder) error {
    orchardRPC := &OrchardRPC{builder: builder}

    // Register Zcash-compatible methods
    server := rpc.NewServer()
    server.RegisterName("", orchardRPC)  // Methods without prefix (getblockchaininfo)

    // Start HTTP server
    return http.ListenAndServe(":8232", server)
}
```

#### Key Changes from Railgun

| Railgun | Orchard |
|---------|---------|
| Custom `z_sendmany` encoding | Zcash v5 Orchard bundle encoding |
| Multi-asset transfers | Single-asset (Orchard pool) |
| Custom address format | Orchard unified address via FFI |
| Custom tree state JSON | Zcash-compatible tree state format |

---

### 3. Witness/Builder (`builder/orchard/witness/`)

The Orchard builder constructs **Orchard bundles** from user notes and submits them to JAM.

#### Files to Update/Create

**`builder/orchard/witness/builder.go`**
```go
package witness

import (
    "github.com/colorfulnotion/jam/builder/orchard/merkle"
    "github.com/colorfulnotion/jam/builder/orchard/ffi"
    "github.com/colorfulnotion/jam/types"
)

// OrchardBuilder constructs Orchard bundles and submits to JAM
type OrchardBuilder struct {
    serviceID uint32
    tree      *merkle.OrchardTree
    nullifiers *merkle.NullifierSet
    jamClient  types.NodeClient  // CE146/147/148 interface
    notePool   *NotePool
}

// NewOrchardBuilder creates a new Orchard builder
func NewOrchardBuilder(dataDir string, jamClient types.NodeClient) (*OrchardBuilder, error) {
    tree, err := merkle.NewOrchardTree(dataDir)
    if err != nil {
        return nil, err
    }

    nullifiers, err := merkle.NewNullifierSet(dataDir)
    if err != nil {
        return nil, err
    }

    return &OrchardBuilder{
        serviceID:  OrchardServiceID,
        tree:       tree,
        nullifiers: nullifiers,
        jamClient:  jamClient,
        notePool:   NewNotePool(),
    }, nil
}

// BuildBundle constructs an Orchard bundle from pending notes
func (b *OrchardBuilder) BuildBundle() (*types.WorkPackage, error) {
    // 1. Select notes to spend
    notesToSpend := b.notePool.SelectNotes(MaxActions)

    // 2. Build Orchard bundle via FFI
    bundleBytes, err := ffi.OrchardBuildBundle(notesToSpend)
    if err != nil {
        return nil, err
    }

    // 3. Parse bundle to extract nullifiers and commitments
    bundle, err := ffi.OrchardParseBundle(bundleBytes)
    if err != nil {
        return nil, err
    }

    // 4. Verify bundle proof locally (sanity check)
    if err := ffi.OrchardVerifyBundle(bundleBytes); err != nil {
        return nil, err
    }

    // 5. Update local state (optimistic)
    for _, action := range bundle.Actions {
        // Mark nullifiers spent
        if err := b.nullifiers.MarkSpent(action.Nullifier); err != nil {
            return nil, err
        }

        // Append new commitments
        if err := b.tree.AppendCommitment(action.Cmx); err != nil {
            return nil, err
        }
    }

    // 6. Create work package payload
    payload := OrchardBundlePayload{
        BundleBytes: bundleBytes,
        Fee:         bundle.ValueBalance,
        GasLimit:    DefaultGasLimit,
    }

    // 7. Return work package
    return &types.WorkPackage{
        ServiceID: b.serviceID,
        Payload:   payload.Encode(),
    }, nil
}

// SubmitToJAM sends work package to JAM network via CE146
func (b *OrchardBuilder) SubmitToJAM(wp *types.WorkPackage) error {
    return b.jamClient.SubmitWorkPackage(wp)
}

// TrackGuarantees monitors work package status via CE147/148
func (b *OrchardBuilder) TrackGuarantees(wpHash common.Hash) (*GuaranteeStatus, error) {
    return b.jamClient.GetWorkPackageStatus(wpHash)
}

// GetTreeState returns current Orchard tree state
func (b *OrchardBuilder) GetTreeState(height string) (*TreeState, error) {
    // TODO: Map height to block state
    return &TreeState{
        Root: b.tree.GetRoot(),
        Size: b.tree.GetSize(),
    }, nil
}

// SubmitBundle submits an Orchard bundle (from RPC)
func (b *OrchardBuilder) SubmitBundle(bundleBytes []byte) (string, error) {
    // Parse bundle
    bundle, err := ffi.OrchardParseBundle(bundleBytes)
    if err != nil {
        return "", err
    }

    // Create work package
    wp := &types.WorkPackage{
        ServiceID: b.serviceID,
        Payload:   bundleBytes,
    }

    // Submit to JAM
    if err := b.SubmitToJAM(wp); err != nil {
        return "", err
    }

    // Return operation ID
    return generateOperationID(wp), nil
}
```

**`builder/orchard/witness/notepool.go`**
```go
package witness

import "github.com/colorfulnotion/jam/common"

// NotePool manages unspent Orchard notes
type NotePool struct {
    notes []OrchardNote
}

// OrchardNote represents an unspent Orchard note
type OrchardNote struct {
    Commitment [32]byte
    Nullifier  [32]byte
    Value      uint64
    Rho        [32]byte
    Psi        [32]byte
    Address    string
}

// AddNote adds a received note to the pool
func (np *NotePool) AddNote(note OrchardNote) {
    np.notes = append(np.notes, note)
}

// SelectNotes selects notes for spending (up to maxActions)
func (np *NotePool) SelectNotes(maxActions int) []OrchardNote {
    if len(np.notes) <= maxActions {
        return np.notes
    }
    return np.notes[:maxActions]
}

// GetUnspentNotes returns all unspent notes with minConf confirmations
func (np *NotePool) GetUnspentNotes(minConf int) ([]OrchardNote, error) {
    // TODO: Filter by confirmations
    return np.notes, nil
}
```

#### Key Changes from Railgun

| Railgun | Orchard |
|---------|---------|
| Custom proof batching | Orchard bundle proof (single proof for N actions) |
| Multi-asset note selection | Single-asset Orchard notes |
| `BuildBundle()` with custom circuits | `ffi.OrchardBuildBundle()` via FFI |
| Custom witness generation | Orchard witness via `orchard` crate |

---

## FFI Integration Points

All three Go packages (`merkle`, `rpc`, `witness`) rely on Rust FFI for cryptographic operations:

### Required FFI Functions

```go
// builder/orchard/ffi/ffi.go
package ffi

// #cgo LDFLAGS: -L../../target/release -lorchard_ffi
// #include "orchard.h"
import "C"

// OrchardGenerateAddress generates a new Orchard address
func OrchardGenerateAddress() (string, error)

// OrchardBuildBundle constructs an Orchard bundle from notes
func OrchardBuildBundle(notes []OrchardNote) ([]byte, error)

// OrchardVerifyBundle verifies an Orchard bundle proof
func OrchardVerifyBundle(bundleBytes []byte) error

// OrchardParseBundle decodes an Orchard bundle
func OrchardParseBundle(bundleBytes []byte) (*OrchardBundle, error)

// OrchardTreeAppend appends a commitment to the tree
func OrchardTreeAppend(root [32]byte, size uint64, cm [32]byte) ([32]byte, error)

// OrchardTreeGetPath returns Merkle path for an index
func OrchardTreeGetPath(root [32]byte, size uint64, index uint64) ([][32]byte, error)
```

See [orchard/docs/FFI.md](../orchard/docs/FFI.md) for complete FFI specification.

---

## Testing Strategy

### Unit Tests

**Merkle Tree**:
```go
// builder/orchard/merkle/tree_test.go
func TestOrchardTreeAppend(t *testing.T) {
    tree := NewOrchardTree(t.TempDir())

    // Append commitment
    cm := [32]byte{1, 2, 3, ...}
    err := tree.AppendCommitment(cm)
    require.NoError(t, err)

    // Verify size incremented
    assert.Equal(t, uint64(1), tree.GetSize())
}
```

**RPC Handlers**:
```go
// builder/orchard/rpc/zcash_rpc_test.go
func TestZGetNewAddress(t *testing.T) {
    rpc := &OrchardRPC{builder: mockBuilder}

    var result string
    err := rpc.ZGetNewAddress([]string{"orchard"}, &result)
    require.NoError(t, err)

    // Verify Orchard address format
    assert.True(t, strings.HasPrefix(result, "u1"))
}
```

**Builder**:
```go
// builder/orchard/witness/builder_test.go
func TestBuildBundle(t *testing.T) {
    builder := NewOrchardBuilder(t.TempDir(), mockJAMClient)

    // Add notes to pool
    builder.notePool.AddNote(testNote1)
    builder.notePool.AddNote(testNote2)

    // Build bundle
    wp, err := builder.BuildBundle()
    require.NoError(t, err)

    // Verify work package
    assert.Equal(t, OrchardServiceID, wp.ServiceID)
}
```

---

## References

### Zcash Protocol
- [Zcash Protocol Specification](https://zips.z.cash/protocol/protocol.pdf) - Section 4.1.8 (Orchard)
- [ZIP-224: Orchard Shielded Protocol](https://zips.z.cash/zip-0224)
- [ZIP-225: Version 5 Transaction Format](https://zips.z.cash/zip-0225)

### Implementation
- [`orchard` crate documentation](https://docs.rs/orchard/)
- [`zcash_primitives` crate](https://docs.rs/zcash_primitives/)
- [Halo2 Book](https://zcash.github.io/halo2/)

### JAM Orchard Docs
- [ORCHARD.md](../orchard/docs/ORCHARD.md) - Complete protocol specification
- [RPC.md](../orchard/docs/RPC.md) - Zcash RPC reference
- [FFI.md](../orchard/docs/FFI.md) - FFI integration guide
- [WALLET.md](../orchard/docs/WALLET.md) - Wallet integration
- [LIGHTCLIENT.md](../orchard/docs/LIGHTCLIENT.md) - Light client support

---

**End of Migration Document**
