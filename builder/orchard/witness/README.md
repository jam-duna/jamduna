# Railgun Builder & Witness Preparation

**Status**: 🚧 Pending Migration

This directory will contain Railgun work package building and Merkle witness preparation logic.

## Planned Files

- `builder.go` - Main RailgunBuilder implementation
- `bundle.go` - BuildBundle logic
- `witness.go` - Merkle witness generation
- `proofs.go` - Halo2 proof verification
- `pool.go` - Proof pool management

## Migration Source

Core logic will be migrated from:
- `statedb/railgun_rollup.go` - Railgun rollup/builder logic

## Responsibilities

The `RailgunBuilder` will:
1. **Collect Halo2 proofs** from users (spend/withdraw/issuance)
2. **Verify proofs locally** before inclusion
3. **Update Merkle state** (append commitments, mark nullifiers spent)
4. **Generate Merkle witnesses** for state changes
5. **Build work packages** with proofs + witnesses
6. **Submit to JAM** via CE146 NodeClient interface
7. **Track guarantees** via CE147/148

## Proof Types

Railgun uses **Halo2 on Pasta curves (Pallas/Vesta) with IPA PCS**:

- `vk_id=1`: spend_v1 (44 public inputs, "railgun_spend_v1" domain)
- `vk_id=2`: withdraw_v1 (17 public inputs, "railgun_withdraw_v1" domain)
- `vk_id=3`: issuance_v1 (11 public inputs, "railgun_issue_v1" domain)
- `vk_id=4`: batch_agg_v1 (17 public inputs, "railgun_batch_v1" domain)

## Interface

```go
type RailgunBuilder struct {
    storage   merkle.MerkleStateStorage
    jamClient types.NodeClient
    proofPool *ProofPool
}

func (b *RailgunBuilder) BuildBundle() (*types.WorkPackage, error)
func (b *RailgunBuilder) SubmitToJAM(wp *types.WorkPackage) error
func (b *RailgunBuilder) TrackGuarantees(wpHash common.Hash) (*GuaranteeStatus, error)
```

See [../docs/DESIGN.md](../../../services/railgun/docs/DESIGN.md) for circuit specifications.
See [../../docs/ROLLUP.md](../../docs/ROLLUP.md) for migration plan.
