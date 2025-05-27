## jamduna

Generate a spec with:

```
bin/jamduna gen-spec chainspecs/dev-config.json chainspecs/jamduna-spec.json
```


## polkajam

* [polkajam-releases](https://github.com/paritytech/polkajam-releases/releases)
* [dev-config.json](https://gist.github.com/zdave-parity/72eb9cfe07756d2c0c13c3064600190d) 
 is quite limited in what it can control; most of the state is just fixed by gen-spec. 

* `dev-spec.json` [JIP-4](https://github.com/polkadot-fellows/JIPs/pull/1) you pass to polkajam like: polkajam --chain dev-spec.json


In particular, gen-spec always includes a null authorizer (https://crates.io/crates/jam-null-authorizer) and a bootstrap service (https://crates.io/crates/jam-bootstrap-service)

Options:

* With `RUST_LOG=jam_node::net=trace` you should get a log message printed containing the expected application protocol

``
2025-05-15 15:13:41 main TRACE jam_node::net  Using application protocol jamnp-s/0/0259fbe9
```

* Force the interpreter backend and set `RUST_LOG=polkavm=trace`

```
2025-05-22 20:22:42 tokio-runtime-worker DEBUG polkavm::interpreter  Compiling block:
2025-05-22 20:22:42 tokio-runtime-worker DEBUG polkavm::interpreter    [4]: 8206: charge_gas
2025-05-22 20:22:42 tokio-runtime-worker DEBUG polkavm::interpreter    [5]: 8206: sp = sp + 0xfffffffffffffd00
2025-05-22 20:22:42 tokio-runtime-worker DEBUG polkavm::interpreter    [6]: 8210: u64 [sp + 0x2f8] = ra
2025-05-22 20:22:42 tokio-runtime-worker DEBUG polkavm::interpreter    [7]: 8214: u64 [sp + 0x2f0] = s0
2025-05-22 20:22:42 tokio-runtime-worker DEBUG polkavm::interpreter    [8]: 8218: u64 [sp + 0x2e8] = s1
2025-05-22 20:22:42 tokio-runtime-worker DEBUG polkavm::interpreter    [9]: 8222: i32 s0 = a0 + 0
2025-05-22 20:22:42 tokio-runtime-worker DEBUG polkavm::interpreter    [10]: 8224: jump 9459 if s0 == 0
2025-05-22 20:22:42 tokio-runtime-worker TRACE polkavm::interpreter::raw_handlers    -> resolved to fallthrough
2025-05-22 20:22:42 tokio-runtime-worker TRACE polkavm::interpreter::raw_handlers  [4]: charge_gas: 6 (49999999 -> 49999993)
```

 