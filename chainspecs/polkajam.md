
## First Contact with Polkajam

Linux binaries here: https://drive.google.com/file/d/1q_v-YoVUd21ZHKWQcFGlLgR04H5n2uB-/view

dev-config.json + dev-spec.json here: https://gist.github.com/zdave-parity/72eb9cfe07756d2c0c13c3064600190d

### Notes from Dave Emett

* `dev-spec.json` [JIP-4](https://github.com/polkadot-fellows/JIPs/pull/1) you pass to polkajam like: polkajam --chain dev-spec.json

* `dev-config.json` is quite limited in what it can control; most of the state is just fixed by gen-spec. 


In particular, gen-spec always includes a null authorizer (https://crates.io/crates/jam-null-authorizer) and a bootstrap service (https://crates.io/crates/jam-bootstrap-service)

* You can pass `polkajam --chain <path-to-chainspec-file>`

* With `RUST_LOG=jam_node::net=trace` you should get a log message printed containing the expected application protocol

``
2025-05-15 15:13:41 main TRACE jam_node::net  Using application protocol jamnp-s/0/0259fbe9
```

 