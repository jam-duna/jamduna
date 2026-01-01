# Builder Refactoring Status

**Last Updated**: 2025-12-30

**Major Change**: Replaced Railgun with Zcash Orchard protocol (see [docs/ORCHARD_MIGRATION.md](docs/ORCHARD_MIGRATION.md))

---

## ✅ Phase 1: Complete

**Goal**: Establish new directory structure without breaking existing code.

### Completed Tasks

- ✅ Created `builder/` directory structure
- ✅ Created `builder/docs/ROLLUP.md` with complete refactoring plan
- ✅ Created `builder/evm/` subdirectories (verkle, witness, rpc, docs)
- ✅ Created `builder/orchard/` subdirectories (merkle, witness, rpc, docs)
- ✅ Added README.md files documenting each directory's purpose
- ✅ Documented migration plan for each subsystem
- ✅ **Migrated from Railgun to Orchard** (Zcash protocol)

### Directory Structure

```
builder/
├── docs/
│   ├── ROLLUP.md                    # Complete refactoring guide
│   └── ORCHARD_MIGRATION.md         # Railgun → Orchard migration doc
├── evm/
│   ├── verkle/
│   │   └── README.md                # Verkle state management docs
│   ├── witness/
│   │   └── README.md                # BuildBundle + witness prep docs
│   ├── rpc/
│   │   └── README.md                # EVM user RPC docs
│   └── docs/                        # EVM-specific docs (empty)
├── orchard/
│   ├── merkle/
│   │   └── README.md                # Orchard Sinsemilla tree docs
│   ├── witness/
│   │   └── README.md                # Orchard bundle building docs
│   ├── rpc/
│   │   └── README.md                # Zcash-compatible z_* RPC docs
│   ├── src/                         # Rust FFI implementation
│   ├── Cargo.toml                   # Orchard crate dependencies
│   └── docs/
│       ├── ORCHARD.md               # Complete protocol spec
│       ├── RPC.md                   # Zcash RPC reference
│       ├── FFI.md                   # FFI integration guide
│       ├── WALLET.md                # Wallet integration
│       └── LIGHTCLIENT.md           # Light client support
└── STATUS.md                        # This file
```

### Files Created

1. **builder/docs/ROLLUP.md** - Complete architectural refactoring plan
2. **builder/docs/ORCHARD_MIGRATION.md** - Railgun → Orchard migration documentation
3. **builder/evm/verkle/README.md** - EVM Verkle state migration plan
4. **builder/evm/witness/README.md** - EVM builder/witness documentation
5. **builder/evm/rpc/README.md** - EVM user RPC migration plan
6. **builder/orchard/merkle/README.md** - Orchard Sinsemilla tree design
7. **builder/orchard/witness/README.md** - Orchard bundle building documentation
8. **builder/orchard/rpc/README.md** - Zcash-compatible RPC migration plan
9. **builder/orchard/docs/ORCHARD.md** - Complete Zcash Orchard protocol specification
10. **builder/orchard/docs/RPC.md** - Full Zcash RPC reference
11. **builder/orchard/docs/FFI.md** - FFI architecture and interface design
12. **builder/orchard/docs/WALLET.md** - Wallet integration guide
13. **builder/orchard/docs/LIGHTCLIENT.md** - Light client support

---

## 🚧 Phase 2: Pending

**Goal**: Separate JAM storage from builder storage.

### Tasks

- [ ] Move `storage/verkle*.go` → `builder/evm/verkle/`
- [ ] Implement `builder/orchard/merkle/` (Orchard Sinsemilla tree)
- [ ] Implement Orchard FFI integration (`builder/orchard/src/`)
- [ ] Update import paths
- [ ] Test storage isolation

### Orchard-Specific Tasks

- [ ] **FFI Core** (Week 1-2)
  - [ ] Integrate `orchard` crate via FFI
  - [ ] Implement key generation functions
  - [ ] Implement Orchard bundle encoding/decoding

