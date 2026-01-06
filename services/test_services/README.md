# Test Services

This directory contains test services for performance testing and development purposes.

## Overview

Test services use `main()` as their entry point (instead of `refine()`) and are built with the `-m` flag in polkatool. These services are designed for testing specific functionality or performance characteristics of the JAM/PolkaVM runtime.

## Building

Build all test services:
```bash
make test-services
```

Build from the test_services directory:
```bash
cd test_services
make test-services
```

## Available Test Services

### simple-test
A simple performance test service that:
- Performs 100 iterations of Blake2b hashing
- Uses 1MB stack and 1MB heap
- Logs each hash iteration for performance analysis
- Entry point: `main()` function

### arithmetic-bench
A pure arithmetic benchmark that:
- Runs a tight loop of add/sub/mul/div/bitwise ops
- Allows loop count to be passed via input bytes
- Entry point: `main(start_address, length)`

### memory-bench
A memory-focused benchmark that:
- Allocates a heap buffer and repeatedly reads/writes with a stride pattern
- Allows iterations, buffer size, and stride to be passed via input bytes
- Entry point: `main(start_address, length)`

## Adding New Test Services

1. Create a new directory under `test_services/`:
```bash
mkdir test_services/my-test
```

2. Create a `Cargo.toml`:
```toml
[package]
name = "my-test"
version = "0.1.0"
edition = "2024"

[dependencies]
polkavm-derive = { workspace = true }
simplealloc = { workspace = true }
utils = { path = "../../utils" }

[profile.release]
opt-level = "z"
lto = "fat"
codegen-units = 1
panic = "abort"
strip = true
overflow-checks = false
```

3. Create `src/main.rs` with a `main()` entry point:
```rust
#![no_std]
#![no_main]

extern crate alloc;

use polkavm_derive::min_stack_size;
use simplealloc::SimpleAlloc;

const SIZE0: usize = 0x10000;
min_stack_size!(SIZE0);

const SIZE1: usize = 0x10000;
#[global_allocator]
static ALLOCATOR: SimpleAlloc<SIZE1> = SimpleAlloc::new();

#[polkavm_derive::polkavm_export]
extern "C" fn main() -> (u64, u64) {
    // Your test code here
    (0, 0)
}

#[cfg(all(not(test), target_arch = "riscv32", target_feature = "e"))]
#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    unsafe {
        core::arch::asm!("unimp", options(noreturn));
    }
}
```

4. Add to workspace in `services/Cargo.toml`:
```toml
members = [
    # ...
    "test_services/my-test",
]
```

5. Build with `make test-services`

## Output Files

For each test service, the build process generates:
- `{service}.pvm` - JAM-ready executable
- `{service}_blob.pvm` - Raw code blob for disassembly
- `{service}.txt` - Disassembled output with raw bytes

## Differences from Production Services

| Aspect | Production Services | Test Services |
|--------|-------------------|---------------|
| Entry Point | `refine()` / `accumulate()` | `main()` |
| Build Flag | (none) | `-m` |
| Purpose | JAM blockchain execution | Testing & benchmarking |
| Location | `services/` | `services/test_services/` |

## Clean Up

Remove all build artifacts:
```bash
make clean
```
