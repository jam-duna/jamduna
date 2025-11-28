// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/// @title USDM - Stablecoin + RFC#119 Validator Rewards + Polkadot NPoS Staking
/// @notice ERC20 stablecoin with integrated validator rewards and NPoS staking
/// @dev Combines THREE systems:
///      1. ERC20 token (USDM stablecoin)
///      2. RFC#119 validator rewards (approval/availability tallying, tit-for-tat)
///      3. Polkadot NPoS staking (validators, nominators, eras, slashing)
///
/// POLKADOT NPoS KEY CONCEPTS:
/// - Validators: Run nodes, produce blocks, guarantee finality (must avoid misbehavior & downtime)
/// - Nominators: Stake behind validators, share rewards & slashing risks
/// - Eras: 24-hour periods where validator set & rewards are calculated
/// - Sessions: Shorter periods within eras for consensus operations
/// - Bonding: Locking funds for staking (requires unbonding period to withdraw)
/// - Commission: Validator fee deducted before distributing rewards to nominators
/// - Slashing: Punishment for misbehavior (0.01% to 100% stake loss)
///
/// NPoS STAKING FLOW:
/// 1. Bond funds (locks USDM tokens for staking)
/// 2. Choose role: validate() or nominate(validators)
/// 3. Wait for next era to become active
/// 4. Earn ETH rewards based on era points
/// 5. Unbond to start withdrawal process
/// 6. Wait BONDING_DURATION eras (28 eras)
/// 7. Withdraw unbonded USDM funds
///
/// STORAGE LAYOUT (CRITICAL - DO NOT REORDER):
/// - Slot 0: balanceOf mapping (ERC20 + Staking balance)
/// - Slot 1: nonces mapping (ERC20 permit)
/// - Slot 2: allowance mapping (ERC20)
/// - Slot 3+: RFC#119 + NPoS state
///
/// RFC#119 DEVIATIONS:
/// - Backing is pre-baked into approvalUsages off-chain (0.8x for backers, 1x for approvals)
/// - Tit-for-tat is ON-CHAIN (not private), violating RFC privacy assumptions
/// - No pruning (keeps data for tit-for-tat computation)
/// - Tit-for-tat includes exponential decay (95% retention) to clear accidental stiffing
///
/// STAKING MODEL:
/// - Stake USDM tokens (ERC20)
/// - Earn ETH rewards from both RFC#119 epochs and NPoS eras
/// - 28-era unbonding period
///
/// ⚠️ SECURITY WARNINGS:
/// 1. sigVerificationEnabled = false by default (TESTING ONLY!)
///    - Must enable for production deployment
///    - Ed25519 verification is NOT implemented; enabling in production will revert
/// 2. ORACLE DEPENDENCIES:
///    - RFC#119 validators registered via setValidatorAddress (owner-only)
///    - NPoS exposures/points set via setEraStakers/awardEraPoints (owner-only)
///    - Reward pools funded externally via receive()
/// 3. testingMode = true allows advanceEpoch() by anyone
///    - Must disable for production
///
/// PRODUCTION CHECKLIST:
/// [ ] Implement real Ed25519 signature verification
/// [ ] Call setSigVerificationEnabled(true)
/// [ ] Call setTestingMode(false)
/// [ ] Register all RFC#119 validators via setValidatorAddress
/// [ ] Set up oracle for NPoS exposure snapshots
/// [ ] Fund rewardPool with sufficient ETH
contract USDM {
    // ========================================================================
    // ERC20 Constants
    // ========================================================================

    string public constant name = "USDM";
    string public constant symbol = "USDM";
    uint8 public constant decimals = 18;
    uint256 public constant totalSupply = 61_000_000 * 10**18;

    // ========================================================================
    // SLOT 0, 1, 2, 3 (MUST NOT CHANGE - compatibility + Grandpa staking)
    // ========================================================================

    mapping(address => uint256) public balanceOf;                      // slot 0
    mapping(address => uint256) public nonces;                         // slot 1
    mapping(address => address) public bonded;                         // slot 2 - Stash -> Controller
    mapping(address => StakingLedger) public ledger;                   // slot 3 - Controller -> StakingLedger
    mapping(address => mapping(address => uint256)) public allowance;  // slot 4 (moved from 2)

    // ========================================================================
    // RFC#119 Validator Rewards Constants
    // ========================================================================

    uint16 public constant NUM_VALIDATORS = 6;
    uint16 public constant F = (NUM_VALIDATORS - 1) / 3; // f = 1 for 6 validators

    // Reward weights (basis points, 10000 = 100%)
    uint16 public constant APPROVAL_REWARD_WEIGHT = 7300; // 73%
    uint16 public constant AVAILABILITY_REWARD_WEIGHT = 700; // 7%
    uint16 public constant MAX_NO_PENALTY_NOSHOWS = 10;

    // ========================================================================
    // NPoS Staking Constants
    // ========================================================================

    /// @notice Number of eras that must pass before unbonded funds can be withdrawn
    /// @dev Polkadot uses 28 eras (28 days), Kusama uses 7 eras (7 days)
    uint32 public constant BONDING_DURATION = 28;

    /// @notice Maximum number of unlocking chunks per account
    /// @dev Prevents unbounded storage growth
    uint32 public constant MAX_UNLOCKING_CHUNKS = 32;

    /// @notice Maximum number of nominators per validator (informational)
    /// @dev Not enforced by setEraStakers (oracle-only function)
    ///      Oracle must respect this limit when computing exposures
    uint16 public constant MAX_NOMINATORS_PER_VALIDATOR = 256;

    /// @notice Maximum number of validators a nominator can support
    uint16 public constant MAX_NOMINATIONS = 16;

    /// @notice Minimum bond amount to become a nominator (in USDM, ~250 tokens)
    uint256 public constant MIN_NOMINATOR_BOND = 250 ether;

    /// @notice Minimum bond amount to become a validator (in USDM, ~350 tokens)
    uint256 public constant MIN_VALIDATOR_BOND = 350 ether;

    /// @notice Duration of an era in blocks (~24 hours at 6s/block = 14,400 blocks)
    uint32 public constant ERA_DURATION = 14400;

    /// @notice Maximum number of validators in the active set
    uint16 public constant MAX_VALIDATORS = 100;

    // ========================================================================
    // RFC#119 Structs
    // ========================================================================
    //
    // ⚠️ CRITICAL: NUM_VALIDATORS (6) MUST ALWAYS MATCH ApprovalTallyLine[6] ARRAY SIZES
    // If NUM_VALIDATORS is changed, ALL fixed-size arrays must be updated and contract redeployed:
    //   - ApprovalTallyLine[6] in ValidatorSubmission
    //   - medianApprovals[6], percentileNoshows[6], availabilityRewards[6], finalPoints[6] in EpochData
    //   - All loops: for (uint16 i = 0; i < NUM_VALIDATORS; i++)
    //
    // ========================================================================

    struct ApprovalTallyLine {
        uint32 approvalUsages;   // α: Pre-weighted (backing=0.8, approval=1.0)
        uint32 noshows;          // No-shows observed
        uint32 usedDownloads;    // β: Chunks downloaded from this validator
        uint32 usedUploads;      // γ: Chunks uploaded to this validator
    }

    struct ValidatorSubmission {
        uint16 validatorIndex;
        ApprovalTallyLine[6] lines;
        bytes signature;
        bool submitted;
    }

    struct EpochData {
        uint16 submissionCount;
        bool rewardsComputed;
        mapping(uint16 => ValidatorSubmission) submissions;
        uint32[6] medianApprovals;
        uint32[6] percentileNoshows;
        uint64[6] availabilityRewards;
        uint64[6] finalPoints;
    }

    struct TitForTatState {
        mapping(uint16 => uint64) cumulativeStiffing;
        uint64 totalStiffing;
        mapping(uint64 => bool) epochUpdated;
        bool initialized;
    }

    // ========================================================================
    // NPoS Staking Structs
    // ========================================================================

    /// @notice Reward destination options
    /// @dev RewardDestination.None silently forfeits rewards (they are burned)
    enum RewardDestination {
        Staked,     // Add rewards to pendingRewards (same as Stash - no auto-compounding)
        Stash,      // Pay rewards to stash account via pendingRewards
        Account,    // Pay rewards to specific account via pendingRewards
        None        // Forfeit rewards (silently burned - they disappear)
    }

    /// @notice Reasons for slashing
    enum SlashReason {
        Offline,            // Validator offline/unresponsive
        Equivocation,       // Double-signing (severe)
        InvalidBlock,       // Produced invalid block
        GrandpaEquivocation // GRANDPA finality equivocation (severe)
    }

    /// @notice Tracks bonded funds and unbonding schedule
    /// @dev Maps to Substrate's StakingLedger
    struct StakingLedger {
        address stash;              // Account holding the bonded funds
        uint256 total;              // Total bonded amount (including unbonding)
        uint256 active;             // Currently active stake
        UnlockChunk[] unlocking;    // Scheduled unbondings
    }

    /// @notice Scheduled withdrawal after unbonding period
    struct UnlockChunk {
        uint256 value;      // Amount to unlock
        uint32 era;         // Era when funds become available
    }

    /// @notice Validator preferences and commission
    struct ValidatorPrefs {
        uint32 commission;  // Commission percentage (0-1,000,000,000 = 0-100%)
        bool blocked;       // Whether accepting new nominations
        bool exists;        // Whether validator is registered
    }

    /// @notice Nominator's validator selections
    struct Nominations {
        address[] targets;  // Validators to nominate (max 16)
        uint32 submittedIn; // Era when nominations were submitted
    }

    /// @notice Validator's exposure (backing) in an era
    /// @dev Snapshot of total stake and nominator breakdown
    struct Exposure {
        uint256 total;              // Total stake backing this validator
        uint256 own;                // Validator's own stake
        IndividualExposure[] others; // Nominators backing this validator
    }

    /// @notice Individual nominator's exposure to a validator
    struct IndividualExposure {
        address who;        // Nominator address
        uint256 value;      // Amount nominated
    }

    /// @notice Active era information
    struct ActiveEraInfo {
        uint32 index;       // Current era number
        uint64 start;       // Block number when era started (0 if never started)
    }

    /// @notice Era reward points for validators
    struct EraRewardPoints {
        uint256 total;                      // Total points across all validators
        mapping(address => uint256) individual; // Points per validator
    }

    /// @notice Slash event record
    struct SlashEvent {
        address validator;      // Validator that was slashed
        uint256 amount;         // Amount slashed from validator
        uint256 nominatorAmount; // Total amount slashed from nominators
        uint32 era;             // Era when slash occurred
        SlashReason reason;     // Why slash occurred
    }

    /// @notice Phragmén election solution score (matches sp_npos_elections::ElectionScore)
    struct ElectionScore {
        uint128 minimalStake;      // Minimal winner stake (maximize)
        uint128 sumStake;          // Sum of all winner stakes (maximize)
        uint128 sumStakeSquared;   // Sum squared of stakes (minimize - variance)
    }

    /// @notice Compact assignment of nominator stake to validators
    /// @dev Matches sp_npos_elections::Assignment with Perbill distribution
    struct Assignment {
        address who;                // Nominator address
        address[] distribution;     // Validator addresses
        uint32[] per_bill;         // Per-billion (0-1,000,000,000) stake allocation per validator
    }

    /// @notice Phragmén election solution (matches pallet_election_provider_multi_phase::RawSolution)
    struct PhragmenSolution {
        address[] winners;          // Elected validator addresses
        Assignment[] assignments;   // Nominator stake distributions
        ElectionScore score;        // Claimed election score
        uint32 round;              // Era/round this solution is for
    }

    // ========================================================================
    // State Variables (Slot 3+)
    // ========================================================================

    // Governance
    address public owner;

    // Security flags
    bool public sigVerificationEnabled = false; // INSECURE: Must enable for production!
    bool public testingMode = true; // Set false for production

    // RFC#119 Epoch State
    /// @notice Epoch data storage (never pruned - grows unbounded)
    /// @dev Each epoch keeps full ValidatorSubmission data including ApprovalTallyLine[6]
    ///      Design choice: No pruning to support tit-for-tat queries
    ///      For production: Consider adding finalizeAndPrune(epoch) to delete submission
    ///      details after rewards/tit-for-tat computed, keeping only hash + finalPoints
    mapping(uint64 => EpochData) public epochs;
    uint64 public currentEpoch;
    mapping(uint16 => address) public validatorAddresses; // ORACLE-SET: Not derived from chain state
    mapping(address => bytes32) public ecdsaToEd25519;
    mapping(uint16 => TitForTatState) public titForTatStates;
    mapping(uint16 => mapping(uint64 => bool)) public epochRewardsClaimed;

    // RFC#119 reverse lookup: stash address -> validator index (if assigned)
    mapping(address => uint16) public stashToValidatorIndex;
    mapping(address => bool) public isRFC119Validator;

    // Tit-for-tat decay configuration
    uint64 public tftDecayFactor = 9500; // 95% retention per epoch (500 = 5% decay)

    // Reward conversion rates (adjustable)
    uint256 public epochPointsToWei = 1e12; // 1 point = 0.000001 ETH

    // NPoS Era State
    /// @notice Current active era
    ActiveEraInfo public activeEra;

    /// @notice Block number when current era started
    uint64 public eraStartBlock;

    /// @notice Total amount bonded across all stakers (in USDM)
    uint256 public totalStaked;

    /// @notice ETH reward pool for both RFC#119 and NPoS rewards (on-chain balance tracker)
    uint256 public rewardPool;

    /// @notice Portion of rewardPool already promised to pendingRewards
    /// @dev Prevents over-allocation of ETH rewards vs funded balance
    uint256 public allocatedRewards;

    // bonded and ledger moved to slots 2-3 (top of file) for Grandpa integration

    /// @notice Staker address -> RewardDestination
    mapping(address => RewardDestination) public payee;

    /// @notice Custom reward destination address (when payee = Account)
    mapping(address => address) public payeeAccount;

    /// @notice Validator address -> ValidatorPrefs
    mapping(address => ValidatorPrefs) public validators;

    /// @notice Nominator address -> Nominations
    mapping(address => Nominations) public nominators;

    /// @notice Era -> Validator -> Exposure (ORACLE-SET: Not derived from ledger)
    mapping(uint32 => mapping(address => Exposure)) public erasStakers;

    /// @notice Era -> Total stake in that era
    mapping(uint32 => uint256) public erasTotalStake;

    /// @notice Era -> EraRewardPoints
    mapping(uint32 => EraRewardPoints) public erasRewardPoints;

    /// @notice Era -> Total validator reward pool for era (ORACLE-SET)
    mapping(uint32 => uint256) public erasValidatorReward;

    /// @notice Validator -> Era -> Whether payout has been claimed
    mapping(address => mapping(uint32 => bool)) public eraClaimedRewards;

    /// @notice All active validators in current era
    /// @dev NOTE: Currently unused - never populated by contract
    ///      Reserved for future on-chain validator set tracking
    ///      getActiveValidatorCount() returns 0, getActiveValidator() reverts
    address[] public activeValidators;

    /// @notice Slash events history
    SlashEvent[] public slashHistory;

    // Combined reward tracking
    mapping(address => uint256) public pendingRewards;

    // ========================================================================
    // Events
    // ========================================================================

    // ERC20
    event Transfer(address indexed from, address indexed to, uint256 amount);
    event Approval(address indexed owner, address indexed spender, uint256 amount);

    // RFC#119
    event EpochRewardsSubmitted(uint64 indexed epoch, uint16 indexed validatorIndex, uint16 submissionCount);
    event EpochComplete(uint64 indexed epoch, uint64[6] finalPoints);
    event TitForTatUpdated(uint16 indexed myIndex, uint16 indexed targetValidator, int64 stiffingDelta);
    event Ed25519KeyRegistered(address indexed ecdsaAddress, bytes32 ed25519PublicKey);
    event EpochRewardsClaimed(uint16 indexed validatorIndex, uint64 indexed epoch, address recipient, uint256 amount);

    // NPoS Staking
    event Bonded(address indexed stash, uint256 amount);
    event Unbonded(address indexed stash, uint256 amount);
    event Withdrawn(address indexed stash, uint256 amount);
    event ValidatorRegistered(address indexed validator, uint32 commission);
    event ValidatorPrefsSet(address indexed validator, uint32 commission, bool blocked);
    event Nominated(address indexed nominator, address[] targets);
    event Chilled(address indexed staker);
    event RewardPaid(address indexed staker, address indexed dest, uint256 amount, uint32 era);
    event Slashed(address indexed validator, uint256 amount, SlashReason reason, uint32 era);
    event NominatorSlashed(address indexed nominator, address indexed validator, uint256 amount);
    event NewEra(uint32 indexed eraIndex, uint256 totalStake, uint16 validatorCount);
    event RewardDestinationSet(address indexed stash, RewardDestination dest);
    event EraPayout(uint32 indexed era, uint256 validatorPayout);
    event RewardPoolFunded(address indexed funder, uint256 amount);
    event RewardsWithdrawn(address indexed account, uint256 amount);
    event SignatureVerificationSet(bool enabled);
    event TestingModeSet(bool enabled);
    event SubmissionsPaused(bool paused);
    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);
    event TFTDecayFactorSet(uint64 factor);
    event EpochPointsToWeiSet(uint256 rate);
    event RewardPoolRescued(address indexed recipient, uint256 amount);

    // ========================================================================
    // Modifiers
    // ========================================================================

    modifier onlyOwner() {
        require(msg.sender == owner, "Only owner");
        _;
    }

    // ========================================================================
    // Constructor
    // ========================================================================

    constructor() {
        balanceOf[msg.sender] = totalSupply;
        emit Transfer(address(0), msg.sender, totalSupply);
        owner = msg.sender;
        activeEra.index = 0;
        activeEra.start = uint64(block.number);
        eraStartBlock = uint64(block.number);
    }

    // ========================================================================
    // ERC20 Functions
    // ========================================================================

    function transfer(address to, uint256 amount) external returns (bool) {
        require(to != address(0), "Invalid to");

        balanceOf[msg.sender] -= amount;
        balanceOf[to] += amount;
        emit Transfer(msg.sender, to, amount);
        return true;
    }

    function approve(address spender, uint256 amount) external returns (bool) {
        allowance[msg.sender][spender] = amount;
        emit Approval(msg.sender, spender, amount);
        return true;
    }

    function transferFrom(address from, address to, uint256 amount) external returns (bool) {
        require(to != address(0), "Invalid to");

        uint256 allowed = allowance[from][msg.sender];
        if (allowed != type(uint256).max) {
            allowance[from][msg.sender] = allowed - amount;
        }
        balanceOf[from] -= amount;
        balanceOf[to] += amount;
        emit Transfer(from, to, amount);
        return true;
    }

    /// @notice Get total supply (ERC20 compatibility helper)
    /// @dev The public constant totalSupply auto-generates totalSupply() view function (standard ERC20)
    ///      This getTotalSupply() is a non-standard pure helper for explicit calls
    /// @return Total token supply (61,000,000 * 10^18)
    function getTotalSupply() external pure returns (uint256) {
        return totalSupply;
    }

    // ========================================================================
    // RFC#119: Dual-Key Registration
    // ========================================================================

    function registerEd25519Key(bytes32 ed25519PublicKey, bytes calldata ecdsaProof) external {
        require(ed25519PublicKey != bytes32(0), "Invalid ed25519 key");
        require(ecdsaProof.length == 65, "Invalid ECDSA signature length");

        bytes32 message = keccak256(abi.encodePacked("Register Ed25519:", ed25519PublicKey));
        bytes32 ethSignedHash = keccak256(abi.encodePacked("\x19Ethereum Signed Message:\n32", message));

        bytes32 r; bytes32 s; uint8 v;
        assembly {
            r := calldataload(ecdsaProof.offset)
            s := calldataload(add(ecdsaProof.offset, 32))
            v := byte(0, calldataload(add(ecdsaProof.offset, 64)))
        }

        require(v == 27 || v == 28, "Invalid ECDSA v");

        address recovered = ecrecover(ethSignedHash, v, r, s);
        require(recovered == msg.sender, "ECDSA signature verification failed");

        ecdsaToEd25519[msg.sender] = ed25519PublicKey;
        emit Ed25519KeyRegistered(msg.sender, ed25519PublicKey);
    }

    // ========================================================================
    // RFC#119: Epoch Submission
    // ========================================================================

    /// @notice Submit epoch rewards data (RFC#119 validators only)
    /// @dev ORACLE DEPENDENCY: Validators submit off-chain computed tallies
    ///      SECURITY: Signature verification must be enabled for production!
    function submitEpochRewards(uint64 epoch, ApprovalTallyLine[6] calldata lines, bytes calldata signature) external {
        require(!submissionsPaused, "Submissions paused");
        require(epoch == currentEpoch || epoch == currentEpoch - 1, "Invalid epoch");

        // FIXED: Proper validator check using registry
        require(isRFC119Validator[msg.sender], "Not an RFC#119 validator");
        uint16 validatorIndex = stashToValidatorIndex[msg.sender];

        EpochData storage epochData = epochs[epoch];
        require(!epochData.submissions[validatorIndex].submitted, "Already submitted");

        bytes32 messageHash = _hashMessageWithoutUploads(epoch, lines);

        // SECURITY: Block if sig verification not enabled
        if (sigVerificationEnabled) {
            require(verifySignature(validatorIndex, messageHash, signature), "Invalid signature");
        } else {
            require(testingMode, "Signature verification disabled in production mode");
        }

        epochData.submissions[validatorIndex].validatorIndex = validatorIndex;
        epochData.submissions[validatorIndex].lines = lines;
        epochData.submissions[validatorIndex].signature = signature;
        epochData.submissions[validatorIndex].submitted = true;
        epochData.submissionCount++;

        emit EpochRewardsSubmitted(epoch, validatorIndex, epochData.submissionCount);

        if (epochData.submissionCount == NUM_VALIDATORS) {
            _computeEpochRewards(epoch);
        }
    }

    function _computeEpochRewards(uint64 epoch) internal {
        EpochData storage epochData = epochs[epoch];
        require(epochData.submissionCount == NUM_VALIDATORS, "Epoch incomplete");
        require(!epochData.rewardsComputed, "Already computed");

        // 1. Median approvals
        for (uint16 i = 0; i < NUM_VALIDATORS; i++) {
            uint32[] memory values = new uint32[](NUM_VALIDATORS);
            for (uint16 j = 0; j < NUM_VALIDATORS; j++) {
                values[j] = epochData.submissions[j].lines[i].approvalUsages;
            }
            epochData.medianApprovals[i] = _median(values);
        }

        // 2. 2/3 percentile noshows
        for (uint16 i = 0; i < NUM_VALIDATORS; i++) {
            uint32[] memory values = new uint32[](NUM_VALIDATORS);
            for (uint16 j = 0; j < NUM_VALIDATORS; j++) {
                values[j] = epochData.submissions[j].lines[i].noshows;
            }
            epochData.percentileNoshows[i] = _twoThirdsPercentile(values);
        }

        // 3. Reweight availability
        uint64[6] memory reweightedDownloads;
        for (uint16 submitter = 0; submitter < NUM_VALIDATORS; submitter++) {
            uint32 totalDownloads = 0;
            for (uint16 i = 0; i < NUM_VALIDATORS; i++) {
                if (i != submitter) {
                    totalDownloads += epochData.submissions[submitter].lines[i].usedDownloads;
                }
            }

            if (totalDownloads > 0) {
                for (uint16 i = 0; i < NUM_VALIDATORS; i++) {
                    if (i != submitter) {
                        uint64 reweight = (uint64(epochData.medianApprovals[i]) * uint64(F + 1)) / totalDownloads;
                        uint64 contrib = reweight * epochData.submissions[submitter].lines[i].usedDownloads;
                        reweightedDownloads[i] += contrib;
                    }
                }
            }
        }

        for (uint16 i = 0; i < NUM_VALIDATORS; i++) {
            epochData.availabilityRewards[i] = reweightedDownloads[i];
        }

        // 4. Final points
        for (uint16 i = 0; i < NUM_VALIDATORS; i++) {
            uint64 approvalReward = uint64(epochData.medianApprovals[i]) * APPROVAL_REWARD_WEIGHT / 10000;
            uint64 availabilityReward = reweightedDownloads[i] * AVAILABILITY_REWARD_WEIGHT / 10000;

            uint32 excessNoshows = 0;
            if (epochData.percentileNoshows[i] > MAX_NO_PENALTY_NOSHOWS) {
                excessNoshows = epochData.percentileNoshows[i] - MAX_NO_PENALTY_NOSHOWS;
            }
            uint64 noshowPenalty = (approvalReward * excessNoshows) / 100;

            epochData.finalPoints[i] = approvalReward + availabilityReward - noshowPenalty;
        }

        epochData.rewardsComputed = true;
        emit EpochComplete(epoch, epochData.finalPoints);
    }

    // ========================================================================
    // RFC#119: Tit-for-Tat
    // ========================================================================
    //
    // INVARIANT: totalStiffing ≈ Σ_v cumulativeStiffing[v] for myIndex
    //
    // Decay behavior:
    // - Decay only applies when updateTitForTat is called for a given (myIndex, v) edge
    // - Edges with no traffic (never iterated) will never decay
    // - This is acceptable for testing; production may want periodic decay
    //
    // ========================================================================

    function updateTitForTat(uint64 epoch) external {
        require(isRFC119Validator[msg.sender], "Not an RFC#119 validator");
        uint16 myIndex = stashToValidatorIndex[msg.sender];
        require(epochs[epoch].rewardsComputed, "Epoch not complete");

        ValidatorSubmission storage mySubmission = epochs[epoch].submissions[myIndex];
        require(mySubmission.submitted, "Validator did not submit");

        TitForTatState storage tftState = titForTatStates[myIndex];
        require(!tftState.epochUpdated[epoch], "TFT already updated for epoch");
        tftState.epochUpdated[epoch] = true;

        if (!tftState.initialized) {
            tftState.initialized = true;
        }

        for (uint16 v = 0; v < NUM_VALIDATORS; v++) {
            if (v == myIndex) continue;

            uint32 myUploadsToV = mySubmission.lines[v].usedUploads;
            uint64 myReweightedUploadsToV = 0;

            if (myUploadsToV > 0) {
                uint32 totalUploads = 0;
                for (uint16 i = 0; i < NUM_VALIDATORS; i++) {
                    if (i != myIndex) {
                        totalUploads += mySubmission.lines[i].usedUploads;
                    }
                }

                if (totalUploads > 0) {
                    uint64 reweight = (uint64(epochs[epoch].medianApprovals[myIndex]) * uint64(F + 1)) / totalUploads;
                    myReweightedUploadsToV = reweight * myUploadsToV;
                }
            }

            uint64 theirReweightedDownloadsFromMe = 0;
            ValidatorSubmission storage theirSubmission = epochs[epoch].submissions[v];
            if (theirSubmission.submitted) {
                uint32 theirDownloadsFromMe = theirSubmission.lines[myIndex].usedDownloads;

                if (theirDownloadsFromMe > 0) {
                    uint32 theirTotalDownloads = 0;
                    for (uint16 i = 0; i < NUM_VALIDATORS; i++) {
                        if (i != v) {
                            theirTotalDownloads += theirSubmission.lines[i].usedDownloads;
                        }
                    }

                    if (theirTotalDownloads > 0) {
                        uint64 reweight = (uint64(epochs[epoch].medianApprovals[v]) * uint64(F + 1)) / theirTotalDownloads;
                        theirReweightedDownloadsFromMe = reweight * theirDownloadsFromMe;
                    }
                }
            }

            // FIXED: Apply decay to existing cumulative stiffing (95% retention)
            uint64 decayed = (tftState.cumulativeStiffing[v] * tftDecayFactor) / 10000;
            uint64 decayAmount = tftState.cumulativeStiffing[v] - decayed;
            tftState.cumulativeStiffing[v] = decayed;
            tftState.totalStiffing = tftState.totalStiffing > decayAmount ?
                                     tftState.totalStiffing - decayAmount : 0;

            int64 stiffingDelta;
            if (myReweightedUploadsToV > theirReweightedDownloadsFromMe) {
                uint64 positiveStiffing = myReweightedUploadsToV - theirReweightedDownloadsFromMe;
                require(positiveStiffing <= uint64(type(int64).max), "Stiffing delta overflow");
                stiffingDelta = int64(positiveStiffing);
                tftState.cumulativeStiffing[v] += positiveStiffing;
                tftState.totalStiffing += positiveStiffing;
            } else {
                uint64 negativeDelta = theirReweightedDownloadsFromMe - myReweightedUploadsToV;
                require(negativeDelta <= uint64(type(int64).max), "Stiffing delta overflow");
                stiffingDelta = -int64(negativeDelta);
            }

            emit TitForTatUpdated(myIndex, v, stiffingDelta);
        }
    }

    function getSkipProbability(uint16 myIndex, uint16 targetValidator) external view returns (uint16) {
        TitForTatState storage tftState = titForTatStates[myIndex];
        if (!tftState.initialized || tftState.totalStiffing == 0) return 0;

        uint64 stiffing = tftState.cumulativeStiffing[targetValidator];
        if (stiffing == 0) return 0;

        uint64 probability = (stiffing * 10000) / tftState.totalStiffing;
        return uint16(probability > 10000 ? 10000 : probability);
    }

    function getCumulativeStiffing(uint16 myIndex, uint16 targetValidator) external view returns (uint64) {
        return titForTatStates[myIndex].cumulativeStiffing[targetValidator];
    }

    // ========================================================================
    // RFC#119: Epoch Reward Claims
    // ========================================================================

    function claimEpochRewards(uint64 epoch) external {
        require(isRFC119Validator[msg.sender], "Not an RFC#119 validator");
        uint16 validatorIndex = stashToValidatorIndex[msg.sender];
        require(epochs[epoch].rewardsComputed, "Rewards not computed");
        require(!epochRewardsClaimed[validatorIndex][epoch], "Already claimed");

        uint64 points = epochs[epoch].finalPoints[validatorIndex];
        uint256 rewardAmount = uint256(points) * epochPointsToWei; // FIXED: Configurable rate
        require(_availableRewardPool() >= rewardAmount, "Insufficient unallocated rewards");

        epochRewardsClaimed[validatorIndex][epoch] = true;
        pendingRewards[msg.sender] += rewardAmount;
        allocatedRewards += rewardAmount;

        emit EpochRewardsClaimed(validatorIndex, epoch, msg.sender, rewardAmount);
    }

    function claimMultipleEpochs(uint64[] calldata epochs_) external {
        require(isRFC119Validator[msg.sender], "Not an RFC#119 validator");
        uint16 validatorIndex = stashToValidatorIndex[msg.sender];

        uint256 totalReward = 0;
        for (uint256 i = 0; i < epochs_.length; i++) {
            uint64 epoch = epochs_[i];
            if (epochs[epoch].rewardsComputed && !epochRewardsClaimed[validatorIndex][epoch]) {
                uint64 points = epochs[epoch].finalPoints[validatorIndex];
                uint256 rewardAmount = uint256(points) * epochPointsToWei; // FIXED: Configurable rate

                epochRewardsClaimed[validatorIndex][epoch] = true;
                totalReward += rewardAmount;

                emit EpochRewardsClaimed(validatorIndex, epoch, msg.sender, rewardAmount);
            }
        }
        require(_availableRewardPool() >= totalReward, "Insufficient unallocated rewards");

        pendingRewards[msg.sender] += totalReward;
        allocatedRewards += totalReward;
    }

    // ========================================================================
    // NPoS: Core Staking Functions
    // ========================================================================
    //
    // STASH vs CONTROLLER (Polkadot pattern):
    // - Stash (msg.sender on bond/bondExtra/setPayee): "Cold" account holding funds
    // - Controller (msg.sender on unbond/withdrawUnbonded/validate/nominate/chill): "Hot" account for operations
    // - Most users set stash == controller (same address for simplicity)
    // - If split: only controller can unbond/withdraw, only stash can bondExtra/setPayee
    //
    // ========================================================================

    /// @notice Bond USDM funds for staking
    /// @dev Locks USDM tokens and creates stash-controller pair
    ///      msg.sender becomes the stash account
    /// @param controller Controller account (can be same as stash)
    /// @param value Amount of USDM to bond
    /// @param payee_ Reward destination
    function bond(address controller, uint256 value, RewardDestination payee_) external {
        require(value > 0, "Cannot bond zero");
        require(bonded[msg.sender] == address(0), "Already bonded");
        require(ledger[controller].stash == address(0), "Controller already in use");
        require(balanceOf[msg.sender] >= value, "Insufficient balance");

        balanceOf[msg.sender] -= value;
        balanceOf[address(this)] += value;
        emit Transfer(msg.sender, address(this), value);

        totalStaked += value;
        bonded[msg.sender] = controller;

        ledger[controller] = StakingLedger({
            stash: msg.sender,
            total: value,
            active: value,
            unlocking: new UnlockChunk[](0)
        });

        payee[msg.sender] = payee_;
        emit Bonded(msg.sender, value);
    }

    /// @notice Bond additional USDM funds to existing stake
    /// @dev msg.sender must be the stash account
    /// @param maxAdditional Amount to add to bond
    function bondExtra(uint256 maxAdditional) external {
        address controller = bonded[msg.sender];
        require(controller != address(0), "Not a stash");
        require(balanceOf[msg.sender] >= maxAdditional, "Insufficient balance");

        StakingLedger storage l = ledger[controller];
        require(l.stash == msg.sender, "Not stash account");

        balanceOf[msg.sender] -= maxAdditional;
        balanceOf[address(this)] += maxAdditional;
        emit Transfer(msg.sender, address(this), maxAdditional);

        totalStaked += maxAdditional;
        l.total += maxAdditional;
        l.active += maxAdditional;

        emit Bonded(msg.sender, maxAdditional);
    }

    /// @notice Schedule a portion of stash to be unlocked
    /// @dev Funds will be available after BONDING_DURATION eras (28 eras)
    ///      msg.sender must be the controller account
    /// @param value Amount to unbond
    function unbond(uint256 value) external {
        StakingLedger storage l = ledger[msg.sender];
        require(l.stash != address(0), "Not a controller");
        require(value > 0, "Cannot unbond zero");
        require(l.active >= value, "Insufficient bonded amount");
        require(l.unlocking.length < MAX_UNLOCKING_CHUNKS, "Too many unlock chunks");

        l.active -= value;
        uint32 unlockEra = activeEra.index + BONDING_DURATION;
        l.unlocking.push(UnlockChunk({ value: value, era: unlockEra }));

        emit Unbonded(l.stash, value);
    }

    /// @notice Withdraw all unbonded USDM funds that have passed the unbonding period
    /// @dev msg.sender must be the controller account
    function withdrawUnbonded() external {
        StakingLedger storage l = ledger[msg.sender];
        require(l.stash != address(0), "Not a controller");

        uint256 totalWithdraw = 0;
        uint256 newLength = 0;

        for (uint256 i = 0; i < l.unlocking.length; i++) {
            if (l.unlocking[i].era <= activeEra.index) {
                totalWithdraw += l.unlocking[i].value;
            } else {
                if (newLength != i) {
                    l.unlocking[newLength] = l.unlocking[i];
                }
                newLength++;
            }
        }

        require(totalWithdraw > 0, "No funds available to withdraw");

        while (l.unlocking.length > newLength) {
            l.unlocking.pop();
        }

        l.total -= totalWithdraw;
        totalStaked -= totalWithdraw;

        balanceOf[address(this)] -= totalWithdraw;
        balanceOf[l.stash] += totalWithdraw;
        emit Transfer(address(this), l.stash, totalWithdraw);

        emit Withdrawn(l.stash, totalWithdraw);
    }

    // ========================================================================
    // NPoS: Validator Functions
    // ========================================================================

    /// @notice Declare intention to validate
    /// @param commission Commission percentage (0-1,000,000,000 = 0-100%)
    /// @param blocked Whether accepting new nominations
    function validate(uint32 commission, bool blocked) external {
        address controller = msg.sender;
        StakingLedger storage l = ledger[controller];
        require(l.stash != address(0), "Not a controller");
        require(l.active >= MIN_VALIDATOR_BOND, "Insufficient bond");
        require(commission <= 1_000_000_000, "Commission too high");

        address stash = l.stash;
        delete nominators[stash];

        validators[stash] = ValidatorPrefs({
            commission: commission,
            blocked: blocked,
            exists: true
        });

        emit ValidatorRegistered(stash, commission);
    }

    /// @notice Declare intention to nominate validators
    /// @param targets List of validator addresses to nominate (max 16)
    function nominate(address[] calldata targets) external {
        address controller = msg.sender;
        StakingLedger storage l = ledger[controller];
        require(l.stash != address(0), "Not a controller");
        require(l.active >= MIN_NOMINATOR_BOND, "Insufficient bond");
        require(targets.length > 0 && targets.length <= MAX_NOMINATIONS, "Invalid number of targets");

        address stash = l.stash;

        for (uint256 i = 0; i < targets.length; i++) {
            require(validators[targets[i]].exists, "Target not a validator");
            require(!validators[targets[i]].blocked, "Target blocked");
        }

        delete validators[stash];

        nominators[stash] = Nominations({
            targets: targets,
            submittedIn: activeEra.index
        });

        emit Nominated(stash, targets);
    }

    /// @notice Stop participating in staking (remain bonded but don't validate/nominate)
    function chill() external {
        address controller = msg.sender;
        StakingLedger storage l = ledger[controller];
        require(l.stash != address(0), "Not a controller");

        address stash = l.stash;
        delete validators[stash];
        delete nominators[stash];

        emit Chilled(stash);
    }

    /// @notice Update validator commission and preferences
    /// @param commission New commission percentage (0-1,000,000,000 = 0-100%)
    /// @param blocked Whether to block new nominations
    function setValidatorPrefs(uint32 commission, bool blocked) external {
        address controller = msg.sender;
        StakingLedger storage l = ledger[controller];
        require(l.stash != address(0), "Not a controller");
        require(commission <= 1_000_000_000, "Commission too high");

        address stash = l.stash;
        require(validators[stash].exists, "Not a validator");

        validators[stash].commission = commission;
        validators[stash].blocked = blocked;

        emit ValidatorPrefsSet(stash, commission, blocked);
    }

    // ========================================================================
    // NPoS: Reward Functions
    // ========================================================================

    /// @notice Set reward destination
    /// @param dest Destination for rewards
    function setPayee(RewardDestination dest) external {
        require(bonded[msg.sender] != address(0), "Not a stash");
        payee[msg.sender] = dest;
        emit RewardDestinationSet(msg.sender, dest);
    }

    /// @notice Set custom reward destination account
    /// @param account Address to receive rewards
    function setPayeeAccount(address account) external {
        require(bonded[msg.sender] != address(0), "Not a stash");
        require(account != address(0), "Invalid account");
        payee[msg.sender] = RewardDestination.Account;
        payeeAccount[msg.sender] = account;
        emit RewardDestinationSet(msg.sender, RewardDestination.Account);
    }

    /// @notice Claim rewards for a validator's stakers in a specific era
    /// @dev Distributes ETH rewards to validator and their nominators
    /// @param validatorStash Validator address
    /// @param era Era to claim rewards for
    function payoutStakers(address validatorStash, uint32 era) external {
        require(era < activeEra.index, "Era not finished");
        require(!eraClaimedRewards[validatorStash][era], "Already claimed");
        require(erasValidatorReward[era] > 0, "No rewards for era");

        Exposure storage exposure = erasStakers[era][validatorStash];
        require(exposure.total > 0, "No exposure");

        eraClaimedRewards[validatorStash][era] = true;

        EraRewardPoints storage points = erasRewardPoints[era];
        uint256 validatorPoints = points.individual[validatorStash];
        require(validatorPoints > 0, "No points earned");
        require(points.total > 0, "No era points");

        uint256 totalReward = (erasValidatorReward[era] * validatorPoints) / points.total;
        require(_availableRewardPool() >= totalReward, "Insufficient unallocated rewards");

        ValidatorPrefs memory prefs = validators[validatorStash];
        uint256 commission = (totalReward * prefs.commission) / 1_000_000_000;
        uint256 nominatorRewards = totalReward - commission;

        uint256 allocated = 0;
        allocated += _payReward(validatorStash, commission, era);

        if (exposure.own > 0) {
            uint256 validatorShare = (nominatorRewards * exposure.own) / exposure.total;
            allocated += _payReward(validatorStash, validatorShare, era);
            nominatorRewards -= validatorShare;
        }

        for (uint256 i = 0; i < exposure.others.length; i++) {
            address nominator = exposure.others[i].who;
            uint256 nominatorStake = exposure.others[i].value;
            uint256 nominatorReward = (nominatorRewards * nominatorStake) / exposure.total;
            allocated += _payReward(nominator, nominatorReward, era);
        }

        allocatedRewards += allocated;
    }

    /// @notice Internal function to distribute reward to staker
    /// @dev If dest == None, rewards are forfeited (silently burned)
    /// @return credited Amount added to pendingRewards (0 if forfeited)
    function _payReward(address stash, uint256 amount, uint32 era) internal returns (uint256 credited) {
        if (amount == 0) return 0;

        address controller = bonded[stash];
        require(controller != address(0), "Stash not bonded");

        RewardDestination dest = payee[stash];

        if (dest == RewardDestination.Staked) {
            // NOTE: Currently same as Stash - just credits pendingRewards
            // True "compound stake" would require auto-bonding, not implemented
            pendingRewards[stash] += amount;
            emit RewardPaid(stash, stash, amount, era);
            return amount;
        } else if (dest == RewardDestination.Stash) {
            pendingRewards[stash] += amount;
            emit RewardPaid(stash, stash, amount, era);
            return amount;
        } else if (dest == RewardDestination.Account) {
            address account = payeeAccount[stash];
            require(account != address(0), "Payee account not set");
            pendingRewards[account] += amount;
            emit RewardPaid(stash, account, amount, era);
            return amount;
        }
        // If dest == None: rewards are forfeited (burned)
        // Amount disappears from totalReward calculation but not credited anywhere
        return 0;
    }

    /// @notice Withdraw accumulated ETH rewards (both RFC#119 and NPoS)
    function withdrawRewards() external {
        uint256 amount = pendingRewards[msg.sender];
        require(amount > 0, "No rewards to withdraw");
        require(rewardPool >= amount, "Insufficient reward pool");
        require(allocatedRewards >= amount, "Accounting mismatch");
        require(address(this).balance >= amount, "Insufficient contract balance");

        pendingRewards[msg.sender] = 0;
        allocatedRewards -= amount;
        rewardPool -= amount;

        (bool success, ) = msg.sender.call{value: amount}("");
        require(success, "Transfer failed");

        emit RewardsWithdrawn(msg.sender, amount);
    }

    // ========================================================================
    // NPoS: Slashing (Owner Only)
    // ========================================================================

    /// @notice Slash a validator for misbehavior
    /// @dev Only callable by owner (governance/slashing logic)
    ///      Slashed USDM tokens remain in contract (effectively burned - unrecoverable)
    /// @param validator Validator to slash
    /// @param slashFraction Fraction to slash (0-1,000,000,000 = 0-100%)
    /// @param reason Reason for slashing
    function slashValidator(address validator, uint32 slashFraction, SlashReason reason) external onlyOwner {
        require(slashFraction <= 1_000_000_000, "Invalid fraction");

        uint32 era = activeEra.index;
        Exposure storage exposure = erasStakers[era][validator];

        uint256 validatorSlash = (exposure.own * slashFraction) / 1_000_000_000;
        uint256 nominatorSlashTotal = 0;

        address controller = bonded[validator];
        require(controller != address(0), "Validator not bonded");

        StakingLedger storage l = ledger[controller];
        if (l.active >= validatorSlash) {
            l.active -= validatorSlash;
            l.total -= validatorSlash;
            totalStaked -= validatorSlash;
        } else {
            validatorSlash = l.active;
            l.active = 0;
            l.total -= validatorSlash;
            totalStaked -= validatorSlash;
        }

        for (uint256 i = 0; i < exposure.others.length; i++) {
            address nominator = exposure.others[i].who;
            uint256 nominatorStake = exposure.others[i].value;
            uint256 nominatorSlash = (nominatorStake * slashFraction) / 1_000_000_000;

            address nominatorController = bonded[nominator];
            if (nominatorController == address(0)) continue;

            StakingLedger storage nl = ledger[nominatorController];

            if (nl.active >= nominatorSlash) {
                nl.active -= nominatorSlash;
                nl.total -= nominatorSlash;
                totalStaked -= nominatorSlash;
                nominatorSlashTotal += nominatorSlash;
                emit NominatorSlashed(nominator, validator, nominatorSlash);
            }
        }

        slashHistory.push(SlashEvent({
            validator: validator,
            amount: validatorSlash,
            nominatorAmount: nominatorSlashTotal,
            era: era,
            reason: reason
        }));

        emit Slashed(validator, validatorSlash, reason, era);
    }

    // ========================================================================
    // NPoS: Era Management
    // ========================================================================

    /// @notice Trigger new era (PERMISSIONLESS)
    /// @dev Simplified version - full implementation requires off-chain election computation
    ///      In Polkadot, validator election uses NPoS algorithm computed off-chain
    ///
    ///      WARNING: This is permissionless - anyone can call once ERA_DURATION blocks pass
    ///      If your oracle/off-chain system expects to control era transitions, this can race.
    ///      Consider making this onlyOwner or adding an autoEra flag if needed.
    function newEra() external {
        require(block.number >= eraStartBlock + ERA_DURATION, "Era not finished");

        activeEra.index++;
        activeEra.start = uint64(block.number);
        eraStartBlock = uint64(block.number);

        // Clear activeValidators (never populated - reserved for future use)
        delete activeValidators;

        emit NewEra(activeEra.index, 0, 0);
    }

    /// @notice Award era points to a validator (owner only)
    /// @dev ORACLE DEPENDENCY: Points must be computed off-chain and fed by oracle
    ///      Points are typically earned by block authorship, attestations, etc.
    function awardEraPoints(address validator, uint256 points) external onlyOwner {
        uint32 era = activeEra.index;
        EraRewardPoints storage eraPoints = erasRewardPoints[era];
        eraPoints.individual[validator] += points;
        eraPoints.total += points;
    }

    /// @notice Set total validator reward pool for an era (owner only)
    /// @dev ORACLE DEPENDENCY: Reward amount must be backed by ETH in rewardPool
    ///      Typically computed from inflation schedule off-chain
    ///      NOTE: Only checks individual reward <= rewardPool, not sum across all eras
    ///      Operator must ensure total allocated rewards don't exceed funded pool
    function setEraReward(uint32 era, uint256 reward) external onlyOwner {
        require(era <= activeEra.index, "Era not started");
        require(_availableRewardPool() >= reward, "Insufficient unallocated rewards");
        erasValidatorReward[era] = reward;
        emit EraPayout(era, reward);
    }

    /// @notice Set exposure snapshot for a validator in an era (owner only)
    /// @dev ORACLE DEPENDENCY: Exposure computed off-chain via NPoS election algorithm
    ///      Safe to call multiple times for same (era, validator) - old snapshot is replaced.
    ///      erasTotalStake is properly adjusted (subtracts old, adds new).
    function setEraStakers(
        uint32 era,
        address validator,
        uint256 own,
        address[] calldata nominators_,
        uint256[] calldata stakes
    ) external onlyOwner {
        require(nominators_.length == stakes.length, "Length mismatch");

        Exposure storage exposure = erasStakers[era][validator];

        // FIXED: Subtract old total before updating (prevents double-counting)
        if (exposure.total > 0) {
            erasTotalStake[era] -= exposure.total;
        }

        exposure.own = own;
        exposure.total = own;

        delete exposure.others;

        for (uint256 i = 0; i < nominators_.length; i++) {
            exposure.others.push(IndividualExposure({
                who: nominators_[i],
                value: stakes[i]
            }));
            exposure.total += stakes[i];
        }

        erasTotalStake[era] += exposure.total;
    }

    // ========================================================================
    // Helper Functions
    // ========================================================================

    function _hashMessageWithoutUploads(uint64 epoch, ApprovalTallyLine[6] calldata lines) internal pure returns (bytes32) {
        uint32[6][3] memory dataWithoutUploads;
        for (uint16 i = 0; i < NUM_VALIDATORS; i++) {
            dataWithoutUploads[0][i] = lines[i].approvalUsages;
            dataWithoutUploads[1][i] = lines[i].noshows;
            dataWithoutUploads[2][i] = lines[i].usedDownloads;
        }
        return keccak256(abi.encode(epoch, dataWithoutUploads));
    }

    function _median(uint32[] memory values) internal pure returns (uint32) {
        _quickSort(values, 0, int256(values.length - 1));
        return values[values.length / 2];
    }

    function _twoThirdsPercentile(uint32[] memory values) internal pure returns (uint32) {
        _quickSort(values, 0, int256(values.length - 1));
        uint256 idx = (2 * values.length) / 3;
        if (idx >= values.length) idx = values.length - 1;
        return values[idx];
    }

    function _quickSort(uint32[] memory arr, int256 left, int256 right) internal pure {
        int256 i = left;
        int256 j = right;
        if (i == j) return;
        uint32 pivot = arr[uint256(left + (right - left) / 2)];
        while (i <= j) {
            while (arr[uint256(i)] < pivot) i++;
            while (pivot < arr[uint256(j)]) j--;
            if (i <= j) {
                (arr[uint256(i)], arr[uint256(j)]) = (arr[uint256(j)], arr[uint256(i)]);
                i++; j--;
            }
        }
        if (left < j) _quickSort(arr, left, j);
        if (i < right) _quickSort(arr, i, right);
    }

    /// @notice Verify Ed25519 signature (TESTING-ONLY PLACEHOLDER)
    /// @dev ⚠️ SECURITY: Reverts in production because Ed25519 verification is not implemented.
    ///      Production deployments MUST wire a real Ed25519 verifier before enabling signatures.
    function verifySignature(uint16 validatorIndex, bytes32, bytes calldata signature) internal view returns (bool) {
        address validatorAddress = validatorAddresses[validatorIndex];
        require(validatorAddress != address(0), "Validator address not set");

        bytes32 ed25519Key = ecdsaToEd25519[validatorAddress];
        require(ed25519Key != bytes32(0), "Ed25519 key not registered");
        require(signature.length == 64, "Invalid ed25519 signature length");

        // INSECURE PLACEHOLDER: Only allowed while testingMode is true
        require(testingMode, "Ed25519 verification not implemented");
        return true;
    }

    // ========================================================================
    // View Functions
    // ========================================================================

    function getEpochRewards(uint64 epoch) external view returns (uint64[6] memory) {
        require(epochs[epoch].rewardsComputed, "Rewards not computed");
        return epochs[epoch].finalPoints;
    }

    function getMedianApprovals(uint64 epoch) external view returns (uint32[6] memory) {
        require(epochs[epoch].rewardsComputed, "Rewards not computed");
        return epochs[epoch].medianApprovals;
    }

    function getAvailabilityRewards(uint64 epoch) external view returns (uint64[6] memory) {
        require(epochs[epoch].rewardsComputed, "Rewards not computed");
        return epochs[epoch].availabilityRewards;
    }

    function hasSubmitted(uint64 epoch, uint16 validatorIndex) external view returns (bool) {
        return epochs[epoch].submissions[validatorIndex].submitted;
    }

    function getSubmissionCount(uint64 epoch) external view returns (uint16) {
        return epochs[epoch].submissionCount;
    }

    function hasEd25519Key(address account) external view returns (bool) {
        return ecdsaToEd25519[account] != bytes32(0);
    }

    function getEd25519Key(address account) external view returns (bytes32) {
        return ecdsaToEd25519[account];
    }

    function getLedger(address controller) external view returns (address stash, uint256 total, uint256 active, uint256 unlockingCount) {
        StakingLedger storage l = ledger[controller];
        return (l.stash, l.total, l.active, l.unlocking.length);
    }

    function getUnlockChunk(address controller, uint256 index) external view returns (uint256 value, uint32 era) {
        UnlockChunk storage chunk = ledger[controller].unlocking[index];
        return (chunk.value, chunk.era);
    }

    function getEraStakers(uint32 era, address validator) external view returns (uint256 total, uint256 own, uint256 nominatorCount) {
        Exposure storage exposure = erasStakers[era][validator];
        return (exposure.total, exposure.own, exposure.others.length);
    }

    function getEraStakersNominator(uint32 era, address validator, uint256 index) external view returns (address who, uint256 value) {
        IndividualExposure storage nominator = erasStakers[era][validator].others[index];
        return (nominator.who, nominator.value);
    }

    function getNominations(address nominator) external view returns (address[] memory targets, uint32 submittedIn) {
        Nominations storage nom = nominators[nominator];
        return (nom.targets, nom.submittedIn);
    }

    function getEraPoints(uint32 era, address validator) external view returns (uint256) {
        return erasRewardPoints[era].individual[validator];
    }

    function getTotalEraPoints(uint32 era) external view returns (uint256) {
        return erasRewardPoints[era].total;
    }

    function isBonded(address stash) external view returns (bool) {
        return bonded[stash] != address(0);
    }

    function isValidator(address stash) external view returns (bool) {
        return validators[stash].exists;
    }

    function isNominator(address stash) external view returns (bool) {
        return nominators[stash].targets.length > 0;
    }

    function getCurrentEra() external view returns (uint32) {
        return activeEra.index;
    }

    function getActiveValidatorCount() external view returns (uint256) {
        return activeValidators.length;
    }

    function getActiveValidator(uint256 index) external view returns (address) {
        return activeValidators[index];
    }

    // ========================================================================
    // Admin Functions
    // ========================================================================

    /// @notice Register an RFC#119 validator (owner only)
    /// @dev Prevents duplicate registrations and maintains bidirectional mapping
    event ValidatorAddressUpdated(uint16 indexed validatorIndex, address indexed oldAddress, address indexed newAddress);

    function setValidatorAddress(uint16 validatorIndex, address validatorAddress) external onlyOwner {
        require(validatorIndex < NUM_VALIDATORS, "Invalid validator index");
        require(validatorAddress != address(0), "Invalid address");

        // Disallow reusing the same address for a different validator slot
        uint16 existingIndex = stashToValidatorIndex[validatorAddress];
        if (isRFC119Validator[validatorAddress]) {
            require(existingIndex == validatorIndex, "Validator already registered");
        }

        // Clear old mapping if exists
        address oldAddress = validatorAddresses[validatorIndex];
        if (oldAddress != address(0)) {
            isRFC119Validator[oldAddress] = false;
            delete stashToValidatorIndex[oldAddress];
        }

        // Set new mapping
        validatorAddresses[validatorIndex] = validatorAddress;
        stashToValidatorIndex[validatorAddress] = validatorIndex;
        isRFC119Validator[validatorAddress] = true;

        emit ValidatorAddressUpdated(validatorIndex, oldAddress, validatorAddress);
    }

    /// @notice Advance epoch (testing only)
    /// @dev WARNING: In production mode (testingMode=false), epochs never advance automatically
    ///      This is intentional - production requires a proper epoch scheduler/oracle
    ///      For testing, set testingMode=true and call this manually
    function advanceEpoch() external {
        require(testingMode, "Only in testing mode");
        currentEpoch++;
    }

    /// @notice Enable/disable signature verification (owner only)
    function setSigVerificationEnabled(bool enabled) external onlyOwner {
        sigVerificationEnabled = enabled;
        emit SignatureVerificationSet(enabled);
    }

    /// @notice Toggle testing mode (owner only)
    function setTestingMode(bool enabled) external onlyOwner {
        testingMode = enabled;
        emit TestingModeSet(enabled);
    }

    /// @notice Set tit-for-tat decay factor (owner only)
    /// @param factor Retention factor in basis points (10000 = 100%, 9500 = 95%)
    function setTFTDecayFactor(uint64 factor) external onlyOwner {
        require(factor <= 10000, "Factor must be <= 10000");
        tftDecayFactor = factor;
        emit TFTDecayFactorSet(factor);
    }

    /// @notice Set epoch points to Wei conversion rate (owner only)
    function setEpochPointsToWei(uint256 rate) external onlyOwner {
        epochPointsToWei = rate;
        emit EpochPointsToWeiSet(rate);
    }

    /// @notice Emergency: rescue stuck ETH from reward pool (owner only)
    /// @param amount Amount to rescue (must be <= rewardPool)
    function rescueRewardPool(uint256 amount) external onlyOwner {
        address payable recipient = payable(owner);
        require(recipient != address(0), "Invalid recipient");
        require(address(this).balance >= amount, "Insufficient contract balance");
        require(_availableRewardPool() >= amount, "Amount exceeds unallocated pool");
        rewardPool -= amount;
        (bool success, ) = recipient.call{value: amount}("");
        require(success, "Transfer failed");
        emit RewardPoolRescued(recipient, amount);
    }

    /// @notice Emergency: pause all RFC#119 submissions (owner only)
    bool public submissionsPaused = false;

    function pauseSubmissions(bool paused) external onlyOwner {
        submissionsPaused = paused;
        emit SubmissionsPaused(paused);
    }

    /// @notice Transfer ownership (owner only)
    function transferOwnership(address newOwner) external onlyOwner {
        require(newOwner != address(0), "Invalid owner");
        emit OwnershipTransferred(owner, newOwner);
        owner = newOwner;
    }

    // ========================================================================
    // Funding
    // ========================================================================

    receive() external payable {
        rewardPool += msg.value;
        emit RewardPoolFunded(msg.sender, msg.value);
    }

    function _availableRewardPool() internal view returns (uint256) {
        require(rewardPool >= allocatedRewards, "Allocated exceeds pool");
        return rewardPool - allocatedRewards;
    }

    // ========================================================================
    // Phragmén Election Feasibility Check
    // ========================================================================

    /// @notice Validate a Phragmén election solution against current snapshot
    /// @dev Implements feasibility_check similar to pallet_election_provider_multi_phase
    /// @param solution The Phragmén solution to validate
    /// @return valid Whether the solution passes all checks
    /// @return validatorKeys If valid, the encoded validator keys (336 bytes each: 32 Bandersnatch + 32 Ed25519 + 144 BLS + 128 metadata)
    function feasibility_check(PhragmenSolution calldata solution)
        external
        view
        returns (bool valid, bytes memory validatorKeys)
    {
        // Check 1: Round must match current era
        if (solution.round != activeEra.index) {
            return (false, "");
        }

        // Check 2: Winner count must be reasonable (1 to 1023)
        uint256 numWinners = solution.winners.length;
        if (numWinners == 0 || numWinners > 1023) {
            return (false, "");
        }

        // Check 3: All winners must be registered validators
        for (uint256 i = 0; i < numWinners; i++) {
            address winner = solution.winners[i];
            if (!validators[winner].exists) {
                return (false, "");
            }
        }

        // Check 4: Validate assignments structure
        for (uint256 i = 0; i < solution.assignments.length; i++) {
            Assignment calldata assignment = solution.assignments[i];

            // 4a: Nominator must exist and have bonded funds
            if (ledger[assignment.who].active == 0) {
                return (false, "");
            }

            // 4b: Nominator must have nominated these validators
            address[] memory targets = nominators[assignment.who].targets;
            if (targets.length == 0) {
                return (false, "");
            }

            // 4c: Distribution and per_bill arrays must match
            if (assignment.distribution.length != assignment.per_bill.length) {
                return (false, "");
            }

            // 4d: All distribution targets must be winners
            uint256 totalPerBill = 0;
            for (uint256 j = 0; j < assignment.distribution.length; j++) {
                bool isWinner = false;
                for (uint256 k = 0; k < numWinners; k++) {
                    if (assignment.distribution[j] == solution.winners[k]) {
                        isWinner = true;
                        break;
                    }
                }
                if (!isWinner) {
                    return (false, "");
                }

                // 4e: Accumulate per_bill (should sum to 1_000_000_000)
                totalPerBill += assignment.per_bill[j];
            }

            // 4f: Total allocation must be exactly 1 billion (100%)
            if (totalPerBill != 1_000_000_000) {
                return (false, "");
            }
        }

        // Check 5: Recompute score and verify it matches claimed score
        ElectionScore memory recomputed = _recomputeScore(solution.winners, solution.assignments);

        if (recomputed.minimalStake != solution.score.minimalStake ||
            recomputed.sumStake != solution.score.sumStake ||
            recomputed.sumStakeSquared != solution.score.sumStakeSquared) {
            return (false, "");
        }

        // Check 6: Score must meet minimum threshold (prevent low-quality solutions)
        // Minimum: at least 1000 USDM total stake
        if (recomputed.sumStake < 1000 * 10**18) {
            return (false, "");
        }

        // All checks passed - encode validator keys for designate host call
        // Format: For each winner, output 336 bytes:
        //   - 32 bytes: Bandersnatch key (from ecdsaToEd25519 mapping or zero if not set)
        //   - 32 bytes: Ed25519 key (from ecdsaToEd25519 mapping or zero if not set)
        //   - 144 bytes: BLS key (zero - not implemented yet)
        //   - 128 bytes: metadata (zero)
        validatorKeys = new bytes(numWinners * 336);

        for (uint256 i = 0; i < numWinners; i++) {
            address winner = solution.winners[i];
            uint256 offset = i * 336;

            // Get Ed25519 key from mapping (32 bytes)
            bytes32 ed25519Key = ecdsaToEd25519[winner];

            // Copy Bandersnatch key (use Ed25519 for now - TODO: separate mapping)
            for (uint256 j = 0; j < 32; j++) {
                validatorKeys[offset + j] = ed25519Key[j];
            }

            // Copy Ed25519 key
            for (uint256 j = 0; j < 32; j++) {
                validatorKeys[offset + 32 + j] = ed25519Key[j];
            }

            // BLS (144 bytes) and metadata (128 bytes) remain zero
        }

        return (true, validatorKeys);
    }

    /// @notice Recompute election score from winners and assignments
    /// @dev Matches sp_npos_elections scoring logic
    function _recomputeScore(
        address[] calldata winners,
        Assignment[] calldata assignments
    ) internal view returns (ElectionScore memory) {
        // Build support array: backing per validator
        uint256[] memory supportAmounts = new uint256[](winners.length);

        // Initialize with validator's own stake
        for (uint256 i = 0; i < winners.length; i++) {
            address winner = winners[i];
            supportAmounts[i] = ledger[winner].active;
        }

        // Add nominator stakes
        for (uint256 i = 0; i < assignments.length; i++) {
            Assignment calldata assignment = assignments[i];
            uint256 nominatorStake = ledger[assignment.who].active;

            for (uint256 j = 0; j < assignment.distribution.length; j++) {
                address validator = assignment.distribution[j];
                uint256 allocation = (nominatorStake * assignment.per_bill[j]) / 1_000_000_000;

                // Find validator index in winners array
                for (uint256 k = 0; k < winners.length; k++) {
                    if (winners[k] == validator) {
                        supportAmounts[k] += allocation;
                        break;
                    }
                }
            }
        }

        // Compute score components
        uint128 minimalStake = type(uint128).max;
        uint128 sumStake = 0;
        uint128 sumStakeSquared = 0;

        for (uint256 i = 0; i < winners.length; i++) {
            uint128 stake = uint128(supportAmounts[i]);

            if (stake < minimalStake) {
                minimalStake = stake;
            }

            sumStake += stake;
            sumStakeSquared += stake * stake / 10**18; // Normalize to prevent overflow
        }

        return ElectionScore({
            minimalStake: minimalStake,
            sumStake: sumStake,
            sumStakeSquared: sumStakeSquared
        });
    }
}