- [ ] **Merkle Tree** (Week 3-4)
  - [ ] Sinsemilla hash implementation
  - [ ] Commitment tree operations (depth 32)
  - [ ] Subtree root management for light clients

---

## 🚧 Phase 3: Pending

**Goal**: Separate JAM state transitions from builder business logic.

### Tasks

- [ ] Move `statedb/rollup.go` → `builder/evm/witness/builder.go`
- [ ] Implement `builder/orchard/witness/builder.go` (Orchard bundle building)
- [ ] Implement `Builder` interface for both services
- [ ] Test builder isolation

### Orchard Bundle Building

- [ ] Note selection and transaction construction
- [ ] Orchard Action circuit witness generation
- [ ] Bundle proof generation (Halo2/IPA)
- [ ] Spend authorization signatures
- [ ] Binding signature generation

---

## 🚧 Phase 4: Pending

**Goal**: Separate JAM network RPC from user-facing RPC.

### Tasks

- [ ] Move `node/node_evm_rpc.go` → `builder/evm/rpc/eth_rpc.go`
- [ ] Implement `builder/orchard/rpc/zcash_rpc.go` (Zcash-compatible handlers)
- [ ] Update RPC registration for independent servers
- [ ] Test RPC isolation

### Orchard RPC Methods

- [ ] `z_getnewaddress` - Orchard address generation via FFI
- [ ] `z_gettreestate` - Orchard tree state from JAM storage
- [ ] `z_sendmany` - Orchard bundle submission
- [ ] `getblockchaininfo` - Orchard pool statistics

---

## 🚧 Phase 5: Pending

**Goal**: Create separate builder binaries.

### Tasks

- [ ] Create `cmd/evm-builder/main.go`
- [ ] Create `cmd/orchard-builder/main.go`
- [ ] Implement CE146/147/148 integration (builder → JAM)
- [ ] Test builder-to-JAM communication

### Orchard Builder Binary

- [ ] Independent HTTP server (port 8232)
- [ ] Zcash-compatible RPC interface (z_*)
- [ ] Orchard bundle submission to JAM
- [ ] Note pool and key management

---

## Next Steps

### For EVM (Phase 2)
```bash
# Copy verkle files to new location
cp storage/verkle*.go builder/evm/verkle/

# Update package declarations
sed -i '' 's/package storage/package verkle/g' builder/evm/verkle/*.go
```

### For Orchard (Phase 2)
```bash
# Implement Orchard Sinsemilla tree
cd builder/orchard
cargo build --release

# Implement FFI bindings
# (see builder/orchard/docs/FFI.md for interface design)
```

See [builder/docs/ROLLUP.md](docs/ROLLUP.md) for the complete refactoring plan.
See [builder/docs/ORCHARD_MIGRATION.md](docs/ORCHARD_MIGRATION.md) for Orchard migration details.

---

## Orchard Protocol Summary

**Key Differences from Railgun:**

| Feature | Railgun (Old) | Orchard (New) |
|---------|---------------|---------------|
| Protocol | Custom | Zcash Orchard (production-ready) |
| Circuits | Multiple VKs (4 types) | Single Action circuit (K=11) |
| Public Inputs | 44/17/11 fields | 9 fields per action |
| Hash Function | Poseidon | Sinsemilla |
| Asset Support | Multi-asset | Single asset |
| Compatibility | Custom wallets | Zcash ecosystem |
| Maintenance | Custom circuits | `orchard` crate updates |

**Benefits:**
- ✅ Production-ready cryptography from Zcash
- ✅ Ecosystem compatibility (wallets, tools)
- ✅ Simplified maintenance
- ✅ Battle-tested security

**Trade-offs:**
- ❌ Single-asset pool only
- ❌ No public withdrawals in bundle-only mode
- ❌ Fixed circuit size (K=11)

---

**End of Status Document**
