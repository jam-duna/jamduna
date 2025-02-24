#![no_std]
#![no_main]

extern crate alloc;

use simplealloc::SimpleAlloc;

#[global_allocator]
static ALLOCATOR: SimpleAlloc<4096> = SimpleAlloc::new();

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    unsafe {
        core::arch::asm!("unimp", options(noreturn));
    }
}

#[polkavm_derive::polkavm_import]
extern "C" {
    #[polkavm_import(index = 0)]
    pub fn gas() -> u64;
    // accumulate
    #[polkavm_import(index = 1)]
    pub fn lookup(s: u64, h: u64, o: u64, f: u64, l: u64) -> u64;
    #[polkavm_import(index = 2)]
    pub fn read(s: u64, ko: u64, kz: u64, o: u64, f: u64, l: u64) -> u64;
    #[polkavm_import(index = 3)]
    pub fn write(ko: u64, kz: u64, bo: u64, bz: u64) -> u64;
    #[polkavm_import(index = 4)]
    pub fn info(s: u64, o: u64) -> u64;
    #[polkavm_import(index = 5)]
    pub fn bless(m: u64, a: u64, v: u64, o: u64, n: u64) -> u64;
    #[polkavm_import(index = 6)]
    pub fn assign(c: u64, o: u64) -> u64;

    #[polkavm_import(index = 9)]
    pub fn new(o: u64, l: u64, g: u64, m: u64) -> u64;
    #[polkavm_import(index = 10)]
    pub fn upgrade(o: u64, g: u64, m: u64) -> u64;

    #[polkavm_import(index = 12)]
    pub fn eject(d: u64, o: u64) -> u64;
    #[polkavm_import(index = 13)]
    pub fn query(o: u64, z: u64) -> u64;
    #[polkavm_import(index = 14)]
    pub fn solicit(o: u64, z: u64) -> u64;
    #[polkavm_import(index = 15)]
    pub fn forget(o: u64, z: u64) -> u64;
    #[polkavm_import(index = 16)]
    pub fn oyield(o: u64) -> u64;

    // refine
    #[polkavm_import(index = 18)]
    pub fn fetch(start_address: u64, offset: u64, maxlen: u64, omega_10: u64, omega_11: u64, omega_12: u64) -> u64;
    #[polkavm_import(index = 19)]
    pub fn export(out: u64, out_len: u64) -> u64;
}

pub const NONE: u64 = u64::MAX;


fn write_result(result: u64, key: u8) {
    // TODO:  write_result
}

pub const NONE: u64 = u64::MAX;

#[polkavm_derive::polkavm_export]
extern "C" fn refine() -> u64 {
    let mut buffer = [0u8; 12];
    let offset: u64 = 0;
    let maxlen: u64 = buffer.len() as u64;
    let result = unsafe {
        fetch(
            buffer.as_mut_ptr() as u64,
            offset,
            maxlen,
            5,
            0,
            0,
        )
    };

    if result != NONE {
        let n = u32::from_le_bytes(buffer[0..4].try_into().unwrap());
        let fib_n = u32::from_le_bytes(buffer[4..8].try_into().unwrap());
        let fib_n_minus_1 = u32::from_le_bytes(buffer[8..12].try_into().unwrap());

        let new_fib_n = fib_n + fib_n_minus_1;

        buffer[0..4].copy_from_slice(&(n + 1).to_le_bytes());
        buffer[4..8].copy_from_slice(&new_fib_n.to_le_bytes());
        buffer[8..12].copy_from_slice(&fib_n.to_le_bytes());

    } else {
        buffer[0..4].copy_from_slice(&1_u32.to_le_bytes());
        buffer[4..8].copy_from_slice(&1_u32.to_le_bytes());
        buffer[8..12].copy_from_slice(&0_u32.to_le_bytes());
    }

    unsafe {
        export(buffer.as_ptr() as u64, buffer.len() as u64);
    }

    // set the output address to register a0 and output length to register a1
    let buffer_addr = buffer.as_ptr() as u64;
    let buffer_len = buffer.len() as u64;
    unsafe {
        core::arch::asm!(
            "mv a1, {0}",
            in(reg) buffer_len,
        );
    }
    // this equals to a0 = buffer_addr
    buffer_addr
}

