# Publishing the fuzzer target and fuzzer to jamtestnet


1. Be sure to change FUZZER_VERSION in  duna_fuzzer.go to the latest version 
```
FUZZ_VERSION = "0.7.0.6"
```

2. Make the fuzzer

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
./duna_fuzzer_mac --test-dir ~/Github/jamtestnet/0.7.0/jam-conformance 
./duna_fuzzer_mac --test-dir ~/Github/jam/jamtestvectors/traces/preimages
./duna_fuzzer_mac --test-dir ~/Github/jam/jamtestvectors/traces/storage
```

Check that our target has no issue!

5. After changing the version in the command below to match up, publish to https://github.com/jam-duna/jamtestnet

```
gh release create v0.7.0.6 \
  $JAM_PATH/cmd/duna_fuzzer/duna_fuzzer_linux \
  $JAM_PATH/cmd/duna_fuzzer/duna_fuzzer_mac \
  $JAM_PATH/cmd/duna_target/duna_target_linux \
  $JAM_PATH/cmd/duna_target/duna_target_mac \
  --repo jam-duna/jamtestnet \
  --title "v0.7.0.6 Fuzzer + Fuzzer target" \
  --notes $'Release v0.7.0.6 of duna_fuzzer and duna_target\n\nIncludes:\n- duna_fuzzer_mac\n- duna_fuzzer_linux\n- duna_target_mac\n- duna_target_linux'  
```


