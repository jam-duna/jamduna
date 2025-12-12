#![no_std]
#![no_main]
#![allow(non_snake_case)]

extern crate alloc;
use alloc::format;
use alloc::vec;
use alloc::vec::Vec;
use core::mem;
//use core::mem::transmute;
use core::primitive::u64;

const SIZE0: usize = 0x40000;
// allocate memory for stack
use polkavm_derive::min_stack_size;
min_stack_size!(SIZE0);

const SIZE1: usize = 0x400000;
// allocate memory for heap
use simplealloc::SimpleAlloc;
#[global_allocator]
static ALLOCATOR: SimpleAlloc<SIZE1> = SimpleAlloc::new();

use core::slice;
use core::sync::atomic::{AtomicU64, Ordering};
use utils::functions::{log_info, log_error, parse_accumulate_args, parse_refine_args, fetch_accumulate_inputs};
use utils::helpers::{leak_output, empty_output};
use utils::host_functions::gas;
use utils::effects::{ExecutionEffects, WriteIntent, WriteEffectEntry};
use utils::objects::ObjectRef;
use utils::hash_functions::blake2b_hash;
use evm_service::{accumulator, serialize_execution_effects, format_object_id};

// Log structure matching EVM Log format
#[derive(Clone)]
pub struct Log {
    pub address: [u8; 20],     // H160
    pub topics: Vec<[u8; 32]>, // Vec<H256>
    pub data: Vec<u8>,         // Vec<u8>
}

/// Serialize logs to binary format matching EVM receipt format
/// Format: [log_count:u16][Log...][Log...]
/// Each Log: [address:20B][topic_count:u8][topics:32B*N][data_len:u32][data:bytes]
fn serialize_logs(logs: &[Log]) -> Vec<u8> {
    let mut result = Vec::new();

    // Log count (2 bytes LE)
    let count = logs.len() as u16;
    result.extend_from_slice(&count.to_le_bytes());

    // Serialize each log
    for log in logs {
        // Address (20 bytes)
        result.extend_from_slice(&log.address);

        // Topic count (1 byte)
        let topic_count = log.topics.len() as u8;
        result.push(topic_count);

        // Topics (32 bytes each)
        for topic in &log.topics {
            result.extend_from_slice(topic);
        }

        // Data length (4 bytes LE)
        let data_len = log.data.len() as u32;
        result.extend_from_slice(&data_len.to_le_bytes());

        // Data
        result.extend_from_slice(&log.data);
    }

    result
}

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

fn lcm(a: u64, b: u64) -> u64 {
    a / gcd(a, b) * b
}
/// Helper to create a u256 topic from a u64 value (LE encoding in first 8 bytes)
fn u64_to_topic(val: u64) -> [u8; 32] {
    let mut topic = [0u8; 32];
    topic[0..8].copy_from_slice(&val.to_le_bytes());
    topic
}

/// Helper to convert program results into a log
/// Creates log with topics: [program_id, input1, input2, ...] and data containing outputs
fn results_to_log(program_id: u8, inputs: &[u64], outputs: &[u64]) -> Log {
    let mut topics = vec![u64_to_topic(program_id as u64)];
    for &input in inputs {
        topics.push(u64_to_topic(input));
    }

    // Put outputs in log data (serialized as LE u64s)
    let mut data = Vec::new();
    for &output in outputs {
        data.extend_from_slice(&output.to_le_bytes());
    }

    Log {
        address: [0u8; 20],
        topics,
        data,
    }
}

