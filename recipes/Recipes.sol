// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

library MathLib {
    // Modular exponentiation: (base^exp) % m
    function modExp(uint256 base, uint256 exp, uint256 modulus) internal pure returns (uint256 result) {
        result = 1;
        base = base % modulus;
        while (exp > 0) {
            if (exp & 1 != 0) {
                result = mulmod(result, base, modulus);
            }
            base = mulmod(base, base, modulus);
            exp >>= 1;
        }
    }

    function modInv(int256 a, int256 m) internal pure returns (int256) {
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
    function remEuclid(int256 a, int256 m) internal pure returns (int256) {
        int256 r = a % m;
        return r >= 0 ? r : r + m;
    }

    function gcd(uint256 a, uint256 b) internal pure returns (uint256) {
        while (b != 0) {
            uint256 r = a % b;
            a = b;
            b = r;
        }
        return a;
    }

    // Extended Euclidean Algorithm
    // Returns (g, x, y) such that g = gcd(a, b), and ax + by = g
    function extendedGCD(int256 a, int256 b) internal pure returns (int256 g, int256 x, int256 y) {
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

    function min(uint256 a, uint256 b) internal pure returns (uint256) {
        return a < b ? a : b;
    }

    function max(uint256 a, uint256 b) internal pure returns (uint256) {
        return a > b ? a : b;
    }

    function abs(int256 x) internal pure returns (uint256) {
        return uint256(x >= 0 ? x : -x);
    }

    function integerSqrt(uint256 n) internal pure returns (uint256) {
        if (n == 0 || n == 1) return n;
        uint256 lo = 0;
        uint256 hi = n;
        while (lo <= hi) {
            uint256 mid = (lo + hi) / 2;
            uint256 sq = mid * mid;
            if (sq == n) return mid;
            else if (sq < n) lo = mid + 1;
            else hi = mid - 1;
        }
        return hi;
    }

    function integerNthRoot(uint256 n, uint256 k) internal pure returns (uint256) {
        if (k == 0) return 0;
        if (n == 0 || n == 1) return n;

        uint256 lo = 0;
        uint256 hi = min(n, 1 << (64 / k)) + 1;

        while (lo < hi) {
            uint256 mid = (lo + hi + 1) >> 1;
            uint256 acc = 1;
            bool overflow = false;

            for (uint256 i = 0; i < k; i++) {
                uint256 prev = acc;
                acc = acc * mid;
                if (acc / mid != prev || acc > n) {
                    overflow = true;
                    break;
                }
            }

            if (!overflow && acc <= n) {
                lo = mid;
            } else {
                hi = mid - 1;
            }
        }

        return lo;
    }

    function trailingZeros(uint256 x) internal pure returns (uint256) {
        if (x == 0) return 256;
        uint256 tz = 0;
        while ((x & 1) == 0) {
            x >>= 1;
            tz++;
        }
        return tz;
    }

    // Factorial function
    function fact(uint256 n) public pure returns (uint256) {
        uint256 f = 1;
        for (uint256 i = 2; i <= n; i++) {
            f *= i;
        }
        return f;
    }
    
    function integerLogMul(uint256 n, uint256 b) internal pure returns (uint256) {
        if (b < 2) return 0;
        uint256 p = b;
        uint256 k = 0;
        while (p <= n) {
            uint256 next = p * b;
            if (next / b != p) break;
            p = next;
            k++;
        }
        return k;
    }

    function steinGCD(uint256 a, uint256 b) internal pure returns (uint256) {
        if (a == 0) return b;
        if (b == 0) return a;

        uint256 shift = trailingZeros(a | b);
        a >>= trailingZeros(a);

        while (b != 0) {
            b >>= trailingZeros(b);
            if (a > b) (a, b) = (b, a);
            b -= a;
        }

        return a << shift;
    }

    function subGCD(uint256 a, uint256 b) internal pure returns (uint256) {
        if (a == 0) return b;
        if (b == 0) return a;

        while (a != b) {
            if (a > b) a -= b;
            else b -= a;
        }

        return a;
    }



    // Computes floor(log_b(n)) using repeated division
    function integerLogDiv(uint256 n, uint256 b) public pure returns (uint256) {
        if (b < 2) return 0;

        uint256 k = 0;
        while (n >= b) {
            n /= b;
            k += 1;
        }

        return k;
    }
}

contract Recipes {
    using MathLib for uint256;
    using MathLib for int256;

    uint256 constant MOD_NTT = 17;
    uint256 constant NTT_N = 8;
    uint256 constant ROOT = 3;

    // NTT for length-8 input over mod 17 with root 3
    function ntt(uint256[8] memory a) public pure returns (uint256[8] memory out) {
        uint256 w = MathLib.modExp(ROOT, (MOD_NTT - 1) / NTT_N, MOD_NTT);

        for (uint256 k = 0; k < NTT_N; k++) {
            uint256 sum = 0;
            for (uint256 j = 0; j < NTT_N; j++) {
                uint256 wjk = MathLib.modExp(w, (j * k) % NTT_N, MOD_NTT);
                sum = addmod(sum, mulmod(a[j], wjk, MOD_NTT), MOD_NTT);
            }
            out[k] = sum;
        }
    }


    // Euler's criterion: returns true if a is a quadratic residue mod p
    function isQuadraticResidue(uint256 a, uint256 p) public pure returns (bool) {
        if (a % p == 0) {
            return true;
        }
        return MathLib.modExp(a % p, (p - 1) / 2, p) == 1;
    }


    // Returns true if p is a Wilson prime: (p-1)! ≡ -1 mod p
    function isWilsonPrime(uint256 p) public pure returns (bool) {
        if (p < 2) {
            return false;
        }

        uint256 f = 1;
        for (uint256 i = 1; i < p; i++) {
            f = (f * i) % p;
        }

        return f == p - 1;
    }

    function jacobi(uint256 a, uint256 n) public pure returns (int256) {
        if (n == 0 || n % 2 == 0) {
            return 0;
        }

        int256 t = 1;
        a = a % n;

        while (a != 0) {
            // While a is even
            while (a % 2 == 0) {
                a /= 2;
                uint256 r = n % 8;
                if (r == 3 || r == 5) {
                    t = -t;
                }
            }

            // Swap a and n
            (a, n) = (n, a);

            // If a ≡ n ≡ 3 mod 4
            if ((a % 4 == 3) && (n % 4 == 3)) {
                t = -t;
            }

            a = a % n;
        }

        return n == 1 ? t : int256(0);
    }

    // Solovay–Strassen primality test
    function solovayStrassen(uint256 n, uint32 k, uint256 seed) public pure returns (bool) {
        if (n < 2) return false;

        for (uint32 i = 0; i < k; i++) {
            // TODO: replace this with PRG
            uint256 a = (uint256(keccak256(abi.encodePacked(seed, i))) % (n - 2)) + 2;

            int256 x = jacobi(a, n);
            if (x == 0) return false;

            uint256 r = MathLib.modExp(a, (n - 1) / 2, n);
            int256 rSigned = (r > n / 2) ? int256(r) - int256(n) : int256(r);

            if ((rSigned - x) % int256(n) != 0) {
                return false;
            }
        }
        return true;
    }

   struct Complex {
        uint256 x;
        uint256 y;
        uint256 w;
        uint256 p;
    }

    function cipolla(uint256 n, uint256 p) public pure returns (bool, uint256) {
        if (n == 0) return (true, 0);
        if (MathLib.modExp(n, (p - 1) / 2, p) != 1) return (false, 0); // not a quadratic residue

        uint256 a = 0;
        uint256 w = 0;

        for (uint256 x = 1; x < p; ++x) {
            w = addmod(mulmod(x, x, p), p - (n % p), p);
            if (MathLib.modExp(w, (p - 1) / 2, p) == p - 1) {
                a = x;
                break;
            }
        }

        Complex memory comp = Complex(a, 1, w, p);
        Complex memory res = powComplex(comp, (p + 1) / 2);

        return (true, res.x);
    }

    function powComplex(Complex memory base, uint256 exp) internal pure returns (Complex memory) {
        Complex memory result = Complex(1, 0, base.w, base.p);

        while (exp > 0) {
            if (exp & 1 != 0) {
                result = mulComplex(result, base);
            }
            base = mulComplex(base, base);
            exp >>= 1;
        }

        return result;
    }

    function mulComplex(Complex memory a, Complex memory b) internal pure returns (Complex memory) {
        uint256 p = a.p;
        uint256 w = a.w;
        uint256 x = addmod(mulmod(a.x, b.x, p), mulmod(mulmod(a.y, b.y, p), w, p), p);
        uint256 y = addmod(mulmod(a.x, b.y, p), mulmod(a.y, b.x, p), p);
        return Complex(x, y, w, p);
    }

    // Fermat factorization: returns (a, b) such that a * b == n, or (0, 0) if not found
    function fermatFactor(uint256 n) public pure returns (uint256, uint256) {
        if (n < 2) return (0, 0);

        uint256 x = MathLib.integerSqrt(n) + 1;
        while (true) {
            uint256 t = x * x - n;
            uint256 y = MathLib.integerSqrt(t);
            if (y * y == t) {
                uint256 a = x - y;
                uint256 b = x + y;
                return (a, b);
            }
            unchecked { x += 1; } // Allow unchecked increment for gas savings
        }
        return (0, 0);
    }

    // Burnside's Lemma: count distinct colorings of necklaces with n beads and k colors
    function burnsideNecklace(uint256 n, uint256 k) public pure returns (uint256) {
        uint256 sum = 0;
        for (uint256 r = 0; r < n; r++) {
            uint256 g = MathLib.gcd(r, n);
            sum += k ** g;
        }
        return sum / n;
    }

    // Frobenius number for two coprime integers a and b
    function frobenius(uint256 a, uint256 b) public pure returns (uint256) {
        if (MathLib.gcd(a, b) != 1) {
            return 0;
        }
        return a * b - a - b;
    }

    // Computes p(n) mod m using Euler's pentagonal number theorem
    function partitionMod(uint256 n, uint256 m) public pure returns (uint256) {
        uint256[] memory p = new uint256[](n + 1);
        p[0] = 1;

        for (uint256 i = 1; i <= n; i++) {
            uint256 k = 1;
            while (true) {
                uint256 g1 = k * (3 * k - 1) / 2;
                uint256 g2 = k * (3 * k + 1) / 2;

                if (g1 > i) {
                    break;
                }

                // sign = +1 if k is odd, -1 mod m if k is even
                uint256 sign = (k % 2 == 0) ? m - 1 : 1;

                p[i] = (p[i] + sign * p[i - g1] % m) % m;

                if (g2 <= i) {
                    p[i] = (p[i] + sign * p[i - g2] % m) % m;
                }

                k += 1;
            }
        }

        return p[n];
    }    

    // Estimate condition number using ∞-norm for a 2×2 matrix
    function conditionNumber2x2(int256[2][2] memory a) public pure returns (uint256) {
        uint256 normA = MathLib.max(
            MathLib.abs(a[0][0]) + MathLib.abs(a[0][1]),
            MathLib.abs(a[1][0]) + MathLib.abs(a[1][1])
        );

        int256 det = a[0][0] * a[1][1] - a[0][1] * a[1][0];
        if (det == 0) {
            return type(uint256).max; // equivalent to u64::MAX
        }

        uint256 normInv = MathLib.max(
            MathLib.abs(a[1][1]) + MathLib.abs(a[0][1]),
            MathLib.abs(a[0][0]) + MathLib.abs(a[1][0])
        );

        return (normA * normInv) / MathLib.abs(det);
    }

    // Check primality using trial division
    function isPrime(uint256 n) public pure returns (bool) {
        if (n < 2) return false;
        if (n % 2 == 0) return n == 2;

        for (uint256 i = 3; i * i <= n; i += 2) {
            if (n % i == 0) return false;
        }
        return true;
    }

    // Return next prime ≥ (n+1)
    function nextPrime(uint256 n) public pure returns (uint256) {
        if (n < 2) return 2;
        n += 1;
        while (!isPrime(n)) {
            n += 1;
        }
        return n;
    }

    // Generate RSA key triple (n, e, d) from a seed
    function rsaKeygen(uint256 seed) public pure returns (uint256 n, uint256 e, uint256 d) {
        uint256 p = nextPrime(uint256(keccak256(abi.encodePacked(seed, "p"))) % 1000 + 100);

        uint256 q;
        while (true) {
            q = nextPrime(uint256(keccak256(abi.encodePacked(seed, "q", q))) % 1000 + 100);
            if (q != p) break;
        }

        n = p * q;
        uint256 phi = (p - 1) * (q - 1);
        e = 65537 % phi;

        int256 dSigned = MathLib.modInv(int256(e), int256(phi));
        require(dSigned > 0, "No modular inverse");
        d = uint256(dSigned);
    }

    // DSA Signing (requires seed to simulate randomness)
    function dsaSign(
        uint256 m,
        uint256 p,
        uint256 q,
        uint256 g,
        uint256 x,
        uint256 seed
    ) public pure returns (uint256 r, uint256 s) {
        uint256 k = (uint256(keccak256(abi.encodePacked(seed))) % (q - 1)) + 1;
        r = MathLib.modExp(g, k, p) % q;
        int256 kinv = MathLib.modInv(int256(k), int256(q));
        require(kinv > 0, "No inverse");
        s = mulmod(uint256(kinv), (m + mulmod(x, r, q)) % q, q);
    }

    // DSA Verification
    function dsaVerify(
        uint256 m,
        uint256 r,
        uint256 s,
        uint256 p,
        uint256 q,
        uint256 g,
        uint256 y
    ) public pure returns (bool) {
        if (r == 0 || r >= q || s == 0 || s >= q) {
            return false;
        }

        int256 winv = MathLib.modInv(int256(s), int256(q));
        if (winv < 0) return false;

        uint256 w = uint256(winv);
        uint256 u1 = mulmod(m, w, q);
        uint256 u2 = mulmod(r, w, q);
        uint256 v1 = MathLib.modExp(g, u1, p);
        uint256 v2 = MathLib.modExp(y, u2, p);
        uint256 v = mulmod(v1, v2, p) % q;

        return v == r;
    }

    // Legendre symbol (a | p)
    function legendreSymbol(int256 a, int256 p) public pure returns (int8) {
        require(p > 2 && p % 2 == 1, "p must be an odd prime");

        uint256 aMod = uint256((a % p + p) % p); // a rem_euclid p
        uint256 exp = uint256((p - 1) / 2);
        uint256 ls = MathLib.modExp(aMod, exp, uint256(p));

        if (ls == 1) {
            return 1;
        } else if (ls == uint256(p) - 1) {
            return -1;
        } else {
            return 0;
        }
    }

    // Lucas–Lehmer test for Mersenne primes
    function lucasLehmer(uint256 p) public pure returns (bool) {
        if (p == 2) return true;

        uint256 m = (1 << p) - 1; // Mersenne number M_p = 2^p - 1
        uint256 s = 4;

        for (uint256 i = 0; i < p - 2; i++) {
            s = addmod(mulmod(s, s, m), m - 2, m); // s = (s * s - 2) mod m
        }

        return s == 0;
    }

    // Lucas sequence (U_n, V_n) mod m for given P, Q
    function lucasSequence(uint256 n, int256 P, int256 Q, int256 m) public pure returns (int256, int256) {
        require(m != 0, "Modulus m cannot be zero");

        if (n == 0) {
            return (0, MathLib.remEuclid(2, m));
        }
        if (n == 1) {
            return (MathLib.remEuclid(1, m), MathLib.remEuclid(P, m));
        }

        int256 U0 = 0;
        int256 V0 = MathLib.remEuclid(2, m);
        int256 U1 = MathLib.remEuclid(1, m);
        int256 V1 = MathLib.remEuclid(P, m);

        for (uint256 i = 2; i <= n; i++) {
            int256 U2 = MathLib.remEuclid(P * U1 - Q * U0, m);
            int256 V2 = MathLib.remEuclid(P * V1 - Q * V0, m);
            U0 = U1;
            V0 = V1;
            U1 = U2;
            V1 = V2;
        }

        return (U1, V1);
    }


    // Miller–Rabin test using strong bases
    function isPrimeMiller(uint256 n) public pure returns (bool) {
        if (n < 2) return false;
        uint256[7] memory bases = [uint256(2), 325, 9375, 28178, 450775, 9780504, 1795265022];
        for (uint256 i = 0; i < bases.length; i++) {
            uint256 a = bases[i];
            if (a % n == 0) continue;

            uint256 d = n - 1;
            uint256 s = 0;
            while (d & 1 == 0) {
                d >>= 1;
                s += 1;
            }

            uint256 x = MathLib.modExp(a, d, n);
            if (x == 1 || x == n - 1) continue;

            bool found = false;
            for (uint256 j = 1; j < s; j++) {
                x = mulmod(x, x, n);
                if (x == n - 1) {
                    found = true;
                    break;
                }
            }
            if (!found) return false;
        }
        return true;
    }

    // Strong Lucas pseudoprime test (Selfridge method)
    function isStrongLucasPrp(uint256 n) public pure returns (bool) {
        if (n < 2) return false;

        int256 D = 5;
        int256 sign = 1;
        while (legendreSymbol(D, int256(n)) != -1) {
            D = -D - 2 * sign;
            sign = -sign;
        }

        int256 P = 1;
        int256 Q = (1 - D) / 4;

        uint256 d = n + 1;
        uint256 s = 0;
        while (d & 1 == 0) {
            d >>= 1;
            s += 1;
        }

        (int256 U, int256 V) = lucasSequence(d, P, Q, int256(n));
        if (U == 0) return true;

        for (uint256 i = 1; i < s; i++) {
            U = MathLib.remEuclid(U * V, int256(n));
            V = MathLib.remEuclid(V * V - 2 * int256(MathLib.modExp(uint256(MathLib.remEuclid(Q, int256(n))), 1, n)), int256(n));
            if (U == 0) return true;
        }

        return false;
    }

    // Baillie–PSW primality test
    function bailliePsw(uint256 n) public pure returns (bool) {
        return isPrimeMiller(n) && isStrongLucasPrp(n);
    }


    // Newton's method for integer square root
    function newtonSqrt(uint256 n) public pure returns (uint256) {
        if (n < 2) return n;

        uint256 x = n;
        while (true) {
            uint256 y = (x + n / x) >> 1;
            if (y >= x) {
                return x;
            }
            x = y;
        }
        return 0;
    }


    // Compute determinant directly for 3×3 matrix
    function det3Direct(int256[3][3] memory m) public pure returns (int256) {
        return
            m[0][0] * (m[1][1] * m[2][2] - m[1][2] * m[2][1]) -
            m[0][1] * (m[1][0] * m[2][2] - m[1][2] * m[2][0]) +
            m[0][2] * (m[1][0] * m[2][1] - m[1][1] * m[2][0]);
    }

    // Bareiss algorithm for 3×3 determinant with pivot guard
    function detBareiss3x3(int256[3][3] memory mat) public pure returns (int256) {
        int256[3][3] memory m2 = mat;
        int256 denom = 1;

        for (uint256 k = 0; k < 2; k++) {
            int256 pivot = m2[k][k];

            if (pivot == 0 || denom == 0) {
                return det3Direct(mat); // fallback
            }

            for (uint256 i = k + 1; i < 3; i++) {
                for (uint256 j = k + 1; j < 3; j++) {
                    m2[i][j] = (m2[i][j] * pivot - m2[i][k] * m2[k][j]) / denom;
                }
            }

            denom = pivot;
        }

        return m2[2][2];
    }


    // Smith normal form invariant factors (d1, d2) for 2×2 integer matrix
    function smithNormalForm2x2(int256[2][2] memory mat) public pure returns (int256 d1, int256 d2) {
        int256 a = mat[0][0];
        int256 b = mat[0][1];
        int256 c = mat[1][0];
        int256 d = mat[1][1];

        uint256 g = MathLib.gcd(MathLib.abs(a), MathLib.abs(b));
        g = MathLib.gcd(g, MathLib.abs(c));
        g = MathLib.gcd(g, MathLib.abs(d));

        int256 det = a * d - b * c;
        d1 = int256(g);
        d2 = int256(uint256(MathLib.abs(det)) / g);
    }


    // Computes HNF of a 2x2 matrix
    function hermiteNormalForm2x2(int256[2][2] memory mat) public pure returns (int256[2][2] memory h) {
        int256 m00 = mat[0][0];
        int256 m10 = mat[1][0];
        int256 m01 = mat[0][1];
        int256 m11 = mat[1][1];

        // If the first column is entirely zero, return canonicalized diagonal
        if (m00 == 0 && m10 == 0) {
            h[0][0] = 0;
            h[1][0] = 0;
            h[0][1] = m01;
            h[1][1] = m11;
            return h;
        }

        (int256 g, int256 s, int256 t) = MathLib.extendedGCD(m00, m10);
        int256 u = m00 / g;
        int256 v = m10 / g;

        int256 a = g;
        int256 b = ((s * m01 + t * m11) % a + a) % a;
        int256 d = -v * m01 + u * m11;

        h[0][0] = a;
        h[0][1] = b;
        h[1][0] = 0;
        h[1][1] = d;

        return h;
    }

    // 2D LLL reduction of integer basis vectors
    function lllReduce2D(int256[2] memory b1, int256[2] memory b2) public pure returns (int256[2] memory r1, int256[2] memory r2) {
        int256[2] memory v1 = b1;
        int256[2] memory v2 = b2;

        while (true) {
            int256 dot = v2[0] * v1[0] + v2[1] * v1[1];
            int256 norm = v1[0] * v1[0] + v1[1] * v1[1];
            if (norm == 0) break;

            int256 mu = (2 * dot + norm) / (2 * norm);
            int256[2] memory newV2 = [v2[0] - mu * v1[0], v2[1] - mu * v1[1]];

            int256 newNorm = newV2[0] * newV2[0] + newV2[1] * newV2[1];

            if (newNorm < norm) {
                // Swap b1 and new b2
                v2 = v1;
                v1 = newV2;
            } else {
                v2 = newV2;
                break;
            }
        }

        return (v1, v2);
    }

    // Modular exponentiation using Montgomery ladder
    function modExpLadder(uint256 base, uint256 exp, uint256 m) public pure returns (uint256) {
        uint256 r0 = 1 % m;
        uint256 r1 = base % m;

        for (uint256 i = 0; i < 64; i++) {
            uint256 bit = (exp >> (63 - i)) & 1;
            if (bit == 0) {
                r1 = mulmod(r0, r1, m);
                r0 = mulmod(r0, r0, m);
            } else {
                r0 = mulmod(r0, r1, m);
                r1 = mulmod(r1, r1, m);
            }
        }

        return r0;
    }


    // Check if n is a perfect square using binary search
    function isPerfectSquare(uint256 n) public pure returns (bool) {
        if (n < 2) return true;

        uint256 lo = 0;
        uint256 hi = (n >> 1) + 1;

        while (lo <= hi) {
            uint256 mid = (lo + hi) >> 1;
            uint256 sq = mid * mid;

            if (sq == n) {
                return true;
            } else if (sq < n) {
                lo = mid + 1;
            } else {
                hi = mid - 1;
            }
        }

        return false;
    }

    // Count number of ways to make change for n cents using coins 1,5,10,25
    function coinChangeCount(uint256 n) public pure returns (uint256) {
        uint256[4] memory coins = [uint256(1), 5, 10, 25];
        uint256[] memory dp = new uint256[](n + 1);
        dp[0] = 1;

        for (uint256 ci = 0; ci < coins.length; ci++) {
            uint256 c = coins[ci];
            for (uint256 i = c; i <= n; i++) {
                dp[i] += dp[i - c];
            }
        }

        return dp[n];
    }

    // Find minimum number of coins needed to make n cents using coins 1,5,10,25
    function coinChangeMin(uint256 n) public pure returns (int256) {
        uint256[4] memory coins = [uint256(1), 5, 10, 25];
        int256[] memory dp = new int256[](n + 1);

        // Initialize dp array to "infinity"
        for (uint256 i = 1; i <= n; i++) {
            dp[i] = type(int256).max;
        }

        dp[0] = 0;

        for (uint256 ci = 0; ci < coins.length; ci++) {
            uint256 c = coins[ci];
            for (uint256 i = c; i <= n; i++) {
                if (dp[i - c] != type(int256).max) {
                    int256 candidate = dp[i - c] + 1;
                    if (candidate < dp[i]) {
                        dp[i] = candidate;
                    }
                }
            }
        }

        return dp[n] == type(int256).max ? -1 : dp[n];
    }

    // Chinese Remainder Theorem for two congruences:
    // x ≡ a1 (mod n1)
    // x ≡ a2 (mod n2)
    function crt2(int256 a1, int256 n1, int256 a2, int256 n2) public pure returns (int256) {
        int256 inv = MathLib.modInv(n1, n2);
        int256 t = MathLib.remEuclid((a2 - a1) * inv, n2);
        return a1 + n1 * t;
    }

    // Garner's algorithm for solving system of modular equations
    function garner(int256[] memory a, int256[] memory n) public pure returns (int256) {
        require(a.length == n.length, "Mismatched input lengths");
        int256 x = a[0];
        int256 prod = n[0];

        for (uint256 i = 1; i < a.length; i++) {
            int256 ni = n[i];
            int256 inv = MathLib.modInv(MathLib.remEuclid(prod, ni), ni);
            int256 t = MathLib.remEuclid((a[i] - x) * inv, ni);
            x += prod * t;
            prod *= ni;
        }

        return x;
    }
    function tribonacci(uint256 n) public pure returns (uint256) {
        if (n == 0) {
            return 0;
        } else if (n == 1 || n == 2) {
            return 1;
        } else {
            uint256 a = 0;
            uint256 b = 1;
            uint256 c = 1;
            for (uint256 i = 3; i <= n; i++) {
                uint256 d = a + b + c;
                a = b;
                b = c;
                c = d;
            }
            return c;
        }
    }


    // Computes binomial coefficient C(n, k)
    function binomial(uint256 n, uint256 k) public pure returns (uint256) {
        if (k > n) return 0;
        if (k > n - k) {
            k = n - k;
        }
        uint256 res = 1;
        for (uint256 i = 1; i <= k; i++) {
            res = (res * (n + 1 - i)) / i;
        }
        return res;
    }

    // Computes Narayana number N(n, k) = (1/n) * C(n, k) * C(n, k-1)
    function narayana(uint256 n, uint256 k) public pure returns (uint256) {
        if (k < 1 || k > n) {
            return 0;
        }
        uint256 c1 = binomial(n, k);
        uint256 c2 = binomial(n, k - 1);
        return (c1 * c2) / n;

    }

    function motzkin(uint256 n) public pure returns (uint256) {
        if (n < 2) {
            return 1;
        }

        uint256[] memory m = new uint256[](n + 1);
        m[0] = 1;
        m[1] = 1;

        for (uint256 i = 2; i <= n; i++) {
            uint256 term1 = (2 * i + 1) * m[i - 1];
            uint256 term2 = (3 * i - 3) * m[i - 2];
            uint256 num = term1 + term2;
            m[i] = num / (i + 2);
        }

        return m[n];
    }

    // Compute next LFSR state (16-bit) with taps at bits 0, 2, 3, 5
    function lfsrNext(uint16 state) public pure returns (uint16) {
        uint16 bit = ((state >> 0) ^ (state >> 2) ^ (state >> 3) ^ (state >> 5)) & 1;
        state = (state >> 1) | (bit << 15);
        return state;
    }

    // Blum Blum Shub next value: xₙ₊₁ = xₙ² mod n
    function bbsNext(uint256 x, uint256 n) public pure returns (uint256) {
        return mulmod(x, x, n);
    }

    // First-Fit Decreasing bin packing algorithm
    function binPackingFFD(uint256[] memory sizes, uint256 cap) public pure returns (uint256) {
        // Sort sizes descending (simple bubble sort for Solidity)
        for (uint256 i = 0; i < sizes.length; i++) {
            for (uint256 j = i + 1; j < sizes.length; j++) {
                if (sizes[j] > sizes[i]) {
                    (sizes[i], sizes[j]) = (sizes[j], sizes[i]);
                }
            }
        }

        uint256[] memory bins = new uint256[](sizes.length); // at most one bin per item
        uint256 binCount = 0;

        for (uint256 i = 0; i < sizes.length; i++) {
            uint256 s = sizes[i];
            bool placed = false;
            for (uint256 j = 0; j < binCount; j++) {
                if (bins[j] + s <= cap) {
                    bins[j] += s;
                    placed = true;
                    break;
                }
            }
            if (!placed) {
                bins[binCount] = s;
                binCount++;
            }
        }

        return binCount;
    }


}

contract Recipes2 {

    // Young Tableaux count using hook-length formula
    function youngTableaux(uint256 r, uint256 c) public pure returns (uint256) {
        uint256 n = r * c;
        uint256 denom = 1;

        for (uint256 i = 0; i < r; i++) {
            for (uint256 j = 0; j < c; j++) {
                uint256 hook = (r - i) + (c - j) - 1;
                denom *= hook;
            }
        }

        return MathLib.fact(n) / denom;
    }

    function getAngle(uint256 i) internal pure returns (int32) {
        if (i == 0) return 51471;
        else if (i == 1) return 30385;
        else if (i == 2) return 16054;
        else if (i == 3) return 8153;
        else if (i == 4) return 4097;
        else if (i == 5) return 2049;
        else if (i == 6) return 1025;
        else if (i == 7) return 512;
        else if (i == 8) return 256;
        else if (i == 9) return 128;
        else if (i == 10) return 64;
        else if (i == 11) return 32;
        else if (i == 12) return 16;
        else if (i == 13) return 8;
        else if (i == 14) return 4;
        else if (i == 15) return 2;
        else revert("Invalid index");
    }

    // Computes (cos(angle), sin(angle)) in fixed-point using CORDIC
    // Input angle in scaled units, where 51471 ≈ 45 degrees ≈ π/4
    function cordic(int32 angle) public pure returns (int32 xOut, int32 yOut) {
        int32 x = 39797;
        int32 y = 0;
        int32 z = angle;

        for (uint8 i = 0; i < 16; i++) {
            int32 dx = x >> i;
            int32 dy = y >> i;
            if (z >= 0) {
                x -= dy;
                y += dx;
                z -= getAngle(i);
            } else {
                x += dy;
                y -= dx;
                z += getAngle(i);
            }
        }

        return (x, y); // (cos, sin) in fixed-point
    }

    // Linear Congruential Generator (LCG)
    struct Lcg {
        uint32 state;
        uint32 a;
        uint32 c;
    }

    function nextLcg(Lcg storage rng) internal {
        unchecked {
            rng.state = uint32(uint256(rng.state) * rng.a + rng.c);
        }
    }

    // Xor 
    struct XorShift64Star {
        uint64 state;
    }

    function newXorShift64Star(uint64 seed) internal pure returns (XorShift64Star memory) {
        require(seed != 0, "Seed must be non-zero");
        return XorShift64Star({ state: seed });
    }

    function nextU64(XorShift64Star storage rng) internal {
        unchecked {
            uint64 x = rng.state;
            x ^= x << 13;
            x ^= x >> 7;
            x ^= x << 17;
            rng.state = x;
            uint64 result = uint64(uint256(x) * 0x2545F4914F6CDD1D);
            rng.state = result; // update state again to simulate wrapping behavior
        }
    }

    // Euler's Totient Function    
    function totient(uint64 n) public pure returns (uint64) {
        uint64 result = n;

        if (n % 2 == 0) {
            result /= 2;
            while (n % 2 == 0) {
                n /= 2;
            }
        }

        uint64 p = 3;
        while (p * p <= n) {
            if (n % p == 0) {
                result = result / p * (p - 1);
                while (n % p == 0) {
                    n /= p;
                }
            }
            p += 2;
        }

        if (n > 1) {
            result = result / n * (n - 1);
        }

        return result;
    }

    function linearMu(uint256 n) public pure returns (int32[] memory) {
        bool[] memory isComp = new bool[](n + 1);
        int32[] memory mu = new int32[](n + 1);
        uint256[] memory primes = new uint256[](n + 1);
        uint256 primeCount = 0;

        mu[1] = 1;

        for (uint256 i = 2; i <= n; ++i) {
            if (!isComp[i]) {
                primes[primeCount++] = i;
                mu[i] = -1;
            }

            for (uint256 j = 0; j < primeCount; ++j) {
                uint256 p = primes[j];
                uint256 ip = i * p;
                if (ip > n) break;

                isComp[ip] = true;

                if (i % p == 0) {
                    mu[ip] = 0;
                    break;
                } else {
                    mu[ip] = -mu[i];
                }
            }
        }

        return mu;
    }

    // Sum of Divisors
    function sigma(uint256 n) public pure returns (uint256) {
        if (n == 0) return 0;

        uint256 result = 1;
        uint256 p = 2;

        while (p * p <= n) {
            if (n % p == 0) {
                uint256 sum = 1;
                uint256 power = 1;
                while (n % p == 0) {
                    n /= p;
                    power *= p;
                    sum += power;
                }
                result *= sum;
            }
            p += (p == 2) ? 1 : 2; // check 2, then only odd numbers
        }

        if (n > 1) {
            result *= (1 + n);
        }

        return result;
    }

    function divisorCount(uint256 n) public pure returns (uint256) {
        if (n == 0) return 0;

        uint256 count = 1;
        uint256 p = 2;

        while (p * p <= n) {
            if (n % p == 0) {
                uint256 exp = 0;
                while (n % p == 0) {
                    n /= p;
                    exp += 1;
                }
                count *= (exp + 1);
            }
            p += (p == 2) ? 1 : 2; // check 2, then odd numbers only
        }

        if (n > 1) {
            count *= 2; // n is a prime > sqrt(n)
        }

        return count;
    }

    function mobius(uint256 n) public pure returns (int256) {
        if (n == 0) return 0;

        int256 m = 1;
        uint256 p = 2;

        while (p * p <= n) {
            if (n % p == 0) {
                n /= p;
                if (n % p == 0) {
                    return 0; // square factor → μ(n) = 0
                }
                m = -m;
            }
            p += (p == 2) ? 1 : 2; // check 2, then only odd primes
        }

        if (n > 1) {
            m = -m;
        }

        return m;
    }

    function dirichletConvolution(uint256 n) public pure returns (uint256) {
        uint256 sum = 0;
        for (uint256 d = 1; d * d <= n; ++d) {
            if (n % d == 0) {
                uint256 d2 = n / d;
                if (d == d2) {
                    sum += f_d(d) * g_d(d2);
                } else {
                    sum += f_d(d) * g_d(d2) + f_d(d2) * g_d(d);
                }
            }
        }
        return sum;
    }

    // Example f(d) = 1
    function f_d(uint256 d) internal pure returns (uint256) {
        return 1*d; // TODO
    }

    // Example g(k) = k
    function g_d(uint256 k) internal pure returns (uint256) {
        return k;
    }

     // Computes coefficients a[n] such that P(x)/Q(x) = sum a[n] x^n
    function gfCoeff(int256[] memory P, int256[] memory Q, uint256 N) public pure returns (int256[] memory) {
        uint256 d = Q.length - 1;
        int256[] memory a = new int256[](N + 1);

        for (uint256 n = 0; n <= N; n++) {
            int256 sum = n < P.length ? P[n] : int256(0);
            for (uint256 i = 1; i <= d; i++) {
                if (n >= i) {
                    sum -= a[n - i] * Q[i];
                }
            }
            require(Q[0] != 0, "Q[0] cannot be zero");
            a[n] = sum / Q[0];
        }

        return a;
    }

    // Simulated Diffie-Hellman shared secret using pseudo-random inputs
    function diffieHellman(uint256 seed) public pure returns (uint256) {
        uint256 p = 23; // small fixed prime
        uint256 g = 5;  // fixed generator

        // TODO: replace keccak256 with PRG
        uint256 a = (uint256(keccak256(abi.encodePacked(seed, "a"))) % (p - 2)) + 1;
        uint256 b = (uint256(keccak256(abi.encodePacked(seed, "b"))) % (p - 2)) + 1;

        uint256 B = MathLib.modExp(g, b, p);       // B = g^b mod p
        return MathLib.modExp(B, a, p);            // shared secret = B^a mod p
    }

    // q-Analog: [n]_q = (q^n - 1) / (q - 1)
    function qAnalog(uint256 n, uint256 q) public pure returns (uint256) {
        if (q == 1) {
            return n;
        } else {
            uint256 qpow = 1;
            for (uint256 i = 0; i < n; i++) {
                qpow *= q;
            }
            return (qpow - 1) / (q - 1);
        }
    }

    // Brent’s Pollard Rho factorization
    function pollardRhoBrent(uint256 n, uint256 seed) public pure returns (uint256) {
        if (n < 2) return 0;

        uint256 y = uint256(keccak256(abi.encodePacked(seed, "y"))) % n;
        uint256 c = uint256(keccak256(abi.encodePacked(seed, "c"))) % n;
        uint256 m = (uint256(keccak256(abi.encodePacked(seed, "m"))) % (n - 1)) + 1;

        uint256 g = 1;
        uint256 r = 1;
        uint256 q = 1;

        uint256 x;
        uint256 ys;

        while (g == 1) {
            x = y;
            for (uint256 i = 0; i < r; i++) {
                y = addmod(mulmod(y, y, n), c, n);
            }

            uint256 k = 0;
            while (k < r && g == 1) {
                ys = y;
                for (uint256 i = 0; i < m && k < r; i++) {
                    y = addmod(mulmod(y, y, n), c, n);
                    uint256 diff = x > y ? x - y : y - x;
                    q = mulmod(q, diff, n);
                    k++;
                }
                g = MathLib.gcd(q, n);
            }

            r *= 2;
        }

        if (g == n) {
            while (true) {
                ys = addmod(mulmod(ys, ys, n), c, n);
                uint256 diff = x > ys ? x - ys : ys - x;
                g = MathLib.gcd(diff, n);
                if (g > 1) break;
            }
        }

        return g == n ? 0 : g; // 0 indicates failure
    }
    struct Pcg {
        uint64 state;
        uint64 inc;
    }

    function nextPcg(Pcg storage rng) internal returns (uint32) {
        unchecked {
            uint64 oldState = rng.state;
            rng.state = oldState * 6364136223846793005 + (rng.inc | 1);
            uint64 x = ((oldState >> 18) ^ oldState) >> 27;
            uint32 r = uint32(oldState >> 59);
            return rotateRight32(uint32(x), r);
        }
    }

    function rotateRight32(uint32 x, uint32 r) internal pure returns (uint32) {
        return (x >> r) | (x << (32 - r));
    }

    struct Mwc {
        uint64 state;
        uint64 carry;
    }

    function nextMwc(Mwc storage rng) internal returns (uint32) {
        unchecked {
            uint256 t = uint256(rng.state) * 4294957665 + rng.carry;
            rng.state = uint64(t & 0xFFFFFFFF);
            rng.carry = uint64(t >> 32);
            return uint32(rng.state);
        }
    }

    // CRC32
    function crc32(bytes memory data) public pure returns (uint32) {
        uint32 crc = 0xFFFFFFFF;
        for (uint i = 0; i < data.length; i++) {
            crc ^= uint32(uint8(data[i]));
            for (uint j = 0; j < 8; j++) {
                if ((crc & 1) != 0) {
                    crc = (crc >> 1) ^ 0xEDB88320;
                } else {
                    crc >>= 1;
                }
            }
        }
        return ~crc;
    }    

    // FNV-1a
    function fnv1a(bytes memory data) public pure returns (uint32) {
        uint32 hash = 2166136261;
        for (uint i = 0; i < data.length; i++) {
            hash ^= uint32(uint8(data[i]));
            hash = uint32(uint256(hash) * 16777619);
        }
        return hash;
    }

    //  Murmur3 Finalizer (32-bit)
    function murmur3Finalizer(uint32 h) public pure returns (uint32) {
        unchecked {
            h ^= h >> 16;
            h = uint32(uint256(h) * 0x85EBCA6B);
            h ^= h >> 13;
            h = uint32(uint256(h) * 0xC2B2AE35);
            h ^= h >> 16;
            return h;
        }
    }

    // Jenkins One-at-a-Time Hash
    function jenkins(bytes memory data) public pure returns (uint32) {
        uint32 hash = 0;
        for (uint i = 0; i < data.length; i++) {
            hash = uint32(uint256(hash) + uint8(data[i]));
            hash = uint32(uint256(hash) + (uint256(hash) << 10));
            hash ^= hash >> 6;
        }
        hash = uint32(uint256(hash) + (uint256(hash) << 3));
        hash ^= hash >> 11;
        hash = uint32(uint256(hash) + (uint256(hash) << 15));
        return hash;
    }
}