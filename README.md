# JAM

*Developer*: Colorful Notion, Inc.
*Address*: [5D58imQFuMXDTknQS2D14gDU2duiUC18MGxDnTKajjJS9F3g](https://polkadot.subscan.io/account/5D58imQFuMXDTknQS2D14gDU2duiUC18MGxDnTKajjJS9F3g)

We are developing [JAM](https://jam.web3.foundation/) in Go in Summer 2024/Fall 2024. 

**Goal**: Achieve **Milestone 1: IMPORTER** -- State-transitioning conformance tests pass and can import blocks.

For background on JAM, see [this](https://wiki.polkadot.network/docs/learn-jam-chain) and the [JAM Gray paper](https://graypaper.com/)


## Building JAM

```
# make jam
Building JAM...
mkdir -p bin
go build -o bin/jam jam.go
```

## Running JAM

```
# bin/jam -h
Usage: jam [options]
  -bandersnatch string
    	Bandersnatch Seed (only for development)
  -bls string
    	BLS private key (only for development)
  -datadir string
    	Specifies the directory for the blockchain, keystore, and other data. (default "/root/.jam")
  -ed25519 string
    	Ed25519 Seed (only for development)
  -genesis string
    	Specifies the genesis state json file.
  -h	Displays help information about the commands and flags.
  -metadata string
    	Node metadata (default "Alice")
  -port int
    	Specifies the network listening port. (default 9900)
  -ts int
    	Epoch0 Unix timestamp (will override genesis config)
  -validatorindex int
    	Validator Index (only for development)
```

## Libraries used

* [bandersnatch-vrfs-spec](https://github.com/davxy/bandersnatch-vrfs-spec) in `crypto`







