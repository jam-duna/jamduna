#![no_std]
#![no_main]
#![allow(non_snake_case)]

extern crate alloc;
use alloc::format;
use alloc::string::String;
use alloc::vec;
use alloc::vec::Vec;
use core::mem;
//use core::mem::transmute;
use core::primitive::u64;

const SIZE0: usize = 0x10000;
// allocate memory for stack
use polkavm_derive::min_stack_size;
min_stack_size!(SIZE0);

const SIZE1: usize = 0x10000;
// allocate memory for heap
use simplealloc::SimpleAlloc;
#[global_allocator]
static ALLOCATOR: SimpleAlloc<SIZE1> = SimpleAlloc::new();

use core::slice;
use core::sync::atomic::{AtomicU64, Ordering};
use utils::constants::FIRST_READABLE_ADDRESS;
use utils::functions::{call_log, log_info, log_error, parse_accumulate_args, parse_refine_args};
use utils::host_functions::gas;
use utils::effects::{ExecutionEffects, WriteIntent, WriteEffectEntry};
use utils::objects::ObjectRef;
use utils::hash_functions::blake2b_hash;
use evm_service::{EvmBlockPayload, deserialize_execution_effects, format_object_id};

#[cfg(target_arch = "riscv32")]
use polkavm_derive::sbrk as polkavm_sbrk;

#[cfg(not(target_arch = "riscv32"))]
fn polkavm_sbrk(_size: usize) {}

/// A simple xorshift64* PRNG
#[derive(Clone, Copy)]
pub struct XorShift64Star {
    state: u64,
}


impl XorShift64Star {
    pub const fn new(seed: u64) -> Self {
        // seed must be nonzero
        XorShift64Star { state: seed }
    }

    #[inline]
    pub fn next_u64(&mut self) -> u64 {
        let mut x = self.state;
        x ^= x << 13;
        x ^= x >> 7;
        x ^= x << 17;
        self.state = x;
        x.wrapping_mul(0x2545F4914F6CDD1D)
    }
}

// we’ll put the RNG state in an AtomicU64 to avoid `unsafe` in get_random_number
static RNG_STATE: AtomicU64 = AtomicU64::new(0x_1234_5678_9ABC_DEF1);

/// Initialize the global PRNG (call once at startup, with any nonzero seed)
pub fn seed_rng(seed: u64) {
    assert!(seed != 0);
    RNG_STATE.store(seed, Ordering::SeqCst);
}

/// Returns the next random `u64`
pub fn get_random_number() -> u64 {
    // load-and-update atomically
    let mut state = RNG_STATE.load(Ordering::SeqCst);
    // xorshift64* step
    state ^= state << 13;
    state ^= state >> 7;
    state ^= state << 17;
    let out = state.wrapping_mul(0x2545F4914F6CDD1D);
    RNG_STATE.store(state, Ordering::SeqCst);
    out
}

#[allow(dead_code)]
fn lcm(a: u64, b: u64) -> u64 {
    a / gcd(a, b) * b
}

// GCD
fn gcd(mut a: u64, mut b: u64) -> u64 {
    while b != 0 {
        let r = a % b;
        a = b;
        b = r;
    }
    a
}

// Extended GCD
fn extended_gcd(a: i64, b: i64) -> (i64, i64, i64) {
    if b == 0 {
        (a.abs(), a.signum(), 0)
    } else {
        let (g, x1, y1) = extended_gcd(b, a % b);
        (g, y1, x1 - (a / b) * y1)
    }
}

// 2. Extended GCD → modular inverse
fn mod_inv(a: i64, m: i64) -> Option<i64> {
    let (g, x, _) = extended_gcd(a, m);
    if g != 1 {
        None
    } else {
        Some((x % m + m) % m)
    }
}

// Integer √ (binary search)
fn integer_sqrt(n: u64) -> u64 {
    let mut lo = 0;
    let mut hi = n;
    while lo <= hi {
        let mid = (lo + hi) / 2;
        let sq = mid.saturating_mul(mid);
        if sq == n {
            return mid;
        } else if sq < n {
            lo = mid + 1;
        } else {
            hi = mid - 1;
        }
    }
    hi
}

// integer nth root
fn integer_nth_root(n: u64, k: u32) -> u64 {
    let mut lo = 0u64;
    let mut hi = n.min(1 << (64 / k)) + 1;
    while lo < hi {
        let mid = (lo + hi + 1) >> 1;
        let mut acc = 1u64;
        for _ in 0..k {
            acc = acc.saturating_mul(mid as u64);
            if acc > n as u64 {
                break;
            }
        }
        if acc <= n as u64 {
            lo = mid;
        } else {
            hi = mid - 1;
        }
    }
    lo
}

fn is_prime(n: u64) -> bool {
    if n < 2 {
        return false;
    }
    if n % 2 == 0 {
        return n == 2;
    }
    let mut i = 3;
    while i * i <= n {
        if n % i == 0 {
            return false;
        }
        i += 2;
    }
    true
}

// binomial for Narayana
fn binomial(n: u64, k: u64) -> u64 {
    let k = k.min(n - k);
    let mut res = 1u64;
    for i in 1..=k {
        res = res * (n + 1 - i) as u64 / i as u64;
    }
    res
}

fn jacobi(mut a: u64, mut n: u64) -> i64 {
    if n == 0 || n & 1 == 0 {
        return 0;
    }
    let mut t = 1;
    a %= n;
    while a != 0 {
        while a & 1 == 0 {
            a >>= 1;
            let r = n % 8;
            if r == 3 || r == 5 {
                t = -t;
            }
        }
        mem::swap(&mut a, &mut n);
        if a & n & 2 != 0 {
            t = -t;
        }
        a %= n;
    }
    if n == 1 {
        t
    } else {
        0
    }
}

#[allow(dead_code)]
fn dist2(a: (i64, i64), b: (i64, i64)) -> u64 {
    let dx = a.0 - b.0;
    let dy = a.1 - b.1;
    (dx * dx + dy * dy) as u64
}
// Next prime ≥ n
fn next_prime(mut n: u64) -> u64 {
    if n < 2 {
        return 2;
    }
    n += 1;
    while !is_prime(n) {
        n += 1;
    }
    n
}
/*
fn primes_up_to(n: usize) -> Vec<u64> {
    let mut sieve = vec![true; n+1];
    let mut p = Vec::new();
    for i in 2..=n {
        if sieve[i] {
            p.push(i as u64);
            let mut j = i*i;
            while j <= n {
                sieve[j] = false;
                j += i;
            }
        }
    }
    p
}
     */

// Prime factors
fn prime_factors(mut n: u64) -> Vec<u64> {
    let mut f = Vec::new();
    while n % 2 == 0 {
        f.push(2);
        n /= 2;
    }
    let mut p = 3;
    while p * p <= n {
        while n % p == 0 {
            f.push(p);
            n /= p;
        }
        p += 2;
    }
    if n > 1 {
        f.push(n);
    }
    f
}
// Primitive root mod p
fn primitive_root(p: u64) -> u64 {
    let phi = p - 1;
    let mut pf = prime_factors(phi);
    pf.sort();
    pf.dedup();
    'outer: for g in 2..p {
        for &q in &pf {
            if mod_exp(g, phi / q, p) == 1 {
                continue 'outer;
            }
        }
        return g;
    }
    0
}

// Primitive Root Test (for prime p)
/*
fn primitive_root_test(p: u64) -> Option<u64> {
    if p < 2 { return None; }
    let phi = p - 1;
    let mut facs = trial_division_wheel(phi);
    facs.sort(); facs.dedup();
    for g in 2..p {
        if facs.iter().all(|&q| mod_exp(g, phi / q, p) != 1) {
            return Some(g);
        }
    }
    None
}

 */
// 3. Modular exponentiation
fn mod_exp(mut base: u64, mut exp: u64, m: u64) -> u64 {
    let mut res = 1 % m;
    base %= m;
    while exp > 0 {
        if exp & 1 == 1 {
            res = res.wrapping_mul(base) % m;
        }
        base = base.wrapping_mul(base) % m;
        exp >>= 1;
    }
    res
}

// 4. CRT for two congruences
fn crt2(a1: i64, n1: i64, a2: i64, n2: i64) -> i64 {
    let inv = mod_inv(n1, n2).unwrap();
    let t = ((a2 - a1).rem_euclid(n2) * inv).rem_euclid(n2);
    a1 + n1 * t
}
// 5. Garner’s general CRT
fn garner(a: &[i64], n: &[i64]) -> i64 {
    let mut x = a[0];
    let mut prod = n[0];
    for i in 1..a.len() {
        let inv = mod_inv(prod.rem_euclid(n[i]), n[i]).unwrap();
        let t = ((a[i] - x).rem_euclid(n[i]) * inv).rem_euclid(n[i]);
        x += prod * t;
        prod *= n[i];
    }
    x
}

// 7. Floor log₂
//fn floor_log2(n: u64) -> u32 { 63 - n.leading_zeros() }

// 8. CLZ, CTZ, Popcount, Parity
#[allow(dead_code)]
fn clz(n: u64) -> u32 {
    n.leading_zeros()
}

#[allow(dead_code)]
fn ctz(n: u64) -> u32 {
    n.trailing_zeros()
}

#[allow(dead_code)]
fn popcount(n: u64) -> u32 {
    n.count_ones()
}
#[allow(dead_code)]
fn parity(n: u64) -> u32 {
    (n.count_ones() & 1) as u32
}
// 9. Bit reversal & Gray code
fn reverse_bits32(n: u32) -> u32 {
    n.reverse_bits()
}
fn gray_encode(n: u64) -> u64 {
    n ^ (n >> 1)
}
fn gray_decode(mut g: u64) -> u64 {
    let mut n = g;
    while g > 0 {
        g >>= 1;
        n ^= g;
    }
    n
}
// 10. Sieve & segmented sieve
fn sieve(n: usize) -> Vec<usize> {
    let mut is_prime = vec![true; n + 1];
    let mut p = Vec::new();
    for i in 2..=n {
        if is_prime[i] {
            p.push(i);
            let mut j = i * i;
            while j <= n {
                is_prime[j] = false;
                j += i;
            }
        }
    }
    p
}

fn is_prime_small(n: u64) -> bool {
    if n < 2 {
        return false;
    }
    if n % 2 == 0 {
        return n == 2;
    }
    let mut i = 3;
    while i * i <= n {
        if n % i == 0 {
            return false;
        }
        i += 2;
    }
    true
}
/*
fn pollards_rho(n: u64) -> u64 {
    if n % 2 == 0 {
        return 2;
    }

    let mut rng = thread_rng();
    let c: u64 = rng.gen_range(1..n);
    let mut x: u64 = rng.gen_range(2..n);
    let mut y = x;

    // annotate v as u64 (and return type u64 if you like)
    let f = |v: u64| -> u64 { (v.wrapping_mul(v).wrapping_add(c)) % n };

    let mut d = 1;
    while d == 1 {
        x = f(x);
        y = f(f(y));
        d = gcd((x as i64 - y as i64).abs() as u64, n);
        if d == n {
            // retry with a new random constant
            return pollards_rho(n);
        }
    }
    d
}

fn factor(n: u64) -> Vec<u64> {
    if n == 1 {
        return vec![];
    }
    if is_prime_small(n) {
        return vec![n];
    }
    let d = pollards_rho(n);
    let mut l = factor(d);
    let mut r = factor(n / d);
    l.append(&mut r);
    l.sort_unstable();
    l
}
*/

// 12. Fast Fibonacci (fast doubling)
fn fib(n: u64) -> (u64, u64) {
    if n == 0 {
        return (0, 1);
    }
    let (a, b) = fib(n >> 1);
    let c = a.wrapping_mul(b.wrapping_mul(2).wrapping_sub(a));
    let d = a.wrapping_mul(a).wrapping_add(b.wrapping_mul(b));
    if n & 1 == 0 {
        (c, d)
    } else {
        (d, c.wrapping_add(d))
    }
}
// 13–14. Factorial & binomial & Catalan
fn catalan(n: u64) -> u64 {
    (1..=n).fold(1u64, |c, i| c.wrapping_mul(4 * i as u64 - 2) / (i as u64 + 1))
}
// 15. Karatsuba 64×64→128
fn karatsuba(x: u64, y: u64) -> u128 {
    // mask and all intermediates promoted to u128
    let mask = (1u128 << 32) - 1;
    let x0 = (x as u128) & mask;
    let x1 = (x as u128) >> 32;
    let y0 = (y as u128) & mask;
    let y1 = (y as u128) >> 32;

    let z0 = x0 * y0;
    let z2 = x1 * y1;
    let z1 = (x0 + x1) * (y0 + y1) - z0 - z2;

    // now shifts are < 128, so no overflow
    (z2 << 64) | (z1 << 32) | z0
}

