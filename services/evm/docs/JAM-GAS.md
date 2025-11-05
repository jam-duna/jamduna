# JAM Gas Model Architecture

This document describes how the JAM-native gas model is integrated into the EVM
service, the invariants it relies on, and how the resulting measurements flow
through execution and fee accounting.

## High-Level Overview

* **Goal**: Replace the vendor EVM gasometer with a host-measured gas model that
  charges only for JAM host function usage while treating all EVM opcodes as
  zero cost.
* **Core type**: `JAMGasState` in `services/evm/src/jam_gas.rs` wraps the
  vendor `State<'config>` and implements the `InvokerState` interface expected
  by the standard invoker.
* **Host meter**: The JAM runtime exposes the host function `utils::host_functions::gas()`
  which returns a monotonically decreasing counter. Gas consumption is derived
  from deltas of this value.
* **Reporting path**: `JAMGasState` snapshots the counter in `record_gas()` and
  exposes the current total via the `reported_gas_used()` hook. The standard
  invoker (patched in `services/vendor/evm`) prioritises this explicit
  measurement when populating `TransactValue::used_gas`.

### Code References

- `JAMGasState` implementation: `services/evm/src/jam_gas.rs:40`
- Invoker reporting patch: `services/vendor/evm/src/standard/invoker/state.rs:47` and
  `services/vendor/evm/src/standard/invoker/mod.rs:520`
- Fee accounting loop: `services/evm/src/main.rs:575`
- Chainspec configuration example: `chainspecs/jamduna-spec.json:1`

## Execution Lifecycle

1. **Transaction setup**
   `services/evm/src/main.rs` builds `TransactArgs` with `gas_price` forced to
   zero. This prevents the vendor invoker from performing legacy gas prepayment
   or coinbase tipping. The JAM model charges manually later.

2. **Interpreter activation**
   The work dispatcher instantiates
   `DispatchEtable<JAMGasState<'_>, MajikOverlay<'_>, CallCreateTrap>`. Because
   `JAMGasState` implements `InvokerState`, the interpreter constructs each
   runtime frame using the JAM wrapper instead of the vendor gasometer.

3. **Gas metering during execution**
   * `JAMGasState::new()` captures the host counter baseline, optionally logging
     debug information.
   * `JAMGasState::record_gas()` is invoked by the interpreter whenever an
     opcode would normally charge vendor gas. The implementation ignores the
     vendor argument, updates `jam_gas_used` with the current host delta, and
     throws an `OutOfGas` if the configured JAM limit is exceeded.
   * Nested calls use `JAMGasState::substate()` so every frame tracks its own
     limit and baseline while still sharing the same host counter.

4. **Finalisation**
   After the interpreter finishes, the standard invoker asks the substate for
   `effective_gas()` and `reported_gas_used()`. `JAMGasState::effective_gas()`
   returns the remaining JAM allowance (`limit - current_usage`). The patched
   invoker subtracts this from the original `gas_limit`, but overrides the
  result with the explicit value returned by `reported_gas_used()` when present.
   The final `TransactValue` therefore contains an authoritative JAM reading.

5. **Fee accounting**
   `services/evm/src/main.rs` receives the `TransactValue`, extracts
   `used_gas`, and:
   * Logs the measurement for telemetry.
   * Accumulates a running total for the block (`total_jam_gas_used`) which is
     written into the execution effects.
   * Computes the caller fee: `jam_gas_used * decoded.gas_price`.
   * Debits the caller and credits the block coinbase through the
     `MajikOverlay`. Because `gas_price` was set to zero in the `TransactArgs`,
     the vendor invoker has not already charged the account, preventing double
     withdrawals.

## Error Handling & Limits

* **Limit enforcement**
  `record_gas()` compares the latest host reading with the configured limit. If
  the limit is exceeded the interpreter receives an `OutOfGas` and the
  transaction reverts normally. The rejected call still reflects the measured
  overage in debug logs.

* **Fallback path**
  Should the host counter fail (returning zero deltas), the architecture still
  behaves correctly: `reported_gas_used()` would mirror the fallback value from
  the vendor invoker (`gas_limit - effective_gas`). This preserves existing
  behaviour without silently returning bogus numbers.

## Balance Conservation Invariant

The gas subsystem must preserve total balance across all accounts, including
coinbase. Formally:

```
Σ balances_before == Σ balances_after
```

