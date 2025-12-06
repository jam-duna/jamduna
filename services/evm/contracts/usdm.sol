// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract USDM {
    string public constant name = "USDM";
    string public constant symbol = "USDM";
    uint8 public constant decimals = 18;

    // Hardcoded governance precompile address since both contracts are precompiles
    address public constant governance = 0x0000000000000000000000000000000000000002;

    uint64 public constant TOTAL_CORES = 2;
    uint64 public constant TOTAL_VALIDATORS = 6;
    uint64 public constant VALIDATOR_BYTES = 336;
    uint64 public constant MEMO_SIZE = 128;
    uint64 public constant MAX_AUTH_QUEUE_ITEMS = 80;

    mapping(address => uint256) public balanceOf; // slot 0
    mapping(address => uint256) public nonces; // slot 1
    mapping(address => mapping(address => uint256)) public allowance; // slot 2
    uint256 public totalSupply; // slot 3

    event Transfer(address indexed from, address indexed to, uint256 amount);
    event Approval(address indexed owner, address indexed spender, uint256 amount);
    event Mint(address indexed to, uint256 amount);
    event Burn(address indexed from, uint256 amount);

    // Accumulate host function events
    event Bless(uint64 m, uint64 v, uint64 r, uint64 n, bytes boldA, bytes boldZ);
    event Assign(uint64 c, uint64 a, bytes queueWorkReport);
    event Designate(bytes validators);
    event New(bytes32 codeHash, uint64 l, uint64 g, uint64 m, uint64 f, uint64 i);
    event Upgrade(bytes32 codeHash, uint64 g, uint64 m);
    event TransferAccumulate(uint64 d, uint64 a, uint64 g, bytes memo);
    event Eject(uint64 d, bytes32 hashData);
    event Write(bytes key, bytes value);
    event Solicit(bytes32 hashData, uint64 z);
    event Forget(bytes32 hashData, uint64 z);
    event Provide(uint64 s, uint64 z, bytes data);

    modifier onlyGovernance() {
        require(msg.sender == governance, "Not governance");
        _;
    }
    
    function transfer(address to, uint256 amount) external returns (bool) {
        require(to != address(0), "Transfer to zero address");
        require(balanceOf[msg.sender] >= amount, "Insufficient balance");
        unchecked {
            balanceOf[msg.sender] -= amount;
            balanceOf[to] += amount;
        }
        emit Transfer(msg.sender, to, amount);
        return true;
    }
    
    function approve(address spender, uint256 amount) external returns (bool) {
        allowance[msg.sender][spender] = amount;
        emit Approval(msg.sender, spender, amount);
        return true;
    }
    
    function transferFrom(address from, address to, uint256 amount) external returns (bool) {
        require(from != address(0), "Transfer from zero address");
        require(to != address(0), "Transfer to zero address");
        require(balanceOf[from] >= amount, "Insufficient balance");

        uint256 allowed = allowance[from][msg.sender];
        if (allowed != type(uint256).max) {
            require(allowed >= amount, "Insufficient allowance");
            allowance[from][msg.sender] = allowed - amount;
        }

        unchecked {
            balanceOf[from] -= amount;
            balanceOf[to] += amount;
        }
        emit Transfer(from, to, amount);
        return true;
    }

    function mint(address to, uint256 amount) external onlyGovernance {
        require(to != address(0), "Mint to zero address");
        totalSupply += amount;
        balanceOf[to] += amount;
        emit Mint(to, amount);
        emit Transfer(address(0), to, amount);
    }

    function burn(address from, uint256 amount) external onlyGovernance {
        require(balanceOf[from] >= amount, "Insufficient balance");
        balanceOf[from] -= amount;
        totalSupply -= amount;
        emit Burn(from, amount);
        emit Transfer(from, address(0), amount);
    }

    // Accumulate host functions
    function bless(uint64 m, uint64 v, uint64 r, uint64 n, bytes calldata boldA, bytes calldata boldZ) external onlyGovernance {
        require(n > 0, "Invalid n");
        require(boldA.length == TOTAL_CORES * 4, "Invalid boldA length");
        require(boldZ.length == n * 12, "Invalid boldZ length");
        emit Bless(m, v, r, n, boldA, boldZ);
    }

    function assign(uint64 c, uint64 a, bytes calldata queueWorkReport) external onlyGovernance {
        require(queueWorkReport.length == MAX_AUTH_QUEUE_ITEMS * 32, "Invalid queue length");
        emit Assign(c, a, queueWorkReport);
    }

    function designate(bytes calldata validators) external onlyGovernance {
        require(validators.length == VALIDATOR_BYTES * TOTAL_VALIDATORS, "Invalid validators length");
        emit Designate(validators);
    }

    function newService(bytes calldata codeHash, uint64 l, uint64 g, uint64 m, uint64 f, uint64 i) external onlyGovernance {
        require(codeHash.length == 32, "Invalid code hash length");
        emit New(bytes32(codeHash), l, g, m, f, i);
    }

    function upgrade(bytes calldata codeHash, uint64 g, uint64 m) external onlyGovernance {
        require(codeHash.length == 32, "Invalid code hash length");
        emit Upgrade(bytes32(codeHash), g, m);
    }

    function transferAccumulate(uint64 d, uint64 a, uint64 g, bytes calldata memo) external onlyGovernance {
        require(memo.length == MEMO_SIZE, "Invalid memo length");
        emit TransferAccumulate(d, a, g, memo);
    }

    function eject(uint64 d, bytes calldata hashData) external onlyGovernance {
        require(hashData.length == 32, "Invalid hash length");
        emit Eject(d, bytes32(hashData));
    }

    function write(bytes calldata key, bytes calldata value) external onlyGovernance {
        require(key.length > 0, "Invalid key length");
        require(value.length > 0, "Invalid value length");
        emit Write(key, value);
    }

    function solicit(bytes calldata hashData, uint64 z) external onlyGovernance {
        require(hashData.length == 32, "Invalid hash length");
        emit Solicit(bytes32(hashData), z);
    }

    function forget(bytes calldata hashData, uint64 z) external onlyGovernance {
        require(hashData.length == 32, "Invalid hash length");
        emit Forget(bytes32(hashData), z);
    }

    function provide(uint64 s, uint64 z, bytes calldata data) external onlyGovernance {
        require(data.length == z, "Invalid data length");
        emit Provide(s, z, data);
    }
}