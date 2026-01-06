#![no_std]
#![no_main]

use polkavm_derive::min_stack_size;
use simplealloc::SimpleAlloc;

const STACK_SIZE: usize = 0x10000;
min_stack_size!(STACK_SIZE);

const HEAP_SIZE: usize = 0x10000;
#[global_allocator]
static ALLOCATOR: SimpleAlloc<HEAP_SIZE> = SimpleAlloc::new();

const DEFAULT_ITERS: u64 = 1_000_000;
const OUTPUT_LEN: u64 = 32;

static mut OUTPUT: [u8; 32] = [0; 32];

fn mul_upper_u_u(a: u64, b: u64) -> u64 {
    ((a as u128 * b as u128) >> 64) as u64
}

fn mul_upper_s_s(a: i64, b: i64) -> i64 {
    ((a as i128 * b as i128) >> 64) as i64
}

fn mul_upper_s_u(a: i64, b: u64) -> i64 {
    ((a as i128 * b as i128) >> 64) as i64
}

#[inline(never)]
fn cover_missing_ops(seed: u64) -> u64 {
    let mut v32 = seed as u32;
    let mut v64 = seed ^ 0x1234_5678;

    unsafe {
        let lhs32 = v32 as i32;
        let rhs32 = (v32 as i32).wrapping_add(1);
        let mut out32: i32;

        core::arch::asm!(
            "mulw {out}, {lhs}, {rhs}",
            out = lateout(reg) out32,
            lhs = in(reg) lhs32,
            rhs = in(reg) rhs32,
            options(nostack),
        );
        v32 = out32 as u32;

        let denom32 = (lhs32 | 1) as i32;
        core::arch::asm!(
            "remw {out}, {lhs}, {rhs}",
            out = lateout(reg) out32,
            lhs = in(reg) lhs32,
            rhs = in(reg) denom32,
            options(nostack),
        );
        v32 ^= out32 as u32;

        let lhs64 = v64 as i64;
        let denom64 = (lhs64 | 1) as i64;
        let mut out64: i64;
        core::arch::asm!(
            "rem {out}, {lhs}, {rhs}",
            out = lateout(reg) out64,
            lhs = in(reg) lhs64,
            rhs = in(reg) denom64,
            options(nostack),
        );
        v64 ^= out64 as u64;

        let mut shl32_imm: i32;
        core::arch::asm!(
            "slliw {out}, {lhs}, 3",
            out = lateout(reg) shl32_imm,
            lhs = in(reg) v32 as i32,
            options(nostack),
        );
        v32 ^= shl32_imm as u32;
    }

    let mulc32: u32 = 13;
    let mulc64: u64 = 23456;
    v32 = v32.wrapping_mul(mulc32);
    v64 = v64.wrapping_mul(mulc64);

    let sh32 = (v32 & 31) as u32;
    let sh64 = (v64 & 63) as u32;
    let const32: u32 = 37;
    let const64: u64 = 41;

    let alt_shl32 = const32.wrapping_shl(sh32);
    let alt_shr32 = const32.wrapping_shr(sh32);
    let alt_sar32 = ((const32 as i32) >> sh32) as u32;
    let alt_rot32 = const32.rotate_right(sh32);

    let alt_shl64 = const64.wrapping_shl(sh64);
    let alt_shr64 = const64.wrapping_shr(sh64);
    let alt_sar64 = ((const64 as i64) >> sh64) as u64;
    let alt_rot64 = const64.rotate_right(sh64);

    v32 ^= alt_shl32 ^ alt_shr32 ^ alt_sar32 ^ alt_rot32;
    v64 ^= alt_shl64 ^ alt_shr64 ^ alt_sar64 ^ alt_rot64;

    let cond = (v32 & 1) as u64;
    if cond == 0 {
        v64 = 0x1234;
    }

    let sh32_reg = (v32 & 31) as i32;
    let sh64_reg = (v64 & 63) as i64;
    let mut asm_srlw: i32;
    let mut asm_sraw: i32;
    let mut asm_mulw: i32;
    let mut asm_srl: i64;
    let mut asm_sra: i64;
    unsafe {
        core::arch::asm!(
            "addi {tmp32}, zero, 13",
            "srlw {out_srlw}, {tmp32}, {sh32}",
            "sraw {out_sraw}, {tmp32}, {sh32}",
            "mulw {out_mulw}, {tmp32}, {var32}",
            "addi {tmp64}, zero, 17",
            "srl {out_srl}, {tmp64}, {sh64}",
            "sra {out_sra}, {tmp64}, {sh64}",
            tmp32 = out(reg) _,
            out_srlw = lateout(reg) asm_srlw,
            out_sraw = lateout(reg) asm_sraw,
            out_mulw = lateout(reg) asm_mulw,
            tmp64 = out(reg) _,
            out_srl = lateout(reg) asm_srl,
            out_sra = lateout(reg) asm_sra,
            sh32 = in(reg) sh32_reg,
            sh64 = in(reg) sh64_reg,
            var32 = in(reg) v32 as i32,
            options(nostack),
        );
    }
    v32 ^= asm_srlw as u32;
    v32 ^= asm_sraw as u32;
    v32 ^= asm_mulw as u32;
    v64 ^= asm_srl as u64;
    v64 ^= asm_sra as u64;

    v64 ^ v32 as u64
}

