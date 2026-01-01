# Railgun User-Facing RPC

**Status**: 🚧 Pending Migration

This directory will contain Railgun user-facing JSON-RPC methods (z_* namespace, Zcash-compatible).

## Planned Files

- `zcash_rpc.go` - Zcash-compatible JSON-RPC method handlers
- `railgun_types.go` - RPC request/response types
- `server.go` - RPC server setup and registration

## Migration Source

Files will be moved from:
- `node/node_railgun_rpc.go` - Railgun RPC methods

## RPC Methods

The following `z_*` methods will be implemented (Zcash/Orchard-compatible):

### Chain Info
- `getblockchaininfo` - Chain statistics
- `z_getblockchaininfo` - Orchard-specific statistics
- `getbestblockhash` - Latest block hash
- `getblockcount` - Block height
- `getblockhash` - Hash by height
- `getblock` - Block by hash
- `getblockheader` - Block header by hash

### Tree State
- `z_gettreestate` - Commitment tree state (root + size)
- `z_getsubtreesbyindex` - Subtree roots (for light clients)
- `z_getnotescount` - Count of active notes

### Wallet Operations
- `z_getnewaddress` - Generate new Orchard address
- `z_listaddresses` - List wallet addresses
- `z_validateaddress` - Validate address format
- `z_getbalance` - Total balance across notes
- `z_listunspent` - List unspent notes
- `z_listreceivedbyaddress` - List notes received
- `z_listnotes` - List all notes with metadata

### Transactions
- `z_sendmany` - Send to multiple recipients
- `z_sendmanywithchangeto` - Send with explicit change address
- `z_viewtransaction` - View transaction details

## Interface

```go
type RailgunRPCServer struct {
    builder *witness.RailgunBuilder
}

func RegisterRailgunRPC(server *rpc.Server, builder *witness.RailgunBuilder)
```

## Protocol Notes

- **Privacy-native**: No sender/recipient identity leakage
- **Multi-asset**: Single shielded pool for all assets
- **Compliance**: Equity assets require allowlist proofs
- **Builder-sponsored**: Users don't pay gas directly

See [../docs/RAILGUN.md](../../../services/railgun/docs/RAILGUN.md) for service specification.
See [../docs/RPC.md](../../../services/railgun/docs/RPC.md) for RPC details.
See [../../docs/ROLLUP.md](../../docs/ROLLUP.md) for migration plan.
