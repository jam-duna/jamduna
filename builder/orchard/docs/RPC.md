# Zcash JSON-RPC Reference (JAM Orchard Service)

This document describes the **complete Zcash-compatible JSON-RPC API** for JAM's Orchard service, supporting **both Orchard (shielded) and transparent transactions**.

- **Transport**: HTTP JSON-RPC 1.0
- **Default Port**: 8232
- **Auth**: None (unauthenticated for development; production deployments should use reverse proxy auth)
- **Implementation**: [builder/orchard/rpc/handler.go](../rpc/handler.go)

---

## API Categories

| Category | Methods | Description |
|----------|---------|-------------|
| **[Blockchain](#blockchain-rpcs-shared)** | `getblockchaininfo`, `getbestblockhash`, `getblockcount`, `getblockhash`, `getblock`, `getblockheader`, `z_getblockchaininfo` | Chain state queries (shared by both pools) |
| **[Orchard Tree](#orchard-tree-state-rpcs)** | `z_gettreestate`, `z_getsubtreesbyindex`, `z_getnotescount` | Orchard commitment tree and witness data |
| **[Orchard Wallet](#orchard-wallet-rpcs)** | `z_getnewaddress`, `z_listaddresses`, `z_validateaddress`, `z_getbalance`, `z_listunspent`, `z_listreceivedbyaddress`, `z_listnotes` | Orchard shielded address and note management |
| **[Orchard Transactions](#orchard-transaction-rpcs)** | `z_sendmany`, `z_sendmanywithchangeto`, `z_sendraworchardbundle`, `z_viewtransaction` | Orchard shielded transaction construction and viewing |
| **[Orchard Mempool](#orchard-mempool-rpcs)** | `z_getmempoolinfo`, `z_getrawmempool` | Orchard shielded mempool queries |
| **[Transparent Transactions](#transparent-transaction-rpcs)** | `getrawtransaction`, `sendrawtransaction` | Transparent transaction queries and broadcast |
| **[Transparent Mempool](#transparent-mempool-rpcs)** | `getmempoolinfo`, `getrawmempool` | Transparent mempool queries |

---

## Implementation Status

### ✅ **JAM Integration Complete**
- Orchard bundle processing via JAM refine/accumulate
- Transparent transaction verification via UTXO Merkle proofs
- Tree state reads from JAM service storage
- Builder-sponsored transaction submission
- Witness-based execution model integration

### ✅ **Dual Pool Architecture**
- **Orchard Pool**: Privacy-preserving shielded transactions (note-based)
- **Transparent Pool**: UTXO-based transactions with visible amounts/addresses
- Work package integration for both pools via expanded payload format

### ⚠️ **Security warning (malicious builder)**
- Transparent RPC methods (`getrawtransaction`, `sendrawtransaction`, mempool queries) depend on **builder-supplied UTXO roots/proofs** (optional snapshots) that are only checked against the claimed pre-state root/size. There is **no JAM witness or genesis anchor** for the transparent UTXO set yet, so an untrusted builder could fabricate a compatible snapshot/root and create spendable inputs. Treat transparent RPCs as **experimental and trusted-builder-only** until the UTXO root/size is anchored in JAM state.

### 🚧 **Orchard FFI Integration** (In Progress)
- Basic FFI stub functions implemented
- Need full `orchard` crate integration for proof generation
- Bundle verification and address derivation pending

---

## Blockchain RPCs (Shared)

These methods query chain state and work for both Orchard and transparent transactions.

### getblockchaininfo
Returns information about the Orchard-enabled JAM blockchain.

```json
// Request
{"method": "getblockchaininfo", "params": [], "id": 1}

// Response
{
  "result": {
    "chain": "orchard",
    "blocks": 150000,
    "headers": 150000,
    "upgrades": {
      "orchard": {
        "activationheight": 1,
        "status": "active"
      }
    }
  }
}
```

### z_getblockchaininfo
Extended blockchain info with Orchard-specific data.

```json
// Request
{"method": "z_getblockchaininfo", "params": [], "id": 1}

// Response
{
  "result": {
    "blocks": 150000,
    "orchard": {
      "commitmentTreeSize": 45000,
      "valuePools": {
        "chainValue": 1.5
      }
    }
  }
}
```

### getblock
Returns block information including Orchard bundles.

```json
// Request
{"method": "getblock", "params": ["0x1a2b3c4d...", 1], "id": 1}

// Response
{
  "result": {
    "hash": "0x1a2b3c4d...",
    "height": 150000,
    "version": 1,
    "time": 1735571200,
    "tx": [
      {
        "txid": "0x5a6b7c8d...",
        "orchard": {
          "actions": [
            {
              "cv": "0x1234567890abcdef...",  // Value commitment
              "nf": "0xabcdef1234567890...",  // Nullifier
              "rk": "0xfedcba0987654321...",  // Randomized verification key
              "cmx": "0x0fedcba987654321...", // Extracted note commitment
              "encCiphertext": "0x...",       // Encrypted note
              "outCiphertext": "0x..."        // Output description
            }
          ],
          "flags": 3,               // spends_enabled | outputs_enabled
          "valueBalance": 0,        // Net value in/out of Orchard pool
          "anchor": "0x9876543210fedcba...",
          "proof": "0x...",         // Orchard bundle proof
          "bindingSig": "0x..."     // Bundle binding signature
        }
      }
    ]
  }
}
```

---

## Orchard Tree State RPCs

These methods provide access to the Orchard commitment tree for light client witness building.

### z_gettreestate
Returns the current Orchard commitment tree state.

**Parameters**:
1. `height` (number, required) - Block height (currently ignored, returns latest state)

```json
// Request
{"method": "z_gettreestate", "params": [150000], "id": 1}

// Response
{
  "result": {
    "orchard": {
      "commitmentTreeSize": 45000,
      "commitmentTreeRoot": "0x9876543210fedcba..."
    }
  }
}
```

### z_getsubtreesbyindex
Returns Orchard subtree roots for light client witness building.

```json
// Request
{"method": "z_getsubtreesbyindex", "params": ["orchard", 0, 10], "id": 1}

// Response
{
  "result": {
    "pool": "orchard",
    "start_index": 0,
    "subtrees": [
      {
        "root": "0x1234567890abcdef...",
        "height": 16,  // Subtree height within the main tree
        "index": 0,
        "commitments": [
          "0xaaaaaaaa...",
          "0xbbbbbbbb..."
        ]
      },
      // ... up to 10 subtrees
    ]
  }
}
```

---

## Orchard Wallet RPCs

These methods manage Orchard shielded addresses and notes.

### z_getnewaddress
Generates a new Orchard diversified address.

```json
// Request
{"method": "z_getnewaddress", "params": ["orchard"], "id": 1}

// Response
{
  "result": "u1abcdef1234567890fedcba9876543210abcdef1234567890fedcba9876543210abcdef12"
}
```

### z_validateaddress
Validates an Orchard address format.

```json
// Request
{"method": "z_validateaddress", "params": ["u1abcdef1234567890..."], "id": 1}

// Response
{
  "result": {
    "isvalid": true,
    "type": "orchard",
    "diversifier": "1234567890abcdef",
    "diversifiedtransmissionkey": "0xfedcba9876543210..."
  }
}
```

### z_getbalance
Returns the total balance for Orchard addresses.

```json
// Request
{"method": "z_getbalance", "params": ["*", 1], "id": 1}

// Response
{
  "result": 1.5  // Balance in USDx (converted from zatoshi)
}
```

### z_listunspent
Lists unspent Orchard notes.

```json
// Request
{"method": "z_listunspent", "params": [1, 9999999, ["u1abcdef..."], false], "id": 1}

// Response
{
  "result": [
    {
      "txid": "0x5a6b7c8d...",
      "pool": "orchard",
      "amount": 0.5,
      "amountZat": 50000000,
      "memo": "",
      "change": false,
      "spendable": true,
      "address": "u1abcdef1234567890...",
      "actionIndex": 0,
      "confirmations": 150
    }
  ]
}
```

### z_listreceivedbyaddress
List notes received by a specific Orchard address.

**Parameters**:
1. `address` (string, required) - Orchard address
2. `minconf` (number, optional, default=1) - Minimum confirmations

**Returns**: Array of received notes

```json
// Request
{"method": "z_listreceivedbyaddress", "params": ["u1abcdef...", 1], "id": 1}

// Response
{
  "result": [
    {
      "txid": "0x5a6b7c8d...",
      "amount": 0.5,
      "memo": "Payment received",
      "confirmations": 150,
      "blockheight": 149850
    }
  ]
}
```

### z_listnotes
List all Orchard notes in the wallet.

**Parameters**:
1. `minconf` (number, optional, default=1) - Minimum confirmations
2. `maxconf` (number, optional, default=9999999) - Maximum confirmations

**Returns**: Array of notes with full details

```json
// Request
{"method": "z_listnotes", "params": [1, 9999999], "id": 1}

// Response
{
  "result": [
    {
      "txid": "0x5a6b7c8d...",
      "pool": "orchard",
      "actionIndex": 0,
      "address": "u1abcdef...",
      "amount": 0.5,
      "amountZat": 50000000,
      "memo": "",
      "confirmations": 150,
      "spendable": true,
      "change": false
    }
  ]
}
```

---

## Orchard Transaction RPCs

These methods handle Orchard shielded transaction construction, submission, and viewing.

### z_sendmany
Sends Orchard transactions to multiple recipients.

```json
// Request
{
  "method": "z_sendmany",
  "params": [
    "u1abcdef1234567890...",  // From address
    [
      {
        "address": "u1fedcba9876543210...",  // To address
        "amount": 0.1,
        "memo": "Payment for services"
      }
    ],
    1,        // Min confirmations
    0.00001   // Fee
  ],
  "id": 1
}

// Response
{
  "result": "opid-12345678-abcd-ef90-1234-567890abcdef"  // Operation ID
}
```

### z_sendmanywithchangeto
Send Orchard transaction with explicit change address.

```json
// Request
{
  "method": "z_sendmanywithchangeto",
  "params": [
    "u1abcdef1234567890...",  // From address
    [
      {"address": "u1fedcba9876543210...", "amount": 0.1}
    ],
    1,        // Min confirmations
    0.00001,  // Fee
    "u1change1234567890..."   // Change address
  ],
  "id": 1
}

// Response
{
  "result": "opid-12345678-abcd-ef90-1234-567890abcdef"
}
```

### z_sendraworchardbundle
Submit a pre-built Orchard bundle (for client-side bundle construction).

**Parameters**:
1. `bundle_hex` (string, required) - Serialized Orchard bundle in hex format

**Returns**: Transaction ID (string)

```json
// Request
{
  "method": "z_sendraworchardbundle",
  "params": ["01abcdef..."],
  "id": 1
}

// Response
{
  "result": "0x9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1a0b9c8d7e6f5a4b3c2d1e0f9a8b"
}
```

### z_viewtransaction
View details of an Orchard transaction.

**Parameters**:
1. `txid` (string, required) - Transaction ID

**Returns**: JSON object with transaction details including actions, value balance, and memo data

```json
// Request
{
  "method": "z_viewtransaction",
  "params": ["0x9a8b7c6d..."],
  "id": 1
}

// Response
{
  "result": {
    "txid": "0x9a8b7c6d...",
    "orchard": {
      "actions": [
        {
          "spend": {
            "cv": "0x1234567890abcdef...",
            "nullifier": "0xabcdef1234567890...",
            "rk": "0xfedcba0987654321..."
          },
          "output": {
            "cv": "0x1234567890abcdef...",
            "cmu": "0x0fedcba987654321...",
            "ephemeralKey": "0x...",
            "encCiphertext": "0x...",
            "outCiphertext": "0x..."
          }
        }
      ],
      "valueBalance": 0,
      "anchor": "0x9876543210fedcba...",
      "bindingSig": "0x..."
    }
  }
}
```

**Note**: `z_sendmany` and `z_sendmanywithchangeto` currently return synchronously (not via operation IDs), so `z_getoperationstatus` is not yet implemented.

---

## Orchard Mempool RPCs

These methods query the Orchard shielded transaction mempool.

### z_getmempoolinfo
Returns statistics about the Orchard mempool.

```json
// Request
{"method": "z_getmempoolinfo", "params": [], "id": 1}

// Response
{
  "result": {
    "size": 15,              // Number of transactions
    "bytes": 45000,          // Total size in bytes
    "usage": 45000,          // Memory usage
    "maxmempool": 300000000  // Max mempool size
  }
}
```

### z_getrawmempool
Query Orchard mempool contents.

**Parameters**:
1. `verbose` (boolean, optional, default=false) - Return detailed info if true

```json
// Request (non-verbose)
{"method": "z_getrawmempool", "params": [false], "id": 1}

// Response
{
  "result": [
    "0x9a8b7c6d...",
    "0x1a2b3c4d..."
  ]
}

// Request (verbose)
{"method": "z_getrawmempool", "params": [true], "id": 1}

// Response
{
  "result": {
    "0x9a8b7c6d...": {
      "size": 3000,
      "time": 1735571200,
      "height": 150000
    }
  }
}
```

---

## JAM Integration Specifics

### Builder Submission Flow
When `z_sendmany` is called:

1. **Bundle Construction**: Build Orchard bundle with actions for spends/outputs
2. **Proof Generation**: Generate Halo2 proof via Orchard FFI
3. **JAM Submission**: Submit `SubmitOrchard` extrinsic to JAM builders
4. **Guarantor Verification**: JAM guarantors verify bundle via refine process
5. **State Updates**: Commitment tree and nullifier updates via accumulate

### Witness-Based Execution
- RPC methods read from JAM service storage using witnesses
- Tree state comes from verified JAM state roots
- No local state maintained beyond what's needed for light client operation

### Builder Privacy Model
- Users submit to RPC without revealing identity
- Builder pays JAM execution costs
- Fee reimbursement handled by JAM service accounting

---

## Error Handling

### Standard Error Codes
```json
// Invalid address format
{
  "error": {
    "code": -5,
    "message": "Invalid Orchard address format"
  }
}

// Insufficient funds
{
  "error": {
    "code": -6,
    "message": "Insufficient confirmed funds"
  }
}

// FFI proof generation failure
{
  "error": {
    "code": -4,
    "message": "Bundle proof generation failed"
  }
}
```

### JAM-Specific Errors
```json
// Builder submission failure
{
  "error": {
    "code": -32000,
    "message": "JAM builder submission failed",
    "data": "Network timeout or builder rejection"
  }
}

// Witness verification failure
{
  "error": {
    "code": -32001,
    "message": "State witness verification failed",
    "data": "Merkle proof invalid or state root mismatch"
  }
}
```

---

## Implementation TODOs

### High Priority
1. **Complete FFI Integration** (2-3 weeks)
   - Replace address generation stubs with real Orchard key derivation
   - Implement bundle proof generation using `orchard` crate
   - Add bundle verification for incoming transactions

2. **Performance Optimization** (1-2 weeks)
   - Cache proving/verifying keys for faster proof generation
   - Implement subtree root caching for light client efficiency
   - Optimize note scanning and witness generation

### Medium Priority
3. **Advanced Features** (3-4 weeks)
   - Hardware wallet integration for key management
   - Batch transaction support for higher throughput
   - Advanced memo and metadata handling

4. **Testing and Compatibility** (2 weeks)
   - Full Zcash RPC compatibility test suite
   - Performance benchmarks against zcashd
   - Integration tests with real Orchard test vectors

---

## Transparent Transaction RPCs

### Overview

The Orchard service provides **Zcash-compatible transparent transaction RPCs** for web wallet integration. These methods emulate a Zcash full-node interface, enabling wallets to broadcast and query transparent transactions without code changes.

**Implementation**: [handler.go](../rpc/handler.go), [transparent_tx.go](../rpc/transparent_tx.go)

**Status**: ✅ Fully Implemented (2026-01-04)

**State binding (anti-malicious-builder)**: Even if a work package carries no `TransparentTxData` extrinsic (no transparent transactions in the block), validators emit no-op transparent state transitions so accumulate checks the builder-supplied transparent roots/sizes. Builders cannot zero these fields to sidestep transparent verification.

---

### `getrawtransaction`

Retrieve a raw transaction by txid.

**Parameters**:
1. `txid` (string, required) - Transaction ID
2. `verbose` (boolean, optional, default=false) - Return JSON object if true

**Returns**:
- **Non-verbose**: Raw transaction hex string
- **Verbose**: JSON object with transaction details

**Example Request**:
```json
{
  "jsonrpc": "1.0",
  "method": "getrawtransaction",
  "params": ["abcd1234...", false],
  "id": 1
}
```

**Example Response** (non-verbose):
```json
{
  "result": "0100000001abcd...",
  "error": null,
  "id": 1
}
```

**Example Response** (verbose):
```json
{
  "result": {
    "txid": "abcd1234...",
    "version": 5,
    "locktime": 0,
    "vin": [...],
    "vout": [...],
    "blockhash": "",
    "confirmations": 0,
    "time": 1704398400,
    "blocktime": 0
  },
  "error": null,
  "id": 1
}
```

**Scope**: Mempool/submitted transactions only (no historical blockchain queries yet)

---

### `sendrawtransaction`

Submit a raw transaction to the mempool.

**Parameters**:
1. `hexstring` (string, required) - Raw transaction hex
2. `allowhighfees` (boolean, optional, default=false) - Allow high fees (currently ignored)

**Returns**: Transaction ID (string)

**Example Request**:
```json
{
  "jsonrpc": "1.0",
  "method": "sendrawtransaction",
  "params": ["0100000001abcd..."],
  "id": 1
}
```

**Example Response**:
```json
{
  "result": "abcd1234567890...",
  "error": null,
  "id": 1
}
```

**Validation**:
- ✅ Hex decoding
- ✅ Transaction parsing (Zcash v5)
- ✅ ZIP-244 TxID computation
- ✅ Mempool addition

**Web Wallet Workflow**:
1. Wallet builds transaction client-side (WASM)
2. Wallet signs transaction with private keys (never sent to server)
3. Wallet calls `sendrawtransaction` with signed hex
4. Server validates and adds to mempool
5. Server returns txid for tracking

---

## Transparent Mempool RPCs

These methods query the transparent transaction mempool.

### `getrawmempool`

Query the transparent mempool.

**Parameters**:
1. `verbose` (boolean, optional, default=false) - Return detailed info if true

**Returns**:
- **Non-verbose**: Array of txids
- **Verbose**: Object with txid → transaction details

**Example Request**:
```json
{
  "jsonrpc": "1.0",
  "method": "getrawmempool",
  "params": [false],
  "id": 1
}
```

**Example Response** (non-verbose):
```json
{
  "result": ["txid1...", "txid2...", "txid3..."],
  "error": null,
  "id": 1
}
```

**Example Response** (verbose):
```json
{
  "result": {
    "txid1...": {
      "size": 250,
      "fee": 0.0001,
      "time": 1704398400
    },
    "txid2...": { ... }
  },
  "error": null,
  "id": 1
}
```

### `getmempoolinfo`

Get transparent mempool statistics.

**Parameters**: None

**Returns**: JSON object with mempool stats

**Example Request**:
```json
{
  "jsonrpc": "1.0",
  "method": "getmempoolinfo",
  "params": [],
  "id": 1
}
```

**Example Response**:
```json
{
  "result": {
    "size": 42,
    "bytes": 10500,
    "usage": 10500,
    "maxmempool": 300000000,
    "mempoolminfee": 0.00000100
  },
  "error": null,
  "id": 1
}
```

---

## Web Wallet Integration

### Transparent Transaction Flow

**1. Transaction Building** (Client-Side):
- Web wallet WASM module builds Zcash v5 transparent transaction
- User provides UTXOs, recipients, amounts
- **Private keys NEVER leave the browser**
- Transaction signing happens **client-side**

**2. Transaction Broadcast**:
```javascript
// Wallet calls sendrawtransaction with signed tx
const response = await fetch('http://localhost:8232', {
  method: 'POST',
  body: JSON.stringify({
    jsonrpc: '1.0',
    method: 'sendrawtransaction',
    params: [signedTxHex],
    id: 1
  })
});
const {result: txid} = await response.json();
```

**3. Transaction Verification**:
```javascript
// Verify broadcast succeeded
const tx = await rpc('getrawtransaction', [txid, true]);

// Check mempool status
const mempool = await rpc('getrawmempool', [false]);
console.log('In mempool:', mempool.includes(txid));
```

---

### Security Properties

**Client-Side Key Isolation**:
- ✅ Private keys isolated in browser (never transmitted)
- ✅ Only signed raw transaction hex sent to server
- ✅ Server cannot access or derive private keys
- ✅ Full non-custodial model

**Mempool Security**:
- ✅ Transaction parsing and hex validation
- ✅ ZIP-244 TxID computation (consensus-accurate)
- ✅ P2PKH/P2SH signature verification (Go builder)
- ✅ Rust validator signature verification (P2PKH/P2SH) when tag=4 is present

---

### Dual Pool Architecture

The service supports **both pools** simultaneously:

**Transparent Pool** (UTXO-based):
- Standard Bitcoin-style transactions
- Visible amounts and addresses
- Mempool via `TransparentTxStore`
- RPCs: `sendrawtransaction`, `getrawtransaction`, `getrawmempool`, `getmempoolinfo`

**Orchard Pool** (note-based):
- Privacy-preserving shielded transactions
- Hidden amounts and recipients
- Mempool via `OrchardTxPool`
- RPCs: `z_sendmany`, `z_listunspent`, `z_getbalance`, etc.

---

### Limitations

**Current Scope**:
- ✅ Mempool queries work (submitted transactions)
- ⚠️ Historical lookups are limited to the index only (no chain scan)
- ✅ Block index for `getrawtransaction` and `getblock` lookups (LevelDB-backed)
- ⚠️ Full transaction storage remains in-memory; mempool entries are persisted

**Future Work**:
- Persistent transaction storage beyond mempool + index
- Confirmation tracking across multiple blocks
- Reorg handling

---

## Complete RPC Method Reference

### Blockchain Methods (Shared)
| Method | Pool | Status | Description |
|--------|------|--------|-------------|
| `getblockchaininfo` | Both | ✅ | General blockchain info |
| `z_getblockchaininfo` | Both | ✅ | Extended blockchain info with pool statistics |
| `getbestblockhash` | Both | ✅ | Latest block hash |
| `getblockcount` | Both | ✅ | Current block height |
| `getblockhash` | Both | ✅ | Block hash by height |
| `getblock` | Both | ✅ | Block data with pool transactions |
| `getblockheader` | Both | ✅ | Block header only |

### Orchard Methods
| Method | Category | Status | Description |
|--------|----------|--------|-------------|
| `z_gettreestate` | Tree | ✅ | Orchard commitment tree state |
| `z_getsubtreesbyindex` | Tree | 🚧 | Subtree roots for light clients |
| `z_getnotescount` | Tree | ✅ | Note count in tree |
| `z_getnewaddress` | Wallet | 🚧 | Generate Orchard address (FFI pending) |
| `z_listaddresses` | Wallet | ✅ | List all Orchard addresses |
| `z_validateaddress` | Wallet | 🚧 | Validate Orchard address format (FFI pending) |
| `z_getbalance` | Wallet | ✅ | Orchard pool balance |
| `z_listunspent` | Wallet | ✅ | List unspent Orchard notes |
| `z_listreceivedbyaddress` | Wallet | ✅ | Notes received by address |
| `z_listnotes` | Wallet | ✅ | All Orchard notes |
| `z_sendmany` | Transaction | 🚧 | Send to multiple recipients (FFI pending) |
| `z_sendmanywithchangeto` | Transaction | 🚧 | Send with explicit change address (FFI pending) |
| `z_sendraworchardbundle` | Transaction | ✅ | Submit pre-built bundle |
| `z_viewtransaction` | Transaction | ✅ | View transaction details |
| `z_getoperationstatus` | Transaction | ❌ | Check async operation status (not yet implemented) |
| `z_getmempoolinfo` | Mempool | ✅ | Orchard mempool statistics |
| `z_getrawmempool` | Mempool | ✅ | Orchard mempool contents |

### Transparent Methods
| Method | Category | Status | Description |
|--------|----------|--------|-------------|
| `getrawtransaction` | Transaction | ✅ | Retrieve raw transparent transaction |
| `sendrawtransaction` | Transaction | ✅ | Broadcast transparent transaction |
| `getrawmempool` | Mempool | ✅ | Transparent mempool contents |
| `getmempoolinfo` | Mempool | ✅ | Transparent mempool statistics |

**Legend**:
- ✅ **Fully Implemented**
- 🚧 **In Progress** (FFI integration pending)

---

## Performance Targets

- **Address Generation**: <100ms via FFI
- **Bundle Proof Generation**: <5s for standard transactions
- **Tree State Queries**: <50ms from JAM storage
- **Note Scanning**: <1s per 1000 notes (light client)
- **RPC Response Time**: <200ms for standard queries

This Orchard RPC implementation provides full Zcash compatibility while leveraging JAM's unique builder/guarantor architecture for enhanced privacy and scalability.
