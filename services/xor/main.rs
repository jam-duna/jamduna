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
    // #[polkavm_import(index = 3)]
    // pub fn write(key_ptr: *const u8, key_len: u32, value: *const u8, value_len: u32) -> u32;

    #[polkavm_import(index = 3)]
    pub fn write(ko: u32, kz: u32, bo: u32, bz: u32) -> u32;

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
    let mut register = [0u32; 2];

    // 使用輸出暫存器取得 a3 和 a4 的值
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

    let mut buffer = [0u8; 8];
    buffer[0..4].copy_from_slice(&omega_8.to_le_bytes());
    buffer[4..8].copy_from_slice(&omega_9.to_le_bytes());

    // XOR 前 4 位元組與後 4 位元組
    let mut result = [0u8; 4];
    result[0] = (buffer[0] | buffer[4]) & !(buffer[0] & buffer[4]);
    result[1] = (buffer[1] | buffer[5]) & !(buffer[1] & buffer[5]);
    result[2] = (buffer[2] | buffer[6]) & !(buffer[2] & buffer[6]);
    result[3] = (buffer[3] | buffer[7]) & !(buffer[3] & buffer[7]);

    // 回傳結果
    let buffer_addr = result.as_ptr() as u32;
    let buffer_len = result.len() as u32;

    unsafe {
        core::arch::asm!(
            "mv a3, {0}",
            "mv a4, {1}",
            in(reg) buffer_addr,
            in(reg) buffer_len,
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
