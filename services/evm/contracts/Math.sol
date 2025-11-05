// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

contract Math {
    // ========== EVENTS ==========
    event FibCache(uint256 n, uint256 value);
    event FibComputed(uint256 n, uint256 value);
    event ModExpCache(uint256 base, uint256 exp, uint256 modulus, uint256 result);
    event ModExpComputed(uint256 base, uint256 exp, uint256 modulus, uint256 result);
    event GcdCache(uint256 a, uint256 b, uint256 result);
    event GcdComputed(uint256 a, uint256 b, uint256 result);
    event IntegerSqrtCache(uint256 n, uint256 result);
    event IntegerSqrtComputed(uint256 n, uint256 result);
    event FactCache(uint256 n, uint256 result);
    event FactComputed(uint256 n, uint256 result);
    event IsPrimeCache(uint256 n, bool result);
    event IsPrimeComputed(uint256 n, bool result);
    event NextPrimeCache(uint256 n, uint256 result);
    event NextPrimeComputed(uint256 n, uint256 result);
    event JacobiCache(uint256 a, uint256 n, int256 result);
    event JacobiComputed(uint256 a, uint256 n, int256 result);
    event BinomialCache(uint256 n, uint256 k, uint256 result);
    event BinomialComputed(uint256 n, uint256 k, uint256 result);
    event IsQuadraticResidueCache(uint256 a, uint256 p, bool result);
    event IsQuadraticResidueComputed(uint256 a, uint256 p, bool result);
    event RsaKeygenCache(uint256 seed, uint256 n, uint256 e);
    event RsaKeygenComputed(uint256 seed, uint256 n, uint256 e);
    event BurnsideNecklaceCache(uint256 n, uint256 k, uint256 result);
    event BurnsideNecklaceComputed(uint256 n, uint256 k, uint256 result);
    event FermatFactorCache(uint256 n, uint256 p, uint256 q);
    event FermatFactorComputed(uint256 n, uint256 p, uint256 q);
    event NarayanaCache(uint256 n, uint256 k, uint256 result);
    event NarayanaComputed(uint256 n, uint256 k, uint256 result);
    event YoungTableauxCache(uint256 r, uint256 c, uint256 result);
    event YoungTableauxComputed(uint256 r, uint256 c, uint256 result);

    // ========== CACHE MAPPINGS ==========

    // High Impact Caches (1000+ gas saved)
    mapping(bytes32 => uint256) public modExpCache;
    mapping(bytes32 => bool) public isPrimeCache;
    mapping(bytes32 => uint256) public factCache;
    mapping(bytes32 => bool) public bailliePswCache;
    mapping(bytes32 => bytes) public cipollaCache; // stores (bool, uint256)

    // Medium Impact Caches (500-1000 gas saved)
    mapping(bytes32 => uint256) public gcdCache;
    mapping(bytes32 => uint256) public integerSqrtCache;
    mapping(bytes32 => int256) public jacobiCache;
    mapping(bytes32 => uint256) public binomialCache;
    mapping(bytes32 => uint64) public totientCache;

    // Consistent Impact Caches (200-500 gas saved)
    mapping(bytes32 => uint256) public nextPrimeCache;
    mapping(bytes32 => int8) public legendreSymbolCache;
    mapping(bytes32 => uint256) public partitionModCache;
    mapping(bytes32 => uint256) public sigmaCache;

    // Caller Function Caches (functions that use cached functions)
    mapping(bytes32 => bytes) public nttCache; // stores uint256[8]
    mapping(bytes32 => bool) public isQuadraticResidueCache;
    mapping(bytes32 => bool) public solovayStrassenCache;
    mapping(bytes32 => bytes) public dsaSignCache; // stores (uint256, uint256)
    mapping(bytes32 => bool) public dsaVerifyCache;
    mapping(bytes32 => bool) public isPrimeMillerCache;
    mapping(bytes32 => bool) public isStrongLucasPrpCache;
    mapping(bytes32 => uint256) public diffieHellmanCache;
    mapping(bytes32 => bytes) public rsaKeygenCache; // stores (uint256, uint256, uint256)
    mapping(bytes32 => uint256) public burnsideNecklaceCache;
    mapping(bytes32 => uint256) public frobeniusCache;
    mapping(bytes32 => bytes) public fermatFactorCache; // stores (uint256, uint256)
    mapping(bytes32 => uint256) public narayanaCache;
    mapping(bytes32 => bytes) public smithNormalForm2x2Cache; // stores (int256, int256)
    mapping(bytes32 => uint256) public pollardRhoBrentCache;
    mapping(bytes32 => uint256) public youngTableauxCache;
    mapping(bytes32 => bool) public isWilsonPrimeCache;
    mapping(bytes32 => bool) public lucasLehmerCache;
    mapping(bytes32 => bytes) public lucasSequenceCache; // stores (int256, int256)

    // Fibonacci Cache
    mapping(uint256 => uint256) private fibCache;
    uint256 private lastFibComputed;

    // ========== FIBONACCI FUNCTIONS ==========

    // Iterative Fibonacci with caching (memoization)
    // Function selector: 0x61047ff4
    function fibonacci(uint256 n) public returns (uint256) {
        // Check if value is already cached
        if (fibCache[n] != 0 || n == 0) {
            emit FibCache(n, fibCache[n]);
            return fibCache[n];
        }

        // Initialize a and b from the last cached values
        uint256 a;
        uint256 b;
        uint256 i;

        if (lastFibComputed < 10) {
            // Start from scratch
            a = 0;
            b = 1;
            fibCache[1] = 1;
            i = 2;
        } else {
            a = fibCache[lastFibComputed - 1];
            b = fibCache[lastFibComputed];
            i = lastFibComputed + 1;
        }

        // Calculate and store fibonacci values from i to n
        for (; i <= n; i++) {
            uint256 temp = a + b;
            a = b;
            b = temp;

            // Store fib(i) in cache
            fibCache[i] = temp;
        }

        lastFibComputed = n;
        emit FibComputed(n, fibCache[n]);
        return fibCache[n];
    }

    // Public function to clear Fibonacci cache if needed
    // Function selector: 0x5cc53425
    function clearFibCache() external {
        // Reset cache to base cases only
        fibCache[0] = 0;
        fibCache[1] = 1;
        // Note: Other cached values remain but will be overwritten if recalculated
    }

    // View function to check cached Fibonacci value without computation
    // Function selector: 0x0fcdf678
    function getCachedFib(uint256 n) external view returns (uint256) {
        return fibCache[n];
    }

    // View function to check if a Fibonacci value is cached
    // Function selector: 0xe8f50161
    function isFibCached(uint256 n) external view returns (bool) {
        return (fibCache[n] != 0 || n == 0);
    }
    
    // ========== CORE MATH FUNCTIONS (WITH CACHING) ==========

    // Modular exponentiation: (base^exp) % m - HIGH IMPACT
    // Function selector: 0x3148f14f
    function modExp(uint256 base, uint256 exp, uint256 modulus) public returns (uint256 result) {
        bytes32 key = keccak256(abi.encodePacked("modExp", base, exp, modulus));
        if (modExpCache[key] != 0) {
            emit ModExpCache(base, exp, modulus, modExpCache[key]);
            return modExpCache[key];
        }

        result = 1;
        base = base % modulus;
        while (exp > 0) {
            if (exp & 1 != 0) {
                result = mulmod(result, base, modulus);
            }
            base = mulmod(base, base, modulus);
            exp >>= 1;
        }

        emit ModExpComputed(base, exp, modulus, result);
        modExpCache[key] = result;
        return result;
    }

    // Modular inverse - helper function
    // Function selector: 0x28619888
    function modInv(int256 a, int256 m) public pure returns (int256) {
        int256 t = 0;
        int256 newT = 1;
        int256 r = m;
        int256 newR = a;

        while (newR != 0) {
            int256 quotient = r / newR;
            (t, newT) = (newT, t - quotient * newT);
            (r, newR) = (newR, r - quotient * newR);
        }

        if (r > 1) return -1; // no inverse
        if (t < 0) t += m;
        return t;
    }

    // Euclidean remainder for signed integers: a rem_euclid m
    // Function selector: 0xde3f32a2
    function remEuclid(int256 a, int256 m) public pure returns (int256) {
        int256 r = a % m;
        return r >= 0 ? r : r + m;
    }

    // GCD - MEDIUM IMPACT
    // Function selector: 0xb9650dac
    function gcd(uint256 a, uint256 b) public returns (uint256) {
        bytes32 key = keccak256(abi.encodePacked("gcd", a, b));
        if (gcdCache[key] != 0) {
            emit GcdCache(a, b, gcdCache[key]);
            return gcdCache[key];
        }

        uint256 result;
        while (b != 0) {
            uint256 r = a % b;
            a = b;
            b = r;
        }
        result = a;

        emit GcdComputed(a, b, result);
        gcdCache[key] = result;
        return result;
    }

    // Extended Euclidean Algorithm
    // Function selector: 0xfef7b20d
    function extendedGCD(int256 a, int256 b) public pure returns (int256 g, int256 x, int256 y) {
        x = 1;
        y = 0;
        int256 x1 = 0;
        int256 y1 = 1;
        while (b != 0) {
            int256 q = a / b;
            (a, b) = (b, a % b);
            (x, x1) = (x1, x - q * x1);
            (y, y1) = (y1, y - q * y1);
        }
        g = a;
    }

    // Function selector: 0x7ae2b5c7
    function min(uint256 a, uint256 b) public pure returns (uint256) {
        return a < b ? a : b;
    }

    // Function selector: 0x6d5433e6
    function max(uint256 a, uint256 b) public pure returns (uint256) {
        return a > b ? a : b;
    }

    // Function selector: 0x1b5ac4b5
    function abs(int256 x) public pure returns (uint256) {
        return uint256(x >= 0 ? x : -x);
    }

    // Integer square root - MEDIUM IMPACT
    // Function selector: 0xae24b4b5
    function integerSqrt(uint256 n) public returns (uint256) {
        bytes32 key = keccak256(abi.encodePacked("integerSqrt", n));
        if (integerSqrtCache[key] != 0) {
            emit IntegerSqrtCache(n, integerSqrtCache[key]);
            return integerSqrtCache[key];
        }

        if (n == 0 || n == 1) {
            emit IntegerSqrtComputed(n, n);
            integerSqrtCache[key] = n;
            return n;
        }

        uint256 lo = 0;
        uint256 hi = n;
        while (lo <= hi) {
            uint256 mid = (lo + hi) / 2;
            uint256 sq = mid * mid;
            if (sq == n) {
                emit IntegerSqrtComputed(n, mid);
                integerSqrtCache[key] = mid;
                return mid;
            }
            else if (sq < n) lo = mid + 1;
            else hi = mid - 1;
        }

        emit IntegerSqrtComputed(n, hi);
        integerSqrtCache[key] = hi;
        return hi;
    }

    // Factorial function - HIGH IMPACT
    // Function selector: 0x0b95d228
    function fact(uint256 n) public returns (uint256) {
        bytes32 key = keccak256(abi.encodePacked("fact", n));
        if (factCache[key] != 0) {
            emit FactCache(n, factCache[key]);
            return factCache[key];
        }

        uint256 f = 1;
        for (uint256 i = 2; i <= n; i++) {
            f *= i;
        }

        emit FactComputed(n, f);
        factCache[key] = f;
        return f;
    }

    // Check primality using trial division - HIGH IMPACT
    // Function selector: 0x42703494
    function isPrime(uint256 n) public returns (bool) {
        bytes32 key = keccak256(abi.encodePacked("isPrime", n));
        uint256 cached = isPrimeCache[key] ? 1 : 0;
        if (cached != 0) {
            emit IsPrimeCache(n, isPrimeCache[key]);
            return isPrimeCache[key];
        }

        if (n < 2) {
            emit IsPrimeComputed(n, false);
            isPrimeCache[key] = false;
            return false;
        }
        if (n % 2 == 0) {
            bool result = n == 2;
            emit IsPrimeComputed(n, result);
            isPrimeCache[key] = result;
            return result;
        }

        for (uint256 i = 3; i * i <= n; i += 2) {
            if (n % i == 0) {
                emit IsPrimeComputed(n, false);
                isPrimeCache[key] = false;
                return false;
            }
        }

        emit IsPrimeComputed(n, true);
        isPrimeCache[key] = true;
        return true;
    }

    // Return next prime ≥ (n+1) - CONSISTENT IMPACT
    // Function selector: 0xb67b60c7
    function nextPrime(uint256 n) public returns (uint256) {
        bytes32 key = keccak256(abi.encodePacked("nextPrime", n));
        if (nextPrimeCache[key] != 0) {
            emit NextPrimeCache(n, nextPrimeCache[key]);
            return nextPrimeCache[key];
        }

        if (n < 2) {
            emit NextPrimeComputed(n, 2);
            nextPrimeCache[key] = 2;
            return 2;
        }
        n += 1;
        while (!isPrime(n)) {
            n += 1;
        }

        emit NextPrimeComputed(n, n);
        nextPrimeCache[key] = n;
        return n;
    }

    // Jacobi symbol - MEDIUM IMPACT
    // Function selector: 0xdca4f74b
    function jacobi(uint256 a, uint256 n) public returns (int256) {
        bytes32 key = keccak256(abi.encodePacked("jacobi", a, n));
        if (jacobiCache[key] != 0) {
            emit JacobiCache(a, n, jacobiCache[key]);
            return jacobiCache[key];
        }

        if (n == 0 || n % 2 == 0) {
            emit JacobiComputed(a, n, 0);
            jacobiCache[key] = 0;
            return 0;
        }

        int256 t = 1;
        a = a % n;

        while (a != 0) {
            while (a % 2 == 0) {
                a /= 2;
                uint256 r = n % 8;
                if (r == 3 || r == 5) {
                    t = -t;
                }
            }

            (a, n) = (n, a);

            if ((a % 4 == 3) && (n % 4 == 3)) {
                t = -t;
            }

            a = a % n;
        }

        int256 result = n == 1 ? t : int256(0);
        emit JacobiComputed(a, n, result);
        jacobiCache[key] = result;
        return result;
    }

    // Binomial coefficient C(n, k) - MEDIUM IMPACT
    // Function selector: 0x3a14bf4c
    function binomial(uint256 n, uint256 k) public returns (uint256) {
        bytes32 key = keccak256(abi.encodePacked("binomial", n, k));
        if (binomialCache[key] != 0) {
            emit BinomialCache(n, k, binomialCache[key]);
            return binomialCache[key];
        }

        if (k > n) {
            emit BinomialComputed(n, k, 0);
            binomialCache[key] = 0;
            return 0;
        }
        if (k > n - k) {
            k = n - k;
        }
        uint256 res = 1;
        for (uint256 i = 1; i <= k; i++) {
            res = (res * (n + 1 - i)) / i;
        }

        emit BinomialComputed(n, k, res);
        binomialCache[key] = res;
        return res;
    }

    // Euler's criterion: returns true if a is a quadratic residue mod p - CACHED CALLER
    // Function selector: 0x126766f2
    function isQuadraticResidue(uint256 a, uint256 p) public returns (bool) {
        bytes32 key = keccak256(abi.encodePacked("isQuadraticResidue", a, p));
        uint256 cached = isQuadraticResidueCache[key] ? 1 : 0;
        if (cached != 0) {
            emit IsQuadraticResidueCache(a, p, isQuadraticResidueCache[key]);
            return isQuadraticResidueCache[key];
        }

        if (a % p == 0) {
            emit IsQuadraticResidueComputed(a, p, true);
            isQuadraticResidueCache[key] = true;
            return true;
        }
        bool result = modExp(a % p, (p - 1) / 2, p) == 1;
        emit IsQuadraticResidueComputed(a, p, result);
        isQuadraticResidueCache[key] = result;
        return result;
    }

    // RSA key generation - CACHED CALLER
    // Function selector: 0x5169f08b
    function rsaKeygen(uint256 seed) public returns (uint256 n, uint256 e, uint256 d) {
        bytes32 key = keccak256(abi.encodePacked("rsaKeygen", seed));
        if (rsaKeygenCache[key].length > 0) {
            (n, e, d) = abi.decode(rsaKeygenCache[key], (uint256, uint256, uint256));
            emit RsaKeygenCache(seed, n, e);
            return (n, e, d);
        }

        uint256 p = nextPrime(uint256(keccak256(abi.encodePacked(seed, "p"))) % 1000 + 100);

        uint256 q;
        while (true) {
            q = nextPrime(uint256(keccak256(abi.encodePacked(seed, "q", q))) % 1000 + 100);
            if (q != p) break;
        }

        n = p * q;
        uint256 phi = (p - 1) * (q - 1);
        e = 65537 % phi;

        int256 dSigned = modInv(int256(e), int256(phi));
        require(dSigned > 0, "No modular inverse");
        d = uint256(dSigned);

        emit RsaKeygenComputed(seed, n, e);
        rsaKeygenCache[key] = abi.encode(n, e, d);
        return (n, e, d);
    }

    // Burnside's Lemma - CACHED CALLER
    // Function selector: 0xcf74db35
    function burnsideNecklace(uint256 n, uint256 k) public returns (uint256) {
        bytes32 key = keccak256(abi.encodePacked("burnsideNecklace", n, k));
        if (burnsideNecklaceCache[key] != 0) {
            emit BurnsideNecklaceCache(n, k, burnsideNecklaceCache[key]);
            return burnsideNecklaceCache[key];
        }

        uint256 sum = 0;
        for (uint256 r = 0; r < n; r++) {
            uint256 g = gcd(r, n);
            sum += k ** g;
        }
        uint256 result = sum / n;

        emit BurnsideNecklaceComputed(n, k, result);
        burnsideNecklaceCache[key] = result;
        return result;
    }

    // Fermat factorization - CACHED CALLER
    // Function selector: 0xd1f7033f
    function fermatFactor(uint256 n) public returns (uint256, uint256) {
        bytes32 key = keccak256(abi.encodePacked("fermatFactor", n));
        if (fermatFactorCache[key].length > 0) {
            (uint256 p, uint256 q) = abi.decode(fermatFactorCache[key], (uint256, uint256));
            emit FermatFactorCache(n, p, q);
            return (p, q);
        }

        if (n < 2) {
            emit FermatFactorComputed(n, 0, 0);
            fermatFactorCache[key] = abi.encode(uint256(0), uint256(0));
            return (0, 0);
        }

        uint256 x = integerSqrt(n) + 1;
        for (uint256 iterations = 0; iterations < 10000; iterations++) {
            uint256 t = x * x - n;
            uint256 y = integerSqrt(t);
            if (y * y == t) {
                uint256 a = x - y;
                uint256 b = x + y;
                emit FermatFactorComputed(n, a, b);
                fermatFactorCache[key] = abi.encode(a, b);
                return (a, b);
            }
            unchecked { x += 1; }
        }

        emit FermatFactorComputed(n, 0, 0);
        fermatFactorCache[key] = abi.encode(uint256(0), uint256(0));
        return (0, 0);
    }

    // Narayana number - CACHED CALLER
    // Function selector: 0x590eb433
    function narayana(uint256 n, uint256 k) public returns (uint256) {
        bytes32 key = keccak256(abi.encodePacked("narayana", n, k));
        if (narayanaCache[key] != 0) {
            emit NarayanaCache(n, k, narayanaCache[key]);
            return narayanaCache[key];
        }

        if (k < 1 || k > n) {
            emit NarayanaComputed(n, k, 0);
            narayanaCache[key] = 0;
            return 0;
        }
        uint256 c1 = binomial(n, k);
        uint256 c2 = binomial(n, k - 1);
        uint256 result = (c1 * c2) / n;

        emit NarayanaComputed(n, k, result);
        narayanaCache[key] = result;
        return result;
    }

    // Young Tableaux count - CACHED CALLER
    // Function selector: 0x026b141c
    function youngTableaux(uint256 r, uint256 c) public returns (uint256) {
        bytes32 key = keccak256(abi.encodePacked("youngTableaux", r, c));
        if (youngTableauxCache[key] != 0) {
            emit YoungTableauxCache(r, c, youngTableauxCache[key]);
            return youngTableauxCache[key];
        }

        uint256 n = r * c;
        uint256 denom = 1;

        for (uint256 i = 0; i < r; i++) {
            for (uint256 j = 0; j < c; j++) {
                uint256 hook = (r - i) + (c - j) - 1;
                denom *= hook;
            }
        }

        uint256 result = fact(n) / denom;
        emit YoungTableauxComputed(r, c, result);
        youngTableauxCache[key] = result;
        return result;
    }


    // Clear cache functions for testing/management
    // Function selector: 0x64c9bd8b
    function clearModExpCache(bytes32 key) external {
        delete modExpCache[key];
    }

}