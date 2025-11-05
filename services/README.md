# JAM Services

This repository contains JAM (Join-Accumulate Machine) services and the complete toolchain needed to build them.

## Overview

This is a self-contained workspace that includes:
- **JAM Services**: EVM, Algo, Bootstrap, Auth Copy, Null Authorizer, Doom, and utility services
- **PolkaTool**: Service compilation and linking tool
- **PolkaVM Ecosystem**: Complete virtual machine implementation and tooling
- **Build Infrastructure**: Guest programs, examples, and build scripts

## Quick Start

### Using the Self-Contained Services Makefile

**From the services directory** (recommended for independent use):

```bash
cd services

# Build the EVM service
make evm

# Build other individual services
make algo
make bootstrap
make auth_copy
make null_authorizer
make doom

# Build all services
make all

# Clean all build artifacts
make clean

# List available services and commands
make list
```

### Building from Main Repository

From the main repository root:

```bash
# Build the EVM service
make evm
```

This will:
1. Compile the EVM service using the RISC-V target
2. Convert it to PolkaVM bytecode using polkatool
3. Generate disassembly output
4. Create the final `.pvm` service files

### Building Other Services

```bash
# Build individual services
make algo
make bootstrap
make auth_copy
make null_authorizer
make doom

# Build all services
make all
```

## Advanced Build Options

### Using Direct Cargo Commands

You can also build using direct cargo commands from the services directory:

```bash
cd services

# Build using workspace cargo commands
cargo build -p evm --release --target crates/polkavm-linker/riscv64emac-unknown-none-polkavm.json
cargo run -p polkatool jam-service evm/target/riscv64emac-unknown-none-polkavm/release/evm -o evm/evm.pvm

# Or use the polkatool directly for any service
cargo run -p polkatool jam-service algo/target/riscv64emac-unknown-none-polkavm/release/algo -o algo/algo.pvm
```

### Makefile Targets Reference

The services Makefile provides these additional targets:

```bash
# Development helpers
make dev-evm          # Quick EVM development build
make dev-algo         # Quick Algo development build
make dev-bootstrap    # Quick Bootstrap development build

# Service groups
make core-services    # Build bootstrap + evm
make auth-services    # Build auth_copy + null_authorizer
make compute-services # Build algo + doom

# Utilities
make info            # Show build environment information
make verify          # Quick build verification (EVM smoke test)
make parallel        # Build all services in parallel
make dry-run         # Show build commands without executing

# CI/CD
make ci-build        # Full CI build with verification
make ci-test         # Verify all service artifacts exist
```

## Service Artifacts

Each service build generates:
- `{service}.pvm` - JAM-ready service bytecode
- `{service}_blob.pvm` - Raw service bytecode
- `{service}.txt` - Human-readable disassembly

## Available Services

| Service | Description |
|---------|-------------|
| **evm** | Ethereum Virtual Machine implementation |
| **algo** | Algorithmic computation service |
| **bootstrap** | Bootstrap service for chain initialization |
| **auth_copy** | Authentication copying service |
| **null_authorizer** | Null authorization service |
| **doom** | Doom game implementation |
| **utils** | Shared utility functions |

## Toolchain Components

### PolkaTool
- **Location**: `tools/polkatool/`
- **Purpose**: Converts RISC-V ELF binaries to PolkaVM bytecode
- **Usage**: `cargo run -p polkatool jam-service <input> -o <output>`

### PolkaVM Crates
- **polkavm**: Core virtual machine runtime
- **polkavm-linker**: ELF to PolkaVM converter
- **polkavm-disassembler**: Bytecode analysis tool
- **polkavm-assembler**: Assembly support
- **simplealloc**: Service memory allocator

### Guest Programs
- **Location**: `guest-programs/`
- **Purpose**: Examples and build infrastructure
- **Includes**: Benchmarks, tests, and example programs

## Prerequisites

- **Rust Nightly**: Required for building services
- **RISC-V Target**: Services compile to `riscv64emac-unknown-none-polkavm`
- **Build Tools**: Standard Rust toolchain with `cargo`

### Toolchain Setup

```bash
# Install nightly Rust (if not already installed)
rustup install nightly

# The RISC-V target configuration is included in this workspace
# No additional target installation required
```

## Workspace Structure

```
services/
├── Cargo.toml              # Workspace configuration
├── tools/
│   ├── polkatool/          # Service build tool
│   └── spectool/           # Service analysis tool
├── crates/
│   ├── polkavm*/           # Complete PolkaVM ecosystem
│   └── simplealloc/        # Service allocator
├── guest-programs/         # Build examples and infrastructure
├── evm/                    # Ethereum Virtual Machine service
├── algo/                   # Algorithmic computation service
├── bootstrap/              # Bootstrap service
├── auth_copy/              # Authentication service
├── null_authorizer/        # Null authorization service
├── doom/                   # Doom game service
└── utils/                  # Shared utilities
```

## Development

### Adding a New Service

1. Create a new directory: `mkdir my_service`
2. Add `Cargo.toml` with service configuration
3. Implement service logic in `src/main.rs`
4. Add to workspace members in root `Cargo.toml`
5. Build with: `make my_service` (from main repo) or `cargo build -p my_service`

### Service Dependencies

Services typically depend on:
- `polkavm-derive`: Procedural macros for PolkaVM
- `simplealloc`: Memory allocation
- `utils`: Shared service utilities
- `primitive-types`: Common data types

Example `Cargo.toml`:
```toml
[package]
name = "my_service"
version = "0.1.0"
edition = "2021"

[[bin]]
name = "my_service"
path = "src/main.rs"

[dependencies]
polkavm-derive = { workspace = true }
simplealloc = { workspace = true }
utils = { path = "../utils" }
```

## Troubleshooting

### Build Issues

1. **"target not found"**: Ensure you're using the correct target JSON path
2. **"polkatool not found"**: Run from main repo with `make {service}` or from services directory with full cargo commands
3. **Dependency errors**: Run `cargo clean` and rebuild

### Performance

- Services are built in release mode by default for optimal performance
- LTO (Link Time Optimization) is disabled to support the PolkaVM target
- Debug information is included for development

## Integration

This services workspace is designed to be:
- **Self-contained**: No external dependencies on main repository
- **Portable**: Can be used in other JAM implementations
- **Modular**: Individual services can be built independently

### Using as a Git Submodule

For integration into other projects, add this repository as a git submodule:

```bash
# Add as submodule
git submodule add https://github.com/jam-duna/services.git services

# Initialize and update
git submodule update --init --recursive

# Build services from your project
cd services && make all
```

### Standalone Usage

You can also clone this repository directly for standalone service development:

```bash
# Clone the services repository
git clone https://github.com/jam-duna/services.git
cd services

# Build all services
make all

# Or build individual services
make evm
make algo
```

The self-contained Makefile makes it easy to integrate JAM services into any project without complex dependency management.