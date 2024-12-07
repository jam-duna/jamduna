#![no_std]
#![no_main]

extern crate alloc;

use alloc::vec::Vec;
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
    pub fn gas() -> i64;
    #[polkavm_import(index = 1)]
    pub fn lookup(service: u32, hash_ptr: *const u8, out: *mut u8, out_len: u32) -> u32;
    #[polkavm_import(index = 2)]
    pub fn read(service: u32, key_ptr: *const u8, key_len: u32, out: *mut u8, out_len: u32) -> u32;
    #[polkavm_import(index = 3)]
    pub fn write(key_ptr: *const u8, key_len: u32, value: *const u8, value_len: u32) -> u32;
    #[polkavm_import(index = 4)]
    pub fn info(service: u32, out: *mut u8) -> u32;
    #[polkavm_import(index = 5)]
    pub fn empower(m: u32, a: u32, v: u32, o: u32, n: u32) -> u32;
    #[polkavm_import(index = 6)]
    pub fn assign(c: u32, out: *mut u8) -> u32;
    #[polkavm_import(index = 7)]
    pub fn designate(out: *mut u8) -> u32;
    #[polkavm_import(index = 8)]
    pub fn checkpoint() -> u64;
    #[polkavm_import(index = 9)]
    pub fn new(service: u32, hash_ptr: *const u8, out: *mut u8, out_len: u32) -> u32;
    #[polkavm_import(index = 10)]
    pub fn upgrade(out: *const u8, g: u64, m: u64) -> u32;
    #[polkavm_import(index = 11)]
    pub fn transfer(d: u32, a: u64, g: u64, out: *mut u8) -> u32;
    #[polkavm_import(index = 12)]
    pub fn quit(d: u32, a: u64, g: u64, out: *mut u8) -> u32;
    #[polkavm_import(index = 13)]
    pub fn solicit(hash_ptr: *const u8, z: u32) -> u32;
    #[polkavm_import(index = 14)]
    pub fn forget(hash_ptr: *const u8, z: u32) -> u32;
    #[polkavm_import(index = 15)]
    pub fn historical_lookup(service: u32, hash_ptr: *const u8, out: *mut u8, out_len: u32) -> u32;
    #[polkavm_import(index = 16)]
    pub fn import(import_index: u32, out: *mut u8, out_len: u32) -> u32;
    #[polkavm_import(index = 17)]
    pub fn export(out: *const u8, out_len: u32) -> u32;
    #[polkavm_import(index = 18)]
    pub fn machine(out: *const u8, out_len: u32) -> u32;
    #[polkavm_import(index = 19)]
    pub fn peek(out: *const u8, out_len: u32, i: u32) -> u32;
    #[polkavm_import(index = 20)]
    pub fn poke(n: u32, a: u32, b: u32, l: u32) -> u32;
    #[polkavm_import(index = 21)]
    pub fn invoke(n: u32, out: *mut u8) -> u32;
    #[polkavm_import(index = 22)]
    pub fn expunge(n: u32) -> u32;
    #[polkavm_import(index = 99)]
    pub fn blake2b(data: *const u8, data_len: u32, hash_ptr: *mut u8) -> u32;
    #[polkavm_import(index = 100)]
    pub fn blake2s(data: *const u8, data_len: u32, hash_ptr: *mut u8) -> u32;
    #[polkavm_import(index = 101)]
    pub fn ecrecover(h: *const u8, v: *const u8, r: *const u8, s: *const u8, out: *mut u8) -> u32;
    #[polkavm_import(index = 102)]
    pub fn sha2_256(data: *const u8, data_len: u32, hash_ptr: *mut u8) -> u32;
}

#[polkavm_derive::polkavm_export]
extern "C" fn is_authorized() -> u32 {
    0
}

