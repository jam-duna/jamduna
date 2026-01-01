# EVM User-Facing RPC

**Status**: 🚧 Pending Migration

This directory will contain EVM user-facing JSON-RPC methods (eth_* namespace).

## Planned Files

- `eth_rpc.go` - Ethereum JSON-RPC method handlers
- `eth_types.go` - RPC request/response types
- `server.go` - RPC server setup and registration

## Migration Source

Files will be moved from:
- `node/node_evm_rpc.go` - EVM RPC methods
- `node/node_evm_tx.go` - Transaction operations
- `node/node_evm_logsreceipt.go` - Logs and receipts
- `node/node_evm_block.go` - Block operations
- `node/node_evm_contracts.go` - Contract state access

## RPC Methods

The following `eth_*` methods will be implemented:

### State Access
- `eth_getBalance`
- `eth_getCode`
- `eth_getStorageAt`
- `eth_getTransactionCount`

### Transaction Operations
- `eth_sendRawTransaction`
- `eth_estimateGas`
- `eth_call`

### Transaction Queries
- `eth_getTransactionReceipt`
- `eth_getTransactionByHash`
- `eth_getTransactionByBlockHashAndIndex`
- `eth_getTransactionByBlockNumberAndIndex`

### Block Queries
- `eth_blockNumber`
- `eth_getBlockByNumber`
- `eth_getBlockByHash`

### Event Logs
- `eth_getLogs`

### Network Metadata
- `eth_chainId`
- `eth_accounts`
- `eth_gasPrice`

## Interface

```go
type EVMRPCServer struct {
    builder *witness.EVMBuilder
}

func RegisterEthereumRPC(server *rpc.Server, builder *witness.EVMBuilder)
```

See [../docs/ROLLUP.md](../../docs/ROLLUP.md) for full migration plan.
