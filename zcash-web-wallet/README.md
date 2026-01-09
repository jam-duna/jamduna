# Zcash Web Wallet

[![CI](https://github.com/LeakIX/zcash-web-wallet/actions/workflows/ci.yml/badge.svg)](https://github.com/LeakIX/zcash-web-wallet/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/LeakIX/zcash-web-wallet/graph/badge.svg)](https://codecov.io/gh/LeakIX/zcash-web-wallet)

A privacy-preserving Zcash wallet toolkit that runs entirely in your browser.
Generate wallets, view shielded transactions, and track balances - all
client-side with no server dependencies.

## Features

- **Wallet Generation**: Create and restore Zcash testnet wallets with BIP39
  seed phrases
- **Transaction Viewer**: Decode shielded transaction details using viewing keys
- **CLI Tool**: Command-line wallet generation and note/balance tracking
- **Privacy First**: All cryptographic operations happen locally - keys never
  leave your device
- **Orchard Support**: Full support for the latest Orchard shielded pool

## Quick Start

### Web Interface

```bash
make install    # Install dependencies
make build      # Build WASM module and Sass
make serve      # Serve on http://localhost:3000
```

Note: On macOS, `make build` requires Homebrew LLVM (`brew install llvm`) so
`secp256k1-sys` can compile for `wasm32-unknown-unknown`.

### macOS Troubleshooting

- **GNU sed required**: The Makefile expects `gsed` on macOS.
  - Install with `brew install gnu-sed`, then run `make serve`.
  - If you just want to view the UI without rebuilding, you can serve the
    existing frontend directly:

```bash
cd frontend
python3 -m http.server 3000
```

- **npm cache permission error**: If Sass fails with an `EACCES` error, use a
  local npm cache directory:

```bash
mkdir -p .npm-cache
NPM_CONFIG_CACHE=$PWD/.npm-cache make serve
```

From the repo root, you can run `make -C zcash-web-wallet serve`.

### CLI Tool

```bash
make build-cli  # Build the CLI

# Generate a new testnet wallet
./target/release/zcash-wallet generate --output wallet.json

# Restore from seed phrase
./target/release/zcash-wallet restore --seed "your 24 words here" --output wallet.json

# Get testnet faucet instructions
./target/release/zcash-wallet faucet
```

## JAM Service Address Derivation (Rust + Go)

This uses the public dev seed and treats `serviceID` as the ZIP-32 account
index. Both snippets derive the same unified shielded address at address index
0 for service IDs 1 and 2.

Dev seed (24 words):

```
abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon art
```

### Rust (direct ZIP-32 via `zcash_wallet_core`)

```rust
use std::error::Error;
use zcash_primitives::consensus::Network;
use zcash_wallet_core::restore_wallet;

const DEV_SEED: &str = "abandon abandon abandon abandon abandon abandon abandon abandon \
abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon \
abandon abandon abandon abandon abandon abandon art";

fn main() -> Result<(), Box<dyn Error>> {
    let network = Network::TestNetwork; // switch to Network::MainNetwork if needed

    for service_id in [1u32, 2u32] {
        let wallet = restore_wallet(DEV_SEED, network, service_id, 0)?;
        let ua = wallet
            .unified_address
            .ok_or("missing unified address")?;
        println!("service_id={} ua={}", service_id, ua);
    }

    Ok(())
}
```

### Go (call the Rust CLI for exact parity)

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"
    "os/exec"
    "strconv"
)

const devSeed = "abandon abandon abandon abandon abandon abandon abandon abandon " +
    "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon " +
    "abandon abandon abandon abandon abandon abandon art"

type walletJSON struct {
    UnifiedAddress string `json:"unified_address"`
}

func deriveUA(serviceID uint32) (string, error) {
    tmp, err := os.CreateTemp("", "jam-wallet-*.json")
    if err != nil {
        return "", err
    }
    defer os.Remove(tmp.Name())

    cmd := exec.Command(
        "zcash-web-wallet/target/release/zcash-wallet",
        "restore",
        "--seed", devSeed,
        "--account", strconv.FormatUint(uint64(serviceID), 10),
        "--address-index", "0",
        "--output", tmp.Name(),
    )
    // add "--mainnet" if you want mainnet
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil {
        return "", err
    }

    data, err := os.ReadFile(tmp.Name())
    if err != nil {
        return "", err
    }

    var w walletJSON
    if err := json.Unmarshal(data, &w); err != nil {
        return "", err
    }
    return w.UnifiedAddress, nil
}

func main() {
    for _, sid := range []uint32{1, 2} {
        ua, err := deriveUA(sid)
        if err != nil {
            panic(err)
        }
        fmt.Printf("service_id=%d ua=%s\n", sid, ua)
    }
}
```

## Ledger Zcash Support (Overview)

Ledger keeps the Zcash seed on-device and derives keys using the Zcash BIP32/ZIP-32
path (coin type 133). The host wallet builds the transaction and sends signing
data to the device; the device signs after on-screen confirmation. Transparent
(t-address) signing is the baseline support. Shielded support depends on the
Ledger app/version: Sapling may be supported; Orchard is typically not. When
shielded support exists, the host still generates proofs while the device signs
spend authorization.

## Development

```bash
make test       # Run all tests (Rust + e2e)
make lint       # Lint all code (clippy, prettier, shellcheck)
make format     # Format all code
make help       # Show all available commands
```

## Architecture

```
Browser                              Zcash Node
   |                                     |
   |  1. User enters txid + viewing key  |
   |  2. Fetch raw tx via RPC            |
   |------------------------------------>|
   |  3. Raw transaction hex             |
   |<------------------------------------|
   |  4. WASM decrypts locally           |
   |     (keys never leave browser)      |
```

## Project Structure

- `core/` - Shared Rust library for wallet derivation
- `wasm-module/` - Rust WASM library for browser-based operations
- `cli/` - Command-line wallet and note tracking tool
- `frontend/` - Web interface (Bootstrap + vanilla JS)

## License

MIT
