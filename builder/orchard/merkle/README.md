# Railgun Merkle State Management

**Status**: 🚧 New Implementation

This directory will contain Railgun Merkle tree state management for the privacy-native UTXO service.

## Planned Files

- `merkle.go` - Core Merkle tree operations (commitment tree)
- `nullifiers.go` - Nullifier set management
- `witness.go` - Merkle witness/proof generation
- `merkle_interface.go` - Public interface definitions
- `storage.go` - Persistent storage layer

## Design

Railgun uses a **single global commitment tree** (Poseidon Merkle, depth=32) shared by all assets:

```
commitment_root : Hash
commitment_size : u64
```

And a **nullifier set** to prevent double-spends:

```
nullifier_<nf> → [1u8; 32]
```

## Interface

The main interface will be `MerkleStateStorage` providing:

```go
type MerkleStateStorage interface {
    // Commitment tree operations
    GetCommitmentRoot() (common.Hash, error)
    GetCommitmentSize() (uint64, error)
    AppendCommitment(commitment common.Hash) error
    GetMerklePath(index uint64) ([]common.Hash, error)

    // Nullifier set operations
    IsNullifierSpent(nullifier common.Hash) (bool, error)
    MarkNullifierSpent(nullifier common.Hash) error

    // Witness generation
    GenerateCommitmentWitness(index uint64) ([]byte, error)
    GenerateNullifierWitness(nullifier common.Hash) ([]byte, error)
}
```

## Cryptography

- **Hash**: Poseidon (in-circuit compatible)
- **Tree depth**: 32 levels
- **Commitment**: `Poseidon("note_cm_v1", asset_id, amount, owner_pk, rho, note_rseed, unlock_height, memo_hash)`
- **Nullifier**: `Poseidon("nf_v1", sk_spend, rho, cm)`

See [../docs/DESIGN.md](../../../services/railgun/docs/DESIGN.md) for full protocol design.
See [../../docs/ROLLUP.md](../../docs/ROLLUP.md) for migration plan.