// 16–17. MP add/sub/mul naive (2‑limb)
fn mp_add(a: [u64; 2], b: [u64; 2]) -> [u64; 2] {
    let (r0, carry) = a[0].overflowing_add(b[0]);
    let (r1, _) = a[1].overflowing_add(b[1].wrapping_add(carry as u64));
    [r0, r1]
}
fn mp_sub(a: [u64; 2], b: [u64; 2]) -> [u64; 2] {
    let (r0, bor) = a[0].overflowing_sub(b[0]);
    let (r1, _) = a[1].overflowing_sub(b[1].wrapping_add(bor as u64));
    [r0, r1]
}
fn mp_mul_naive(a: [u64; 2], b: [u64; 2]) -> [u64; 4] {
    let mut res = [0u64; 4];

    for i in 0..2 {
        // carry must be big enough to hold the high half
        let mut carry: u128 = 0;

        for j in 0..2 {
            let idx = i + j;
            // promote everything to u128
            let t: u128 = (a[i] as u128) * (b[j] as u128) + (res[idx] as u128) + carry;

            // low 64 bits back into res
            res[idx] = t as u64;
            // high 64 bits become the new carry
            carry = t >> 64;
        }

        // leftover carry goes in the next limb
        res[i + 2] = carry as u64;
    }

    res
}

// 18–19 Montgomery & Barrett (32‑bit)
fn mont_mul32(a: u32, b: u32, m: u32, m_prime: u32) -> u32 {
    let t = a as u64 * b as u64;
    let m0 = (t as u32).wrapping_mul(m_prime);
    let tmp = t.wrapping_add((m0 as u64).wrapping_mul(m as u64));
    let u = (tmp >> 32) as u32;
    if u >= m {
        u - m
    } else {
        u
    }
}
fn mont_reduce32(t: u64, m: u32, m_prime: u32) -> u32 {
    let m0 = (t as u32).wrapping_mul(m_prime);
    let mut u = ((t + m0 as u64 * m as u64) >> 32) as u32;
    if u >= m {
        u -= m
    }
    u
}
fn barrett_reduce(x: u64, m: u32, mu: u64) -> u32 {
    // do the 64×64→128 multiplication in u128
    let prod: u128 = (x as u128) * (mu as u128);
    // now shifting right by 64 is fine on a 128‑bit value
    let q = (prod >> 64) as u64;

    // compute the remainder
    let mut r = x.wrapping_sub(q.wrapping_mul(m as u64));
    // one subtraction should suffice if mu was precomputed correctly
    if r >= m as u64 {
        r -= m as u64;
    }

    r as u32
}

// 20. NTT (mod 17, N=8)
const MOD_NTT: u64 = 17;
const NTT_N: usize = 8;
const ROOT: u64 = 3;
fn ntt(a: &[u64; NTT_N]) -> [u64; NTT_N] {
    let w = mod_exp(ROOT, (MOD_NTT - 1) / NTT_N as u64, MOD_NTT);
    let mut out = [0u64; NTT_N];
    for k in 0..NTT_N {
        let mut sum = 0;
        for j in 0..NTT_N {
            sum = (sum + a[j] * mod_exp(w, (j * k) as u64, MOD_NTT)) % MOD_NTT;
        }
        out[k] = sum;
    }
    out
}
// 21. CORDIC Q16
fn cordic(angle: i32) -> (i32, i32) {
    const ANG: [i32; 16] = [51471, 30385, 16054, 8153, 4097, 2049, 1025, 512, 256, 128, 64, 32, 16, 8, 4, 2];
    const K: i32 = 39797;
    let mut x = K;
    let mut y = 0;
    let mut z = angle;
    for i in 0..16 {
        let dx = x >> i;
        let dy = y >> i;
        if z >= 0 {
            x -= dy;
            y += dx;
            z -= ANG[i];
        } else {
            x += dy;
            y -= dx;
            z += ANG[i];
        }
    }
    (x, y)
}
// 22. Fixed-point Q16
fn fix_mul(a: i32, b: i32) -> i32 {
    ((a as i64 * b as i64) >> 16) as i32
}
fn fix_div(a: i32, b: i32) -> i32 {
    // avoid divide-by-zero
    if b == 0 {
        return 0;
    }
    (((a as i64) << 16) / (b as i64)) as i32
}
// 23. LCG
struct Lcg {
    state: u32,
    a: u32,
    c: u32,
}
impl Lcg {
    fn next(&mut self) -> u32 {
        self.state = self.state.wrapping_mul(self.a).wrapping_add(self.c);
        self.state
    }
}
// 24. Xorshift64
fn xorshift64(mut x: u64) -> u64 {
    x ^= x << 13;
    x ^= x >> 7;
    x ^= x << 17;
    x
}
// 25. PCG32
struct Pcg {
    state: u64,
    inc: u64,
}
impl Pcg {
    fn next(&mut self) -> u32 {
        let old = self.state;
        self.state = old.wrapping_mul(6364136223846793005).wrapping_add(self.inc | 1);
        let x = ((old >> 18) ^ old) >> 27;
        let r = (old >> 59) as u32;
        (x as u32).rotate_right(r)
    }
}
// 26. MWC
struct Mwc {
    state: u64,
    carry: u64,
}
impl Mwc {
    fn next(&mut self) -> u32 {
        let t = self.state.wrapping_mul(4294957665).wrapping_add(self.carry);
        self.state = t & 0xFFFF_FFFF;
        self.carry = t >> 32;
        self.state as u32
    }
}
// 27–29. CRC32, Adler-32, FNV-1a
fn crc32(data: &[u8]) -> u32 {
    let mut crc = 0xFFFF_FFFF;
    for &b in data {
        crc ^= b as u32;
        for _ in 0..8 {
            crc = if crc & 1 != 0 { (crc >> 1) ^ 0xEDB8_8320 } else { crc >> 1 };
        }
    }
    !crc
}

fn adler32(data: &[u8]) -> u32 {
    const MODAD: u32 = 65_521;
    let (mut a, mut b) = (1u32, 0u32);

    for &byte in data {
        let v = byte as u32; // promote u8 → u32
        a = (a + v) % MODAD;
        b = (b + a) % MODAD;
    }

    (b << 16) | a
}

fn fnv1a(data: &[u8]) -> u32 {
    let mut h = 2166136261;
    for &b in data {
        h ^= b as u32;
        h = h.wrapping_mul(16777619);
    }
    h
}

// ─── 41–50 ─────────────────────────────────────────────────────────────────

// 30. Murmur3 finalizer
fn murmur3_finalizer(mut h: u32) -> u32 {
    h ^= h >> 16;
    h = h.wrapping_mul(0x85EB_CA6B);
    h ^= h >> 13;
    h = h.wrapping_mul(0xC2B2_AE35);
    h ^= h >> 16;
    h
}
// 31. Jenkins
fn jenkins(data: &[u8]) -> u32 {
    let mut h = 0u32;
    for &b in data {
        h = h.wrapping_add(b as u32);
        h = h.wrapping_add(h << 10);
        h ^= h >> 6;
    }
    h = h.wrapping_add(h << 3);
    h ^= h >> 11;
    h = h.wrapping_add(h << 15);
    h
}
// 32–33. Bresenham line & circle
/*
fn bresenham_line(x0:i32,y0:i32,x1:i32,y1:i32)->Vec<(i32,i32)>{
    let (dx,dy)=((x1-x0).abs(),-(y1-y0).abs());
    let (sx,sy)=(if x0<x1{1}else{-1},if y0<y1{1}else{-1});
    let mut err=dx+dy; let (mut x,mut y)=(x0,y0);
    let mut pts=Vec::new();
    loop {
        pts.push((x,y));
        if x==x1 && y==y1 { break }
        let e2=2*err;
        if e2>=dy { err+=dy; x+=sx; }
        if e2<=dx { err+=dx; y+=sy; }
    }
    pts
}
fn bresenham_circle(cx:i32,cy:i32,r:i32)->Vec<(i32,i32)>{
    let(mut x,mut y)=(0,r);
    let mut d=3-2*r;
    let mut pts=Vec::new();
    while y>=x {
        for &(dx,dy) in &[
            ( x, y),( -x, y),( x, -y),( -x, -y),
            ( y, x),( -y, x),( y, -x),( -y, -x)
        ] {
            pts.push((cx+dx,cy+dy));
        }
        if d<=0 { d+=4*x+6; }
        else { d+=4*(x-y)+10; y-=1; }
        x+=1;
    }
    pts
}

// 34. DDA line
fn dda_line(x0:i32,y0:i32,x1:i32,y1:i32)->Vec<(i32,i32)>{
    let(dx,dy)=(x1-x0,y1-y0);
    let steps=dx.abs().max(dy.abs());
    (0..=steps).map(|i|{
        let x=x0+dx*i/steps; let y=y0+dy*i/steps;
        (x,y)
    }).collect()
}
// 35. Horner
fn horner(coeff:&[i64],x:i64)->i64{
    coeff.iter().rev().fold(0,|res,&c|res.wrapping_mul(x).wrapping_add(c))
}
// 36. Finite difference
fn finite_difference(seq:&[i64],ord:usize)->Vec<i64>{
    let mut d=seq.to_vec();
    for _ in 0..ord {
        if d.len()<2 {break}
        d=d.windows(2).map(|w|w[1]-w[0]).collect();
    }
    d
}

    // 37. Diophantine
fn solve_diophantine(a:i64,b:i64,c:i64)->Option<(i64,i64)>{
    let(g,x0,y0)=extended_gcd(a,b);
    if c%g!=0 {None} else { Some((x0*(c/g),y0*(c/g))) }
}

    // 38. Integer log10
fn integer_log10(mut n:u64)->u32{
    let mut l=0;
    while n>=10 { n/=10; l+=1; }
    l
}


    */

// 51. Euler’s Totient Function φ(n)
fn phi(mut n: u64) -> u64 {
    let mut result = n;
    if n % 2 == 0 {
        result = result / 2;
        while n % 2 == 0 {
            n /= 2;
        }
    }
    let mut p = 3;
    while p * p <= n {
        if n % p == 0 {
            result = result / p * (p - 1);
            while n % p == 0 {
                n /= p;
            }
        }
        p += 2;
    }
    if n > 1 {
        result = result / n * (n - 1);
    }
    result
}

// 52. Linear (Euler) sieve computing primes, φ and μ up to N
fn linear_sieve(n: usize) -> (Vec<usize>, Vec<u64>, Vec<i32>) {
    let mut is_comp = vec![false; n + 1];
    let mut primes = Vec::new();
    let mut phi = vec![0u64; n + 1];
    let mut mu = vec![0i32; n + 1];
    phi[1] = 1;
    mu[1] = 1;
    for i in 2..=n {
        if !is_comp[i] {
            primes.push(i);
            phi[i] = (i - 1) as u64;
            mu[i] = -1;
        }
        for &p in &primes {
            let ip = i * p;
            if ip > n {
                break;
            }
            is_comp[ip] = true;
            if i % p == 0 {
                phi[ip] = phi[i] * p as u64;
                mu[ip] = 0;
                break;
            } else {
                phi[ip] = phi[i] * (p as u64 - 1);
                mu[ip] = -mu[i];
            }
        }
    }
    (primes, phi, mu)
}

// 53. Sum‑of‑Divisors σ(n)
fn sigma(mut n: u64) -> u64 {
    let mut result: u64 = 1;
    let mut p = 2;
    while p * p <= n {
        if n % p == 0 {
            let mut sum = 1u64;
            let mut power = 1u64;
            while n % p == 0 {
                n /= p;
                power *= p as u64;
                sum += power;
            }
            result *= sum;
        }
        p += if p == 2 { 1 } else { 2 };
    }
    if n > 1 {
        result *= 1 + n as u64;
    }
    result as u64
}

// 54. Divisor‑Count Function d(n)
fn divisor_count(mut n: u64) -> u64 {
    let mut count = 1;
    let mut p = 2;
    while p * p <= n {
        if n % p == 0 {
            let mut exp = 0;
            while n % p == 0 {
                n /= p;
                exp += 1;
            }
            count *= exp + 1;
        }
        p += if p == 2 { 1 } else { 2 };
    }
    if n > 1 {
        count *= 2;
    }
    count
}

// 55. Möbius Function μ(n)
fn mobius(mut n: u64) -> i64 {
    let mut m = 1;
    let mut p = 2;
    while p * p <= n {
        if n % p == 0 {
            n /= p;
            if n % p == 0 {
                return 0;
            }
            m = -m;
        }
        p += if p == 2 { 1 } else { 2 };
    }
    if n > 1 {
        m = -m;
    }
    m
}

