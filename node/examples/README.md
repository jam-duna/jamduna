# JAM EVM RPC Testing Guide

## Quick Start

### 1. Start the Node

In one terminal:
```bash
cd node
make evm_node
```

This starts a JAM node with:
- EVM service enabled
- RPC server on port **11100** (DefaultTCPPort + validator index 0)
- Genesis transactions that create the USDM contract
- Test transactions (ERC20 transfers)

### 2. Test RPC Methods

In another terminal, choose your testing tool:

#### Option A: Quick Monitor (Real-time Dashboard)
```bash
cd node/examples
go run rpc_monitor_watch.go   # Refreshes every 3 seconds with countdown
```

#### Option B: Comprehensive Test Suite
```bash
# Continuous mode (refreshes every 10 seconds with countdown)
go run rpc_live.go

# One-time run
go run rpc_live.go once
```

#### Option C: Simple Examples
```bash
# Continuous mode (refreshes every 5 seconds with countdown)
go run rpc_simple.go

# One-time run
go run rpc_simple.go once
```

## What Works Now ✅

### Fully Functional Methods

| Method | Status | Notes |
|--------|--------|-------|
| `GetBalance` | ✅ | Reads from USDM contract via SSR shards + DA |
| `GetTransactionCount` | ✅ | Reads nonces from USDM slot 1 |
| `GetStorageAt` | ✅ | Generic contract storage with shard resolution |
| `GetTransactionReceipt` | ✅ | Full receipt with success status, gas, from/to |
| `GetBlockByNumber` | ✅ | Returns blocks with transaction hashes |
| `TxPoolStatus` | ✅ | Pool statistics |
| `TxPoolContent` | ✅ | Pending/queued transactions |
| `TxPoolInspect` | ✅ | Human-readable pool summary |

### Example Output

```bash
$ go run rpc_live.go

JAM EVM RPC Live Test
=====================

Connecting to 127.0.0.1:11100...
✅ Connected to RPC server

Test: GetTransactionReceipt
---------------------------------------------------
✅ Status: 0x1
✅ GasUsed: 0x53c0
✅ From: 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
✅ To: 0x0000000000000000000000000000000000000001
✅ Logs: 0 events
```

## Architecture

### RPC Transport: Go `net/rpc` (NOT JSON-RPC)

**Important:** The current implementation uses Go's `net/rpc` package, not HTTP JSON-RPC.

```go
// This works ✅
client, _ := rpc.Dial("tcp", "127.0.0.1:11100")
client.Call("jam.GetBalance", []string{address, "latest"}, &result)

// This does NOT work ❌
// curl -X POST http://localhost:11100 \
//   -H "Content-Type: application/json" \
//   -d '{"jsonrpc":"2.0","method":"jam.GetBalance",...}'
```

### Data Flow

```
RPC Method Call
    ↓
ReadObject (service storage)
    ↓
ObjectRef lookup (WorkPackageHash, IndexStart, IndexEnd)
    ↓
FetchJAMDASegments (DA retrieval)
    ↓
Parse payload (SSR/Receipt/etc)
    ↓
Return result
```

## Known Limitations

### Partially Working ⚠️

1. **GetTransactionReceipt**
   - ✅ Success status, gas used, from/to addresses work
   - ❌ Logs parsing returns empty (stub at node_rpc_evmtx.go:369)
   - ❌ Block hashes are placeholders
   - ❌ Contract addresses not calculated

2. **GetBlockByNumber**
   - ✅ Returns blocks with transactions
   - ❌ Block hashes are computed from block number (not actual JAM block hash)
   - ❌ Gas calculations are placeholders

### Not Functional ❌

1. **SendRawTransaction** - No RLP decoding, signature recovery returns zero address
2. **Call** - Returns hardcoded dummy bytes, doesn't execute
3. **GetLogs** - Always returns empty array
4. **GetBlockByHash** - Returns null
5. **Historical state** - All block numbers resolve to "latest"

## Test Files

All test utilities are in this directory (`node/examples/`):

### `rpc_live.go` - Comprehensive Live Test

Tests all working methods against a running node:
- Connection verification
- TxPool methods
- Balance/nonce queries
- Storage reads
- Transaction receipts
- Block retrieval

```bash
go run rpc_live.go
```

### `rpc_monitor.go` - Quick State Monitor

Shows current state snapshot (one-time):
- TxPool statistics
- Latest block number and transaction count
- Issuer balance and nonce
- Genesis completion status

```bash
go run rpc_monitor.go
```

### `rpc_monitor_watch.go` - Continuous Monitor

Same as `rpc_monitor.go` but refreshes every 3 seconds:
- Live dashboard with formatted output
- Auto-refresh (Press Ctrl+C to stop)
- Shows recent transaction hashes

```bash
go run rpc_monitor_watch.go
```

### `watch_rpc.sh` - Shell Script Monitor

Bash script that runs `rpc_monitor.go` in a loop:

```bash
bash watch_rpc.sh
# or
./watch_rpc.sh
```

### `rpc_simple.go` - Simple Example

Basic example showing how to connect and make RPC calls:

```bash
go run rpc_simple.go
```

### Custom Testing

Create your own test client:

```go
package main

import (
    "fmt"
    "net/rpc"
)

func main() {
    client, _ := rpc.Dial("tcp", "127.0.0.1:11100")
    defer client.Close()

    var balance string
    client.Call("jam.GetBalance",
        []string{"0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", "latest"},
        &balance)
    fmt.Printf("Balance: %s\n", balance)
}
```

