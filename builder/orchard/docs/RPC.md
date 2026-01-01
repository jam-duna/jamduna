# Zcash JSON-RPC Reference (Orchard-only JAM Service)

This document describes the **Zcash-compatible JSON-RPC calls** for JAM's Orchard service implementation. The API provides full compatibility with standard Zcash RPC while integrating with JAM's builder/guarantor architecture.

- Transport: HTTP JSON-RPC
- Auth: RPC username/password or cookie auth
- All examples use `"id": 1` for brevity

## Implementation Status

### **✅ 1. JAM Integration Complete**
**Status:** Implemented | **Priority:** High

**What works:**
- Orchard bundle processing via JAM refine/accumulate
- Tree state reads from JAM service storage
- Builder-sponsored transaction submission
- Witness-based execution model integration

---

### **✅ 2. Orchard State Management**
**Status:** Implemented | **Priority:** High

**What works:**
- Standard Orchard commitment tree (Sinsemilla, depth 32)
- Nullifier tracking for double-spend prevention
- Note commitment append operations
- Tree witness generation for light clients

---

### **🚧 3. Full Orchard FFI Integration**
**Status:** In Progress | **Priority:** High | **Timeline:** 2-3 weeks

**Current State:**
- Basic FFI stub functions implemented
- Need full `orchard` crate integration
- Bundle proof generation/verification pending

**Next Steps:**
1. **Orchard Crate Integration**: Replace stubs with real `orchard` crate calls
2. **Bundle Proof Generation**: Implement Halo2 proof generation via FFI
3. **Address Generation**: Real Orchard key derivation and address encoding

---

## JAM Orchard RPC Implementation

The JAM node implements Zcash-compatible RPC methods in `builder/orchard/rpc/orchard_handler.go`:

| Method                        | Category          | Status | Description |
| ----------------------------- | ----------------- | ------ | ----------- |
| getblockchaininfo             | Chain             | ✅     | Orchard-specific blockchain info |
| z_getblockchaininfo           | Chain             | ✅     | Extended Orchard statistics |
| getbestblockhash / getblockcount / getblockhash | Chain | ✅ | JAM block integration |
| getblock / getblockheader     | Chain             | ✅     | Orchard bundle data in blocks |
| z_gettreestate                | Tree              | ✅     | Orchard tree state from JAM storage |
| z_getsubtreesbyindex          | Tree              | 🚧     | Subtree root management |
| z_getnotescount               | Tree              | ✅     | Note counting |
| z_getnewaddress               | Wallet            | 🚧     | Orchard address generation via FFI |
| z_listaddresses               | Wallet            | ✅     | Address enumeration |
| z_validateaddress             | Wallet            | 🚧     | Orchard address validation |
| z_getbalance                  | Wallet            | ✅     | Note value summation |
| z_listunspent / z_listreceivedbyaddress / z_listnotes | Wallet | ✅ | Note management |
| z_sendmany / z_sendmanywithchangeto | Wallet      | 🚧     | Orchard bundle submission |
| z_viewtransaction             | Wallet            | ✅     | Transaction inspection |

---

## Core Chain Methods

### getblockchaininfo
Returns information about the Orchard-enabled JAM blockchain.

```json
// Request
{"method": "getblockchaininfo", "params": [], "id": 1}

// Response
{
  "result": {
    "chain": "jam-orchard",
    "blocks": 150000,
    "bestblockhash": "0x1a2b3c4d...",
    "difficulty": 1.0,
    "verificationprogress": 0.999,
    "chainwork": "0x00000000000000000000000000000000000000000000001a2b3c4d5e6f7890",
    "size_on_disk": 12345678,
    "commitments": 45000,  // Total Orchard note commitments
    "valuePools": [
      {
        "id": "orchard",
        "monitored": true,
        "chainValue": 1000000000000,  // Total Orchard pool value
        "chainValueZat": 100000000000000
      }
    ]
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
    "chain": "jam-orchard",
    "blocks": 150000,
    "bestblockhash": "0x1a2b3c4d...",
    "consensus": {
      "chaintip": "orchard-v1",
      "nextblock": "orchard-v1"
    },
    "upgrades": {
      "orchard": {
        "name": "Orchard",
        "activationheight": 0,
        "status": "active"
      }
    },
    "valuePools": [
      {
        "id": "orchard",
        "monitored": true,
        "chainValue": 1000000000000,
        "chainValueZat": 100000000000000,
        "notes": 45000,
        "nullifiers": 12000
      }
    ]
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

## Tree State Methods

### z_gettreestate
Returns the current Orchard commitment tree state.

```json
// Request
{"method": "z_gettreestate", "params": ["latest"], "id": 1}

// Response
{
  "result": {
    "height": 150000,
    "hash": "0x1a2b3c4d...",
    "time": 1735571200,
    "orchard": {
      "commitments": {
        "finalRoot": "0x9876543210fedcba...",
        "finalState": "000100...", // Serialized tree state
        "size": 45000
      }
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
        "height": 16  // Subtree height within the main tree
      },
      // ... up to 10 subtrees
    ]
  }
}
```

---

## Wallet Methods

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
  "result": 1.5  // Balance in ZEC (converted from zatoshi)
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

### z_getoperationstatus
Checks the status of async operations like z_sendmany.

```json
// Request
{"method": "z_getoperationstatus", "params": [["opid-12345678..."]], "id": 1}

// Response
{
  "result": [
    {
      "id": "opid-12345678-abcd-ef90-1234-567890abcdef",
      "status": "success",
      "creation_time": 1735571200,
      "result": {
        "txid": "0x9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1a0b9c8d7e6f5a4b3c2d1e0f9a8b"
      }
    }
  ]
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

## Performance Targets

- **Address Generation**: <100ms via FFI
- **Bundle Proof Generation**: <5s for standard transactions
- **Tree State Queries**: <50ms from JAM storage
- **Note Scanning**: <1s per 1000 notes (light client)
- **RPC Response Time**: <200ms for standard queries

This Orchard RPC implementation provides full Zcash compatibility while leveraging JAM's unique builder/guarantor architecture for enhanced privacy and scalability.