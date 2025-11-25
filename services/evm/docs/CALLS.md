# EVM Call and EstimateGas Implementation

This document explains how the EVM service handles unsigned call data for `eth_call` and `eth_estimateGas` RPC methods using `PayloadType::Call` (0x03).

## Overview

The EVM service supports four payload types:
- **PayloadType::Builder (0x00)**: Builder-submitted transaction bundles
- **PayloadType::Transactions (0x01)**: Normal transaction execution with signed RLP-encoded transactions + optional block witnesses
- **PayloadType::Genesis (0x02)**: Genesis bootstrap mode for initializing state/DA  (genesis-only)
- **PayloadType::Call (0x03)**: EstimateGas/Call mode for unsigned transaction simulation (read-only)

`PayloadType::Call` enables static calls and gas estimation without requiring transaction signatures, which is essential for read-only operations like `balanceOf()` queries.

## Architecture

### Request Flow

```
eth_call / eth_estimateGas RPC
    ↓
Node creates WorkPackage with payload "B"
    ↓
EVM service receives single extrinsic with unsigned call data
    ↓
decode_call_args() parses call parameters
    ↓
EVM runtime executes the call
    ↓
Returns: ExecutionEffects + call output
```

### Data Format

#### Input (Extrinsic Format)
```
caller(20) + target(20) + gas_limit(32) + gas_price(32) +
value(32) + call_kind(4) + data_len(8) + data
```

**Field Details:**
- `caller`: 20-byte address initiating the call
- `target`: 20-byte contract address (or zero for contract creation)
- `gas_limit`: 32-byte big-endian gas limit
- `gas_price`: 32-byte big-endian gas price
- `value`: 32-byte big-endian ETH value to transfer
- `call_kind`: 4-byte little-endian (0 = CALL, 1 = CREATE)
- `data_len`: 8-byte little-endian length of call data
- `data`: Variable-length call data (e.g., function selector + arguments)

#### Output (Result Format)
```
ExecutionEffects Serialization + Call Output
```

**ExecutionEffects Structure:**
- ExecutionEffects now holds only `write_intents`; there is no gas or export counter in the struct.【F:services/utils/src/effects.rs†L15-L41】
- The serializer emits **only meta-shard write intents** using the compact format described in `writes.rs` (ld byte + prefix + packed ObjectRef).【F:services/evm/src/writes.rs†L1-L69】
- Because call mode produces a single non–meta-shard `WriteIntent`, the serialized refine output is currently **empty**.

**Call Output:**
- The call result bytes live inside the `payload` of the placeholder `WriteIntent` created by `refine_call_payload`, but they are dropped by serialization because the intent is not a meta-shard entry.【F:services/evm/src/refiner.rs†L676-L814】
- RPC clients therefore receive an empty refine response for `PayloadType::Call` in the current codebase; surfacing the call output will require extending the serializer to carry non–meta-shard data.

## Implementation Details

### Rust Service (services/evm/src/refiner.rs)

#### PayloadType::Call Detection
```rust
match metadata.payload_type {
    PayloadType::Call => {
        // Single extrinsic with unsigned call data
        // Used for eth_estimateGas and eth_call RPCs
        return BlockRefiner::refine_call_payload(
            refine_context,
            extrinsics,
            metadata.payload_size,
        );
    }
    // ... other payload types
}
```

#### Call Arguments Decoding
Located in `refine_call_payload()`:
```rust
let decoded = match decode_call_args(extrinsic.as_ptr() as u64, extrinsic.len() as u64) {
    Some(d) => d,
    None => {
        log_error("❌ PayloadCall: Failed to decode call args");
        return None;
    }
};
```

The `decode_call_args()` function (tx.rs):
- Parses unsigned call data without signature verification
- Validates minimum buffer length (148 bytes)
- Extracts all transaction parameters
- Returns `DecodedTransactArgs` struct

#### EVM Execution
```rust
let result = evm::transact(args, None, &mut overlay, &invoker);
```

Executes the call using the same EVM runtime as signed transactions, but:
- No signature verification
- **Gas price set to zero** - no balance deductions for gas fees
- No state modifications are persisted (read-only)
- No receipts generated
- No transaction records created
- No block witnesses processed

#### Result Serialization (Lines 272-304)
```rust
match result {
    Ok(tx_result) => {
        let gas_used = tx_result.used_gas.as_u64();
        let output = match tx_result.call_create {
            evm::standard::TransactValueCallCreate::Call { retval, .. } => retval,
            evm::standard::TransactValueCallCreate::Create { .. } => Vec::new(),
        };

        // Create ExecutionEffects with gas_used
        let execution_effects = utils::effects::ExecutionEffects {
            write_intents: Vec::new(),
            export_count: 0,
            gas_used,
        };

        // Serialize and append output
        let mut buffer = serialize_execution_effects(&execution_effects, &candidate_writes);
        buffer.extend_from_slice(&output);

        return leak_output(buffer);
    }
}
```