The integration test `TestEVM` enforces this by comparing the global balance
sum before and after execution. Any deviation signals a double charge, missing
coinbase credit, or misconfigured `gas_price` path. The zero `TransactGasPrice`
paired with manual coinbase credit in `services/evm/src/main.rs` is critical
for maintaining this invariant.

## Chainspec Integration

The JAM gas model piggybacks on chainspec configuration such as `jamduna-spec`
(`chainspecs/jamduna-spec.json`) for parameters like:

- Default gas limits supplied to transactions.
- Whether JAM-specific host functions are enabled within the runtime.
- Per-service identifiers used when exporting gas-related execution effects.

When adjusting gas parameters, ensure chainspec values remain aligned with the
limits enforced by `JAMGasState`.

## Why the Invoker Patch Is Necessary

Historically the standard invoker assumed that gas usage equals the difference
between the initial limit and the residual reported via `effective_gas()`. With
the JAM model the true measurement is only available by sampling the host meter
after execution. By extending the `InvokerState` trait with
`reported_gas_used()` we ensure:

* A single, authoritative figure is propagated to `TransactValue::used_gas`.
* Consumers such as execution effects, receipts, and downstream accounting do
  not need to be aware of the JAM-specific implementation details.
* Future gas models can plug into the same hook without changing higher-level
  logic.

```
┌───────────────┐       host fn        ┌──────────────┐
│ EVM opcodes   │ ───── gas() ───────► │ JAMGasState  │
└───────────────┘                     │  (record_gas)│
                                      └──────┬───────┘
                                             │ reported_gas_used()
                                      ┌──────▼───────┐
                                      │ Invoker      │
                                      │ (transact)   │
                                      └──────┬───────┘
                                             │ used_gas
                                      ┌──────▼───────┐
                                      │ TransactValue│
                                      └──────┬───────┘
                                             │
                                      ┌──────▼───────┐
                                      │ Fee accounting
                                      │ (main.rs)     │
                                      └───────────────┘
```

## Testing Checklist

When validating changes to the JAM gas path:

1. Run `TestEVM` (or equivalent service integration test) and confirm log lines
   show `JAM Gas: used=<non-zero>` matching the host meter delta.
2. Inspect receipts to ensure `used_gas` mirrors the logged value.
3. Verify caller balances: vendor prepayment should be zero, and the manually
   debited fee must match `used_gas * gas_price`.
4. Confirm coinbase balances increase exactly by the charged fee (no drift).
5. Exercise error paths (e.g. transactions that exceed the JAM limit) to ensure
   `OutOfGas` triggers and the measurement still reflects the limit bust.

Keeping these checks green prevents regressions like the historical fixed-gas
reporting behaviour and catches balance mismatches early.

## Implementation Details

### Architecture

The implementation uses a simple before/after measurement approach:

1. **Measure JAM gas before transaction execution** using `unsafe { utils::host_functions::gas() }`
2. **Execute transaction with vendor gas_price set to ZERO** to prevent double-charging
3. **Measure JAM gas after transaction execution**
4. **Calculate actual consumption**: `jam_gas_consumed = gas_before - gas_after`
5. **Charge fees based on JAM gas consumption**: `fee = jam_gas_consumed * user_gas_price`

### Key Files

- **[services/evm/src/main.rs](../src/main.rs)** - Main implementation (transaction execution loop)
- **[services/evm/src/state.rs](../src/state.rs)** - MajikOverlay wrapper methods (balance helpers)
- **[node/node_jamtest_evm.go](../../../node/node_jamtest_evm.go)** - Test validation

### Gas Measurement

```rust
// Before transaction execution
let gas_before = unsafe { utils::host_functions::gas() };

// Execute transaction with zero vendor gas_price
let result = evm::transact(args, None, &mut overlay, &invoker);

// After transaction execution
let gas_after = unsafe { utils::host_functions::gas() };
let jam_gas_consumed = gas_before.saturating_sub(gas_after);
```

### Vendor Gas Price Configuration

```rust
// Set vendor gas_price to ZERO to prevent double-charging
// The vendor gasometer still works (prevents infinite loops)
// but doesn't charge fees
let args = TransactArgs {
    gas_price: TransactGasPrice::Legacy(U256::zero()),
    gas_limit,
    // ... other fields
};
```

### Fee Charging - Success Case

