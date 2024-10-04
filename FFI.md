

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


Linux:
```
cd bls
cargo build --release
cd ..
cd bandersnatch
cargo build --release
cd ..
```

Mac: same as above but add [TODO] to both `cargo build` instructions
PC: same as above but add [TODO] to both `cargo build` instructions





