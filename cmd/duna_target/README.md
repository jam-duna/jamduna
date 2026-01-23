# Publishing the fuzzer target and fuzzer to jamtestnet

0. Check DEFAULT_ASN_VERSION in duna_bench, duna_target, duna_fuzzer
```
DEFAULT_ASN_VERSION = 1
```

1. Be sure to change PATCH_VERSION in fuzzer_asn.go to the latest version 
```
PATCH_VERSION = 9

# JAM_VERSION = <GP_VERSION>.<PATCH_VERSION>
GP_VERSION = 0.7.2
JAM_VERSION = 0.7.2.9
```

2. Make the fuzzer with DEFAULT_ASN_VERSION

```
cd duna_fuzzer
make 
```

3. Make the fuzzer target

```
cd duna_target
make 
```

4. Test it out with:

Run the target:
```
./duna_target_mac 
./duna_target_mac --pvm-logging debug
```

Run the fuzzer with some test directory:
```
./duna_fuzzer_mac --test-dir ~/Github/jamtestnet/0.7.2/jam-conformance 
./duna_fuzzer_mac --test-dir ~/Github/jam/jamtestvectors/traces/preimages
./duna_fuzzer_mac --test-dir ~/Github/jam/jamtestvectors/traces/storage
```

Check that our target has no issue!

5. After changing the version in the command below to match up, publish to https://github.com/jam-duna/jamtestnet

**Note:** Only the Linux target is published. The binary uses the compiler backend for better performance.

```
gh release create v0.7.2.9 \
  $JAM_PATH/cmd/duna_target/duna_target_linux \
  --repo jam-duna/jamtestnet \
  --title "v0.7.2.9 Target" \
  --notes $'Release v0.7.2.9 of duna_target\n\nLinux binary built with compiler backend for optimal performance.\n\n**Published binary:**\n- duna_target_linux'
```