// 56. Linear Sieve for μ(n) only
fn linear_mu(n: usize) -> Vec<i32> {
    let mut is_comp = vec![false; n + 1];
    let mut primes = Vec::new();
    let mut mu = vec![0i32; n + 1];
    mu[1] = 1;
    for i in 2..=n {
        if !is_comp[i] {
            primes.push(i);
            mu[i] = -1;
        }
        for &p in &primes {
            let ip = i * p;
            if ip > n {
                break;
            }
            is_comp[ip] = true;
            if i % p == 0 {
                mu[ip] = 0;
                break;
            } else {
                mu[ip] = -mu[i];
            }
        }
    }
    mu
}

// 57. Dirichlet Convolution (f * g)(n)
fn dirichlet_convolution<F, G>(n: u64, f: F, g: G) -> u64
where
    F: Fn(u64) -> u64,
    G: Fn(u64) -> u64,
{
    let mut sum = 0;
    let mut d = 1;
    while d * d <= n {
        if n % d == 0 {
            let d2 = n / d;
            if d == d2 {
                sum += f(d) * g(d2);
            } else {
                sum += f(d) * g(d2) + f(d2) * g(d);
            }
        }
        d += 1;
    }
    sum
}
/*
// Prime Counting (Legendre’s formula)
fn phi_legendre(x: u64, s: usize, primes: &[u64]) -> u64 {
    if s == 0 { return x; }
    if s == 1 { return x - x/2; }
    phi_legendre(x, s-1, primes) - phi_legendre(x / primes[s-1], s-1, primes)
}
*/

// Prime Counting (Naïve trial division)
fn pi_trial(n: u64) -> u64 {
    let mut cnt = 0;
    for i in 2..=n {
        if is_prime_small(i) {
            cnt += 1;
        }
    }
    cnt
}

// 61. Legendre symbol (a|p) via Euler’s criterion
fn legendre_symbol(a: i64, p: i64) -> i32 {
    let ls = mod_exp(a.rem_euclid(p) as u64, ((p - 1) / 2) as u64, p as u64);
    if ls == 1 {
        1
    } else if ls == p as u64 - 1 {
        -1
    } else {
        0
    }
}

// 62. Tonelli–Shanks: sqrt(a) mod p, p an odd prime
fn tonelli_shanks(n: u64, p: u64) -> Option<u64> {
    if n == 0 {
        return Some(0);
    }
    if p == 2 {
        return Some(n);
    }
    // check solution exists
    if mod_exp(n, (p - 1) / 2, p) != 1 {
        return None;
    }
    // write p-1 = q * 2^s
    let mut q = p - 1;
    let mut s = 0;
    while q & 1 == 0 {
        q >>= 1;
        s += 1;
    }
    // find z a quadratic non-residue
    let mut z = 2;
    while mod_exp(z, (p - 1) / 2, p) != p - 1 {
        z += 1;
    }
    let mut m = s;
    let mut c = mod_exp(z, q, p);
    let mut t = mod_exp(n, q, p);
    let mut r = mod_exp(n, (q + 1) / 2, p);
    while t != 1 {
        // find least i (0 < i < m) such that t^(2^i) == 1
        let mut t2i = t;
        let mut i = 0;
        while t2i != 1 {
            t2i = (t2i * t2i) % p;
            i += 1;
            if i == m {
                return None;
            }
        }
        let b = mod_exp(c, 1 << (m - i - 1), p);
        m = i;
        c = (b * b) % p;
        t = (t * c) % p;
        r = (r * b) % p;
    }
    Some(r)
}

fn cipolla(n: u64, p: u64) -> Option<u64> {
    if n == 0 {
        return Some(0);
    }
    if mod_exp(n, (p - 1) / 2, p) != 1 {
        return None;
    }
    // find a such that w = a^2 - n is non-residue
    let mut a = 0;
    let mut w = 0;
    for x in 1..p {
        w = (x * x + p - n % p) % p;
        if mod_exp(w, (p - 1) / 2, p) == p - 1 {
            a = x;
            break;
        }
    }
    // define multiplication in F_p^2
    #[derive(Copy, Clone)]
    struct Complex {
        x: u64,
        y: u64,
        w: u64,
        p: u64,
    }
    impl Complex {
        fn mul(self, other: Complex) -> Complex {
            let x = (self.x * other.x % self.p + self.y * other.y % self.p * self.w % self.p) % self.p;
            let y = (self.x * other.y % self.p + self.y * other.x % self.p) % self.p;
            Complex {
                x,
                y,
                w: self.w,
                p: self.p,
            }
        }
        fn pow(mut self, mut exp: u64) -> Complex {
            let mut res = Complex {
                x: 1,
                y: 0,
                w: self.w,
                p: self.p,
            };
            while exp > 0 {
                if exp & 1 == 1 {
                    res = res.mul(self);
                }
                self = self.mul(self);
                exp >>= 1;
            }
            res
        }
    }
    let comp = Complex { x: a, y: 1, w, p };
    let res = comp.pow((p + 1) / 2);
    Some(res.x)
}

// Lucas–Lehmer test for Mersenne primes M_p = 2^p - 1
fn lucas_lehmer(p: u64) -> bool {
    if p == 2 {
        return true;
    }
    let m = (1u64 << p) - 1;
    let mut s = 4u64;
    for _ in 0..(p - 2) {
        // use wrapping_mul so debug builds won't panic
        s = s.wrapping_mul(s).wrapping_sub(2) % m;
    }
    s == 0
}

// Lucas sequence U_n, V_n mod m (naïve O(n))
fn lucas_sequence(n: u64, P: i64, Q: i64, m: i64) -> (i64, i64) {
    if n == 0 {
        return (0, 2 % m);
    }
    if n == 1 {
        return (1 % m, P % m);
    }
    let mut U0 = 0i64;
    let mut V0 = 2i64 % m;
    let mut U1 = 1i64 % m;
    let mut V1 = P % m;
    for _ in 2..=n {
        let U2 = (P * U1 - Q * U0).rem_euclid(m);
        let V2 = (P * V1 - Q * V0).rem_euclid(m);
        U0 = U1;
        V0 = V1;
        U1 = U2;
        V1 = V2;
    }
    (U1, V1)
}

// Strong Lucas probable prime test
fn is_strong_lucas_prp(n: u64) -> bool {
    if n < 2 {
        return false;
    }
    // find D,P,Q for Selfridge’s method
    let mut D = 5i64;
    let mut sign = 1;
    while legendre_symbol(D, n as i64) != -1 {
        D = -(D + 2 * sign);
        sign = -sign;
    }
    let P = 1i64;
    let Q = ((1 - D) / 4) as i64;
    // write n+1 = d*2^s
    let mut d = n + 1;
    let mut s = 0;
    while d & 1 == 0 {
        d >>= 1;
        s += 1;
    }
    // compute Lucas sequence
    let (mut U, mut V) = lucas_sequence(d, P, Q, n as i64);
    if U == 0 {
        return true;
    }
    for _ in 1..s {
        // U, V update for doubling
        let U2 = (U * V).rem_euclid(n as i64);
        let V2 = (V * V - 2 * mod_exp(Q.rem_euclid(n as i64) as u64, 1, n) as i64).rem_euclid(n as i64);
        U = U2;
        V = V2;
        if U == 0 {
            return true;
        }
    }
    false
}

// Baillie–PSW primality test
fn is_prime_miller(n: u64) -> bool {
    if n < 2 {
        return false;
    }
    for &a in &[2u64, 325, 9375, 28178, 450775, 9780504, 1795265022] {
        if a % n == 0 {
            return true;
        }
        let mut d = n - 1;
        let mut s = 0;
        while d & 1 == 0 {
            d >>= 1;
            s += 1;
        }
        let mut x = mod_exp(a, d, n);
        if x == 1 || x == n - 1 {
            continue;
        }
        let mut composite = true;
        for _ in 1..s {
            x = (x * x) % n;
            if x == n - 1 {
                composite = false;
                break;
            }
        }
        if composite {
            return false;
        }
    }
    true
}
fn baillie_psw(n: u64) -> bool {
    is_prime_miller(n) && is_strong_lucas_prp(n)
}

// Newton’s Integer Square Root
fn newton_sqrt(n: u64) -> u64 {
    if n < 2 {
        return n;
    }
    let mut x = n;
    loop {
        let y = (x + n / x) >> 1;
        if y >= x {
            return x;
        }
        x = y;
    }
}

// Bareiss Algorithm for 3×3 Determinant
/// Standard direct formula for a 3×3 determinant.
fn det3_direct(mat: [[i64; 3]; 3]) -> i64 {
    let a = mat[0][0];
    let b = mat[0][1];
    let c = mat[0][2];
    let d = mat[1][0];
    let e = mat[1][1];
    let f = mat[1][2];
    let g = mat[2][0];
    let h = mat[2][1];
    let i = mat[2][2];
    a * (e * i - f * h) - b * (d * i - f * g) + c * (d * h - e * g)
}

/// Bareiss algorithm with zero‑pivot guard
fn det_bareiss_3x3(mat: [[i64; 3]; 3]) -> i64 {
    // Copy into a mutable 2D array
    let mut m2 = mat;
    let mut denom = 1i64;

    for k in 0..2 {
        let pivot = m2[k][k];
        // If either pivot (at k>0) or denominator is zero, fall back
        if pivot == 0 || denom == 0 {
            return det3_direct(mat);
        }
        for i in (k + 1)..3 {
            for j in (k + 1)..3 {
                // this division is now safe: denom != 0
                m2[i][j] = (m2[i][j] * pivot - m2[i][k] * m2[k][j]) / denom;
            }
        }
        denom = pivot;
    }

    // After two elimination steps, m2[2][2] is the determinant
    m2[2][2]
}

// Smith Normal Form for 2×2
fn smith_normal_form_2x2(mat: [[i64; 2]; 2]) -> (i64, i64) {
    let a = mat[0][0];
    let b = mat[0][1];
    let c = mat[1][0];
    let d = mat[1][1];
    let mut g = gcd(a.abs() as u64, b.abs() as u64);
    g = gcd(g, c.abs() as u64);
    g = gcd(g, d.abs() as u64);
    let det = a * d - b * c;
    (g as i64, (det.abs() as u64 / g) as i64)
}

// Hermite Normal Form for 2×2
fn hermite_normal_form_2x2(mat: [[i64; 2]; 2]) -> [[i64; 2]; 2] {
    let m00 = mat[0][0];
    let m10 = mat[1][0];
    let m01 = mat[0][1];
    let m11 = mat[1][1];

    // If the entire first column is zero, g == 0 and we can't divide by it
    if m00 == 0 && m10 == 0 {
        // HNF of a matrix whose first column is zero:
        // you can choose to return `mat` unchanged, or
        // canonicalize second column into the diagonal:
        return [[0, m01], [0, m11]];
    }

    // Now gcd(m00, m10) is nonzero
    let (g, s, t) = extended_gcd(m00, m10);
    // g > 0 by construction, so these divides are safe
    let u = m00 / g;
    let v = m10 / g;
    let a = g;
    let b = ((s * m01 + t * m11) % a + a) % a;
    let d = -v * m01 + u * m11;
    [[a, b], [0, d]]
}

// LLL Reduction in 2D
fn lll_reduce_2d(mut b1: (i64, i64), mut b2: (i64, i64)) -> ((i64, i64), (i64, i64)) {
    loop {
        let dot = b2.0 * b1.0 + b2.1 * b1.1;
        let norm = b1.0 * b1.0 + b1.1 * b1.1;
        if norm == 0 {
            break;
        }
        let mu = ((2 * dot + norm) / (2 * norm)) as i64;
        let nb2 = (b2.0 - mu * b1.0, b2.1 - mu * b1.1);
        if nb2.0 * nb2.0 + nb2.1 * nb2.1 < norm {
            b2 = b1;
            b1 = nb2;
        } else {
            b2 = nb2;
            break;
        }
    }
    (b1, b2)
}

// Binary Long Division (u64 ÷ u64)
fn long_div(dividend: u64, divisor: u64) -> (u64, u64) {
    let mut q = 0u64;
    let mut r = 0u64;
    for i in (0..64).rev() {
        r = (r << 1) | ((dividend >> i) & 1);
        if r >= divisor {
            r -= divisor;
            q |= 1 << i;
        }
    }
    (q, r)
}

// Barrett Division for u64
fn barrett_div(n: u64, d: u64) -> u64 {
    // Compute mu = floor(2^64 / d) in u128
    let mu: u128 = (1u128 << 64) / (d as u128);

    // Multiply n·mu as u128, then shift down by 64 to approximate n/d
    let mut q = ((n as u128 * mu) >> 64) as u64;

    // Clamp to [0, d-1]
    if q >= d {
        q = d - 1;
    }

    q
}