### Go Node (node/node_evm_tx.go)

#### Creating Simulation Work Package
```go
// Build payload: type=0x03 (Call), tx_count=1, witness_count=0
payload := make([]byte, 7)
payload[0] = 0x03 // PayloadType::Call
binary.LittleEndian.PutUint32(payload[1:5], 1)  // tx_count = 1
binary.LittleEndian.PutUint16(payload[5:7], 0)  // witness_count = 0

WorkItem: types.WorkItem{
    Payload: payload,
    ExportCount: 0,  // No exports for static call
    ...
}
```

The extrinsic format matches the Rust input specification exactly.

#### Parsing Results (Lines 415-441)
```go
effects, err := statedb.DeserializeExecutionEffects(result.Ok)
if err != nil {
    // Handle error
}

// ExecutionEffects format: [export_count:2][gas_used:8][write_intents_count:2][write_intents...]
// For payload "B", write_intents should be empty (count=0)
effectsHeaderSize := 12 // 2 + 8 + 2

if len(result.Ok) > effectsHeaderSize {
    // Extract the call output (everything after ExecutionEffects header)
    callOutput := result.Ok[effectsHeaderSize:]
    return callOutput, nil
}
```

#### EstimateGas Implementation (Lines 220-273)
Returns only the `gas_used` value from ExecutionEffects:
```go
effects, err := statedb.DeserializeExecutionEffects(workReport.Results[0].Result.Ok)
return effects.GasUsed, nil
```

#### Call Implementation (Lines 275-281)
Returns the call output bytes:
```go
callOutput := result.Ok[effectsSize:]
return callOutput, nil
```

## Example: balanceOf() Query

### Test Implementation (node/node_jamtest_evm.go:234-260)

```go
// Test Call for balanceOf(devAccount0)
devAccount0, _ := common.GetEVMDevAccount(0)

// balanceOf(address) selector: 0x70a08231
calldataBalanceOf := make([]byte, 36)
calldataBalanceOf[0] = 0x70  // Function selector byte 1
calldataBalanceOf[1] = 0xa0  // Function selector byte 2
calldataBalanceOf[2] = 0x82  // Function selector byte 3
calldataBalanceOf[3] = 0x31  // Function selector byte 4

// Encode address parameter (32 bytes, left-padded)
copy(calldataBalanceOf[16:36], devAccount0[:])

// Execute the call
callResult, err := n1.Call(devAccount0, &usdmAddress, 100000, 1000000000, 0, calldataBalanceOf, "latest")

// Parse result - should be 61MM * 10^18
expectedBalance := new(big.Int).Mul(big.NewInt(61_000_000), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
actualBalance := new(big.Int).SetBytes(callResult[len(callResult)-32:])

if actualBalance.Cmp(expectedBalance) != 0 {
    log.Error(log.Node, "Call balanceOf MISMATCH", "expected", expectedBalance, "actual", actualBalance)
}
```

### Execution Flow

1. **Node constructs extrinsic**:
   ```
   [devAccount0:20][usdmAddress:20][100000:32][1000000000:32]
   [0:32][0:4][36:8][0x70a08231...address:36]
   ```

2. **Rust service decodes**:
   - Caller: devAccount0
   - Target: usdmAddress (0x01)
   - Gas limit: 100000
   - Data: `0x70a08231` + left-padded address

3. **EVM executes**:
   - Calls USDM contract at 0x01
   - Executes `balanceOf(address)` function
   - Returns 32-byte uint256 balance

4. **Rust service returns**:
   ```
   [export_count:2][gas_used:8][write_count:2][balance:32]
   ```
   Total: 12 + 32 = 44 bytes

5. **Node parses**:
   - Skips first 12 bytes (ExecutionEffects header)
   - Extracts remaining bytes as balance (32 bytes)
   - Converts to big.Int: 61,000,000 × 10^18

## Key Differences from Signed Transactions

