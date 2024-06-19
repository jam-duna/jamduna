# Ring VRF library

We use a FFI to the W3F Ring Library.  At present, both sign and verify is in `RingVRFSign` while `RingVRFVerify` is just a stub.  

```go
        signature, err := RingVRFSign(secret, domain, message, transcript)
        if err != nil {
                t.Fatalf("Failed to sign: %v", err)
	}
        fmt.Printf("Signature: %x\n", signature)


        err = RingVRFVerify(pks, pubkey, domain, message, transcript, signature)
        if err != nil {
		t.Fatalf("Failed to verify: %v", err)
        }
        t.Logf("Verified signature")
```

The test runs through both:
```
# go test
Signature: a1e29fc1bf91dd404ab2655026068f033c0125a65586b1d3204c6ae1ddcb241d007042f1043a071f8d63d53b50630e3e3e563f488add2af5f767920040a7110d04874b47f5ccc69cf3bc32cc903327dd6968a8dbc839b8cdc0aaea2fd1728b2d0ac58ee84cbced172ef51bc3f09c00bee1aec724f52646051edd4b2afb7d875a09008d4e34e061f6e08a1791402da52f1d74891c9aa530306dfa9b1009927fe5062e80
PASS
ok  	github.com/colorfulnotion/jam/ring-vrf	0.799s
```

Next steps
* get RingVRFVerify to work, and figure out how randomness/entropy gets passed in
* situate the above inside `safrole`
* pass tests cases when ready


### How it works:
1. Cargo.toml of bandersnatch_vrfs has the key library generation directive 
```
[lib]
crate-type = ["cdylib"]
```

where building the library generates `libbandersnatch_vrfs.so` in `target/release` (This can also be done with "make bandersnatch")

```
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



