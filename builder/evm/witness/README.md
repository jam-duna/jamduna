# EVM Builder & Witness Preparation

**Status**: 🚧 Pending Migration

This directory will contain EVM work package building and witness preparation logic.

## Planned Files

- `builder.go` - Main EVMBuilder implementation
- `bundle.go` - BuildBundle logic
- `witness.go` - Verkle witness generation
- `txpool.go` - Transaction selection and pooling

## Migration Source

Core logic will be migrated from:
- `statedb/rollup.go` - Rollup/builder logic

## Responsibilities

The `EVMBuilder` will:
1. **Select transactions** from mempool
2. **Execute transactions** against Verkle state
3. **Generate Verkle witnesses** for state changes
4. **Build work packages** with transactions + receipts + witnesses
5. **Submit to JAM** via CE146 NodeClient interface
6. **Track guarantees** via CE147/148

## Interface

```go
type EVMBuilder struct {
    storage   verkle.VerkleStateStorage
    jamClient types.NodeClient
    txPool    *TxPool
}

func (b *EVMBuilder) BuildBundle() (*types.WorkPackage, error)
func (b *EVMBuilder) SubmitToJAM(wp *types.WorkPackage) error
func (b *EVMBuilder) TrackGuarantees(wpHash common.Hash) (*GuaranteeStatus, error)
```

See [../docs/ROLLUP.md](../../docs/ROLLUP.md) for full migration plan.
