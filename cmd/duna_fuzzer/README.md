# Publishing the fuzzer

1. Be sure to change the `PeerInfo` version (e.g. "jam-duna-fuzzer-v0.13") in duna_fuzzer.go

2. Run this after changing the version to publish to https://github.com/jam-duna/jamtestnet

```
gh release create v0.6.7.13 \
  /Users/michael/Desktop/jam/cmd/duna_fuzzer/duna_fuzzer_linux \
  /Users/michael/Desktop/jam/cmd/duna_fuzzer/duna_fuzzer_mac \
  --repo jam-duna/jamtestnet \
  --title "v0.6.7.13" \
  --notes $'Release v0.6.7.13 of jamduna-fuzzer.\n\nIncludes:\n- duna_fuzzer_mac\n- duna_fuzzer_linux'
  ```