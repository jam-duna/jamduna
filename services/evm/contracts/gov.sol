// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

interface IUSDM {
    // ERC20 functions
    function mint(address to, uint256 amount) external;
    function burn(address from, uint256 amount) external;

    // Accumulate host functions
    function bless(uint64 m, uint64 v, uint64 r, uint64 n, bytes calldata boldA, bytes calldata boldZ) external;
    function assign(uint64 c, uint64 a, bytes calldata queueWorkReport) external;
    function designate(bytes calldata validators) external;
    function newService(bytes calldata codeHash, uint64 l, uint64 g, uint64 m, uint64 f, uint64 i) external;
    function upgrade(bytes calldata codeHash, uint64 g, uint64 m) external;
    function transferAccumulate(uint64 d, uint64 a, uint64 g, bytes calldata memo) external;
    function eject(uint64 d, bytes calldata hashData) external;
    function write(bytes calldata key, bytes calldata value) external;
    function solicit(bytes calldata hashData, uint64 z) external;
    function forget(bytes calldata hashData, uint64 z) external;
    function provide(uint64 s, uint64 z, bytes calldata data) external;

    // Balance query for voting power
    function balanceOf(address account) external view returns (uint256);
    function totalSupply() external view returns (uint256);
}

contract Governance {
    // Hardcoded USDM precompile address since both contracts are precompiles
    IUSDM public constant usdm = IUSDM(0x0000000000000000000000000000000000000001);

    // Governance parameters - ZERO DELAYS FOR TESTING
    uint256 public constant VOTING_PERIOD = 0;      // No voting period for testing
    uint256 public constant EXECUTION_DELAY = 0;    // No execution delay for testing
    uint256 public constant QUORUM_PERCENTAGE = 1;  // Only need 1% participation (was 10%)
    uint256 public constant PASS_THRESHOLD = 50;    // 50% of votes cast

    uint256 public proposalCount;

    enum ProposalState {
        Pending,    // Created but voting hasn't started
        Active,     // Voting period active
        Defeated,   // Vote failed
        Succeeded,  // Vote passed but not yet executed
        Executed,   // Proposal executed
        Expired     // Execution window expired
    }

    struct Proposal {
        uint256 id;
        address proposer;
        string description;
        bytes callData;         // Encoded function call to execute
        uint256 startTime;      // When voting starts
        uint256 endTime;        // When voting ends
        uint256 executionTime;  // Earliest execution time (endTime + delay)
        uint256 ayeVotes;       // Votes in favor
        uint256 nayVotes;       // Votes against
        bool executed;          // Whether proposal was executed
        mapping(address => bool) hasVoted; // Track if address has voted
        mapping(address => bool) vote;     // Track vote direction (true = aye, false = nay)
    }

    mapping(uint256 => Proposal) public proposals;

    event ProposalCreated(
        uint256 indexed proposalId,
        address indexed proposer,
        string description,
        uint256 startTime,
        uint256 endTime
    );

    event VoteCast(
        uint256 indexed proposalId,
        address indexed voter,
        bool support,
        uint256 weight
    );

    event ProposalExecuted(uint256 indexed proposalId);

    // Create a new proposal
    function propose(string memory description, bytes memory callData) external returns (uint256) {
        require(bytes(description).length > 0, "Empty description");
        require(callData.length > 0, "Empty call data");

        // Require proposer to have minimum token balance (0.1% of supply)
        uint256 totalSupply = usdm.totalSupply();
        require(usdm.balanceOf(msg.sender) >= totalSupply / 1000, "Insufficient tokens to propose");

        uint256 proposalId = ++proposalCount;
        Proposal storage proposal = proposals[proposalId];

        proposal.id = proposalId;
        proposal.proposer = msg.sender;
        proposal.description = description;
        proposal.callData = callData;
        proposal.startTime = block.timestamp;
        proposal.endTime = block.timestamp + VOTING_PERIOD;
        proposal.executionTime = proposal.endTime + EXECUTION_DELAY;

        emit ProposalCreated(
            proposalId,
            msg.sender,
            description,
            proposal.startTime,
            proposal.endTime
        );

        return proposalId;
    }

    // Vote on a proposal - votes can be changed during voting period
    function vote(uint256 proposalId, bool support) external {
        Proposal storage proposal = proposals[proposalId];
        require(proposal.id != 0, "Proposal does not exist");
        require(block.timestamp >= proposal.startTime, "Voting not started");
        require(block.timestamp <= proposal.endTime, "Voting ended");

        uint256 weight = usdm.balanceOf(msg.sender);
        require(weight > 0, "No voting power");

        // If already voted, subtract previous vote
        if (proposal.hasVoted[msg.sender]) {
            if (proposal.vote[msg.sender]) {
                proposal.ayeVotes -= weight;
            } else {
                proposal.nayVotes -= weight;
            }
        }

        // Record new vote
        proposal.hasVoted[msg.sender] = true;
        proposal.vote[msg.sender] = support;

        // Add weight to new tally
        if (support) {
            proposal.ayeVotes += weight;
        } else {
            proposal.nayVotes += weight;
        }

        emit VoteCast(proposalId, msg.sender, support, weight);
    }

    // Execute a successful proposal
    function execute(uint256 proposalId) external {
        Proposal storage proposal = proposals[proposalId];
        require(proposal.id != 0, "Proposal does not exist");
        require(!proposal.executed, "Proposal already executed");
        require(block.timestamp >= proposal.executionTime, "Execution delay not met");
        require(block.timestamp <= proposal.executionTime + 30 days, "Proposal expired");

        ProposalState state = getProposalState(proposalId);
        require(state == ProposalState.Succeeded, "Proposal not succeeded");

        proposal.executed = true;

        // Execute the proposal call
        (bool success, ) = address(usdm).call(proposal.callData);
        require(success, "Proposal execution failed");

        emit ProposalExecuted(proposalId);
    }

    // Get proposal state
    function getProposalState(uint256 proposalId) public view returns (ProposalState) {
        Proposal storage proposal = proposals[proposalId];
        require(proposal.id != 0, "Proposal does not exist");

        if (proposal.executed) {
            return ProposalState.Executed;
        }

        if (block.timestamp < proposal.startTime) {
            return ProposalState.Pending;
        }

        if (block.timestamp < proposal.endTime) {
            return ProposalState.Active;
        }

        if (block.timestamp > proposal.executionTime + 30 days) {
            return ProposalState.Expired;
        }

        // Check if proposal succeeded
        uint256 totalVotes = proposal.ayeVotes + proposal.nayVotes;
        uint256 quorum = usdm.totalSupply() * QUORUM_PERCENTAGE / 100;

        if (totalVotes < quorum) {
            return ProposalState.Defeated;
        }

        if (proposal.ayeVotes * 100 > totalVotes * PASS_THRESHOLD) {
            return ProposalState.Succeeded;
        }

        return ProposalState.Defeated;
    }

    // Get voting results
    function getProposalVotes(uint256 proposalId) external view returns (
        uint256 ayeVotes,
        uint256 nayVotes,
        uint256 totalVotes
    ) {
        Proposal storage proposal = proposals[proposalId];
        require(proposal.id != 0, "Proposal does not exist");

        ayeVotes = proposal.ayeVotes;
        nayVotes = proposal.nayVotes;
        totalVotes = ayeVotes + nayVotes;
    }

    // Check if address has voted on proposal
    function hasVoted(uint256 proposalId, address voter) external view returns (bool) {
        return proposals[proposalId].hasVoted[voter];
    }

    // Get vote direction for address
    function getVote(uint256 proposalId, address voter) external view returns (bool) {
        require(proposals[proposalId].hasVoted[voter], "Address has not voted");
        return proposals[proposalId].vote[voter];
    }

    // Helper functions to create common proposal types
    function proposeMint(address to, uint256 amount, string memory description) external returns (uint256) {
        bytes memory callData = abi.encodeWithSignature("mint(address,uint256)", to, amount);
        return this.propose(description, callData);
    }

    function proposeBurn(address from, uint256 amount, string memory description) external returns (uint256) {
        bytes memory callData = abi.encodeWithSignature("burn(address,uint256)", from, amount);
        return this.propose(description, callData);
    }

    function proposeNewService(
        bytes memory codeHash,
        uint64 l,
        uint64 g,
        uint64 m,
        uint64 f,
        uint64 i,
        string memory description
    ) external returns (uint256) {
        bytes memory callData = abi.encodeWithSignature(
            "newService(bytes,uint64,uint64,uint64,uint64,uint64)",
            codeHash, l, g, m, f, i
        );
        return this.propose(description, callData);
    }
}