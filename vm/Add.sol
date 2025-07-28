// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.0;

contract Add {
    function add(uint256 a, uint256 b) external pure returns (uint256) {
        return a + b;
    }
}