#[polkavm_derive::polkavm_export]
extern "C" fn refine() -> u32 {
    // Input message
    let mut register = [0u32; 2];
    unsafe {
        core::arch::asm!(
            "mv {0}, a3",
            "mv {1}, a4",
            out(reg) register[0],
            out(reg) register[1],
        );
    }
    let omega_8: u32 = unsafe { *(register[0] as *const u32) };
    let omega_9: u32 = unsafe { *(register[1] as *const u32) };
    // omega_8 is the pointer to the input message
    let input_ptr = omega_8 as *const u8;
    // omega_9 is the length of the input message
    let input_len = omega_9 as u32;
    let input = unsafe { core::slice::from_raw_parts(input_ptr, input_len as usize) };
    // Constants definitions
    const KECCAK_ROUND_CONSTANTS: [u64; 24] = [
        0x0000000000000001,
        0x0000000000008082,
        0x800000000000808A,
        0x8000000080008000,
        0x000000000000808B,
        0x0000000080000001,
        0x8000000080008081,
        0x8000000000008009,
        0x000000000000008A,
        0x0000000000000088,
        0x0000000080008009,
        0x000000008000000A,
        0x000000008000808B,
        0x800000000000008B,
        0x8000000000008089,
        0x8000000000008003,
        0x8000000000008002,
        0x8000000000000080,
        0x000000000000800A,
        0x800000008000000A,
        0x8000000080008081,
        0x8000000000008080,
        0x0000000080000001,
        0x8000000080008008,
    ];

    const ROTATION_OFFSETS: [[u64; 5]; 5] = [
        [0, 36, 3, 41, 18],
        [1, 44, 10, 45, 2],
        [62, 6, 43, 15, 61],
        [28, 55, 25, 21, 56],
        [27, 20, 39, 8, 14],
    ];

    // State array initialization
    let mut state = [[0u64; 5]; 5];
    const RATE_IN_BYTES: usize = 1088 / 8; // The bitrate of Keccak-256 is 1088 bits, i.e., 136 bytes

    // Padding
    let mut padded_input = [0u8; RATE_IN_BYTES * 2]; // Max size in case we need two blocks
    let mut padded_input_len = 0;

    // Copy the input into padded_input
    for &byte in input.iter() {
        padded_input[padded_input_len] = byte;
        padded_input_len += 1;
    }

    // Append 0x01
    padded_input[padded_input_len] = 0x01;
    padded_input_len += 1;

    // Pad zeros until (padded_input_len % RATE_IN_BYTES) == RATE_IN_BYTES - 1
    while (padded_input_len % RATE_IN_BYTES) != (RATE_IN_BYTES - 1) {
        padded_input[padded_input_len] = 0x00;
        padded_input_len += 1;
    }

    // Append 0x80
    padded_input[padded_input_len] = 0x80;
    padded_input_len += 1;

    // Now padded_input_len should be a multiple of RATE_IN_BYTES
    assert_eq!(padded_input_len % RATE_IN_BYTES, 0);

    // Absorption phase
    let num_blocks = padded_input_len / RATE_IN_BYTES;
    for block_index in 0..num_blocks {
        let chunk = &padded_input[block_index * RATE_IN_BYTES..(block_index + 1) * RATE_IN_BYTES];
        for i in 0..(RATE_IN_BYTES / 8) {
            let mut bytes = [0u8; 8];
            let start = i * 8;
            let end = start + 8;
            bytes.copy_from_slice(&chunk[start..end]);
            let block = u64::from_le_bytes(bytes);
            let x = i % 5;
            let y = i / 5;
            state[x][y] ^= block;
        }

        // Keccak-f[1600] permutation
        for round in 0..24 {
            // θ (Theta) step
            let mut c = [0u64; 5];
            for x in 0..5 {
                c[x] = state[x][0]
                    ^ state[x][1]
                    ^ state[x][2]
                    ^ state[x][3]
                    ^ state[x][4];
            }

            let mut d = [0u64; 5];
            for x in 0..5 {
                d[x] = c[(x + 4) % 5] ^ c[(x + 1) % 5].rotate_left(1);
            }

            for x in 0..5 {
                for y in 0..5 {
                    state[x][y] ^= d[x];
                }
            }

            // ρ (Rho) and π (Pi) steps
            let mut b = [[0u64; 5]; 5];
            for x in 0..5 {
                for y in 0..5 {
                    b[y][(2 * x + 3 * y) % 5] =
                        state[x][y].rotate_left(ROTATION_OFFSETS[x][y] as u32);
                }
            }

            // χ (Chi) step
            for x in 0..5 {
                for y in 0..5 {
                    state[x][y] = b[x][y] ^ ((!b[(x + 1) % 5][y]) & b[(x + 2) % 5][y]);
                }
            }

            // ι (Iota) step
            state[0][0] ^= KECCAK_ROUND_CONSTANTS[round];
        }
    }

    // Squeezing phase
    let mut output = [0u8; 32]; // Fixed-size output array
    let mut output_offset = 0;
    while output_offset < output.len() {
        for y in 0..5 {
            for x in 0..5 {
                let lane_bytes = state[x][y].to_le_bytes();
                for &byte in lane_bytes.iter() {
                    if output_offset < output.len() {
                        output[output_offset] = byte;
                        output_offset += 1;
                    } else {
                        break;
                    }
                }
                if output_offset >= output.len() {
                    break;
                }
            }
            if output_offset >= output.len() {
                break;
            }
        }

        if output_offset >= output.len() {
            break;
        }

        // Apply Keccak-f[1600] permutation again
        for round in 0..24 {
            // θ (Theta) step
            let mut c = [0u64; 5];
            for x in 0..5 {
                c[x] = state[x][0]
                    ^ state[x][1]
                    ^ state[x][2]
                    ^ state[x][3]
                    ^ state[x][4];
            }

            let mut d = [0u64; 5];
            for x in 0..5 {
                d[x] = c[(x + 4) % 5] ^ c[(x + 1) % 5].rotate_left(1);
            }

            for x in 0..5 {
                for y in 0..5 {
                    state[x][y] ^= d[x];
                }
            }

            // ρ (Rho) and π (Pi) steps
            let mut b = [[0u64; 5]; 5];
            for x in 0..5 {
                for y in 0..5 {
                    b[y][(2 * x + 3 * y) % 5] =
                        state[x][y].rotate_left(ROTATION_OFFSETS[x][y] as u32);
                }
            }

            // χ (Chi) step
            for x in 0..5 {
                for y in 0..5 {
                    state[x][y] = b[x][y] ^ ((!b[(x + 1) % 5][y]) & b[(x + 2) % 5][y]);
                }
            }

            // ι (Iota) step
            state[0][0] ^= KECCAK_ROUND_CONSTANTS[round];
        }
    }
    // Change output vec to u8 array
    let mut output_array = [0u8; 32];
    output_array.copy_from_slice(&output[..32]);
    let output_ptr = output_array.as_ptr();
    let output_len = output_array.len() as u32;
    unsafe {
        core::arch::asm!(
            "mv a3, {0}",
            "mv a4, {1}",
            in(reg) output_ptr,
            in(reg) output_len,
        );
    }
    0
}

#[polkavm_derive::polkavm_export]
extern "C" fn accumulate() -> u32 {
    0
}

#[polkavm_derive::polkavm_export]
extern "C" fn on_transfer() -> u32 {
    0
}