fn run_program(idx: u8) -> (u64, Vec<Log>) {
    let gas_start = unsafe { gas() };

    let result = match idx {
        0 => {
            // Tribonacci
            let n = (get_random_number() % 30) as u32;
            let res = tribonacci(n);
            vec![results_to_log(0, &[n as u64], &[res])]
        }

        1 => {
            // Narayana Numbers
            let n = (get_random_number() % 20) + 1;
            let k = (get_random_number() % n) + 1;
            let res = narayana(n, k);
            vec![results_to_log(1, &[n, k], &[res])]
        }
        2 => {
            // Motzkin Numbers
            let n = (get_random_number() % 20) as usize;
            let res = motzkin(n);
            vec![results_to_log(2, &[n as u64], &[res])]
        }

        3 => {
            // Legendre Symbol
            let p = ((get_random_number() % 1000) | 1) + 2;
            let a = (get_random_number() % p) as i64;
            let res = legendre_symbol(a, p as i64);
            vec![results_to_log(3, &[a as u64, p], &[res as u64])]
        }
        4 => {
            // Tonelli-Shanks Algorithm
            let p = ((get_random_number() % 1000) | 1) + 2;
            let n = get_random_number() % p;
            let res = match tonelli_shanks(n, p) {
                Some(r) => r,
                None => 0,
            };
            vec![results_to_log(4, &[n, p], &[res])]
        }
        5 => {
            // Miller-Rabin Primality Test
            let n = ((get_random_number() % 10_000) | 1) + 2;
            let res = is_prime_miller(n);
            vec![results_to_log(5, &[n], &[res as u64])]
        }
        6 => {
            // Prime Factorization
            let n = (get_random_number() % 10_000) + 2;
            let factors = prime_factors(n);
            vec![results_to_log(6, &[n], &factors)]
        }
        7 => {
            // Primitive Root
            let p = next_prime((get_random_number() % 100) + 2);
            let res = primitive_root(p);
            vec![results_to_log(7, &[p], &[res])]
        }
        8 => {
            // Fibonacci pair
            let n = get_random_number() % 50;
            let (f1, f2) = fib(n);
            vec![results_to_log(8, &[n], &[f1, f2])]
        }
        9 => {
            // Catalan Number
            let n = get_random_number() % 20;
            let res = catalan(n);
            vec![results_to_log(9, &[n], &[res])]
        }
        10 => {
            // Bell Number
            let n = (get_random_number() % 15) as usize;
            let res = bell(n);
            vec![results_to_log(10, &[n as u64], &[res])]
        }
        11 => {
            // Derangement
            let n = (get_random_number() % 20) as u32;
            let res = derangement(n);
            vec![results_to_log(11, &[n as u64], &[res])]
        }
        12 => {
            // Eulerian Number
            let n = (get_random_number() % 12) as usize + 1;
            let k = (get_random_number() % n as u64) as usize;
            let res = eulerian(n, k);
            vec![results_to_log(12, &[n as u64, k as u64], &[res])]
        }
        13 => {
            // Partition Count
            let n = (get_random_number() % 50) as usize;
            let res = partition_count(n);
            vec![results_to_log(13, &[n as u64], &[res])]
        }
        14 => {
            // Perfect Power Test
            let n = (get_random_number() % 100_000) + 2;
            let (base, exp) = match perfect_power(n) {
                Some((b, e)) => (b, e as u64),
                None => (n, 1),
            };
            vec![results_to_log(14, &[n], &[base, exp])]
        }
        15 => {
            let n = get_random_number() % 30;
            let k = get_random_number() % (n + 1);
            let res = binomial(n, k);
            vec![results_to_log(15, &[n, k], &[res])]
        }
        16 => {
            let a = (get_random_number() % 1000) as i64 + 1;
            let b = (get_random_number() % 1000) as i64 + 1;
            let (g, x, y) = extended_gcd(a, b);
            vec![results_to_log(16, &[a as u64, b as u64], &[g as u64, x as u64, y as u64])]
        }
        17 => {
            let a1 = (get_random_number() % 100) as i64;
            let n1 = ((get_random_number() % 98) + 2) as i64;
            let a2 = (get_random_number() % 100) as i64;
            let n2 = ((get_random_number() % 98) + 2) as i64;
            let res = if gcd(n1 as u64, n2 as u64) == 1 {
                crt2(a1, n1, a2, n2) as u64
            } else {
                0
            };
            vec![results_to_log(17, &[a1 as u64, n1 as u64, a2 as u64, n2 as u64], &[res])]
        }
        18 => {
            let a = (get_random_number() % 10_000) + 1;
            let b = (get_random_number() % 10_000) + 1;
            let res = lcm(a, b);
            vec![results_to_log(18, &[a, b], &[res])]
        }
        19 => {
            let n = (get_random_number() % 8) as usize + 3;
            let mut weights = Vec::new();
            let mut values = Vec::new();
            for _ in 0..n {
                weights.push((get_random_number() % 20 + 1) as usize);
                values.push(get_random_number() % 50 + 1);
            }
            let cap = (get_random_number() % 50 + 20) as usize;
            let res = knapsack(&weights, &values, cap);
            vec![results_to_log(19, &[n as u64, cap as u64], &[res])]
        }
        20 => {
            let len1 = (get_random_number() % 10 + 5) as usize;
            let len2 = (get_random_number() % 10 + 5) as usize;
            let mut a = Vec::new();
            let mut b = Vec::new();
            for _ in 0..len1 {
                a.push((get_random_number() % 26 + 97) as u8);
            }
            for _ in 0..len2 {
                b.push((get_random_number() % 26 + 97) as u8);
            }
            let res = lcs(&a, &b);
            vec![results_to_log(20, &[len1 as u64, len2 as u64], &[res as u64])]
        }
        21 => {
            let len = (get_random_number() % 15 + 5) as usize;
            let mut seq = Vec::new();
            for _ in 0..len {
                seq.push(get_random_number() % 100);
            }
            let res = lis_length(&seq);
            vec![results_to_log(21, &[len as u64], &[res as u64])]
        }
        22 => {
            let len1 = (get_random_number() % 10 + 5) as usize;
            let len2 = (get_random_number() % 10 + 5) as usize;
            let mut a = Vec::new();
            let mut b = Vec::new();
            for _ in 0..len1 {
                a.push((get_random_number() % 26 + 97) as u8);
            }
            for _ in 0..len2 {
                b.push((get_random_number() % 26 + 97) as u8);
            }
            let res = levenshtein(&a, &b);
            vec![results_to_log(22, &[len1 as u64, len2 as u64], &[res as u64])]
        }
        23 => {
            let p = (get_random_number() % 50) + 2;
            let res = lucas_lehmer(p);
            vec![results_to_log(23, &[p], &[res as u64])]
        }
        24 => {
            let n = get_random_number() % 20;
            let P = 1;
            let Q = 1;
            let m = (get_random_number() % 100) as i64 + 1;
            let (U, V) = lucas_sequence(n, P, Q, m);
            vec![results_to_log(24, &[n, P as u64, Q as u64, m as u64], &[U as u64, V as u64])]
        }
        25 => {
            let n = ((get_random_number() % 10_000) | 1) + 2;
            let res = baillie_psw(n);
            vec![results_to_log(25, &[n], &[res as u64])]
        }
        26 => {
            let n = get_random_number() % 1_000_000;
            let res = newton_sqrt(n);
            vec![results_to_log(26, &[n], &[res])]
        }
        27 => {
            let mut mat = [[0i64; 3]; 3];
            for i in 0..3 {
                for j in 0..3 {
                    mat[i][j] = (get_random_number() % 101) as i64 - 50;
                }
            }
            let res = det_bareiss_3x3(mat);
            vec![results_to_log(27, &[], &[res as u64])]
        }
        28 => {
            let mat = [
                [(get_random_number() % 101) as i64 - 50, (get_random_number() % 101) as i64 - 50],
                [(get_random_number() % 101) as i64 - 50, (get_random_number() % 101) as i64 - 50],
            ];
            let (d1, d2) = smith_normal_form_2x2(mat);
            vec![results_to_log(28, &[], &[d1 as u64, d2 as u64])]
        }
        29 => {
            let mat = [
                [(get_random_number() % 101) as i64 - 50, (get_random_number() % 101) as i64 - 50],
                [(get_random_number() % 101) as i64 - 50, (get_random_number() % 101) as i64 - 50],
            ];
            let h = hermite_normal_form_2x2(mat);
            vec![results_to_log(29, &[], &[h[0][0] as u64, h[0][1] as u64, h[1][0] as u64, h[1][1] as u64])]
        }
        30 => {
            let b1 = ((get_random_number() % 101) as i64 - 50, (get_random_number() % 101) as i64 - 50);
            let b2 = ((get_random_number() % 101) as i64 - 50, (get_random_number() % 101) as i64 - 50);
            let (r1, r2) = lll_reduce_2d(b1, b2);
            vec![results_to_log(30, &[], &[r1.0 as u64, r1.1 as u64, r2.0 as u64, r2.1 as u64])]
        }

        31 => {
            // Montgomery Ladder ModExp
            let base = get_random_number() % 1_000;
            let exp = get_random_number() % 1_000;
            let m = (get_random_number() % 1_000) + 1;
            let res = mod_exp_ladder(base, exp, m);
            vec![results_to_log(idx, &[], &[res])]
        }

        32 => {
            // Stein's Binary GCD
            let a = get_random_number() % 1_000_000;
            let b = get_random_number() % 1_000_000;
            let res = stein_gcd(a, b);
            vec![results_to_log(idx, &[], &[res])]
        }
        33 => {
            // Subtraction‑Only GCD
            let a = get_random_number() % 1_000_000;
            let b = get_random_number() % 1_000_000;
            let res = sub_gcd(a, b);
            vec![results_to_log(idx, &[], &[res])]
        }

        34 => {
            // Integer Log via Multiplication
            let n = (get_random_number() % 1_000_000) + 1;
            let b = (get_random_number() % 9) + 2;
            let res = integer_log_mul(n, b);
            vec![results_to_log(idx, &[], &[res as u64])]
        }
        35 => {
            // Integer Log via Division
            let n = (get_random_number() % 1_000_000) + 1;
            let b = (get_random_number() % 9) + 2;
            let res = integer_log_div(n, b);
            vec![results_to_log(idx, &[], &[res as u64])]
        }
        36 => {
            // Perfect Square Test
            let n = get_random_number() % 1_000_000;
            let res = is_perfect_square(n);
            vec![results_to_log(idx, &[], &[res as u64])]
        }

        37 => {
            // Coin Change Count
            let n = (get_random_number() % 100) as usize;
            let res = coin_change_count(n);
            vec![results_to_log(idx, &[], &[res])]
        }
        38 => {
            // Coin Change Min
            let n = (get_random_number() % 100) as usize;
            let res = coin_change_min(n);
            vec![results_to_log(idx, &[], &[res as u64])]
        }

        39 => {
            let base = (get_random_number() % 1000) + 1;
            let exp = get_random_number() % 1000;
            let m = (get_random_number() % 999) + 1;
            let me = mod_exp(base, exp, m);
            let inv = mod_inv(base as i64, m as i64);
            let outputs = match inv {
                Some(i) => vec![me, i as u64],
                None => vec![me],
            };
            vec![results_to_log(idx, &[], &outputs)]
        }
        40 => {
            let a1 = (get_random_number() % 100) as i64;
            let n1 = ((get_random_number() % 98) + 2) as i64;
            let a2 = (get_random_number() % 100) as i64;
            let n2 = ((get_random_number() % 98) + 2) as i64;
            let mut results = Vec::new();
            if gcd(n1 as u64, n2 as u64) == 1 {
                let crt_res = crt2(a1, n1, a2, n2);
                results.push(crt_res as u64);
            }
            let mods = [2, 3, 5];
            let rems = [
                (get_random_number() % 2) as i64,
                (get_random_number() % 3) as i64,
                (get_random_number() % 5) as i64,
            ];
            let garner_res = garner(&rems, &mods);
            results.push(garner_res as u64);
            let n = get_random_number() % 1_000_000;
            let k = (get_random_number() % 4) + 2;
            let root_res = integer_nth_root(n, k.try_into().unwrap());
            results.push(root_res);
            vec![results_to_log(idx, &[], &results)]
        }

        41 => {
            let mut poly = [0u64; NTT_N];
            for i in 0..NTT_N {
                poly[i] = get_random_number() % MOD_NTT;
            }
            let res = ntt(&poly);
            vec![results_to_log(idx, &[], &res)]
        }
        42 => {
            // CORDIC Rotation
            let angle = (get_random_number() % 200_001) as i32 - 100_000;
            let (c, s) = cordic(angle);
            vec![results_to_log(idx, &[], &[c as u64, s as u64])]
        }

        43 => {
            // Pseudo-Random Generators
            let mut lcg = Lcg {
                state: get_random_number() as u32,
                a: 1664525,
                c: 1013904223,
            };
            let lcg_res = lcg.next();

            let xor_res = xorshift64(get_random_number() as u64);

            let mut pcg = Pcg {
                state: get_random_number() as u64,
                inc: get_random_number() as u64,
            };
            let pcg_res = pcg.next();

            let mut mwc = Mwc {
                state: get_random_number() as u64,
                carry: get_random_number() as u64 & 0xFFFF_FFFF,
            };
            let mwc_res = mwc.next();

            vec![results_to_log(idx, &[], &[lcg_res as u64, xor_res, pcg_res as u64, mwc_res as u64])]
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
            vec![results_to_log(idx, &[], &[crc as u64, adler as u64, fnv as u64, murmur as u64, jenk as u64])]
        }
        45 => {
            // Euler's Totient φ(n)
            let n = (get_random_number() % 100_000) + 1;
            let res = phi(n);
            vec![results_to_log(idx, &[], &[res])]
        }

        46 => {
            // Linear SieveMu
            let n = (get_random_number() % 1_000) as usize + 1;
            let mu = linear_mu(n);
            let res = mu[n];
            vec![results_to_log(idx, &[], &[res as u64])]
        }
        47 => {
            // Sum of Divisors
            let n = (get_random_number() % 100_000) + 1;
            let res = sigma(n);
            vec![results_to_log(idx, &[], &[res])]
        }
        48 => {
            // Divisor Count d(n)
            let n = (get_random_number() % 100_000) + 1;
            let res = divisor_count(n);
            vec![results_to_log(idx, &[], &[res])]
        }
        49 => {
            // Mobius
            let n = (get_random_number() % 100_000) + 1;
            let res = mobius(n);
            vec![results_to_log(idx, &[], &[res as u64])]
        }
        50 => {
            // Dirichlet Convolution (1 * id)
            let n = (get_random_number() % 1_000) + 1;
            let res = dirichlet_convolution(n, |_| 1, |d| d);
            vec![results_to_log(idx, &[], &[res])]
        }
        51 => {
            // Jacobi Symbol (a/n)
            let n = ((get_random_number() % 999) | 1) + 2; // odd ≥3
            let a = (get_random_number() % (n as u64)) as i64;
            let res = jacobi(a.try_into().unwrap(), (n as i64).try_into().unwrap());
            vec![results_to_log(idx, &[], &[res as u64])]
        }
        52 => {
            // Cipolla's Algorithm
            let p = ((get_random_number() % 1000) | 1) + 2;
            let n = get_random_number() % p;
            match cipolla(n, p) {
                Some(r) => {
                    vec![results_to_log(idx, &[], &[r])]
                }
                None => {
                    vec![results_to_log(idx, &[], &[idx as u64])]
                }
            }
        }
        53..=u8::MAX => {
            vec![results_to_log(idx, &[], &[idx as u64])]
        }
    };

    let gas_end = unsafe { gas() };
    (gas_start - gas_end, result)
}

