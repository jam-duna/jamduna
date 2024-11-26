# DA Test 

## Overview

This project tests the functionality of Data Availability (DA) in JAM, with binary in a tiny (V=6) network and then in a large (V=384) network in prep for the JAM Toaster setup.

## Features

- **Validator Network Setup**: Dynamically generates a validator network with metadata and cryptographic keys.
- **Genesis Configuration**: Supports custom genesis state through JSON files or defaults.
- **Node Initialization**: Initializes a node for DA simulation with peer networking and secrets.
- **Simulation Execution**: Runs a DA simulation loop.
- **Command-line Configuration**: Customizable parameters such as port, genesis file, timestamps, and validator details.

## Usage

### Running the Simulation

To run the DA test, use the following command:

```bash
# compiles
make da
# launches tiny network
make da_test
```

### Options

```
-h	Displays help information.	N/A
-datadir	Directory for blockchain, keystore, and other data.	~/.jam
-port	Network listening port.	9900
-ts	Unix timestamp for Epoch0.	Next multiple of 12 seconds from now.
-validatorindex	Validator index for development.	0
-genesis	Path to genesis state JSON file.	N/A
-ed25519	Ed25519 private key seed (development only).	N/A
-bandersnatch	Bandersnatch seed (development only).	N/A
-bls	BLS private key (development only).	N/A
-metadata	Node metadata.	Alice
```

## Docker

After the V=6 setup, we will test on V=384 in Docker.