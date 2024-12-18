# JAM

*Developer*: Colorful Notion, Inc.
*Address*: [5D58imQFuMXDTknQS2D14gDU2duiUC18MGxDnTKajjJS9F3g](https://polkadot.subscan.io/account/5D58imQFuMXDTknQS2D14gDU2duiUC18MGxDnTKajjJS9F3g)

We are developing [JAM](https://jam.web3.foundation/) in Go in Summer 2024/Fall 2024. 

**Goal**: Achieve **Milestone 1: IMPORTER** -- State-transitioning conformance tests pass and can import blocks.

## Documentation:

* [docs.jamcha.in](https://docs.jamcha.in/)
* [graypaper](https://graypaper.fluffylabs.dev/#/293bf5a/348e00348e00)
* [JAM Setup Service Guide](https://hackmd.io/@clw98/Hy5xvMYxJg)
* [JAM Trustless Supercomputing Test Suite](https://hackmd.io/nk0Tr0iIQHmLm7WIXe_OoQ)
* [JAM TestNet repo](https://github.com/jam-duna/jamtestnet/tree/main/traces/assurances/jam_duna) | [sheet](https://docs.google.com/spreadsheets/d/1ueAisCMOx7B-m_fXMLT0FXBxfVzydJyr-udE8jKwDN8/edit?gid=615049643#gid=615049643)
* [JAM-NP](https://github.com/zdave-parity/jam-np/blob/main/simple.md)
* [JAM SDK](https://hackmd.io/@polkadot/jamsdk)

Supporting docs:

* [JAM DA](https://hackmd.io/NDwqmV_XTjukvCxC98kLmg?view) - Stanley
* [Discussion topics](https://hackmd.io/2y70ehKYS3aLzKvqZPWhog?edit)
* [RAM Model](https://hackmd.io/cAXPsZt1StWI4dbPk_UWAQ?view) - William
* [Parent-Child VMs](https://hackmd.io/ldPJih0ISMCP6pU5aXzaWg)  - Shawn
* [Ordered Accumulation](https://hackmd.io/jeZLW09nRse8q3t_PavyrA) - Stanley
* [Recent History Test](https://hackmd.io/H_vBBOR-RS-r3tdpS2AHOA) - Stanley
* [JAM0 @ Bangkok](https://hackmd.io/qA7NNyjyQIil8oSq3aafjw) - Sourabh

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







