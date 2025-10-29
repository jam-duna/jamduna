# JAM Ethereum JSON-RPC API Documentation

This document describes the Ethereum JSON-RPC compatible API implemented in the JAM network, providing seamless integration between Ethereum clients and JAM's State/DA system.

## Overview

The JAM node implements a comprehensive set of Ethereum JSON-RPC methods that allow existing Ethereum tooling (MetaMask, web3.js, ethers.js, etc.) to interact with JAM's blockchain infrastructure. All Ethereum state and transaction data is stored and managed through JAM's Service 0x01 (EVM service) using the JAM State/DA system.

## RPC Endpoint

The Ethereum JSON-RPC methods are available through the JAM node's RPC interface:

```
HTTP: http://localhost:8080/rpc
WebSocket: ws://localhost:8080/ws
```

All methods are accessed via the `jam.` namespace (e.g., `jam.GetBalance`).

## Implemented Methods

### Account Information

#### `GetBalance(address, blockNumber) → uint256`

Fetches the balance of an Ethereum address from JAM State/DA.

**Parameters:**
- `address` (string): 20-byte Ethereum address (hex-encoded with 0x prefix)
- `blockNumber` (string): Block number ("latest", "earliest", "pending", or hex number)

**Returns:**
- `string`: Balance in Wei as hex-encoded uint256

**Implementation:**
- Reads from USDM contract (0x01) storage
- Computes storage key using keccak256(abi.encode(address, slot)) where slot=0 for balances
- Uses ReadObject abstraction to fetch SSR payload from DA
- Parses shard data to find balance value
- Returns 0x0 if address has no balance record

**Example:**
```json
{
  "method": "jam.GetBalance",
  "params": ["0x742d35Cc6634C0532925a3b8D4f3C8C0df2c2F17", "latest"]
}
```

#### `GetTransactionCount(address, blockNumber) → uint256`

Fetches the nonce (transaction count) of an Ethereum address from JAM State/DA.

**Parameters:**
- `address` (string): 20-byte Ethereum address (hex-encoded with 0x prefix)
- `blockNumber` (string): Block number ("latest", "earliest", "pending", or hex number)

**Returns:**
- `string`: Nonce as hex-encoded uint256

**Implementation:**
- Reads from USDM contract (0x01) storage
- Computes storage key using keccak256(abi.encode(address, slot)) where slot=1 for nonces
- Uses ReadObject abstraction to fetch SSR payload from DA
- Parses shard data to find nonce value
- Returns 0x0 if address has no nonce record

**Example:**
```json
{
  "method": "jam.GetTransactionCount",
  "params": ["0x742d35Cc6634C0532925a3b8D4f3C8C0df2c2F17", "latest"]
}
```

### Transaction Execution

#### `Call(transactionObject, blockNumber) → data`

Simulates a transaction execution without submitting it to the network.

**Parameters:**
- `transactionObject` (object): Transaction call object with fields:
  - `from` (optional): Sender address
  - `to` (optional): Recipient address (null for contract creation)
  - `gas` (optional): Gas limit (default: 21000)
  - `gasPrice` (optional): Gas price in Wei (default: 1 Gwei)
  - `value` (optional): Value in Wei (default: 0)
  - `data` (optional): Transaction data (hex-encoded)
- `blockNumber` (string): Block number for state context

**Returns:**
- `string`: Execution result data (hex-encoded)

**Implementation:**
- Parses transaction object into internal format
- Creates read-only EVM execution context using MajikBackend
- Returns execution result without state changes

**Example:**
```json
{
  "method": "jam.Call",
  "params": [
    {
      "to": "0x742d35Cc6634C0532925a3b8D4f3C8C0df2c2F17",
      "data": "0x70a08231000000000000000000000000742d35cc6634c0532925a3b8d4f3c8c0df2c2f17"
    },
    "latest"
  ]
}
```

#### `SendRawTransaction(signedTxData) → txHash`

