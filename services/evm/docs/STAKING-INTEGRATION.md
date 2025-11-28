# USDM Contract: Staking Integration Guide

## Overview

This guide covers the complete integration of the USDM contract with JAM's staking and reward systems, including:
- **Epoch Tracking**: Manual epoch state with an external scheduler
- **Signature Generation**: ECDSA signatures over Ed25519 key commitments
- **Oracle Integration**: Current oracle dependencies and trustless alternatives
- **Validator Management**: Registration and reward submission workflows

The USDM contract keeps **epoch state in `currentEpoch`**. There is **no on-chain time-based epoch calculation**; production deployments must advance epochs via an external scheduler/oracle (for example, a keeper that calls `setEraPoints`/`setEraStakers` after incrementing `currentEpoch`). The `advanceEpoch()` helper only works in `testingMode` and should never be relied on for mainnet operation.

## Key Concepts

```solidity
// USDM epoch state
uint64 public currentEpoch;            // Manually advanced epoch counter
bool public testingMode = true;        // Enables advanceEpoch() for local testing

// Testing-only helper
function advanceEpoch() external {
    require(testingMode, "Only in testing mode");
    currentEpoch++;
}
```

### Comparison with JAM Safrole

JAM's safrole logic derives epochs directly from time, whereas **USDM relies on explicit state updates**. The conceptual mapping is:

- JAM: `epoch = ts / EpochLength`
- USDM: `epoch = currentEpoch` (must be incremented off-chain)

If production needs to mirror safrole epochs, the external scheduler should compute the expected epoch from wall-clock time and update `currentEpoch` accordingly before accepting new submissions.

## Epoch Timeline

```
Genesis:     Arbitrary deployment time
Scheduler:   Off-chain process increments `currentEpoch`

Action Timeline (example)
-------------------------
T0: Deploy contract → currentEpoch = 0
T1: Off-chain keeper computes that epoch 1 should start → calls advance/scheduler → currentEpoch = 1
T2: Validators submit epoch 1 rewards (contract accepts current or previous epoch)
T3: Keeper increments to epoch 2, snapshots NPoS exposure for era 2
```

## Function Reference

### Core Functions

**`currentEpoch`**
- State variable tracking the active epoch for RFC#119 submissions.
- Accepts `epoch == currentEpoch` or `epoch == currentEpoch - 1` in `submitEpochRewards`.

**`advanceEpoch()`**
- Testing-only helper gated by `testingMode`.
- Use an off-chain scheduler in production to update `currentEpoch` before opening submissions for the next epoch.

### Deprecated Functions

**`advanceEpoch()` - Testing Convenience Only**
- Only works in `testingMode` (default for local dev).
- Do not expose in production; replace with a controlled off-chain scheduler that updates `currentEpoch` alongside oracle inputs.

## Usage Examples

### 1. Submit Rewards for Current Epoch

```javascript
const { ethers } = require("ethers");

async function submitCurrentEpochRewards(contract, signer, lines) {
    // Read the mutable epoch counter set by your scheduler
    const currentEpoch = await contract.currentEpoch();

    console.log(`Submitting rewards for epoch ${currentEpoch}`);

    // Generate signature
    const messageHash = computeMessageHash(currentEpoch, lines);
    const ed25519Key = await contract.getEd25519Key(await signer.getAddress());
    const signature = await generateCommitmentSignature(signer, messageHash, ed25519Key);

    // Submit (contract validates epoch is current or previous)
    const tx = await contract.submitEpochRewards(currentEpoch, lines, signature);
    await tx.wait();

    console.log("✅ Rewards submitted!");
}
```

### 2. Advance Epoch in Testing

```javascript
// Local/testing only: advance currentEpoch without a scheduler
async function bumpEpoch(contract) {
    await contract.advanceEpoch();
    const epoch = await contract.currentEpoch();
    console.log(`currentEpoch is now ${epoch}`);
}
```

### 3. Scheduler Pseudocode (production)