// Sliding‑Window Modular Exponentiation (w=4)
fn mod_exp_sliding(base: u64, exp: u64, m: u64) -> u64 {
    let w = 4;
    let size = 1 << w;
    let mut pre = vec![0u64; size];
    pre[0] = 1 % m;
    pre[1] = base % m;
    for i in 2..size {
        pre[i] = (pre[i - 1] * pre[1]) % m;
    }
    let mut result = 1u64 % m;
    let mut bits = Vec::new();
    let mut e = exp;
    while e > 0 {
        bits.push((e & 1) as u8);
        e >>= 1;
    }
    bits.reverse();
    let mut i = 0;
    while i < bits.len() {
        if bits[i] == 0 {
            result = (result * result) % m;
            i += 1;
        } else {
            let mut l = 1;
            let maxl = (bits.len() - i).min(w);
            for ll in 2..=maxl {
                let mut val = 0;
                for j in 0..ll {
                    val = (val << 1) | bits[i + j] as usize;
                }
                if val & 1 == 0 {
                    break;
                }
                l = ll;
            }
            let mut window_val = 0;
            for j in 0..l {
                window_val = (window_val << 1) | bits[i + j] as usize;
            }
            for _ in 0..l {
                result = (result * result) % m;
            }
            result = (result * pre[window_val]) % m;
            i += l;
        }
    }
    result
}

// Montgomery Ladder Exponentiation
fn mod_exp_ladder(base: u64, exp: u64, m: u64) -> u64 {
    let mut r0 = 1 % m;
    let mut r1 = base % m;
    for i in (0..64).rev() {
        if ((exp >> i) & 1) == 0 {
            r1 = (r0 * r1) % m;
            r0 = (r0 * r0) % m;
        } else {
            r0 = (r0 * r1) % m;
            r1 = (r1 * r1) % m;
        }
    }
    r0
}

// Toom‑Cook 3‑Way Multiplication for 64‑bit
fn toom3_64(x: u64, y: u64) -> u64 {
    // use i128 everywhere so that 21*4=84‑bit shifts are legal
    let B: i128 = 1 << 21;
    let mask: i128 = B - 1;

    let x = x as i128;
    let y = y as i128;

    let a0 = x & mask;
    let a1 = (x >> 21) & mask;
    let a2 = x >> 42;
    let b0 = y & mask;
    let b1 = (y >> 21) & mask;
    let b2 = y >> 42;

    // point‑evaluations
    let p0 = a0 * b0;
    let p1 = (a0 + a1 + a2) * (b0 + b1 + b2);
    let pm1 = (a0 - a1 + a2) * (b0 - b1 + b2);
    let p2 = (a0 + 2 * a1 + 4 * a2) * (b0 + 2 * b1 + 4 * b2);
    let pinf = a2 * b2;

    // interpolation
    let r0 = p0;
    let r4 = pinf;
    let u1 = p1 - r0 - r4;
    let u2 = pm1 - r0 - r4;
    let r2 = (u1 + u2) / 2;
    let r1 = u1 - r2;
    let r3 = (p2 - r0 - 2 * r1 - 4 * r2 - 16 * r4) / 8;

    // recombine via shifts instead of overflowing multiplies:
    //
    //   r0
    // + r1 *  B      == r1 << 21
    // + r2 *  B^2    == r2 << (21*2)
    // + r3 *  B^3    == r3 << (21*3)
    // + r4 *  B^4    == r4 << (21*4)
    //
    let res = r0 + (r1 << 21) + (r2 << 42) + (r3 << 63) + (r4 << 84);

    // drop back to u64 (low 64 bits)
    res as u64
}

// Fast Walsh–Hadamard Transform (length=8)
fn fwht(a: &mut [u64; 8]) {
    let mut len = 1;
    while len < 8 {
        for i in (0..8).step_by(len * 2) {
            for j in 0..len {
                let u = a[i + j];
                let v = a[i + j + len];
                a[i + j] = u.wrapping_add(v);
                a[i + j + len] = u.wrapping_sub(v);
            }
        }
        len <<= 1;
    }
}

// Next lexicographic permutation
fn next_lexicographic_permutation(v: &mut [u64]) -> bool {
    let n = v.len();
    if n < 2 {
        return false;
    }
    let mut i = n - 1;
    while i > 0 && v[i - 1] >= v[i] {
        i -= 1;
    }
    if i == 0 {
        return false;
    }
    let mut j = n - 1;
    while v[j] <= v[i - 1] {
        j -= 1;
    }
    v.swap(i - 1, j);
    v[i..].reverse();
    true
}

// Next combination (k of n)
fn next_combination(comb: &mut [usize], n: usize) -> bool {
    let k = comb.len();
    for i in (0..k).rev() {
        if comb[i] < n - k + i {
            comb[i] += 1;
            for j in i + 1..k {
                comb[j] = comb[j - 1] + 1;
            }
            return true;
        }
    }
    false
}

// Permutation ranking & unranking (n≤10)
fn factorial(n: u64) -> u64 {
    (1..=n).product()
}

fn perm_rank(v: &[u64]) -> u64 {
    let n = v.len();
    let mut rank = 0;
    let mut used = vec![false; n];
    for i in 0..n {
        let mut smaller = 0;
        for x in 0..(v[i] as usize) {
            if !used[x] {
                smaller += 1;
            }
        }
        rank += smaller as u64 * factorial((n - 1 - i) as u64);
        used[v[i] as usize] = true;
    }
    rank
}

fn perm_unrank(mut rank: u64, n: usize) -> Vec<u64> {
    let mut elems: Vec<u64> = (0..n as u64).collect();
    let mut res = Vec::with_capacity(n);
    for i in (0..n).rev() {
        let f = factorial(i as u64);
        let idx = (rank / f) as usize;
        rank %= f;
        res.push(elems.remove(idx));
    }
    res
}

// Combination ranking & unranking (n≤30,k≤15)
fn comb(n: usize, k: usize) -> u64 {
    let mut c = 1;
    for i in 1..=k {
        c = c * (n + 1 - i) as u64 / i as u64;
    }
    c
}
fn comb_rank(cmb: &[usize], n: usize) -> u64 {
    let k = cmb.len();
    let mut rank = 0;
    for i in 0..k {
        let start = if i == 0 { 0 } else { cmb[i - 1] + 1 };
        for j in start..cmb[i] {
            rank += comb(n - j - 1, k - i - 1);
        }
    }
    rank
}
fn comb_unrank(mut rank: u64, n: usize, k: usize) -> Vec<usize> {
    let mut res = Vec::with_capacity(k);
    let mut x = 0;
    for i in 0..k {
        for j in x..n {
            let c = comb(n - j - 1, k - i - 1);
            if rank < c {
                res.push(j);
                x = j + 1;
                break;
            }
            rank -= c;
        }
    }
    res
}

// Partition count p(n) via pentagonal theorem
fn partition_count(n: usize) -> u64 {
    let mut p = vec![0; n + 1];
    p[0] = 1;
    for i in 1..=n {
        let mut k = 1;
        while {
            let g1 = k * (3 * k - 1) / 2;
            if g1 > i {
                false
            } else {
                let sign = if k % 2 == 0 { -1 } else { 1 };
                p[i] = (p[i] as i64 + sign * (p[i - g1] as i64)) as u64;
                true
            }
        } {
            k += 1;
        }
    }
    p[n]
}

// Enumerate partitions of n (simple recursive)
fn enum_partitions(n: u64) -> Vec<Vec<u64>> {
    fn helper(n: u64, max: u64, cur: &mut Vec<u64>, out: &mut Vec<Vec<u64>>) {
        if n == 0 {
            out.push(cur.clone());
            return;
        }
        // start k = min(max, n) so that n - k >= 0
        let mut k = max.min(n);
        while k > 0 {
            cur.push(k);
            helper(n - k, k, cur, out);
            cur.pop();
            k -= 1;
        }
    }
    let mut out = Vec::new();
    helper(n, n, &mut Vec::new(), &mut out);
    out
}

// Coin change: count ways (coins 1,5,10,25)
fn coin_change_count(n: usize) -> u64 {
    let coins = [1, 5, 10, 25];
    let mut dp = vec![0; n + 1];
    dp[0] = 1;
    for &c in &coins {
        for i in c..=n {
            dp[i] += dp[i - c];
        }
    }
    dp[n]
}

// Coin change: min coins
fn coin_change_min(n: usize) -> i64 {
    let coins = [1, 5, 10, 25];
    let mut dp = vec![i64::MAX; n + 1];
    dp[0] = 0;
    for &c in &coins {
        for i in c..=n {
            if dp[i - c] != i64::MAX {
                dp[i] = dp[i].min(dp[i - c] + 1);
            }
        }
    }
    if dp[n] == i64::MAX {
        -1
    } else {
        dp[n]
    }
}

// 0/1 Knapsack (n items)
fn knapsack(weights: &[usize], values: &[u64], cap: usize) -> u64 {
    let mut dp = vec![0; cap + 1];
    for i in 0..weights.len() {
        for w in (weights[i]..=cap).rev() {
            dp[w] = dp[w].max(dp[w - weights[i]] + values[i]);
        }
    }
    dp[cap]
}

// 91. Unbounded Knapsack
fn unbounded_knapsack(weights: &[usize], values: &[u64], cap: usize) -> u64 {
    let mut dp = vec![0u64; cap + 1];
    for w in 0..=cap {
        for i in 0..weights.len() {
            if weights[i] <= w {
                dp[w] = dp[w].max(dp[w - weights[i]] + values[i]);
            }
        }
    }
    dp[cap]
}

// 92. Longest Common Subsequence (length only)
fn lcs(a: &[u8], b: &[u8]) -> usize {
    let (m, n) = (a.len(), b.len());
    let mut dp = vec![vec![0usize; n + 1]; m + 1];
    for i in 1..=m {
        for j in 1..=n {
            dp[i][j] = if a[i - 1] == b[j - 1] {
                dp[i - 1][j - 1] + 1
            } else {
                dp[i - 1][j].max(dp[i][j - 1])
            };
        }
    }
    dp[m][n]
}

// 93. Longest Increasing Subsequence (O(n²))
fn lis_length(seq: &[u64]) -> usize {
    let n = seq.len();
    let mut dp = vec![1usize; n];
    let mut best = 0;
    for i in 0..n {
        for j in 0..i {
            if seq[j] < seq[i] {
                dp[i] = dp[i].max(dp[j] + 1);
            }
        }
        best = best.max(dp[i]);
    }
    best
}

// 94. Levenshtein Edit Distance
fn levenshtein(a: &[u8], b: &[u8]) -> usize {
    let (m, n) = (a.len(), b.len());
    let mut dp = vec![vec![0usize; n + 1]; m + 1];
    for i in 0..=m {
        dp[i][0] = i;
    }
    for j in 0..=n {
        dp[0][j] = j;
    }
    for i in 1..=m {
        for j in 1..=n {
            let cost = if a[i - 1] == b[j - 1] { 0 } else { 1 };
            dp[i][j] = (dp[i - 1][j] + 1).min(dp[i][j - 1] + 1).min(dp[i - 1][j - 1] + cost);
        }
    }
    dp[m][n]
}

// 95. Damerau–Levenshtein Distance (with adjacent transpositions)
fn damerau_levenshtein(a: &[u8], b: &[u8]) -> usize {
    let (m, n) = (a.len(), b.len());
    let mut dp = vec![vec![0usize; n + 1]; m + 1];
    for i in 0..=m {
        dp[i][0] = i;
    }
    for j in 0..=n {
        dp[0][j] = j;
    }
    for i in 1..=m {
        for j in 1..=n {
            let cost = if a[i - 1] == b[j - 1] { 0 } else { 1 };
            dp[i][j] = (dp[i - 1][j] + 1).min(dp[i][j - 1] + 1).min(dp[i - 1][j - 1] + cost);
            if i > 1 && j > 1 && a[i - 1] == b[j - 2] && a[i - 2] == b[j - 1] {
                dp[i][j] = dp[i][j].min(dp[i - 2][j - 2] + 1);
            }
        }
    }
    dp[m][n]
}

// 96. Matrix Chain Multiplication (min scalar multiplications)
fn matrix_chain(dims: &[usize]) -> usize {
    let n = dims.len() - 1;
    let mut dp = vec![vec![0usize; n]; n];
    for l in 2..=n {
        for i in 0..=n - l {
            let j = i + l - 1;
            dp[i][j] = usize::MAX;
            for k in i..j {
                let cost = dp[i][k] + dp[k + 1][j] + dims[i] * dims[k + 1] * dims[j + 1];
                dp[i][j] = dp[i][j].min(cost);
            }
        }
    }
    dp[0][n - 1]
}