#[polkavm_derive::polkavm_export]
extern "C" fn accumulate() -> u64 {
    let S: 64; // TODO fill in with the service being accumulated
    // read the input start address and length from register a0 and a1
    let omega_7: u64; // accumulate input start address
    let omega_8: u64; // accumulate input length

    unsafe {
        core::arch::asm!(
            "mv {0}, a0",
            "mv {1}, a1",
            out(reg) omega_7,
            out(reg) omega_8,
        );
    }

    // fetch all_accumulation_o
    let mut start_address = omega_7 + 4 + 4; // 4 bytes time slot + 4 bytes service index
    let mut remaining_length = omega_8 - 4 - 4; // 4 bytes time slot + 4 bytes service index
    let all_accumulation_o = unsafe { core::slice::from_raw_parts(start_address as *const u8, remaining_length as usize) };

    // fetch the number of accumulation_o
    let all_accumulation_o_discriminator_length = extract_discriminator(all_accumulation_o);
    let num_of_accumulation_o = decode_e(&all_accumulation_o[..all_accumulation_o_discriminator_length as usize]);

    // update the address pointer and remaining length
    start_address += all_accumulation_o_discriminator_length as u64;
    remaining_length -= all_accumulation_o_discriminator_length as u64;

    // set variables for storing work result address and length
    let mut work_result_address: u64 = 0;
    let mut work_result_length: u64 = 0;

    // set variables for storing auth output address and length
    let mut auth_output_address: u64 = 0;
    let mut auth_output_length: u64 = 0;

    for n in 0.. num_of_accumulation_o {
        // we only use the 0th accumulation_o
        if n > 0 {
            break;
        }
        // fetch work result prefix
        let accumulation_o = unsafe { core::slice::from_raw_parts(start_address as *const u8, remaining_length as usize) };
        let work_result_prefix = &accumulation_o[..1];

        start_address += 1;
        remaining_length -= 1;

        // fetch work result
        if work_result_prefix[0] == 0 {
            let accumulation_o = unsafe { core::slice::from_raw_parts(start_address as *const u8, remaining_length as usize) };
            let work_result_discriminator_length = extract_discriminator(accumulation_o);
            work_result_length = if work_result_discriminator_length > 0 {
                decode_e(&accumulation_o[..work_result_discriminator_length as usize])
            } else {
                0
            };

            start_address += work_result_discriminator_length as u64;
            remaining_length -= work_result_discriminator_length as u64;

            // store the work result address
            work_result_address = start_address;

            // update the address pointer and remaining length
            start_address += work_result_length as u64;
            remaining_length -= work_result_length as u64;
        }

        // skip l, k which are two 32 bytes hashes
        start_address += 32 + 32;
        remaining_length -= 32 + 32;

        // fetch auth output prefix
        let accumulation_o = unsafe { core::slice::from_raw_parts(start_address as *const u8, remaining_length as usize) };
        let auth_output_discriminator_length = extract_discriminator(accumulation_o);
        auth_output_length = if auth_output_discriminator_length > 0 {
            decode_e(&accumulation_o[..auth_output_discriminator_length as usize])
        } else {
            0
        };

        start_address += auth_output_discriminator_length as u64;
        remaining_length -= auth_output_discriminator_length as u64;

        // store the auth output address
        auth_output_address = start_address;

        // update the address pointer and remaining length
        start_address += auth_output_length as u64;
        remaining_length -= auth_output_length as u64;
    }

    // write FIB result to storage
    let key = [0u8; 1];
    let n: u64 = unsafe { ( *(work_result_address as *const u32)).into() };
    unsafe {
        write(key.as_ptr() as u64, key.len() as u64, work_result_address, work_result_length);
    }

    // Prepare some keys and hashes.
    let mut jamkey = [0u8; 3];
    let mut jamval = [0u8; 3];
    jamkey[0] = b'j';
    jamkey[1] = b'a';
    jamkey[2] = b'm';
    jamval[0] = b'D';
    jamval[1] = b'O';
    jamval[2] = b'T';

    let mut jamhash = [0u8; 32];
    // blake2b("jam") = 6454de314f3033b1487360f72ca5731ea11ede4880bbfb48415490766b2bbdf63efdc450b71b51ebf0303955ddef8e6dcabc2d0fb993b1992fb4cd894c9538f3
    jamhash[0] = b'a';
    jamhash[31] = b'f';

    let mut dothash = [0u8; 32];
    // blake2b("dot") = c4270f653225c64d617f65403194eace85530ddd5c1ab3283eadc5093f6d1d7721975d9f66d3a0d1f3d1769aa67b22155f3e29df7b94f5a7cfb6c64ac0e5d17b
    dothash[0] = b'b';
    dothash[31] = b'3';

    const JAM_ADDRESS: u64 = 0xFFFF1000;
    const JAMHASH_ADDRESS: u64 = 0xFFFF2000;
    const DOTHASH_ADDRESS: u64 = 0xFFFF3000;
    const INFO_ADDRESS: u64 = 0xFFFF4000;

    // Depending on what "n" is, test different host functions
    if n == 1 {
        let read_none_result = unsafe { read(S, jamkey.as_ptr() as u64, 3, JAM_ADDRESS, 0, 3) };
        write_result(read_none_result, 1);

        let write_result1 = unsafe { write(jamkey.as_ptr() as u64, jamkey.len() as u64, JAM_ADDRESS, 3) };
        write_result(write_result1, 2);

        let read_ok_result = unsafe { read(S, jamkey.as_ptr() as u64, 3, JAM_ADDRESS, 0, 3) };
        write_result(read_ok_result, 5);

        let forget_result = unsafe { forget(0, 0) };
        write_result(forget_result, 6);
    } else if n == 2 {
        let read_result = unsafe { read(S, jamkey.as_ptr() as u64, 3, JAM_ADDRESS, 0, 3) };
        write_result(read_result, 1);

        let write_result1 = unsafe { write(jamkey.as_ptr() as u64, jamkey.len() as u64, JAM_ADDRESS, 0) };
        write_result(write_result1, 2);

        let read_ok_result = unsafe { read(S, jamkey.as_ptr() as u64, 3, JAM_ADDRESS, 0, 3) };
        write_result(read_ok_result, 5);

        let solicit_result = unsafe { solicit(JAMHASH_ADDRESS, 3) };
        write_result(solicit_result, 6);
    } else if n == 3 {
        let solicit_result = unsafe { solicit(JAMHASH_ADDRESS, 3) };
        write_result(solicit_result, 1);

        let query_jamhash_result = unsafe { query(JAMHASH_ADDRESS, 3) };
        write_result(query_jamhash_result, 2);
        write_result(query_jamhash_result, 3);

        let query_none_result = unsafe { query(DOTHASH_ADDRESS, 3) };
        write_result(query_none_result, 5);

        let eject_result = unsafe { eject(DOTHASH_ADDRESS, 3) };
        write_result(eject_result, 6);
    } else if n == 4 {
        let lookup_none_result = unsafe { lookup(S, DOTHASH_ADDRESS, 3, 0, 3) };
        write_result(lookup_none_result, 5);

        let assign_result = unsafe { assign(1000, JAMHASH_ADDRESS) };
        write_result(assign_result, 6);
    } else if n == 5 {
        let lookup_result = unsafe { lookup(S, JAMHASH_ADDRESS, 3, 0, 3) };
        write_result(lookup_result, 1);

        let read_ok_result = unsafe { read(S, jamkey.as_ptr() as u64, 3, JAM_ADDRESS, 0, 3) };
        write_result(read_ok_result, 2);

        let eject_who_result = unsafe { eject(DOTHASH_ADDRESS, 3) };
        write_result(eject_who_result, 5);

        let overflow_s = 0xFFFFFFFFFFFFu64;
        let bless_who_result = unsafe { bless(overflow_s, 0, 0, DOTHASH_ADDRESS, 0) };
        write_result(bless_who_result, 6);
    } else if n == 6 {
        let solicit_result = unsafe { solicit(JAMHASH_ADDRESS, 3) };
        write_result(solicit_result, 1);

        let query_jamhash_result = unsafe { query(JAMHASH_ADDRESS, 3) };
        write_result(query_jamhash_result, 2);
        write_result(query_jamhash_result, 3);

        let new_authorization_queue_address = 0xFF00000;
        let assign_ok_result = unsafe { assign(new_authorization_queue_address, JAMHASH_ADDRESS) };
        write_result(assign_ok_result, 5);
    } else if n == 7 {
        let forget_result = unsafe { forget(JAMHASH_ADDRESS, 3) };
        write_result(forget_result, 1);

        let query_jamhash_result = unsafe { query(JAMHASH_ADDRESS, 3) };
        write_result(query_jamhash_result, 2);
        write_result(query_jamhash_result, 3);
    } else if n == 8 {
        let lookup_result = unsafe { lookup(S, DOTHASH_ADDRESS, 3, 0, 3) };
        write_result(lookup_result, 1);

        let query_jamhash_result = unsafe { query(JAMHASH_ADDRESS, 3) };
        write_result(query_jamhash_result, 2);
        write_result(query_jamhash_result, 3);
    } else if n == 9 {
        let eject_result = unsafe { eject(DOTHASH_ADDRESS, 3) };
        write_result(eject_result, 1);

        let g = 911911;
        let m = 911911;
        let new_result = unsafe { new(JAM_ADDRESS, 3, g, m) };
        write_result(new_result, 2);

        let upgrade_result = unsafe { upgrade(new_result, g, m) };
        write_result(upgrade_result, 5);

        let bless_ok_result = unsafe { bless(0, 1, 1, DOTHASH_ADDRESS, 0) };
        write_result(bless_ok_result, 6);
    } else if n == 10 {
        // nothing
    }

    // write info to 8
    key[0] = 8;
    let _info_result = unsafe { info(S, INFO_ADDRESS) };
    unsafe {
        write(key.as_ptr() as u64, key.len() as u64, INFO_ADDRESS, 100);
    }

    // write gas to 9
    key[0] = 9;
    let gas_result = unsafe { gas() };
    let gas_bytes = gas_result.to_le_bytes();
    unsafe {
        write(key.as_ptr() as u64, key.len() as u64, gas_bytes.as_ptr() as u64, 8);
    }

    // Prepare an output buffer (pad result to 32 bytes).
    let mut output_bytes_32 = [0u8; 32];
    unsafe {
        core::ptr::copy_nonoverlapping(
            work_result_address as *const u8,
            output_bytes_32.as_mut_ptr(),
            work_result_length as usize
        );
    }
    let result_address = output_bytes_32.as_ptr() as u64;
    let result_length = output_bytes_32.len() as u64;

    // write yield
    if n % 3 == 0 {
        unreachable!();
    } else if n % 2 == 0 {
        unsafe { oyield(output_bytes_32.as_ptr() as u64); }
    } else {
        unsafe { oyield(output_bytes_32.as_ptr() as u64); }
    }

    unsafe {
        core::arch::asm!(
            "mv a1, {0}",
            in(reg) result_length,
        );
    }
    result_address
}

