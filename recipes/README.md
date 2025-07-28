
### Compiling with Revive

```
# ./resolc-x86_64-unknown-linux-musl MyToken.sol 
Compiler run successful. No output requested. Use --asm and --bin flags.
# ./resolc-x86_64-unknown-linux-musl MyToken.sol --asm > MyToken.asm
# ./resolc-x86_64-unknown-linux-musl MyToken.sol --bin > MyToken.bin
```

1. Take 40-50 "algo" programs in Rust, turn them into Solidity, then with the above into PVM and then run them with our PVM recompiler.

2. For N iterations (1K, 10K, 100K, 1MM), how much EVM gas does it take?

