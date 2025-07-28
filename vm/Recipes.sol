// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.24;

contract Recipes {
    function run(uint8 idx) public pure returns (uint256) {
        if (idx == 0) {
            return tribonacci(10);
        } else if (idx == 1) {
            return narayana(5, 3);
        } else if (idx == 2) {
            return motzkin(10);
        }
        return 0;
    }

    function tribonacci(uint256 n) internal pure returns (uint256) {
        if (n == 0) return 0;
        if (n == 1 || n == 2) return 1;
        uint256 a = 0;
        uint256 b = 1;
        uint256 c = 1;
        uint256 d;
        for (uint256 i = 3; i <= n; ++i) {
            d = a + b + c;
            a = b;
            b = c;
            c = d;
        }
        return c;
    }

    function narayana(uint256 n, uint256 k) internal pure returns (uint256) {
        if (n == 0 || k == 0 || k > n) return 0;
        return (comb(n - 1, k - 1) * comb(n, k)) / n;
    }

    function motzkin(uint256 n) internal pure returns (uint256) {
        if (n == 0) return 1;
        if (n == 1) return 1;
        uint256 a = 1;
        uint256 b = 1;
        uint256 c;
        for (uint256 i = 2; i <= n; ++i) {
            c = ((2 * i + 1) * b + 3 * (i - 1) * a) / (i + 2);
            a = b;
            b = c;
        }
        return b;
    }

    function comb(uint256 n, uint256 k) internal pure returns (uint256) {
        if (k > n) return 0;
        uint256 r = 1;
        for (uint256 i = 0; i < k; ++i) {
            r = (r * (n - i)) / (i + 1);
        }
        return r;
    }

}