#[polkavm_derive::polkavm_export]
extern "C" fn refine(start_address: u64, length: u64) -> (u64, u64) {
    log_info(&format!("🚀 refine called with start_address=0x{:x}, length={}", start_address, length));
    // preallocate 100mb for output
    polkavm_sbrk(1000 * 1024 * 1024); 
    let args = if let Some(a) = parse_refine_args(start_address, length) {
        a
    } else {
        log_error("❌ refine: failed to parse args, returning empty_output");
        return empty_output();
    };

    let payload_ptr = args.wi_payload_start_address as *const u8;
    let payload_len = args.wi_payload_length as usize;
    let payload = unsafe { slice::from_raw_parts(payload_ptr, payload_len) };
    log_info(&format!("📦 refine: parsed args successfully, payload_len={}", payload_len));

    // run each (program_id, count) pair and create WriteIntents
    let pairs = payload_len / 2;
    log_info(&format!("🔄 refine: processing {} payload pairs", pairs));

    let mut write_intents = Vec::new();
    let mut total_gas_used = 0u64;
    let mut export_count: u16 = 0;

    for i in 0..pairs {
        let program_id = payload[i * 2];
        let p_id = program_id % 170;
        let count = payload[i * 2 + 1] as u64;
        let iterations = (count*count as u64).min(2); // Limit to 500 iterations to prevent overflow
        log_info(&format!("🔢 refine: pair #{} - program_id={}, count={}, iterations={} (capped)", i, program_id, count, iterations));
//        let iterations = count*count*count as u64; // Uncapped iterations (count^3)
//        log_info(&format!("🔢 refine: pair #{} - program_id={}, count={}, iterations={}", i, program_id, count, iterations));
        let mut gas_used = 0 as u64;
        let mut all_logs = Vec::new();
        let mut actual_iterations = 0;
        for iter in 0..iterations {
            if iter % 1000 == 0 {
                let gas_remaining = unsafe { gas() };
                log_info(&format!("🔄 refine: iteration {}/{} for program_id={}, gas_remaining={}", iter, iterations, p_id, gas_remaining));
                
                // Check if we're running low on gas (reserve 10M gas for cleanup)
                if gas_remaining < 10_000_000 {
                    log_info(&format!("⚠️ refine: stopping early at iteration {} due to low gas (remaining={})", iter, gas_remaining));
                    break;
                }
            }
            let (gas, logs) = run_program(p_id);
            gas_used += gas;
            all_logs.extend(logs);
            actual_iterations = iter + 1;
        }
        log_info(&format!("✅ refine: completed {}/{} iterations for program_id={}, gas_used={}", actual_iterations, iterations, p_id, gas_used));
        total_gas_used += gas_used;

        // Create a WriteIntent for this pair
        // Object ID: hash of (pair_index, program_id)
        let mut id_bytes = [0u8; 2];
        id_bytes[0] = i as u8;
        id_bytes[1] = program_id;
        let object_id = blake2b_hash(&id_bytes);

        // Receipt payload format: [logs_len:4][logs_payload]
        // Serialize logs using EVM log format
        let logs_payload = serialize_logs(&all_logs);
        // Encode a receipt-like structure: [logs_len][logs][status][cumulative_gas][log_index][tx_hash]
        let mut intent_payload = Vec::new();
        intent_payload.extend_from_slice(&(logs_payload.len() as u32).to_le_bytes());
        intent_payload.extend_from_slice(&logs_payload);
        intent_payload.extend_from_slice(&[1u8]); // status OK
        intent_payload.extend_from_slice(&(gas_used as u64).to_le_bytes());
        intent_payload.extend_from_slice(&(i as u32).to_le_bytes());

        let mut write_intent = WriteIntent {
            effect: WriteEffectEntry {
                object_id,
                ref_info: ObjectRef {
                    work_package_hash: args.wphash,
                    index_start: export_count,
                    payload_length: intent_payload.len() as u32,
                    object_kind: 3, // ObjectKind::Receipt
                },
                payload: intent_payload,
                tx_index: i as u32,
            },
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
        log_info(&format!("📝 refine: created WriteIntent for pair #{}, object_id={}", i, format_object_id(&object_id)));
    }
    log_info(&format!("🎯 refine: completed processing all {} pairs", pairs));

    // Create ExecutionEffects
    log_info(&format!("✅ refine: creating ExecutionEffects with {} write_intents, total_gas_used={}", write_intents.len(), total_gas_used));
    let execution_effects = ExecutionEffects {
        write_intents,
        contract_intents: Vec::new(),
    };
    let buffer = serialize_execution_effects(&execution_effects, 0, 0);
    log_info(&format!("🎯 refine returning buffer length={}", buffer.len()));
    leak_output(buffer)
}

#[unsafe(no_mangle)]
static mut output_bytes_32: [u8; 32] = [0; 32];

/// Accumulate orders all ExecutionEffects from refine calls and produces a final commitment across objects
#[polkavm_derive::polkavm_export]
pub extern "C" fn accumulate(start_address: u64, length: u64) -> (u64, u64) {
    let Some(args) = parse_accumulate_args(start_address, length) else {
        log_error( "Accumulate: parse_accumulate_args failed");
        return empty_output();
    };

    if args.num_accumulate_inputs == 0 {
        log_error( "Accumulate: num_accumulate_inputs is zero, returning empty");
        return empty_output();
    }

    let accumulate_inputs = match fetch_accumulate_inputs(args.num_accumulate_inputs as u64) {
        Ok(inputs) => inputs,
        Err(e) => {
            log_error(&format!("Accumulate: fetch_accumulate_inputs failed: {:?}", e));
            return empty_output();
        }
    };

    // Accumulate execution effects from any payloads (Transactions, Blocks)
    let Some(accumulate_root) = accumulator::BlockAccumulator::accumulate(args.s, args.t, &accumulate_inputs) else {
        return empty_output();
    };

    leak_output(accumulate_root.to_vec())
}