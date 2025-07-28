# jamduna

*Developer*: Colorful Notion, Inc.
*Address*: [5D58imQFuMXDTknQS2D14gDU2duiUC18MGxDnTKajjJS9F3g](https://polkadot.subscan.io/account/5D58imQFuMXDTknQS2D14gDU2duiUC18MGxDnTKajjJS9F3g)

### JAM Docs:

[docs.jamcha.in](https://docs.jamcha.in/) | [graypaper](https://graypaper.fluffylabs.dev/#/293bf5a/348e00348e00) | [JAM-NP](https://github.com/zdave-parity/jam-np/blob/main/simple.md) |  [JAM SDK](https://hackmd.io/@polkadot/jamsdk)


## Quickstart Guide

```bash
% jamduna -h
JAM DUNA node

Usage:
  ./jamduna [command]

Available Commands:
  gen-keys    Generate keys for validators, pls generate keys for all validators before running the node
  gen-spec    Generate new chain spec from the spec config
  help        Help about any command
  list-keys   List keys for validators
  print-spec  Generate new chain spec from the spec config
  run         Run the JAM DUNA node
  test-refine Run the refine test
  test-stf    Run the STF Validation

Flags:
  -c, --config string      Path to the config file
  -h, --help               Displays help information about the commands and flags.
  -l, --log-level string   Log level (debug, info, warn, error) (default "trace")
  -t, --temp               Use a temporary data directory, removed on exit. Conflicts with data-path
  -v, --version            Prints the version of the program.

Use "./jamduna [command] --help" for more information about a command.
```

## Run a Single Local Node

```
./jamduna run --chain chainspecs/jamduna-spec.go --dev-validator 0
```

## Launch a Local "Tiny" Testnet

```bash
rm -rf ~/.jamduna; \
bin/jamduna gen-keys \
for i in $(seq 0 5); do \
  bin/jamduna run --chain chainspecs/jamduna-spec.json --dev-validator "$i"  & \
done; \
```

## Shutdown All Local Nodes

```bash
pkill -f jamduna
```


## Disclosures

No one on our team viewed any other team's JAM implementations code, but we wish to summarize our interactions with other teams for completeness.

Our interactions with fellow teams concerning implementation are:
* Public [JAM Telegram room](https://t.me/jamtestnet) wherein many JAM Implementers are sharing published blocks/state transitions and Discord room;  A few conversations with Jason @ JavaJAM to review how to participate in a [tiny testnet](https://github.com/jam-duna/jamtestnet/issues/69); The #implementers room in the JAM DAO discord.  
* Public [JAM DUNA JAM Testnet repo](https://github.com/jam-duna/jamtestnet/releases) on resolving discrepancies on published blocks/state_transitions/state_snapshot in the GP 0.5.x to 0.6.x series.  This was enormously valuable to fix interpretation issues.
* [JAM0 event at Devcon 7, sponsored by Colorful Notion](https://forum.polkadot.network/t/jam0-jam-implementers-meetup-sub0-devcon-7-bangkok-nov-11-nov-16-2024/10866)
* We built many services using a fork of koute/polkavm [here](https://github.com/colorfulnotion/)
* Public conversations with @koute concerning using polkatool over github to build our first services (see [here](https://forum.polkadot.network/t/building-jam-services-in-rust/10161)) and a few private DMs concerning toolchain usage and in-person discussion in May concerning CoreEVM service development
* Many critically helpful public conversations with Polkajam team leads (Dave Emett, davxy, koute) in Let's JAM.
* Generative AI usage restricted to: learning how to FFI into Rust (bandersnatch + bls + erasure coding package), in-line documentation + PR summary and basic debugging (eg fmt.Printf auto generation), learning about QUIC+X86 opcodes, building fuzzer tests.