```rust
// Accumulate JAM host gas across all successful transactions
total_jam_gas_used = total_jam_gas_used.saturating_add(jam_gas_consumed);

// Charge fees BEFORE committing so they're persisted
// CRITICAL: Must use overlay.withdrawal/deposit (NOT overlay.overlay.*)
// to properly record storage writes for balance shards
let gas_price = decoded.gas_price;
let gas_fee = U256::from(jam_gas_consumed).saturating_mul(gas_price);
let coinbase = overlay.overlay.block_coinbase();

match overlay.withdrawal(decoded.caller, gas_fee) {
    Ok(_) => {
        overlay.deposit(coinbase, gas_fee);
        call_log(2, None, &format!(
            "  💰 Charged JAM gas fee: {} wei to coinbase", gas_fee
        ));
    }
    Err(_) => {
        call_log(1, None, &format!(
            "  ⚠️  Caller insufficient balance for JAM gas fee (need {})", gas_fee
        ));
    }
}

// IMPORTANT: Fee charging must happen BEFORE commit
overlay.commit_transaction()?;
```

### Fee Charging - Error Case

```rust
// Revert the failed transaction
overlay.revert_transaction()?;

// Accumulate gas from failed transactions
total_jam_gas_used = total_jam_gas_used.saturating_add(jam_gas_consumed);

// Begin new transaction for fee charging (separate from reverted tx)
overlay.begin_transaction(tx_index + 1000);

// CRITICAL: Must use overlay.withdrawal/deposit (NOT overlay.overlay.*)
let gas_price = decoded.gas_price;
let gas_fee = U256::from(jam_gas_consumed).saturating_mul(gas_price);
let coinbase = overlay.overlay.block_coinbase();

match overlay.withdrawal(decoded.caller, gas_fee) {
    Ok(_) => {
        overlay.deposit(coinbase, gas_fee);
        call_log(2, None, &format!(
            "  💰 Charged JAM gas fee (reverted): {} wei to coinbase", gas_fee
        ));
    }
    Err(_) => {
        call_log(1, None, &format!(
            "  ⚠️  Caller insufficient balance for JAM gas fee (need {})", gas_fee
        ));
    }
}

// Commit the fee transaction
overlay.commit_transaction()?;
```

## CRITICAL: Fee Persistence via MajikOverlay

### The Problem

Using `overlay.overlay.withdrawal()` and `overlay.overlay.deposit()` directly bypasses the MajikOverlay wrapper methods that:
1. Mirror balances into the 0x01 system contract storage
2. Record shard writes via `tracker.record_write(WriteKey::storage(...))`

**Result:** Fees appear to be charged in debug logs, but the storage shard still contains 0x00 for the coinbase balance key. The fee recipient never actually receives payment.

### The Solution

Always use the `MajikOverlay` wrapper methods (defined in `services/evm/src/state.rs`):

```rust
// ❌ WRONG - bypasses storage persistence
overlay.overlay.withdrawal(caller, gas_fee)?;
overlay.overlay.deposit(coinbase, gas_fee);

// ✅ CORRECT - properly records storage writes
overlay.withdrawal(caller, gas_fee)?;
overlay.deposit(coinbase, gas_fee);
```

### How MajikOverlay Methods Work

#### Withdrawal Method

```rust
fn withdrawal(&mut self, source: H160, value: U256) -> Result<(), ExitError> {
    // Use storage operations to avoid conflicts with USDM transfers
    let system_contract = h160_from_low_u64_be(0x01);
    let storage_key = balance_storage_key(source);

    // Read current balance from storage
    let current_balance = self.overlay.storage(system_contract, storage_key);
    let current_value = U256::from_big_endian(current_balance.as_bytes());

    // Check sufficient balance
    if current_value < value {
        return Err(ExitException::OutOfFund.into());
    }

    // Subtract withdrawal amount
    let new_value = current_value - value;
    let mut encoded = [0u8; 32];
    new_value.to_big_endian(&mut encoded);
    let new_storage_value = H256::from(encoded);

    // Write new balance to storage
    self.overlay.set_storage(system_contract, storage_key, new_storage_value).unwrap();

    // CRITICAL: Record the storage write for shard export
    self.tracker
        .borrow_mut()
        .record_write(WriteKey::storage(system_contract, storage_key));

    Ok(())
}
```

#### Deposit Method

```rust
fn deposit(&mut self, target: H160, value: U256) {
    let system_contract = h160_from_low_u64_be(0x01);
    let storage_key = balance_storage_key(target);

    // Read current balance
    let current_balance = self.overlay.storage(system_contract, storage_key);
    let current_value = U256::from_big_endian(current_balance.as_bytes());

    // Add deposit amount
    let new_value = current_value.saturating_add(value);
    let mut encoded = [0u8; 32];
    new_value.to_big_endian(&mut encoded);
    let new_storage_value = H256::from(encoded);

    // Write new balance to storage
    self.overlay.set_storage(system_contract, storage_key, new_storage_value).unwrap();

    // CRITICAL: Record the storage write for shard export
    self.tracker
        .borrow_mut()
        .record_write(WriteKey::storage(system_contract, storage_key));
}
```

