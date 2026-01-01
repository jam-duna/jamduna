# JAM Builder Architecture

**Last Updated**: 2025-12-29

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Independent Service RPC Servers](#independent-service-rpc-servers)
3. [Package Responsibilities](#package-responsibilities)
4. [Service Handler Pattern](#service-handler-pattern)
5. [Testing Strategy](#testing-strategy)

---

## Architecture Overview

The JAM architecture uses **independent RPC servers** for each service. No routing, no dispatching - each service runs its own HTTP server on its own port.

```
┌───────────────────────────────────────────────────────────────┐
│                    JAM Network (6 nodes)                       │
│                   Validators/Core Protocol                     │
│                                                                │
│  • Consensus (Safrole, BABE, GRANDPA)                         │
│  • Block production & finalization                            │
│  • Work package/report processing                             │
│  • P2P networking                                             │
│                                                                │
│  RPC Port: 8540 (JAM core methods only)                       │
└───────────────────────────────────────────────────────────────┘

                                │
                                │
                ┌───────────────┴───────────────┐
                │                               │
                ▼                               ▼

┌─────────────────────────────┐   ┌─────────────────────────────┐
│    EVM Builder              │   │    Orchard Builder          │
│                             │   │                             │
│  • EVM transaction pool     │   │  • Orchard note pool        │
│  • Bundle construction      │   │  • Bundle construction      │
│  • Verkle witness gen       │   │  • Halo2 proof gen          │
│                             │   │                             │
│  RPC Port: 8545             │   │  RPC Port: 8232             │
│  Methods: eth_*             │   │  Methods: z_* (Zcash-compatible)               │
│           jam_txPool*       │   │           getblock*         │
└─────────────────────────────┘   └─────────────────────────────┘
        ▲                                   ▲
        │                                   │
        │                                   │
    Users submit                        Users submit
    EVM transactions                    Orchard transactions
    via eth_sendRawTransaction          via z_sendmany
```

### Flow

1. **Users → Service RPC Servers**: Users submit transactions directly to the service endpoint
   - EVM users → `http://localhost:8545` (eth_sendRawTransaction, eth_call, etc.)
   - Orchard users → `http://localhost:8232` (z_sendmany, z_getbalance, etc.)

2. **Builders → JAM Network**: Builders package transactions into work packages and submit to validators
   - Builders call JAM core RPC methods on validator nodes
   - Work packages include service-specific witnesses/proofs

3. **No Routing**: Each service RPC server is completely independent
   - No prefix-based routing (no `if method.startsWith("eth_")...`)
   - No method dispatching from node package
   - Clean separation: EVM server knows nothing about Orchard, and vice versa

---

## Independent Service RPC Servers

### EVM RPC Server

**File**: [builder/evm/rpc/server.go](../../builder/evm/rpc/server.go)

**Port**: 8545 (default, configurable via `--rpc-port`)

**Methods**:
- `eth_chainId`, `eth_accounts`, `eth_gasPrice`
- `eth_getBalance`, `eth_getStorageAt`, `eth_getTransactionCount`, `eth_getCode`
- `eth_call`, `eth_estimateGas`, `eth_sendRawTransaction`
- `eth_getTransactionReceipt`, `eth_getTransactionByHash`
- `eth_getBlockByNumber`, `eth_getBlockByHash`, `eth_blockNumber`
- `eth_getLogs`
- `jam_txPoolStatus`, `jam_txPoolContent`, `jam_txPoolInspect`

**Starting the server**:
```go
// jam.go
handler, _ := n.GetServiceHandler(serviceID)
evmHandler := handler.(*evmrpc.EVMServiceHandler)
rpcHandler := evmrpc.NewEVMRPCHandler(evmHandler)
server := evmrpc.NewEVMHTTPServer(rpcHandler)
server.Start(8545)
```

**Example client usage**:
```bash
# EVM RPC requests go directly to port 8545
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'

curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266","latest"],"id":1}'
```

### Orchard RPC Server

**File**: [builder/orchard/rpc/server.go](../../builder/orchard/rpc/server.go)

**Port**: 8232 (default, configurable via `--rpc-port + 1`)

**Methods**:
- `getblockchaininfo`, `getbestblockhash`, `getblockcount`, `getblockhash`, `getblock`, `getblockheader`
- `z_getblockchaininfo`, `z_gettreestate`, `z_getsubtreesbyindex`, `z_getnotescount`
- `z_getnewaddress`, `z_listaddresses`, `z_validateaddress`
- `z_getbalance`, `z_listunspent`, `z_listreceivedbyaddress`, `z_listnotes`
- `z_sendmany`, `z_sendmanywithchangeto`, `z_viewtransaction`

**Starting the server**:
```go
// jam.go
handler, _ := n.GetServiceHandler(serviceID)
orchardHandler := handler.(*orchardrpc.OrchardServiceHandler)
rpcHandler := orchardrpc.NewOrchardRPCHandler(orchardHandler)
server := orchardrpc.NewOrchardHTTPServer(rpcHandler)
server.Start(8232)
```

**Example client usage**:
```bash
# Orchard RPC requests go directly to port 8232
curl -X POST http://localhost:8232 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"1.0","method":"z_getblockchaininfo","params":[],"id":1}'

curl -X POST http://localhost:8232 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"1.0","method":"z_getbalance","params":["u1abc...",1],"id":1}'
```

### JAM Core RPC Server

**File**: [node/node_rpc.go](../../node/node_rpc.go)

**Port**: 8540 (default)

**Methods**: JAM core protocol only
- `Jam.FetchState129`, `Jam.VerifyState129`
- `Jam.Block`, `Jam.BestBlock`, `Jam.FinalizedBlock`
- `Jam.StateRoot`, `Jam.BeefyRoot`
- `Jam.GetRefineContext`
- `Jam.ServiceInfo`, `Jam.WorkPackage`, `Jam.AuditWorkPackage`

**No service routing**: The routing logic has been removed. callJamMethod() only handles JAM core methods.

---

## Package Responsibilities

### node/ - JAM Protocol Core

**Purpose**: Core JAM blockchain protocol - consensus, networking, block processing.

**Key Files**:
- [node.go](../../node/node.go) - Core node logic, consensus, block processing
- [node_rpc.go](../../node/node_rpc.go) - JAM core RPC server (NO service routing)
- [client.go](../../node/client.go) - Node client for inter-node communication

**Responsibilities**:
- ✅ Consensus (Safrole, BABE, GRANDPA)
- ✅ Block production and finalization
- ✅ Network protocol and P2P communication
- ✅ JAM core RPC methods (Jam.FetchState129, etc.)
- ✅ Work package and work report processing
- ✅ Service handler lifecycle management

**What node/ does NOT do**:
- ❌ Service RPC methods (eth_*, z_* (Zcash-compatible), etc.) - each service has its own server
- ❌ RPC routing/dispatching - no `if method.startsWith("eth_")...`
- ❌ EVM-specific types (transactions, receipts)
- ❌ Orchard-specific types (notes, commitments)

### statedb/ - State Management

**Purpose**: Generic state storage and service account types.

**Key Files**:
- [state.go](../../statedb/state.go) - Generic state management
- [service.go](../../statedb/service.go) - Service account structures
- [evmtypes/](../../statedb/evmtypes/) - EVM type definitions
- [orchardtypes/](../../statedb/orchardtypes/) - Orchard type definitions

**Responsibilities**:
- ✅ Service account types (balance, nonce, code, storage)
- ✅ Block payload structures
- ✅ Storage interface (JAMStorage)
- ✅ Service code constants (EVMServiceCode = 0, OrchardServiceCode = 1)

**Anti-patterns**:
- ❌ Service business logic (transaction execution, proof verification)
- ❌ RPC methods
- ❌ Dependencies on builder/ packages

### builder/ - Service Implementations

**Purpose**: Service-specific implementations with their own state management, RPC servers, and witness generation.

#### builder/evm/

**Structure**:
```
evm/
├── verkle/              # EVM Verkle tree state
├── witness/             # EVM witness generation
├── rpc/                 # EVM RPC layer
│   ├── server.go        # EVMHTTPServer - independent HTTP server
│   ├── handler.go       # EVMRPCHandler - eth_* RPC methods
│   ├── service_handler.go # EVMServiceHandler
│   ├── rollup.go        # EVM rollup state management
│   ├── txpool.go        # Transaction pool
│   └── utils.go         # Utilities
└── register.go          # Service factory registration
```

**RPC Server**: Runs on port 8545, serves all eth_* and jam_txPool* methods

#### builder/orchard/

**Structure**:
```
orchard/
├── orchard/              # Orchard Merkle tree state
├── witness/             # Orchard witness + Halo2 proof generation
├── rpc/                 # Orchard RPC layer
│   ├── server.go        # OrchardHTTPServer - independent HTTP server
│   ├── handler.go       # OrchardRPCHandler - z_* (Zcash-compatible) RPC methods
│   ├── service_handler.go # OrchardServiceHandler
│   └── rollup.go        # Orchard rollup state management
└── register.go          # Service factory registration
```

**RPC Server**: Runs on port 8232, serves all z_* (Zcash-compatible) and getblock* methods

---

## Testing Strategy

### Test Organization

```
node/
└── node_test.go         # JAM core tests only

builder/evm/
├── rpc/
│   ├── rollup_test.go       # EVM rollup unit tests
│   └── txpool_test.go       # TxPool unit tests
└── integration_test.go      # EVM integration tests (to be created)

builder/orchard/
└── integration_test.go      # Orchard integration tests (to be created)

types/
└── service_handler_test.go  # ServiceHandler tests (to be created)
```

### EVM Tests

```go
// builder/evm/integration_test.go
func TestEVMHTTPServer(t *testing.T) {
    // Start EVM RPC server
    storage := initStorage(t.TempDir())
    handler, _ := evmrpc.NewEVMServiceHandler(storage, statedb.EVMServiceCode, nil)
    rpcHandler := evmrpc.NewEVMRPCHandler(handler)
    server := evmrpc.NewEVMHTTPServer(rpcHandler)
    server.Start(8545)

    // Test eth_chainId
    resp := httpPost("http://localhost:8545", `{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}`)
    require.Contains(t, resp, "0x")

    // Test eth_getBalance
    resp = httpPost("http://localhost:8545", `{"jsonrpc":"2.0","method":"eth_getBalance","params":["0xf39Fd6...","latest"],"id":1}`)
    require.Contains(t, resp, "result")
}
```

### Orchard Tests

```go
// builder/orchard/integration_test.go
func TestOrchardHTTPServer(t *testing.T) {
    storage := initStorage(t.TempDir())
    handler, _ := orchardrpc.NewOrchardServiceHandler(storage, statedb.OrchardServiceCode, nil)
    rpcHandler := orchardrpc.NewOrchardRPCHandler(handler)
    server := orchardrpc.NewOrchardHTTPServer(rpcHandler)
    server.Start(8232)

    // Test z_getblockchaininfo
    resp := httpPost("http://localhost:8232", `{"jsonrpc":"1.0","method":"z_getblockchaininfo","params":[],"id":1}`)
    require.Contains(t, resp, "orchard")
}
```

### Running Tests

```bash
# All tests
go test ./...

# EVM only
go test ./builder/evm/...

# Orchard only
go test ./builder/orchard/...

# JAM core only
go test ./node/...
```

---

## Running the System

### Starting a JAM Validator Node

```bash
# JAM core RPC on port 8540
./bin/jam run --role=validator --rpc-port=8540
```

### Starting an EVM Builder

```bash
# EVM RPC server on port 8545
./bin/jam run --role=builder --services=0 --rpc-port=8545
```

Service ID 0 = EVMServiceCode, so this starts the EVM builder with RPC on port 8545.

### Starting a Orchard Builder

```bash
# Orchard RPC server on port 8232
./bin/jam run --role=builder --services=1 --rpc-port=8232
```

Service ID 1 = OrchardServiceCode, so this starts the Orchard builder with RPC on port 8232.

### Starting Multiple Services on One Builder

```bash
# EVM on 8545, Orchard on 8546
./bin/jam run --role=builder --services=0,1 --rpc-port=8545
```

This starts:
- EVM RPC server on port 8545 (basePort + 0)
- Orchard RPC server on port 8546 (basePort + 1)

---

## Key Design

**Independent RPC Servers**: Each service runs its own HTTP server on its own port. No routing, no dispatching.

```go
// EVM on port 8545
evmServer.Start(8545)

// Orchard on port 8232
orchardServer.Start(8232)
```

**Hardcoded Initialization**: Services are initialized directly in jam.go based on serviceID.

```go
if serviceID == 0 {
    // Start EVM
} else if serviceID == 1 {
    // Start Orchard
}
```

No abstraction, no factories, no registration. Just simple if/else.

---

**End of Document**