fn read_iterations(start_address: u64, length: u64) -> u64 {
    if length == 0 || start_address == 0 {
        return DEFAULT_ITERS;
    }

    let len = core::cmp::min(length, usize::MAX as u64) as usize;
    if len == 0 {
        return DEFAULT_ITERS;
    }

    let slice = unsafe { core::slice::from_raw_parts(start_address as *const u8, len) };
    if slice.is_empty() {
        return DEFAULT_ITERS;
    }

    let mut buf = [0u8; 8];
    let copy_len = core::cmp::min(slice.len(), buf.len());
    buf[..copy_len].copy_from_slice(&slice[..copy_len]);
    u64::from_le_bytes(buf)
}

#[polkavm_derive::polkavm_export]
extern "C" fn main(start_address: u64, length: u64) -> (u64, u64) {
    let iterations = read_iterations(start_address, length);
    let per_op_iters = iterations;
    let mixed_iters = iterations;

    let mut a: u64 = 0x1234_5678_9abc_def0;
    let mut b: u64 = 0xfedc_ba98_7654_3210;
    let mut c: u64 = 0x9e37_79b9_7f4a_7c15;
    let mut d: u64 = 0x9ddf_ea08_eb38_2d69;
    let mut s64: i64 = -0x1234_5678_9abc_def;

    let mut a32: u32 = 0x1234_5678;
    let mut b32: u32 = 0x9abc_def0;
    let mut c32: u32 = 0x0f1e_2d3c;
    let mut d32: u32 = 0x4b5a_6978;
    let mut s32: i32 = -0x1234_567;

    let mut i = 0u64;
    while i < per_op_iters {
        a = a.wrapping_add(b);
        b = b.wrapping_sub(c);
        c = c.wrapping_mul(d | 1);
        d = d.wrapping_add(c / (a | 1));
        a = a.wrapping_add(c % (b | 1));

        a = a.wrapping_add(0x1234_5678_9abc_def0);
        b = b.wrapping_mul(7);
        d = 0x5a5a_5a5a_5a5a_5a5a_u64.wrapping_sub(d);

        let denom_s64 = (s64 | 1) as i128;
        let num_s64 = s64 as i128;
        let div_s64 = (num_s64 / denom_s64) as i64;
        let rem_s64 = (num_s64 % denom_s64) as i64;
        s64 = s64.wrapping_add(div_s64).wrapping_add(rem_s64);

        let mul_uu = mul_upper_u_u(a, b);
        let mul_ss = mul_upper_s_s(s64, b as i64);
        let mul_su = mul_upper_s_u(s64, c);
        c = c.wrapping_add(mul_uu);
        d = d.wrapping_add(mul_ss as u64);
        a = a.wrapping_add(mul_su as u64);

        let max_u = if a > b { a } else { b };
        let min_u = if a < b { a } else { b };
        let max_s = if s64 > d as i64 { s64 } else { d as i64 };
        let min_s = if s64 < c as i64 { s64 } else { c as i64 };
        b = b.wrapping_add(max_u ^ min_u);
        s64 = s64.wrapping_add(max_s ^ min_s);

        i = i.wrapping_add(1);
    }

    i = 0;
    while i < per_op_iters {
        a32 = a32.wrapping_add(b32);
        b32 = b32.wrapping_sub(c32);
        c32 = c32.wrapping_mul(d32 | 1);
        d32 = d32.wrapping_add(c32 / (a32 | 1));
        a32 = a32.wrapping_add(c32 % (b32 | 1));

        a32 = a32.wrapping_add(0x1234_5678);
        b32 = b32.wrapping_mul(5);
        d32 = 0x5a5a_5a5a_u32.wrapping_sub(d32);

        let denom_s32 = (s32 | 1) as i64;
        let num_s32 = s32 as i64;
        let div_s32 = (num_s32 / denom_s32) as i32;
        let rem_s32 = (num_s32 % denom_s32) as i32;
        s32 = s32.wrapping_add(div_s32).wrapping_add(rem_s32);

        i = i.wrapping_add(1);
    }

    i = 0;
    while i < per_op_iters {
        a &= b;
        b |= c;
        c ^= d;
        d = d & !a;
        a = a | !b;
        b = !(b ^ c);

        let sh64a = (a & 63) as u32;
        let sh64b = (b & 63) as u32;
        let sh64c = (c & 63) as u32;
        c = c.wrapping_shl(sh64a);
        d = d.wrapping_shr(sh64b);
        s64 >>= sh64c;

        a = a.wrapping_shl(3);
        b = b.wrapping_shr(5);
        s64 >>= 7;
        a = a.wrapping_shl(11);
        b = b.wrapping_shr(13);
        s64 >>= 17;

        a32 &= b32;
        b32 |= c32;
        c32 ^= d32;
        d32 = d32 & !a32;
        a32 = a32 | !b32;
        b32 = !(b32 ^ c32);

        let sh32a = (a32 & 31) as u32;
        let sh32b = (b32 & 31) as u32;
        let sh32c = (c32 & 31) as u32;
        c32 = c32.wrapping_shl(sh32a);
        d32 = d32.wrapping_shr(sh32b);
        s32 >>= sh32c;

        a32 = a32.wrapping_shl(2);
        b32 = b32.wrapping_shr(4);
        s32 >>= 6;
        a32 = a32.wrapping_shl(9);
        b32 = b32.wrapping_shr(12);
        s32 >>= 15;

        i = i.wrapping_add(1);
    }

    i = 0;
    while i < per_op_iters {
        let rot64 = (a & 63) as u32;
        a = a.rotate_left(rot64);
        b = b.rotate_right(rot64);
        c = c.rotate_right(7);
        d = d.rotate_right(19);

        let rot32 = (a32 & 31) as u32;
        a32 = a32.rotate_left(rot32);
        b32 = b32.rotate_right(rot32);
        c32 = c32.rotate_right(5);
        d32 = d32.rotate_right(11);

        let lt_u = if a < b { 1u64 } else { 0 };
        let lt_s = if s64 < b as i64 { 1u64 } else { 0 };
        let lt_u_imm = if a < 0x1234_5678 { 1u64 } else { 0 };
        let lt_s_imm = if s64 < -0x1234 { 1u64 } else { 0 };
        let gt_u_imm = if b > 0x9abc_def0 { 1u64 } else { 0 };
        let gt_s_imm = if s64 > 0x3456_i64 { 1u64 } else { 0 };
        c = c.wrapping_add(lt_u + lt_s + lt_u_imm + lt_s_imm + gt_u_imm + gt_s_imm);

        let flag64 = c & 1;
        a = if flag64 == 0 { b } else { a };
        b = if flag64 != 0 { c } else { b };
        d = if flag64 == 0 { 0x55aa_55aa_55aa_55aa } else { d };

        let flag32 = (a32 ^ b32) & 1;
        a32 = if flag32 == 0 { b32 } else { a32 };
        b32 = if flag32 != 0 { c32 } else { b32 };

        i = i.wrapping_add(1);
    }

    i = 0;
    while i < per_op_iters {
        let ones32 = a32.count_ones() as u32;
        let ones64 = a.count_ones() as u64;
        let lz32 = b32.leading_zeros() as u32;
        let lz64 = b.leading_zeros() as u64;
        let tz32 = c32.trailing_zeros() as u32;
        let tz64 = c.trailing_zeros() as u64;

        a32 = a32.wrapping_add(ones32).wrapping_add(lz32).wrapping_add(tz32);
        a = a.wrapping_add(ones64).wrapping_add(lz64).wrapping_add(tz64);

        let se8 = (a32 as i8) as i32;
        let se16 = (b32 as i16) as i32;
        let ze16 = (c32 as u16) as u32;
        s32 = s32.wrapping_add(se8).wrapping_add(se16);
        d32 = d32.wrapping_add(ze16);

        let se8_64 = (a as i8) as i64;
        let se16_64 = (b as i16) as i64;
        let ze16_64 = (c as u16) as u64;
        s64 = s64.wrapping_add(se8_64).wrapping_add(se16_64);
        d = d.wrapping_add(ze16_64);
        a = a.wrapping_add(d);

        a = a.swap_bytes();
        a32 = a32.swap_bytes();
        d = a;

        i = i.wrapping_add(1);
    }

    i = 0;
    while i < per_op_iters {
        let denom32 = s32 | 1;
        let denom64 = s64 | 1;

        let mul32 = s32.wrapping_mul((a32 as i32) | 1);
        let div32 = s32 / denom32;
        let rem32 = s32 % denom32;
        s32 = mul32 ^ div32 ^ rem32;

        let mul64 = s64.wrapping_mul((a as i64) | 1);
        let div64 = s64 / denom64;
        let rem64 = s64 % denom64;
        s64 = mul64 ^ div64 ^ rem64;

        a32 = a32.wrapping_mul(0x1357);
        a = a.wrapping_mul(0x1234_5678);

        a32 = a32.wrapping_shl(3);
        s32 >>= 2;
        a = a.wrapping_shl(7);
        s64 >>= 5;

        let sh32 = (a32 & 31) as u32;
        let sh64 = (a & 63) as u32;
        let alt_shl32 = 0x1234_5678_u32.wrapping_shl(sh32);
        let alt_shr32 = 0x8765_4321_u32.wrapping_shr(sh32);
        let alt_sar32 = (-0x1234_5678_i32) >> sh32;
        let alt_shl64 = 0x1234_5678_9abc_def0_u64.wrapping_shl(sh64);
        let alt_shr64 = 0xfedc_ba98_7654_3210_u64.wrapping_shr(sh64);
        let alt_sar64 = (-0x1234_5678_9abc_def_i64) >> sh64;
        let alt_rot32 = 0x89ab_cdef_u32.rotate_right(sh32);
        let alt_rot64 = 0x0123_4567_89ab_cdef_u64.rotate_right(sh64);

        a32 ^= alt_shl32 ^ alt_shr32 ^ (alt_sar32 as u32) ^ alt_rot32;
        a ^= alt_shl64 ^ alt_shr64 ^ (alt_sar64 as u64) ^ alt_rot64;

        let cond = (a32 & 1) as u64;
        b = if cond == 0 { 0x1234_5678_u64 } else { b };

        i = i.wrapping_add(1);
    }

    i = 0;
    while i < mixed_iters {
        a = a.wrapping_add(b ^ c);
        b = b.wrapping_mul(3).wrapping_add(d);
        c = c.wrapping_sub(a);
        d = d.wrapping_add(c / (a | 1));

        let sh = (a & 63) as u32;
        a = a.rotate_left(sh);
        b = b.wrapping_shr((b & 63) as u32);
        s64 >>= (c & 63) as u32;

        a32 = a32.wrapping_add(b32 ^ c32);
        b32 = b32.wrapping_mul(3).wrapping_add(d32);
        c32 = c32.wrapping_sub(a32);
        d32 = d32.wrapping_add(c32 / (a32 | 1));

        let sh32 = (a32 & 31) as u32;
        a32 = a32.rotate_left(sh32);
        b32 = b32.wrapping_shr((b32 & 31) as u32);
        s32 >>= (c32 & 31) as u32;

        let cmp = if a > b { 1u64 } else { 0 };
        a = if cmp == 0 { b } else { a };
        c = c.wrapping_add(cmp);

        let cnt = a32.count_ones() as u32;
        b32 = b32.wrapping_add(cnt);

        i = i.wrapping_add(1);
    }

    let cover = cover_missing_ops(a ^ b ^ c ^ d);

    let packed32 = ((a32 as u64) << 32) | b32 as u64;
    let packed32b = ((c32 as u64) << 32) | d32 as u64;

    unsafe {
        let out0 = a ^ s64 as u64;
        let out1 = b ^ s32 as u32 as u64;
        let out2 = c ^ packed32 ^ cover;
        let out3 = d ^ packed32b ^ iterations ^ cover;

        OUTPUT[0..8].copy_from_slice(&out0.to_le_bytes());
        OUTPUT[8..16].copy_from_slice(&out1.to_le_bytes());
        OUTPUT[16..24].copy_from_slice(&out2.to_le_bytes());
        OUTPUT[24..32].copy_from_slice(&out3.to_le_bytes());

        let out_ptr = core::ptr::addr_of_mut!(OUTPUT) as *mut u8;
        (out_ptr as u64, OUTPUT_LEN)
    }
}

#[cfg(all(not(test), target_arch = "riscv32", target_feature = "e"))]
#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    unsafe {
        core::arch::asm!("unimp", options(noreturn));
    }
}

#[cfg(not(all(not(test), target_arch = "riscv32", target_feature = "e")))]
#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