```javascript
// Pseudocode for an off-chain keeper that advances epochs and publishes oracle data
async function runScheduler(contract, provider) {
    while (true) {
        // Compute the epoch you expect based on wall-clock time
        const desiredEpoch = Math.floor(Date.now() / 3600_000); // example hourly cadence
        const onchainEpoch = await contract.currentEpoch();

        if (desiredEpoch > onchainEpoch) {
            // Update contract state *before* opening submissions
            await contractOwner.advanceEpoch();
            await oracle.setEraStakers(desiredEpoch, exposures);
            await oracle.awardEraPoints(desiredEpoch, points);
            console.log(`Advanced to epoch ${desiredEpoch}`);
        }

        await new Promise(resolve => setTimeout(resolve, 60_000)); // sleep 1 minute
    }
}
```

## Testing

### Testing the Scheduler

```javascript
const { expect } = require("chai");

describe("Epoch Scheduler", function() {
    let contract;

    beforeEach(async function() {
        const USDM = await ethers.getContractFactory("USDM");
        contract = await USDM.deploy();
        await contract.deployed();
    });

    it("increments currentEpoch via advanceEpoch in testingMode", async function() {
        await contract.advanceEpoch();
        expect(await contract.currentEpoch()).to.equal(1);
    });

    it("rejects advanceEpoch when testingMode is false", async function() {
        await contract.setTestingMode(false);
        await expect(contract.advanceEpoch()).to.be.revertedWith("Only in testing mode");
    });

    it("accepts submissions for current or previous epoch", async function() {
        // Set validator address to allow submission
        const [owner] = await ethers.getSigners();
        await contract.setValidatorAddress(0, owner.address);
        await contract.registerEd25519Key(ethers.constants.HashZero.replace(/^0x/, "0x1"), "0x" + "11".repeat(65));

        const lines = new Array(6).fill().map(() => ({
            approvalUsages: 1,
            noshows: 0,
            usedDownloads: 0,
            usedUploads: 0
        }));

        await contract.submitEpochRewards(0, lines, "0x" + "00".repeat(64));
        await contract.advanceEpoch();
        await contract.submitEpochRewards(1, lines, "0x" + "00".repeat(64)); // current epoch
    });
});
```

## Production Considerations

### 1. Scheduler Cadence

- Pick a cadence (hourly, per-session, or per-era) and keep it consistent.
- Increment `currentEpoch` **before** validators start submitting for that window.
- Push oracle data (era stakers + era points) in the same transaction batch to avoid mismatches.

### 2. Testing Mode vs Production

- `testingMode = true` enables `advanceEpoch()` for local/devnets and skips signature checks when disabled.
- Production should set `testingMode = false` and keep epoch updates behind an authenticated scheduler account.

### 3. Submission Windows

- `submitEpochRewards` accepts `epoch == currentEpoch` or `epoch == currentEpoch - 1`, so schedulers can offer a one-epoch grace period.
- If the scheduler misses a tick, keep `currentEpoch` consistent with the intended timeline before reopening submissions.

## Monitoring

```javascript
// Monitor epoch transitions
async function monitorEpochs(contract) {
    let lastEpoch = await contract.getCurrentEpoch();

    setInterval(async () => {
        const currentEpoch = await contract.getCurrentEpoch();

        if (currentEpoch > lastEpoch) {
            console.log(`🔔 New epoch: ${currentEpoch}`);

            // Trigger epoch-specific actions
            await onNewEpoch(currentEpoch);

            lastEpoch = currentEpoch;
        }
    }, 10000); // Check every 10 seconds
}

async function onNewEpoch(epoch) {
    console.log(`Processing epoch ${epoch}...`);

    // 1. Submit rewards for previous epoch
    // 2. Claim rewards from 2 epochs ago
    // 3. Update tit-for-tat
    // etc.
}
```

---

# Signature Generation

## ECDSA Over Ed25519 Key Commitments

The USDM contract uses **ECDSA signatures over Ed25519 key commitments** to verify epoch reward submissions. This approach:

- ✅ Works on any EVM chain (no precompiles needed)
- ✅ Proves validator controls both ECDSA (MetaMask) and Ed25519 (RFC#119) keys
- ✅ Production-ready without requiring Ed25519 on-chain verification

### How It Works

```
Validator signs:
  commitment = keccak256(messageHash || ed25519PublicKey)

Using:
  - messageHash: Hash of epoch rewards data
  - ed25519PublicKey: Validator's registered 32-byte Ed25519 public key
  - ECDSA signature: Standard MetaMask/ethers.js signature
```

## JavaScript/TypeScript Implementation

### Generate Signature (Off-Chain)

```javascript
const { ethers } = require("ethers");

/**
 * Generate ECDSA signature over Ed25519 key commitment
 * @param {ethers.Signer} signer - MetaMask or ethers.js signer
 * @param {string} messageHash - 32-byte hash of epoch rewards message (0x...)
 * @param {string} ed25519PublicKey - 32-byte Ed25519 public key (0x...)
 * @returns {string} ECDSA signature (65 bytes: r, s, v)
 */
async function generateCommitmentSignature(signer, messageHash, ed25519PublicKey) {
    // 1. Create commitment binding message to Ed25519 key
    const commitment = ethers.utils.solidityKeccak256(
        ["bytes32", "bytes32"],
        [messageHash, ed25519PublicKey]
    );

    // 2. Sign commitment with ECDSA (EIP-191 personal sign)
    const signature = await signer.signMessage(ethers.utils.arrayify(commitment));

    return signature; // 0x... (65 bytes)
}

// Example usage:
async function submitEpochRewards(contract, signer, epoch, lines) {
    // Get validator's registered Ed25519 key
    const validatorAddress = await signer.getAddress();
    const ed25519PublicKey = await contract.getEd25519Key(validatorAddress);

    if (ed25519PublicKey === ethers.constants.HashZero) {
        throw new Error("Ed25519 key not registered. Call registerEd25519Key first.");
    }

    // Compute message hash (matches contract's _hashMessageWithoutUploads)
    const messageHash = computeMessageHash(epoch, lines);

    // Generate ECDSA signature over commitment
    const signature = await generateCommitmentSignature(
        signer,
        messageHash,
        ed25519PublicKey
    );

    // Submit to contract
    const tx = await contract.submitEpochRewards(epoch, lines, signature);
    await tx.wait();

    console.log("Epoch rewards submitted successfully!");
}

/**
 * Compute message hash matching contract's _hashMessageWithoutUploads
 */
function computeMessageHash(epoch, lines) {
    // Extract fields (excluding usedUploads) from ApprovalTallyLine[6]
    const dataWithoutUploads = [[], [], []]; // [approvals, noshows, downloads]

    for (let i = 0; i < 6; i++) {
        dataWithoutUploads[0][i] = lines[i].approvalUsages;
        dataWithoutUploads[1][i] = lines[i].noshows;
        dataWithoutUploads[2][i] = lines[i].usedDownloads;
    }

    return ethers.utils.solidityKeccak256(
        ["uint64", "uint32[6][3]"],
        [epoch, dataWithoutUploads]
    );
}
```

### Verify Signature (Testing/Debugging)

```javascript
/**
 * Verify signature matches contract's verification logic
 * @param {string} validatorAddress - Validator's ECDSA address
 * @param {string} messageHash - 32-byte message hash
 * @param {string} ed25519PublicKey - 32-byte Ed25519 public key
 * @param {string} signature - 65-byte ECDSA signature
 * @returns {boolean} True if signature is valid
 */
function verifyCommitmentSignature(validatorAddress, messageHash, ed25519PublicKey, signature) {
    // Recreate commitment
    const commitment = ethers.utils.solidityKeccak256(
        ["bytes32", "bytes32"],
        [messageHash, ed25519PublicKey]
    );

    // Recover signer from signature
    const recovered = ethers.utils.verifyMessage(
        ethers.utils.arrayify(commitment),
        signature
    );

    return recovered.toLowerCase() === validatorAddress.toLowerCase();
}

// Example:
const isValid = verifyCommitmentSignature(
    "0x1234...", // validator address
    "0xabcd...", // message hash
    "0x5678...", // ed25519 public key
    "0x9876..."  // signature
);
console.log("Signature valid:", isValid);
```

## Complete Validator Workflow

```javascript
const { ethers } = require("ethers");

async function main() {
    // 1. Setup
    const provider = new ethers.providers.JsonRpcProvider("http://localhost:8545");
    const wallet = new ethers.Wallet(PRIVATE_KEY, provider);
    const contract = new ethers.Contract(CONTRACT_ADDRESS, ABI, wallet);

    // 2. Register Ed25519 key (one-time per validator)
    const ed25519PublicKey = "0x1234567890abcdef..."; // 32 bytes

    if (!(await contract.hasEd25519Key(wallet.address))) {
        console.log("Registering Ed25519 key...");

        // Create registration message
        const message = ethers.utils.solidityKeccak256(
            ["string", "bytes32"],
            ["Register Ed25519:", ed25519PublicKey]
        );

        // Sign with ECDSA
        const ecdsaProof = await wallet.signMessage(ethers.utils.arrayify(message));

        // Register on-chain
        const tx = await contract.registerEd25519Key(ed25519PublicKey, ecdsaProof);
        await tx.wait();
        console.log("Ed25519 key registered!");
    }

    // 3. Submit epoch rewards
    const epoch = await contract.getCurrentEpoch();
    const lines = [
        { approvalUsages: 1000, noshows: 2, usedDownloads: 800, usedUploads: 750 },
        { approvalUsages: 950,  noshows: 1, usedDownloads: 700, usedUploads: 680 },
        { approvalUsages: 1020, noshows: 0, usedDownloads: 850, usedUploads: 820 },
        { approvalUsages: 980,  noshows: 3, usedDownloads: 720, usedUploads: 700 },
        { approvalUsages: 1010, noshows: 1, usedDownloads: 790, usedUploads: 770 },
        { approvalUsages: 990,  noshows: 2, usedDownloads: 760, usedUploads: 740 }
    ];

    console.log(`Computing message hash for epoch ${epoch}...`);
    const messageHash = computeMessageHash(epoch, lines);

    console.log("Generating signature...");
    const signature = await generateCommitmentSignature(
        wallet,
        messageHash,
        ed25519PublicKey
    );

    console.log("Submitting epoch rewards...");
    const tx = await contract.submitEpochRewards(epoch, lines, signature);
    const receipt = await tx.wait();

    console.log(`✅ Submitted! Gas used: ${receipt.gasUsed}`);
}

main().catch(console.error);
```

## Security Considerations

### What This Proves

✅ **Validator controls ECDSA private key** (can sign with MetaMask)
✅ **Validator claims ownership of Ed25519 key** (commitment binds both keys)
✅ **Message authenticity** (signature covers epoch rewards data)

### What This Doesn't Prove

⚠️ **Off-chain Ed25519 verification still required** for full RFC#119 compliance
⚠️ **Validators could register Ed25519 keys they don't control** (trust on first use)

### Mitigation for Production

For production RFC#119:
1. **Off-chain validation**: Validators should also submit Ed25519 signatures to a separate system
2. **Slashing**: Penalize validators who submit invalid off-chain Ed25519 signatures
3. **Reputation**: Track validator honesty over time

## Gas Costs

Approximate gas costs (on L2 @ 0.1 gwei):

| Operation | Gas | Cost |
|-----------|-----|------|
| Register Ed25519 key | ~45,000 | ~$0.0007 |
| Submit epoch rewards | ~70,000 | ~$0.001 |
| Signature verification (internal) | ~6,000 | included |

## Troubleshooting

**"Ed25519 key not registered"**
- Call `registerEd25519Key()` first
- Check with `hasEd25519Key(address)`

**"Invalid signature"**
- Ensure you're signing the commitment, not the message directly
- Commitment = keccak256(messageHash || ed25519PublicKey)
- Use EIP-191 personal sign (signMessage in ethers.js)

**"Invalid ECDSA signature length"**
- Signature must be exactly 65 bytes (r: 32, s: 32, v: 1)
- ethers.js `signMessage()` returns correct format

---

# Oracle Dependencies

The USDM contract currently has **5 oracle dependencies** that require off-chain data to be fed by the contract owner:

### 1. **Validator Registry** (Can be eliminated)
**Function:** `setValidatorAddress(uint16 validatorIndex, address validatorAddress)`
- **What:** Registers RFC#119 validators and their ECDSA addresses
- **Oracle Data:** Validator index → ECDSA address mapping
- **Can be eliminated:** ✅ **YES** - with JAM state precompile access to C7/C8/C9
- **Keeps:** `ed25519ToEcdsa` mapping (validators still self-register ECDSA for signing)

### 2. **NPoS Exposure Snapshots** (Partial elimination possible)
**Function:** `setEraStakers(uint32 era, address validator, uint256 own, address[] nominators, uint256[] stakes)`
- **What:** Sets validator exposure (own stake + nominator stakes) per era
- **Oracle Data:** Which nominators back which validators, stake amounts
- **Why Oracle:** NPoS election algorithm runs off-chain
- **Can be eliminated:** ⚠️ **PARTIALLY** - if JAM state exposes staking data (C10, C11)
- **JAM State Needed:**
  - C10: Staking ledger (bonded amounts, unbonding schedule)
  - C11: Nominations (who nominates whom)

### 3. **Era Points** (Can be eliminated)
**Function:** `awardEraPoints(address validator, uint256 points)`
- **What:** Awards points to validators for block production, attestations
- **Oracle Data:** Validator activity metrics (blocks authored, GRANDPA votes, para-validation)
- **Why Oracle:** Activity tracking happens in consensus layer
- **Can be eliminated:** ✅ **YES** - if JAM state tracks validator activity
- **JAM State Needed:** C12: Validator activity metrics per era

### 4. **Era Rewards** (Can be eliminated)
**Function:** `setEraReward(uint32 era, uint256 reward)`
- **What:** Sets total reward pool for an era
- **Oracle Data:** Inflation schedule, total issuance, treasury allocation
- **Why Oracle:** Economic parameters computed off-chain
- **Can be eliminated:** ✅ **YES** - if JAM has on-chain inflation schedule
- **JAM State Needed:** C13: Economic parameters, issuance model

### 5. **Slashing** (Complex - requires governance)
**Function:** `slashValidator(address validator, uint32 slashFraction, SlashReason reason)`
- **What:** Slashes validator for misbehavior
- **Oracle Data:** Equivocation proofs, offline evidence, dispute resolution
- **Why Oracle:** Requires off-chain evidence verification and governance decisions
- **Can be eliminated:** ⚠️ **PARTIALLY** - requires on-chain dispute resolution
- **JAM State Needed:** C14: Slashing events, offense reports, dispute outcomes

---

## JAM State Precompile for Trustless Operation

To eliminate oracles, the EVM interpreter can expose JAM state via a precompile at address `0x0800`:

### Required JAM State Exports

```
C7  - Next Validators ([]types.Validator)
C8  - Current Validators ([]types.Validator)
C9  - Prior Validators ([]types.Validator)
C10 - Staking Ledger (bonded amounts, unbonding schedules)
C11 - Nominations (nominator → validator mappings)
C12 - Validator Activity (era points, block production metrics)
C13 - Economic Parameters (inflation rate, era rewards)
C14 - Slashing Events (offenses, dispute outcomes)
```

### Precompile Interface (Hypothetical)

```solidity
// JAM State Precompile at 0x0800
interface IJamState {
    /// @notice Get validators for an epoch (returns C7/C8/C9 based on timing)
    /// @param epoch Epoch number
    /// @return validators Array of Validator structs
    function getValidators(uint64 epoch) external view returns (Validator[] memory);

    /// @notice Get staking ledger for an account
    /// @param stash Stash account address
    /// @return ledger Staking ledger (bonded, active, unlocking)
    function getStakingLedger(address stash) external view returns (StakingLedger memory);

    /// @notice Get nominations for a nominator
    /// @param nominator Nominator address
    /// @return validators List of nominated validators
    function getNominations(address nominator) external view returns (address[] memory);

    /// @notice Get validator activity for an era
    /// @param era Era number
    /// @param validator Validator address
    /// @return points Era points earned
    function getEraPoints(uint32 era, address validator) external view returns (uint256);

    /// @notice Get reward amount for an era
    /// @param era Era number
    /// @return reward Total reward pool
    function getEraReward(uint32 era) external view returns (uint256);
}
```

### Contract Integration Pattern

```solidity
address constant JAMSTATE_PRECOMPILE = address(0x0800);

function hasJamStateAccess() public view returns (bool) {
    return JAMSTATE_PRECOMPILE.code.length > 0;
}

function getValidatorsForEpoch(uint64 epoch) internal view returns (Validator[6] memory) {
    if (hasJamStateAccess()) {
        (bool success, bytes memory data) = JAMSTATE_PRECOMPILE.staticcall(
            abi.encodeWithSignature("getValidators(uint64)", epoch)
        );
        if (success) {
            return abi.decode(data, (Validator[6]));
        }
    }

    // Fallback: use oracle-set validators
    return jamValidators;
}
```

---

## Trustless vs Oracle-Based Comparison

| Feature | Oracle-Based (Current) | JAM State Precompile (Future) |
|---------|------------------------|-------------------------------|
| **Validator Set** | Owner calls `setValidatorAddress()` | Read from C7/C8/C9 automatically |
| **Trust Model** | Trust contract owner | Trust JAM consensus |
| **Update Frequency** | Manual per validator change | Automatic with epochs |
| **Attack Surface** | Owner key compromise | JAM consensus compromise |
| **Gas Costs** | High (storage writes) | Low (state reads) |
| **Latency** | Depends on oracle | Real-time with blocks |
| **Staking Data** | Owner calls `setEraStakers()` | Read from C10/C11 |
| **Activity Tracking** | Owner calls `awardEraPoints()` | Read from C12 |
| **Rewards** | Owner calls `setEraReward()` | Read from C13 |

---

## Hybrid Approach (Recommended)

**Phase 1: Eliminate Validator Oracle (Easiest)**
- ✅ Implement JAM state access for C7/C8/C9
- ✅ Remove `setValidatorAddress()` function
- ✅ Keep `ed25519ToEcdsa` for ECDSA signing
- **Impact:** Trustless validator set, auto-rotation

**Phase 2: Eliminate Staking Oracle**
- ✅ Expose C10 (staking ledger) and C11 (nominations)
- ✅ Remove `setEraStakers()` function
- ✅ Contract reads exposures directly from JAM state
- **Impact:** Trustless staking, no exposure snapshots needed

**Phase 3: Eliminate Activity/Rewards Oracle**
- ✅ Expose C12 (validator activity) and C13 (rewards)
- ✅ Remove `awardEraPoints()` and `setEraReward()`
- **Impact:** Fully automated reward distribution

**Keep Oracle For:**
- ⚠️ Slashing (requires governance/dispute resolution)
- ⚠️ Emergency functions (pause, rescue)
- ⚠️ Configuration (decay factors, conversion rates)

---

## Summary

✅ **Automatic epoch progression** based on `block.timestamp`
✅ **Matches JAM safrole logic** for compatibility
✅ **No manual `advanceEpoch()` calls** in production
✅ **Phase tracking** for intra-epoch timing
✅ **Graceful handling** of late submissions (previous epoch allowed)

**With JAM State Precompile:**
✅ **Trustless validator set** from C7/C8/C9
✅ **Automatic rotation** without oracle intervention
✅ **Reduced attack surface** (no owner key dependency for validators)
✅ **Real-time synchronization** with JAM consensus

The epoch timing system provides a robust, automatic mechanism for time-based reward cycles. When combined with JAM state access, it can operate in a fully trustless manner without requiring external oracle triggers for validator management.