// 97. Optimal Binary Search Tree (given key frequencies)
fn optimal_bst(freq: &[u64]) -> u64 {
    let n = freq.len();
    let mut dp = vec![vec![0u64; n]; n];
    let mut sum = vec![0u64; n + 1];
    for i in 1..=n {
        sum[i] = sum[i - 1] + freq[i - 1];
    }
    for l in 1..=n {
        for i in 0..=n - l {
            let j = i + l - 1;
            dp[i][j] = u64::MAX;
            let w = sum[j + 1] - sum[i];
            for r in i..=j {
                let c = w + (if r > i { dp[i][r - 1] } else { 0 }) + (if r < j { dp[r + 1][j] } else { 0 });
                dp[i][j] = dp[i][j].min(c);
            }
        }
    }
    dp[0][n - 1]
}

// 98. Dynamic Time Warping (absolute difference cost)
fn dtw(a: &[u64], b: &[u64]) -> u64 {
    let m = a.len();
    let n = b.len();
    let inf = u64::MAX / 4;
    let mut dp = vec![vec![inf; n + 1]; m + 1];
    dp[0][0] = 0;
    for i in 1..=m {
        dp[i][0] = inf;
    }
    for j in 1..=n {
        dp[0][j] = inf;
    }
    for i in 1..=m {
        for j in 1..=n {
            let cost = if a[i - 1] > b[j - 1] {
                a[i - 1] - b[j - 1]
            } else {
                b[j - 1] - a[i - 1]
            };
            dp[i][j] = cost + dp[i - 1][j].min(dp[i][j - 1]).min(dp[i - 1][j - 1]);
        }
    }
    dp[m][n]
}

// 99. Needleman–Wunsch Global Alignment (match=2, mismatch=−1, gap=−1)
fn needleman_wunsch(a: &[u8], b: &[u8]) -> i64 {
    let (m, n) = (a.len(), b.len());
    let mut dp = vec![vec![0i64; n + 1]; m + 1];
    let gap = -1;
    for i in 1..=m {
        dp[i][0] = dp[i - 1][0] + gap;
    }
    for j in 1..=n {
        dp[0][j] = dp[0][j - 1] + gap;
    }
    for i in 1..=m {
        for j in 1..=n {
            let score = if a[i - 1] == b[j - 1] { 2 } else { -1 };
            dp[i][j] = dp[i - 1][j - 1] + score;
            dp[i][j] = dp[i][j].max(dp[i - 1][j] + gap);
            dp[i][j] = dp[i][j].max(dp[i][j - 1] + gap);
        }
    }
    dp[m][n]
}

// 100. Smith–Waterman Local Alignment (match=2, mismatch=−1, gap=−1)
fn smith_waterman(a: &[u8], b: &[u8]) -> i64 {
    let (m, n) = (a.len(), b.len());
    let mut dp = vec![vec![0i64; n + 1]; m + 1];
    let mut best = 0;
    let gap = -1;
    for i in 1..=m {
        for j in 1..=n {
            let score = if a[i - 1] == b[j - 1] { 2 } else { -1 };
            dp[i][j] = (dp[i - 1][j - 1] + score).max(dp[i - 1][j] + gap).max(dp[i][j - 1] + gap).max(0);
            best = best.max(dp[i][j]);
        }
    }
    best
}

// 1. Stein’s Binary GCD Algorithm
fn stein_gcd(mut a: u64, mut b: u64) -> u64 {
    if a == 0 {
        return b;
    }
    if b == 0 {
        return a;
    }
    let shift = (a | b).trailing_zeros();
    a >>= a.trailing_zeros();
    loop {
        b >>= b.trailing_zeros();
        if a > b {
            mem::swap(&mut a, &mut b);
        }
        b -= a;
        if b == 0 {
            break;
        }
    }
    a << shift
}

// 2. GCD via only subtraction (no division/shifts)
fn sub_gcd(mut a: u64, mut b: u64) -> u64 {
    if a == 0 {
        return b;
    }
    if b == 0 {
        return a;
    }
    while a != b {
        if a > b {
            a -= b;
        } else {
            b -= a;
        }
    }
    a
}

// Binary‑Search Integer Division: ⌊a/b⌋
fn binary_div(a: u64, b: u64) -> u64 {
    if b == 0 {
        return 0;
    }

    let mut low: u64 = 0;
    let mut high: u64 = 1;

    while high.saturating_mul(b) <= a {
        high <<= 1;
    }
    while low < high {
        let mid = (low + high + 1) >> 1;
        if mid.saturating_mul(b) <= a {
            low = mid;
        } else {
            high = mid - 1;
        }
    }
    low
}

// 4. Integer Log via repeated multiplication: ⌊log_b(n)⌋
fn integer_log_mul(n: u64, b: u64) -> u32 {
    if b < 2 {
        return 0;
    }
    let mut p = b;
    let mut k = 0;
    while p <= n {
        p = p.saturating_mul(b);
        k += 1;
    }
    k
}

// 5. Integer Log via repeated division: ⌊log_b(n)⌋
fn integer_log_div(mut n: u64, b: u64) -> u32 {
    if b < 2 {
        return 0;
    }
    let mut k = 0;
    while n >= b {
        n /= b;
        k += 1;
    }
    k
}

// 6. Perfect Square Test
fn is_perfect_square(n: u64) -> bool {
    let mut lo = 0u64;
    let mut hi = (n >> 1).saturating_add(1);
    while lo <= hi {
        let mid = (lo + hi) >> 1;
        let sq = mid.saturating_mul(mid);
        if sq == n {
            return true;
        } else if sq < n {
            lo = mid + 1;
        } else {
            hi = mid - 1;
        }
    }
    false
}

// 7. Perfect Power Test: n = a^k?
fn perfect_power(n: u64) -> Option<(u64, u32)> {
    if n < 2 {
        return None;
    }
    let max_k = 64 - n.leading_zeros();
    for k in 2..=max_k {
        let a = integer_nth_root(n, k);
        if a > 1 && a.pow(k) == n {
            return Some((a, k));
        }
    }
    None
}

// 8. Stern–Brocot Tree Navigation: path to num/den
fn stern_brocot_path(num: u64, den: u64) -> String {
    let mut path = String::new();
    let (mut ln, mut ld) = (0u64, 1u64);
    let (mut rn, mut rd) = (1u64, 0u64);
    while ln + rn != num || ld + rd != den {
        let mn = ln + rn;
        let md = ld + rd;
        if num * md > mn * den {
            // go right
            path.push('R');
            ln = mn;
            ld = md;
        } else {
            // go left
            path.push('L');
            rn = mn;
            rd = md;
        }
    }
    path
}

// 9. Continued Fraction Convergents
fn continued_fraction_convergents(mut num: u64, mut den: u64) -> Vec<(u64, u64)> {
    let mut coeffs = Vec::new();
    while den != 0 {
        let a = num / den;
        coeffs.push(a);
        let r = num % den;
        num = den;
        den = r;
    }
    let mut conv = Vec::new();
    let mut p0 = 1u64;
    let mut p1 = coeffs[0] as u64;
    let mut q0 = 0u64;
    let mut q1 = 1u64;
    conv.push((p1 as u64, q1 as u64));
    for &a in &coeffs[1..] {
        let p2 = a as u64 * p1 + p0;
        let q2 = a as u64 * q1 + q0;
        conv.push((p2 as u64, q2 as u64));
        p0 = p1;
        p1 = p2;
        q0 = q1;
        q1 = q2;
    }
    conv
}

// 10. Farey Sequence Generation (den ≤ n)
fn farey_sequence(n: u64) -> Vec<(u64, u64)> {
    let mut seq = vec![(0, 1), (1, n)];
    let mut a = 0u64;
    let mut b = 1u64;
    let mut c = 1u64;
    let mut d = n;
    while c <= n {
        let k = (n + b) / d;
        let e = k * c - a;
        let f = k * d - b;
        seq.push((c, d));
        a = c;
        b = d;
        c = e;
        d = f;
    }
    seq
}

// 11. Lucas Numbers: L₀=2, L₁=1, Lₙ=Lₙ₋₁+Lₙ₋₂
fn lucas(n: u32) -> u64 {
    match n {
        0 => 2,
        1 => 1,
        _ => {
            let mut a = 2u64;
            let mut b = 1u64;
            for _ in 2..=n {
                let c = a + b;
                a = b;
                b = c;
            }
            b
        }
    }
}

// 12. Tribonacci Numbers: T₀=0, T₁=1, T₂=1, Tₙ=Tₙ₋₁+Tₙ₋₂+Tₙ₋₃
fn tribonacci(n: u32) -> u64 {
    match n {
        0 => 0,
        1 | 2 => 1,
        _ => {
            let mut a = 0u64;
            let mut b = 1u64;
            let mut c = 1u64;
            for _ in 3..=n {
                let d = a + b + c;
                a = b;
                b = c;
                c = d;
            }
            c
        }
    }
}

// 13. Pell Numbers: P₀=0, P₁=1, Pₙ=2·Pₙ₋₁+Pₙ₋₂
fn pell(n: u32) -> u64 {
    match n {
        0 => 0,
        1 => 1,
        _ => {
            let mut a = 0u64;
            let mut b = 1u64;
            for _ in 2..=n {
                let c = 2 * b + a;
                a = b;
                b = c;
            }
            b
        }
    }
}

// 14. Stirling Numbers of the First Kind s(n,k)
fn stirling1(n: usize, k: usize) -> i64 {
    if k > n {
        return 0;
    }
    let mut dp = vec![vec![0i64; k + 1]; n + 1];
    dp[0][0] = 1;
    for i in 1..=n {
        let m = i.min(k);
        for j in 1..=m {
            dp[i][j] = dp[i - 1][j - 1] - (i as i64 - 1) * dp[i - 1][j];
        }
    }
    dp[n][k]
}

// 15. Stirling Numbers of the Second Kind S(n,k)
fn stirling2(n: usize, k: usize) -> u64 {
    if k > n {
        return 0;
    }
    let mut dp = vec![vec![0u64; k + 1]; n + 1];
    dp[0][0] = 1;
    for i in 1..=n {
        let m = i.min(k);
        for j in 1..=m {
            dp[i][j] = j as u64 * dp[i - 1][j] + dp[i - 1][j - 1];
        }
    }
    dp[n][k]
}

// 16. Bell Numbers via Bell triangle
fn bell(n: usize) -> u64 {
    let mut bell = vec![vec![0u64; n + 1]; n + 1];
    bell[0][0] = 1;
    for i in 1..=n {
        bell[i][0] = bell[i - 1][i - 1];
        for j in 1..=i {
            bell[i][j] = bell[i][j - 1] + bell[i - 1][j - 1];
        }
    }
    bell[n][0]
}

// 17. Derangement Count !n, D(0)=1, D(1)=0, D(n)=(n-1)(D(n-1)+D(n-2))
fn derangement(n: u32) -> u64 {
    match n {
        0 => 1,
        1 => 0,
        _ => {
            let mut d0 = 1u64;
            let mut d1 = 0u64;
            for i in 2..=n {
                let d = (i as u64 - 1) * (d1 + d0);
                d0 = d1;
                d1 = d;
            }
            d1
        }
    }
}

// 18. Eulerian Numbers A(n,k): A(0,0)=1; A(n,k)=(n-k)*A(n-1,k-1)+(k+1)*A(n-1,k)
fn eulerian(n: usize, k: usize) -> u64 {
    if k > n {
        return 0;
    }
    let mut dp = vec![vec![0u64; n]; n + 1];
    dp[0][0] = 1;
    for i in 1..=n {
        for j in 0..i {
            let a = if j > 0 { (i - j) as u64 * dp[i - 1][j - 1] } else { 0 };
            let b = if j < i - 1 { (j + 1) as u64 * dp[i - 1][j] } else { 0 };
            dp[i][j] = a + b;
        }
    }
    dp[n][k]
}

// 25. Eulerian Path/Circuit Check (undirected)
fn eulerian_path_circuit(adj: &Vec<Vec<bool>>) -> (bool, bool) {
    let n = adj.len();
    let mut odd = 0;
    for i in 0..n {
        let deg = adj[i].iter().filter(|&&b| b).count();
        if deg % 2 == 1 {
            odd += 1;
        }
    }
    let circuit = odd == 0;
    let path = odd == 0 || odd == 2;
    (circuit, path)
}

