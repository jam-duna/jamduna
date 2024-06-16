# JAM

*Developer*: Colorful Notion, Inc.
*Address*: [5D58imQFuMXDTknQS2D14gDU2duiUC18MGxDnTKajjJS9F3g](https://polkadot.subscan.io/account/5D58imQFuMXDTknQS2D14gDU2duiUC18MGxDnTKajjJS9F3g)

We are developing [JAM](https://jam.web3.foundation/) in Go in Summer 2024/Fall 2024. 

**Goal**: Achieve **Milestone 1: IMPORTER** -- State-transitioning conformance tests pass and can import blocks.

For background on JAM, see [this](https://wiki.polkadot.network/docs/learn-jam-chain) and the [JAM Gray paper](https://graypaper.com/)

## Request for Fellowship Candidacy

Assuming we succeed at passing Milestone 1, we request Polkadot Fellowship candidacy of:
* Sourabh Niyogi (Level 3)
* Michael Chung (Level 2)

## Declarations 

In our implementation, beyond the Graypaper, we relied on the following for either FFIs or our learning of JAM:
 1. [Koute's PVM Implementation](https://github.com/w3f/ring-vrf) (no FFI)
 2. [W3F Ring VRF](https://github.com/w3f/ring-vrf) (FFI)
 3. Polkadot SDK: [Sassafras](https://github.com/paritytech/polkadot-sdk/blob/ae0b3bf6733e7b9e18badb16128a6b25bef1923b/polkadot/erasure-coding/src/lib.rs) + [RFC #26](https://github.com/polkadot-fellows/RFCs/pull/26) (no FFI)
 4. Polkadot-SDK: [Grandpa](https://github.com/paritytech/polkadot-sdk/blob/ae0b3bf6733e7b9e18badb16128a6b25bef1923b/substrate/client/consensus/grandpa/src/lib.rs) (no FFI) 
 5. Polkadot-SDK: [Erasure Coding](https://github.com/paritytech/polkadot-sdk/blob/ae0b3bf6733e7b9e18badb16128a6b25bef1923b/polkadot/erasure-coding/src/lib.rs) (FFI)