| Aspect | PayloadType::Call (0x03) | PayloadType::Transactions (0x01) |
|--------|----------------------------|-----------------------------------|
| **Payload Format** | `0x03 + tx_count:4 + witness_count:2` | `0x01 + tx_count:4 + witness_count:2` |
| **Signature** | Not required | Required (verified via secp256k1) |
| **State Changes** | Not persisted | Persisted to state |
| **Receipts** | Not generated | Generated and stored |
| **RLP Encoding** | Not required | Required for tx hash |
| **Gas Charging** | Simulated only | Charged to caller |
| **Nonce Check** | Skipped | Validated |
| **Exports** | 0 (no DA objects) | Variable (receipts, shards, SSR, block metadata) |
| **Block Witnesses** | Not supported | Optional (for metadata computation) |
| **Handler** | `refine_call_payload()` | `refine_payload_transactions()` |

## Gas Computation

For `PayloadType::Call`, gas is computed using the **JAM Gas Model**:
- EVM opcodes have zero cost
- Gas consumption is measured via JAM host function calls
- The `gas_used` value represents pure JAM gas (not vendor gas)

See `jam_gas.rs` for the JAMGasState implementation.

## Error Handling

### Rust Service Errors
- **Insufficient extrinsics**: Returns empty output with error log
- **Decode failure**: Returns empty output with error log
- **Execution failure**: Returns empty output with error log

### Go Node Errors
- **Simulation failure**: Returns error to RPC caller
- **Deserialization failure**: Logs warning, returns raw result
- **No results**: Returns "no result from simulation" error

## Testing

Run the EVM test suite to verify payload "B" functionality:
```bash
go test -tags network_test -run TestEVM
```

The test performs:
1. Genesis bootstrap with USDM contract deployment
2. Balance initialization (61MM USDM to devAccount0)
3. `balanceOf()` call via payload "B"
4. Balance verification
5. Multiple signed transfer transactions

Expected result: balanceOf call returns exactly 61,000,000 × 10^18.

## Future Enhancements

Potential improvements for payload "B" mode:
- **Block number support**: Execute calls at historical state
- **Access lists**: Support EIP-2930 access list transactions
- **Tracing**: Return execution traces for debugging
- **Gas optimization**: Binary search for optimal gas limit
- **Batch calls**: Support multiple calls in single work package

## Related Files

- **Rust Service**:
  - `services/evm/src/refiner.rs` - PayloadType routing and `refine_call_payload()` handler
  - `services/evm/src/tx.rs` - `decode_call_args()` function
  - `services/evm/src/jam_gas.rs` - JAM gas model

- **Go Node**:
  - `node/node_evm_tx.go` - Call/EstimateGas implementation
  - `statedb/rollup.go` - Work package creation with payload type 0x03

- **Utilities**:
  - `services/utils/src/effects.rs` - ExecutionEffects definition
  - `types/statewitness.go` - Go-side ExecutionEffects deserialization

## Comparison to Standard Ethereum

Standard Ethereum RPC:
```
eth_call → Direct state read (no work package)
eth_estimateGas → Binary search on gas limit
```

JAM EVM (PayloadType::Call):
```
eth_call → Work package with PayloadType::Call (0x03) → Single execution
eth_estimateGas → Work package with PayloadType::Call (0x03) → Single execution
```

Benefits of JAM approach:
- Consistent execution model across all operations (same EVM runtime)
- Proper gas accounting via JAM host functions
- Verifiable results via work reports
- Compatible with JAM's work package architecture
- Unified payload format across all operation types

## Security Considerations

1. **No Authentication**: `PayloadType::Call` accepts any caller address without signature verification. This is safe because:
   - State changes are not persisted
   - Used only for read-only operations
   - Gas is not actually charged

2. **Gas Limits**: Calls are still subject to gas limits to prevent DoS:
   - Default limit: RefineGasAllocation / 2
   - Can be overridden per call
   - Prevents infinite loops

3. **State Isolation**: Each call operates on a clean overlay that is discarded after execution.

## Troubleshooting

### Common Issues

**Issue**: Call returns empty output
- **Cause**: EVM execution reverted
- **Solution**: Check contract exists, function signature is correct, and parameters are valid

**Issue**: Wrong gas estimate
- **Cause**: Gas limit too low for actual execution
- **Solution**: Increase gas limit parameter in Call/EstimateGas

**Issue**: Balance mismatch
- **Cause**: State not properly initialized or wrong storage slot
- **Solution**: Verify genesis bootstrap completed successfully, check storage key computation

### Debug Logging

Enable verbose logging in Rust:
```rust
call_log(2, None, &format!("📞 Payload 'B': Call from={:?}, gas_limit={}", caller, gas_limit));
```

Enable debug logging in Go:
```go
log.Debug(log.Node, "executeSimulationWorkPackage: extracted call output",
    "gas_used", effects.GasUsed, "output_len", len(callOutput))
```