// 19. Narayana Numbers N(n,k) = (1/n)·C(n,k)·C(n,k-1)
fn narayana(n: u64, k: u64) -> u64 {
    if k < 1 || k > n {
        return 0;
    }
    let c1 = binomial(n, k);
    let c2 = binomial(n, k - 1);
    (c1 * c2 / n as u64) as u64
}

// 20. Motzkin Numbers via recurrence: M₀=1, M₁=1, Mₙ=((2n+1)Mₙ₋₁+(3n-3)Mₙ₋₂)/(n+2)
fn motzkin(n: usize) -> u64 {
    if n < 2 {
        return 1;
    }
    let mut m = vec![0u64; n + 1];
    m[0] = 1;
    m[1] = 1;
    for i in 2..=n {
        let num = (2 * (i as u64) + 1) * m[i - 1] + (3 * (i as u64) - 3) * m[i - 2];
        m[i] = num / (i as u64 + 2);
    }
    m[n]
}

// 21. Adjacency Matrix Powers (count paths of length k)
fn mat_mul(a: &Vec<Vec<u64>>, b: &Vec<Vec<u64>>) -> Vec<Vec<u64>> {
    let n = a.len();
    let mut c = vec![vec![0u64; n]; n];
    for i in 0..n {
        for j in 0..n {
            for k in 0..n {
                c[i][j] = c[i][j].wrapping_add(a[i][k].wrapping_mul(b[k][j]));
            }
        }
    }
    c
}

#[allow(dead_code)]
fn mat_pow(adj: &Vec<Vec<u64>>, k: u32) -> Vec<Vec<u64>> {
    let n = adj.len();
    let mut res = vec![vec![0u64; n]; n];
    for i in 0..n {
        res[i][i] = 1;
    }
    let mut base = adj.clone();
    let mut exp = k;
    while exp > 0 {
        if exp & 1 == 1 {
            res = mat_mul(&res, &base);
        }
        base = mat_mul(&base, &base);
        exp >>= 1;
    }
    res
}

// 22. Perfect Matching Count (bitmask DP)
fn perfect_matchings(adj: &Vec<Vec<bool>>) -> u64 {
    let n = adj.len();
    let size = 1 << n;
    let mut dp = vec![0u64; size];
    dp[0] = 1;
    for mask in 0..size {
        let cnt = mask.count_ones();
        if cnt & 1 == 1 {
            continue;
        }
        if cnt == 0 {
            continue;
        }
        let i = mask.trailing_zeros() as usize;
        let mask_i = mask ^ (1 << i);
        for j in (i + 1)..n {
            if mask & (1 << j) != 0 && adj[i][j] {
                dp[mask] = dp[mask].wrapping_add(dp[mask_i ^ (1 << j)]);
            }
        }
    }
    dp[size - 1]
}

// 23. Chromatic Polynomial: brute count of k-colorings
fn chromatic_count(adj: &Vec<Vec<bool>>, k: u32) -> u64 {
    let n = adj.len();
    let mut count = 0;
    let mut colors = vec![0u32; n];
    fn dfs(i: usize, n: usize, k: u32, adj: &Vec<Vec<bool>>, colors: &mut Vec<u32>, cnt: &mut u64) {
        if i == n {
            *cnt += 1;
            return;
        }
        for c in 0..k {
            let mut ok = true;
            for j in 0..i {
                if adj[i][j] && colors[j] == c {
                    ok = false;
                    break;
                }
            }
            if ok {
                colors[i] = c;
                dfs(i + 1, n, k, adj, colors, cnt);
            }
        }
    }
    dfs(0, n, k, adj, &mut colors, &mut count);
    count
}

// 24. Spanning-Tree Count (Matrix-Tree Theorem via Bareiss)
fn bareiss_det(mut m: Vec<Vec<i64>>) -> i64 {
    let n = m.len();
    let mut denom = 1i64;
    for k in 0..n - 1 {
        if denom == 0 {
            return 0;
        }
        for i in k + 1..n {
            for j in k + 1..n {
                m[i][j] = (m[i][j] * m[k][k] - m[i][k] * m[k][j]) / denom;
            }
        }
        denom = m[k][k];
    }
    m[n - 1][n - 1]
}
fn spanning_tree_count(adj: &Vec<Vec<u64>>) -> u64 {
    let n = adj.len();
    let mut lap = vec![vec![0i64; n]; n];
    for i in 0..n {
        let mut deg = 0i64;
        for j in 0..n {
            if i != j && adj[i][j] > 0 {
                deg += 1;
                lap[i][j] = -(adj[i][j] as i64);
            }
        }
        lap[i][i] = deg;
    }
    // build minor by removing last row/col
    let mut minor = vec![vec![0i64; n - 1]; n - 1];
    for i in 0..n - 1 {
        for j in 0..n - 1 {
            minor[i][j] = lap[i][j];
        }
    }
    bareiss_det(minor) as u64
}

// 26. Topological Sort (Kahn’s algorithm)
fn topo_sort(adj: &Vec<Vec<bool>>) -> Option<Vec<usize>> {
    let n = adj.len();
    let mut indeg = vec![0usize; n];
    for u in 0..n {
        for v in 0..n {
            if adj[u][v] {
                indeg[v] += 1;
            }
        }
    }
    let mut q = Vec::new();
    for i in 0..n {
        if indeg[i] == 0 {
            q.push(i);
        }
    }
    let mut order = Vec::new();
    while let Some(u) = q.pop() {
        order.push(u);
        for v in 0..n {
            if adj[u][v] {
                indeg[v] -= 1;
                if indeg[v] == 0 {
                    q.push(v);
                }
            }
        }
    }
    if order.len() == n {
        Some(order)
    } else {
        None
    }
}

// Strongly Connected Components (Tarjan)
fn tarjan_scc(adj: &Vec<Vec<bool>>) -> Vec<Vec<usize>> {
    let n = adj.len();
    let mut index = vec![None; n];
    let mut low = vec![0; n];
    let mut onstack = vec![false; n];
    let mut stack = Vec::new();
    let mut idx = 0;
    let mut comps = Vec::new();
    fn dfs(
        u: usize,
        adj: &Vec<Vec<bool>>,
        index: &mut Vec<Option<usize>>,
        low: &mut Vec<usize>,
        stack: &mut Vec<usize>,
        onstack: &mut Vec<bool>,
        idx: &mut usize,
        comps: &mut Vec<Vec<usize>>,
    ) {
        index[u] = Some(*idx);
        low[u] = *idx;
        *idx += 1;
        stack.push(u);
        onstack[u] = true;
        for v in 0..adj.len() {
            if adj[u][v] {
                if index[v].is_none() {
                    dfs(v, adj, index, low, stack, onstack, idx, comps);
                    low[u] = low[u].min(low[v]);
                } else if onstack[v] {
                    low[u] = low[u].min(index[v].unwrap());
                }
            }
        }
        if low[u] == index[u].unwrap() {
            let mut comp = Vec::new();
            while let Some(w) = stack.pop() {
                onstack[w] = false;
                comp.push(w);
                if w == u {
                    break;
                }
            }
            comps.push(comp);
        }
    }
    for u in 0..n {
        if index[u].is_none() {
            dfs(u, adj, &mut index, &mut low, &mut stack, &mut onstack, &mut idx, &mut comps);
        }
    }
    comps
}

