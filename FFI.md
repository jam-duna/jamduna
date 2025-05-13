

# Setup

In your .bashrc (or equivalent) file put this
```
export JAM_PATH=/root/go/src/github.com/colorfulnotion/jam
export LD_LIBRARY_PATH=$JAM_PATH/bls/target/release:$JAM_PATH/bandersnatch/target/release
export CARGO_MANIFEST_DIR=$JAM_PATH/bandersnatch
```
Adjust `JAM_PATH` to match where you have the `jam` repo.  There is nothing special about "/root/go/src" part of `JAM_PATH` but the "github.com/colorfulnotion/jam" part is important and should _not_ be deviated from.  

* The `LD_LIBRARY_PATH` is where static crypto libraries ".a" will go (see "Building FFI libraries")
* The `CARGO_MANIFEST_DIR` is where the `bandersnatch` library expects to find the zcash.bin files, which are used in RingVRF setup


# Building FFI libraries

For FFI, we have 2 go packages (`bandersnatch`, `bls`) and might have something useful from the erasure coding later, all in Rust.

Rust has Cargo.toml specifying _static_ library usage via

```
[lib]
crate-type = ["staticlib"]
```

in both packages.  This `staticlib` choice of `crate-type`, when we compile a `jam` binary, we do *NOT* need to also copy over a bunch of `.so` files (if `cdylib` was there instead).

Rust [Tier 1 Target List](https://doc.rust-lang.org/nightly/rustc/platform-support.html)

| Target                      | Notes                                           |  
|-----------------------------|-------------------------------------------------|
| aarch64-unknown-linux-gnu   | ARM64 Linux (kernel 4.1, glibc 2.17+)           |
| aarch64-apple-darwin        | ARM64 macOS (11.0+, Big Sur+)                   |
| i686-pc-windows-gnu         | 32-bit MinGW (Windows 10+, Windows Server 2016+)|
| i686-pc-windows-msvc        | 32-bit MSVC (Windows 10+, Windows Server 2016+) |
| i686-unknown-linux-gnu      | 32-bit Linux (kernel 3.2+, glibc 2.17+)         |
| x86_64-apple-darwin         | 64-bit macOS (10.12+, Sierra+)                  |
| x86_64-pc-windows-gnu       | 64-bit MinGW (Windows 10+, Windows Server 2016+)|
| x86_64-pc-windows-msvc      | 64-bit MSVC (Windows 10+, Windows Server 2016+) |
| x86_64-unknown-linux-gnu    | 64-bit Linux (kernel 3.2+, glibc 2.17+)         |

Set Default host target:
```
rustup set default-host <target>
```

Mac Apple Sillicon is using `aarch64-apple-darwin`; Linux is using `x86_64-unknown-linux-gnu`

Verify Default Target with rustup:

# Windows (x86_x64):
```
# Setting target for PC
$ rustup set default-host x86_64-pc-windows-gnu
```

# macOS (Apple Sillicon):
```
# Setting target as Mac Apple Sillicon (M1,M2,.)
$ rustup set default-host aarch64-apple-darwin

# Verifying Target
$ rustup show
Default host: aarch64-apple-darwin
rustup home:  /Users/michael/.rustup

installed toolchains
--------------------
stable-aarch64-apple-darwin (default)

active toolchain
----------------
stable-aarch64-apple-darwin (default)
rustc 1.81.0 (eeb90cda1 2024-09-04)

# Checking rustc setting
$ rustc --version --verbose
rustc 1.81.0 (eeb90cda1 2024-09-04)
binary: rustc
commit-hash: eeb90cda1969383f56a2637cbd3037bdf598841c
commit-date: 2024-09-04
host: aarch64-apple-darwin
release: 1.81.0
LLVM version: 18.1.7
```
# Linux:
```
# Setting target for 64-bit Linux
$ rustup set default-host x86_64-unknown-linux-gnu

# Verifying Target x86_64
$ rustup show
Default host: x86_64-unknown-linux-gnu
rustup home:  /home/ntust/.rustup

stable-x86_64-unknown-linux-gnu (default)
rustc 1.81.0 (eeb90cda1 2024-09-04)

# Checking rustc setting
$ rustc --version --verbose
rustc 1.81.0 (eeb90cda1 2024-09-04)
binary: rustc
commit-hash: eeb90cda1969383f56a2637cbd3037bdf598841c
commit-date: 2024-09-04
host: x86_64-unknown-linux-gnu
release: 1.81.0
LLVM version: 18.1.7
```

# Building FFI via MakeFile
```
$ make ffi
Building BLS...
cd bls && cargo build --release
warning: `bls` (lib) generated 7 warnings (run `cargo fix --lib -p bls` to apply 7 suggestions)
    Finished `release` profile [optimized] target(s) in 0.09s
Built BLS library!

Building Bandersnatch...
cd bandersnatch && cargo build --release

warning: `bandersnatch` (lib) generated 9 warnings (run `cargo fix --lib -p bandersnatch` to apply 4 suggestions)
    Finished `release` profile [optimized] target(s) in 0.07s
Built Bandersnatch library!
```


# Cross Platform Build
```
# For Linux (musl = static)
rustup target add x86_64-unknown-linux-musl
rustup target add aarch64-unknown-linux-musl

# For macOS (cross-compiling from macOS to x86_64 macOS is not always default)
rustup target add x86_64-apple-darwin
rustup target add aarch64-apple-darwin

# For Windows
rustup target add x86_64-pc-windows-gnu

# Check Installed Platforms
rustup target list --installed
```

# JamDuna Binaries

| Target Name                | GOOS     | GOARCH | Output Binary Path                 | Matches Official Binary       |
|---------------------------|----------|--------|------------------------------------|--------------------------------|
| `static_jam_linux_amd64`  | `linux`  | `amd64`| `bin/jamduna-linux-amd64`          | ✅ Linux (x86_64)              |
| `static_jam_linux_arm64`  | `linux`  | `arm64`| `bin/jamduna-linux-arm64`          | ✅ Linux (aarch64)             |
| `static_jam_darwin_amd64` | `darwin` | `amd64`| `bin/jamduna-mac-amd64`            | ✅ macOS (x86_64)              |
| `static_jam_darwin_arm64` | `darwin` | `arm64`| `bin/jamduna-mac-arm64`            | ✅ macOS (aarch64)             |
| `static_jam_windows_amd64`| `windows`| `amd64`| `bin/jamduna-windows-amd64.exe`    | ✅ Windows(⚠Experimental)      |