Submits a signed transaction to the JAM guarantor mempool.

**Parameters:**
- `signedTxData` (string): RLP-encoded signed transaction (hex-encoded with 0x prefix)

**Returns:**
- `string`: Transaction hash (32-byte hex string)

**Implementation:**
- Parses and validates the signed transaction
- Recovers sender address from signature
- Adds transaction to the guarantor TxPool
- Returns transaction hash for tracking

**Example:**
```json
{
  "method": "jam.SendRawTransaction",
  "params": ["0xf86c808504a817c800825208943535353535353535353535353535353535353535880de0b6b3a76400008025a04f4c17305743700648bc4f6cd3038ec6f6af0df73e31757d0bf5fd1c2c73e4e5a0632b7e71b83e4c2f4dab7e8c9b92a8e3e3b3a7a5b5c5d5e5f5a5b5c5d5e5f5"]
}
```

### Transaction Information

#### `GetTransactionReceipt(txHash) → receipt`

Fetches the receipt of a transaction from JAM State/DA.

**Parameters:**
- `txHash` (string): 32-byte transaction hash (hex-encoded with 0x prefix)

**Returns:**
- `object`: Transaction receipt with fields:
  - `transactionHash`: Transaction hash
  - `transactionIndex`: Index in block
  - `blockHash`: Containing block hash
  - `blockNumber`: Containing block number
  - `from`: Sender address
  - `to`: Recipient address (null for contract creation)
  - `cumulativeGasUsed`: Total gas used in block
  - `gasUsed`: Gas used by this transaction
  - `contractAddress`: Created contract address (null if not creation)
  - `logs`: Array of event logs
  - `logsBloom`: Bloom filter for logs
  - `status`: "0x1" for success, "0x0" for failure
  - `effectiveGasPrice`: Actual gas price used
  - `type`: Transaction type ("0x0" for legacy)

**Implementation:**
- Uses tx_to_objectID to compute receipt ObjectID from txHash
- Calls ReadObject to fetch receipt payload from DA
- Parses receipt format: [1B success] [8B gas] [4B payload_len] [payload] [4B logs_len] [logs]
- Extracts transaction details, contract address, and logs
- Returns null if transaction not found

#### `GetTransactionByHash(txHash) → transaction`

Fetches transaction details by hash from JAM State/DA.

**Parameters:**
- `txHash` (string): 32-byte transaction hash (hex-encoded with 0x prefix)

**Returns:**
- `object`: Transaction object with fields:
  - `hash`: Transaction hash
  - `nonce`: Sender nonce
  - `blockHash`: Containing block hash (null if pending)
  - `blockNumber`: Containing block number (null if pending)
  - `transactionIndex`: Index in block (null if pending)
  - `from`: Sender address
  - `to`: Recipient address (null for contract creation)
  - `value`: Value transferred in Wei
  - `gasPrice`: Gas price in Wei
  - `gas`: Gas limit
  - `input`: Transaction data
  - `v`, `r`, `s`: Signature components

**Implementation:**
- Uses tx_to_objectID to compute receipt ObjectID from txHash
- Calls ReadObject to fetch receipt payload from DA
- Parses receipt to extract original transaction details
- Returns null if transaction not found

### Storage Access

#### `GetStorageAt(address, position, blockNumber) → value`

Fetches storage value at a specific position for a contract address.

**Parameters:**
- `address` (string): 20-byte contract address (hex-encoded with 0x prefix)
- `position` (string): Storage position (32-byte hex-encoded uint256)
- `blockNumber` (string): Block number for state context

**Returns:**
- `string`: Storage value (32-byte hex-encoded)

**Implementation:**
- Resolves block number to StateDB for historical reads
- Computes ObjectID for contract using ssr_to_objectID
- Uses ReadObject abstraction to fetch contract SSR payload from DA
- Parses shard data format: [2B count] + [entries: 32B key_hash + 32B value] * count
- Hashes the storage key and searches for matching entry
- Returns zero-padded 32-byte value if not found