### Why This Matters

The wrapper methods ensure:
1. **Balance changes are written to storage keys** in the 0x01 system contract
2. **Storage writes are tracked** via `WriteKey::storage(system_contract, storage_key)`
3. **The exported shard includes the updated balance values**
4. **Changes persist after transaction commit**

Without using the wrapper methods, the balance changes happen only in the ephemeral overlay and are never written to storage shards, so they're lost after the work package completes.

## Transaction Ordering

### Success Case

1. Execute transaction
2. **Charge fees (withdrawal + deposit) BEFORE commit**
3. Commit transaction (includes fee changes)
4. Record transaction receipt

### Error Case

1. Execute transaction (fails)
2. **Revert transaction** (undoes any state changes)
3. **Begin new transaction** (separate from reverted tx)
4. **Charge fees (withdrawal + deposit)**
5. **Commit fee transaction**
6. Record transaction receipt

The key insight: fee charging must happen **within** a transaction context (either the original transaction for success, or a new transaction for errors) so the storage writes are included when committing.

## Gas Comparison Logging

The implementation logs both vendor gas and JAM host gas for comparison:

```rust
call_log(2, None, &format!(
    "  ⛽ Gas comparison: vendor={}, jam_host={}",
    vendor_gas_used, jam_gas_consumed
));
```

This allows monitoring the difference between theoretical EVM gas and actual JAM resource consumption.

## Test Validation

The test suite validates that coinbase actually receives fees:

```go
balance := new(big.Int).SetBytes(balanceHash.Bytes())
if balance.Cmp(big.NewInt(0)) == 0 {
    log.Error(log.Node, "❌ CRITICAL: Coinbase failed to collect fees",
              "address", coinbaseAddress.String(), "balance", balance.String())
    t.Fatalf("Coinbase balance is 0 - fee collection failed")
}
```

This test will fail if fees aren't properly persisted, ensuring the implementation remains correct.

## Benefits of This Approach

### Simple Implementation
- Only 3 lines of code for gas measurement
- No custom state wrappers or trait implementations
- Direct measurement of actual resource consumption

### Vendor Gasometer Still Works
- Vendor gasometer prevents infinite loops (OutOfGas protection)
- But doesn't charge fees (gas_price = 0)
- Provides fallback safety mechanism

### Accurate Resource Accounting
- Measures actual JAM host resource consumption
- No theoretical pricing or opcode gas tables
- Charges based on real computation, storage, and DA costs

### Future-Proof
- Independent of EVM gas model changes
- Can adjust JAM pricing without EVM dependencies
- Simpler to reason about and maintain

## Troubleshooting

| Symptom | Likely Cause | Suggested Fix |
|---------|--------------|---------------|
| Balance mismatch reported by TestEVM | Double charging (vendor + manual) or missing coinbase credit | Ensure `TransactArgs` uses zero `gas_price` in `services/evm/src/main.rs`, and confirm manual debit/credit paths execute |
| Zero gas reported in receipts | Host meter unavailable or baseline capture failed | Check host `gas()` availability, verify `JAMGasState::new` logging, and ensure `reported_gas_used()` is called |
| `OutOfGas` on trivial transactions | JAM limit misconfigured or chainspec mismatch | Validate gas limit passed into `JAMGasState::new` and the chainspec gas limits |
| Coinbase not collecting fees | Manual charging skipped due to early error or using wrong overlay methods | Inspect overlay commit logs, ensure fee calculation uses `used_gas` and runs after successful commit, verify using `overlay.withdrawal/deposit` not `overlay.overlay.*` |

If these steps do not recover normal behaviour, enable `debug_enabled` in
`JAMGasState::new` to capture detailed host meter transitions and review the
logs emitted by `call_log`.

## Summary

The current implementation:
1. ✅ Measures JAM host gas before/after transaction execution
2. ✅ Sets vendor gas_price to ZERO to prevent double-charging
3. ✅ Charges fees based on actual JAM gas consumption
4. ✅ Uses `overlay.withdrawal/deposit` to properly persist fees
5. ✅ Validates coinbase receives fees in tests

This approach provides accurate resource accounting based on actual JAM host consumption while maintaining safety through vendor gasometer limits.
