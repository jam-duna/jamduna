# PolkaVM Linker Target Specification

## riscv64emac-unknown-none-polkavm.json

This is the Rust target specification file for PolkaVM (RISC-V 64-bit).

### IMPORTANT: Format Requirements (Rust ≥1.91)

**Both fields MUST be numbers (no quotes):**

- `"max-atomic-width": 64` ← number
- `"target-pointer-width": 64` ← number (changed from string in Rust 1.91)

### Why This Matters

The Rust target spec format changed in Rust 1.91 ([PR #144443](https://github.com/rust-lang/rust/pull/144443)):

- **Rust ≤1.90**: `"target-pointer-width": "64"` (string)
- **Rust ≥1.91**: `"target-pointer-width": 64` (number/u16)

This project's Makefile uses `cargo +nightly`, which means:
- **Different nightly versions = different format requirements**
- **Rust nightly 1.87** (old): requires string
- **Rust nightly 1.92** (current): requires number

**Solution**: Upgrade all machines to **Rust nightly ≥1.91** with `rustup update nightly`

### If You See Build Failures

**Error 1**: `target-pointer-width: invalid type: string "64", expected u16`
- **Cause**: Your Rust nightly is ≥1.91, but JSON has string format
- **Fix**: Use number format (remove quotes)

**Error 2**: `Field target-pointer-width in target specification is required`
- **Cause**: Your Rust nightly is ≤1.90, but JSON has number format
- **Fix**: Run `rustup update nightly` to upgrade to 1.91+

**Correct format for Rust ≥1.91**:
```json
"max-atomic-width": 64,
"target-pointer-width": 64,
```

### Rust Version Requirements

This configuration requires **Rust nightly ≥1.91**.

Tested with:
```
rustc 1.92.0-nightly (f6aa851db 2025-10-07)
```

If you have an older nightly (<1.91), upgrade:
```bash
rustup update nightly
```

### References

- [Rust Target Specification Documentation](https://doc.rust-lang.org/rustc/targets/custom.html)
- [PolkaVM Repository](https://github.com/koute/polkavm)
