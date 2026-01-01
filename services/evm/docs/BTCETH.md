# JAM Bridge Design for BTC and ETH Movement

This document describes a **JAM-style, trust-minimized bridge design** for **BTC and ETH movement**, inspired by tBTC (SPV-based verification) and Ethereum light-client / optimistic verification designs. The goal is a practical, secure, and extensible system suitable for implementation as a **JAM service plus on-chain contracts**.

---

## Proof-of-Concept Scope (actionable)

**Objective**: Stand up a minimal end-to-end bridge flow for BTC→JAM and JAM→BTC along with ETH→JAM optimistic deposits, exercising staking/slashing hooks without requiring production-grade light clients on day one.

**Target environment**: Bitcoin regtest + Ethereum Sepolia + a JAM devnet with the bridge service enabled.

### Must-haves for the first demo

- Deterministic BTC deposit address derivation from `deposit_id` + recipient + target chain (Taproot script path acceptable).
- SPV verifier that accepts a bounded header chain (for example, the most recent 288 blocks) and checks cumulative work plus Merkle inclusion.
- Threshold signer set (static for the PoC) with mock stake accounting and a single vault UTXO.
- Optimistic ETH deposit path that records the receipt proof and enforces a fixed challenge window (no light-client headers yet).
- Slashable events wired to state: missed signing deadline, equivocation (conflicting signatures), invalid authorization payload.

### Nice-to-haves (stretch)

- Automated signer rotation after N redemptions or after a timeout.
- BTC vault rollover (sweep to a fresh UTXO) driven by the signer set.
- Basic fee accounting for relayers (flat fee per proof submission) and signers (per release).

### PoC success criteria

- A user can deposit BTC on regtest, a relayer proves it, and wrapped BTC appears on a JAM service account.
- A user burns wrapped BTC on JAM, signers produce a payout transaction on regtest, and SPV proof finalizes the redemption.
- An ETH deposit on Sepolia emits an optimistic receipt event that finalizes on JAM after the challenge window expires.

### PoC environment + configs (regtest + Sepolia)

- **Bitcoin**: regtest with 1s blocks, 6-block confirmation rule, header cache depth 288; RPC auth baked into fixtures; vault UTXO seeded at height 1.
- **Ethereum**: Sepolia (chainId 11155111) with a single `EthVault` address baked into fixtures; challenge window = 10 minutes; receipts validated against block headers only (no beacon).
- **Keys**: static signer set (2-of-3) published in fixtures for BTC and the Sepolia vault admin; deterministic HD path for deposit address derivation from `deposit_id`.
- **Tooling**: `bridge-cli btc-spv` to submit proofs against cached headers; `bridge-cli eth-receipt` to post optimistic deposits; both accept `--fixture` to replay the canned demo.

### 7-day implementation plan

**Days 1-2: BTC Foundation**
1. **BTC regtest setup:** Deploy deterministic wallet with known keys for PoC vault
2. **Deposit address generation:** Implement P2WPKH or simple P2TR script path binding
3. **Mock signer committee:** 2-of-3 multisig with known test keys for deterministic signing

**Days 3-4: SPV and State Machines**
4. **SPV verifier:** Accept 6-block header chain, verify cumulative work, validate Merkle inclusion
   - Target: <200ms verification time for typical deposit proof
5. **Bridge state tracking:** Implement deposit/redemption state machines with simple in-memory storage

**Days 5-6: ETH Optimistic Path**
6. **Sepolia contract:** Simple vault contract that emits `Deposit(user, amount, memo)` events
7. **Optimistic verification:** 10-minute challenge window with receipt proof validation

**Day 7: End-to-End Integration**
8. **CLI tools:** BTC deposit sender, ETH deposit sender, relayer that submits proofs
9. **Integration test:** Complete BTC→JAM→BTC and ETH→JAM flows with automated verification

**Performance targets:**
- BTC SPV verification: <200ms per proof
- ETH receipt validation: <100ms per deposit
- Signer operations: <5s for threshold signature generation
- Memory usage: <128MB for full bridge state
- Deterministic replay: All operations reproducible with same seed data

---

## Goals

* Trust-minimized (no single custodian, no admin key)
* Two-way movement:

  * BTC → JAM / EVM (mint wrapped BTC)
  * JAM / EVM → BTC (burn + release BTC)
  * ETH → JAM / other EVM (mint wrapped ETH)
  * JAM / other EVM → ETH (burn + release ETH)
* Economic security via staking and slashing
* Cryptographic verification where feasible
* Clear liveness guarantees and recovery paths

---

## High-Level Architecture

### Components

1. **JAM Bridge Service**

   * Runs on JAM
   * Maintains bridge state machines (deposits, mints, burns, redemptions)
   * Manages signer sets, staking, slashing, and rotation
   * Verifies external-chain proofs (BTC SPV, ETH proofs)

2. **Signer Committees (Threshold)**

   * Randomly selected, stake-weighted
   * Control:

     * BTC vault UTXOs via threshold ECDSA (or MuSig2)
     * ETH vault contract actions (via threshold signatures)
   * Subject to deadlines and slashing