### Block Information

#### `GetBlockByHash(blockHash, fullTx) → block`

Fetches a JAM block with all EVM transactions across cores.

**Parameters:**
- `blockHash` (string): 32-byte block hash (hex-encoded with 0x prefix)
- `fullTx` (boolean): Include full transaction objects (true) or just hashes (false)

**Returns:**
- `object`: Block object or `null` if not found

**Status:** Placeholder implementation (returns null)

#### `GetBlockByNumber(blockNumber, fullTx) → block`

Fetches a JAM block by number with all EVM transactions across cores.

**Parameters:**
- `blockNumber` (string): Block number ("latest", "earliest", "pending", or hex number)
- `fullTx` (boolean): Include full transaction objects (true) or just hashes (false)

**Returns:**
- `object`: Block object or `null` if not found

**Status:** Placeholder implementation (returns null)

### Event Logs

#### `GetLogs(filterObject) → logs[]`

Fetches event logs matching filter criteria from JAM State/DA.

**Parameters:**
- `filterObject` (object): Log filter with fields:
  - `fromBlock` (optional): Starting block number
  - `toBlock` (optional): Ending block number
  - `address` (optional): Contract address(es) to filter
  - `topics` (optional): Topic filter array

**Returns:**
- `array`: Array of matching log objects with fields:
  - `address`: Contract address that emitted the log
  - `topics`: Array of indexed log topics (32-byte hex strings)
  - `data`: Non-indexed log data
  - `blockNumber`: Block number
  - `transactionHash`: Transaction hash
  - `transactionIndex`: Transaction index in block
  - `blockHash`: Block hash
  - `logIndex`: Log index in block
  - `removed`: Always false (no chain reorganizations in JAM)

**Implementation:**
- Resolves fromBlock and toBlock to block numbers
- Iterates through block range, reading transaction hashes from block metadata
- For each transaction, uses ReadObject to fetch receipt and extract logs
- Filters logs by address and topics according to filter criteria
- Returns empty array if no logs match or blocks not found

## Transaction Pool Management

Additional methods for managing the guarantor transaction pool:

### `TxPoolStatus() → statistics`

Returns current transaction pool statistics.

**Returns:**
- `object`: Pool statistics including pending/queued counts and totals

### `TxPoolContent() → content`

Returns all pending and queued transactions in the pool.

**Returns:**
- `object`: Object containing `pending` and `queued` transaction arrays

### `TxPoolInspect() → summary`

Returns human-readable transaction pool summary.

**Returns:**
- `string`: Formatted summary of pool status and recent transactions

## Architecture Integration

### JAM State/DA Integration

All Ethereum state is stored in JAM's Service 0x01 using the following architecture:

#### Object Types (DA Storage)
- **Receipt Objects** (kind=0x03): Transaction receipts with gas usage, logs, and contract addresses
  - ObjectID: `tx_to_objectID(txHash)` = `keccak256("tx:" || txHash)`
  - Format: `[1B success] [8B gas] [4B payload_len] [payload] [4B logs_len] [logs]`

- **SSR (Shard State Reference) Objects** (kind=0x01): Contract storage shards
  - ObjectID: `ssr_to_objectID(contractAddress)` = `keccak256("ssr:" || address)`
  - Format: `[2B count] + [entries: 32B key_hash + 32B value] * count`

- **ObjectRef Metadata**: Maps ObjectID → DA location
  - Stored in service storage with ObjectID as key
  - Contains: WorkPackageHash, IndexStart, IndexEnd, PayloadLength

#### Storage Layout (USDM Contract at 0x01)
- **Balances**: `keccak256(abi.encode(address, 0))` → balance (uint256)
- **Nonces**: `keccak256(abi.encode(address, 1))` → nonce (uint256)
- **Contract Storage**: Storage keys are hashed and stored in SSR shards

