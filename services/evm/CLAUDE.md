
## Building the EVM Service

The EVM service needs to be rebuilt after making Rust changes to `services/evm/src/`:

```bash
cd /Users/michael/Desktop/jam/services
bash -c "/usr/bin/make evm"
```

Or from the project root:
```bash
bash -c "cd /Users/michael/Desktop/jam/services && /usr/bin/make evm"
```

This will:
1. Build the Rust code with cargo
2. Generate the `.pvm` files using polkatool
3. Create disassembly output in `services/evm/evm.txt`

**Note:** The build must complete successfully before running Go tests that depend on the EVM service.

## Running Tests

### EVM Tests

```bash
/opt/homebrew/bin/go test -run=TestEVMBlocksTransfers -v /Users/michael/Desktop/jam/statedb
```

Other common test patterns:
```bash
# Run all statedb tests
/opt/homebrew/bin/go test -v /Users/michael/Desktop/jam/statedb

# Run specific test with pattern
/opt/homebrew/bin/go test -run=TestEVMGenesis -v ./statedb/...
```

## Project Structure

- `services/evm/src/` - Rust EVM service implementation
  - `refiner.rs` - Block refiner logic (processes transactions)
  - `state.rs` - State backend and overlay (MajikBackend, MajikOverlay)
  - `sharding.rs` - Storage sharding logic
  - `meta_sharding.rs` - Meta-shard management

- `statedb/` - Go integration and tests
  - `rollup_test.go` - EVM integration tests including TestEVMBlocksTransfers

## Common Issues

### "No rule to make target 'evm'"

This happens when running make from the wrong directory or with shell issues. Use the full bash command:
```bash
bash -c "cd /Users/michael/Desktop/jam/services && /usr/bin/make evm"
```

### Shell Path Issues

If you see errors like `cd:1: command not found: __gvm_is_function`, wrap commands with `bash -c`:
```bash
bash -c "your command here"
```

## Debug Logging

The Rust service uses logging macros:
- `log_info!()` - Info level logs (shown in green)
- `log_debug!()` - Debug level logs (shown in cyan)
- `log_error!()` - Error level logs (shown in red)

Logs appear in Go test output with color codes like:
```
[32m[INFO-builder] Your message here[0m
[36m[DEBUG-builder] Your debug message[0m
```

## Compiling USDM Contract

The USDM contract at `services/evm/contracts/usdm.sol` must be compiled to **runtime bytecode** (not deployment bytecode).

### Command to compile:
```bash
/opt/homebrew/bin/solc --bin-runtime --optimize --optimize-runs=200 \
  /Users/michael/Desktop/jam/services/evm/contracts/usdm.sol \
  2>&1 | grep -A1 "Binary of the runtime" | tail -1 | xxd -r -p \
  > /Users/michael/Desktop/jam/services/evm/contracts/usdm-runtime.bin
```

### Why runtime bytecode?
- **Deployment bytecode**: Contains constructor + runtime code, returned when you deploy with CREATE
- **Runtime bytecode**: Only the executable contract code, what actually runs when you CALL the contract
- The EVM precompile loader expects runtime bytecode, not deployment bytecode

### After recompiling:
1. Rebuild the EVM service (it embeds the bytecode via `include_bytes!`)
2. Restart any running nodes (genesis state must reload the new bytecode)

## Workflow

1. Make changes to Rust code in `services/evm/src/`
2. Build the EVM service: `bash -c "cd services && /usr/bin/make evm"`
3. Run Go tests: `/opt/homebrew/bin/go test -run=TestName -v ./statedb`
4. Iterate!
