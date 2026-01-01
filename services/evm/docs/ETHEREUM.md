# Ethereum Integration via Verkle State Validation

**Status**: 🚧 Design Phase - Implementation Plan

**Last Updated**: 2025-12-21

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Verkle State Validation](#verkle-state-validation)
4. [Beacon Chain Service](#beacon-chain-service)
5. [Foreign EVM Chain Services](#foreign-evm-chain-services)
6. [Asset Registry](#asset-registry)
7. [Inbound Transfers (Ethereum → JAM)](#inbound-transfers-ethereum--jam)
8. [Outbound Transfers (JAM → Ethereum)](#outbound-transfers-jam--ethereum)
9. [Security Model](#security-model)
10. [Implementation Plan](#implementation-plan)

---

## Overview

### Goal

Enable **bi-directional asset transfers** between Ethereum (and other EVM chains like Base) and JAM services, using **100% Verkle-based state validation** without relying on light clients, merkle proofs, or block headers.

### Key Principles

1. **Verkle-Only Validation**: All Ethereum state transitions are validated by reading/writing Verkle tree keys
2. **External Submission**: Third-party relayers submit work packages representing Ethereum state transitions
3. **Finalization via Beacon Chain**: A dedicated Beacon Chain service validates finality signals
4. **Whitelisted Contracts**: Only approved bridge contracts can mint/burn foreign assets
5. **Asset Registry**: Foreign assets (USDC, USDT, WBTC, WETH) have globally registered asset IDs in JAM State
6. **Service-to-Service Transfers**: Foreign chains are represented as JAM services (Ethereum=1, Base=10, etc.)

### Design Choices

| Choice | Rationale |
|--------|-----------|
| Verkle-only (no light clients) | Simplifies validation, leverages JAM's native Verkle infrastructure |
| External WP submission | Decentralizes relaying, anyone can submit state transitions |
| Beacon chain finality | Ethereum's canonical finality mechanism (vs. probabilistic) |
| Whitelisted contracts | Security: prevents unauthorized minting |
| Global asset registry | Enables cross-service asset fungibility |
| Outgoing queue in JAM State | Provable withdrawals for Ethereum contracts |

### Proof-of-Concept Objectives

Focused scope for the first working demo:

- **Single-chain target**: Ethereum mainnet state emulated on Sepolia; Beacon finality can be stubbed with "always finalized" unless explicitly toggled.
- **Block-by-block submission**: One work package per execution block with a bounded modified-key set and pre-generated Verkle multiproof.
- **Minimal bridge surface**: Parse one whitelisted bridge contract emitting `DepositInitiated(asset_id, amount, service_id, memo)` and emit a single `AccumulateInstruction::Transfer` per event.
- **Simplified registry**: Hard-code a tiny asset map (e.g., USDC=1337, WETH=2002) in service state for memo decoding.
- **Operator toolchain**: CLI helper to serialize `EthereumStateTransition` + witness into a JAM work package, plus a smoke-test that replays one Sepolia block.

**PoC acceptance tests** (manual or scripted):

1. Submit a work package for a Sepolia block containing a bridge deposit; refine verifies the multiproof and accumulate queues an inbound transfer for the target service.
2. A second work package updates to the next block with no bridge events; state root advances and the block number increments without errors.
3. When Beacon finality is toggled off (simulating reorg risk), the bridge event is rejected until finality is re-enabled.

### Sepolia PoC fast path (10-day budget)

- **Environment**: Sepolia execution payloads with an “always finalized” flag default-on; toggleable to test Beacon dependency.
- **Scope**: single whitelisted bridge contract, single asset map, one block-at-a-time submission with ≤512 modified stems.
- **Day 1-2**: extract Sepolia block + witness into a deterministic fixture; hard-code asset registry map.
- **Day 3-4**: wire refine/accumulate for `EthereumStateTransition` with multiproof verification and block number tracking.
- **Day 5-6**: CLI helper to package fixture into a WP (`evm-wp pack --block <n>`) and replay it against the service.
- **Day 7**: negative-path toggle for Beacon finality gate; ensure deposits fail when finality is disabled.
- **Day 8**: smoke test with a second empty block; confirm root/number advance deterministically.
- **Day 9-10**: bench multiproof verification and WP decoding; capture timings in docs/logs.

**PoC performance targets** (measured on Sepolia fixture):
- Verkle multiproof verification: <750ms for ≤512 keys on a laptop-class CPU.
- WP decode + apply (refine): <250ms.
- Accumulate processing per bridge event: <50ms (single event per block in PoC).
- WP size bound: <512 KiB per block; reject above bound with clear error.
- CLI packaging time: <5s to produce WP + witness from fixture JSON.

---

## Architecture

### Service Topology

```
┌─────────────────────────────────────────────────────────────────┐
│                        JAM Network                               │
│                                                                  │
│  ┌──────────────────┐      ┌──────────────────┐                │
│  │ Beacon Chain (0) │      │  Ethereum (1)    │                │
│  │ Service          │◄─────┤  EVM Service     │                │
│  │                  │ finality                 │                │
│  │ - Validates      │      │ - Verkle root    │                │
│  │   finality       │      │ - State keys     │                │
│  │ - Epoch data     │      │ - Bridge events  │                │
│  └──────────────────┘      └──────────────────┘                │
│                                     │                            │
│                                     │ DeferredTransfer           │
│                                     ▼                            │
│  ┌──────────────────────────────────────────────────┐          │
│  │         Target JAM Service (e.g., Railgun)       │          │
│  │                                                   │          │
│  │  Accumulate receives:                             │          │
│  │    - sender_service: 1 (Ethereum)                │          │
│  │    - amount: 1000 * 10^6 (USDC decimals)         │          │
│  │    - asset_id: 1337 (USDC)                       │          │
│  │    - memo: [recipient_pk, ...]                   │          │
│  │                                                   │          │
│  │  → Mints 1000 USDC into internal ledger          │          │
│  └──────────────────────────────────────────────────┘          │
│                                     │                            │
│                                     │ DeferredTransfer (outbound)│
│                                     ▼                            │
│  ┌──────────────────┐      ┌──────────────────┐                │
│  │  Base (10)       │      │ Other chains...  │                │
│  │  EVM Service     │      │                  │                │
│  └──────────────────┘      └──────────────────┘                │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
                                │
                                │ Off-chain relayer
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Ethereum Mainnet                              │
│                                                                  │
│  JAMBridge.sol (whitelisted contract)                           │
│    - Reads JAM outgoing queue proof                             │
│    - Verifies Verkle proof against JAM root                     │
│    - Unlocks USDC from escrow → user receives on Ethereum       │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Service IDs

| Service ID | Chain | Purpose |
|------------|-------|---------|
| 0 | Beacon Chain | Finality validation (Ethereum consensus layer) |
| 1 | Ethereum Mainnet | Execution layer state validation |
| 10 | Base | L2 EVM chain state validation |
| 42161 | Arbitrum | L2 EVM chain (future) |
| ... | ... | Other EVM chains |

**Note**: Service 0 (Beacon Chain) validates finality for Ethereum (Service 1). Other chains may have different finality mechanisms.

---

## Verkle State Validation

### Core Concept

Instead of validating Ethereum blocks via headers + Merkle proofs, JAM validators validate **Verkle state transitions** directly:

1. **Relayer submits work package**: Contains Ethereum state transition (block N → block N+1)
2. **JAM service stores Verkle root**: Service state tracks current Ethereum Verkle root
3. **Refine validates transition**: Reads/writes thousands of Verkle keys to validate state changes
4. **Accumulate updates root**: New Verkle root committed to JAM state

### Verkle Key Structure

Ethereum's Verkle tree uses a **stem-based structure** (EIP-6800):

```
Key = stem || suffix
  - stem: 31 bytes (determines tree position)
  - suffix: 1 byte (determines leaf within stem)

Example keys for account 0x1234...5678:
  - Balance:        stem(0x1234...5678) || 0x00
  - Nonce:          stem(0x1234...5678) || 0x01
  - Code hash:      stem(0x1234...5678) || 0x02
  - Code size:      stem(0x1234...5678) || 0x03
  - Storage slot 0: stem(0x1234...5678, 0) || 0x00
  - Storage slot 1: stem(0x1234...5678, 0) || 0x01
```

### State Transition Work Package

**Payload format** (submitted by relayer):

```rust
struct EthereumStateTransition {
    /// Previous Verkle root (must match service state)
    prev_root: [u8; 32],

    /// New Verkle root (target state)
    new_root: [u8; 32],

    /// Block number
    block_number: u64,

    /// Compact list of modified keys (stem compression)
    /// Format: [num_stems: u16] + stems + [num_keys_per_stem: u8] + suffixes
    modified_keys: Vec<u8>,

    /// Witness data: Verkle proofs for all modified keys
    /// Proves old_value → new_value for each key
    verkle_witness: Vec<u8>,

    /// Bridge events (deposits/withdrawals detected in this block)
    bridge_events: Vec<BridgeEvent>,
}

struct BridgeEvent {
    /// Event type: Deposit or Withdrawal
    event_type: EventType,

    /// Whitelisted contract that emitted event
    contract: [u8; 20],

    /// Asset ID (e.g., 1337 for USDC)
    asset_id: u32,

    /// Amount (in asset's native decimals)
    amount: u128,

    /// Destination service (for deposits) or sender (for withdrawals)
    service_id: u32,

    /// User-specific data (recipient address, memo, etc.)
    user_data: [u8; 128],

    /// Event log index (for uniqueness)
    log_index: u32,
}
```

### Refine Validation Steps

The Ethereum service's refine phase validates the state transition:

```rust
// services/ethereum/src/refiner.rs

fn refine_state_transition(
    transition: EthereumStateTransition,
    current_root: [u8; 32],
) -> Result<ExecutionEffects> {
    // 1. Verify prev_root matches current state
    if transition.prev_root != current_root {
        return Err("Root mismatch: transition does not build on current state");
    }

    // 2. Deserialize modified keys
    let keys = decode_modified_keys(&transition.modified_keys)?;

    // 3. Verify Verkle witness
    // For each key: verify proof that old_value → new_value is valid
    verify_verkle_transition(
        &transition.prev_root,
        &transition.new_root,
        &keys,
        &transition.verkle_witness,
    )?;

    // 4. Validate bridge events
    let mut accumulate_instructions = Vec::new();
    for event in transition.bridge_events {
        validate_bridge_event(&event, &keys)?;

        // Convert deposits to AccumulateInstructions
        if event.event_type == EventType::Deposit {
            accumulate_instructions.push(AccumulateInstruction::Transfer {
                destination_service: event.service_id,
                amount: event.amount as u64,
                gas_limit: 200_000,
                memo: encode_foreign_asset_memo(event.asset_id, event.user_data),
            });
        }
    }

    // 5. Generate write intents
    Ok(ExecutionEffects {
        write_intents: vec![
            WriteIntent {
                key: b"verkle_root".to_vec(),
                value: transition.new_root.to_vec(),
            },
            WriteIntent {
                key: b"block_number".to_vec(),
                value: transition.block_number.to_le_bytes().to_vec(),
            },
        ],
        accumulate_instructions,
    })
}
```

### Key Validation: Modified Keys Proof

**Critical security check**: Ensure the relayer isn't omitting key updates.

```rust
fn verify_verkle_transition(
    old_root: &[u8; 32],
    new_root: &[u8; 32],
    keys: &[(VerkleKey, OldValue, NewValue)],
    witness: &[u8],
) -> Result<()> {
    // Parse witness (IPA proof in Ethereum Verkle spec)
    let proof = VerkleProof::deserialize(witness)?;

    // Verify multi-key update proof
    // Proves: old_root + updates → new_root
    proof.verify_multiproof(
        old_root,
        new_root,
        keys.iter().map(|(k, old, new)| (k, old, new)),
    )?;

    Ok(())
}
```

**Design choice**: We trust the relayer to provide all modified keys, but we **cryptographically verify** the Verkle proof. If a relayer omits keys, the proof verification fails.

### Block-by-Block vs. Epoch-by-Epoch

**Option 1: Block-by-Block Submission** (Initial implementation)
- Each work package represents 1 Ethereum block
- Pros: Simple, fine-grained
- Cons: High overhead (1 WP per 12 seconds)

**Option 2: Epoch-by-Epoch Submission** (Optimized)
- Each work package represents 32 blocks (1 Ethereum epoch)
- Pros: Reduces JAM WP overhead by 32x
- Cons: Larger witnesses (more keys modified)

**Recommendation**: Start with Option 1, migrate to Option 2 after proof-of-concept.

---

## Beacon Chain Service

### Purpose

The Beacon Chain service (Service 0) validates **finality** for Ethereum's execution layer (Service 1).

### Responsibilities

1. **Track Beacon Chain state**: Latest justified/finalized checkpoints
2. **Validate finality signals**: Ensure Ethereum blocks are finalized before accepting deposits
3. **Prevent reorgs**: Only process deposits from finalized blocks

### State Schema

```rust
// Service 0 state keys
beacon_state = {
    // Latest finalized checkpoint (epoch, root)
    "finalized_epoch": u64,
    "finalized_root": [u8; 32],

    // Latest justified checkpoint
    "justified_epoch": u64,
    "justified_root": [u8; 32],

    // Mapping: epoch → beacon block root
    "epoch_{epoch}_root": [u8; 32],

    // Mapping: execution block number → finalized (bool)
    "exec_block_{number}_finalized": bool,
}
```

### Beacon Block Work Package

```rust
struct BeaconBlockTransition {
    /// Slot number
    slot: u64,

    /// Epoch (slot / 32)
    epoch: u64,

    /// Beacon block root
    block_root: [u8; 32],

    /// Execution payload block number (links to Service 1)
    execution_block_number: u64,

    /// Execution payload block hash
    execution_block_hash: [u8; 32],

    /// Finality data
    justified_checkpoint: Checkpoint,
    finalized_checkpoint: Checkpoint,

    /// Proof: SSZ Merkle proof of finality
    finality_proof: Vec<u8>,
}

struct Checkpoint {
    epoch: u64,
    root: [u8; 32],
}
```

### Refine: Validate Finality

```rust
// services/beacon/src/refiner.rs

fn refine_beacon_block(
    beacon: BeaconBlockTransition,
    current_state: BeaconState,
) -> Result<ExecutionEffects> {
    // 1. Verify slot ordering
    if beacon.slot <= current_state.latest_slot {
        return Err("Slot already processed");
    }

    // 2. Verify finality proof (SSZ Merkle proof)
    verify_finality_ssz_proof(
        &beacon.block_root,
        &beacon.finalized_checkpoint,
        &beacon.finality_proof,
    )?;

    // 3. Check if execution block is now finalized
    let newly_finalized = beacon.finalized_checkpoint.epoch > current_state.finalized_epoch;

    let mut write_intents = vec![
        WriteIntent {
            key: b"finalized_epoch".to_vec(),
            value: beacon.finalized_checkpoint.epoch.to_le_bytes().to_vec(),
        },
        WriteIntent {
            key: b"finalized_root".to_vec(),
            value: beacon.finalized_checkpoint.root.to_vec(),
        },
    ];

    if newly_finalized {
        // Mark all execution blocks in finalized epoch as safe
        // Service 1 can now process deposits from these blocks
        write_intents.push(WriteIntent {
            key: format!("exec_block_{}_finalized", beacon.execution_block_number).into_bytes(),
            value: vec![1],
        });
    }

    Ok(ExecutionEffects {
        write_intents,
        accumulate_instructions: vec![],
    })
}
```

### Cross-Service Finality Check

Service 1 (Ethereum execution) queries Service 0 (Beacon) for finality:

```rust
// services/ethereum/src/refiner.rs

fn validate_bridge_event(
    event: &BridgeEvent,
    block_number: u64,
) -> Result<()> {
    // Query Beacon service: is this block finalized?
    let finalized = read_service_state(
        0, // Beacon service
        format!("exec_block_{}_finalized", block_number).as_bytes(),
    )?;

    if finalized != [1] {
        return Err("Cannot process deposit: block not finalized");
    }

    Ok(())
}
```

**Design choice**: Deposits are only processed from **finalized** Ethereum blocks. This prevents reorg-based double-spending.

---

## Foreign EVM Chain Services

### Multi-Chain Support

Each foreign EVM chain is represented as a separate JAM service:

| Service ID | Chain | Beacon Service | Notes |
|------------|-------|----------------|-------|
| 1 | Ethereum Mainnet | 0 | Primary chain |
| 10 | Base | - | Uses L1 finality (Ethereum) |
| 42161 | Arbitrum | - | Uses L1 finality |
| 8453 | Optimism | - | Uses L1 finality |

**L2 Finality**: L2 chains (Base, Arbitrum, etc.) inherit finality from Ethereum L1. Their services can query Service 0 for L1 finality.

### Service State Schema

```rust
// Each EVM chain service tracks:
evm_service_state = {
    // Current Verkle root
    "verkle_root": [u8; 32],

    // Latest processed block
    "block_number": u64,
    "block_hash": [u8; 32],

    // Whitelisted bridge contracts
    "whitelist_{contract_address}": bool,

    // Outgoing transfer queue (for withdrawals)
    "outgoing_queue_size": u64,
    "outgoing_queue_{index}": OutgoingTransfer,

    // Asset registry (per-chain instances of global assets)
    "asset_{asset_id}_total_locked": u128,
}
```

### Whitelisted Contracts

Only approved contracts can emit bridge events:

```rust
// Ethereum mainnet (Service 1) whitelist example
whitelist = {
    "0x1234...5678": true, // JAMBridge USDC contract
    "0xabcd...ef00": true, // JAMBridge USDT contract
    "0x9999...1111": true, // JAMBridge multi-asset contract
}
```

**Whitelist management**: Controlled via governance (see LONGTERM.md §9).

---

## Asset Registry

### Global Asset IDs

JAM State maintains a **global asset registry** for foreign assets:

```rust
// JAM State (not service-specific)
asset_registry = {
    // Asset metadata
    "asset_{id}_name": String,
    "asset_{id}_decimals": u8,
    "asset_{id}_origin_chain": u32, // Service ID
    "asset_{id}_origin_contract": [u8; 20], // ERC20 address

    // Total supply across all JAM services
    "asset_{id}_total_supply": u128,
}
```

### Registered Assets (Initial Set)

| Asset ID | Symbol | Decimals | Origin Chain | Contract Address |
|----------|--------|----------|--------------|------------------|
| 1337 | USDC | 6 | Ethereum (1) | 0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48 |
| 1984 | USDT | 6 | Ethereum (1) | 0xdAC17F958D2ee523a2206206994597C13D831ec7 |
| 2001 | WBTC | 8 | Ethereum (1) | 0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599 |
| 2002 | WETH | 18 | Ethereum (1) | 0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2 |
| 3000 | DAI | 18 | Ethereum (1) | 0x6B175474E89094C44Da98b954EedeAC495271d0F |

**Design choice**: Use fixed, well-known asset IDs (1337, 1984, etc.) to avoid collisions and enable cross-service fungibility.

### Per-Service Asset Tracking

Each JAM service tracks balances for foreign assets:

```rust
// Example: Railgun service (ID 5) tracking USDC
railgun_service_state = {
    // Foreign asset balances (not native CASH/STOCK/RSU/OPTION)
    "foreign_asset_{asset_id}_{user_pk}_balance": u128,

    // Total foreign assets held by service
    "foreign_asset_{asset_id}_total": u128,
}
```

**Invariant**: `sum(service.foreign_asset_{id}_total) <= asset_registry.asset_{id}_total_supply`

---

## Inbound Transfers (Ethereum → JAM)

### User Flow

```
┌────────────────────────────────────────────────────────────────┐
│ Step 1: User deposits USDC on Ethereum                         │
│  - Calls JAMBridge.depositToService(1337, 1000e6, 5, memo)    │
│    - asset_id: 1337 (USDC)                                     │
│    - amount: 1000 * 10^6 (USDC has 6 decimals)                │
│    - target_service: 5 (Railgun)                               │
│    - memo: user-specific data (e.g., Railgun public key)      │
│  - Contract locks USDC in escrow                               │
│  - Emits DepositInitiated(asset_id, amount, service, memo)    │
└────────────────────────────────────────────────────────────────┘
                            │
                            │ Off-chain: Wait for finality
                            ▼
┌────────────────────────────────────────────────────────────────┐
│ Step 2: Beacon service finalizes Ethereum block                │
│  - Service 0 marks exec_block_{N}_finalized = true             │
└────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌────────────────────────────────────────────────────────────────┐
│ Step 3: Relayer submits work package to Service 1              │
│  - Includes Verkle state transition for block N                │
│  - Extracts bridge event: DepositInitiated(...)                │
│  - Proves event exists via Verkle key (logs are in state)      │
└────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌────────────────────────────────────────────────────────────────┐
│ Step 4: Service 1 refine validates deposit                     │
│  - Checks block is finalized (query Service 0)                 │
│  - Verifies Verkle proof of event log                          │
│  - Validates contract is whitelisted                           │
│  - Emits AccumulateInstruction::Transfer {                     │
│      destination: 5, // Railgun service                        │
│      amount: 1000000000, // 1000 * 10^6                        │
│      asset_id: 1337, // USDC                                   │
│      memo: user_memo,                                           │
│    }                                                            │
└────────────────────────────────────────────────────────────────┘
                            │
                            │ JAM runtime creates DeferredTransfer
                            ▼
┌────────────────────────────────────────────────────────────────┐
│ Step 5: Service 5 (Railgun) accumulate receives transfer       │
│  - sender_service: 1 (Ethereum)                                │
│  - amount: 1000000000                                           │
│  - asset_id: 1337 (USDC)                                       │
│  - memo: parse to get user's Railgun public key                │
│                                                                 │
│  Actions:                                                       │
│  - Mint 1000 USDC into Railgun's internal ledger              │
│  - Create commitment for user (shielded balance)               │
│  - Update foreign_asset_1337_total += 1000000000              │
└────────────────────────────────────────────────────────────────┘
```

### Solidity Bridge Contract

```solidity
// contracts/ethereum/JAMBridge.sol

/// @title JAMBridge - Ethereum ↔ JAM Asset Bridge
/// @notice Whitelisted contract for cross-chain USDC/USDT transfers
contract JAMBridge {
    /// @notice JAM asset registry (immutable mappings)
    mapping(uint32 => address) public assetContracts; // asset_id → ERC20 address

    /// @notice Escrow balances
    mapping(uint32 => uint256) public totalLocked; // asset_id → total escrowed

    /// @notice Deposit event (monitored by JAM relayers)
    event DepositInitiated(
        address indexed sender,
        uint32 indexed assetId,
        uint256 amount,
        uint32 targetService,
        bytes memo
    );

    /// @notice Withdrawal executed (from JAM outgoing queue)
    event WithdrawalExecuted(
        address indexed recipient,
        uint32 indexed assetId,
        uint256 amount,
        uint64 jamQueueIndex
    );

    /// @notice Deposit assets to JAM service
    /// @param assetId Global asset ID (1337=USDC, 1984=USDT, ...)
    /// @param amount Amount in asset's native decimals
    /// @param targetService JAM service ID to mint into
    /// @param memo User-specific data (e.g., recipient address in target service)
    function depositToService(
        uint32 assetId,
        uint256 amount,
        uint32 targetService,
        bytes calldata memo
    ) external {
        require(amount > 0, "Amount must be positive");
        require(memo.length <= 128, "Memo too long");

        // Get ERC20 contract
        address token = assetContracts[assetId];
        require(token != address(0), "Asset not supported");

        // Transfer tokens to escrow
        IERC20(token).transferFrom(msg.sender, address(this), amount);
        totalLocked[assetId] += amount;

        // Emit event (relayers monitor this)
        emit DepositInitiated(msg.sender, assetId, amount, targetService, memo);
    }

    /// @notice Withdraw assets from JAM (called by relayer with proof)
    /// @param recipient Ethereum address to receive assets
    /// @param assetId Global asset ID
    /// @param amount Amount to withdraw
    /// @param jamQueueIndex Index in JAM outgoing queue
    /// @param jamStateRoot JAM Verkle root containing withdrawal proof
    /// @param verkleProof Verkle proof of outgoing_queue_{index}
    function withdrawFromJAM(
        address recipient,
        uint32 assetId,
        uint256 amount,
        uint64 jamQueueIndex,
        bytes32 jamStateRoot,
        bytes calldata verkleProof
    ) external {
        // Verify JAM state root (could use JAM light client or optimistic bridge)
        // For now: assume jamStateRoot is validated off-chain or via L1 contract

        // Verify Verkle proof: proves outgoing_queue_{index} exists in JAM state
        require(
            verifyVerkleProof(
                jamStateRoot,
                buildQueueKey(jamQueueIndex),
                encodeWithdrawal(recipient, assetId, amount),
                verkleProof
            ),
            "Invalid Verkle proof"
        );

        // Check not already processed
        require(!processedWithdrawals[jamQueueIndex], "Already processed");
        processedWithdrawals[jamQueueIndex] = true;

        // Release from escrow
        address token = assetContracts[assetId];
        totalLocked[assetId] -= amount;
        IERC20(token).transfer(recipient, amount);

        emit WithdrawalExecuted(recipient, assetId, amount, jamQueueIndex);
    }

    /// @dev Build JAM state key for outgoing queue entry
    function buildQueueKey(uint64 index) internal pure returns (bytes32) {
        // Key format: keccak256("outgoing_queue_{index}")
        return keccak256(abi.encodePacked("outgoing_queue_", index));
    }

    /// @dev Encode withdrawal data (must match JAM encoding)
    function encodeWithdrawal(
        address recipient,
        uint32 assetId,
        uint256 amount
    ) internal pure returns (bytes memory) {
        // Format: [recipient(20) | asset_id(4) | amount(16)]
        return abi.encodePacked(recipient, assetId, uint128(amount));
    }

    /// @dev Verify Verkle proof (IPA-based, EIP-6800 format)
    function verifyVerkleProof(
        bytes32 root,
        bytes32 key,
        bytes memory value,
        bytes calldata proof
    ) internal view returns (bool) {
        // TODO: Implement Verkle proof verification
        // Uses IPA (Inner Product Argument) commitment scheme
        // See: https://eips.ethereum.org/EIPS/eip-6800

        // Placeholder: always return true (INSECURE - for design doc only)
        return true;
    }
}
```

### Memo Format: Inbound Transfers

```
Foreign Asset Deposit Memo (v1)
Total: 128 bytes

Byte Range   | Field            | Type    | Description
-------------|------------------|---------|------------------------------------------
[0]          | version          | u8      | Memo version (0x01)
[1..5]       | asset_id         | u32     | Foreign asset ID (e.g., 1337 for USDC)
[5..25]      | origin_sender    | Address | Ethereum address that initiated deposit
[25..128]    | service_data     | [u8;103]| Service-specific data (e.g., Railgun pk)
```

Example: USDC deposit to Railgun
```rust
memo = {
    0x01,              // version
    0x39, 0x05, 0x00, 0x00,  // asset_id = 1337 (USDC, little-endian)
    0xf3, 0x9f, ...,   // origin_sender (Ethereum address, 20 bytes)
    0x11, 0x11, ...,   // Railgun public key (32 bytes)
    0x00, 0x00, ...,   // reserved (71 bytes)
}
```

### Service Accumulate: Process Inbound Transfer

```rust
// services/railgun/src/accumulator.rs

fn process_foreign_asset_deposit(
    transfer: &DeferredTransfer,
) -> Result<()> {
    // Parse memo v1
    if transfer.memo[0] != 0x01 {
        return Err("Invalid memo version");
    }

    // Extract asset_id
    let asset_id = u32::from_le_bytes([
        transfer.memo[1],
        transfer.memo[2],
        transfer.memo[3],
        transfer.memo[4],
    ]);

    // Extract origin sender (for logging/auditing)
    let origin_sender = &transfer.memo[5..25];

    // Extract service-specific data (Railgun public key)
    let mut recipient_pk = [0u8; 32];
    recipient_pk.copy_from_slice(&transfer.memo[25..57]);

    // Validate asset is registered
    let asset_decimals = read_jam_state(
        format!("asset_{}_decimals", asset_id).as_bytes()
    )?;
    if asset_decimals.is_empty() {
        return Err("Asset not registered");
    }

    // Mint into Railgun's foreign asset ledger
    // (Similar to deposit flow, but for foreign assets)
    let commitment = compute_commitment(transfer.amount, recipient_pk, rho);
    insert_commitment(commitment)?;

    // Update foreign asset total
    let current_total = read_service_state(
        format!("foreign_asset_{}_total", asset_id).as_bytes()
    )?;
    let new_total = u128::from_le_bytes(current_total) + transfer.amount as u128;
    write_service_state(
        format!("foreign_asset_{}_total", asset_id).as_bytes(),
        &new_total.to_le_bytes(),
    )?;

    log::info!(
        "✅ Minted {} of asset {} from Ethereum sender 0x{}",
        transfer.amount,
        asset_id,
        hex::encode(origin_sender)
    );

    Ok(())
}
```

---

## Outbound Transfers (JAM → Ethereum)

### User Flow

```
┌────────────────────────────────────────────────────────────────┐
│ Step 1: User withdraws USDC from JAM service (e.g., Railgun)   │
│  - Submits WithdrawForeignAsset extrinsic                      │
│    - asset_id: 1337 (USDC)                                     │
│    - amount: 500 * 10^6                                        │
│    - destination_chain: 1 (Ethereum)                           │
│    - recipient: 0xABCD...1234 (Ethereum address)               │
│  - Burns USDC from Railgun's internal ledger                   │
└────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌────────────────────────────────────────────────────────────────┐
│ Step 2: Railgun refine emits AccumulateInstruction             │
│  - destination_service: 1 (Ethereum)                           │
│  - amount: 500000000                                            │
│  - memo: withdrawal_memo_v1(recipient, asset_id)               │
└────────────────────────────────────────────────────────────────┘
                            │
                            │ JAM runtime creates DeferredTransfer
                            ▼
┌────────────────────────────────────────────────────────────────┐
│ Step 3: Ethereum service (1) accumulate receives transfer      │
│  - Parses memo: recipient=0xABCD...1234, asset_id=1337         │
│  - Adds to outgoing queue in JAM State:                        │
│      outgoing_queue_{index} = {                                │
│        recipient: 0xABCD...1234,                               │
│        asset_id: 1337,                                         │
│        amount: 500000000,                                       │
│        timestamp: current_timeslot,                            │
│      }                                                          │
│  - Increments outgoing_queue_size                              │
└────────────────────────────────────────────────────────────────┘
                            │
                            │ Off-chain: Relayer monitors queue
                            ▼
┌────────────────────────────────────────────────────────────────┐
│ Step 4: Relayer submits withdrawal to Ethereum                 │
│  - Reads outgoing_queue_{index} from JAM State                 │
│  - Generates Verkle proof of queue entry                       │
│  - Calls JAMBridge.withdrawFromJAM(                            │
│      recipient,                                                 │
│      asset_id,                                                  │
│      amount,                                                    │
│      jamQueueIndex,                                             │
│      jamStateRoot,                                              │
│      verkleProof                                                │
│    )                                                            │
└────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌────────────────────────────────────────────────────────────────┐
│ Step 5: JAMBridge verifies and releases USDC                   │
│  - Verifies Verkle proof against JAM state root                │
│  - Checks queue entry not already processed                    │
│  - Transfers USDC from escrow to recipient                     │
│  - User receives 500 USDC on Ethereum                          │
└────────────────────────────────────────────────────────────────┘
```

### Outbound Queue Schema

```rust
// Ethereum service (1) state
ethereum_service_state = {
    // Queue metadata
    "outgoing_queue_size": u64,
    "outgoing_queue_head": u64, // First unprocessed index

    // Queue entries
    "outgoing_queue_{index}": OutgoingTransfer,
}

struct OutgoingTransfer {
    /// Ethereum recipient address
    recipient: [u8; 20],

    /// Asset ID
    asset_id: u32,

    /// Amount (in asset's native decimals)
    amount: u128,

    /// Timestamp (JAM timeslot)
    timestamp: u32,

    /// Optional: User nonce (for uniqueness)
    nonce: u64,
}
```

### Service Accumulate: Enqueue Withdrawal

```rust
// services/ethereum/src/accumulator.rs

fn process_outbound_transfer(
    transfer: &DeferredTransfer,
) -> Result<()> {
    // Parse withdrawal memo
    if transfer.memo[0] != 0x01 {
        return Err("Invalid memo version");
    }

    // Extract recipient (Ethereum address)
    let mut recipient = [0u8; 20];
    recipient.copy_from_slice(&transfer.memo[1..21]);

    // Extract asset_id
    let asset_id = u32::from_le_bytes([
        transfer.memo[21],
        transfer.memo[22],
        transfer.memo[23],
        transfer.memo[24],
    ]);

    // Read current queue size
    let queue_size_bytes = read_service_state(b"outgoing_queue_size")?;
    let queue_size = u64::from_le_bytes(queue_size_bytes);

    // Append to queue
    let outgoing = OutgoingTransfer {
        recipient,
        asset_id,
        amount: transfer.amount as u128,
        timestamp: current_timeslot(),
        nonce: queue_size, // Use index as nonce
    };

    write_service_state(
        format!("outgoing_queue_{}", queue_size).as_bytes(),
        &outgoing.serialize(),
    )?;

    // Increment queue size
    write_service_state(
        b"outgoing_queue_size",
        &(queue_size + 1).to_le_bytes(),
    )?;

    log::info!(
        "✅ Enqueued withdrawal: {} of asset {} to 0x{}",
        outgoing.amount,
        asset_id,
        hex::encode(&recipient)
    );

    Ok(())
}
```

### Withdrawal Memo Format

```
Withdrawal Memo (v1)
Total: 128 bytes

Byte Range   | Field            | Type    | Description
-------------|------------------|---------|------------------------------------------
[0]          | version          | u8      | Memo version (0x01)
[1..21]      | recipient        | Address | Ethereum address to receive assets
[21..25]     | asset_id         | u32     | Foreign asset ID (e.g., 1337 for USDC)
[25..128]    | reserved         | [u8;103]| Reserved (must be zero)
```

---

## Security Model

### Trust Assumptions

1. **Verkle Proof Soundness**: Verkle proofs are cryptographically secure (IPA-based)
2. **Beacon Finality**: Ethereum's beacon chain finality is honest (⅔+ validators)
3. **Whitelist Integrity**: Whitelisted contracts are non-malicious (governance-controlled)
4. **Relayer Liveness**: At least one honest relayer submits state transitions (permissionless)

### Attack Vectors & Mitigations

| Attack | Mitigation |
|--------|----------|
| **Relayer omits bridge events** | Verkle proof must include all modified keys; omission causes proof failure |
| **Malicious contract emits fake deposit** | Only whitelisted contracts accepted; whitelist governance-controlled |
| **Ethereum reorg after deposit processed** | Only process deposits from **finalized** blocks (Service 0 validation) |
| **Double-spend withdrawal** | Ethereum contract tracks `processedWithdrawals[jamQueueIndex]` |
| **Relayer censors withdrawals** | Permissionless relaying; anyone can submit queue proofs |
| **Front-running withdrawals** | Queue is FIFO; proofs are deterministic |

### Finality Delay

**Ethereum finality**: ~12-15 minutes (2 epochs)

**User experience**:
- Deposits: User waits ~15 minutes for finality, then assets mint in JAM
- Withdrawals: User waits ~15 minutes for JAM state root to propagate to Ethereum

**Optimization**: Use optimistic bridges for faster UX (e.g., liquidity providers front-run finality).

---

## Implementation Plan

### Phase 1: Beacon Chain Service (Weeks 1-2)

**Goal**: Validate Ethereum finality in JAM

#### Tasks
- [ ] **1.1 Service Scaffold**
  - Create `services/beacon/` crate
  - Define state schema (finalized_epoch, justified_epoch, etc.)
  - Implement refine/accumulate entry points

- [ ] **1.2 SSZ Merkle Proof Verification**
  - Import SSZ library (e.g., `ssz_rs`)
  - Implement `verify_finality_ssz_proof()`
  - Test against real beacon chain data

- [ ] **1.3 Cross-Service Finality Query**
  - Add `read_service_state(service_id, key)` host function
  - Service 1 queries Service 0 for `exec_block_{N}_finalized`

- [ ] **1.4 Testing**
  - Unit tests: SSZ proof verification
  - Integration test: Process beacon blocks, mark execution blocks finalized

**Files**:
- `services/beacon/src/lib.rs`
- `services/beacon/src/refiner.rs`
- `services/beacon/src/accumulator.rs`
- `services/beacon/tests/finality_test.rs`

---

### Phase 2: Ethereum Service - Verkle Validation (Weeks 3-5)

**Goal**: Validate Ethereum state transitions via Verkle proofs

#### Tasks
- [ ] **2.1 Service Scaffold**
  - Create `services/ethereum/` crate (reuse `services/evm` structure)
  - State schema: `verkle_root`, `block_number`, `whitelist_{contract}`

- [ ] **2.2 Verkle Proof Library**
  - Import Verkle library (e.g., Rust port of `go-ethereum/trie/verkle`)
  - Implement `verify_verkle_multiproof(old_root, new_root, keys, proof)`
  - Support IPA (Inner Product Argument) verification

- [ ] **2.3 Work Package Format**
  - Define `EthereumStateTransition` struct
  - Implement `decode_modified_keys()` (stem compression)
  - Serialize/deserialize Verkle witnesses

- [ ] **2.4 Refine Logic**
  - Validate `prev_root` matches service state
  - Verify Verkle multiproof
  - Update `verkle_root` and `block_number`

- [ ] **2.5 Testing**
  - Mock Ethereum blocks with Verkle roots
  - Test multiproof verification
  - Test root mismatch rejection

**Files**:
- `services/ethereum/src/lib.rs`
- `services/ethereum/src/refiner.rs`
- `services/ethereum/src/verkle.rs` (proof verification)
- `services/ethereum/tests/verkle_test.rs`

---

### Phase 3: Bridge Event Parsing (Weeks 6-7)

**Goal**: Extract deposit/withdrawal events from Ethereum state

#### Tasks
- [ ] **3.1 Event Log Verkle Keys**
  - Map Ethereum logs to Verkle keys (EIP-6800 spec)
  - Implement `extract_logs_from_verkle_keys(keys)`

- [ ] **3.2 Whitelist Validation**
  - Check `whitelist_{contract}` in service state
  - Reject events from non-whitelisted contracts

- [ ] **3.3 DepositInitiated Event Parsing**
  - ABI-decode `DepositInitiated(address,uint32,uint256,uint32,bytes)`
  - Extract `asset_id`, `amount`, `target_service`, `memo`

- [ ] **3.4 AccumulateInstruction Emission**
  - Convert deposits to `AccumulateInstruction::Transfer`
  - Encode foreign asset memo v1

- [ ] **3.5 Testing**
  - Mock Ethereum logs in Verkle state
  - Test event extraction and filtering
  - Test AccumulateInstruction generation

**Files**:
- `services/ethereum/src/events.rs`
- `services/ethereum/src/abi.rs` (event decoding)
- `services/ethereum/tests/events_test.rs`

---

### Phase 4: Asset Registry (Week 8)

**Goal**: Global asset ID management in JAM State

#### Tasks
- [ ] **4.1 JAM State Schema**
  - Define asset registry keys in core
  - Implement `register_asset(id, name, decimals, origin_chain, contract)`

- [ ] **4.2 Governance Integration**
  - Add asset registration to governance proposals
  - Whitelist management (add/remove contracts)

- [ ] **4.3 Initial Assets**
  - Register USDC (1337), USDT (1984), WBTC (2001), WETH (2002), DAI (3000)
  - Add Ethereum mainnet whitelist contracts

- [ ] **4.4 Testing**
  - Test asset registration
  - Test cross-service asset queries

**Files**:
- `core/jam_state/asset_registry.rs` (new)
- `core/jam_state/governance.rs` (modifications)
- `tests/asset_registry_test.rs`

---

### Phase 5: Inbound Transfers (Weeks 9-10)

**Goal**: Deposits from Ethereum → JAM services

#### Tasks
- [ ] **5.1 Service Accumulate - Foreign Assets**
  - Extend Railgun/other services to handle foreign asset deposits
  - Implement `process_foreign_asset_deposit()`
  - Track `foreign_asset_{id}_total`

- [ ] **5.2 Memo Parsing**
  - Parse foreign asset memo v1
  - Extract `asset_id`, `origin_sender`, `service_data`

- [ ] **5.3 Solidity Bridge Contract**
  - Deploy `JAMBridge.sol` to Ethereum testnet
  - Implement `depositToService()`
  - Test event emission

- [ ] **5.4 End-to-End Test**
  - User deposits USDC on Ethereum
  - Relayer submits WP to Service 1
  - Service 1 emits AccumulateInstruction
  - Railgun mints USDC for user

**Files**:
- `services/railgun/src/accumulator.rs` (modifications)
- `contracts/ethereum/JAMBridge.sol` (new)
- `tests/e2e_deposit_test.rs`

---

### Phase 6: Outbound Transfers (Weeks 11-12)

**Goal**: Withdrawals from JAM → Ethereum

#### Tasks
- [ ] **6.1 Outgoing Queue**
  - Implement queue in Ethereum service accumulate
  - `process_outbound_transfer()` appends to queue

- [ ] **6.2 Withdrawal Memo**
  - Define withdrawal memo v1
  - Parse `recipient`, `asset_id`

- [ ] **6.3 Verkle Proof Generation (Off-chain)**
  - Build relayer tool to generate Verkle proofs of queue entries
  - Query JAM state root

- [ ] **6.4 Solidity Withdrawal Verification**
  - Implement `withdrawFromJAM()` in JAMBridge.sol
  - Verify Verkle proof on-chain
  - Release assets from escrow

- [ ] **6.5 End-to-End Test**
  - User withdraws USDC from Railgun
  - Service 1 enqueues withdrawal
  - Relayer submits proof to Ethereum
  - User receives USDC on Ethereum

**Files**:
- `services/ethereum/src/accumulator.rs` (modifications)
- `contracts/ethereum/JAMBridge.sol` (modifications)
- `tools/relayer/verkle_proof_generator.rs` (new)
- `tests/e2e_withdrawal_test.rs`

---

### Phase 7: Multi-Chain Support (Weeks 13-14)

**Goal**: Support Base, Arbitrum, other L2s

#### Tasks
- [ ] **7.1 Base Service (ID 10)**
  - Clone Ethereum service template
  - Configure for Base chain ID
  - Use Ethereum L1 finality (query Service 0)

- [ ] **7.2 Bridge Contracts per Chain**
  - Deploy JAMBridge.sol to Base, Arbitrum, etc.
  - Configure asset contracts (USDC on Base != USDC on Ethereum)

- [ ] **7.3 Testing**
  - Test deposit from Base → JAM
  - Test withdrawal JAM → Base

**Files**:
- `services/base/` (new, clone of ethereum)
- `contracts/base/JAMBridge.sol`

---

### Phase 8: Relayer Infrastructure (Weeks 15-16)

**Goal**: Permissionless relayer network

#### Tasks
- [ ] **8.1 Ethereum → JAM Relayer**
  - Monitor Ethereum for `DepositInitiated` events
  - Fetch Verkle proofs from Ethereum node
  - Submit WPs to JAM network

- [ ] **8.2 JAM → Ethereum Relayer**
  - Monitor JAM outgoing queue
  - Generate Verkle proofs of queue entries
  - Submit to Ethereum JAMBridge contract

- [ ] **8.3 Incentives**
  - Relayer fee model (e.g., 0.1% of bridged amount)
  - Gas cost reimbursement

- [ ] **8.4 Testing**
  - Run relayers on testnets
  - Stress test with high volume

**Files**:
- `tools/relayer/eth_to_jam_relayer.rs`
- `tools/relayer/jam_to_eth_relayer.rs`

---

## Open Questions & Design Decisions

### Q1: How to verify JAM state root in Ethereum contract?

**Options**:
1. **Optimistic bridge**: JAM validators post state roots to Ethereum; fraud proofs if invalid
2. **Light client**: Ethereum contract verifies JAM consensus (Safrole + BEEFY)
3. **Trusted relayer**: Centralized for MVP, decentralize later

**Recommendation**: Start with Option 3 (trusted relayer), migrate to Option 1 (optimistic) for mainnet.

### Q2: Verkle proof verification cost on Ethereum?

**Challenge**: IPA proofs are gas-expensive (~1M gas for multiproof)

**Mitigations**:
- Batch multiple withdrawals into single proof
- Use EIP-4844 blobs for proof data (cheaper storage)
- L2-first: Verify on Arbitrum/Base (cheaper gas)

### Q3: How to handle Ethereum hard forks?

**Example**: Verkle transition (EIP-6800) activates mid-2025

**Strategy**:
- Service 1 tracks `verkle_activated` flag
- Pre-Verkle: Use MPT (Merkle Patricia Trie) validation
- Post-Verkle: Switch to Verkle validation
- Work packages include `proof_type` field

### Q4: L2 finality vs. L1 finality?

**L2 chains** (Base, Arbitrum) have different finality models:
- Base: 7-day optimistic challenge period
- Arbitrum: Similar optimistic model

**Options**:
1. Use L1 finality (wait for L2 state root posted to Ethereum L1)
2. Use L2 "soft finality" (faster but less secure)

**Recommendation**: Option 1 for mainnet (security), Option 2 for testnet (UX).

---

## Reviewer Notes, Opinions, and Open Questions

### Architecture and Design Trade-offs

**Verkle-Only Strategy Risk Assessment**
- **Strength**: Eliminates light client complexity and leverages JAM's native Verkle infrastructure
- **Risk**: Heavy dependency on production-ready, audited Verkle multiproof verifiers before deployment
- **Mitigation needed**: Staged rollout plan (simulated proofs → audited library → on-chain verifier) with MPT-based fallback mode
- **Open question**: What are the benchmark targets (proof size, verification time per WP) that would trigger a temporary compatibility mode?

**System Boundary and Responsibility Matrix**
- **Gap identified**: Unclear which verification rules live in JAM service code vs. off-chain relayers vs. L1 contracts
- **Recommendation**: Create explicit responsibility matrix to prevent ambiguous error handling paths
- **Example boundary issues**: Malformed proofs, queue entries, witness format validation

**Cross-Service Dependencies**
- **Concern**: Service 1 dependency on Service 0 for finality creates single point of failure
- **Alternative approach**: Require quorum of independent relayers to agree on finalized checkpoints
- **Liveness question**: Should Service 1 buffer deposits in "pending" queue or halt entirely during Beacon outages?

### Proof and Witness Management

**Proof Format Stability**
- **Current assumption**: EIP-6800 IPA proofs will remain stable
- **Risk**: Coupling to specific EIP draft revision
- **Recommendation**: Version witness format with `proof_version` byte in work package payload
- **Future-proofing**: Allow rotation of proving systems without protocol changes

**Witness Construction Security**
- **Critical path**: Honest relayer assumption for key inclusion in Verkle proofs
- **Safeguard needed**: Randomized spot-checks against public RPC nodes for early warning
- **Client diversity**: Plan independent implementations (Rust + Go) to cross-check proofs before production
- **Conformance testing**: Narrow test suite mirroring `go-ethereum` vectors to reduce drift risk

**Proof Size and Performance**
- **Known issue**: IPA proof verification costs ~1M gas, expensive for single transactions
- **Mitigation strategies need detail**:
  - Specific batching thresholds and trigger conditions
  - Cost-benefit analysis of L2-first verification
  - EIP-4844 blob integration timeline and implementation
- **Hybrid approach**: Batch until proof size threshold hit rather than hard epoch/block switch

### Economic and Incentive Design

**Relayer Economic Model**
- **Current gap**: Phase 8 mentions relayer fees but lacks implementation detail
- **Missing components**:
  - MEV protection mechanisms
  - Handling of failed submissions and gas reimbursement
  - Anti-censorship incentives (fee rebates for competing relayers)
  - Dynamic fee schedules to prevent fee gouging
- **Fee model questions**: Who pays for Verkle witnesses - governance subsidy vs. bridge contract levy?

**Asset Registry Economics**
- **Scalability concern**: Global registry approach may not scale to hundreds of tokens across dozens of chains
- **Governance pathway**: How whitelist evolves (on-chain vote, multisig, upgrade slot) needs specification
- **Risk containment**: Per-chain limits (asset, notional, daily velocity) to contain contract upgrade fallout

**Queue Economics**
- **Congestion vector**: Outgoing queue could become bottleneck during high load
- **Missing mechanisms**: Explicit fee markets, priority rules, queue depth caps per asset/service
- **Withdrawal economics**: Need dynamic pricing that encourages batchable withdrawals

### Operational and Safety Considerations

**Failure Recovery and Circuit Breakers**
- **Missing operational plans**: Rollback procedures, work package replays, proof re-fetch strategies
- **Circuit breaker needs**:
  - High-value transfer limits
  - Governance emergency procedures for bridge pausing/upgrading
  - Service pause mechanisms if repeated invalid transitions detected
- **Recovery artifacts**: Where to persist replayable data (witnesses, decoded events) for incident response

**Monitoring and Observability**
- **Gap**: Specific monitoring and alerting requirements for production deployment
- **Telemetry needs**: Metrics/logs for proof verification failures before validator penalties
- **Queue observability**: RPC methods for queue length, oldest index to help wallets diagnose issues
- **Performance monitoring**: Expected hardware/network costs for validators verifying large witnesses

**Fault Containment**
- **Current risk**: Malformed bridge event could poison entire state transition
- **Isolation strategy**: Separate bridge validation into optional instruction set
- **Asset freeze capability**: Support "frozen" asset states for compromised bridge contracts

### Multi-Chain and Interoperability

**L2 Finality Complexity**
- **Design tension**: Using L1 finality for L2s creates 7-day withdrawal delays for Optimistic Rollups
- **Strategy needed**: Clear approach for different finality models per chain
- **Event deduplication**: Are log indices sufficient across reorg scenarios, or need block-hash scoped identifiers?

**Non-EVM Chain Support**
- **Future-proofing**: Reserve type byte in memo format to differentiate EVM/Non-EVM payloads
- **Registry growth**: How to extend without retooling memo parsers

**Asset Decimal Handling**
- **Risk**: Mismatched decimals between ERC-20 tokens and JAM registry could cause rounding loss
- **Example**: WBTC's 8 decimals vs. USDC's 6 decimals
- **Enforcement needed**: Clear rules for decimal validation during mint/burn cycles

### Technical Implementation Concerns

**Throughput vs. Determinism Trade-off**
- **Block-by-block**: Predictable latency but high bandwidth overhead
- **Epoch-level batching**: Better overhead but complicated partial failure recovery
- **Open question**: How to re-drive failed epoch submission without replaying accepted blocks?

**State Growth Management**
- **Monotonic growth**: Outgoing queues and per-chain registries will grow indefinitely
- **Missing**: Explicit pruning/compaction expectations and Verkle key stability interaction
- **Queue replay protection**: Need time-bounded validity or replay windows for outgoing entries

**Asset ID Lifecycle**
- **Gap**: No retirement or slashing flows if bridge contract compromised
- **Registry management**: Asset ID allocation strategy (single root service vs. child service subranges)
- **Audit mechanisms**: How to prevent two services minting same asset ID with divergent assumptions

### Security and Trust Model

**Relayer Trust Surface**
- **Current model**: Multiproof verification guards against omitted keys, but relayer shapes which blocks get posted
- **Missing**: Clear liveness assumptions and anti-censorship mechanisms
- **Slashing concerns**: No conditions for malicious relayer punishment

**Cross-Service Governance**
- **Authority question**: Who updates whitelists for L2 services inheriting L1 finality?
- **Consistency risk**: How to avoid whitelist desyncs across services
- **Governance attestation**: Should Service 0 attest to governance results?

### Testing and Validation

**Test Realism Gap**
- **Current plan**: Heavy reliance on mocked transitions
- **Need**: Test vectors from actual Ethereum Verkle testnets or Geth devnets
- **Risk**: Serialization drift without real-world validation

**Witness Size Limits**
- **Performance question**: Maximum acceptable witness size before validator degradation
- **Defense mechanism**: How service rejects oversized submissions

### Recovery and Emergency Procedures

**Beacon Dependency Recovery**
- **Scenario**: Service 0 stalls during stress events
- **Options**: Force-pause deposits while allowing withdrawals vs. complete halt
- **Operator runbook**: Define procedures for mainnet stress events

**Cryptography Bug Response**
- **Scenario**: Verkle verifier bug discovered post-deployment
- **Circuit breaker**: Governance-controlled pause of inbound deposits
- **Upgrade path**: How to preserve safety without halting all JAM services

**Queue and Asset Recovery**
- **Stuck withdrawals**: Gossip strategy for external verifiers to detect issues
- **Asset freeze**: Procedures for compromised bridge contracts
- **Emergency governance**: Fast-track procedures for critical security issues

---

## References

- **EIP-6800**: Ethereum Verkle Tree (https://eips.ethereum.org/EIPS/eip-6800)
- **EIP-4844**: Proto-Danksharding (blob storage)
- **Beacon Chain Spec**: Ethereum consensus layer (https://github.com/ethereum/consensus-specs)
- **JAM Graypaper §13.3**: Inter-service transfers
- **TRANSFERS.md**: Cross-service transfer memo formats

---

## Appendix: Example Work Package

### Ethereum State Transition (Block 19000000 → 19000001)

```json
{
  "service_id": 1,
  "work_item": {
    "prev_root": "0x3a5f...",
    "new_root": "0x7b2e...",
    "block_number": 19000001,
    "modified_keys": [
      {
        "stem": "0x1234...",
        "suffixes": [0, 1, 2],
        "old_values": ["0x00...", "0x05", "0xabcd..."],
        "new_values": ["0x1000...", "0x06", "0xabcd..."]
      }
    ],
    "verkle_witness": "0x<IPA_PROOF_BYTES>",
    "bridge_events": [
      {
        "event_type": "Deposit",
        "contract": "0x1234567890abcdef...",
        "asset_id": 1337,
        "amount": 1000000000,
        "service_id": 5,
        "user_data": "0x1111...",
        "log_index": 42
      }
    ]
  }
}
```

---

**End of Document**
