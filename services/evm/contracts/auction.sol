// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

/**
 * @title AuthorizerAuction
 * @notice JAM Builder Separation auction contract with commit-reveal mechanism
 * @dev Implements sealed-bid uniform price auction for authorization queue slots
 */
contract AuthorizerAuction {
    // ============ Constants ============

    /// @notice Minimum bid requirement (0.1 JAM)
    uint128 public constant BASE_FEE = 0.1 ether;

    /// @notice Authorizer code hash (set at deployment)
    bytes32 public immutable AUTHORIZER_CODE_HASH;

    /// @notice Commit phase duration (6 minutes)
    uint256 public constant COMMIT_PHASE_DURATION = 6 minutes;

    /// @notice Reveal phase duration (2 minutes)
    uint256 public constant REVEAL_PHASE_DURATION = 2 minutes;

    /// @notice Total cycle duration (8 minutes)
    uint256 public constant CYCLE_DURATION = COMMIT_PHASE_DURATION + REVEAL_PHASE_DURATION;

    /// @notice Number of authorization slots per core
    uint256 public constant QUEUE_SIZE = 80;

    /// @notice Maximum bids per core per cycle (gas DoS protection)
    uint256 public constant MAX_BIDS_PER_CORE_CYCLE = 500;

    // ============ State Variables ============

    /// @notice Genesis timestamp for deterministic cycle timing
    uint256 public immutable GENESIS_TIME;

    /// @notice Bid submissions per core per cycle
    mapping(uint16 => mapping(uint32 => BidSubmission[])) public submissions;

    /// @notice Pending refunds (pull payment pattern to avoid DoS)
    mapping(address => uint256) public pendingRefunds;

    /// @notice Track finalized cycles to prevent re-finalization
    mapping(uint16 => mapping(uint32 => bool)) public finalized;

    /// @notice Treasury address for collected fees
    address public treasury;

    /// @notice Only accumulate can call finalization
    address public accumulateCaller;

    /// @notice Re-entrancy guard
    uint256 private locked = 1;

    // ============ Structs ============

    struct BidSubmission {
        address submitter;               // Address that submitted the commitment (must match revealer)
        address builder;                 // Revealed during reveal phase
        uint128 declaredValue;           // Revealed during reveal phase
        bytes32 salt;                    // Revealed during reveal phase (32 bytes)
        bytes32 commitment;              // Blake2b(AUTHORIZER_CODE_HASH || configuration_blob)
        uint256 payment;                 // Payment locked upfront
        bool revealed;                   // True if revealed
    }

    // ============ Events ============

    event BidCommitted(
        uint16 indexed core,
        uint32 indexed cycle,
        address indexed submitter,
        uint256 submissionIndex,
        bytes32 commitment,
        uint256 payment
    );

    event BidRevealed(
        uint16 indexed core,
        uint32 indexed cycle,
        uint256 submissionIndex,
        address indexed builder,
        uint128 declaredValue
    );

    event QueueFinalized(
        uint16 indexed core,
        uint32 indexed cycle,
        uint256 numWinners,
        uint128 clearingPrice,
        uint256 totalCollected
    );

    event RefundQueued(
        address indexed recipient,
        uint256 amount
    );

    event RefundWithdrawn(
        address indexed recipient,
        uint256 amount
    );

    event UnrevealedBidWithdrawn(
        address indexed submitter,
        uint16 indexed core,
        uint32 indexed cycle,
        uint256 submissionIndex,
        uint256 amount
    );

    // ============ Errors ============

    error CommitPhaseEnded();
    error CommitPhaseNotEnded();
    error RevealPhaseEnded();
    error PaymentBelowMinimum();
    error AlreadyRevealed();
    error PaymentLessThanDeclared();
    error InvalidReveal();
    error NotSubmitter();
    error NoRefundAvailable();
    error OnlyAccumulate();
    error CycleInPast();
    error CycleTooFarInFuture();
    error CommitPhaseNotStarted();
    error TooManyBids();
    error AlreadyFinalized();
    error CycleNotEnded();
    error NotFinalized();
    error InvalidIndex();
    error AlreadyWithdrawn();
    error TreasuryTransferFailed();
    error RefundTransferFailed();
    error ReentrancyGuard();
    error CleanupTooSoon();
    error Blake2bNotImplemented();
    error InvalidTreasury();
    error InvalidAccumulateCaller();

    // ============ Constructor ============

    constructor(
        bytes32 _authorizerCodeHash,
        address _treasury,
        address _accumulateCaller,
        uint256 _genesisTime
    ) {
        // Sanity checks to prevent deployment mistakes
        if (_treasury == address(0)) revert InvalidTreasury();
        if (_accumulateCaller == address(0)) revert InvalidAccumulateCaller();

        AUTHORIZER_CODE_HASH = _authorizerCodeHash;
        treasury = _treasury;
        accumulateCaller = _accumulateCaller;
        GENESIS_TIME = _genesisTime;
    }

    // ============ Modifiers ============

    modifier onlyAccumulate() {
        if (msg.sender != accumulateCaller) revert OnlyAccumulate();
        _;
    }

    modifier nonReentrant() {
        if (locked != 1) revert ReentrancyGuard();
        locked = 2;
        _;
        locked = 1;
    }

    // ============ External Functions ============

    /**
     * @notice Submit a sealed bid commitment
     * @param core Core index
     * @param cycle Cycle number
     * @param commitment Blake2b(AUTHORIZER_CODE_HASH || configuration_blob)
     */
    function submitBid(
        uint16 core,
        uint32 cycle,
        bytes32 commitment
    ) external payable {
        // Check cycle is current or next (prevent storage bloat from far-future bids)
        uint32 currentCycle = getCurrentCycle();
        if (cycle < currentCycle) revert CycleInPast();
        if (cycle > currentCycle + 1) revert CycleTooFarInFuture();

        // Check timing
        uint256 cycleStart = getCycleStartTime(cycle);
        if (block.timestamp < cycleStart) revert CommitPhaseNotStarted();
        if (block.timestamp >= cycleStart + COMMIT_PHASE_DURATION) revert CommitPhaseEnded();
        if (msg.value < BASE_FEE) revert PaymentBelowMinimum();

        // Gas DoS protection
        if (submissions[core][cycle].length >= MAX_BIDS_PER_CORE_CYCLE) revert TooManyBids();

        uint256 submissionIndex = submissions[core][cycle].length;

        submissions[core][cycle].push(BidSubmission({
            submitter: msg.sender,
            builder: address(0),
            declaredValue: 0,
            salt: bytes32(0),
            commitment: commitment,
            payment: msg.value,
            revealed: false
        }));

        emit BidCommitted(core, cycle, msg.sender, submissionIndex, commitment, msg.value);
    }

    /**
     * @notice Reveal a previously committed bid
     * @param core Core index
     * @param cycle Cycle number
     * @param submissionIndex Index of submission in array
     * @param builder Builder address (from configuration_blob)
     * @param declaredValue Bid value (from configuration_blob)
     * @param salt Random salt (from configuration_blob)
     */
    function revealBid(
        uint16 core,
        uint32 cycle,
        uint256 submissionIndex,
        address builder,
        uint128 declaredValue,
        bytes32 salt
    ) external {
        uint256 cycleStart = getCycleStartTime(cycle);
        if (block.timestamp < cycleStart + COMMIT_PHASE_DURATION) revert CommitPhaseNotEnded();
        if (block.timestamp >= cycleStart + CYCLE_DURATION) revert RevealPhaseEnded();

        // Bounds check to avoid generic panic
        if (submissionIndex >= submissions[core][cycle].length) revert InvalidIndex();

        BidSubmission storage bid = submissions[core][cycle][submissionIndex];

        // Only the original submitter can reveal
        if (msg.sender != bid.submitter) revert NotSubmitter();
        if (bid.revealed) revert AlreadyRevealed();
        if (bid.payment < declaredValue) revert PaymentLessThanDeclared();

        // Recompute commitment: configuration_blob = builder || declaredValue || salt || core || cycle
        bytes memory configurationBlob = abi.encodePacked(
            builder,           // 20 bytes
            declaredValue,     // 16 bytes (uint128)
            salt,              // 32 bytes
            core,              // 2 bytes (uint16)
            cycle              // 4 bytes (uint32)
        );
        // Total: 74 bytes

        bytes32 computedCommitment = blake2b(abi.encodePacked(AUTHORIZER_CODE_HASH, configurationBlob));

        if (computedCommitment != bid.commitment) revert InvalidReveal();

        // Store revealed values
        bid.builder = builder;
        bid.declaredValue = declaredValue;
        bid.salt = salt;
        bid.revealed = true;

        emit BidRevealed(core, cycle, submissionIndex, builder, declaredValue);
    }

    /**
     * @notice Finalize auction and populate authorization queue (called by accumulate)
     * @param core Core index
     * @param cycle Cycle number
     * @return queue Array of 80 authorization hashes
     * @dev Accumulate caller must invoke hostAssign() with returned queue
     */
    function finalizeQueue(uint16 core, uint32 cycle) external onlyAccumulate nonReentrant returns (bytes32[80] memory queue) {
        // Check cycle has ended
        uint256 cycleStart = getCycleStartTime(cycle);
        if (block.timestamp < cycleStart + CYCLE_DURATION) revert CycleNotEnded();

        // Prevent re-finalization
        if (finalized[core][cycle]) revert AlreadyFinalized();
        finalized[core][cycle] = true;

        BidSubmission[] storage bids = submissions[core][cycle];

        // Filter to only revealed bids and rank by declaredValue (highest first)
        BidSubmission[] memory ranked = rankRevealedBids(bids);

        // Determine clearing price (80th highest bid = uniform price for all winners)
        uint128 clearingPrice = BASE_FEE;
        if (ranked.length >= QUEUE_SIZE) {
            // Standard case: 80+ bids, use 80th highest (marginal winner)
            clearingPrice = max(BASE_FEE, ranked[QUEUE_SIZE - 1].declaredValue);
        } else if (ranked.length > 0) {
            // Partial fill: fewer than 80 bids, use lowest bid (all bids win)
            clearingPrice = max(BASE_FEE, ranked[ranked.length - 1].declaredValue);
        }
        // ranked.length == 0: No bids, clearingPrice = BASE_FEE, all slots become NULL_AUTHORIZER

        // Populate queue with winners and settle payments
        uint256 totalCollected = 0;
        uint256 numWinners = ranked.length < QUEUE_SIZE ? ranked.length : QUEUE_SIZE;

        for (uint256 i = 0; i < numWinners; i++) {
            // Authorization hash is the stored commitment (after successful reveal)
            queue[i] = ranked[i].commitment;

            // Collect clearing price, queue refund for overpayment (pull payment pattern)
            totalCollected += clearingPrice;
            uint256 refund = ranked[i].payment - clearingPrice;
            if (refund > 0) {
                // FIXED: Refund to submitter (who paid), not builder
                pendingRefunds[ranked[i].submitter] += refund;
                emit RefundQueued(ranked[i].submitter, refund);
            }
        }

        // Fill remaining slots with null authorizer hash
        bytes32 NULL_AUTHORIZER_HASH = bytes32(0);
        for (uint256 i = numWinners; i < QUEUE_SIZE; i++) {
            queue[i] = NULL_AUTHORIZER_HASH;
        }

        // Queue refunds for losers (pull payment pattern)
        for (uint256 i = QUEUE_SIZE; i < ranked.length; i++) {
            // FIXED: Refund to submitter (who paid), not builder
            pendingRefunds[ranked[i].submitter] += ranked[i].payment;
            emit RefundQueued(ranked[i].submitter, ranked[i].payment);
        }

        // Transfer collected funds to treasury (use call to handle contract treasury)
        (bool success, ) = treasury.call{value: totalCollected}("");
        if (!success) revert TreasuryTransferFailed();

        emit QueueFinalized(core, cycle, numWinners, clearingPrice, totalCollected);

        return queue;
    }

    /**
     * @notice Withdraw pending refunds (pull payment pattern)
     */
    function withdrawRefund() external nonReentrant {
        uint256 amount = pendingRefunds[msg.sender];
        if (amount == 0) revert NoRefundAvailable();

        pendingRefunds[msg.sender] = 0;

        // Use .call instead of .transfer to avoid gas stipend issues
        (bool success, ) = msg.sender.call{value: amount}("");
        if (!success) revert RefundTransferFailed();

        emit RefundWithdrawn(msg.sender, amount);
    }

    /**
     * @notice Withdraw unrevealed bid after cycle ends
     * @param core Core index
     * @param cycle Cycle number
     * @param submissionIndex Index of submission
     * @dev No penalty for non-reveal (full refund). Change if penalty desired.
     */
    function withdrawUnrevealedBid(
        uint16 core,
        uint32 cycle,
        uint256 submissionIndex
    ) external nonReentrant {
        uint256 cycleStart = getCycleStartTime(cycle);
        if (block.timestamp < cycleStart + CYCLE_DURATION) revert CycleNotEnded();

        // Bounds check
        if (submissionIndex >= submissions[core][cycle].length) revert InvalidIndex();

        BidSubmission storage bid = submissions[core][cycle][submissionIndex];
        if (msg.sender != bid.submitter) revert NotSubmitter();
        if (bid.revealed) revert AlreadyRevealed();
        if (bid.payment == 0) revert AlreadyWithdrawn();

        uint256 amount = bid.payment;
        bid.payment = 0;

        // Use .call instead of .transfer to avoid gas stipend issues
        (bool success, ) = msg.sender.call{value: amount}("");
        if (!success) revert RefundTransferFailed();

        emit UnrevealedBidWithdrawn(msg.sender, core, cycle, submissionIndex, amount);
    }

    // ============ Internal Functions ============

    /**
     * @notice Filter to revealed bids and rank by declaredValue (highest first)
     * @param bids Array of all bid submissions
     * @return ranked Sorted array of revealed bids
     * @dev Uses quicksort for simplicity. For more predictable gas costs, consider
     *      using top-K selection algorithm (see selectTop80() alternative below)
     */
    function rankRevealedBids(BidSubmission[] storage bids) internal view returns (BidSubmission[] memory ranked) {
        // Count revealed bids
        uint256 revealedCount = 0;
        for (uint256 i = 0; i < bids.length; i++) {
            if (bids[i].revealed) {
                revealedCount++;
            }
        }

        // Create array of revealed bids
        ranked = new BidSubmission[](revealedCount);
        uint256 idx = 0;
        for (uint256 i = 0; i < bids.length; i++) {
            if (bids[i].revealed) {
                ranked[idx] = bids[i];
                idx++;
            }
        }

        // Sort by declaredValue (descending - highest first)
        // FIXED: Guard against empty array
        if (ranked.length > 1) {
            quickSort(ranked, 0, int256(ranked.length - 1));
        }

        return ranked;
    }

    /**
     * @notice Alternative: Top-80 selection with predictable gas cost
     * @param bids Array of all bid submissions
     * @return top Array of top 80 bids (or all if fewer), sorted descending
     * @dev More gas-efficient than quicksort for selecting winners only
     *      Cost: O(revealedCount × 80) = ~40,000 comparisons for 500 bids
     *      No recursion, no pathological cases
     *
     * To use: Replace rankRevealedBids() call in finalizeQueue() with selectTop80()
     */
    function selectTop80(BidSubmission[] storage bids) internal view returns (BidSubmission[] memory top) {
        // Count revealed bids
        uint256 revealedCount = 0;
        for (uint256 i = 0; i < bids.length; i++) {
            if (bids[i].revealed) {
                revealedCount++;
            }
        }

        // If ≤80 bids, return all (sorted)
        if (revealedCount <= QUEUE_SIZE) {
            top = new BidSubmission[](revealedCount);
            uint256 idx = 0;
            for (uint256 i = 0; i < bids.length; i++) {
                if (bids[i].revealed) {
                    top[idx] = bids[i];
                    idx++;
                }
            }
            // Sort the small array
            if (top.length > 1) {
                quickSort(top, 0, int256(top.length - 1));
            }
            return top;
        }

        // More than 80 bids: use selection algorithm
        // Maintain sorted array of top 80 (descending by declaredValue)
        top = new BidSubmission[](QUEUE_SIZE);
        uint256 topCount = 0;

        for (uint256 i = 0; i < bids.length; i++) {
            if (!bids[i].revealed) continue;

            BidSubmission memory candidate = bids[i];

            // If we haven't filled 80 yet, insert sorted
            if (topCount < QUEUE_SIZE) {
                // Find insertion position (maintain descending order)
                uint256 pos = topCount;
                for (uint256 j = 0; j < topCount; j++) {
                    if (candidate.declaredValue > top[j].declaredValue) {
                        pos = j;
                        break;
                    }
                }

                // Shift elements right to make room
                for (uint256 k = topCount; k > pos; k--) {
                    top[k] = top[k-1];
                }

                // Insert candidate
                top[pos] = candidate;
                topCount++;
            }
            // If array is full, check if candidate beats lowest (at index 79)
            else if (candidate.declaredValue > top[QUEUE_SIZE - 1].declaredValue) {
                // Find insertion position
                uint256 pos = QUEUE_SIZE - 1;
                for (uint256 j = 0; j < QUEUE_SIZE - 1; j++) {
                    if (candidate.declaredValue > top[j].declaredValue) {
                        pos = j;
                        break;
                    }
                }

                // Shift elements right (dropping the last one)
                for (uint256 k = QUEUE_SIZE - 1; k > pos; k--) {
                    top[k] = top[k-1];
                }

                // Insert candidate
                top[pos] = candidate;
            }
        }

        return top;
    }

    /**
     * @notice QuickSort implementation for bid ranking
     * @param arr Array to sort
     * @param left Left index
     * @param right Right index
     */
    function quickSort(BidSubmission[] memory arr, int256 left, int256 right) internal pure {
        if (left >= right) return;

        int256 i = left;
        int256 j = right;
        uint128 pivot = arr[uint256(left + (right - left) / 2)].declaredValue;

        while (i <= j) {
            // Sort descending: higher declaredValue comes first
            while (arr[uint256(i)].declaredValue > pivot) i++;
            while (pivot > arr[uint256(j)].declaredValue) j--;

            if (i <= j) {
                (arr[uint256(i)], arr[uint256(j)]) = (arr[uint256(j)], arr[uint256(i)]);
                i++;
                j--;
            }
        }

        if (left < j) quickSort(arr, left, j);
        if (i < right) quickSort(arr, i, right);
    }

    /**
     * @notice Return maximum of two uint128 values
     */
    function max(uint128 a, uint128 b) internal pure returns (uint128) {
        return a > b ? a : b;
    }

    /**
     * @notice Blake2b hash function (must be implemented as precompile or library)
     * @dev ⚠️ CRITICAL: This MUST be replaced with actual Blake2b before deployment
     * @param data Data to hash
     * @return Hash result (32 bytes)
     */
    function blake2b(bytes memory data) internal pure returns (bytes32) {
        // ⚠️ PREVENT ACCIDENTAL DEPLOYMENT WITH WRONG HASH
        // Use custom error for gas efficiency
        revert Blake2bNotImplemented();

        // For testing only, uncomment below (but NEVER deploy to mainnet):
        // return keccak256(data);

        // Production: Replace entire function with JAM EVM precompile call
        // Example (pseudocode):
        // return blake2bPrecompile(data);
    }

    // ============ View Functions ============

    /**
     * @notice Get deterministic cycle start time from genesis
     * @param cycle Cycle number
     * @return Cycle start timestamp
     */
    function getCycleStartTime(uint32 cycle) public view returns (uint256) {
        return GENESIS_TIME + (uint256(cycle) * CYCLE_DURATION);
    }

    /**
     * @notice Get current cycle number
     * @return Current cycle
     */
    function getCurrentCycle() public view returns (uint32) {
        if (block.timestamp < GENESIS_TIME) return 0;
        return uint32((block.timestamp - GENESIS_TIME) / CYCLE_DURATION);
    }

    /**
     * @notice Get submission count for a core/cycle
     */
    function getSubmissionCount(uint16 core, uint32 cycle) external view returns (uint256) {
        return submissions[core][cycle].length;
    }

    /**
     * @notice Get specific bid submission
     */
    function getSubmission(uint16 core, uint32 cycle, uint256 index) external view returns (BidSubmission memory) {
        return submissions[core][cycle][index];
    }

    /**
     * @notice Get commit deadline for cycle
     */
    function getCommitDeadline(uint32 cycle) external view returns (uint256) {
        return getCycleStartTime(cycle) + COMMIT_PHASE_DURATION;
    }

    /**
     * @notice Get reveal deadline for cycle
     */
    function getRevealDeadline(uint32 cycle) external view returns (uint256) {
        return getCycleStartTime(cycle) + CYCLE_DURATION;
    }

    /**
     * @notice Check if cycle is in commit phase
     */
    function isCommitPhase(uint32 cycle) external view returns (bool) {
        uint256 cycleStart = getCycleStartTime(cycle);
        return block.timestamp >= cycleStart && block.timestamp < cycleStart + COMMIT_PHASE_DURATION;
    }

    /**
     * @notice Check if cycle is in reveal phase
     */
    function isRevealPhase(uint32 cycle) external view returns (bool) {
        uint256 cycleStart = getCycleStartTime(cycle);
        return block.timestamp >= cycleStart + COMMIT_PHASE_DURATION &&
               block.timestamp < cycleStart + CYCLE_DURATION;
    }

    // ============ Admin Functions ============

    /**
     * @notice Update treasury address (governance only)
     */
    function setTreasury(address _treasury) external onlyAccumulate {
        treasury = _treasury;
    }

    /**
     * @notice Update accumulate caller address (governance only)
     */
    function setAccumulateCaller(address _accumulateCaller) external onlyAccumulate {
        accumulateCaller = _accumulateCaller;
    }

    /**
     * @notice Clean up old submission data after finalization (gas optimization)
     * @param core Core index
     * @param cycle Cycle number
     * @dev Can only be called 1 day after cycle ends to allow for any disputes
     * @dev Force-refunds any unrevealed bids before cleanup to prevent fund loss
     */
    function cleanupCycle(uint16 core, uint32 cycle) external nonReentrant {
        // FIXED: Check that cycle IS finalized (not backwards)
        if (!finalized[core][cycle]) revert NotFinalized();
        if (block.timestamp <= getCycleStartTime(cycle) + CYCLE_DURATION + 1 days) revert CleanupTooSoon();

        // Force-refund any unrevealed bids before cleanup to prevent fund loss
        BidSubmission[] storage bids = submissions[core][cycle];
        for (uint256 i = 0; i < bids.length; i++) {
            if (!bids[i].revealed && bids[i].payment > 0) {
                pendingRefunds[bids[i].submitter] += bids[i].payment;
                emit RefundQueued(bids[i].submitter, bids[i].payment);
            }
        }

        delete submissions[core][cycle];
    }
}
