# EVM Service

A JAM service providing Ethereum Virtual Machine (EVM) execution capabilities with DA-backed state storage and JAM-native bootstrapping.

## Documentation

For detailed information on specific components and concepts, see:

- **[Backend Architecture](docs/BACKEND.md)** - MajikBackend implementation and storage architecture
- **[Blocks and Payloads](docs/BLOCKS.md)** - Payload types, witness handling, and execution flow
- **[RPC Interface](docs/RPC.md)** - JSON-RPC API specification and endpoints
- **[Stablecoins & Cores](docs/STABLECOINS-CORES.md)** - System contract responsibilities and token accounting
- **[UBT Integration](docs/UBT-CODEX.md)** - Witness formats, verification, and ExecutionMode usage

## Overview

The EVM service implements a full Ethereum-compatible runtime within the JAM protocol, featuring:

- **EVM Transaction Execution**: Standard Ethereum transaction processing with gas metering
- **DA-Backed State**: Contract storage and code stored in JAM's Data Availability layer
- **Bootstrap Protocol**: Genesis state initialization using JAM work packages
- **System Contracts**: USDM token contract for universal accounting

## Architecture

### Core Components

- **`MajikBackend`**: DA-style storage backend with object versioning and caching (see [Backend Architecture](docs/BACKEND.md))
- **`MajikOverlay`**: Transaction isolation and dependency tracking (see [Backend Architecture](docs/BACKEND.md))
- **Bootstrap Interpreter**: Genesis state initialization from extrinsics
- **Sharded Storage**: SSR-based storage with automatic sharding and caching (see [UBT Integration](docs/UBT-CODEX.md))

### Service Accord

The service follows the Majik Service Accord for object versioning:

- **`ObjectRef`**: References an object version by work package hash, segment range, version, and timeslot
- **`WriteEffectEntry`**: Combines object ID, reference info, header, and payload
- **`WriteIntent`**: Wraps a write effect with its dependencies

## Bootstrap Protocol

### Genesis Work Package (Payload "G")

The genesis bootstrap runs inside the first work package using the standard refine → accumulate pipeline. The runtime switches to a bootstrap interpreter that processes specialized extrinsics to establish initial state.

#### Bootstrap Extrinsic Format

| Command | Layout | Purpose | Example Size |
|---------|--------|---------|--------------|
| `0x41` (`'A'`) | `[0x41][address:20][code_len:u32 LE][code:code_len]` | Deploy contract code | 2704 bytes (USDM) |
| `0x4B` (`'K'`) | `[0x4B][address:20][storage_key:32][storage_value:32]` | Set storage slot | 85 bytes |

#### USDM System Contract Bootstrap

For the USDM contract at `0x0000000000000000000000000000000000000001`:

1. **Contract Code** (1 extrinsic):
   - Deploy USDM bytecode (2679 bytes)

2. **Contract Storage** (2 extrinsics):
   - Issuer balance: `keccak256(abi.encode(issuer, 0))` → `U256(61_000_000e18)`
   - Issuer nonce: `keccak256(abi.encode(issuer, 1))` → `U256(1)`

### Object Creation

Bootstrap creates three DA objects per contract:

1. **Code Object** (`ObjectKind::Code`)
   - Object ID: `[address:20][0x00][zeros:11]`
   - Payload: Raw contract bytecode

2. **SSR Metadata** (`ObjectKind::SsrMetadata`)
   - Object ID: `[address:20][0x02][zeros:11]`
   - Payload: Storage structure metadata

3. **Storage Shard** (`ObjectKind::StorageShard`)
   - Object ID: `[address:20][0x01][ld:1][prefix56:7][zeros:3]`
   - Payload: Serialized storage entries

## Normal Execution Flow

### Work Packages 2+ (Payload "0" or "B")

After bootstrap, the service processes standard EVM transactions:

1. **Backend Initialization**: `MajikBackend::new()` loads imported DA objects
2. **Transaction Execution**: Standard EVM processing with gas metering
3. **State Updates**: Modified storage exported as new DA objects
4. **Dependencies**: Objects reference previous versions via `ObjectRef`

### Caching Optimizations

- **Balance Caching**: Prevents infinite DA fetches for cross-instance access (see [Backend Architecture](docs/BACKEND.md))
- **Negative Code Caching**: Avoids repeated DA lookups for EOAs (Externally Owned Accounts)
- **Storage Sharding**: Efficient access to contract storage via SSR resolution (see [UBT Integration](docs/UBT-CODEX.md))

## Building

```bash
make evm
```

This produces `services/evm/evm.pvm` ready for JAM runtime deployment.

## Testing

The service includes comprehensive testing for:

- Bootstrap protocol execution
- EVM transaction processing
- DA object import/export
- Storage sharding and caching
- Gas metering and limits

### Example Test Flow

```bash
# Build the service
make evm

# Run JAM with EVM service (see [RPC Interface](docs/RPC.md) for API endpoints)
./bin/mac-amd64/jamduna run --chain chainspecs/jamduna-spec.json
```

## Benefits

- **JAM-Native**: Uses standard ObjectRef dependencies and DA exports
- **Deterministic**: Identical bootstrap extrinsics produce identical state
- **Extensible**: Additional contracts can be bootstrapped with more `A`/`K` commands
- **Seamless**: Automatic handoff from bootstrap to normal execution
- **Scalable**: Automatic storage sharding for large contract state
- **Auditable**: Clear versioning chain through object references

## State Structure

### System Address Layout

- `0x01`: USDM system contract (universal token accounting)
- `0x02-0xFF`: Reserved for future system contracts
- User contracts: Standard Ethereum addresses

### Storage Layout

Contract storage uses JAM's sharded approach (see [UBT Integration](docs/UBT-CODEX.md) for details):
- SSR metadata tracks storage structure
- Individual shards contain key-value pairs
- Automatic sharding based on storage access patterns
- Negative caching for non-existent accounts

This design provides Ethereum compatibility while leveraging JAM's DA layer for scalable, verifiable state storage.

## See Also

- **[Backend Architecture](docs/BACKEND.md)** - Detailed implementation of storage backends and overlay coordination
- **[Blocks and Payloads](docs/BLOCKS.md)** - Payload handling, witness placement, and block assembly
- **[RPC Interface](docs/RPC.md)** - External interfaces and service integration
- **[UBT Integration](docs/UBT-CODEX.md)** - State commitments, proofs, and execution modes