3. **Verification Modules**

   * **BTC SPV Verifier**

     * Header chain validation (PoW + cumulative work)
     * Merkle inclusion proofs
     * Script/output validation
   * **ETH Verifier** (configurable):

     * A) Ethereum light client (strongest, expensive)
     * B) Optimistic verification + challenge window (recommended)
     * C) Committee attestation only (weakest, not recommended long-term)

4. **Relayers / Watchers (Permissionless)**

   * Submit proofs and challenges
   * Monitor signer behavior
   * Earn fees and slashing rewards

---

## State Machines

### A) BTC → JAM / EVM (Mint Wrapped BTC)

#### 1. Deposit Intent

* User requests a BTC deposit address.
* JAM selects a signer committee.
* JAM creates a **Deposit Descriptor**:

  * `deposit_id`
  * target chain and recipient address
  * BTC script template (P2TR / P2WPKH)

#### 2. BTC Deposit

* User sends BTC to the generated address.

#### 3. SPV Proof Submission

A relayer submits:

* Bitcoin block headers (sufficient cumulative work)
* Merkle inclusion proof for the transaction
* Parsed output showing:

  * correct script
  * correct amount
  * binding to `deposit_id`

#### 4. Finalization and Mint

* JAM verifies confirmations (e.g. ≥ 6 blocks).
* Deposit is finalized.
* Wrapped BTC is minted:

  * on JAM and/or
  * on a target EVM chain via bridge message.

#### Deposit Binding

To prevent replay or misdirection:

* Deposit address is deterministically derived from:

  * `deposit_id`
  * recipient address
  * target chain id
* Alternatively, encode `deposit_id` in a Taproot script path.

---

### B) JAM / EVM → BTC (Burn + Redeem BTC)

#### 1. Burn Request

* User burns wrapped BTC.
* User provides a BTC destination address.

#### 2. Redemption Ticket

JAM creates a redemption obligation:

* `redemption_id`
* amount
* BTC destination
* signer committee
* signing deadline

#### 3. BTC Payout

* Committee threshold-signs a BTC transaction
* Transaction spends from vault UTXOs to user address

#### 4. Proof and Finalization

* Relayer submits SPV proof of BTC transaction inclusion
* JAM finalizes redemption

#### Timeout and Recovery

* If committee fails to sign before deadline:

  * committee is slashed
  * user is compensated from slashed stake or insurance pool
  * committee is rotated or vault recovery is triggered

---

## ETH Movement

ETH supports smart contracts, enabling stronger enforcement, but cross-chain verification is heavier than BTC SPV.

### C) ETH → JAM / Other EVM (Mint Wrapped ETH)

#### Source Vault

* Ethereum contract `EthVault`
* Holds ETH and emits `Deposit` events

#### Verification Options

**Option B (Recommended): Optimistic Verification**

* Relayer submits:

  * transaction receipt proof
  * block reference
* Deposit enters a pending state
* Challenge window opens
* Watchers may submit fraud proofs
* If no valid challenge, deposit finalizes and wrapped ETH is minted

**Option A (Stronger): Ethereum Light Client**

* JAM verifies finalized beacon headers
* Verifies inclusion of execution payload
* High security, high complexity

---

### D) JAM / Other EVM → ETH (Burn + Release ETH)

#### 1. Burn

* User burns wrapped ETH on JAM or target chain

#### 2. Release Authorization

* JAM instructs `EthVault` to release ETH
* Either:

  * signers call `EthVault.release(...)` using threshold signatures
  * or JAM produces a proof consumable by the vault (advanced)

#### 3. Slashing

* If signers fail to release ETH before deadline:

  * stake is slashed
  * user compensated

---

## Slashing Model

Slashing is objective and proof-based.

### Slashable Faults

1. **Liveness Failure**

   * Missed signing or release deadline
   * Proof: on-chain timer + missing signature threshold

2. **Equivocation**

   * Signing conflicting BTC spends or ETH releases
   * Proof: two valid conflicting signatures

3. **Invalid Authorization**

   * Signing an invalid transaction or message
   * Proof: signed payload + deterministic verifier

### Penalty Design

* Ensure:

  * `total_stake * slash_fraction ≥ value_at_risk + safety_buffer`
* Portion of slashed stake rewards reporters

---

## Core Data Structures (Minimal)

* `SignerSet`

  * members
  * threshold
  * epoch
  * stake map

* `BTCDeposit`

  * deposit_id
  * script
  * amount
  * recipient
  * status

* `BTCRedemption`

  * redemption_id
  * amount
  * btc_address
  * deadline
  * status

* `ETHDeposit`

  * deposit_id
  * tx_reference
  * amount
  * recipient
  * status

* `ETHRelease`

  * release_id
  * amount
  * eth_address
  * deadline
  * status

* `FaultEvidence`

  * fault_type
  * subject
  * proof_blob

---

## Practical Recommendation

* **BTC side**: SPV verification + threshold custody + slashing (tBTC-style)
* **ETH side**: Optimistic verification with challenge window first
* Upgrade ETH verification to a light client if stronger guarantees are required

This design balances **security, implementability, and cost**, while remaining aligned with JAM’s service-oriented architecture.

---

## Open Extensions

* Unified BTC+ETH bridge service vs separate services
* Fee markets for relayers and signers
* Insurance pool sizing and governance
* Upgrade path to zk-based light clients

---

**End of document**
