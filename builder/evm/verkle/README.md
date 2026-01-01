# EVM Verkle State Management

**Status**: 🚧 Pending Migration

This directory will contain EVM Verkle state management code migrated from `storage/verkle*.go`.

## Planned Files

- `verkle.go` - Core Verkle state operations
- `verkle_compact.go` - Compact proof generation
- `verkle_delta.go` - State delta tracking
- `verkle_proof.go` - Verkle proof generation/verification
- `verkle_replay.go` - State replay functionality
- `verkle_interface.go` - Public interface definitions

## Migration Source

Files will be moved from:
- `storage/verkle.go`
- `storage/verkle_compact.go`
- `storage/verkle_delta.go`
- `storage/verkle_proof.go`
- `storage/verkle_replay.go`

## Interface

The main interface will be `VerkleStateStorage` providing:
- State access (GetBalance, GetNonce, GetCode, GetStorageAt)
- State modification (SetBalance, SetNonce, SetCode, SetStorageAt)
- Witness generation (GenerateWitness)

See [../docs/ROLLUP.md](../../docs/ROLLUP.md) for full migration plan.