#### Read Path
1. **ReadObjectRef**: Lookup ObjectRef metadata from service storage
2. **FetchJAMDASegments**: Fetch actual payload from DA using WorkPackageHash and segment range
3. **Parse Payload**: Deserialize object-specific format to extract data

### Block Number Resolution

The system supports standard Ethereum block number formats:
- `"latest"`: Current finalized state
- `"earliest"`: Genesis block state
- `"pending"`: Current state plus pending transactions
- `"0x..."`: Specific block number (hex-encoded)

### Transaction Pool (Guarantor Mempool)

The TxPool manages pending Ethereum transactions:
- **Validation**: Gas price, nonce, signature verification
- **Organization**: Efficient lookup by sender and nonce
- **Cleanup**: Automatic removal of expired transactions
- **Statistics**: Comprehensive metrics tracking

## Code Organization

The EVM RPC implementation is organized across multiple files for maintainability:

### `node_evm_rpc.go`
RPC method wrappers that handle JSON-RPC parameter parsing and response formatting. Organized into sections:
- **Network Metadata**: ChainId, Accounts, GasPrice
- **Contract State**: GetBalance, GetStorageAt, GetTransactionCount, GetCode
- **Transaction Operations**: EstimateGas, Call, SendRawTransaction
- **Transaction Queries**: GetTransactionReceipt, GetTransactionByHash, GetLogs
- **Block Queries**: GetBlockByHash, GetBlockByNumber
- **Helper Functions**: parseBlockParameter, isZeroAddress, min

All RPC methods parse JSON parameters and call typed NodeContent methods.

### `node_evm_tx.go`
Transaction-related internal methods in NodeContent:
- **EthereumTransaction**: Core transaction type for EVM operations
- **EthereumTransactionResponse**: JSON-RPC transaction response format
- **Transaction Parsing**: convertPayloadToEthereumTransaction
- **Transaction Operations**: EstimateGas, Call, SendRawTransaction
- **Transaction Queries**: GetTransactionReceipt, GetTransactionByHash
- **Work Package Creation**: createSimulationWorkPackage, executeSimulationWorkPackage
- **Helper Functions**: tx_to_objectID, boolToHexStatus

### `node_evm_logsreceipt.go`
Logs and receipt-related types and methods:
- **TransactionReceipt**: Internal receipt representation
- **EthereumTransactionReceipt**: JSON-RPC receipt format
- **EthereumLog**: Event log representation
- **LogFilter**: Filter criteria for eth_getLogs
- **Receipt Parsing**: parseRawReceipt
- **Log Operations**: GetLogs, getLogsFromBlock, matchesLogFilter, matchesTopicsFilter
- **Log Parsing**: parseLogsFromReceipt, calculateLogsBloom

### `node_evm_block.go`
Block-related operations:
- **EthereumBlock**: JSON-RPC block format
- **Block Operations**: GetBlockByHash, GetBlockByNumber
- **Block Construction**: convertToEthereumBlock, readBlockFromStorage
- **Block Metadata**: extractBlockMetadata

### `node_evm_contracts.go`
Contract state access methods:
- **Storage Operations**: GetBalance, GetStorageAt, GetTransactionCount, GetCode
- **SSR Integration**: ssr_to_objectID, parseSSRPayload
- **USDM Integration**: balanceStorageKey, nonceStorageKey

## Implementation Status

### ✅ Completed

1. **Storage Read Operations** (`node_evm_contracts.go`)
   - ✅ GetBalance - reads from USDM contract via SSR shards
   - ✅ GetTransactionCount - reads nonces from USDM contract
   - ✅ GetStorageAt - reads contract storage with historical state support
   - ✅ ReadObject/ReadObjectRef abstraction for DA access
   - ✅ SSR payload parsing with shard format support