fn run_program(idx: u8) -> (u64, Vec<u64>) {
    let gas_start = unsafe { gas() };

    let result = match idx {
        0 => {
            let n = (get_random_number() % 30) as u32;
            let res = tribonacci(n);
            call_log(3, None, &format!("tribonacci({}) = {}", n, res));
            vec![res]
        }

        1 => {
            // Narayana Numbers
            let n = (get_random_number() % 20) + 1;
            let k = (get_random_number() % n) + 1;
            let res = narayana(n, k);
            call_log(3, None, &format!("narayana({}, {}) = {}", n, k, res));
            vec![res]
        }
        2 => {
            // Motzkin Numbers
            let n = (get_random_number() % 20) as usize;
            let res = motzkin(n);
            call_log(3, None, &format!("motzkin({}) = {}", n, res));
            vec![res]
        }

        3 => {
            // Legendre Symbol
            let p = ((get_random_number() % 1000) | 1) + 2;
            let a = (get_random_number() % p) as i64;
            let res = legendre_symbol(a, p as i64);
            call_log(3, None, &format!("legendre_symbol ( {}/{}) = {}", a, p, res));
            vec![res as u64]
        }
        4 => {
            // Tonelli-Shanks Algorithm
            let p = ((get_random_number() % 1000) | 1) + 2;
            let n = get_random_number() % p;
            match tonelli_shanks(n, p) {
                Some(r) => {
                    call_log(3, None, &format!("tonelli_shanks({}, {}) = {}", n, p, r));
                    vec![r]
                }
                None => {
                    call_log(3, None, &format!("tonelli_shanks({}, {}) = none", n, p));
                    vec![0]
                }
            }
        }
        5 => {
            // Miller-Rabin Primality Test
            let n = ((get_random_number() % 10_000) | 1) + 2;
            let res = is_prime_miller(n);
            call_log(3, None, &format!("is_prime_miller({}) = {}", n, res));
            vec![res as u64]
        }
        6 => {
            // Prime Factorization
            let n = (get_random_number() % 10_000) + 2;
            let factors = prime_factors(n);
            call_log(3, None, &format!("prime_factors({}) = {:?}", n, factors));
            factors
        }
        7 => {
            // Primitive Root
            let p = next_prime((get_random_number() % 100) + 2);
            let res = primitive_root(p);
            call_log(3, None, &format!("primitive_root({}) = {}", p, res));
            vec![res]
        }
        8 => {
            // Fibonacci pair
            let n = get_random_number() % 50;
            let (f1, f2) = fib(n);
            call_log(3, None, &format!("fib({}) = ({}, {})", n, f1, f2));
            vec![f1, f2]
        }
        9 => {
            // Catalan Number
            let n = get_random_number() % 20;
            let res = catalan(n);
            call_log(3, None, &format!("catalan({}) = {}", n, res));
            vec![res]
        }
        10 => {
            // Bell Number
            let n = (get_random_number() % 15) as usize;
            let res = bell(n);
            call_log(3, None, &format!("bell({}) = {}", n, res));
            vec![res]
        }
        11 => {
            // Derangement
            let n = (get_random_number() % 20) as u32;
            let res = derangement(n);
            call_log(3, None, &format!("derangement({}) = {}", n, res));
            vec![res]
        }
        12 => {
            // Eulerian Number
            let n = (get_random_number() % 12) as usize + 1;
            let k = (get_random_number() % n as u64) as usize;
            let res = eulerian(n, k);
            call_log(3, None, &format!("eulerian({}, {}) = {}", n, k, res));
            vec![res]
        }
        13 => {
            // Partition Count
            let n = (get_random_number() % 50) as usize;
            let res = partition_count(n);
            call_log(3, None, &format!("partition_count({}) = {}", n, res));
            vec![res]
        }
        14 => {
            // Perfect Power Test
            let n = (get_random_number() % 100_000) + 2;
            match perfect_power(n) {
                Some((base, exp)) => {
                    call_log(3, None, &format!("perfect_power({}) = {}^{}", n, base, exp));
                    vec![base, exp as u64]
                }
                None => {
                    call_log(3, None, &format!("perfect_power({}) = none", n));
                    vec![n, 1]
                }
            }
        }
        15 => {
            // Binomial Coefficient
            let n = get_random_number() % 30;
            let k = get_random_number() % (n + 1);
            let res = binomial(n, k);
            call_log(3, None, &format!("binomial({}, {}) = {}", n, k, res));
            vec![res]
        }
        16 => {
            // Extended GCD
            let a = (get_random_number() % 1000) as i64 + 1;
            let b = (get_random_number() % 1000) as i64 + 1;
            let (g, x, y) = extended_gcd(a, b);
            call_log(3, None, &format!("extended_gcd({}, {}) = gcd={}, x={}, y={}", a, b, g, x, y));
            vec![g as u64, x as u64, y as u64]
        }
        17 => {
            // Chinese Remainder Theorem (CRT2)
            let a1 = (get_random_number() % 100) as i64;
            let n1 = ((get_random_number() % 98) + 2) as i64;
            let a2 = (get_random_number() % 100) as i64;
            let n2 = ((get_random_number() % 98) + 2) as i64;
            if gcd(n1 as u64, n2 as u64) == 1 {
                let res = crt2(a1, n1, a2, n2);
                call_log(3, None, &format!("crt2({} mod {}, {} mod {}) = {}", a1, n1, a2, n2, res));
                vec![res as u64]
            } else {
                call_log(3, None, &format!("crt2 - moduli not coprime"));
                vec![0]
            }
        }
        18 => {
            // LCM (Least Common Multiple)
            let a = (get_random_number() % 10_000) + 1;
            let b = (get_random_number() % 10_000) + 1;
            let res = lcm(a, b);
            call_log(3, None, &format!("lcm({}, {}) = {}", a, b, res));
            vec![res]
        }
        19 => {
            // Knapsack 0/1
            let n = (get_random_number() % 8) as usize + 3;
            let mut weights = Vec::new();
            let mut values = Vec::new();
            for _ in 0..n {
                weights.push((get_random_number() % 20 + 1) as usize);
                values.push(get_random_number() % 50 + 1);
            }
            let cap = (get_random_number() % 50 + 20) as usize;
            let res = knapsack(&weights, &values, cap);
            call_log(3, None, &format!("knapsack(cap={}) = {}", cap, res));
            vec![res]
        }
        20 => {
            // LCS (Longest Common Subsequence)
            let len1 = (get_random_number() % 10 + 5) as usize;
            let len2 = (get_random_number() % 10 + 5) as usize;
            let mut a = Vec::new();
            let mut b = Vec::new();
            for _ in 0..len1 {
                a.push((get_random_number() % 26 + 97) as u8); // a-z
            }
            for _ in 0..len2 {
                b.push((get_random_number() % 26 + 97) as u8);
            }
            let res = lcs(&a, &b);
            call_log(3, None, &format!("lcs(len1={}, len2={}) = {}", len1, len2, res));
            vec![res as u64]
        }
        21 => {
            // LIS (Longest Increasing Subsequence)
            let len = (get_random_number() % 15 + 5) as usize;
            let mut seq = Vec::new();
            for _ in 0..len {
                seq.push(get_random_number() % 100);
            }
            let res = lis_length(&seq);
            call_log(3, None, &format!("lis_length(len={}) = {}", len, res));
            vec![res as u64]
        }
        22 => {
            // Levenshtein Distance (Edit Distance)
            let len1 = (get_random_number() % 10 + 5) as usize;
            let len2 = (get_random_number() % 10 + 5) as usize;
            let mut a = Vec::new();
            let mut b = Vec::new();
            for _ in 0..len1 {
                a.push((get_random_number() % 26 + 97) as u8); // a-z
            }
            for _ in 0..len2 {
                b.push((get_random_number() % 26 + 97) as u8);
            }
            let res = levenshtein(&a, &b);
            call_log(3, None, &format!("levenshtein(len1={}, len2={}) = {}", len1, len2, res));
            vec![res as u64]
        }
        23 => {
            // Lucas–Lehmer Test
            let p = (get_random_number() % 50) + 2;
            let res = lucas_lehmer(p);
            call_log(3, None, &format!("lucas_lehmer M_{} is prime? {}", p, res));
            vec![res as u64]
        }
        24 => {
            // Lucas Sequence
            let n = get_random_number() % 20;
            let P = 1;
            let Q = 1;
            let m = (get_random_number() % 100) as i64 + 1;
            let (U, V) = lucas_sequence(n, P, Q, m);
            call_log(3, None, &format!("lucas_sequence U_{},V_{} mod {} = ({},{})", n, n, m, U, V));
            vec![U as u64, V as u64]
        }

        25 => {
            // Baillie–PSW Primality Test
            let n = ((get_random_number() % 10_000) | 1) + 2;
            let res = baillie_psw(n);
            call_log(3, None, &format!("baillie_psw {} is prime? {}", n, res));
            vec![res as u64]
        }
        26 => {
            // Newton Integer √
            let n = get_random_number() % 1_000_000;
            let res = newton_sqrt(n);
            call_log(3, None, &format!("newton_sqrt {} = {}", n, res));
            vec![res]
        }
        27 => {
            // Bareiss 3×3 Determinant
            let mut mat = [[0i64; 3]; 3];
            for i in 0..3 {
                for j in 0..3 {
                    mat[i][j] = (get_random_number() % 101) as i64 - 50;
                }
            }
            let res = det_bareiss_3x3(mat);
            call_log(3, None, &format!("det_bareiss_3x3 det = {}", res));
            vec![res as u64]
        }
        28 => {
            // Smith Normal Form 2×2
            let mat = [
                [(get_random_number() % 101) as i64 - 50, (get_random_number() % 101) as i64 - 50],
                [(get_random_number() % 101) as i64 - 50, (get_random_number() % 101) as i64 - 50],
            ];
            let (d1, d2) = smith_normal_form_2x2(mat);
            call_log(3, None, &format!("smith_normal_form_2x2 diag({}, {})", d1, d2));
            vec![d1 as u64, d2 as u64]
        }
        29 => {
            // Hermite Normal Form 2×2
            let mat = [
                [(get_random_number() % 101) as i64 - 50, (get_random_number() % 101) as i64 - 50],
                [(get_random_number() % 101) as i64 - 50, (get_random_number() % 101) as i64 - 50],
            ];
            let h = hermite_normal_form_2x2(mat);
            call_log(
                3,
                None,
                &format!("hermite_normal_form_2x2 H = [[{},{}],[{},{}]]", h[0][0], h[0][1], h[1][0], h[1][1]),
            );
            vec![h[0][0] as u64, h[0][1] as u64, h[1][0] as u64, h[1][1] as u64]
        }
        30 => {
            // LLL Reduction in 2D
            let b1 = ((get_random_number() % 101) as i64 - 50, (get_random_number() % 101) as i64 - 50);
            let b2 = ((get_random_number() % 101) as i64 - 50, (get_random_number() % 101) as i64 - 50);
            let (r1, r2) = lll_reduce_2d(b1, b2);
            call_log(3, None, &format!("lll_reduce_2d b1={:?}, b2={:?}", r1, r2));
            vec![r1.0 as u64, r1.1 as u64, r2.0 as u64, r2.1 as u64]
        }

        31 => {
            // Montgomery Ladder ModExp
            let base = get_random_number() % 1_000;
            let exp = get_random_number() % 1_000;
            let m = (get_random_number() % 1_000) + 1;
            let res = mod_exp_ladder(base, exp, m);
            call_log(3, None, &format!("mod_exp_ladder({}, {}, {}) = {}", base, exp, m, res));
            vec![res]
        }

        32 => {
            // Stein's Binary GCD
            let a = get_random_number() % 1_000_000;
            let b = get_random_number() % 1_000_000;
            let res = stein_gcd(a, b);
            call_log(3, None, &format!("stein_gcd({}, {}) = {}", a, b, res));
            vec![res]
        }
        33 => {
            // Subtraction‑Only GCD
            let a = get_random_number() % 1_000_000;
            let b = get_random_number() % 1_000_000;
            let res = sub_gcd(a, b);
            call_log(3, None, &format!("sub_gcd({}, {}) = {}", a, b, res));
            vec![res]
        }

        34 => {
            // Integer Log via Multiplication
            let n = (get_random_number() % 1_000_000) + 1;
            let b = (get_random_number() % 9) + 2;
            let res = integer_log_mul(n, b);
            call_log(3, None, &format!("integer_log_mul({}, {}) = {}", n, b, res));
            vec![res as u64]
        }
        35 => {
            // Integer Log via Division
            let n = (get_random_number() % 1_000_000) + 1;
            let b = (get_random_number() % 9) + 2;
            let res = integer_log_div(n, b);
            call_log(3, None, &format!("integer_log_div({}, {}) = {}", n, b, res));
            vec![res as u64]
        }
        36 => {
            // Perfect Square Test
            let n = get_random_number() % 1_000_000;
            let res = is_perfect_square(n);
            call_log(3, None, &format!("is_perfect_square({}) = {}", n, res));
            vec![res as u64]
        }

        37 => {
            // Coin Change Count
            let n = (get_random_number() % 100) as usize;
            let res = coin_change_count(n);
            call_log(3, None, &format!("coin_change_count({})={}", n, res));
            vec![res]
        }
        38 => {
            // Coin Change Min
            let n = (get_random_number() % 100) as usize;
            let res = coin_change_min(n);
            call_log(3, None, &format!("coin_change_min({})={}", n, res));
            vec![res as u64]
        }

        39 => {
            // Modular Exponentiation & Inverse
            let base = (get_random_number() % 1000) + 1;
            let exp = get_random_number() % 1000;
            let m = (get_random_number() % 999) + 1;
            let me = mod_exp(base, exp, m);
            let inv = mod_inv(base as i64, m as i64);
            call_log(3, None, &format!("mod_exp({},{},{})={}, mod_inv={:?}", base, exp, m, me, inv));
            match inv {
                Some(i) => vec![me, i as u64],
                None => vec![me],
            }
        }
        40 => {
            // CRT2, Garner & Nth‑Root
            let a1 = (get_random_number() % 100) as i64;
            let n1 = ((get_random_number() % 98) + 2) as i64;
            let a2 = (get_random_number() % 100) as i64;
            let n2 = ((get_random_number() % 98) + 2) as i64;
            let mut results = Vec::new();
            if gcd(n1 as u64, n2 as u64) == 1 {
                let crt_res = crt2(a1, n1, a2, n2);
                call_log(3, None, &format!("crt2 = {}", crt_res));
                results.push(crt_res as u64);
            }
            // Garner
            let mods = [2, 3, 5];
            let rems = [
                (get_random_number() % 2) as i64,
                (get_random_number() % 3) as i64,
                (get_random_number() % 5) as i64,
            ];
            let garner_res = garner(&rems, &mods);
            call_log(3, None, &format!("garner = {}", garner_res));
            results.push(garner_res as u64);
            // Nth‑root
            let n = get_random_number() % 1_000_000;
            let k = (get_random_number() % 4) + 2;
            let root_res = integer_nth_root(n, k.try_into().unwrap());
            call_log(3, None, &format!("nth_root({},{}) = {}", n, k, root_res));
            results.push(root_res);
            results
        }

        41 => {
            // Number Theoretic Transform
            let mut poly = [0u64; NTT_N];
            for i in 0..NTT_N {
                poly[i] = get_random_number() % MOD_NTT;
            }
            let res = ntt(&poly);
            call_log(3, None, &format!("ntt({:?}) = {:?}", poly, res));
            res.to_vec()
        }
        42 => {
            // CORDIC Rotation
            let angle = (get_random_number() % 200_001) as i32 - 100_000;
            let (c, s) = cordic(angle);
            call_log(3, None, &format!("angle={} → cos≈{}, sin≈{}", angle, c, s));
            vec![c as u64, s as u64]
        }

        43 => {
            // Pseudo-Random Generators
            let mut lcg = Lcg {
                state: get_random_number() as u32,
                a: 1664525,
                c: 1013904223,
            };
            let lcg_res = lcg.next();
            call_log(3, None, &format!("lcg.next() = {}", lcg_res));

            let xor_res = xorshift64(get_random_number() as u64);
            call_log(3, None, &format!("xorshift64 = {}", xor_res));

            let mut pcg = Pcg {
                state: get_random_number() as u64,
                inc: get_random_number() as u64,
            };
            let pcg_res = pcg.next();
            call_log(3, None, &format!("pcg.next() = {}", pcg_res));

            let mut mwc = Mwc {
                state: get_random_number() as u64,
                carry: get_random_number() as u64 & 0xFFFF_FFFF,
            };
            let mwc_res = mwc.next();
            call_log(3, None, &format!("mwc.next() = {}", mwc_res));

            vec![lcg_res as u64, xor_res, pcg_res as u64, mwc_res as u64]
        }
        44 => {
            // CRC32, Adler-32, FNV-1a, Murmur, Jenkins
            let len = (get_random_number() % 32) as usize;
            let mut data = Vec::with_capacity(len);
            for _ in 0..len {
                data.push(get_random_number() as u8);
            }
            let crc = crc32(&data);
            let adler = adler32(&data);
            let fnv = fnv1a(&data);
            let murmur = murmur3_finalizer(get_random_number() as u32);
            let jenk = jenkins(&data);
            call_log(
                3,
                None,
                &format!(
                    "crc32={:08x}, adler32={:08x}, fnv1a={:08x}, murmur3={:08x}, jenkins={:08x}",
                    crc, adler, fnv, murmur, jenk
                ),
            );
            vec![crc as u64, adler as u64, fnv as u64, murmur as u64, jenk as u64]
        }
        45 => {
            // Euler's Totient φ(n)
            let n = (get_random_number() % 100_000) + 1;
            let res = phi(n);
            call_log(3, None, &format!("eulerTotient phi({}) = {}", n, res));
            vec![res]
        }

        46 => {
            // Linear SieveMu
            let n = (get_random_number() % 1_000) as usize + 1;
            let mu = linear_mu(n);
            let res = mu[n];
            call_log(3, None, &format!("linear_mu n={}, [n]={}", n, res));
            vec![res as u64]
        }
        47 => {
            // Sum of Divisors
            let n = (get_random_number() % 100_000) + 1;
            let res = sigma(n);
            call_log(3, None, &format!("SumOfDivisors sigma({}) = {}", n, res));
            vec![res]
        }
        48 => {
            // Divisor Count d(n)
            let n = (get_random_number() % 100_000) + 1;
            let res = divisor_count(n);
            call_log(3, None, &format!("divisor_count({}) = {}", n, res));
            vec![res]
        }
        49 => {
            // Mobius
            let n = (get_random_number() % 100_000) + 1;
            let res = mobius(n);
            call_log(3, None, &format!("mobius({}) = {}", n, res));
            vec![res as u64]
        }
        50 => {
            // Dirichlet Convolution (1 * id)
            let n = (get_random_number() % 1_000) + 1;
            let res = dirichlet_convolution(n, |_| 1, |d| d);
            call_log(3, None, &format!("dirichlet_convolution (1 * id)({}) = {}", n, res));
            vec![res]
        }
        51 => {
            // Jacobi Symbol (a/n)
            let n = ((get_random_number() % 999) | 1) + 2; // odd ≥3
            let a = (get_random_number() % (n as u64)) as i64;
            let res = jacobi(a.try_into().unwrap(), (n as i64).try_into().unwrap());
            call_log(2, None, &format!("jacobi( {}/{}) = {}", a, n, res));
            vec![res as u64]
        }
        52 => {
            // Cipolla's Algorithm
            let p = ((get_random_number() % 1000) | 1) + 2;
            let n = get_random_number() % p;
            match cipolla(n, p) {
                Some(r) => {
                    call_log(3, None, &format!("{}", r));
                    vec![r]
                }
                None => {
                    call_log(3, None, &format!("none"));
                    vec![idx as u64]
                }
            }
        }
        53..=u8::MAX => {
            call_log(2, None, &format!("not implemented {}", idx));
            vec![idx as u64]
        }
    };

    let gas_end = unsafe { gas() };
    (gas_start - gas_end, result)
}

