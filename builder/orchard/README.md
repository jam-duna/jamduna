# JAM Orchard Service Builder

Pure Rust implementation for building JAM work packages for the Orchard privacy service.

## Quick Start

```bash
# Build the project
cargo build --release --manifest-path builder/orchard/Cargo.toml

# Run witness-based demo (shows complete verification flow)
cargo run --release --bin orchard-builder -- witness-demo

# Generate sequence of work packages
cargo run --release --bin orchard-builder -- sequence --count 5

# With circuit feature (enables Halo2 verification infrastructure)
cargo build --release --features circuit
```

## What This Does

Creates JAM work packages for Orchard (Zcash's privacy protocol) containing three verification components:

1. **Pre-State Witnesses** - Merkle proofs proving what data was read from state
2. **User Orchard Bundles** - User-submitted transactions with Halo2 zkSNARK proofs
3. **Post-State Witnesses** - Merkle proofs proving new state correctly computed

JAM validators verify all three to ensure correct, privacy-preserving state transitions.

## Architecture at a Glance

```
┌─────────────────────────────────────────────────────────┐
│  JAM Work Package                                       │
├─────────────────────────────────────────────────────────┤
│  1. Pre-State Witnesses (Merkle Proofs)                │
│     └─ Prove: Data exists in current state roots       │
│                                                          │
│  2. User Bundles (Halo2 zkSNARK Proofs)                │
│     └─ Prove: Transactions valid, private, authorized  │
│                                                          │
│  3. Post-State Witnesses (Merkle Proofs)                │
│     └─ Prove: New state roots correctly computed       │
└─────────────────────────────────────────────────────────┘

JAM State: 128 bytes (4 × 32-byte roots)
├─ commitment_root  (unspent notes)
├─ nullifier_root   (spent notes)
├─ fee_root         (builder fees)
└─ commitment_size  (tree size)
```

## Demo Commands

```bash
# Show complete witness-based verification
cargo run --bin orchard-builder -- witness-demo

# Generate 5 work packages with state chaining
cargo run --bin orchard-builder -- sequence --count 5

# Inspect generated package
cargo run --bin orchard-builder -- inspect work_packages/package_000.json
```

## What Gets Verified

### 1. Pre-State Witnesses (Merkle Proofs)
Prove accessed data exists in state:
- Commitment proofs (spent notes exist in tree)
- Nullifier absence proofs (not double-spent)
- Fee balance proofs (builder has balance)

### 2. User Bundles (Halo2 Proofs)
Each user's wallet generates Bundle<Authorized, i64> proving:
- ✅ Authorization (spender owns notes)
- ✅ Existence (notes in Merkle tree)
- ✅ Conservation (value balanced)
- ✅ Uniqueness (nullifiers prevent double-spend)
- ✅ Validity (commitments well-formed)
- ❌ WITHOUT revealing: amounts, senders, receivers

### 3. Post-State Witnesses (Merkle Proofs)
Prove new state correctly computed:
- Commitment insertion proofs
- Nullifier insertion proofs
- Fee update proofs

## Performance

```
Verification per work package:
  Pre-state witnesses:     ~1ms
  User bundles (N users):  ~10ms × N
  Post-state witnesses:    ~1ms
  Total: ~2ms + 10ms per user
```

## Demo vs Production

### Current Demo
- ✅ Correct architecture
- ✅ Merkle witness verification
- ✅ State transition logic
- ❌ No real Halo2 proofs (requires transaction witnesses)

### Production Needs
- Real user wallets (Bundle<Authorized, i64>)
- Proving key (Halo2 keygen, cached)
- Verifying key (fast, deterministic)
- Transaction witnesses (notes, paths, keys)

**Why no real proofs in demo?**
Creating bundles requires real transaction data (notes from previous txs, Merkle paths, spending keys). The verification code IS ready - we just don't have real transaction witnesses.

See [ARCHITECTURE.md](ARCHITECTURE.md) for complete details.

## Key Files

```
src/
├── witness_based.rs    # Core: Stateless verification
├── merkle_impl.rs      # Incremental + sparse Merkle trees
├── state.rs            # JAM Orchard state (128 bytes)
└── bin/
    └── orchard_builder.rs  # CLI tool
```

## Features

- `std`: Standard library (default)
- `circuit`: Halo2 circuit verification
- `ffi`: FFI bindings for Go

## Next Steps

1. **Run the demo**: `cargo run --bin orchard-builder -- witness-demo`
2. **Read the architecture**: See [ARCHITECTURE.md](ARCHITECTURE.md)
3. **Explore the code**: Start with `src/witness_based.rs`

## References

- [ARCHITECTURE.md](ARCHITECTURE.md) - Complete technical design
- [Orchard Spec](https://zips.z.cash/protocol/protocol.pdf)
- [Halo2 Docs](https://zcash.github.io/halo2/)
- [JAM Graypaper](https://graypaper.com/)