2. **Transaction Receipts** (`node_evm_logsreceipt.go`, `node_evm_tx.go`)
   - ✅ GetTransactionReceipt - full receipt with logs and gas data
   - ✅ GetTransactionByHash - transaction details from receipt
   - ✅ Receipt parsing with success status, gas, payload, and logs

3. **Event Logs** (`node_evm_logsreceipt.go`)
   - ✅ GetLogs - filter logs by block range, address, and topics
   - ✅ Log extraction from receipts with proper indexing
   - ✅ Topic-based filtering support

4. **Transaction Operations** (`node_evm_tx.go`)
   - ✅ EstimateGas - estimates gas via work package simulation
   - ✅ Call - simulates transaction execution
   - ✅ SendRawTransaction - submits transactions to mempool

5. **Type Safety Improvements**
   - ✅ Use `common.Hash` for ObjectID, storage keys, and values
   - ✅ Use `uint64` for storage slots in Solidity mappings
   - ✅ Global USDM address constant
   - ✅ Concrete return types instead of interface{}

### 🚧 In Progress / Outstanding

1. **Block Information** (`node_rpc_evmblock.go`)
   - ⏳ GetBlockByHash - placeholder implementation (returns null)
   - ⏳ GetBlockByNumber - placeholder implementation (returns null)
   - Need: Block metadata storage and aggregation across cores

2. **Transaction Execution**
   - ⏳ Call - simulation needs actual EVM execution
   - ⏳ SendRawTransaction - needs integration with guarantor mempool

3. **Historical State Access**
   - ⏳ resolveBlockNumberToState() for specific block numbers
   - Currently only supports "latest"
   - Need: Historical StateDB snapshots

4. **Transaction Pool Integration**
   - ⏳ TxPool connection to RPC methods
   - ⏳ Mempool management and validation
   - ⏳ Transaction replacement and eviction policies

## Outstanding TODOs by File

### `node_rpc_evmblock.go`

**Block Construction:**
- [ ] Line 127: Calculate actual gas used for blocks (currently returns "0x0")
- [ ] Line 214: Implement block fetching by hash (currently returns null)
- [ ] Line 377: Add GetQueuedTransactions method to TxPool

**Historical State Access:**
- [ ] Line 464: Implement earliest block state retrieval
- [ ] Line 468: Implement pending state (current state + pending transactions)
- [ ] Line 479: Implement historical state retrieval by specific block number

**Requirements:**
- Block metadata storage format in service storage
- Aggregation of transactions across multiple cores
- Block hash computation from JAM block data
- Mapping between JAM block numbers and Ethereum block numbers

### `node_rpc_evmtx.go`

**Transaction Details:**
- [ ] Line 264: Extract nonce from transaction payload (currently returns "0x0")
- [ ] Line 274-276: Extract signature components (v, r, s) from payload (currently returns "0x0")
- [ ] Line 318: Get actual block information instead of placeholder values
- [ ] Line 341: Implement proper contract address calculation using CREATE opcode logic
- [ ] Line 354: Calculate cumulative gas used across transactions in block

**Log Parsing:**
- [ ] Line 369: Implement actual log parsing based on EVM service's log format
- [ ] Line 376: Implement bloom filter calculation for logs
- [ ] Line 488: Use real block hash instead of placeholder

**Work Package Execution:**
- [ ] Line 955: Extract actual return value from work report results

**Requirements:**
- Parse EVM service log format from receipt LogsData
- Implement Ethereum logs bloom filter (256-byte bitmap)
- Store and retrieve block metadata (hash, number, timestamp)
- Extract return data from work package execution results
- Calculate contract addresses: `keccak256(rlp([sender, nonce]))[12:]`

### Missing Implementations

**Contract Address Calculation:**
```go
// TODO: Implement proper contract address derivation
// For CREATE: address = keccak256(rlp([sender, nonce]))[12:]
// For CREATE2: address = keccak256(0xff || sender || salt || keccak256(initCode))[12:]
```

