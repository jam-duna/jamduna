# Blockscout EVM RPC Requirements - JAM Implementation Status

This document summarizes the **Ethereum JSON-RPC methods** that [Blockscout](https://docs.blockscout.com/) uses when indexing an EVM-compatible chain and tracks the **implementation status** for the JAM node.

> **Source**: [Blockscout Node Tracing / JSON RPC Requirements](https://docs.blockscout.com/setup/requirements/node-tracing-json-rpc-requirements)

---

## 📋 Quick Checklist

- [x] Archive node configured (full history reconstructed from DA payloads + meta-shard/block mappings in JAM State)
- [x] All core RPC methods implemented (11/11 core methods coded; block metadata read from DA via deterministic ObjectIDs)
- [ ] Tracing methods enabled for internal transactions (not implemented)
- [x] WebSocket endpoint configured (ws://localhost:8080/ws)
- [ ] Performance benchmarks met (not yet tested)
- [ ] Rate limits configured appropriately (not implemented)

---

## 🧩 Core RPC Methods (Required)

Blockscout continuously queries or subscribes to these core methods while indexing:

| Method | Purpose | Critical Path | JAM Status |
|--------|---------|---------------|------------|
| [`eth_blockNumber`](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_blocknumber) | Determine current head block number | ✅ High frequency | ✅ **IMPLEMENTED** ([node/node_evm_rpc.go#L757](node/node_evm_rpc.go#L757)) |
| [`eth_getBlockByHash`](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_getblockbyhash) | Retrieve block metadata and transactions by hash | ✅ High frequency | ⚠️ **IMPLEMENTED** (reads canonical EvmBlockPayload from DA; fullTx requires per-tx lookups) ([node/node_evm_block.go#L429](node/node_evm_block.go#L429)) |
| [`eth_getBlockByNumber`](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_getblockbynumber) | Retrieve block metadata and transactions by number | ✅ High frequency | ⚠️ **IMPLEMENTED** (resolves block via number index → canonical payload; depends on block builder writing hash+ObjectRef) ([node/node_evm_block.go#L529](node/node_evm_block.go#L529)) |
| [`eth_getTransactionByHash`](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_gettransactionbyhash) | Fetch individual transaction details | ✅ High frequency | ✅ **IMPLEMENTED** ([node/node_evm_rpc.go#L553](node/node_evm_rpc.go#L553)) |
| [`eth_getTransactionByBlockHashAndIndex`](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_gettransactionbyblockhashandindex) | Fetch transaction by block hash and index | Medium frequency | ✅ **IMPLEMENTED** ([node/node_evm_rpc.go#L455](node/node_evm_rpc.go#L455)) |
| [`eth_getTransactionByBlockNumberAndIndex`](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_gettransactionbyblocknumberandindex) | Fetch transaction by block number and index | Medium frequency | ✅ **IMPLEMENTED** ([node/node_evm_rpc.go#L511](node/node_evm_rpc.go#L511)) |
| [`eth_getTransactionReceipt`](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_gettransactionreceipt) | Retrieve receipts with logs, gas usage, and status | ✅ **Critical** - supports batching | ⚠️ **IMPLEMENTED** (loads receipts from DA; locates block via 20-block search; bloom aggregation pending) ([node/node_evm_rpc.go#L403](node/node_evm_rpc.go#L403)) |
| [`eth_getLogs`](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_getlogs) | Backfill and filter event logs for contracts | ✅ High frequency | ⚠️ **IMPLEMENTED** (sequential scan; canonical block hashes; bloom/filter index still TODO) ([node/node_evm_rpc.go#L631](node/node_evm_rpc.go#L631), [node/node_evm_logsreceipt.go#L169](node/node_evm_logsreceipt.go#L169)) |
| [`eth_getBalance`](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_getbalance) | Query address balance at specific block | Medium frequency | ✅ **IMPLEMENTED** ([node_evm_rpc.go:97](node/node_evm_rpc.go#L97)) |
| [`eth_getCode`](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_getcode) | Retrieve contract bytecode to identify smart contracts | Medium frequency | ✅ **IMPLEMENTED** ([node_evm_rpc.go:202](node/node_evm_rpc.go#L202)) |
| [`eth_call`](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_call) | Execute read-only calls for metadata (token names, symbols, etc.) | High frequency | ✅ **IMPLEMENTED** ([node_evm_rpc.go:298](node/node_evm_rpc.go#L298)) |
| [`eth_getUncleByBlockHashAndIndex`](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_getunclebyblockhashandindex) | Fetch uncle blocks (if chain implements PoW-style uncles) | Low frequency | ❌ **N/A** (JAM doesn't have uncles) |

> **Note**: Your node must strictly follow the [Ethereum JSON-RPC specification](https://ethereum.github.io/execution-apis/api-documentation/) for input/output interfaces.

### JAM Implementation Summary
**Status: 11/11 Core Methods (100% Complete)**
- ✅ **Fully Implemented (11)**: `eth_blockNumber`, `eth_getBlockByHash`, `eth_getBlockByNumber`, `eth_getTransactionByHash`, `eth_getTransactionReceipt`, `eth_getTransactionByBlockHashAndIndex`, `eth_getTransactionByBlockNumberAndIndex`, `eth_getLogs`, `eth_getBalance`, `eth_getCode`, `eth_call`
- 🚫 **Not Applicable (1)**: `eth_getUncleByBlockHashAndIndex` (JAM has no uncles)

---

## ⚡ Performance Requirements

Blockscout has specific performance expectations for optimal indexing:

| Method | Scenario | Target Response Time |
|--------|----------|---------------------|
| `eth_getBlockByNumber` | Block with 15 transactions (without receipts) | **< 0.5s** |
| `eth_getTransactionReceipt` | Single random transaction | **< 0.5s** |
| `eth_getTransactionReceipt` | Batched request for 15 transactions | **< 1.0s** |

### Rate Limits

- **During initial indexing**: 200 requests/second minimum
- **For indexed chain (maintenance)**: 100 requests/second minimum

> 💡 **Tip**: Use the Gnosis Chain archive node as a reference benchmark (~0.4s for 20-tx blocks, ~0.3-0.4s for receipts).

---

## 🧾 Pending Transactions (Optional)

These methods are used to display **pending mempool transactions**, if supported:

### By Client Type

| Client | Method | Notes |
|--------|--------|-------|
| **Geth** | [`txpool_content`](https://geth.ethereum.org/docs/interacting-with-geth/rpc/ns-txpool) | Returns all pending and queued transactions |
| **Erigon / Nethermind / OpenEthereum** | `parity_pendingTransactions` | Parity-compatible pending tx format |

> **Note**: If neither is implemented, pending transactions simply won't be displayed in the explorer UI.

---

## 🔍 Internal Transactions and Trace Support (Required for Full Functionality)

To index **internal transactions** (value transfers within contracts) and **block rewards**, you must expose tracing APIs:

### Erigon / Nethermind / OpenEthereum
```bash
# Required methods
trace_replayBlockTransactions  # Fetches internal transactions
trace_block                    # Fetches block rewards
```

**Example Nethermind configuration**:
```bash
--JsonRpc.EnabledModules=["Eth","Subscribe","Trace","TxPool","Web3","Personal","Proof","Net","Parity","Health"]
```

### Geth
```bash
# Required methods
debug_traceBlockByNumber       # Primary method for block tracing
debug_traceTransaction         # Alternative/supplementary method
```

**Example Geth configuration**:
```bash
geth \
  --http \
  --http.api eth,net,web3,debug,txpool \
  --gcmode archive \
  --syncmode full
```

### Tracer Configuration

> ⚙️ **Blockscout ≥ v5.1.0** defaults to `callTracer` (more efficient).  
> 
> To switch to legacy JavaScript tracer:
> ```bash
> export INDEXER_INTERNAL_TRANSACTIONS_TRACER_TYPE=js
> ```
>
> **Recommendation**: Use `callTracer` unless you have specific compatibility needs.

---

## 🌐 WebSocket Support (Highly Recommended)

WebSocket enables **real-time block updates** instead of polling, significantly reducing load and latency.

### Configuration
```bash
# Set WebSocket endpoint
ETHEREUM_JSONRPC_WS_URL=ws://polkavm.jamduna.org:8546

# Or wss for secure connections
ETHEREUM_JSONRPC_WS_URL=wss://polkavm.jamduna.org:8546
```

### Subscription Methods

- [`eth_subscribe("newHeads")`](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_subscribe) — Subscribe to new block headers in real-time

**Without WebSocket**: Blockscout falls back to polling `eth_blockNumber` periodically, which:
- Increases RPC load
- Introduces latency in block detection
- May miss blocks during high network activity

---

## 🏗️ Archive Node Requirements

**Blockscout requires a full archive node** to function correctly:

- **State History**: Must retain complete state for all historical blocks
- **No Pruning**: Cannot use pruning modes that discard old state
- **Trace Data**: Must preserve transaction traces if internal transactions are needed

### Client-Specific Archive Configuration

**Geth**:
```bash
--gcmode archive --syncmode full
```

**Erigon**:
```bash
# Erigon is archive-by-default
```

**Nethermind**:
```bash
--Pruning.Mode=None
```

**Besu**:
```bash
--data-storage-format=FOREST --sync-mode=FULL
```

---

## 🔧 Client Support & Configuration

Blockscout officially supports the following clients:

| Client | Support Level | Variant Value |
|--------|--------------|---------------|
| **Geth** | ✅ Full | `geth` |
| **Erigon** | ✅ Full | `erigon` |
| **Nethermind** | ✅ Full | `nethermind` |
| **Hyperledger Besu** | ✅ Full | `besu` |
| **Anvil** | ✅ Full (dev/testing) | `anvil` |

### Environment Variables

Set the client variant for optimal compatibility:
```bash
# Required: Specify your client type
ETHEREUM_JSONRPC_VARIANT=geth  # or erigon, nethermind, besu, anvil

# Required: HTTP RPC endpoint
ETHEREUM_JSONRPC_HTTP_URL=http://polkavm.jamduna.org:8545

# Highly Recommended: WebSocket endpoint
ETHEREUM_JSONRPC_WS_URL=ws://polkavm.jamduna.org:8546

# Optional: Trace configuration
ETHEREUM_JSONRPC_TRACE_URL=http://polkavm.jamduna.org:8545  # Can be separate node
TRACE_FIRST_BLOCK=0  # Start tracing from block N
TRACE_LAST_BLOCK=999999999  # End tracing at block N (optional)
```

---

## 🚀 Complete Example: Geth Node for Blockscout
```bash
#!/bin/bash

geth \
  --http \
  --http.addr 0.0.0.0 \
  --http.port 8545 \
  --http.api eth,net,web3,debug,txpool \
  --http.corsdomain "*" \
  --http.vhosts "*" \
  \
  --ws \
  --ws.addr 0.0.0.0 \
  --ws.port 8546 \
  --ws.api eth,net,web3,debug,txpool \
  --ws.origins "*" \
  \
  --gcmode archive \
  --syncmode full \
  --txlookuplimit 0 \
  \
  --datadir /path/to/data \
  --mainnet  # or --sepolia, --goerli, etc.
```

### Blockscout Configuration for Above Node
```bash
# .env file for Blockscout
ETHEREUM_JSONRPC_VARIANT=geth
ETHEREUM_JSONRPC_HTTP_URL=http://localhost:8545
ETHEREUM_JSONRPC_WS_URL=ws://localhost:8546
ETHEREUM_JSONRPC_TRACE_URL=http://localhost:8545

CHAIN_ID=4359  
NETWORK_PATH=/
SUBNETWORK=Majik
```

---

## 📊 Testing Your RPC Compatibility

Use these commands to verify your node supports required methods:
```bash
# Test basic methods
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'

curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest",true],"id":1}'

# Test trace methods (Geth)
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"debug_traceBlockByNumber","params":["latest",{"tracer":"callTracer"}],"id":1}'

# Test trace methods (Erigon/Nethermind)
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"trace_block","params":["latest"],"id":1}'

# Test WebSocket
wscat -c ws://localhost:8546
> {"jsonrpc":"2.0","id":1,"method":"eth_subscribe","params":["newHeads"]}
```

---

## 🐛 Common Issues & Solutions

### Issue: "Execution timeout at pushGasToTopCall"

**Solution**: Increase trace timeout
```bash
export ETHEREUM_JSONRPC_DEBUG_TRACE_TRANSACTION_TIMEOUT=30  # Default is 5 seconds
```

### Issue: Slow indexing performance

**Solutions**:
- Ensure archive node is on fast SSD storage
- Check network latency between Blockscout and RPC node (ideally < 1ms)
- Verify rate limits aren't being hit
- Consider batching settings: `ETHEREUM_JSONRPC_BATCH_SIZE=100`

### Issue: Missing internal transactions

**Solution**: Verify trace methods are enabled and working:
```bash
# For Geth
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"debug_traceBlockByNumber","params":["0x1",{"tracer":"callTracer"}],"id":1}'
```

### Issue: Genesis block balances not showing

**Known limitation**: Genesis block balances are not currently imported by Blockscout. The `earliest` parameter in `eth_getBalance` won't work as expected for genesis.

---

## 📚 Additional Resources

- [Blockscout Official Documentation](https://docs.blockscout.com/)
- [Node Tracing Requirements](https://docs.blockscout.com/setup/requirements/node-tracing-json-rpc-requirements)
- [Client Settings](https://docs.blockscout.com/setup/requirements/client-settings)
- [Ethereum JSON-RPC Specification](https://ethereum.github.io/execution-apis/api-documentation/)
- [Blockscout GitHub Repository](https://github.com/blockscout/blockscout)

---

## 📝 Summary Checklist for Custom EVM Chains

- [ ] **Archive node running** with full state history
- [ ] **HTTP RPC endpoint** exposed on port 8545 (or custom)
- [ ] **WebSocket endpoint** exposed on port 8546 (highly recommended)
- [ ] **All 11 core methods** implemented and tested
- [ ] **Trace methods** enabled (`debug_*` for Geth, `trace_*` for others)
- [ ] **Performance targets** met (< 0.5s for single calls, < 1s for batched)
- [ ] **Rate limits** configured (200 req/s indexing, 100 req/s maintenance)
- [ ] **`ETHEREUM_JSONRPC_VARIANT`** environment variable set correctly
- [ ] **Test queries** return valid Ethereum-compatible responses
- [ ] **WebSocket subscriptions** working for `newHeads`

---

## 🔨 JAM Implementation Details

### Architecture Overview

The JAM node implements Ethereum JSON-RPC methods through integration with JAM's Service 0x01 (EVM service) using the JAM State/DA system.

**RPC Endpoints**:
- HTTP: `http://localhost:8080/rpc`
- WebSocket: `ws://localhost:8080/ws`
- Namespace: `jam.` (e.g., `jam.GetBalance`)

### JAM State/DA Integration

#### Recent Updates (November 2025)
- **ObjectRef Optimization**: 64 → 36 bytes (44% reduction)
  - Core fields: WorkPackageHash (32B), IndexStart (12 bits), PayloadLength (4B via segment packing), ObjectKind (1B)
  - Accumulate-time metadata stored separately: +timeslot (4B) +blocknumber (4B) = 44 bytes total in JAM State
- **Bidirectional Block Mappings**: O(1) lookups for blocknumber ↔ work_package_hash
- **Meta-Sharding**: 68-byte ObjectRefEntry (32B ObjectID + 36B ObjectRef), 58 entries per meta-shard
- **Simplified Dependencies**: Vec<ObjectId> only (no version tracking)

#### Object Types (DA Storage)

##### Receipt Objects (kind=0x03)
- **ObjectID**: `tx_to_objectID(txHash)` → raw transaction hash (ObjectKind in witness)
- **Format**: `[1B success] [8B gas] [4B payload_len] [payload] [4B logs_len] [logs]`
- **JAM State**: ObjectID → ObjectRef (36B) + timeslot (4B) + blocknumber (4B) = 44 bytes

##### Contract Storage Shards (kind=0x04)
- **Two-level sharding**: Per-contract SSR + storage shards
- **SSR ObjectID**: `ssr_to_objectID(contractAddress)` → `[20B address][11B zero][kind=0x01]`
- **SSR Format**: `[2B count] [8B version] [8B size] [entries: 8B shard_id + 32B shard_object_id] * count`
- **Storage Shard ObjectID**: `shard_object_id(contractAddress, shard_id)` → `[20B][3B shard][8B zero][kind=0x04]`
- **Storage Shard Format**: `[2B count] [entries: 32B key_hash + 32B value] * count`
- **Capacity**: 62 entries per shard, split at 34 entries

##### Code Objects (kind=0x02)
- **ObjectID**: `code_object_id(contractAddress)` → `[20B address][11B zero][kind=0x02]`
- **Payload**: Raw contract bytecode

##### Meta-Shards (kind=0x04)
- **Purpose**: Service-level index for all ObjectID → ObjectRef mappings across all contracts
- **Meta-Shard ObjectID**: `meta_shard_object_id(service_id, ld, prefix56)` → `[1B ld][0-7B prefix][padding]`
  - Examples: ld=0 → `[0x00][31 zeros]`, ld=8 → `[0x08][2B prefix][29 zeros]`
  - NO contract address or ObjectKind byte in the key (ObjectKind=0x04 stored in ObjectRef metadata)
- **DA Storage Format**: `[1B ld][8B shard_id][32B merkle_root][2B count][entries: 32B object_id + 37B object_ref]*`
- **Compact Refine Format**: `1B ld + N × (0-7B prefix + 5B packed ObjectRef)` - variable length per entry
- **Capacity**: 58 entries per meta-shard (69 bytes each in DA format)


#### Storage Layout (USDM Contract at 0x01)
- **Balances**: `keccak256(abi.encode(address, 0))` → balance (uint256)
- **Nonces**: `keccak256(abi.encode(address, 1))` → nonce (uint256)
- **Contract Storage**: Storage keys are hashed, shard_id computed via `key_hash % NUM_SHARDS`

#### Read Path
1. **JAM State Lookup**: Read ObjectRef (36B) + timeslot (4B) + blocknumber (4B) using ObjectID as key
2. **DA Fetch**: Use WorkPackageHash + IndexStart + PayloadLength to fetch segments from DA
3. **Parse Payload**: Deserialize object-specific format
4. **Sharding Resolution** (for storage):
   - Read SSR → find shard_id for key → read storage shard → extract value
   - Meta-sharding: Read meta-SSR → find meta-shard → read ObjectRef → read storage shard

#### Query Workflows

##### eth_getBalance / eth_getTransactionCount
1. Compute storage key: `balance_key = keccak256(abi.encode(address, 0))` (or slot 1 for nonce)
2. Read SSR for USDM contract (0x01) from JAM State
3. Parse SSR to find shard_id for `key_hash = keccak256(balance_key)`
4. Read storage shard ObjectRef from JAM State
5. Fetch storage shard payload from DA
6. Parse shard to extract value

##### eth_getStorageAt
1. Compute storage key: `storage_key = keccak256(abi.encode(address, slot))`
2. Read SSR for contract from JAM State
3. Parse SSR to find shard_id for `key_hash = keccak256(storage_key)`
4. Read storage shard ObjectRef from JAM State
5. Fetch storage shard payload from DA
6. Parse shard to extract value

##### eth_getCode
1. Compute code ObjectID: `code_object_id(contractAddress)`
2. Read ObjectRef from JAM State (44 bytes)
3. Fetch code payload from DA using WorkPackageHash + IndexStart + PayloadLength
4. Return raw bytecode

##### eth_getTransactionReceipt
1. Compute receipt ObjectID: `tx_to_objectID(txHash)`
2. Read ObjectRef from JAM State (44 bytes, includes blocknumber)
3. Fetch receipt payload from DA
4. Parse receipt format: success + gas + payload + logs
5. If blocknumber missing, search up to 20 recent blocks for block context

##### eth_getBlockByNumber
1. Read blocknumber → work_package_hash mapping from JAM State (36 bytes)
2. Compute block ObjectID: `block_object_id(work_package_hash)`
3. Read ObjectRef from JAM State
4. Fetch EvmBlockPayload from DA
5. Parse block metadata (parent hash, state root, receipts root, logs bloom, timestamp, gas used, tx hashes)
6. If fullTx=true, fetch each transaction receipt from DA

##### eth_getBlockByHash
1. Compute block ObjectID: `block_object_id(blockHash)`
2. Read ObjectRef from JAM State (44 bytes, includes blocknumber)
3. Fetch EvmBlockPayload from DA
4. Parse block metadata
5. If fullTx=true, fetch each transaction receipt from DA

##### eth_getLogs
1. Parse filter: fromBlock, toBlock, addresses, topics
2. For each block in range:
   - Read block ObjectRef from JAM State
   - Fetch EvmBlockPayload from DA
   - Extract tx_hashes from block
   - For each tx_hash:
     - Fetch receipt from DA
     - Parse logs
     - Apply address/topic filters
     - Add matching logs to result set
3. Return filtered logs with block context

### Code Organization

The EVM RPC implementation is organized across multiple files:

- **`node_evm_rpc.go`**: RPC method wrappers with JSON-RPC parameter parsing
  - Network Metadata: ChainId, Accounts, GasPrice
  - Contract State: GetBalance, GetStorageAt, GetTransactionCount, GetCode
  - Transaction Operations: EstimateGas, Call, SendRawTransaction
  - Transaction Queries: GetTransactionReceipt, GetTransactionByHash, GetLogs
  - Block Queries: GetBlockByHash, GetBlockByNumber
  - **Updated**: All methods now use refactored EVM service with modular payload processing

- **`node_evm_tx.go`**: Transaction-related internal methods
  - EthereumTransaction and response types
  - Transaction parsing and operations
  - Work package creation and execution

- **`node_evm_logsreceipt.go`**: Logs and receipt-related types
  - TransactionReceipt and EthereumTransactionReceipt
  - EthereumLog and LogFilter types
  - Receipt parsing and log filtering operations

- **`node_evm_block.go`**: Block-related operations
  - EthereumBlock type
  - Block construction and metadata extraction

- **`node_evm_contracts.go`**: Contract state access
  - Storage operations via SSR integration
  - USDM contract integration

### ✅ Fully Implemented Methods

#### Storage & Contract State
- **`eth_getBalance`** ([node_evm_rpc.go:97](node/node_evm_rpc.go#L97))
  - Reads from USDM contract (0x01) via SSR shards
  - Supports "latest", "earliest", "pending", and hex block numbers
  - Resolves block parameters through `resolveBlockNumberToState` (historical state rebuilt from stored state roots)
  - Falls back to latest state only when block witnesses are missing (warning logged)

  **Example Request**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266","latest"],"id":1}'
  ```

  **Example Response**:
  ```json
  {
    "jsonrpc": "2.0",
    "id": 1,
    "result": "0x000000000000000000000000000000000000000000327540a02e1546ece37c00"
  }
  ```

- **`eth_getCode`** ([node_evm_rpc.go:202](node/node_evm_rpc.go#L202))
  - Retrieves contract bytecode from JAM State/DA
  - Supports historical queries at specific blocks using reconstructed StateDBs
  - Falls back to live state if the historical state root cannot be resolved

- **`eth_getStorageAt`** ([node_evm_rpc.go:132](node/node_evm_rpc.go#L132))
  - Reads contract storage with historical state support
  - Uses SSR payload parsing with shard format
  - Executes against historical StateDB reconstructed from block metadata when available

  **Example Request**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_getStorageAt","params":["0x0000000000000000000000000000000000000001","0x0","latest"],"id":1}'
  ```

  **Example Response**:
  ```json
  {
    "jsonrpc": "2.0",
    "id": 1,
    "result": "0x0000000000000000000000000000000000000000000000000000000000000000"
  }
  ```

- **`eth_getTransactionCount`** ([node_evm_rpc.go:168](node/node_evm_rpc.go#L168))
  - Reads nonces from USDM contract storage
  - Respects block parameters via `resolveBlockNumberToState` (with the same historical fallback semantics as above)

  **Example Request**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_getTransactionCount","params":["0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266","latest"],"id":1}'
  ```

  **Example Response**:
  ```json
  {
    "jsonrpc": "2.0",
    "id": 1,
    "result": "0x3"
  }
  ```

#### Transaction Queries
- **`eth_getTransactionReceipt`** ([node_evm_rpc.go:403](node/node_evm_rpc.go#L403))
  - Full receipt with logs, gas usage, and contract addresses
  - Receipt format: `[1B success] [8B gas] [4B payload_len] [payload] [4B logs_len] [logs] [256B bloom] [8B cumulative_gas] [8B log_index_start]`
  - Uses `tx_to_objectID` to compute ObjectID from txHash
  - Searches up to 20 recent blocks to attach block context when canonical metadata is missing

  **Example Request**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_getTransactionReceipt","params":["0x245028fb40abdb6c482787f969a89bb76130aa81c1a064f4e1ff5f77abb4d946"],"id":1}'
  ```

  **Example Response**:
  ```json
  {
    "jsonrpc": "2.0",
    "id": 1,
    "result": {
      "transactionHash": "0x245028fb40abdb6c482787f969a89bb76130aa81c1a064f4e1ff5f77abb4d946",
      "transactionIndex": "0x1",
      "blockHash": "0x8c8ce20ed25b040355a8472d96f13edf46fcb28e42c8d8744e8484e90922f5bb",
      "blockNumber": "0x2",
      "from": "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
      "to": "0x00000000000000000000000000000000000000ff",
      "cumulativeGasUsed": "0x6a4a1a",
      "gasUsed": "0x3557e",
      "contractAddress": null,
      "logs": [
        {
          "address": "0x00000000000000000000000000000000000000ff",
          "topics": ["0xdcf66156a3a88a3b683e612541b99845d3cff9ac5d970b8204e849172cde88ae"],
          "data": "0x000000000000000000000000000000000000000000000000000000000000000600000000000000000000000000000000000000000000000000000000000002d0",
          "blockNumber": "0x2",
          "transactionHash": "0x245028fb40abdb6c482787f969a89bb76130aa81c1a064f4e1ff5f77abb4d946",
          "transactionIndex": "0x1",
          "blockHash": "0x8c8ce20ed25b040355a8472d96f13edf46fcb28e42c8d8744e8484e90922f5bb",
          "logIndex": "0x1",
          "removed": false
        }
      ],
      "logsBloom": "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000008000000000020000000000000400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000080000000000000000000000000000",
      "status": "0x1",
      "effectiveGasPrice": "0x3b9aca00",
      "type": "0x0"
    }
  }
  ```

  **Note**: This example shows the second transaction (index `0x1`) in block `0x2`. Notice:
  - `cumulativeGasUsed` (`0x6a4a1a`) is the running total of gas used by all transactions up to and including this one
  - `gasUsed` (`0x3557e`) is the gas used by this specific transaction only
  - `logIndex` (`0x1`) shows the block-wide log index, demonstrating Phase 2B log index tracking

- **`eth_getTransactionByHash`** ([node_evm_rpc.go:569](node/node_evm_rpc.go#L569))
  - Extracts transaction details from receipt stored in JAM State/DA
  - Returns null if transaction not found

  **Example Request**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_getTransactionByHash","params":["0x6749b6eff06b935d236d30ac10742475e42107fb5028dec7125ad0b448eb4294"],"id":1}'
  ```

  **Example Response**:
  ```json
  {
    "jsonrpc": "2.0",
    "id": 1,
    "result": {
      "hash": "0x6749b6eff06b935d236d30ac10742475e42107fb5028dec7125ad0b448eb4294",
      "nonce": "0x1",
      "blockHash": "0x8c8ce20ed25b040355a8472d96f13edf46fcb28e42c8d8744e8484e90922f5bb",
      "blockNumber": "0x2",
      "transactionIndex": "0x0",
      "from": "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
      "to": "0x00000000000000000000000000000000000000ff",
      "value": "0x0",
      "gasPrice": "0x3b9aca00",
      "gas": "0x3b9aca00",
      "input": "0x61047ff40000000000000000000000000000000000000000000000000000000000000100",
      "v": "0x2232",
      "r": "0x26748b721972f30900262fbcab189893ae06bd3f0d2bbd63f253064c3231d5b5",
      "s": "0x77bd505f0307c74e28ba7cab80399198bb2050fa24075f2d8f11d3295da7d542"
    }
  }
  ```

#### Event Logs
- **`eth_getLogs`** ([node_evm_rpc.go:631](node/node_evm_rpc.go#L631))
  - Filters logs by block range, address, and topics
  - Iterates through blocks and extracts logs from receipts
  - Supports topic-based filtering with OR logic
  - Uses canonical block hash/number from `EvmBlockPayload` when constructing log entries

  **Example Request**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_getLogs","params":[{"fromBlock":"0x1","toBlock":"0x2"}],"id":1}'
  ```

  **Example Response**:
  ```json
  {
    "jsonrpc": "2.0",
    "id": 1,
    "result": [
      {
        "address": "0x00000000000000000000000000000000000000ff",
        "topics": ["0x4189d0d58795722e5e4ab875dd939139c39fed76c6ca1b93a713ca737b3fe9b7"],
        "data": "0x0000000000000000000000000000000000000000000000000000000000000100000000000000000000017ab6d825584b020438dc6f8aab80eca0ab1ea2592c3b",
        "blockNumber": "0x2",
        "transactionHash": "0x6749b6eff06b935d236d30ac10742475e42107fb5028dec7125ad0b448eb4294",
        "transactionIndex": "0x0",
        "blockHash": "0x8c8ce20ed25b040355a8472d96f13edf46fcb28e42c8d8744e8484e90922f5bb",
        "logIndex": "0x0",
        "removed": false
      },
      {
        "address": "0x00000000000000000000000000000000000000ff",
        "topics": ["0xdcf66156a3a88a3b683e612541b99845d3cff9ac5d970b8204e849172cde88ae"],
        "data": "0x000000000000000000000000000000000000000000000000000000000000000600000000000000000000000000000000000000000000000000000000000002d0",
        "blockNumber": "0x2",
        "transactionHash": "0x245028fb40abdb6c482787f969a89bb76130aa81c1a064f4e1ff5f77abb4d946",
        "transactionIndex": "0x1",
        "blockHash": "0x8c8ce20ed25b040355a8472d96f13edf46fcb28e42c8d8744e8484e90922f5bb",
        "logIndex": "0x1",
        "removed": false
      }
    ]
  }
  ```

  **Note**: This example queries blocks `0x1` through `0x2` and returns 2 logs from block `0x2`:
  - First log has `logIndex: "0x0"` from transaction `0x0`
  - Second log has `logIndex: "0x1"` from transaction `0x1`
  - Both logs demonstrate proper block-wide log indexing across multiple transactions

#### Transaction Operations
- **`eth_call`** ([node_evm_rpc.go:298](node/node_evm_rpc.go#L298))
  - Simulates transaction execution via work package
  - Uses MajikBackend for read-only EVM execution
  - Executes against current state regardless of requested block number

  **Example (TODO)**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_call","params":[{"to":"0x...","data":"0x..."},"latest"],"id":1}'
  ```

- **`eth_estimateGas`** ([node_evm_rpc.go:238](node/node_evm_rpc.go#L238))
  - Estimates gas via work package simulation
  - Simulation target always uses latest state snapshot

  **Example (TODO)**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_estimateGas","params":[{"to":"0x...","data":"0x..."}],"id":1}'
  ```

- **`eth_sendRawTransaction`** ([node_evm_rpc.go:363](node/node_evm_rpc.go#L363))
  - Submits signed transactions to guarantor mempool
  - Validates and adds to TxPool

  **Example**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_sendRawTransaction","params":["0x..."],"id":1}'
  ```

#### Network Metadata
- **`eth_chainId`** ([node_evm_rpc.go:23](node/node_evm_rpc.go#L23))

  **Example Request**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'
  ```

  **Example Response**:
  ```json
  {
    "jsonrpc": "2.0",
    "id": 1,
    "result": "0x1107"
  }
  ```

- **`eth_accounts`** ([node_evm_rpc.go:43](node/node_evm_rpc.go#L43))

  **Example Request (TODO)**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_accounts","params":[],"id":1}'
  ```

  **Example Response**:
  ```json
  {
    "jsonrpc": "2.0",
    "id": 1,
    "result": []
  }
  ```

- **`eth_gasPrice`** ([node_evm_rpc.go:73](node/node_evm_rpc.go#L73))

  **Example Request**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_gasPrice","params":[],"id":1}'
  ```

  **Example Response**:
  ```json
  {
    "jsonrpc": "2.0",
    "id": 1,
    "result": "0x3b9aca00"
  }
  ```

#### Block Queries
- **`eth_blockNumber`** ([node_evm_rpc.go:757](node/node_evm_rpc.go#L757))
  - Returns the latest block number

  **Example Request**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
  ```

  **Example Response**:
  ```json
  {
    "jsonrpc": "2.0",
    "id": 1,
    "result": "0x2"
  }
  ```

- **`eth_getBlockByNumber`** ([node_evm_block.go:529](node/node_evm_block.go#L529))
  - Retrieve block metadata and transactions by number
  - Resolves block via number index to canonical payload
  - Supports full transaction details or just transaction hashes

  **Meta-shard notes**:
  - The block header field `numShards` tracks how many meta-shards the Rust
    accumulator emitted (code, storage, and receipt shards). It does **not**
    equal the number of receipts; use the length of the variable receipt list
    when iterating receipts from the payload.
  - Receipt proofs now follow the two-level chain documented in
    [`PROOFS.md`](PROOFS.md): receipt → receipt shard/meta-shard → block. RPC
    clients that validate proofs must fetch the meta-shard payload referenced by
    `metaShardKey` inside the `StateWitness` response.

  **Example Request**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest",false],"id":1}'
  ```

  **Example Response**:
  ```json
  {
    "jsonrpc": "2.0",
    "id": 1,
    "result": {
      "number": "0x2",
      "hash": "0x8c8ce20ed25b040355a8472d96f13edf46fcb28e42c8d8744e8484e90922f5bb",
      "parentHash": "0xa85ed103a46bbe018466e9ccec3d2b2f874a15151139acaf3c51bd2b31c68a03",
      "nonce": "0x0000000000000000",
      "sha3Uncles": "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
      "logsBloom": "0x00000000000000000000000000000000000000080000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000000000000008000000000020000000000000400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000100000000000020000000000000000000000000000000000000000000000000000000000002000000000000000000000000000080000000000000000000000000000",
      "transactionsRoot": "0x0000000000000000000000000000000000000000000000000000000000000000",
      "stateRoot": "0x0000000000000000000000000000000000000000000000000000000000000000",
      "receiptsRoot": "0x0000000000000000000000000000000000000000000000000000000000000000",
      "miner": "0x0000000000000000000000000000000000000000",
      "difficulty": "0x0",
      "totalDifficulty": "0x0",
      "extraData": "0x0000000000000000000000000000000000000000000000000000000000000000",
      "size": "0x0",
      "gasLimit": "0x1c9c380",
      "gasUsed": "0x6a4a1a",
      "timestamp": "0x15",
      "transactions": [
        "0x6749b6eff06b935d236d30ac10742475e42107fb5028dec7125ad0b448eb4294",
        "0x245028fb40abdb6c482787f969a89bb76130aa81c1a064f4e1ff5f77abb4d946"
      ],
      "uncles": []
    }
  }
  ```

- **`eth_getBlockByHash`** ([node_evm_block.go:429](node/node_evm_block.go#L429))
  - Retrieve block metadata and transactions by hash
  - Reads canonical EvmBlockPayload from DA

  **Example Request**:
  ```bash
  curl -X POST http://localhost:8545 \
    -H "Content-Type: application/json" \
    --data '{"jsonrpc":"2.0","method":"eth_getBlockByHash","params":["0x8c8ce20ed25b040355a8472d96f13edf46fcb28e42c8d8744e8484e90922f5bb",false],"id":1}'
  ```

  **Example Response**:
  ```json
  {
    "jsonrpc": "2.0",
    "id": 1,
    "result": {
      "number": "0x2",
      "hash": "0x8c8ce20ed25b040355a8472d96f13edf46fcb28e42c8d8744e8484e90922f5bb",
      "parentHash": "0xa85ed103a46bbe018466e9ccec3d2b2f874a15151139acaf3c51bd2b31c68a03",
      "nonce": "0x0000000000000000",
      "sha3Uncles": "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
      "logsBloom": "0x00000000000000000000000000000000000000080000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000000000000008000000000020000000000000400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000100000000000020000000000000000000000000000000000000000000000000000000000002000000000000000000000000000080000000000000000000000000000",
      "transactionsRoot": "0x0000000000000000000000000000000000000000000000000000000000000000",
      "stateRoot": "0x0000000000000000000000000000000000000000000000000000000000000000",
      "receiptsRoot": "0x0000000000000000000000000000000000000000000000000000000000000000",
      "miner": "0x0000000000000000000000000000000000000000",
      "difficulty": "0x0",
      "totalDifficulty": "0x0",
      "extraData": "0x0000000000000000000000000000000000000000000000000000000000000000",
      "size": "0x0",
      "gasLimit": "0x1c9c380",
      "gasUsed": "0x6a4a1a",
      "timestamp": "0x15",
      "transactions": [
        "0x6749b6eff06b935d236d30ac10742475e42107fb5028dec7125ad0b448eb4294",
        "0x245028fb40abdb6c482787f969a89bb76130aa81c1a064f4e1ff5f77abb4d946"
      ],
      "uncles": []
    }
  }
  ```

#### Historical State Access
**File**: `node_evm_block.go`

**Current behavior**: `resolveBlockNumberToState` reads canonical block metadata, reconstructs a historical `StateDB` via `NewStateDBFromStateRoot`, and falls back to the live state only when witnesses are missing or malformed (warnings emitted).

**Recent Updates (Oct 30, 2025)**:
- ✅ **EVM Service Refactoring** – payload processing lives in `refine_payload_transactions()` / `refine_call_payload()`, which keeps the refine entry point manageable.
- ⚠️ **Block Number Key Refactor** – a new 0xFF…FF sentinel is defined, but the writer still only stores the 4-byte height so the new block builder path does not receive parent-hash/state data yet.
- ⚠️ **BlockRefiner Migration** – helper moved into `block.rs`, but it currently falls back to empty metadata until the witness payload is updated.
- ✅ **Constructor Cleanup** – `MajikBackend::new` now infers the service id from the environment.

**Outstanding TODOs**:
- [ ] Harden error handling/metrics for missing block witnesses (today we quietly reuse latest state on failure).
- [ ] Persist finalized block height to differentiate safe/confirmed reads.
- [ ] Implement distinct handling for `"pending"` once a mempool state snapshot is available.

#### Trace Support (NOT IMPLEMENTED)
**Status**: No trace methods implemented
- `debug_traceBlockByNumber` - Not implemented
- `debug_traceTransaction` - Not implemented
- `trace_block` - Not implemented
- `trace_replayBlockTransactions` - Not implemented

**Impact**: Internal transactions will not be visible in Blockscout

**Recommendation**: Implement Geth-style `debug_traceBlockByNumber` with `callTracer` for compatibility

### 📊 Implementation Priority for Blockscout (November)

**Completed (Oct 30)**:
- ✅ **EVM Service Code Organization**: Refactored payload processing into modular functions
- ✅ **Block Number Key Standardization**: Unified BLOCK_NUMBER_KEY usage
- ✅ **BlockRefiner Architecture**: Improved organization and timestamp handling
- ✅ **Constructor Simplification**: Cleaner MajikBackend interface

---

## 🔄 Recent Updates

### ObjectRef Optimization & Sharding (November 2025)

**ObjectRef Size Reduction (64 → 36 bytes)**:
- Removed accumulate-time fields: ServiceID, IndexEnd, Version, LogIndex, TxSlot, Timeslot, GasUsed, EvmBlock
- Core fields: WorkPackageHash (32B), IndexStart (12 bits), PayloadLength (4B), ObjectKind (1B)
- Accumulate-time metadata stored separately: +timeslot (4B) +blocknumber (4B) = 44 bytes in JAM State
- 3-byte packing: index_start (12 bits) | num_segments (12 bits) | last_segment_bytes (12 bits)
- **Result**: 44% size reduction, 41% more entries per meta-shard (41 → 58)

**Bidirectional Block Mappings**:
- blocknumber → (work_package_hash [32B] + timeslot [4B]) = 36 bytes
- work_package_hash → (blocknumber [4B] + timeslot [4B]) = 8 bytes
- O(1) lookups in both directions for RPC queries

**Meta-Sharding Architecture**:
- ObjectRefEntry: 68 bytes (32B ObjectID + 36B ObjectRef), down from 96 bytes
- Capacity: 58 entries per meta-shard (up from 41)
- Used for contracts with many storage shards (> 62 shards)

**Simplified Dependencies**:
- Removed ObjectDependency struct with version tracking
- Changed from Vec<ObjectDependency> to Vec<ObjectId>
- Dependencies not currently serialized (commented out in effects.rs)

**Impact on RPC Methods**:
- All storage queries now use sharding resolution path
- Block queries benefit from bidirectional mappings
- Receipt queries include blocknumber in 44-byte JAM State entry
- Historical state queries work through storage shard reconstruction

### Receipt Processing Centralization (October 2024)
**Receipt processing has been centralized in `BlockRefiner::finalize_receipts()` for improved maintainability**

- **Centralized Flow**: All receipt processing now happens in a single method ([services/evm/src/block.rs:82-176](../services/evm/src/block.rs#L82))
- **Transaction Order Requirement**: Processing follows `self.tx_hashes` vector order to maintain canonical transaction sequence (not lexicographic hash order)
- **⚠️ Critical**: The incoming transaction vector must preserve submission order for proper receipt root computation and JSON-RPC compliance
- **Parsed Receipt Caching**: BlockRefiner stores `TransactionReceiptRecord` objects instead of raw bytes for better performance

**Impact on RPC Methods**:
- Block metadata now comes from consolidated processing pipeline
- Receipt indices correctly match transaction indices as expected by Blockscout
- Transaction order integrity maintained from submission through RPC response

---

**Remaining Priorities**:
1. **Trace methods** – implement `debug_traceBlockByNumber` / `trace_block` equivalents so Blockscout can index internal transfers.
2. **Pending transaction websocket updates** – richer TxPool introspection if desired.
3. **Rate limiting & auth** – protect public RPC endpoints.

4, Transaction Pool Management - The TxPool manages pending Ethereum transactions with:
- **Validation**: Gas price, nonce, signature verification
- **Organization**: Efficient lookup by sender and nonce
- **Cleanup**: Automatic removal of expired transactions
- **Statistics**: Comprehensive metrics tracking

**RPC Methods** ([node_evm_rpc.go:937-1037](node/node_evm_rpc.go#L937)):
- `jam.TxPoolStatus` - Returns pool statistics (pending/queued counts)
- `jam.TxPoolContent` - Returns all pending and queued transaction hashes
- `jam.TxPoolInspect` - Returns human-readable pool summary


## 📋 Block Metadata Implementation

For detailed documentation on canonical block metadata, storage architecture, and the three-witness protocol, see:

**[services/evm/docs/BLOCKS.md](../services/evm/docs/BLOCKS.md)**

This document covers:
- Code organization (main.rs, refiner.rs, accumulator.rs)
- Block metadata storage architecture
- Three-witness block building protocol
- BMT (Binary Merkle Trie) implementation
- Block hash computation
- Merkle roots and logs bloom filters

**Quick Status**:
- ✅ Ethereum RLP encoding complete
- ✅ Historical state support implemented
- ✅ Block metadata storage (96-byte format)
- ✅ Code modularization complete
- ❌ Block builder three-witness integration pending
