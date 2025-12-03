// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract USDM {
    string public constant name = "USDM";
    string public constant symbol = "USDM";
    uint8 public constant decimals = 18;
    uint256 public constant totalSupply = 61_000_000 * 10**18;
    
    mapping(address => uint256) public balanceOf;                  // slot 0
    mapping(address => uint256) public nonces;                  // slot 1
    mapping(address => mapping(address => uint256)) public allowance; // slot 2
    
    event Transfer(address indexed from, address indexed to, uint256 amount);
    event Approval(address indexed owner, address indexed spender, uint256 amount);
    
    function transfer(address to, uint256 amount) external returns (bool) {
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
        uint256 allowed = allowance[from][msg.sender];
        if (allowed != type(uint256).max) {
            allowance[from][msg.sender] = allowed - amount;
        }
        balanceOf[from] -= amount;
        balanceOf[to] += amount;
        emit Transfer(from, to, amount);
        return true;
    }
}