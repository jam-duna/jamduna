# Rollup Architecture Refactor

## ✅ COMPLETED

This refactoring has been successfully implemented. See below for details of what was done.

---

## Problem Statement

The original `Rollup` struct mixed two concerns:
1. **Testing infrastructure** (stateDB, previousGuarantees, pvmBackend) - needed for standalone tests
2. **Query interface** (serviceID, storage access) - needed for production node usage

This created a conflict:
- In `statedb/rollup_test.go`: Tests needed a full standalone state machine with its own StateDB
- In `node/node_test.go`: Nodes already had StateDBs and storage; rollups should just query them

When nodes tried to initialize rollup genesis, `r.stateDB.sdb` was nil because rollups didn't create their own StateDB.

## Solution: Split into Two Structures (✅ Implemented)

### 1. RollupNode (Testing/Standalone)
**File**: `statedb/rollupnode.go` (lines 1-700 of current rollup.go)

**Purpose**: Self-contained rollup state machine for testing

**Fields**:
```go
type RollupNode struct {
    serviceID          uint32
    stateDB            *StateDB          // Owns its state
    storage            types.JAMStorage  // Direct storage access
    previousGuarantees map[common.Hash]*types.Guarantee
    pvmBackend         string
}
```

**Methods** (block processing, state transitions):
- `NewRollupNode(jamStorage types.JAMStorage, serviceID uint32) (*RollupNode, error)`
- `SubmitEVMGenesis(startBalance int64) error`
- `processWorkPackageBundles(bundles []*types.WorkPackageBundle) error`
- `processWorkPackageBundlesPipelined(bundles []*types.WorkPackageBundle) error`
- `setupValidators() ([]types.Validator, []types.ValidatorSecret, common.Hash, error)`
- `executeAndGuarantee(...) (...)`
- `createGuaranteesBlock(...) (*types.Block, error)`
- `createAssurancesBlock(...) (*types.Block, error)`
- All block/state processing methods (lines 189-700)

**Usage**: `statedb/rollup_test.go` - `TestEVMBlocksTransfers`, `TestEVMGenesis`, etc.

### 2. Rollup (Production/Query Interface)
**File**: `statedb/rollup.go` (lines 700+ of current rollup.go)

**Purpose**: Service-scoped query interface for nodes

**Fields**:
```go
type Rollup struct {
    serviceID uint32
    storage   types.JAMStorage  // For direct storage queries
    node      StateProvider     // For StateDB queries
}

// StateProvider interface - implemented by NodeContent
type StateProvider interface {
    GetStateDB() *StateDB
}
```

**Methods** (RPC queries, state reads):
- `NewRollup(jamStorage types.JAMStorage, serviceID uint32, node StateProvider) (*Rollup, error)`
- `GetBalance(address common.Address, blockNumber string) (*big.Int, error)`
- `GetTransactionCount(address common.Address, blockNumber string) (uint64, error)`
- `GetCode(address common.Address, blockNumber string) ([]byte, error)`
- `GetStorageAt(address common.Address, position common.Hash, blockNumber string) (common.Hash, error)`
- `Call(msg *types.CallMsg, blockNumber string) ([]byte, error)`
- `EstimateGas(msg *types.CallMsg, blockNumber string) (uint64, error)`
- `GetBlockByNumber(blockNumber string, fullTx bool) (map[string]interface{}, error)`
- `GetTransactionByHash(txHash common.Hash) (map[string]interface{}, error)`
- `GetTransactionReceipt(txHash common.Hash) (map[string]interface{}, error)`
- `GetLogs(filter *types.FilterQuery) ([]*types.Log, error)`
- `GetVerkleRoot() common.Hash`
- All RPC/query methods (current lines 700+)

**StateDB Access Pattern**:
```go
func (r *Rollup) GetStateDB() *StateDB {
    if r.node != nil {
        return r.node.GetStateDB()
    }
    return nil
}
```

## Implementation Summary (✅ Completed)

### ✅ Phase 1: Create RollupNode
**Status**: Complete

**Created**: `statedb/rollupnode.go` with:
- RollupNode struct owning its own StateDB
- All block processing methods
- Test helper methods (createTransferTriplesForRound, DeployContract, etc.)
- Simplified SubmitEVMGenesis using MakeGenesisStateTransition

### ✅ Phase 2: Refactor Rollup
**Status**: Complete

