// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

/// @title Shield - JAM Railgun Privacy Layer Interface
/// @notice Precompile contract for depositing/withdrawing assets to/from Railgun
/// @dev Deployed at fixed address 0x0000000000000000000000000000000000000010
/// @dev Emits TransferAccumulate events that EVM refiner converts to AccumulateInstructions
/// @dev JAM runtime processes AccumulateInstructions → creates DeferredTransfer to Railgun
///
/// @dev Memo Formats (128 bytes):
/// @dev - v1 (withdrawal): [0x01 | recipient(20) | asset_id(4) | reserved(103)]
/// @dev - v2 (deposit):    [0x02 | asset_id(4) | pk(32) | rho(32) | reserved(59)]
/// @dev - v3 (burn):       [0x03 | asset_id(4) | proof_len(2) | proof_data | reserved]
contract Shield {
    /// @notice Railgun service ID in JAM
    uint32 public constant RAILGUN_SERVICE_ID = 1;

    /// @notice EVM service ID in JAM
    uint32 public constant EVM_SERVICE_ID = 0;

    /// @notice Minimum gas limit for Railgun accumulate
    uint64 public constant GAS_MIN = 50000;

    /// @notice Maximum gas limit for Railgun accumulate
    uint64 public constant GAS_MAX = 5000000;

    /// @notice JAM transfer accumulate event (parsed by EVM refiner)
    /// @dev This event triggers AccumulateInstruction::Transfer creation
    /// @param d Destination service ID (1 = Railgun)
    /// @param a Amount to transfer
    /// @param g Gas limit for destination accumulate
    /// @param memo 128-byte memo (format depends on destination service)
    event TransferAccumulate(uint64 d, uint64 a, uint64 g, bytes memo);

    /// @notice Deposit event (indexed for off-chain tracking)
    /// @param sender Address that initiated the deposit
    /// @param assetId Asset identifier (1=CASH, 1001=STOCK, 1002=RSU, 1003=OPTION)
    /// @param amount Amount deposited (in wei/smallest unit)
    /// @param commitment Commitment hash (for user verification)
    event DepositInitiated(
        address indexed sender,
        uint32 indexed assetId,
        uint256 amount,
        bytes32 commitment
    );

    /// @notice Mint/Burn event for supply tracking
    /// @param assetId Asset identifier
    /// @param amount Amount minted or burned
    /// @param isMint true = mint (supply increase), false = burn (supply decrease)
    event AssetMintBurn(
        uint32 indexed assetId,
        uint256 amount,
        bool isMint
    );

    /// @notice Shield public funds into Railgun privacy layer
    /// @dev Creates a DeferredTransfer from EVM service (0) → Railgun service (1)
    /// @dev The transfer is processed by JAM runtime between refine and accumulate phases
    /// @param pk Recipient's Railgun public key (32 bytes)
    /// @param rho Blinding factor for commitment (32 bytes, user-generated randomness)
    /// @param amount Amount to shield (in wei/smallest unit)
    /// @param assetId Asset identifier (1=CASH, 1001=STOCK, 1002=RSU, 1003=OPTION)
    /// @param gasLimit Gas limit for Railgun accumulate phase
    function shieldDeposit(
        bytes32 pk,
        bytes32 rho,
        uint256 amount,
        uint32 assetId,
        uint64 gasLimit
    ) external {
        require(amount > 0, "Shield: amount must be positive");
        require(gasLimit >= GAS_MIN && gasLimit <= GAS_MAX, "Shield: gas limit out of bounds");

        // Compute commitment (for user verification only)
        // Note: Actual commitment is computed by Railgun accumulate using Poseidon hash
        bytes32 commitment = _computeCommitment(amount, pk, rho);

        // Encode memo v2: [version | asset_id | pk | rho | reserved]
        bytes memory memo = _encodeDepositMemo(pk, rho, assetId);

        // Emit TransferAccumulate event
        // EVM refiner parses this → creates AccumulateInstruction::Transfer
        // JAM runtime processes instruction → creates DeferredTransfer to Railgun service
        emit TransferAccumulate(uint64(RAILGUN_SERVICE_ID), uint64(amount), gasLimit, memo);

        emit DepositInitiated(msg.sender, assetId, amount, commitment);
    }

    /// @notice Mint new asset supply and deposit to Railgun
    /// @dev Creates new supply in transparent pool, then shields it
    /// @dev Equivalent to: total_supply += amount, then deposit
    /// @param assetId Asset identifier to mint
    /// @param amount Amount to mint
    /// @param pk Recipient's Railgun public key
    /// @param rho Blinding factor for commitment
    /// @param gasLimit Gas limit for Railgun accumulate
    function mint(
        uint32 assetId,
        uint256 amount,
        bytes32 pk,
        bytes32 rho,
        uint64 gasLimit
    ) external {
        require(amount > 0, "Shield: amount must be positive");
        require(gasLimit >= GAS_MIN && gasLimit <= GAS_MAX, "Shield: gas limit out of bounds");

        // Mint creates new supply, then deposits it to Railgun
        // The Railgun accumulate handler will:
        // 1. Increase total_supply[assetId] by amount
        // 2. Increase transparent_supply[assetId] by amount
        // 3. Process deposit (move to shielded pool)

        bytes memory memo = _encodeDepositMemo(pk, rho, assetId);

        // Emit TransferAccumulate event
        // EVM refiner parses this → creates AccumulateInstruction::Transfer
        // JAM runtime processes instruction → creates DeferredTransfer to Railgun service
        emit TransferAccumulate(uint64(RAILGUN_SERVICE_ID), uint64(amount), gasLimit, memo);

        emit AssetMintBurn(assetId, amount, true);  // isMint = true
    }

    /// @notice Burn asset supply (reduce total supply)
    /// @dev Requires ZK proof of burn authorization
    /// @dev Used for option exercise, token destruction, etc.
    /// @param assetId Asset identifier to burn
    /// @param amount Amount to burn
    /// @param proof ZK proof of burn authorization (currently unused, for future)
    /// @param publicInputs Public inputs for burn proof (currently unused, for future)
    function burn(
        uint32 assetId,
        uint256 amount,
        bytes calldata proof,
        bytes calldata publicInputs
    ) external {
        require(amount > 0, "Shield: amount must be positive");

        // TODO: Verify burn proof once burn circuit is implemented
        // For now, anyone can burn (should be restricted in production)
        // verify_burn_proof(proof, publicInputs, assetId, amount);

        // Burn reduces total supply
        // The Railgun service will decrease total_supply[assetId]

        // Encode burn memo v3: [version | asset_id | proof_len | proof_data | reserved]
        bytes memory memo = _encodeBurnMemo(assetId, proof);

        // Emit TransferAccumulate event
        // EVM refiner parses this → creates AccumulateInstruction::Transfer
        // JAM runtime processes instruction → creates DeferredTransfer to Railgun service
        emit TransferAccumulate(uint64(RAILGUN_SERVICE_ID), uint64(amount), GAS_MIN, memo);

        emit AssetMintBurn(assetId, amount, false);  // isMint = false
    }

    /// @dev Encode deposit memo v2 for Railgun
    /// @dev Layout: [version(1) | asset_id(4) | pk(32) | rho(32) | reserved(59)]
    /// @param pk Recipient's Railgun public key
    /// @param rho Blinding factor for commitment
    /// @param assetId Asset identifier
    /// @return memo 128-byte memo for DeferredTransfer
    function _encodeDepositMemo(
        bytes32 pk,
        bytes32 rho,
        uint32 assetId
    ) private pure returns (bytes memory) {
        bytes memory memo = new bytes(128);

        // Version byte (0x02 = memo v2 with multi-asset support)
        memo[0] = 0x02;

        // Asset ID (4 bytes, little-endian)
        memo[1] = bytes1(uint8(assetId));
        memo[2] = bytes1(uint8(assetId >> 8));
        memo[3] = bytes1(uint8(assetId >> 16));
        memo[4] = bytes1(uint8(assetId >> 24));

        // Public key (32 bytes at offset 5)
        for (uint i = 0; i < 32; i++) {
            memo[5 + i] = pk[i];
        }

        // Rho (32 bytes at offset 37)
        for (uint i = 0; i < 32; i++) {
            memo[37 + i] = rho[i];
        }

        // Reserved bytes [69..128] are already zero (default)

        return memo;
    }

    /// @dev Encode burn memo v3 for Railgun
    /// @dev Layout: [version(1) | asset_id(4) | proof_len(2) | proof_data(variable) | reserved]
    /// @param assetId Asset identifier
    /// @param proof ZK proof bytes (currently unused)
    /// @return memo 128-byte memo for DeferredTransfer
    function _encodeBurnMemo(
        uint32 assetId,
        bytes calldata proof
    ) private pure returns (bytes memory) {
        bytes memory memo = new bytes(128);

        // Version byte (0x03 = memo v3 for burn)
        memo[0] = 0x03;

        // Asset ID (4 bytes, little-endian)
        memo[1] = bytes1(uint8(assetId));
        memo[2] = bytes1(uint8(assetId >> 8));
        memo[3] = bytes1(uint8(assetId >> 16));
        memo[4] = bytes1(uint8(assetId >> 24));

        // Proof length (2 bytes, little-endian)
        uint16 proofLen = uint16(proof.length > 122 ? 122 : proof.length);
        memo[5] = bytes1(uint8(proofLen));
        memo[6] = bytes1(uint8(proofLen >> 8));

        // Copy proof data (up to 121 bytes remaining: 128 - 7 = 121)
        for (uint i = 0; i < proofLen; i++) {
            memo[7 + i] = proof[i];
        }

        // Reserved bytes are already zero (default)

        return memo;
    }

    /// @dev Compute commitment (simplified keccak256 - actual implementation uses Poseidon)
    /// @dev This is for user verification only - Railgun recomputes using Poseidon hash
    /// @param amount Amount to commit
    /// @param pk Public key
    /// @param rho Blinding factor
    /// @return commitment 32-byte commitment hash
    function _computeCommitment(
        uint256 amount,
        bytes32 pk,
        bytes32 rho
    ) private pure returns (bytes32) {
        // Simplified hash for user verification
        // Production Railgun uses Poseidon hash in BN254 field
        return keccak256(abi.encodePacked(amount, pk, rho));
    }
}