**Logs Bloom Filter:**
```go
// TODO: Implement Ethereum logs bloom filter
// 256-byte bloom filter with 3 hash functions per log entry
// Bloom addresses and first 4 topics of each log
```

**Signature Extraction:**
```go
// TODO: Parse signature from transaction payload
// Need to determine where v, r, s are stored in receipt payload
// May need to store original signed transaction separately
```

**Block Metadata Schema:**
```go
// TODO: Design block metadata storage
// Proposed format in service storage:
// Key: "block:" || block_number (8 bytes, big-endian)
// Value: [block_hash(32)] [timestamp(8)] [tx_count(4)] [tx_hashes...]
```

## Implementation Priorities

### High Priority (Blocking Core Functionality)

1. **Log Parsing** - Required for GetLogs and GetTransactionReceipt to return actual logs
   - Parse EVM service log format from LogsData bytes
   - Extract address, topics, and data for each log entry
   - File: `node_rpc_evmtx.go:369`

2. **Block Metadata Storage** - Required for GetBlockByNumber/Hash
   - Design storage schema for block metadata
   - Store block hash, timestamp, and transaction list per block
   - Files: `node_rpc_evmblock.go:214,127`

3. **Historical State Access** - Required for block-specific queries
   - Implement StateDB snapshots by block number
   - Support "earliest", "pending", and hex block numbers
   - File: `node_rpc_evmblock.go:464,468,479`

### Medium Priority (Enhanced Functionality)

4. **Contract Address Calculation** - Required for contract creation receipts
   - Implement CREATE and CREATE2 address derivation
   - File: `node_rpc_evmtx.go:341`

5. **Cumulative Gas Tracking** - Required for accurate receipts
   - Track gas usage across transactions in a block
   - File: `node_rpc_evmtx.go:354`

6. **Signature Extraction** - Required for complete transaction responses
   - Parse v, r, s from transaction payload or store separately
   - Files: `node_rpc_evmtx.go:264,274-276`

### Low Priority (Nice to Have)

7. **Logs Bloom Filter** - Optional optimization for log filtering
   - Implement 256-byte bloom filter calculation
   - File: `node_rpc_evmtx.go:376`

8. **Work Report Result Extraction** - Required for Call return values
   - Parse return data from work package execution
   - File: `node_rpc_evmtx.go:955`

9. **TxPool Queued Transactions** - Required for complete pool inspection
   - Add method to retrieve queued (non-executable) transactions
   - File: `node_rpc_evmblock.go:377`

## Error Handling

All methods follow Ethereum JSON-RPC error conventions:

- **-32700**: Parse error (invalid JSON)
- **-32600**: Invalid request
- **-32601**: Method not found
- **-32602**: Invalid parameters
- **-32603**: Internal error

Custom error codes:
- **-32000**: Server error (JAM State/DA access failure)
- **-32001**: Transaction rejected (validation failure)
- **-32002**: Resource not found (address/transaction/block)

## Performance Considerations

- **Caching**: Implement LRU cache for frequently accessed state
- **Batching**: Support batch RPC requests for efficiency
- **Indexing**: Create indices for address balances and transaction lookup
- **Pagination**: Implement pagination for large result sets (logs, transactions)

## Security Considerations

- **Input Validation**: Strict validation of all hex-encoded parameters
- **Gas Limits**: Enforce execution limits to prevent DoS attacks
- **Rate Limiting**: Implement per-client request rate limiting
- **Authentication**: Consider authentication for sensitive operations

## Testing

Comprehensive test coverage includes:
- Unit tests for all RPC methods
- Integration tests with JAM State/DA
- Load testing for transaction pool
- Compatibility testing with Ethereum tools

## Migration Path

For existing Ethereum applications:
1. Update RPC endpoint to JAM node
2. Use `jam.` namespace for method calls
3. No changes required to transaction signing or encoding
4. Monitor for any JAM-specific behaviors or limitations