**Changes to** `statedb/rollup.go`:
- Removed `stateDB`, `previousGuarantees`, `pvmBackend` fields
- Added `node StateProvider` field
- Added `StateProvider` interface
- Updated all methods to use `r.GetStateDB()` instead of `r.stateDB`
- Updated `NewRollup` signature to accept `StateProvider` parameter

### ✅ Phase 3: Update Tests
**Status**: Complete

1. **statedb/rollup_test.go**:
   ```go
   func TestEVMBlocksTransfers(t *testing.T) {
       storage, err := initStorage(t.TempDir())
       if err != nil {
           t.Fatalf("initStorage failed: %v", err)
       }
       chain, err := NewRollupNode(storage, EVMServiceCode)
       if err != nil {
           t.Fatalf("NewRollupNode failed: %v", err)
       }
       err = chain.SubmitEVMGenesis(61_000_000)
       if err != nil {
           t.Fatalf("SubmitEVMGenesis failed: %v", err)
       }
       // Test uses RollupNode for all operations
   }
   ```
   - All tests updated to use `NewRollupNode`
   - Function signatures updated to accept `*RollupNode`
   - Test passes: `TestEVMBlocksTransfers` ✅

### ✅ Phase 4: Update Node Integration
**Status**: Complete

1. **node/node.go - NodeContent** implements `StateProvider`:
   ```go
   func (n *NodeContent) GetStateDB() *StateDB {
       n.statedbMutex.Lock()
       defer n.statedbMutex.Unlock()
       return n.statedb
   }
   ```
   - Located at [node/node.go:263-268](node/node.go#L263-L268)

2. **node/node.go - Rollup creation**:
   ```go
   func (n *NodeContent) GetOrCreateRollup(serviceID uint32) (*statedb.Rollup, error) {
       n.rollupsMutex.RLock()
       rollup, exists := n.rollups[serviceID]
       n.rollupsMutex.RUnlock()

       if exists {
           return rollup, nil
       }

       n.rollupsMutex.Lock()
       defer n.rollupsMutex.Unlock()

       if rollup, exists := n.rollups[serviceID]; exists {
           return rollup, nil
       }

       // Create rollup with node reference - PASSES NODE AS StateProvider
       newRollup, err := statedb.NewRollup(n.store, serviceID, n)
       if err != nil {
           return nil, fmt.Errorf("failed to create rollup for service %d: %w", serviceID, err)
       }

       n.rollups[serviceID] = newRollup
       log.Info(log.Node, "Created new rollup instance", "serviceID", serviceID)
       return newRollup, nil
   }
   ```
   - Located at [node/node.go:234-261](node/node.go#L234-L261)
   - **Key change**: Passes `n` (NodeContent) as the third parameter to `NewRollup`

3. **Genesis initialization removed from node startup**:
   - Previously incorrect code that called `rollup.SubmitEVMGenesis()` in `newNode()` has been removed
   - Genesis is part of JAM state (via `MakeGenesisStateTransition`), not rollup-specific initialization

### ✅ Phase 5: RPC Handler
**Status**: Unchanged (existing code already works)
The RPC handler already works correctly with the new architecture - it calls `GetOrCreateRollup()` which now properly passes the node as StateProvider.

## Key Design Principles

### 1. Separation of Concerns
- **RollupNode**: Owns state, processes blocks, standalone testing
- **Rollup**: Queries state, RPC interface, production usage

### 2. StateDB Ownership
- **RollupNode**: Creates and owns its StateDB (for tests)
- **Rollup**: Borrows StateDB from node (for production)
- **Node**: Owns the canonical JAM StateDB

### 3. Genesis Handling
- **RollupNode.SubmitEVMGenesis()**: Initializes test state
- **Production nodes**: Genesis is part of JAM chain state, not rollup-specific
- **Verkle tree**: Lives in storage, shared across the node

### 4. Interface Design
```go
// Simple interface for Rollup to access node state
type StateProvider interface {
    GetStateDB() *StateDB
}

// NodeContent implements this naturally with thread-safe access
func (n *NodeContent) GetStateDB() *StateDB {
    n.statedbMutex.Lock()
    defer n.statedbMutex.Unlock()
    return n.statedb
}
```

## File Structure After Refactor (✅ Implemented)

```
statedb/
├── rollupnode.go          # RollupNode struct + block processing (~1000 lines)
│                          # - SubmitEVMGenesis (uses MakeGenesisStateTransition)
│                          # - processWorkPackageBundles, SubmitEVMTransactions
│                          # - Test helpers: createTransferTriplesForRound, DeployContract
│                          # - Query methods: GetEVMBlockByNumber, GetLatestBlockNumber
├── rollup.go              # Rollup struct + RPC queries (~1000 lines)
│                          # - StateProvider interface
│                          # - GetStateDB() delegates to node
│                          # - All RPC/query methods
├── rollup_test.go         # Tests using RollupNode ✅
│                          # - TestEVMBlocksTransfers (PASSING)
└── genesis.go             # MakeGenesisStateTransition (used by RollupNode)

node/
├── node.go                # NodeContent implements StateProvider ✅
│                          # - GetStateDB() at line 263-268
│                          # - GetOrCreateRollup() passes 'n' to NewRollup
├── node_rpc.go            # Uses Rollup for queries ✅
└── node_test.go           # Integration tests
```

## Benefits (✅ Achieved)

1. ✅ **Clear ownership**: RollupNode owns state (tests), Rollup queries state (production)
2. ✅ **No nil pointers**: Each struct has what it needs, no optional fields
3. ✅ **Testability**: RollupNode tests standalone, Rollup gets StateDB from node
4. ✅ **Production safety**: Nodes don't create duplicate StateDBs
5. ✅ **RPC efficiency**: Rollup provides clean query interface via StateProvider
6. ✅ **Genesis clarity**: Uses MakeGenesisStateTransition, not custom logic
7. ✅ **Code reuse**: Leveraged existing genesis infrastructure instead of reinventing

## Testing Results (✅ Passing)

### ✅ RollupNode Tests
```bash
$ go test -run TestEVMBlocksTransfers ./statedb
--- PASS: TestEVMBlocksTransfers (16.06s)
PASS
ok      github.com/colorfulnotion/jam/statedb   (cached)
```

### ✅ Build Verification
```bash
$ go build ./statedb/... ./node/...
# Success - both packages build without errors
```

## Resolved Design Questions

1. **Genesis in production**: ✅ **Resolved**
   - Genesis is part of JAM state via `MakeGenesisStateTransition`
   - RollupNode.SubmitEVMGenesis uses this + adds EVM issuer balance
   - Production nodes get genesis from chain state, not rollup-specific init

2. **Verkle tree ownership**: ✅ **Resolved**
   - Storage layer (StateDBStorage) owns and manages Verkle trees
   - Shared by all components via storage interface

3. **RollupNode vs Rollup naming**: ✅ **Kept as-is**
   - Clear distinction: RollupNode = owns state, Rollup = queries state
   - Naming is intuitive and matches usage patterns

4. **StateProvider location**: ✅ **Resolved**
   - Defined in statedb package (rollup.go)
   - Simple interface, minimal dependencies
   - NodeContent implements it naturally

---

# Multi-Rollup Architecture

## Overview

JAM nodes run multiple rollups simultaneously, where each rollup corresponds to a specific service. Each rollup maintains its own EVM state, block history, and RPC endpoints.

## Multi-Service Architecture

### Service Isolation
- Each service has its own `Rollup` instance
- Independent EVM state (verkle trees) per service
- Independent block numbering per service
- Shared JAM consensus layer

### Port Allocation
Services get dedicated RPC endpoints:
```
Service ID | HTTP Port | WS Port
-----------|-----------|----------
0          | 9000      | 9001
1          | 9002      | 9003
2          | 9004      | 9005
N          | 9000+2N   | 9000+2N+1
```

## Storage Architecture

### Service-Scoped State Model
```
serviceID → blockNumber → (verkleRoot, jamStateRoot, blockData)
                        ↓
                   VerkleNode
```

### Storage Interface Extensions
The `JAMStorage` interface was extended with service-scoped methods:

```go
type JAMStorage interface {
    // Service-scoped verkle tree access
    GetVerkleNodeForServiceBlock(serviceID uint32, blockNumber string) (verkle.VerkleNode, bool)

    // Store post-accumulation state for a service
    StoreServiceBlock(serviceID uint32, block *EvmBlockPayload, jamStateRoot common.Hash, jamSlot uint32) error

    // Retrieve service block data
    GetServiceBlock(serviceID uint32, blockNumber string) (*EvmBlockPayload, error)

    // Finalize EVM block after accumulation
    FinalizeEVMBlock(serviceID uint32, block *EvmBlockPayload, jamStateRoot common.Hash, jamSlot uint32) error
}
```

### Key Storage Maps
```go
type StateDBStorage struct {
    // Service-scoped verkle roots
    // Key: "vr_{serviceID}_{blockNumber}" → verkleRoot
    verkleRoots map[string]common.Hash

    // Latest rollup block number per service
    latestRollupBlock map[uint32]uint64
}
```

## Rollup Service Scoping

Each `Rollup` instance is bound to a specific service:

```go
type Rollup struct {
    serviceID uint32           // Service this rollup belongs to
    storage   types.JAMStorage // Service-scoped storage
    node      StateProvider    // Access to node's StateDB
}
```

All RPC methods use service-scoped lookups:
```go
func (r *Rollup) GetBalance(address common.Address, blockNumber string) (common.Hash, error) {
    // Gets verkle tree for THIS service + block number
    tree, ok := r.storage.GetVerkleNodeForServiceBlock(r.serviceID, blockNumber)
    if !ok {
        return common.Hash{}, fmt.Errorf("verkle tree not found for service %d block %s",
            r.serviceID, blockNumber)
    }
    return r.storage.GetBalance(tree, address)
}
```

## Multi-Rollup Node Integration

### Node Structure
```go
type NodeContent struct {
    rollups      map[uint32]*statedb.Rollup
    rollupsMutex sync.RWMutex
}

func (n *NodeContent) GetOrCreateRollup(serviceID uint32) (*statedb.Rollup, error) {
    // Check if rollup exists
    n.rollupsMutex.RLock()
    rollup, exists := n.rollups[serviceID]
    n.rollupsMutex.RUnlock()

    if exists {
        return rollup, nil
    }

    // Create new rollup for this service
    n.rollupsMutex.Lock()
    defer n.rollupsMutex.Unlock()

    newRollup, err := statedb.NewRollup(n.store, serviceID, n)
    if err != nil {
        return nil, err
    }

    n.rollups[serviceID] = newRollup
    return newRollup, nil
}
```

## Accumulate Integration

After work package accumulation, EVM blocks must be finalized:

```go
// After accumulation completes on ALL nodes
func finalizeServiceBlock(nodes []*Node, serviceID uint32, block *EvmBlockPayload,
                          jamStateRoot common.Hash, jamSlot uint32) error {
    // Propagate block to all nodes
    for _, node := range nodes {
        if err := node.storage.FinalizeEVMBlock(serviceID, block, jamStateRoot, jamSlot); err != nil {
            return err
        }
    }
    return nil
}
```

This stores:
- Verkle tree snapshot at block's root
- Service block metadata
- Link to JAM state root

## Implementation Status

### ✅ Completed Features

1. **Service-Scoped Storage**
   - `GetVerkleNodeForServiceBlock` - retrieves verkle tree for specific service+block
   - `StoreServiceBlock` - stores EVM block with service ID scoping
   - `FinalizeEVMBlock` - finalizes block after accumulation
   - Block number tracking per service

2. **Service-Aware Rollup**
   - Constructor accepts `serviceID`
   - All RPC methods use service-scoped lookups
   - Genesis creates block 0 per service

3. **Multi-Rollup Node**
   - `rollups map[uint32]*Rollup` tracks all service rollups
   - `GetOrCreateRollup` manages rollup lifecycle
   - Each service gets dedicated RPC endpoints

4. **Testing**
   - `TestEVMBlocksTransfers` passes with multi-rollup infrastructure
   - Genesis properly initializes service accounts
   - Balance conservation verified across services

### ⏳ In Progress

1. **Multi-Service RPC Routing**
   - Port-based routing to specific rollup instances
   - Service discovery endpoint

2. **Cross-Node Block Propagation**
   - Ensure `FinalizeEVMBlock` called on all nodes after accumulation
   - Not just the accumulating node

## Open Design Questions

1. **Service Discovery**: How do clients discover available services and ports?
   - Proposed: `/services` endpoint on main JAM RPC

2. **State Pruning**: Retention policy for old service blocks?
   - Proposed: Configurable per-service (keep last N blocks)

3. **Genesis Per Service**: How does each service initialize?
   - Current: Each service gets genesis block (block 0) with initial verkle tree

4. **Reorg Handling**: Service blocks and JAM reorgs?
   - Current: Service blocks final when JAM block is final (no service-level reorgs)