#[polkavm_derive::polkavm_export]
extern "C" fn refine(start_address: u64, length: u64) -> (u64, u64) {

       polkavm_sbrk(1024*1024);
    let args = if let Some(a) = parse_refine_args(start_address, length) {
        a
    } else {
        return (FIRST_READABLE_ADDRESS as u64, 0);
    };

    let payload_ptr = args.wi_payload_start_address as *const u8;
    let payload_len = args.wi_payload_length as usize;
    let payload = unsafe { slice::from_raw_parts(payload_ptr, payload_len) };

    // run each (program_id, count) pair and create WriteIntents
    let pairs = payload_len / 2;
    call_log(2, None, &format!("algo refine PAYLOAD {} {}", payload_len, pairs));

    let mut write_intents = Vec::new();
    let mut total_gas_used = 0u64;
        let mut export_count: u16 = 0;

    for i in 0..pairs {
        let program_id = payload[i * 2];
        let p_id = program_id % 170;
        let count = payload[i * 2 + 1] as u64;
        let iterations = 1 as u64;
        call_log(2, None, &format!("algo refine PROGRAM_ID {} ITERATIONS {}", program_id, iterations));
        let mut gas_used = 0 as u64;
        let mut all_results = Vec::new();
        for _ in 0..iterations {
            let (gas, result) = run_program(p_id);
            gas_used += gas;
            all_results.extend(result);
        }
        total_gas_used += gas_used;

        // Create a WriteIntent for this pair
        // Object ID: hash of (pair_index, program_id)
        let mut id_bytes = [0u8; 2];
        id_bytes[0] = i as u8;
        id_bytes[1] = program_id;
        let object_id = blake2b_hash(&id_bytes);

        // Payload: program_id (1 byte) + count (1 byte) + gas_used (8 bytes LE) + all results (8 bytes each LE)
        let mut intent_payload = Vec::new();
        intent_payload.push(program_id);
        intent_payload.push(count as u8);
        intent_payload.extend_from_slice(&gas_used.to_le_bytes());
        for result in all_results {
            intent_payload.extend_from_slice(&result.to_le_bytes());
        }

        let mut write_intent = WriteIntent {
            effect: WriteEffectEntry {
                object_id,
                ref_info: ObjectRef {
                    service_id: args.wi_service_index,
                    work_package_hash: args.wphash,
                    index_start: export_count,
                    index_end: export_count + 1,
                    version: 1,
                    payload_length: intent_payload.len() as u32,
                    timeslot: 0,
                    gas_used: gas_used as u32,
                    evm_block: 0,
                    object_kind: 0,
                    log_index: 0,
                    tx_slot: i as u16,
                },
                payload: intent_payload,
            },
            dependencies: Vec::new(),
        };
        // Export payloads to DA segments
            match write_intent.effect.export_effect(export_count as usize) {
                Ok(next_index) => {
                    export_count = next_index;
                }
                Err(e) => {
                    log_error(&format!("  ❌ Failed to export write intent {}: {:?}", i, e));
                }
            }
        write_intents.push(write_intent);




        call_log(
            2,
            None,
            &format!("algo run_refine {} ITERATIONS {} gas_used {}", program_id, iterations, gas_used),
        )
    }

    // Create ExecutionEffects
    let effects = ExecutionEffects {
        export_count,
        gas_used: total_gas_used,
        state_root: [0u8; 32],
        write_intents,
    };

    // Serialize ExecutionEffects
    // Format: export_count (2B) | gas_used (8B) | count (2B) | WriteIntent entries
    let mut buffer = Vec::new();
    buffer.extend_from_slice(&effects.export_count.to_le_bytes());
    buffer.extend_from_slice(&effects.gas_used.to_le_bytes());

    let count = effects.write_intents.len() as u16;
    buffer.extend_from_slice(&count.to_le_bytes());

    for intent in &effects.write_intents {
        // Serialize: object_id (32B) + object_ref (64B) + dep_count (2B) + payload_length (4B) + payload
        buffer.extend_from_slice(&intent.effect.object_id);
        buffer.extend_from_slice(&intent.effect.ref_info.serialize());

        let dep_count = intent.dependencies.len() as u16;
        buffer.extend_from_slice(&dep_count.to_le_bytes());

        let payload_len = intent.effect.payload.len() as u32;
        buffer.extend_from_slice(&payload_len.to_le_bytes());
        buffer.extend_from_slice(&intent.effect.payload);
    }

    // Leak buffer and return pointer/length
    let ptr = buffer.as_mut_ptr() as u64;
    let len = buffer.len() as u64;


    return (ptr, len);
}

#[unsafe(no_mangle)]
static mut output_bytes_32: [u8; 32] = [0; 32];

#[polkavm_derive::polkavm_export]
extern "C" fn accumulate(start_address: u64, length: u64) -> (u64, u64) {
    polkavm_sbrk(1024*1024);

    let (timeslot, service_id, num_accumulate_inputs) = if let Some(args) = parse_accumulate_args(start_address, length) {
        (args.t, args.s, args.num_accumulate_inputs)
    } else {
        return (FIRST_READABLE_ADDRESS as u64, 0);
    };

    log_info(&format!("algo accumulate: service_id={} timeslot={} num_inputs={}", service_id, timeslot, num_accumulate_inputs));

    // Fetch accumulate inputs
    use utils::functions::fetch_accumulate_inputs;
    let accumulate_inputs = match fetch_accumulate_inputs(num_accumulate_inputs as u64) {
        Ok(inputs) => inputs,
        Err(e) => {
            log_error(&format!("Failed to fetch accumulate inputs: {:?}", e));
            return (FIRST_READABLE_ADDRESS as u64, 0);
        }
    };

    // Read current block number from storage using BLOCK_NUMBER_KEY
    let block_number = match EvmBlockPayload::read_blocknumber_key(service_id) {
        Some(num) => num,
        None => {
            log_info("Failed to read block number, using 0");
            0
        }
    };
    let next_block_number = block_number + 1;
    log_info(&format!("Current block_number={}, next={}", block_number, next_block_number));

    // Collect all WriteIntents from all inputs
    let mut tx_hashes = Vec::new();
    let mut receipt_hashes = Vec::new();
    let mut total_gas_used = 0u64;

    for (idx, input) in accumulate_inputs.iter().enumerate() {
        use utils::functions::AccumulateInput;
        let AccumulateInput::OperandElements(operand_elem) = input else {
            log_error(&format!("Input #{} not OperandElements", idx));
            continue;
        };

        let Some(ok_data) = operand_elem.result.ok.as_ref() else {
            log_error(&format!("Input #{} has no ok_data", idx));
            continue;
        };

        // Deserialize ExecutionEffects using EVM's deserializer
        match deserialize_execution_effects(ok_data) {
            Some(envelope) => {
                log_info(&format!("Input #{}: {} writes, gas_used={}", idx, envelope.writes.len(), envelope.gas_used));
                total_gas_used += envelope.gas_used;

                // Write each object_ref to storage and collect hashes
                for write in envelope.writes {
                    // Write object_ref to storage (object_id => object_ref)
                    write.object_ref.write(&write.object_id);

                    // Add to tx_hashes (the object_id itself)
                    tx_hashes.push(write.object_id);

                    // Compute hash of object_ref for receipt_hashes
                    let ref_hash = blake2b_hash(&write.object_ref.serialize());
                    receipt_hashes.push(ref_hash);

                    log_info(&format!("Wrote object_id={}", format_object_id(&write.object_id)));
                }
            }
            None => {
                log_error(&format!("Failed to deserialize input #{}", idx));
            }
        }
    }

    // Create block payload using EvmBlockPayload structure
    let parent_hash = read_parent_hash(service_id, block_number as u64);

    let mut block_payload = EvmBlockPayload {
        number: next_block_number as u64,
        parent_hash,
        logs_bloom: [0u8; 256],  // Algo service doesn't use logs
        transactions_root: [0u8; 32],  // Will be computed below
        state_root: [0u8; 32],  // Could be set to JAM state root if needed
        receipts_root: [0u8; 32],  // Will be computed below
        miner: [0u8; 20],  // Could be set to validator address
        extra_data: [0u8; 32],  // Could store algo-specific metadata
        size: 0,  // Computed during serialization
        gas_limit: 30_000_000,
        gas_used: total_gas_used,
        timestamp: timeslot as u64,
        tx_hashes,
        receipt_hashes,
    };

    // Compute transactions root and receipts root using EVM's BMT method
    block_payload.transactions_root = block_payload.compute_transactions_root();
    block_payload.receipts_root = block_payload.compute_receipts_root();

    // Write block to storage (reuses EvmBlockPayload::write method)
    if block_payload.write().is_none() {
        log_error("Failed to write block to storage");
        return (FIRST_READABLE_ADDRESS as u64, 0);
    }

    // Compute block hash from header
    let block_hash = block_payload.compute_block_hash();

    // Update block number key using EVM's method
    EvmBlockPayload::write_blocknumber_key(next_block_number, &block_hash);

    log_info(&format!(
        "Block #{} finalized: {} txs, gas={}, block_hash={}, tx_root={}, receipt_root={}",
        next_block_number,
        block_payload.tx_hashes.len(),
        total_gas_used,
        format_object_id(&block_hash),
        format_object_id(&block_payload.transactions_root),
        format_object_id(&block_payload.receipts_root)
    ));

    // Return block hash by copying to FIRST_READABLE_ADDRESS
    unsafe {
        let dest = FIRST_READABLE_ADDRESS as *mut u8;
        core::ptr::copy_nonoverlapping(block_hash.as_ptr(), dest, 32);
    }
    (FIRST_READABLE_ADDRESS as u64, 32)
}

fn read_parent_hash(service_id: u32, block_number: u64) -> [u8; 32] {
    if block_number == 0 {
        return [0u8; 32]; // Genesis parent
    }

    // Read from BLOCK_NUMBER_KEY which contains block_number + parent_hash
    use utils::constants::BLOCK_NUMBER_KEY;
    use utils::host_functions::read as host_read;
    use utils::constants::NONE;

    let mut buffer = [0u8; 36]; // 4 bytes block_number + 32 bytes parent_hash

    let len = unsafe {
        host_read(
            service_id as u64,
            BLOCK_NUMBER_KEY.as_ptr() as u64,
            BLOCK_NUMBER_KEY.len() as u64,
            buffer.as_mut_ptr() as u64,
            0,
            buffer.len() as u64,
        )
    };

    if len == NONE || len < 36 {
        return [0u8; 32];
    }

    // Extract parent_hash from bytes 4-36
    let mut parent_hash = [0u8; 32];
    parent_hash.copy_from_slice(&buffer[4..36]);
    parent_hash
}
