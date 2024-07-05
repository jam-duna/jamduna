# Bandersnatch library

We use a FFI to davxy's IETF + Ring VRF library. 


Rust:
```
# cargo test  -- --nocapture
    Finished test [unoptimized + debuginfo] target(s) in 0.04s
     Running unittests src/lib.rs (target/debug/deps/ark_ec_vrfs_bandersnatch_example-377ddbe1c5fff8c9)

running 1 test
Ring signature verified
 vrf-output-hash: 6b260bfda2e3ef118c529f30b60dfa4678fbeef3682b55ba002aa8633f1b0364
Ietf signature verified
 vrf-output-hash: 6b260bfda2e3ef118c529f30b60dfa4678fbeef3682b55ba002aa8633f1b0364
test tests::test_vrf_signatures ... ok

test result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 18.43s

     Running unittests src/main.rs (target/debug/deps/ark_ec_vrfs_bandersnatch_example-8f42f9a474bb1331)

running 0 tests

test result: ok. 0 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.00s
```

Go:
```
# go test
Verification result: 1
VRF output hash: 6b260bfda2e3ef118c529f30b60dfa4678fbeef3682b55ba002aa8633f1b0364
PASS
ok  	github.com/colorfulnotion/jam/crypto	0.131s
```

Next steps:
* situate the above inside `safrole`
* pass tests cases when ready

### How it works:
1. Cargo.toml 
```
[lib]
crate-type = ["cdylib"]
```

where building the library generates `libbandersnatch_vrfs.so` in `target/release` (This can also be done with "make bandersnatch")

# cargo build --release
Finished release [optimized] target(s) in 0.32s

# ls -l target/release/*.so
-rwxr-xr-x 2 root root 4536904 Jun 19 16:19 target/release/libbandersnatch_vrfs.so
```

2. To use the above library in `target/release`:
```
export LD_LIBRARY_PATH=$(pwd)/target/release:$LD_LIBRARY_PATH
```

3. Then this works:

```
# go test
(see above)
```