## Testing Workflow

### Step 1: Verify Connection

```bash
go run -<<'EOF'
package main
import ("fmt"; "net/rpc")
func main() {
    c, err := rpc.Dial("tcp", "127.0.0.1:11100")
    if err != nil { fmt.Println("❌", err); return }
    defer c.Close()
    fmt.Println("✅ Connected")
}
EOF
```

### Step 2: Check Pool Status

```bash
go run -<<'EOF'
package main
import ("fmt"; "net/rpc")
func main() {
    c, _ := rpc.Dial("tcp", "127.0.0.1:11100")
    defer c.Close()
    var s string
    c.Call("jam.TxPoolStatus", []string{}, &s)
    fmt.Println(s)
}
EOF
```

### Step 3: Get Block with Transactions

```bash
go run -<<'EOF'
package main
import ("fmt"; "net/rpc"; "encoding/json")
func main() {
    c, _ := rpc.Dial("tcp", "127.0.0.1:11100")
    defer c.Close()
    var b string
    c.Call("jam.GetBlockByNumber", []string{"latest", "false"}, &b)
    var block map[string]interface{}
    json.Unmarshal([]byte(b), &block)
    txs := block["transactions"].([]interface{})
    fmt.Printf("Block has %d transactions\n", len(txs))
    for i, tx := range txs {
        fmt.Printf("  %d: %s\n", i, tx)
    }
}
EOF
```

### Step 4: Get Transaction Receipts

Use transaction hashes from Step 3:

```bash
go run -<<'EOF'
package main
import ("fmt"; "net/rpc"; "encoding/json")
func main() {
    c, _ := rpc.Dial("tcp", "127.0.0.1:11100")
    defer c.Close()

    txHash := "0x5354d8160d5cb5f4856484dd0fc83cc1b7aaf5d8642a1eec0c013acb9ce7b103"
    var r string
    c.Call("jam.GetTransactionReceipt", []string{txHash}, &r)

    var receipt map[string]interface{}
    json.Unmarshal([]byte(r), &receipt)
    fmt.Printf("Status: %v\n", receipt["status"])
    fmt.Printf("GasUsed: %v\n", receipt["gasUsed"])
    fmt.Printf("From: %v\n", receipt["from"])
    fmt.Printf("To: %v\n", receipt["to"])
}
EOF
```

## Port Configuration

The RPC server port is: `DefaultTCPPort + validatorIndex`

- Validator 0: **11100**
- Validator 1: **11101**
- Validator 2: **11102**
- etc.

See [node_rpc_port.go](node_rpc_port.go):
```go
var DefaultTCPPort = 11100
```

And [node_rpc.go:934](node_rpc.go:934):
```go
go node.StartRPCServer(int(id))
```

Which calls [node_rpc.go:981](node_rpc.go:981):
```go
address := fmt.Sprintf(":%d", DefaultTCPPort+validatorIndex)
```

## State Seeding

The test creates genesis state in [node_jamtest_evm.go:18-79](node_jamtest_evm.go:18-79):

1. **USDM Contract** deployed at `0x0000000000000000000000000000000000000001`
2. **Issuer (Alice)** at `0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266`
   - Balance: 61M tokens (61,000,000 * 10^18)
   - Nonce: 1
3. **Test Transactions** - ERC20 transfers from issuer to various recipients

**Note:** Genesis takes a few seconds to complete. If balances are zero, wait and re-run the test.

## Troubleshooting

### "Failed to connect: connection refused"

The node isn't running. Start it with:
```bash
cd node && make evm_node
```

### "Balance is zero"

Genesis hasn't completed yet. Wait for the test output to show:
```
evm(0) work item 0 result: ...
```

Then re-run your test.

### "Transaction not found"

Either:
1. The transaction hasn't been processed yet
2. You're using a dummy/wrong hash
3. Check block transactions first with `GetBlockByNumber`

### "No blocks yet"

The test is still initializing. Wait a moment and try again.

## Next Steps

### To Add HTTP/JSON-RPC Support

The RPC methods are implemented, but the transport layer needs work:

1. Add HTTP server wrapper around existing methods
2. Implement JSON-RPC request/response format
3. Add CORS headers for browser access
4. Enable WebSocket support for subscriptions

### To Fix Broken Methods

1. **SendRawTransaction**: Implement RLP decoding and ECDSA signature recovery
2. **Call**: Execute work package and extract return value from work report
3. **GetLogs**: Parse log data from receipt LogsData bytes
4. **GetBlockByHash**: Implement block hash → block number mapping
5. **Historical state**: Implement StateDB snapshots by block number

## Summary

✅ **What Works**: RPC server is functional with Go `net/rpc` transport
✅ **Storage Layer**: DA reads via ReadObject/FetchJAMDASegments working end-to-end
✅ **Core Methods**: GetBalance, GetTransactionCount, GetStorageAt, GetTransactionReceipt
⚠️ **Limitations**: Not JSON-RPC, no HTTP, some methods incomplete
❌ **Blockers**: SendRawTransaction, Call, GetLogs need implementation

The foundation is solid - the two-tier storage (ObjectRef + DA) works correctly. The remaining work is primarily:
1. Transport layer (HTTP/JSON-RPC)
2. Missing method implementations (RLP, logs, execution)
3. Historical state access