// some helpful functions
fn extract_discriminator(input: &[u8]) -> u8 {
    if input.is_empty() {
        return 0;
    }

    let first_byte = input[0];
    match first_byte {
        1..=127 => 1,
        128..=191 => 2,
        192..=223 => 3,
        224..=239 => 4,
        240..=247 => 5,
        248..=251 => 6,
        252..=253 => 7,
        254..=u8::MAX => 8,
        _ => 0,
    }
}

fn power_of_two(exp: u32) -> u64 {
    1 << exp
}

fn decode_e_l(encoded: &[u8]) -> u64 {
    let mut x: u64 = 0;
    for &byte in encoded.iter().rev() {
        x = x.wrapping_mul(256).wrapping_add(byte as u64);
    }
    x
}

fn decode_e(encoded: &[u8]) -> u64 {
    let first_byte = encoded[0];
    if first_byte == 0 {
        return 0;
    }
    if first_byte == 255 {
        return decode_e_l(&encoded[1..9]);
    }
    for l in 0..8 {
        let left_bound  = 256 - power_of_two(8 - l);
        let right_bound = 256 - power_of_two(8 - (l + 1));

        if (first_byte as u64) >= left_bound && (first_byte as u64) < right_bound {
            let x1 = (first_byte as u64) - left_bound;
            let x2 = decode_e_l(&encoded[1..(1 + l as usize)]);
            let x = x1 * power_of_two(8 * l) + x2;
            return x;
        }
    }
    0
}
